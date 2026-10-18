// Package gcpprovider contains GCP API translators. Google API response
// types are converted to provider-neutral observations at this boundary.
package gcpprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/option"
)

// ProjectIdentity is the small GCP project shape required by admission.
type ProjectIdentity struct {
	ID     string
	Number int64
}

// ProjectReader is deliberately narrower than the Google Resource Manager
// service so fake tests do not depend on generated Google API types.
type ProjectReader interface {
	GetProject(context.Context, string) (ProjectIdentity, error)
}

// IdentityClient verifies the requested GCP project. A separate permission
// verifier is required for least-privilege proof; project visibility alone is
// not sufficient to admit a paid lifecycle operation.
type IdentityClient struct {
	projects ProjectReader
	scope    provider.PermissionVerifier
}

var _ provider.IdentityClient = (*IdentityClient)(nil)

// NewIdentity creates a live Resource Manager client using Application
// Default Credentials. The Google client owns credential loading and raw
// token handling inside this package boundary.
func NewIdentity(ctx context.Context, scope provider.PermissionVerifier) (*IdentityClient, error) {
	if ctx == nil {
		return nil, errors.New("GCP identity context is required")
	}
	service, err := cloudresourcemanager.NewService(ctx, option.WithScopes(cloudresourcemanager.CloudPlatformScope))
	if err != nil {
		return nil, errors.New("create GCP Resource Manager client failed")
	}
	return NewIdentityFromProjectReader(resourceManagerProjectReader{service: service}, scope)
}

// NewIdentityWithCredential constructs Resource Manager from one opaque
// credential reference. The resolver callback owns the raw service-account
// JSON; Google client construction happens inside the callback and only the
// provider-neutral identity client is retained.
func NewIdentityWithCredential(ctx context.Context, resolver provider.CredentialResolver, credentialRef string, scope provider.PermissionVerifier) (*IdentityClient, error) {
	if ctx == nil {
		return nil, errors.New("GCP identity context is required")
	}
	if resolver == nil {
		return nil, errors.New("GCP identity credential resolver is required")
	}
	var identity *IdentityClient
	err := provider.UseCredential(ctx, resolver, credentialRef, func(value []byte) error {
		service, err := cloudresourcemanager.NewService(ctx, option.WithCredentialsJSON(append([]byte(nil), value...)), option.WithScopes(cloudresourcemanager.CloudPlatformScope))
		if err != nil {
			return errors.New("create GCP Resource Manager client failed")
		}
		identity, err = NewIdentityFromProjectReader(resourceManagerProjectReader{service: service}, scope)
		return err
	})
	if err != nil {
		return nil, err
	}
	return identity, nil
}

// NewIdentityFromProjectReader injects the provider-local project API for
// deterministic tests and community endpoint wrappers.
func NewIdentityFromProjectReader(reader ProjectReader, scope provider.PermissionVerifier) (*IdentityClient, error) {
	if reader == nil {
		return nil, errors.New("GCP project reader is required")
	}
	return &IdentityClient{projects: reader, scope: scope}, nil
}

// CheckIdentity implements the common provider identity contract.
func (c *IdentityClient) CheckIdentity(ctx context.Context, request provider.IdentityCheckRequest) (provider.IdentityObservation, error) {
	if c == nil || c.projects == nil {
		return provider.IdentityObservation{}, errors.New("GCP project reader is required")
	}
	if ctx == nil {
		return provider.IdentityObservation{}, errors.New("GCP identity context is required")
	}
	if request.Provider != sdk.ProviderID("gcp") {
		return provider.IdentityObservation{}, fmt.Errorf("GCP identity client cannot verify provider %q", request.Provider)
	}
	if err := ctx.Err(); err != nil {
		return provider.IdentityObservation{}, err
	}
	project, err := c.projects.GetProject(ctx, request.AccountOrProjectRef)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return provider.IdentityObservation{}, contextErr
		}
		return provider.IdentityObservation{}, errors.New("GCP project identity lookup failed")
	}
	if strings.TrimSpace(project.ID) == "" || project.ID != request.AccountOrProjectRef {
		return provider.IdentityObservation{}, errors.New("GCP project identity does not match the requested project")
	}
	principal := "project:" + project.ID
	if project.Number > 0 {
		principal = fmt.Sprintf("projects/%d", project.Number)
	}
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

type resourceManagerProjectReader struct {
	service *cloudresourcemanager.Service
}

func (r resourceManagerProjectReader) GetProject(ctx context.Context, id string) (ProjectIdentity, error) {
	if r.service == nil {
		return ProjectIdentity{}, errors.New("GCP Resource Manager service is required")
	}
	project, err := r.service.Projects.Get(id).Context(ctx).Do()
	if err != nil {
		return ProjectIdentity{}, err
	}
	if project == nil {
		return ProjectIdentity{}, errors.New("GCP Resource Manager returned no project")
	}
	return ProjectIdentity{ID: project.ProjectId, Number: project.ProjectNumber}, nil
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
		return nil, false, errors.New("GCP permission scope verification failed")
	}
	for _, permission := range request.RequiredPermissions {
		checks = append(checks, provider.PermissionCheck{Name: permission, Granted: verified})
	}
	return checks, verified, nil
}
