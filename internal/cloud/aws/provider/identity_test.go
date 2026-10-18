package awsprovider

import (
	"context"
	"errors"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

type stsClientFunc func(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)

func (f stsClientFunc) GetCallerIdentity(ctx context.Context, input *sts.GetCallerIdentityInput, options ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
	return f(ctx, input, options...)
}

type permissionVerifierFunc func(context.Context, sdk.ProviderID, string, string, []string) (bool, error)

func (f permissionVerifierFunc) Verify(ctx context.Context, p sdk.ProviderID, account, principal string, permissions []string) (bool, error) {
	return f(ctx, p, account, principal, permissions)
}

func TestIdentityClientChecksAccountAndScopeWithoutLeakingAPIErrors(t *testing.T) {
	client, err := NewIdentityFromClient(stsClientFunc(func(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
		return &sts.GetCallerIdentityOutput{Account: awssdk.String("123456789012"), Arn: awssdk.String("arn:aws:iam::123456789012:role/magelift")}, nil
	}), permissionVerifierFunc(func(_ context.Context, provider sdk.ProviderID, account, principal string, permissions []string) (bool, error) {
		if provider != "aws" || account != "123456789012" || principal == "" || len(permissions) != 1 {
			t.Fatalf("scope request = %q %q %q %#v", provider, account, principal, permissions)
		}
		return true, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := client.CheckIdentity(context.Background(), provider.IdentityCheckRequest{Provider: "aws", AccountOrProjectRef: "123456789012", Region: "eu-west-1", RequiredPermissions: []string{"ecs:DescribeServices"}})
	if err != nil || !observation.LeastPrivilegeVerified || !observation.PermissionChecks[0].Granted {
		t.Fatalf("CheckIdentity() = %#v, %v", observation, err)
	}
}

func TestIdentityClientBlocksUnverifiedScopeAndHonorsCancellation(t *testing.T) {
	client, err := NewIdentityFromClient(stsClientFunc(func(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error) {
		return &sts.GetCallerIdentityOutput{Account: awssdk.String("123456789012"), UserId: awssdk.String("user")}, nil
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := client.CheckIdentity(context.Background(), provider.IdentityCheckRequest{Provider: "aws", AccountOrProjectRef: "123456789012", Region: "eu-west-1", RequiredPermissions: []string{"ecs:DescribeServices"}})
	if err != nil || observation.LeastPrivilegeVerified {
		t.Fatalf("unverified scope = %#v, %v", observation, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.CheckIdentity(ctx, provider.IdentityCheckRequest{Provider: "aws", AccountOrProjectRef: "123456789012", Region: "eu-west-1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled identity error = %v", err)
	}
	if strings.Contains(err.Error(), "123456789012") {
		t.Fatal("identity error exposed account details")
	}
}

func TestNewIdentityWithCredentialBuildsProviderClientInsideResolver(t *testing.T) {
	const credentialRef = "vault://magelift/aws-identity"
	var resolved bool
	client, err := NewIdentityWithCredential(context.Background(), "eu-west-1", provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
		if reference != credentialRef {
			t.Fatalf("credential reference = %q", reference)
		}
		resolved = true
		return consume([]byte(`{"accessKeyId":"AKIATEST","secretAccessKey":"secret-value","sessionToken":"session-value"}`))
	}), credentialRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resolved || client == nil {
		t.Fatalf("resolved = %t, client = %#v", resolved, client)
	}
}
