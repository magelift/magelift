package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const (
	githubOIDCIssuer           = "https://token.actions.githubusercontent.com"
	workloadIdentityUserRole   = "roles/iam.workloadIdentityUser"
	defaultWIFProviderID       = "github"
	defaultWIFProviderDisplay  = "GitHub Actions"
	defaultWIFPoolDisplayLimit = 32
)

// IdentitySpec is the Magento-shaped input for GitHub→GCP WIF bootstrap.
type IdentitySpec struct {
	Project     string
	Environment string
	GCPProject  string
	GitHubOwner string
	GitHubRepo  string
}

// IdentityPlan is the deterministic WIF resource plan (no live API calls).
type IdentityPlan struct {
	GCPProject             string            `json:"gcpProject" yaml:"gcpProject"`
	PoolID                 string            `json:"poolId" yaml:"poolId"`
	PoolName               string            `json:"poolName" yaml:"poolName"`
	ProviderID             string            `json:"providerId" yaml:"providerId"`
	ProviderName           string            `json:"providerName" yaml:"providerName"`
	ServiceAccountID       string            `json:"serviceAccountId" yaml:"serviceAccountId"`
	ServiceAccountEmail    string            `json:"serviceAccountEmail" yaml:"serviceAccountEmail"`
	ServiceAccountName     string            `json:"serviceAccountName" yaml:"serviceAccountName"`
	IssuerURI              string            `json:"issuerUri" yaml:"issuerUri"`
	AttributeMapping       map[string]string `json:"attributeMapping" yaml:"attributeMapping"`
	AttributeCondition     string            `json:"attributeCondition" yaml:"attributeCondition"`
	Repository             string            `json:"repository" yaml:"repository"`
	WorkloadIdentityMember string            `json:"workloadIdentityMember" yaml:"workloadIdentityMember"`
}

// IdentityResult is the Ensure output used in Bootstrap Details.
type IdentityResult struct {
	Plan                IdentityPlan `json:"plan" yaml:"plan"`
	ProjectNumber       string       `json:"projectNumber" yaml:"projectNumber"`
	ProviderResource    string       `json:"providerResource" yaml:"providerResource"`
	ServiceAccountEmail string       `json:"serviceAccountEmail" yaml:"serviceAccountEmail"`
	PoolResource        string       `json:"poolResource" yaml:"poolResource"`
}

// IdentityBootstrapper ensures WIF pool + GitHub OIDC provider + CI SA binding.
type IdentityBootstrapper struct {
	wif WIFAPI
}

// NewIdentityFromClient injects a fake or real WIF client.
func NewIdentityFromClient(wif WIFAPI) (*IdentityBootstrapper, error) {
	if wif == nil {
		return nil, errors.New("WIF client is required")
	}
	return &IdentityBootstrapper{wif: wif}, nil
}

// NewIdentity builds a live WIF client.
func NewIdentity(ctx context.Context) (*IdentityBootstrapper, error) {
	client, err := NewWIFClient(ctx)
	if err != nil {
		return nil, err
	}
	return NewIdentityFromClient(client)
}

// BuildIdentityPlan validates inputs and returns resource IDs/names for Ensure.
func BuildIdentityPlan(spec IdentitySpec) (IdentityPlan, error) {
	if !componentPattern.MatchString(spec.Project) || !componentPattern.MatchString(spec.Environment) {
		return IdentityPlan{}, errors.New("identity bootstrap project and environment must be stable names")
	}
	if !projectPattern.MatchString(spec.GCPProject) {
		return IdentityPlan{}, errors.New("identity bootstrap GCP project ID is invalid")
	}
	if !componentPattern.MatchString(spec.GitHubOwner) || !componentPattern.MatchString(spec.GitHubRepo) {
		return IdentityPlan{}, errors.New("GitHub owner and repository must be lowercase stable names")
	}
	repository := spec.GitHubOwner + "/" + spec.GitHubRepo
	poolID := "ml-" + spec.Project + "-" + spec.Environment
	if len(poolID) > 32 {
		return IdentityPlan{}, errors.New("generated workload identity pool ID exceeds 32 characters")
	}
	saID := "ml-" + spec.Project + "-" + spec.Environment + "-ci"
	if len(saID) > 30 {
		return IdentityPlan{}, errors.New("generated service account ID exceeds 30 characters")
	}
	parent := "projects/" + spec.GCPProject + "/locations/global"
	poolName := parent + "/workloadIdentityPools/" + poolID
	providerName := poolName + "/providers/" + defaultWIFProviderID
	saEmail := saID + "@" + spec.GCPProject + ".iam.gserviceaccount.com"
	saName := "projects/" + spec.GCPProject + "/serviceAccounts/" + saEmail
	condition := fmt.Sprintf("assertion.repository == '%s'", repository)
	mapping := map[string]string{
		"google.subject":       "assertion.sub",
		"attribute.actor":      "assertion.actor",
		"attribute.repository": "assertion.repository",
	}
	// Project number is filled during Ensure; plan uses project ID for create parents.
	member := "principalSet://iam.googleapis.com/projects/{project_number}/locations/global/workloadIdentityPools/" +
		poolID + "/attribute.repository/" + repository
	return IdentityPlan{
		GCPProject:             spec.GCPProject,
		PoolID:                 poolID,
		PoolName:               poolName,
		ProviderID:             defaultWIFProviderID,
		ProviderName:           providerName,
		ServiceAccountID:       saID,
		ServiceAccountEmail:    saEmail,
		ServiceAccountName:     saName,
		IssuerURI:              githubOIDCIssuer,
		AttributeMapping:       mapping,
		AttributeCondition:     condition,
		Repository:             repository,
		WorkloadIdentityMember: member,
	}, nil
}

