package newrelic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

type memoryCredentialSink struct {
	values map[string][]byte
}

func (sink *memoryCredentialSink) Store(_ context.Context, reference string, value []byte) error {
	if sink.values == nil {
		sink.values = make(map[string][]byte)
	}
	sink.values[reference] = append([]byte(nil), value...)
	return nil
}

func TestCredentialLifecycleUsesSeparateManagementKeyAndRedactedSecretSink(t *testing.T) {
	const managementKey = "management-user-key"
	const generatedKey = "generated-license-key"
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("API-Key"); got != managementKey {
			t.Errorf("API-Key = %q, want management key", got)
		}
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode GraphQL request: %v", err)
			return
		}
		if strings.Contains(body.Query, managementKey) || strings.Contains(body.Query, generatedKey) {
			t.Errorf("GraphQL query contains a secret")
		}
		response.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(body.Query, "requestContext"):
			_, _ = response.Write([]byte(`{"data":{"requestContext":{"userId":"user-1"}}}`))
		case strings.Contains(body.Query, "apiAccessCreateKeys"):
			_, _ = response.Write([]byte(`{"data":{"apiAccessCreateKeys":{"createdKeys":[{"id":"ingest-id-1","key":"generated-license-key","type":"INGEST"}],"errors":[]}}}`))
		case strings.Contains(body.Query, "apiAccessDeleteKeys"):
			_, _ = response.Write([]byte(`{"data":{"apiAccessDeleteKeys":{"deletedKeys":[{"id":"ingest-id-1"}],"errors":[]}}}`))
		default:
			t.Errorf("unexpected GraphQL query %q", body.Query)
			_, _ = response.Write([]byte(`{"errors":[{"message":"unexpected query"}]}`))
		}
	}))
	t.Cleanup(server.Close)

	resolver := provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
		if reference != "newrelic://management" {
			t.Fatalf("resolved reference = %q", reference)
		}
		return consume([]byte(managementKey))
	})
	sink := &memoryCredentialSink{}
	client, err := NewCredentialLifecycleClient(server.Client(), resolver, sink, server.URL, 12345)
	if err != nil {
		t.Fatal(err)
	}
	base := sdk.CredentialLifecycleRequest{
		Provider:                "newrelic",
		CredentialRef:           "aws-secrets-manager://magelift/newrelic-license",
		ManagementCredentialRef: "newrelic://management",
		OwnershipMarker:         "magelift/test/newrelic",
		IdempotencyKey:          "credential-1",
	}

	validated, err := sdk.RunCredentialLifecycle(context.Background(), client, sdk.CredentialLifecycleRequest{Action: sdk.CredentialValidate, Provider: base.Provider, CredentialRef: base.CredentialRef, ManagementCredentialRef: base.ManagementCredentialRef, OwnershipMarker: base.OwnershipMarker, IdempotencyKey: base.IdempotencyKey})
	if err != nil {
		t.Fatal(err)
	}
	if validated.OperationID == "" || !validated.ScopeVerified || !validated.RedactionVerified {
		t.Fatalf("validation result = %#v", validated)
	}

	rotatedRequest := base
	rotatedRequest.Action = sdk.CredentialRotate
	rotated, err := sdk.RunCredentialLifecycle(context.Background(), client, rotatedRequest)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.CredentialVersion != "ingest-id-1" || string(sink.values[base.CredentialRef]) != generatedKey {
		t.Fatalf("rotation result/sink = %#v/%q", rotated, sink.values[base.CredentialRef])
	}
	if strings.Contains(rotated.OperationID, generatedKey) {
		t.Fatalf("rotation operation identity contains secret: %q", rotated.OperationID)
	}

	revokedRequest := base
	revokedRequest.Action = sdk.CredentialRevoke
	revokedRequest.CredentialVersion = rotated.CredentialVersion
	revoked, err := sdk.RunCredentialLifecycle(context.Background(), client, revokedRequest)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.CredentialVersion != rotated.CredentialVersion || !revoked.ScopeVerified || !revoked.RedactionVerified {
		t.Fatalf("revocation result = %#v", revoked)
	}
}

func TestCredentialLifecycleNormalizesResolverFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("NerdGraph should not be called when credential resolution fails")
	}))
	t.Cleanup(server.Close)
	secret := "management-secret-that-must-not-escape"
	client, err := NewCredentialLifecycleClient(server.Client(), provider.CredentialResolverFunc(func(context.Context, string, func([]byte) error) error {
		return errors.New(secret)
	}), &memoryCredentialSink{}, server.URL, 12345)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.ExecuteCredentialLifecycle(context.Background(), sdk.CredentialLifecycleRequest{
		Action: sdk.CredentialValidate, Provider: "newrelic", CredentialRef: "newrelic://management",
		OwnershipMarker: "magelift/test/redaction", IdempotencyKey: "validate-1",
	})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("resolver error = %v", err)
	}
}
