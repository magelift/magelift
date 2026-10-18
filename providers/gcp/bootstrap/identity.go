package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/iam/v1"
)

const (
	githubOIDCIssuer             = "https://token.actions.githubusercontent.com"
	workloadIdentityUserRole     = "roles/iam.workloadIdentityUser"
	defaultWIFProviderID         = "github"
	defaultWIFPoolDisplayLimit   = 32
	maxWorkloadIdentityPoolIDLen = 32
	maxServiceAccountIDLen       = 30
	githubWorkflowFile           = ".github/workflows/magelift.yml"
	githubTrustedRef             = "refs/heads/main"
	identityRoleDeploy           = "deploy"
	identityRoleBuild            = "build"
	githubEnvironmentBuild       = "build"
)

const wifOwnershipRefusalHint = "select a fresh MageLift project/environment name, or have the resource owner review recorded ownership before adoption"

// IdentitySpec is the Magento-shaped input for GitHub→GCP WIF bootstrap.
type IdentitySpec struct {
	Project            string
	Environment        string
	GCPProject         string
	GitHubOwner        string
	GitHubRepo         string
	GitHubRepositoryID int64
	GitHubOwnerID      int64
	// Role selects the CI identity: empty or "deploy" for deployment SA, "build" for image build SA.
	Role string
}

