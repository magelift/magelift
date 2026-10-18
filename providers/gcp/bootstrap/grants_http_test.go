package bootstrap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	artifactregistry "google.golang.org/api/artifactregistry/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/option"
	secretmanager "google.golang.org/api/secretmanager/v1"
	storagev1 "google.golang.org/api/storage/v1"
)

func TestRepositoryIAMClientRequestsPolicyVersion3(t *testing.T) {
	t.Parallel()
	const resource = "projects/p/locations/europe-west1/repositories/r"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, ":getIamPolicy") {
			if r.URL.Query().Get("options.requestedPolicyVersion") != "3" {
				t.Errorf("requestedPolicyVersion=%q", r.URL.Query().Get("options.requestedPolicyVersion"))
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"version": float64(3), "etag": "ar"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client, err := NewRepositoryIAMClient(context.Background(),
		option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	policy, err := client.GetRepositoryIAMPolicy(context.Background(), resource)
	if err != nil {
		t.Fatal(err)
	}
	if policy.Version != 3 || policy.Etag != "ar" {
		t.Fatalf("policy=%#v", policy)
	}
}

func TestBucketIAMClientSetPreservesAuditAndConditions(t *testing.T) {
	t.Parallel()
	var setBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/iam") {
			setBody, _ = io.ReadAll(r.Body)
			writeJSON(w, map[string]any{"version": float64(3), "etag": "new"})
			return
		}
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/iam") {
			if r.URL.Query().Get("optionsRequestedPolicyVersion") != "3" {
				t.Errorf("requestedPolicyVersion=%q", r.URL.Query().Get("optionsRequestedPolicyVersion"))
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{
				"version": float64(3),
				"etag":    "old",
				"bindings": []map[string]any{{
					"role":    "roles/storage.objectViewer",
					"members": []string{"user:admin@example.com"},
					"condition": map[string]any{
						"title":      "keep",
						"expression": "request.time < timestamp('2099-01-01T00:00:00Z')",
					},
				}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	service, err := storagev1.NewService(context.Background(),
		option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	client := &sdkBucketIAM{client: service}
	policy, err := client.GetBucketIAMPolicy(context.Background(), "bucket")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := mergeUnconditionalStorageMember(policy, roleStorageObjectAdmin, "serviceAccount:deploy@example.iam.gserviceaccount.com")
	updated.Etag = policy.Etag
	if err := client.SetBucketIAMPolicy(context.Background(), "bucket", updated); err != nil {
		t.Fatal(err)
	}
	body := string(setBody)
	if !strings.Contains(body, `"condition"`) || !strings.Contains(body, `"etag":"old"`) {
		t.Fatalf("set body dropped metadata: %s", body)
	}
}

func TestProjectIAMClientGetRequestsVersion3(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":getIamPolicy") {
			var req cloudresourcemanager.GetIamPolicyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("decode body: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if req.Options == nil || req.Options.RequestedPolicyVersion != 3 {
				t.Errorf("requested version=%v", req.Options)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			writeJSON(w, map[string]any{"version": float64(3), "etag": "crm"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client, err := NewProjectIAMClient(context.Background(),
		option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	policy, err := client.GetProjectIAMPolicy(context.Background(), "example-gcp-project")
	if err != nil {
		t.Fatal(err)
	}
	if policy.Etag != "crm" {
		t.Fatalf("policy=%#v", policy)
	}
}

func TestSecretIAMClientRoundTripUsesNativePolicy(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, ":getIamPolicy") {
			writeJSON(w, map[string]any{"version": float64(3), "etag": "sec"})
			return
		}
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":setIamPolicy") {
			writeJSON(w, map[string]any{"version": float64(3), "etag": "sec2"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	service, err := secretmanager.NewService(context.Background(),
		option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	client := &sdkSecretIAM{client: service}
	resource := "projects/p/secrets/s"
	policy, err := client.GetSecretIAMPolicy(context.Background(), resource)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := mergeUnconditionalSecretMember(policy, roleSecretAccessor, "serviceAccount:deploy@example.iam.gserviceaccount.com")
	updated.Etag = policy.Etag
	if err := client.SetSecretIAMPolicy(context.Background(), resource, updated); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactRegistrySetIAMUsesNativeRequest(t *testing.T) {
	t.Parallel()
	var setBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, ":setIamPolicy") {
			setBody, _ = io.ReadAll(r.Body)
			writeJSON(w, map[string]any{"version": float64(3)})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	service, err := artifactregistry.NewService(context.Background(),
		option.WithHTTPClient(srv.Client()), option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	client := &sdkRepositoryIAM{client: service}
	policy := &artifactregistry.Policy{
		Version: 3,
		Etag:    "e",
		Bindings: []*artifactregistry.Binding{{
			Role: roleArtifactRegistryWriter, Members: []string{"serviceAccount:build@example.iam.gserviceaccount.com"},
		}},
	}
	if err := client.SetRepositoryIAMPolicy(context.Background(), "projects/p/locations/europe-west1/repositories/r", policy); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(setBody), roleArtifactRegistryWriter) {
		t.Fatalf("set body=%s", string(setBody))
	}
}
