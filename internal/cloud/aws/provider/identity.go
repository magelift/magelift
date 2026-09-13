// Package awsprovider contains AWS API translators. AWS SDK types stop at
// this package; callers receive the provider-neutral identity observation.
package awsprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// STSAPI is the smallest AWS surface needed for account identity admission.
// Keeping it local makes deterministic fake-client tests independent of the
// rest of the AWS SDK.
type STSAPI interface {
	GetCallerIdentity(context.Context, *sts.GetCallerIdentityInput, ...func(*sts.Options)) (*sts.GetCallerIdentityOutput, error)
}

// IdentityClient verifies the active AWS account and delegates permission
// scope proof to an optional provider-owned access-analyzer implementation.
// Without a scope verifier the client remains useful for diagnostics but
// admission correctly blocks before mutation.
type IdentityClient struct {
	sts   STSAPI
	scope provider.PermissionVerifier
}

var _ provider.IdentityClient = (*IdentityClient)(nil)

// NewIdentity loads the official AWS SDK default credential chain. Raw
// credentials stay inside the SDK; only the opaque identity result crosses
// this package boundary.
func NewIdentity(ctx context.Context, region string, scope provider.PermissionVerifier) (*IdentityClient, error) {
	if ctx == nil {
		return nil, errors.New("AWS identity context is required")
	}
	if strings.TrimSpace(region) == "" {
		return nil, errors.New("AWS identity region is required")
	}
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	configuration, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, errors.New("load AWS identity configuration failed")
	}
	options := []func(*sts.Options){}
	if endpoint != "" {
		options = append(options, func(options *sts.Options) {
			options.BaseEndpoint = awssdk.String(endpoint)
		})
	}
	return NewIdentityFromClient(sts.NewFromConfig(configuration, options...), scope)
}

// NewIdentityWithCredential constructs STS from one opaque credential
// reference. The resolver callback owns the raw credential bytes; this
// package creates the SDK client inside that callback and returns only the
// provider-neutral identity client.
//
// The credential payload is provider-owned JSON with accessKeyId,
// secretAccessKey, and an optional sessionToken. No payload field is returned
// or included in an error.
func NewIdentityWithCredential(ctx context.Context, region string, resolver provider.CredentialResolver, credentialRef string, scope provider.PermissionVerifier) (*IdentityClient, error) {
	if ctx == nil {
		return nil, errors.New("AWS identity context is required")
	}
	if strings.TrimSpace(region) == "" {
		return nil, errors.New("AWS identity region is required")
	}
	if resolver == nil {
		return nil, errors.New("AWS identity credential resolver is required")
	}
	var identity *IdentityClient
	err := provider.UseCredential(ctx, resolver, credentialRef, func(value []byte) error {
		var raw struct {
			AccessKeyID     string `json:"accessKeyId"`
			SecretAccessKey string `json:"secretAccessKey"`
			SessionToken    string `json:"sessionToken"`
		}
		if err := json.Unmarshal(value, &raw); err != nil {
			return errors.New("AWS identity credential payload is invalid")
		}
		if strings.TrimSpace(raw.AccessKeyID) == "" || strings.TrimSpace(raw.SecretAccessKey) == "" {
			return errors.New("AWS identity credential payload is incomplete")
		}
		configuration, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region), awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(raw.AccessKeyID, raw.SecretAccessKey, raw.SessionToken)))
		if err != nil {
			return errors.New("load AWS identity configuration failed")
		}
		endpoint, err := awsendpoint.FromEnv()
		if err != nil {
			return err
		}
		options := []func(*sts.Options){}
		if endpoint != "" {
			options = append(options, func(options *sts.Options) {
				options.BaseEndpoint = awssdk.String(endpoint)
			})
		}
		identity, err = NewIdentityFromClient(sts.NewFromConfig(configuration, options...), scope)
		return err
	})
	if err != nil {
		return nil, err
	}
	return identity, nil
}

// NewIdentityFromClient injects a narrow AWS identity API for tests and
// provider-specific endpoint wrappers.
func NewIdentityFromClient(client STSAPI, scope provider.PermissionVerifier) (*IdentityClient, error) {
	if client == nil {
		return nil, errors.New("AWS STS identity client is required")
	}
	return &IdentityClient{sts: client, scope: scope}, nil
}

// CheckIdentity implements the common provider identity contract.
func (c *IdentityClient) CheckIdentity(ctx context.Context, request provider.IdentityCheckRequest) (provider.IdentityObservation, error) {
	if c == nil || c.sts == nil {
		return provider.IdentityObservation{}, errors.New("AWS STS identity client is required")
	}
	if ctx == nil {
		return provider.IdentityObservation{}, errors.New("AWS identity context is required")
	}
	if request.Provider != sdk.ProviderID("aws") {
		return provider.IdentityObservation{}, fmt.Errorf("AWS identity client cannot verify provider %q", request.Provider)
	}
	if err := ctx.Err(); err != nil {
		return provider.IdentityObservation{}, err
	}
	identity, err := c.sts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return provider.IdentityObservation{}, contextErr
		}
		return provider.IdentityObservation{}, errors.New("AWS GetCallerIdentity failed")
	}
	if identity == nil || strings.TrimSpace(awssdk.ToString(identity.Account)) == "" {
		return provider.IdentityObservation{}, errors.New("AWS identity response has no account")
	}
	if awssdk.ToString(identity.Account) != request.AccountOrProjectRef {
		return provider.IdentityObservation{}, errors.New("AWS account does not match the requested account")
	}
	principal := strings.TrimSpace(awssdk.ToString(identity.Arn))
	if principal == "" {
		principal = strings.TrimSpace(awssdk.ToString(identity.UserId))
	}
	if principal == "" || strings.ContainsAny(principal, "\r\n\x00") {
		return provider.IdentityObservation{}, errors.New("AWS identity response has no safe principal")
	}
	checks, leastPrivilegeVerified, err := verifyScope(ctx, c.scope, request, principal)
	if err != nil {
		return provider.IdentityObservation{}, err
	}
	return provider.IdentityObservation{
		Provider:               request.Provider,
		AccountOrProjectRef:    request.AccountOrProjectRef,
		Region:                 request.Region,
		PrincipalRef:           principal,
		PermissionChecks:       checks,
		LeastPrivilegeVerified: leastPrivilegeVerified,
	}, nil
}

func verifyScope(ctx context.Context, scope provider.PermissionVerifier, request provider.IdentityCheckRequest, principal string) ([]provider.PermissionCheck, bool, error) {
	checks := make([]provider.PermissionCheck, 0, len(request.RequiredPermissions))
	if scope == nil {
		for _, permission := range request.RequiredPermissions {
			checks = append(checks, provider.PermissionCheck{Name: permission})
		}
		return checks, false, nil
	}
	verified, err := scope.Verify(ctx, request.Provider, request.AccountOrProjectRef, principal, request.RequiredPermissions)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, false, contextErr
		}
		return nil, false, errors.New("AWS permission scope verification failed")
	}
	for _, permission := range request.RequiredPermissions {
		checks = append(checks, provider.PermissionCheck{Name: permission, Granted: verified})
	}
	return checks, verified, nil
}
