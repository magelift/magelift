package scalewayprovider

import (
	"context"
	"encoding/json"
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

func TestIdentityClientChecksProjectAndScope(t *testing.T) {
	client, err := NewIdentityFromProjectReader(projectReaderFunc(func(_ context.Context, id string) (ProjectIdentity, error) {
		return ProjectIdentity{ID: id, OrganizationID: "org-1"}, nil
	}), permissionVerifierFunc(func(_ context.Context, provider sdk.ProviderID, project, principal string, permissions []string) (bool, error) {
		if provider != "scaleway" || project != "project-1" || principal != "project:project-1" || len(permissions) != 1 {
			t.Fatalf("scope request = %q %q %q %#v", provider, project, principal, permissions)
		}
		return true, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := client.CheckIdentity(context.Background(), provider.IdentityCheckRequest{Provider: "scaleway", AccountOrProjectRef: "project-1", Region: "fr-par", RequiredPermissions: []string{"k8s.clusters.get"}})
	if err != nil || !observation.LeastPrivilegeVerified {
		t.Fatalf("CheckIdentity() = %#v, %v", observation, err)
	}
}

func TestIdentityClientDoesNotClaimScopeWithoutVerifier(t *testing.T) {
	client, err := NewIdentityFromProjectReader(projectReaderFunc(func(context.Context, string) (ProjectIdentity, error) {
		return ProjectIdentity{ID: "project-1"}, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := client.CheckIdentity(context.Background(), provider.IdentityCheckRequest{Provider: "scaleway", AccountOrProjectRef: "project-1", Region: "fr-par", RequiredPermissions: []string{"k8s.clusters.get"}})
	if err != nil || observation.LeastPrivilegeVerified {
		t.Fatalf("unverified scope = %#v, %v", observation, err)
	}
}

func TestNewIdentityWithCredentialBuildsProviderClientInsideResolver(t *testing.T) {
	const credentialRef = "vault://magelift/scaleway-identity"
	payload, err := json.Marshal(map[string]string{
		"accessKey":      "SCW1234567890ABCDEFG",
		"secretKey":      "7363616c-6577-6573-6862-6f7579616161",
		"organizationId": "6170692e-7363-616c-6577-61792e636f6d",
	})
	if err != nil {
		t.Fatal(err)
	}
	var resolved bool
	client, err := NewIdentityWithCredential(context.Background(), "6170692e-7363-616c-6577-61792e636f6e", "fr-par", "", provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
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
