package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
)

// ErrWIFNotFound reports a missing WIF/IAM resource during Ensure.
var ErrWIFNotFound = errors.New("wif resource not found")

// WIFPool describes a workload identity pool create/get payload.
type WIFPool struct {
	Name        string
	DisplayName string
}

// WIFProvider describes an OIDC workload identity pool provider.
type WIFProvider struct {
	Name               string
	DisplayName        string
	IssuerURI          string
	AttributeMapping   map[string]string
	AttributeCondition string
}

// WIFServiceAccount is a CI service account used for GitHub federation.
type WIFServiceAccount struct {
	Name  string
	Email string
}

// WIFBinding is an IAM policy binding on a service account.
type WIFBinding struct {
	Role    string
	Members []string
}

// WIFAPI is the injectable IAM/WIF surface used by IdentityBootstrapper.
type WIFAPI interface {
	ProjectNumber(ctx context.Context, projectID string) (string, error)
	GetPool(ctx context.Context, name string) (WIFPool, error)
	CreatePool(ctx context.Context, parent, poolID string, pool WIFPool) (WIFPool, error)
	GetProvider(ctx context.Context, name string) (WIFProvider, error)
	CreateProvider(ctx context.Context, parent, providerID string, provider WIFProvider) (WIFProvider, error)
	GetServiceAccount(ctx context.Context, name string) (WIFServiceAccount, error)
	CreateServiceAccount(ctx context.Context, projectID, accountID, displayName string) (WIFServiceAccount, error)
	GetServiceAccountIAMPolicy(ctx context.Context, resource string) ([]WIFBinding, error)
	SetServiceAccountIAMPolicy(ctx context.Context, resource string, bindings []WIFBinding) error
}

type iamWIFClient struct {
	iam *iam.Service
	crm *cloudresourcemanager.Service
}

// NewWIFClient builds a live IAM + Cloud Resource Manager client for WIF Ensure.
func NewWIFClient(ctx context.Context) (WIFAPI, error) {
	iamService, err := iam.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("create IAM client: %w", err)
	}
	crmService, err := cloudresourcemanager.NewService(ctx, option.WithScopes(cloudresourcemanager.CloudPlatformScope))
	if err != nil {
		return nil, fmt.Errorf("create Resource Manager client: %w", err)
	}
	return iamWIFClient{iam: iamService, crm: crmService}, nil
}

func (c iamWIFClient) ProjectNumber(ctx context.Context, projectID string) (string, error) {
	project, err := c.crm.Projects.Get(projectID).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("resolve GCP project number: %w", err)
	}
	if project.ProjectNumber == 0 {
		return "", errors.New("GCP project number is missing")
	}
	return fmt.Sprintf("%d", project.ProjectNumber), nil
}

func (c iamWIFClient) GetPool(ctx context.Context, name string) (WIFPool, error) {
	pool, err := c.iam.Projects.Locations.WorkloadIdentityPools.Get(name).Context(ctx).Do()
	if err != nil {
		return WIFPool{}, mapWIFError("get workload identity pool", err)
	}
	return WIFPool{Name: pool.Name, DisplayName: pool.DisplayName}, nil
}

func (c iamWIFClient) CreatePool(ctx context.Context, parent, poolID string, pool WIFPool) (WIFPool, error) {
	op, err := c.iam.Projects.Locations.WorkloadIdentityPools.Create(parent, &iam.WorkloadIdentityPool{
		DisplayName: pool.DisplayName,
		Mode:        "FEDERATION_ONLY",
	}).WorkloadIdentityPoolId(poolID).Context(ctx).Do()
	if err != nil {
		return WIFPool{}, fmt.Errorf("create workload identity pool: %w", err)
	}
	name := parent + "/workloadIdentityPools/" + poolID
	if err := waitIAMOperation(ctx, c.iam, op); err != nil {
		return WIFPool{}, err
	}
	got, err := c.GetPool(ctx, name)
	if err != nil {
		return WIFPool{}, err
	}
	return got, nil
}

func (c iamWIFClient) GetProvider(ctx context.Context, name string) (WIFProvider, error) {
	provider, err := c.iam.Projects.Locations.WorkloadIdentityPools.Providers.Get(name).Context(ctx).Do()
	if err != nil {
		return WIFProvider{}, mapWIFError("get workload identity provider", err)
	}
	issuer := ""
	if provider.Oidc != nil {
		issuer = provider.Oidc.IssuerUri
	}
	return WIFProvider{
		Name:               provider.Name,
		DisplayName:        provider.DisplayName,
		IssuerURI:          issuer,
		AttributeMapping:   provider.AttributeMapping,
		AttributeCondition: provider.AttributeCondition,
	}, nil
}

