package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func validClientRequest() ClientRequest {
	return ClientRequest{
		TargetID:            "aws-ecs-fargate",
		Provider:            "aws",
		Runtime:             "ecs-fargate",
		AccountOrProjectRef: "123456789012",
		Region:              "eu-west-1",
		CredentialRefs:      []string{"aws-identity://role/magelift-certifier"},
		OwnershipMarker:     "magelift/test/client",
	}
}

func TestClientRequestRejectsInvalidAndDuplicateReferences(t *testing.T) {
	request := validClientRequest()
	request.CredentialRefs = []string{"raw-token", "raw-token"}
	if err := request.Validate(); err == nil || !strings.Contains(err.Error(), "credential reference") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestUseCredentialValidatesReferenceAndDoesNotWrapConsumerError(t *testing.T) {
	secret := "token=super-secret-value"
	resolver := CredentialResolverFunc(func(_ context.Context, _ string, consume func([]byte) error) error {
		return consume([]byte(secret))
	})
	if err := UseCredential(context.Background(), resolver, "aws-secrets-manager://magelift/test", func(_ []byte) error {
		return errors.New(secret)
	}); err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("UseCredential() error = %v", err)
	}
}

func TestUseCredentialHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := UseCredential(ctx, CredentialResolverFunc(func(context.Context, string, func([]byte) error) error {
		called = true
		return nil
	}), "aws-secrets-manager://magelift/test", func([]byte) error { return nil })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatalf("UseCredential() error = %v, called = %v", err, called)
	}
}

type identityClientFunc func(context.Context, IdentityCheckRequest) (IdentityObservation, error)

func (f identityClientFunc) CheckIdentity(ctx context.Context, request IdentityCheckRequest) (IdentityObservation, error) {
	return f(ctx, request)
}

func TestVerifyIdentityRequiresExactScopeAndPermissions(t *testing.T) {
	request := IdentityCheckRequest{Provider: sdk.ProviderID("aws"), AccountOrProjectRef: "123456789012", Region: "eu-west-1", RequiredPermissions: []string{"ecs:DescribeServices"}}
	client := identityClientFunc(func(context.Context, IdentityCheckRequest) (IdentityObservation, error) {
		return IdentityObservation{Provider: "aws", AccountOrProjectRef: "123456789012", Region: "eu-west-1", PrincipalRef: "arn:aws:iam::123456789012:role/magelift", PermissionChecks: []PermissionCheck{{Name: "ecs:DescribeServices", Granted: true}}, LeastPrivilegeVerified: true}, nil
	})
	observation, err := VerifyIdentity(context.Background(), client, request)
	if err != nil || observation.PrincipalRef == "" {
		t.Fatalf("VerifyIdentity() = %#v, %v", observation, err)
	}

	client = identityClientFunc(func(context.Context, IdentityCheckRequest) (IdentityObservation, error) {
		return IdentityObservation{Provider: "aws", AccountOrProjectRef: "123456789012", Region: "eu-west-1", PrincipalRef: "arn:aws:iam::123456789012:role/magelift", LeastPrivilegeVerified: true}, nil
	})
	if _, err := VerifyIdentity(context.Background(), client, request); err == nil || !strings.Contains(err.Error(), "missing required permissions") {
		t.Fatalf("missing permission error = %v", err)
	}
}