// IdentityPlan is the deterministic WIF resource plan (no live API calls).
type IdentityPlan struct {
	Project                string            `json:"project" yaml:"project"`
	Environment            string            `json:"environment" yaml:"environment"`
	Role                   string            `json:"role" yaml:"role"`
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
	GitHubRepositoryID     int64             `json:"githubRepositoryId" yaml:"githubRepositoryId"`
	GitHubOwnerID          int64             `json:"githubOwnerId" yaml:"githubOwnerId"`
	GitHubEnvironment      string            `json:"githubEnvironment" yaml:"githubEnvironment"`
	WorkloadIdentityMember string            `json:"workloadIdentityMember" yaml:"workloadIdentityMember"`
	SharedOwnershipMarker  string            `json:"sharedOwnershipMarker" yaml:"sharedOwnershipMarker"`
	ServiceAccountMarker   string            `json:"serviceAccountMarker" yaml:"serviceAccountMarker"`
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
	if !validGitHubName(spec.GitHubOwner) || !validGitHubName(spec.GitHubRepo) {
		return IdentityPlan{}, errors.New("GitHub owner and repository must be valid names")
	}
	if spec.GitHubRepositoryID <= 0 || spec.GitHubOwnerID <= 0 {
		return IdentityPlan{}, errors.New("GitHub repository and owner numeric IDs are required; resolve repository metadata before cloud preparation")
	}
	role, err := normalizeIdentityRole(spec.Role)
	if err != nil {
		return IdentityPlan{}, err
	}
	if spec.Environment == githubEnvironmentBuild {
		return IdentityPlan{}, errors.New("environment build is reserved for the CI build identity; choose a shop environment such as test")
	}
	repository := spec.GitHubOwner + "/" + spec.GitHubRepo
	poolID := compactIdentityID(
		"ml-"+spec.Project+"-"+spec.Environment,
		maxWorkloadIdentityPoolIDLen,
		spec.Project+"/"+spec.Environment+"/pool",
	)
	saSuffix := "-ci"
	githubEnvironment := spec.Environment
	if role == identityRoleBuild {
		saSuffix = "-build"
		githubEnvironment = githubEnvironmentBuild
	}
	saID := compactIdentityID(
		"ml-"+spec.Project+"-"+spec.Environment+saSuffix,
		maxServiceAccountIDLen,
		spec.Project+"/"+spec.Environment+"/"+role+"/sa",
	)
	parent := "projects/" + spec.GCPProject + "/locations/global"
	poolName := parent + "/workloadIdentityPools/" + poolID
	providerName := poolName + "/providers/" + defaultWIFProviderID
	saEmail := saID + "@" + spec.GCPProject + ".iam.gserviceaccount.com"
	saName := "projects/" + spec.GCPProject + "/serviceAccounts/" + saEmail
	mapping := githubAttributeMapping()
	condition := githubAttributeCondition(repository, spec.Environment)
	condition += fmt.Sprintf(" && assertion.repository_id == '%d' && assertion.repository_owner_id == '%d'", spec.GitHubRepositoryID, spec.GitHubOwnerID)
	sharedMarker := wifSharedOwnershipMarker(spec.Project, spec.Environment)
	saMarker := wifServiceAccountOwnershipMarker(spec.Project, spec.Environment, role)
	member := workloadIdentityPrincipal(poolID, "{project_number}", githubEnvironment)
	return IdentityPlan{
		Project:                spec.Project,
		Environment:            spec.Environment,
		Role:                   role,
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
		GitHubRepositoryID:     spec.GitHubRepositoryID,
		GitHubOwnerID:          spec.GitHubOwnerID,
		GitHubEnvironment:      githubEnvironment,
		WorkloadIdentityMember: member,
		SharedOwnershipMarker:  sharedMarker,
		ServiceAccountMarker:   saMarker,
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
	if strings.TrimSpace(plan.SharedOwnershipMarker) == "" || strings.TrimSpace(plan.ServiceAccountMarker) == "" {
		return IdentityResult{}, errors.New("identity plan ownership markers are incomplete")
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
	member := workloadIdentityPrincipal(plan.PoolID, projectNumber, plan.GitHubEnvironment)
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
		if err := validateSharedWIFOwnership(plan.SharedOwnershipMarker, pool.DisplayName, pool.Description); err != nil {
			return WIFPool{}, err
		}
		return pool, nil
	}
	if !errors.Is(err, ErrWIFNotFound) {
		return WIFPool{}, err
	}
	marker := plan.SharedOwnershipMarker
	return b.wif.CreatePool(ctx, parent, plan.PoolID, WIFPool{
		DisplayName: truncateDisplay(marker),
		Description: marker,
	})
}

func (b *IdentityBootstrapper) ensureProvider(ctx context.Context, poolName string, plan IdentityPlan) (WIFProvider, error) {
	provider, err := b.wif.GetProvider(ctx, plan.ProviderName)
	if err == nil {
		if err := validateSharedWIFOwnership(plan.SharedOwnershipMarker, provider.DisplayName, provider.Description); err != nil {
			return WIFProvider{}, err
		}
		if err := validateExistingProvider(provider, plan); err != nil {
			return WIFProvider{}, err
		}
		return provider, nil
	}
	if !errors.Is(err, ErrWIFNotFound) {
		return WIFProvider{}, err
	}
	marker := plan.SharedOwnershipMarker
	return b.wif.CreateProvider(ctx, poolName, plan.ProviderID, WIFProvider{
		DisplayName:        truncateDisplay(marker),
		Description:        marker,
		IssuerURI:          plan.IssuerURI,
		AttributeMapping:   plan.AttributeMapping,
		AttributeCondition: plan.AttributeCondition,
	})
}

func validateExistingProvider(provider WIFProvider, plan IdentityPlan) error {
	if strings.TrimSpace(provider.IssuerURI) != plan.IssuerURI {
		return fmt.Errorf("workload identity provider %q issuer mismatch: got %q, want %q", plan.ProviderName, provider.IssuerURI, plan.IssuerURI)
	}
	if !attributeMappingsEqual(provider.AttributeMapping, plan.AttributeMapping) {
		return fmt.Errorf("workload identity provider %q attribute mapping mismatch", plan.ProviderName)
	}
	if provider.AttributeCondition != plan.AttributeCondition {
		return fmt.Errorf("workload identity provider %q attribute condition mismatch", plan.ProviderName)
	}
	return nil
}

func (b *IdentityBootstrapper) ensureServiceAccount(ctx context.Context, plan IdentityPlan) (WIFServiceAccount, error) {
	sa, err := b.wif.GetServiceAccount(ctx, plan.ServiceAccountName)
	if err == nil {
		if err := validateServiceAccountOwnership(plan.ServiceAccountMarker, sa.Description); err != nil {
			return WIFServiceAccount{}, err
		}
		return sa, nil
	}
	if !errors.Is(err, ErrWIFNotFound) {
		return WIFServiceAccount{}, err
	}
	display := truncateDisplay(plan.ServiceAccountMarker)
	return b.wif.CreateServiceAccount(ctx, plan.GCPProject, plan.ServiceAccountID, display, plan.ServiceAccountMarker)
}

func (b *IdentityBootstrapper) ensureWorkloadIdentityBinding(ctx context.Context, saResource, member string) error {
	policy, err := b.wif.GetServiceAccountIAMPolicy(ctx, saResource)
	if err != nil {
		return err
	}
	updated, changed := mergeUnconditionalWorkloadIdentityMember(policy, member)
	if !changed {
		return nil
	}
	if err := b.wif.SetServiceAccountIAMPolicy(ctx, saResource, updated); err != nil {
		if !isIAMPolicyConflict(err) {
			return err
		}
		fresh, err := b.wif.GetServiceAccountIAMPolicy(ctx, saResource)
		if err != nil {
			return err
		}
		retry, changed := mergeUnconditionalWorkloadIdentityMember(fresh, member)
		if !changed {
			return nil
		}
		return b.wif.SetServiceAccountIAMPolicy(ctx, saResource, retry)
	}
	return nil
}

func mergeUnconditionalWorkloadIdentityMember(policy *iam.Policy, member string) (*iam.Policy, bool) {
	if policy == nil {
		policy = &iam.Policy{}
	}
	for _, binding := range policy.Bindings {
		if binding == nil || binding.Role != workloadIdentityUserRole {
			continue
		}
		if binding.Condition != nil {
			continue
		}
		if containsString(binding.Members, member) {
			return policy, false
		}
		binding.Members = append(binding.Members, member)
		return policy, true
	}
	policy.Bindings = append(policy.Bindings, &iam.Binding{
		Role:    workloadIdentityUserRole,
		Members: []string{member},
	})
	return policy, true
}

func normalizeIdentityRole(role string) (string, error) {
	role = strings.TrimSpace(strings.ToLower(role))
	switch role {
	case "", identityRoleDeploy:
		return identityRoleDeploy, nil
	case identityRoleBuild:
		return identityRoleBuild, nil
	default:
		return "", fmt.Errorf("identity role %q is invalid: use build or deploy", role)
	}
}

func validGitHubName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 100 || name == "*" {
		return false
	}
	if strings.Contains(name, "/") || strings.Contains(name, " ") {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func githubAttributeMapping() map[string]string {
	return map[string]string{
		"google.subject":        "assertion.sub",
		"attribute.actor":       "assertion.actor",
		"attribute.repository":  "assertion.repository",
		"attribute.environment": "assertion.environment",
	}
}

func githubAttributeCondition(repository, magentoEnvironment string) string {
	workflowRef := repository + "/" + githubWorkflowFile + "@" + githubTrustedRef
	envCheck := fmt.Sprintf(
		"(assertion.environment == '%s' || assertion.environment == '%s')",
		celQuote(githubEnvironmentBuild),
		celQuote(magentoEnvironment),
	)
	return fmt.Sprintf(
		"assertion.repository == '%s' && assertion.ref == '%s' && assertion.workflow_ref == '%s' && (assertion.event_name == 'push' || assertion.event_name == 'workflow_dispatch') && %s",
		celQuote(repository),
		celQuote(githubTrustedRef),
		celQuote(workflowRef),
		envCheck,
	)
}

func workloadIdentityPrincipal(poolID, projectNumber, githubEnvironment string) string {
	return fmt.Sprintf(
		"principalSet://iam.googleapis.com/projects/%s/locations/global/workloadIdentityPools/%s/attribute.environment/%s",
		projectNumber, poolID, githubEnvironment,
	)
}

func wifSharedOwnershipMarker(project, environment string) string {
	return fmt.Sprintf("MageLift project=%s environment=%s", project, environment)
}

func wifServiceAccountOwnershipMarker(project, environment, role string) string {
	return fmt.Sprintf("%s role=%s", wifSharedOwnershipMarker(project, environment), role)
}

func validateSharedWIFOwnership(marker, _, description string) error {
	if strings.TrimSpace(description) == marker {
		return nil
	}
	return fmt.Errorf(
		"refusing to mutate workload identity resource: missing ownership marker %q; %s",
		marker, wifOwnershipRefusalHint,
	)
}

func validateServiceAccountOwnership(marker, description string) error {
	if strings.TrimSpace(description) == marker {
		return nil
	}
	return fmt.Errorf(
		"refusing to mutate service account: missing ownership marker %q; %s",
		marker, wifOwnershipRefusalHint,
	)
}

func attributeMappingsEqual(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, wantValue := range want {
		if got[key] != wantValue {
			return false
		}
	}
	return true
}

func compactIdentityID(value string, maxLen int, seed string) string {
	if len(value) <= maxLen {
		return value
	}
	digest := sha256.Sum256([]byte(seed))
	suffix := hex.EncodeToString(digest[:])[:8]
	if maxLen <= len(suffix)+1 {
		return suffix[:maxLen]
	}
	prefixLen := maxLen - len(suffix) - 1
	prefix := strings.TrimRight(value[:prefixLen], "-")
	return prefix + "-" + suffix
}

func celQuote(value string) string {
	return strings.ReplaceAll(value, "'", "\\'")
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