func (c iamWIFClient) CreateProvider(ctx context.Context, parent, providerID string, provider WIFProvider) (WIFProvider, error) {
	op, err := c.iam.Projects.Locations.WorkloadIdentityPools.Providers.Create(parent, &iam.WorkloadIdentityPoolProvider{
		DisplayName:        provider.DisplayName,
		AttributeMapping:   provider.AttributeMapping,
		AttributeCondition: provider.AttributeCondition,
		Oidc:               &iam.Oidc{IssuerUri: provider.IssuerURI},
	}).WorkloadIdentityPoolProviderId(providerID).Context(ctx).Do()
	if err != nil {
		return WIFProvider{}, fmt.Errorf("create workload identity provider: %w", err)
	}
	name := parent + "/providers/" + providerID
	if err := waitIAMOperation(ctx, c.iam, op); err != nil {
		return WIFProvider{}, err
	}
	return c.GetProvider(ctx, name)
}

func (c iamWIFClient) GetServiceAccount(ctx context.Context, name string) (WIFServiceAccount, error) {
	sa, err := c.iam.Projects.ServiceAccounts.Get(name).Context(ctx).Do()
	if err != nil {
		return WIFServiceAccount{}, mapWIFError("get service account", err)
	}
	return WIFServiceAccount{Name: sa.Name, Email: sa.Email}, nil
}

func (c iamWIFClient) CreateServiceAccount(ctx context.Context, projectID, accountID, displayName string) (WIFServiceAccount, error) {
	sa, err := c.iam.Projects.ServiceAccounts.Create("projects/"+projectID, &iam.CreateServiceAccountRequest{
		AccountId: accountID,
		ServiceAccount: &iam.ServiceAccount{
			DisplayName: displayName,
		},
	}).Context(ctx).Do()
	if err != nil {
		return WIFServiceAccount{}, fmt.Errorf("create service account: %w", err)
	}
	return WIFServiceAccount{Name: sa.Name, Email: sa.Email}, nil
}

func (c iamWIFClient) GetServiceAccountIAMPolicy(ctx context.Context, resource string) ([]WIFBinding, error) {
	policy, err := c.iam.Projects.ServiceAccounts.GetIamPolicy(resource).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get service account IAM policy: %w", err)
	}
	return fromIAMBindings(policy.Bindings), nil
}

func (c iamWIFClient) SetServiceAccountIAMPolicy(ctx context.Context, resource string, bindings []WIFBinding) error {
	_, err := c.iam.Projects.ServiceAccounts.SetIamPolicy(resource, &iam.SetIamPolicyRequest{
		Policy: &iam.Policy{Bindings: toIAMBindings(bindings)},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("set service account IAM policy: %w", err)
	}
	return nil
}

func mapWIFError(op string, err error) error {
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound {
		return fmt.Errorf("%s: %w", op, ErrWIFNotFound)
	}
	return fmt.Errorf("%s: %w", op, err)
}

func waitIAMOperation(ctx context.Context, service *iam.Service, op *iam.Operation) error {
	if op == nil {
		return errors.New("IAM operation is required")
	}
	name := strings.TrimSpace(op.Name)
	if name == "" {
		return errors.New("IAM operation name is missing")
	}
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		current, err := service.Projects.Locations.WorkloadIdentityPools.Operations.Get(name).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("poll IAM operation: %w", err)
		}
		if current.Done {
			if current.Error != nil {
				return fmt.Errorf("IAM operation failed: %s", current.Error.Message)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("IAM operation timed out")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func fromIAMBindings(bindings []*iam.Binding) []WIFBinding {
	out := make([]WIFBinding, 0, len(bindings))
	for _, binding := range bindings {
		if binding == nil {
			continue
		}
		out = append(out, WIFBinding{Role: binding.Role, Members: append([]string(nil), binding.Members...)})
	}
	return out
}

func toIAMBindings(bindings []WIFBinding) []*iam.Binding {
	out := make([]*iam.Binding, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, &iam.Binding{Role: binding.Role, Members: append([]string(nil), binding.Members...)})
	}
	return out
}