// Ensure creates or verifies the WIF pool, GitHub OIDC provider, CI SA, and binding.
func (b *IdentityBootstrapper) Ensure(ctx context.Context, plan IdentityPlan) (IdentityResult, error) {
	if b == nil || b.wif == nil {
		return IdentityResult{}, errors.New("WIF identity bootstrapper is required")
	}
	if strings.TrimSpace(plan.PoolID) == "" || strings.TrimSpace(plan.ProviderID) == "" || strings.TrimSpace(plan.ServiceAccountID) == "" {
		return IdentityResult{}, errors.New("identity plan is incomplete")
	}
	projectNumber, err := b.wif.ProjectNumber(ctx, plan.GCPProject)
	if err != nil {
		return IdentityResult{}, err
	}
	parent := "projects/" + plan.GCPProject + "/locations/global"
	pool, err := b.ensurePool(ctx, parent, plan)
	if err != nil {
		return IdentityResult{}, err
	}
	provider, err := b.ensureProvider(ctx, pool.Name, plan)
	if err != nil {
		return IdentityResult{}, err
	}
	sa, err := b.ensureServiceAccount(ctx, plan)
	if err != nil {
		return IdentityResult{}, err
	}
	member := fmt.Sprintf(
		"principalSet://iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s/attribute.repository/%s",
		projectNumber, plan.PoolID, plan.Repository,
	)
	if err := b.ensureWorkloadIdentityBinding(ctx, sa.Name, member); err != nil {
		return IdentityResult{}, err
	}
	providerResource := rewriteProjectNumber(provider.Name, plan.GCPProject, projectNumber)
	poolResource := rewriteProjectNumber(pool.Name, plan.GCPProject, projectNumber)
	plan.PoolName = poolResource
	plan.ProviderName = providerResource
	plan.WorkloadIdentityMember = member
	plan.ServiceAccountEmail = sa.Email
	plan.ServiceAccountName = sa.Name
	return IdentityResult{
		Plan:                plan,
		ProjectNumber:       projectNumber,
		ProviderResource:    providerResource,
		ServiceAccountEmail: sa.Email,
		PoolResource:        poolResource,
	}, nil
}

func (b *IdentityBootstrapper) ensurePool(ctx context.Context, parent string, plan IdentityPlan) (WIFPool, error) {
	pool, err := b.wif.GetPool(ctx, plan.PoolName)
	if err == nil {
		return pool, nil
	}
	if !errors.Is(err, ErrWIFNotFound) {
		return WIFPool{}, err
	}
	display := truncateDisplay("magelift " + plan.PoolID)
	return b.wif.CreatePool(ctx, parent, plan.PoolID, WIFPool{DisplayName: display})
}

func (b *IdentityBootstrapper) ensureProvider(ctx context.Context, poolName string, plan IdentityPlan) (WIFProvider, error) {
	provider, err := b.wif.GetProvider(ctx, plan.ProviderName)
	if err == nil {
		if provider.AttributeCondition != plan.AttributeCondition {
			return WIFProvider{}, fmt.Errorf("workload identity provider %q attribute condition mismatch", plan.ProviderName)
		}
		return provider, nil
	}
	if !errors.Is(err, ErrWIFNotFound) {
		return WIFProvider{}, err
	}
	return b.wif.CreateProvider(ctx, poolName, plan.ProviderID, WIFProvider{
		DisplayName:        defaultWIFProviderDisplay,
		IssuerURI:          plan.IssuerURI,
		AttributeMapping:   plan.AttributeMapping,
		AttributeCondition: plan.AttributeCondition,
	})
}

func (b *IdentityBootstrapper) ensureServiceAccount(ctx context.Context, plan IdentityPlan) (WIFServiceAccount, error) {
	sa, err := b.wif.GetServiceAccount(ctx, plan.ServiceAccountName)
	if err == nil {
		return sa, nil
	}
	if !errors.Is(err, ErrWIFNotFound) {
		return WIFServiceAccount{}, err
	}
	return b.wif.CreateServiceAccount(ctx, plan.GCPProject, plan.ServiceAccountID, "MageLift CI")
}

func (b *IdentityBootstrapper) ensureWorkloadIdentityBinding(ctx context.Context, saResource, member string) error {
	bindings, err := b.wif.GetServiceAccountIAMPolicy(ctx, saResource)
	if err != nil {
		return err
	}
	for i := range bindings {
		if bindings[i].Role != workloadIdentityUserRole {
			continue
		}
		if containsString(bindings[i].Members, member) {
			return nil
		}
		bindings[i].Members = append(bindings[i].Members, member)
		return b.wif.SetServiceAccountIAMPolicy(ctx, saResource, bindings)
	}
	bindings = append(bindings, WIFBinding{Role: workloadIdentityUserRole, Members: []string{member}})
	return b.wif.SetServiceAccountIAMPolicy(ctx, saResource, bindings)
}

func rewriteProjectNumber(name, projectID, projectNumber string) string {
	prefix := "projects/" + projectID + "/"
	if strings.HasPrefix(name, prefix) {
		return "projects/" + projectNumber + "/" + strings.TrimPrefix(name, prefix)
	}
	return name
}

func truncateDisplay(value string) string {
	if len(value) <= defaultWIFPoolDisplayLimit {
		return value
	}
	return value[:defaultWIFPoolDisplayLimit]
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
