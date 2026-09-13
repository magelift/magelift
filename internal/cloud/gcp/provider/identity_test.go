package gcpprovider

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"testing"

	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type projectReaderFunc func(context.Context, string) (ProjectIdentity, error)

func (f projectReaderFunc) GetProject(ctx context.Context, id string) (ProjectIdentity, error) {
	return f(ctx, id)
}

type permissionVerifierFunc func(context.Context, sdk.ProviderID, string, string, []string) (bool, error)

func (f permissionVerifierFunc) Verify(ctx context.Context, p sdk.ProviderID, project, principal string, permissions []string) (bool, error) {
	return f(ctx, p, project, principal, permissions)
}

func TestIdentityClientChecksProjectAndPermissionScope(t *testing.T) {
	client, err := NewIdentityFromProjectReader(projectReaderFunc(func(_ context.Context, id string) (ProjectIdentity, error) {
		return ProjectIdentity{ID: id, Number: 1234}, nil
	}), permissionVerifierFunc(func(_ context.Context, provider sdk.ProviderID, project, principal string, permissions []string) (bool, error) {
		if provider != "gcp" || project != "shop-prod" || principal != "projects/1234" || len(permissions) != 1 {
			t.Fatalf("scope request = %q %q %q %#v", provider, project, principal, permissions)
		}
		return true, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := client.CheckIdentity(context.Background(), provider.IdentityCheckRequest{Provider: "gcp", AccountOrProjectRef: "shop-prod", Region: "europe-west1", RequiredPermissions: []string{"container.clusters.get"}})
	if err != nil || !observation.LeastPrivilegeVerified || observation.PrincipalRef != "projects/1234" {
		t.Fatalf("CheckIdentity() = %#v, %v", observation, err)
	}
}

func TestIdentityClientDoesNotAdmitWithoutScopeProof(t *testing.T) {
	client, err := NewIdentityFromProjectReader(projectReaderFunc(func(context.Context, string) (ProjectIdentity, error) {
		return ProjectIdentity{ID: "shop-prod"}, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := client.CheckIdentity(context.Background(), provider.IdentityCheckRequest{Provider: "gcp", AccountOrProjectRef: "shop-prod", Region: "europe-west1", RequiredPermissions: []string{"container.clusters.get"}})
	if err != nil || observation.LeastPrivilegeVerified {
		t.Fatalf("unverified scope = %#v, %v", observation, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.CheckIdentity(ctx, provider.IdentityCheckRequest{Provider: "gcp", AccountOrProjectRef: "shop-prod", Region: "europe-west1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled identity error = %v", err)
	}
}

func TestNewIdentityWithCredentialBuildsProviderClientInsideResolver(t *testing.T) {
	const credentialRef = "vault://magelift/gcp-identity"
	payload := testServiceAccountJSON(t)
	var resolved bool
	client, err := NewIdentityWithCredential(context.Background(), provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
		if reference != credentialRef {
			t.Fatalf("credential reference = %q", reference)
		}
		resolved = true
		return consume(payload)
	}), credentialRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved || client == nil {
		t.Fatalf("resolved = %t, client = %#v", resolved, client)
	}
}

func testServiceAccountJSON(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]string{
		"type":                        "service_account",
		"project_id":                  "magelift-test",
		"private_key_id":              "test-key",
		"private_key":                 string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKey})),
		"client_email":                "identity@magelift-test.iam.gserviceaccount.com",
		"client_id":                   "1234567890",
		"auth_uri":                    "https://accounts.google.com/o/oauth2/auth",
		"token_uri":                   "https://oauth2.googleapis.com/token",
		"auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
		"client_x509_cert_url":        "https://www.googleapis.com/robot/v1/metadata/x509/identity%40magelift-test.iam.gserviceaccount.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
