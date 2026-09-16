// Package scalewayprovider contains Scaleway SDK translators. The official
// SDK has generated provider types and request options; they stay in this
// package and are converted to the shared MageLift identity contract.
package scalewayprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
	account "github.com/scaleway/scaleway-sdk-go/api/account/v3"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// ProjectIdentity is the provider-neutral subset of a Scaleway project.
type ProjectIdentity struct {
	ID             string
	OrganizationID string
}

// ProjectReader keeps generated Scaleway SDK types out of tests and callers.
type ProjectReader interface {
	GetProject(context.Context, string) (ProjectIdentity, error)
}

// IdentityClient verifies the selected Scaleway project and delegates
// least-privilege proof to a provider-owned IAM verifier.
type IdentityClient struct {
	projects ProjectReader
	scope    provider.PermissionVerifier
}

var _ provider.IdentityClient = (*IdentityClient)(nil)

// NewIdentity builds the official Scaleway client from its default credential
// chain. The SDK receives only provider-local configuration; no credential
// value is returned from this constructor or observation.
func NewIdentity(ctx context.Context, projectID, region, organizationID string, scope provider.PermissionVerifier) (*IdentityClient, error) {
	if ctx == nil {
		return nil, errors.New("Scaleway identity context is required")
	}
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(region) == "" {
		return nil, errors.New("Scaleway project ID and region are required")
	}
	options := []scw.ClientOption{scw.WithDefaultProjectID(projectID), scw.WithDefaultRegion(scw.Region(region))}
	if strings.TrimSpace(organizationID) != "" {
		options = append(options, scw.WithDefaultOrganizationID(organizationID))
	}
	client, err := scw.NewClient(options...)
	if err != nil {
		return nil, errors.New("create Scaleway client failed")
	}
	return NewIdentityFromProjectReader(scalewayProjectReader{api: account.NewProjectAPI(client)}, scope)
}

// NewIdentityWithCredential constructs the official Scaleway client from one
// opaque credential reference. The resolver callback owns the raw access and
// secret keys; this package retains only the provider-local API interface.
// The credential payload is provider-owned JSON with accessKey and secretKey
// fields, plus an optional organizationID when the caller has not supplied
// one.
func NewIdentityWithCredential(ctx context.Context, projectID, region, organizationID string, resolver provider.CredentialResolver, credentialRef string, scope provider.PermissionVerifier) (*IdentityClient, error) {
	if ctx == nil {
		return nil, errors.New("Scaleway identity context is required")
	}
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(region) == "" {
		return nil, errors.New("Scaleway project ID and region are required")
	}
	if resolver == nil {
		return nil, errors.New("Scaleway identity credential resolver is required")
	}
	var identity *IdentityClient
	err := provider.UseCredential(ctx, resolver, credentialRef, func(value []byte) error {
		var raw struct {
			AccessKey      string `json:"accessKey"`
			SecretKey      string `json:"secretKey"`
			OrganizationID string `json:"organizationId"`
		}
		if err := json.Unmarshal(value, &raw); err != nil {
			return errors.New("Scaleway identity credential payload is invalid")
		}
		if strings.TrimSpace(raw.AccessKey) == "" || strings.TrimSpace(raw.SecretKey) == "" {
			return errors.New("Scaleway identity credential payload is incomplete")
		}
		resolvedOrganizationID := strings.TrimSpace(organizationID)
		if resolvedOrganizationID == "" {
			resolvedOrganizationID = strings.TrimSpace(raw.OrganizationID)
		}
		options := []scw.ClientOption{scw.WithAuth(raw.AccessKey, raw.SecretKey), scw.WithDefaultProjectID(projectID), scw.WithDefaultRegion(scw.Region(region))}
		if resolvedOrganizationID != "" {
			options = append(options, scw.WithDefaultOrganizationID(resolvedOrganizationID))
		}
		client, err := scw.NewClient(options...)
		if err != nil {
			return errors.New("create Scaleway client failed")
		}
		identity, err = NewIdentityFromProjectReader(scalewayProjectReader{api: account.NewProjectAPI(client)}, scope)
		return err
	})
	if err != nil {
		return nil, err
	}
	return identity, nil
}

// NewIdentityFromProjectReader injects the narrow provider-local project API.
func NewIdentityFromProjectReader(reader ProjectReader, scope provider.PermissionVerifier) (*IdentityClient, error) {
	if reader == nil {
		return nil, errors.New("Scaleway project reader is required")
	}
	return &IdentityClient{projects: reader, scope: scope}, nil
}

// CheckIdentity implements the common provider identity contract.
func (c *IdentityClient) CheckIdentity(ctx context.Context, request provider.IdentityCheckRequest) (provider.IdentityObservation, error) {
	if c == nil || c.projects == nil {
		return provider.IdentityObservation{}, errors.New("Scaleway project reader is required")
	}
	if ctx == nil {
		return provider.IdentityObservation{}, errors.New("Scaleway identity context is required")
	}
	if request.Provider != sdk.ProviderID("scaleway") {
		return provider.IdentityObservation{}, fmt.Errorf("Scaleway identity client cannot verify provider %q", request.Provider)
	}
	if err := ctx.Err(); err != nil {
		return provider.IdentityObservation{}, err
	}
	project, err := c.projects.GetProject(ctx, request.AccountOrProjectRef)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return provider.IdentityObservation{}, contextErr
		}
		return provider.IdentityObservation{}, errors.New("Scaleway project identity lookup failed")
	}
	if strings.TrimSpace(project.ID) == "" || project.ID != request.AccountOrProjectRef {
		return provider.IdentityObservation{}, errors.New("Scaleway project identity does not match the requested project")
	}
	principal := "project:" + project.ID
	checks, leastPrivilegeVerified, err := verifyScope(ctx, c.scope, request, principal)
	if err != nil {
		return provider.IdentityObservation{}, err
	}
	return provider.IdentityObservation{
		Provider:               request.Provider,
		AccountOrProjectRef:    project.ID,
		Region:                 request.Region,
		PrincipalRef:           principal,
		PermissionChecks:       checks,
		LeastPrivilegeVerified: leastPrivilegeVerified,
	}, nil
}

type scalewayProjectAPI interface {
	GetProject(*account.ProjectAPIGetProjectRequest, ...scw.RequestOption) (*account.Project, error)
}

type scalewayProjectReader struct {
	api scalewayProjectAPI
}

func (r scalewayProjectReader) GetProject(ctx context.Context, id string) (ProjectIdentity, error) {
	if r.api == nil {
		return ProjectIdentity{}, errors.New("Scaleway project API is required")
	}
	project, err := r.api.GetProject(&account.ProjectAPIGetProjectRequest{ProjectID: id}, scw.WithContext(ctx))
	if err != nil {
		return ProjectIdentity{}, err
	}
	if project == nil {
		return ProjectIdentity{}, errors.New("Scaleway project API returned no project")
	}
	return ProjectIdentity{ID: project.ID, OrganizationID: project.OrganizationID}, nil
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
		return nil, false, errors.New("Scaleway permission scope verification failed")
	}
	for _, permission := range request.RequiredPermissions {
		checks = append(checks, provider.PermissionCheck{Name: permission, Granted: verified})
	}
	return checks, verified, nil
}
