package fastly

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fastlysdk "github.com/fastly/go-fastly/fastly"
	provider "github.com/magelift/magelift/internal/provider"
)

func TestNativeServiceDecodesFastlyScalarAndObjectVersionShapes(t *testing.T) {
	tests := []struct {
		name          string
		payload       string
		activeVersion int
		version       int
	}{
		{
			name:          "service list shape",
			payload:       `{"id":"svc-1","name":"shop","comment":"magelift/test","active_version":3,"version":4}`,
			activeVersion: 3,
			version:       4,
		},
		{
			name:          "service detail shape",
			payload:       `{"id":"svc-1","name":"shop","comment":"magelift/test","active_version":{"number":3},"version":{"number":4}}`,
			activeVersion: 3,
			version:       4,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var service NativeService
			if err := json.Unmarshal([]byte(test.payload), &service); err != nil {
				t.Fatal(err)
			}
			if service.ActiveVersion != test.activeVersion || service.Version != test.version {
				t.Fatalf("service = %#v", service)
			}
		})
	}
}

func TestSDKClientUsesContextAndOwningInventory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Fastly-Key") != "test-token" {
			t.Fatalf("Fastly authentication header = %q", request.Header.Get("Fastly-Key"))
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/service":
			if request.Method != http.MethodGet {
				writer.WriteHeader(http.StatusCreated)
				_, _ = writer.Write([]byte(`{"id":"svc-1","comment":"magelift/test"}`))
				return
			}
			_, _ = writer.Write([]byte(`[{"id":"svc-1","comment":"magelift/test","active_version":3},{"id":"svc-user","comment":"user"}]`))
		case "/service/svc-1/domain":
			_, _ = writer.Write([]byte(`[{"id":"dom-1","fqdn":"shop.example","comment":"magelift/test"}]`))
		default:
			t.Fatalf("unexpected Fastly path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client, err := fastlysdk.NewClientForEndpoint("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	api, err := NewSDKClientFromClient(client)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := api.Inventory(context.Background(), "magelift/test")
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 2 || resources[0].Identity != "service:svc-1" || resources[1].Identity != "domain:dom-1" {
		t.Fatalf("inventory = %#v", resources)
	}
}

func TestSDKClientCredentialReferenceNeverAppearsInErrors(t *testing.T) {
	secret := "token=super-secret-value"
	resolver := provider.CredentialResolverFunc(func(_ context.Context, _ string, consume func([]byte) error) error {
		return consume([]byte(secret))
	})
	_, err := NewSDKClient(context.Background(), resolver, "aws-secrets-manager://magelift/fastly")
	if err != nil && strings.Contains(err.Error(), secret) {
		t.Fatal("Fastly credential value escaped the provider error boundary")
	}
	_ = json.Valid
}

