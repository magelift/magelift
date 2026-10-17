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

const (
	wifVisibilityAttempts = 15
	wifVisibilityDelay    = 2 * time.Second
	wifTransientAttempts  = 6
	wifTransientDelay     = 2 * time.Second
	iamPolicyVersion3     = 3
)

// WIFPool describes a workload identity pool create/get payload.
type WIFPool struct {
	Name        string
	DisplayName string
	Description string
}

// WIFProvider describes an OIDC workload identity pool provider.
type WIFProvider struct {
	Name               string
	DisplayName        string
	Description        string
	IssuerURI          string
	AttributeMapping   map[string]string
	AttributeCondition string
}

// WIFServiceAccount is a CI service account used for GitHub federation.
type WIFServiceAccount struct {
	Name        string
	Email       string
	Description string
}

// WIFAPI is the injectable IAM/WIF surface used by IdentityBootstrapper.
type WIFAPI interface {
	ProjectNumber(ctx context.Context, projectID string) (string, error)
	GetPool(ctx context.Context, name string) (WIFPool, error)
	CreatePool(ctx context.Context, parent, poolID string, pool WIFPool) (WIFPool, error)
	GetProvider(ctx context.Context, name string) (WIFProvider, error)
	CreateProvider(ctx context.Context, parent, providerID string, provider WIFProvider) (WIFProvider, error)
	GetServiceAccount(ctx context.Context, name string) (WIFServiceAccount, error)
	CreateServiceAccount(ctx context.Context, projectID, accountID, displayName, description string) (WIFServiceAccount, error)
	GetServiceAccountIAMPolicy(ctx context.Context, resource string) (*iam.Policy, error)
	SetServiceAccountIAMPolicy(ctx context.Context, resource string, policy *iam.Policy) error
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
	project, err := retryWIFCall(ctx, wifTransientDelay, func() (*cloudresourcemanager.Project, error) {
		return c.crm.Projects.Get(projectID).Context(ctx).Do()
	})
	if err != nil {
		return "", fmt.Errorf("resolve GCP project number: %w", err)
	}
	if project.ProjectNumber == 0 {
		return "", errors.New("GCP project number is missing")
	}
	return fmt.Sprintf("%d", project.ProjectNumber), nil
}

func (c iamWIFClient) GetPool(ctx context.Context, name string) (WIFPool, error) {
	pool, err := retryWIFCall(ctx, wifTransientDelay, func() (*iam.WorkloadIdentityPool, error) {
		return c.iam.Projects.Locations.WorkloadIdentityPools.Get(name).Context(ctx).Do()
	})
	if err != nil {
		return WIFPool{}, mapWIFError("get workload identity pool", err)
	}
	return WIFPool{Name: pool.Name, DisplayName: pool.DisplayName, Description: pool.Description}, nil
}

func (c iamWIFClient) CreatePool(ctx context.Context, parent, poolID string, pool WIFPool) (WIFPool, error) {
	op, err := c.iam.Projects.Locations.WorkloadIdentityPools.Create(parent, &iam.WorkloadIdentityPool{
		DisplayName: pool.DisplayName,
		Description: pool.Description,
		Mode:        "FEDERATION_ONLY",
	}).WorkloadIdentityPoolId(poolID).Context(ctx).Do()
	if err != nil {
		return WIFPool{}, fmt.Errorf("create workload identity pool: %w", err)
	}
	name := parent + "/workloadIdentityPools/" + poolID
	if err := waitIAMOperation(ctx, c.iam, op); err != nil {
		return WIFPool{}, err
	}
	got, err := waitWIFResource(ctx, wifVisibilityDelay, c.GetPool, name)
	if err != nil {
		return WIFPool{}, err
	}
	return got, nil
}

func (c iamWIFClient) GetProvider(ctx context.Context, name string) (WIFProvider, error) {
	provider, err := retryWIFCall(ctx, wifTransientDelay, func() (*iam.WorkloadIdentityPoolProvider, error) {
		return c.iam.Projects.Locations.WorkloadIdentityPools.Providers.Get(name).Context(ctx).Do()
	})
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
		Description:        provider.Description,
		IssuerURI:          issuer,
		AttributeMapping:   provider.AttributeMapping,
		AttributeCondition: provider.AttributeCondition,
	}, nil
}

func (c iamWIFClient) CreateProvider(ctx context.Context, parent, providerID string, provider WIFProvider) (WIFProvider, error) {
	name := parent + "/providers/" + providerID
	var op *iam.Operation
	var err error
	for attempt := 1; attempt <= wifVisibilityAttempts; attempt++ {
		op, err = c.iam.Projects.Locations.WorkloadIdentityPools.Providers.Create(parent, &iam.WorkloadIdentityPoolProvider{
			DisplayName:        provider.DisplayName,
			Description:        provider.Description,
			AttributeMapping:   provider.AttributeMapping,
			AttributeCondition: provider.AttributeCondition,
			Oidc:               &iam.Oidc{IssuerUri: provider.IssuerURI},
		}).WorkloadIdentityPoolProviderId(providerID).Context(ctx).Do()
		if err == nil {
			break
		}
		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) && apiErr.Code == http.StatusConflict {
			return waitWIFResource(ctx, 0, c.GetProvider, name)
		}
		if !errors.As(err, &apiErr) || apiErr.Code != http.StatusNotFound || attempt == wifVisibilityAttempts {
			return WIFProvider{}, fmt.Errorf("create workload identity provider: %w", err)
		}
		if err := waitWIFVisibility(ctx, wifVisibilityDelay); err != nil {
			return WIFProvider{}, err
		}
	}
	if err := waitIAMOperation(ctx, c.iam, op); err != nil {
		return WIFProvider{}, err
	}
	return waitWIFResource(ctx, wifVisibilityDelay, c.GetProvider, name)
}

