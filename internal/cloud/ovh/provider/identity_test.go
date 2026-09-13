package ovhprovider

import (
	"context"
	"errors"
	"strings"
	"testing"

	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type apiFunc func(context.Context, string, interface{}) error

func (f apiFunc) GetWithContext(ctx context.Context, path string, response interface{}) error {
	return f(ctx, path, response)
}

type permissionVerifierFunc func(context.Context, sdk.ProviderID, string, string, []string) (bool, error)

func (f permissionVerifierFunc) Verify(ctx context.Context, p sdk.ProviderID, project, principal string, permissions []string) (bool, error) {
	return f(ctx, p, project, principal, permissions)
}

func TestIdentityClientChecksAccountProjectAndScope(t *testing.T) {
	client, err := NewIdentityFromClient(apiFunc(func(_ context.Context, path string, response interface{}) error {
		switch path {
		case "/me":
			*response.(*accountResponse) = accountResponse{Nichandle: "AB123-OVH"}
		case "/cloud/project/project-1":
			*response.(*projectResponse) = projectResponse{ID: "project-1"}
		default:
			t.Fatalf("unexpected OVH API path %q", path)
		}
		return nil
	}), permissionVerifierFunc(func(_ context.Context, provider sdk.ProviderID, project, principal string, permissions []string) (bool, error) {
		if provider != "ovh" || project != "project-1" || principal != "AB123-OVH" || len(permissions) != 1 {
			t.Fatalf("scope request = %q %q %q %#v", provider, project, principal, permissions)
		}
		return true, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := client.CheckIdentity(context.Background(), provider.IdentityCheckRequest{Provider: "ovh", AccountOrProjectRef: "project-1", Region: "GRA11", RequiredPermissions: []string{"cloudProject.read"}})
	if err != nil || !observation.LeastPrivilegeVerified {
		t.Fatalf("CheckIdentity() = %#v, %v", observation, err)
	}
}

func TestIdentityClientDoesNotClaimScopeWithoutVerifier(t *testing.T) {
	client, err := NewIdentityFromClient(apiFunc(func(_ context.Context, path string, response interface{}) error {
		if path == "/me" {
			*response.(*accountResponse) = accountResponse{Nichandle: "AB123-OVH"}
			return nil
		}
		*response.(*projectResponse) = projectResponse{ID: "project-1"}
		return nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := client.CheckIdentity(context.Background(), provider.IdentityCheckRequest{Provider: "ovh", AccountOrProjectRef: "project-1", Region: "GRA11", RequiredPermissions: []string{"cloudProject.read"}})
	if err != nil || observation.LeastPrivilegeVerified {
		t.Fatalf("unverified scope = %#v, %v", observation, err)
	}
}

func TestNewIdentityConsumesOnlyOpaqueCredentialReference(t *testing.T) {
	secret := `{"appKey":"application-secret","appSecret":"consumer-secret","consumerKey":"user-secret"}`
	_, err := NewIdentity(context.Background(), "ovh-eu", provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
		if reference != "aws-secrets-manager://magelift/ovh" {
			t.Fatalf("credential reference = %q", reference)
		}
		return consume([]byte(secret))
	}), "aws-secrets-manager://magelift/ovh", nil)
	if err != nil && strings.Contains(err.Error(), "application-secret") {
		t.Fatalf("credential escaped error boundary: %v", err)
	}
	if _, err := NewIdentity(context.Background(), "ovh-eu", provider.CredentialResolverFunc(func(context.Context, string, func([]byte) error) error {
		return errors.New("raw-credential-error")
	}), "aws-secrets-manager://magelift/ovh", nil); err == nil || strings.Contains(err.Error(), "raw-credential-error") {
		t.Fatalf("resolver error was not normalized: %v", err)
	}
}