func TestSDKClientTokenLifecycleUsesOfficialAPIEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Fastly-Key") != "management-token" {
			t.Fatalf("Fastly token authentication header = %q", request.Header.Get("Fastly-Key"))
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/tokens/self":
			_, _ = writer.Write([]byte(`{"id":"old-token","scope":"global"}`))
		case "/automation-tokens":
			if request.Method != http.MethodPost {
				t.Fatalf("token create method = %s", request.Method)
			}
			_, _ = writer.Write([]byte(`{"id":"new-token","access_token":"replacement-secret","scope":"global","services":["svc-1"]}`))
		case "/automation-tokens/new-token":
			if request.Method != http.MethodDelete {
				t.Fatalf("token revoke method = %s", request.Method)
			}
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected Fastly token path %s", request.URL.Path)
		}
	}))
	defer server.Close()
	client, err := fastlysdk.NewClientForEndpoint("management-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	api, err := NewSDKClientFromClient(client)
	if err != nil {
		t.Fatal(err)
	}
	current, err := api.CurrentToken(context.Background())
	if err != nil || current.ID != "old-token" {
		t.Fatalf("current token = %#v, err = %v", current, err)
	}
	created, err := api.CreateToken(context.Background(), "magelift/test", "global", []string{"svc-1"})
	if err != nil || created.ID != "new-token" || created.AccessToken != "replacement-secret" {
		t.Fatalf("created token = %#v, err = %v", created, err)
	}
	if err := api.RevokeToken(context.Background(), created.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSDKClientFeatureLifecycleUsesDocumentedVCLAndTLSAPIEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Fastly-Key") != "test-token" {
			t.Fatalf("Fastly authentication header = %q", request.Header.Get("Fastly-Key"))
		}
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/service/svc-1/version/3/vcl":
			_, _ = writer.Write([]byte(`[{"service_id":"svc-1","version":3,"name":"magelift-main","main":true,"content":"sub vcl_recv {}"}]`))
		case request.Method == http.MethodPost && request.URL.Path == "/service/svc-1/version/3/vcl":
			if err := request.ParseForm(); err != nil {
				t.Fatalf("parse VCL form: %v", err)
			}
			if request.Form.Get("name") != "magelift-main" || request.Form.Get("content") != "sub vcl_recv {}" || request.Form.Get("main") != "1" {
				t.Fatalf("VCL form = %#v", request.Form)
			}
			_, _ = writer.Write([]byte(`{"service_id":"svc-1","version":3,"name":"magelift-main","main":true,"content":"sub vcl_recv {}"}`))
		case request.Method == http.MethodPut && request.URL.Path == "/service/svc-1/version/3/vcl/magelift-main/main":
			writer.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodDelete && request.URL.Path == "/service/svc-1/version/3/vcl/magelift-main":
			writer.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodGet && request.URL.Path == "/tls/subscriptions":
			_, _ = writer.Write([]byte(`{"data":[{"id":"tls-existing","type":"tls_subscription","attributes":{"state":"issued"},"relationships":{"tls_domains":{"data":[{"id":"shop.example"}]}}}]}`))
		case request.Method == http.MethodPost && request.URL.Path == "/tls/subscriptions":
			var payload map[string]any
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatalf("decode TLS request: %v", err)
			}
			data, ok := payload["data"].(map[string]any)
			if !ok || data["type"] != "tls_subscription" {
				t.Fatalf("TLS request data = %#v", payload["data"])
			}
			_, _ = writer.Write([]byte(`{"data":{"id":"tls-1","type":"tls_subscription","attributes":{"state":"pending"},"relationships":{"tls_domains":{"data":[{"id":"shop.example"}]}}}}`))
		case request.Method == http.MethodDelete && request.URL.Path == "/tls/subscriptions/tls-1":
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected Fastly feature request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	client, err := fastlysdk.NewClientForEndpoint("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	api, err := NewSDKClientFromClient(client)
	if err != nil {
		t.Fatal(err)
	}

	vcls, err := api.ListVCLs(context.Background(), "svc-1", 3)
	if err != nil || len(vcls) != 1 || vcls[0].Name != "magelift-main" || !vcls[0].Main {
		t.Fatalf("listed VCLs = %#v, err = %v", vcls, err)
	}
	createdVCL, err := api.CreateVCL(context.Background(), "svc-1", 3, "magelift-main", "sub vcl_recv {}")
	if err != nil || createdVCL.Name != "magelift-main" {
		t.Fatalf("created VCL = %#v, err = %v", createdVCL, err)
	}
	if err := api.SetVCLMain(context.Background(), "svc-1", 3, "magelift-main"); err != nil {
		t.Fatal(err)
	}
	if err := api.DeleteVCL(context.Background(), "svc-1", 3, "magelift-main"); err != nil {
		t.Fatal(err)
	}

	subscriptions, err := api.ListTLSSubscriptions(context.Background())
	if err != nil || len(subscriptions) != 1 || subscriptions[0].ID != "tls-existing" || subscriptions[0].Domains[0] != "shop.example" {
		t.Fatalf("listed TLS subscriptions = %#v, err = %v", subscriptions, err)
	}
	createdTLS, err := api.CreateTLSSubscription(context.Background(), []string{"shop.example"}, "shop.example")
	if err != nil || createdTLS.ID != "tls-1" || createdTLS.State != "pending" {
		t.Fatalf("created TLS subscription = %#v, err = %v", createdTLS, err)
	}
	if err := api.DeleteTLSSubscription(context.Background(), createdTLS.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSDKClientVersionLifecycleUsesDocumentedCloneValidateAndActivateEndpoints(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Fastly-Key") != "test-token" {
			t.Fatalf("Fastly authentication header = %q", request.Header.Get("Fastly-Key"))
		}
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodPut && request.URL.Path == "/service/svc-1/version/3/clone":
			_, _ = writer.Write([]byte(`{"number":4,"service_id":"svc-1","active":false,"locked":false}`))
		case request.Method == http.MethodGet && request.URL.Path == "/service/svc-1/version/4/validate":
			_, _ = writer.Write([]byte(`{"status":"ok","msg":"valid"}`))
		case request.Method == http.MethodPut && request.URL.Path == "/service/svc-1/version/4/activate":
			_, _ = writer.Write([]byte(`{"number":4,"service_id":"svc-1","active":true,"locked":true}`))
		default:
			t.Fatalf("unexpected Fastly version request %s %s", request.Method, request.URL.Path)
		}
	}))
	defer server.Close()
	client, err := fastlysdk.NewClientForEndpoint("test-token", server.URL)
	if err != nil {
		t.Fatal(err)
	}
	api, err := NewSDKClientFromClient(client)
	if err != nil {
		t.Fatal(err)
	}

	cloned, err := api.CloneVersion(context.Background(), "svc-1", 3)
	if err != nil || cloned.Number != 4 || cloned.ServiceID != "svc-1" {
		t.Fatalf("cloned version = %#v, err = %v", cloned, err)
	}
	validation, err := api.ValidateVersion(context.Background(), "svc-1", cloned.Number)
	if err != nil || validation.Status != "ok" || validation.Message != "valid" {
		t.Fatalf("version validation = %#v, err = %v", validation, err)
	}
	activated, err := api.ActivateVersion(context.Background(), "svc-1", cloned.Number)
	if err != nil || activated.Number != 4 || !activated.Active || !activated.Locked {
		t.Fatalf("activated version = %#v, err = %v", activated, err)
	}
}
