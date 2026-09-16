// Package ovhprovider contains OVHcloud API translators. The OVH API is
// generic by path, so this package owns the response shapes and normalizes
// them before they reach MageLift's provider-neutral contracts.
package ovhprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
	"github.com/ovh/go-ovh/ovh"
)

// API is the context-aware OVH generic API surface required by identity
// admission. It is small enough to fake without importing OVH response types.
type API interface {
	GetWithContext(context.Context, string, interface{}) error
}

// IdentityClient verifies the OVHcloud account identity and Public Cloud
// project before delegating least-privilege proof to a provider verifier.
type IdentityClient struct {
	api   API
	scope provider.PermissionVerifier
}

var _ provider.IdentityClient = (*IdentityClient)(nil)

// NewIdentity creates a live OVHcloud client from one opaque credential
// reference. The resolver callback is the only place that sees the raw OVH
// credential payload; the client retains only the resulting API interface.
// The expected secret payload is a JSON object with appKey, appSecret, and
// consumerKey fields. The payload is never returned, logged, or stored.
func NewIdentity(ctx context.Context, endpoint string, resolver provider.CredentialResolver, credentialRef string, scope provider.PermissionVerifier) (*IdentityClient, error) {
	if ctx == nil {
		return nil, errors.New("OVH identity context is required")
	}
	if resolver == nil {
		return nil, errors.New("OVH identity credential resolver is required")
	}
	var identity *IdentityClient
	err := provider.UseCredential(ctx, resolver, credentialRef, func(value []byte) error {
		var credentials struct {
			AppKey      string `json:"appKey"`
			AppSecret   string `json:"appSecret"`
			ConsumerKey string `json:"consumerKey"`
		}
		if err := json.Unmarshal(value, &credentials); err != nil {
			return errors.New("OVH identity credential payload is invalid")
		}
		if strings.TrimSpace(credentials.AppKey) == "" || strings.TrimSpace(credentials.AppSecret) == "" || strings.TrimSpace(credentials.ConsumerKey) == "" {
			return errors.New("OVH identity credential payload is incomplete")
		}
		client, err := ovh.NewClient(endpoint, credentials.AppKey, credentials.AppSecret, credentials.ConsumerKey)
		if err != nil {
			return errors.New("create OVH API client failed")
		}
		identity, err = NewIdentityFromClient(client, scope)
		return err
	})
	if err != nil {
		return nil, err
	}
	return identity, nil
}

// NewIdentityFromClient injects the narrow generic API surface for tests and
// community endpoint implementations.
func NewIdentityFromClient(client API, scope provider.PermissionVerifier) (*IdentityClient, error) {
	if client == nil {
		return nil, errors.New("OVH API client is required")
	}
	return &IdentityClient{api: client, scope: scope}, nil
}

// CheckIdentity implements the common provider identity contract.
func (c *IdentityClient) CheckIdentity(ctx context.Context, request provider.IdentityCheckRequest) (provider.IdentityObservation, error) {
	if c == nil || c.api == nil {
		return provider.IdentityObservation{}, errors.New("OVH API client is required")
	}
	if ctx == nil {
		return provider.IdentityObservation{}, errors.New("OVH identity context is required")
	}
	if request.Provider != sdk.ProviderID("ovh") {
		return provider.IdentityObservation{}, fmt.Errorf("OVH identity client cannot verify provider %q", request.Provider)
	}
	if err := ctx.Err(); err != nil {
		return provider.IdentityObservation{}, err
	}
	var account accountResponse
	if err := c.api.GetWithContext(ctx, "/me", &account); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return provider.IdentityObservation{}, contextErr
		}
		return provider.IdentityObservation{}, errors.New("OVH account identity lookup failed")
	}
	principal := strings.TrimSpace(account.Nichandle)
	if principal == "" {
		principal = strings.TrimSpace(account.CustomerCode)
	}
	if principal == "" || strings.ContainsAny(principal, "\r\n\x00") {
		return provider.IdentityObservation{}, errors.New("OVH account identity has no safe principal")
	}
	projectPath := "/cloud/project/" + url.PathEscape(request.AccountOrProjectRef)
	var project projectResponse
	if err := c.api.GetWithContext(ctx, projectPath, &project); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return provider.IdentityObservation{}, contextErr
		}
		return provider.IdentityObservation{}, errors.New("OVH Public Cloud project lookup failed")
	}
	projectID := strings.TrimSpace(project.ID)
	if projectID == "" {
		projectID = strings.TrimSpace(project.ProjectID)
	}
	if projectID == "" {
		projectID = strings.TrimSpace(project.ServiceName)
	}
	if projectID == "" || projectID != request.AccountOrProjectRef {
		return provider.IdentityObservation{}, errors.New("OVH Public Cloud project does not match the requested project")
	}
	checks, leastPrivilegeVerified, err := verifyScope(ctx, c.scope, request, principal)
	if err != nil {
		return provider.IdentityObservation{}, err
	}
	return provider.IdentityObservation{
		Provider:               request.Provider,
		AccountOrProjectRef:    projectID,
		Region:                 request.Region,
		PrincipalRef:           principal,
		PermissionChecks:       checks,
		LeastPrivilegeVerified: leastPrivilegeVerified,
	}, nil
}

type accountResponse struct {
	Nichandle    string `json:"nichandle"`
	CustomerCode string `json:"customerCode"`
}

type projectResponse struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	ServiceName string `json:"serviceName"`
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
		return nil, false, errors.New("OVH permission scope verification failed")
	}
	for _, permission := range request.RequiredPermissions {
		checks = append(checks, provider.PermissionCheck{Name: permission, Granted: verified})
	}
	return checks, verified, nil
}