// waitWIFResource covers the short read-after-create window in IAM. The
// operation can be DONE before the resource is visible to Get, so a single
// 404 here must not turn a successful create into a failed bootstrap.
func waitWIFResource[T any](ctx context.Context, delay time.Duration, get func(context.Context, string) (T, error), name string) (T, error) {
	var zero T
	var lastErr error
	for attempt := 1; attempt <= wifVisibilityAttempts; attempt++ {
		value, err := get(ctx, name)
		if err == nil {
			return value, nil
		}
		if !errors.Is(err, ErrWIFNotFound) {
			return zero, err
		}
		lastErr = err
		if attempt == wifVisibilityAttempts {
			break
		}
		if err := waitWIFVisibility(ctx, delay); err != nil {
			return zero, err
		}
	}
	return zero, lastErr
}

func waitWIFVisibility(ctx context.Context, delay time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

func retryWIFCall[T any](ctx context.Context, delay time.Duration, call func() (T, error)) (T, error) {
	var zero T
	for attempt := 1; attempt <= wifTransientAttempts; attempt++ {
		value, err := call()
		if err == nil {
			return value, nil
		}
		if !isTransientWIFError(err) || attempt == wifTransientAttempts {
			return zero, err
		}
		if err := waitWIFVisibility(ctx, delay); err != nil {
			return zero, err
		}
	}
	return zero, errors.New("WIF retry loop exhausted")
}

func isTransientWIFError(err error) bool {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func (c iamWIFClient) GetServiceAccount(ctx context.Context, name string) (WIFServiceAccount, error) {
	sa, err := retryWIFCall(ctx, wifTransientDelay, func() (*iam.ServiceAccount, error) {
		return c.iam.Projects.ServiceAccounts.Get(name).Context(ctx).Do()
	})
	if err != nil {
		return WIFServiceAccount{}, mapWIFError("get service account", err)
	}
	return WIFServiceAccount{Name: sa.Name, Email: sa.Email, Description: sa.Description}, nil
}

func (c iamWIFClient) CreateServiceAccount(ctx context.Context, projectID, accountID, displayName, description string) (WIFServiceAccount, error) {
	sa, err := c.iam.Projects.ServiceAccounts.Create("projects/"+projectID, &iam.CreateServiceAccountRequest{
		AccountId: accountID,
		ServiceAccount: &iam.ServiceAccount{
			DisplayName: displayName,
			Description: description,
		},
	}).Context(ctx).Do()
	if err != nil {
		return WIFServiceAccount{}, fmt.Errorf("create service account: %w", err)
	}
	created := WIFServiceAccount{Name: sa.Name, Email: sa.Email, Description: sa.Description}
	// IAM can report the create operation as successful before the service
	// account is visible to GetIamPolicy. Wait through that read-after-create
	// window before the caller applies the workload identity binding.
	return waitWIFResource(ctx, wifVisibilityDelay, c.GetServiceAccount, created.Name)
}

func (c iamWIFClient) GetServiceAccountIAMPolicy(ctx context.Context, resource string) (*iam.Policy, error) {
	policy, err := retryWIFCall(ctx, wifTransientDelay, func() (*iam.Policy, error) {
		return c.iam.Projects.ServiceAccounts.GetIamPolicy(resource).
			OptionsRequestedPolicyVersion(iamPolicyVersion3).
			Context(ctx).Do()
	})
	if err != nil {
		return nil, fmt.Errorf("get service account IAM policy: %w", err)
	}
	return cloneIAMPolicy(policy), nil
}

func (c iamWIFClient) SetServiceAccountIAMPolicy(ctx context.Context, resource string, policy *iam.Policy) error {
	_, err := c.iam.Projects.ServiceAccounts.SetIamPolicy(resource, &iam.SetIamPolicyRequest{
		Policy: cloneIAMPolicy(policy),
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("set service account IAM policy: %w", err)
	}
	return nil
}

func cloneIAMPolicy(policy *iam.Policy) *iam.Policy {
	if policy == nil {
		return &iam.Policy{}
	}
	out := &iam.Policy{
		Etag:    policy.Etag,
		Version: policy.Version,
	}
	if len(policy.AuditConfigs) > 0 {
		out.AuditConfigs = make([]*iam.AuditConfig, len(policy.AuditConfigs))
		for i, cfg := range policy.AuditConfigs {
			if cfg == nil {
				continue
			}
			cloned := &iam.AuditConfig{
				Service:         cfg.Service,
				AuditLogConfigs: make([]*iam.AuditLogConfig, len(cfg.AuditLogConfigs)),
			}
			for j, logCfg := range cfg.AuditLogConfigs {
				if logCfg == nil {
					continue
				}
				cloned.AuditLogConfigs[j] = &iam.AuditLogConfig{
					LogType:         logCfg.LogType,
					ExemptedMembers: append([]string(nil), logCfg.ExemptedMembers...),
				}
			}
			out.AuditConfigs[i] = cloned
		}
	}
	if len(policy.Bindings) > 0 {
		out.Bindings = make([]*iam.Binding, len(policy.Bindings))
		for i, binding := range policy.Bindings {
			if binding == nil {
				continue
			}
			cloned := &iam.Binding{
				Role:    binding.Role,
				Members: append([]string(nil), binding.Members...),
			}
			if binding.Condition != nil {
				cloned.Condition = &iam.Expr{
					Title:       binding.Condition.Title,
					Description: binding.Condition.Description,
					Expression:  binding.Condition.Expression,
					Location:    binding.Condition.Location,
				}
			}
			out.Bindings[i] = cloned
		}
	}
	return out
}

func isIAMPolicyConflict(err error) bool {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == http.StatusConflict
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
