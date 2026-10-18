package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/api/iam/v1"
)

type fakeWIF struct {
	projectNumber string
	pools         map[string]WIFPool
	providers     map[string]WIFProvider
	accounts      map[string]WIFServiceAccount
	policies      map[string]*iam.Policy

	createPoolCalls     int
	createProviderCalls int
	createSACalls       int
	setPolicyCalls      int
	failGetPool         error
	failCreatePool      error
}

func newFakeWIF(projectNumber string) *fakeWIF {
	return &fakeWIF{
		projectNumber: projectNumber,
		pools:         map[string]WIFPool{},
		providers:     map[string]WIFProvider{},
		accounts:      map[string]WIFServiceAccount{},
		policies:      map[string]*iam.Policy{},
	}
}

func (f *fakeWIF) ProjectNumber(context.Context, string) (string, error) {
	return f.projectNumber, nil
}

func (f *fakeWIF) GetPool(_ context.Context, name string) (WIFPool, error) {
	if f.failGetPool != nil {
		return WIFPool{}, f.failGetPool
	}
	pool, ok := f.pools[name]
	if !ok {
		return WIFPool{}, ErrWIFNotFound
	}
	return pool, nil
}

func (f *fakeWIF) CreatePool(_ context.Context, parent, poolID string, pool WIFPool) (WIFPool, error) {
	f.createPoolCalls++
	if f.failCreatePool != nil {
		return WIFPool{}, f.failCreatePool
	}
	name := parent + "/workloadIdentityPools/" + poolID
	created := WIFPool{Name: name, DisplayName: pool.DisplayName, Description: pool.Description}
	f.pools[name] = created
	return created, nil
}

func (f *fakeWIF) GetProvider(_ context.Context, name string) (WIFProvider, error) {
	provider, ok := f.providers[name]
	if !ok {
		return WIFProvider{}, ErrWIFNotFound
	}
	return provider, nil
}

func (f *fakeWIF) CreateProvider(_ context.Context, parent, providerID string, provider WIFProvider) (WIFProvider, error) {
	f.createProviderCalls++
	name := parent + "/providers/" + providerID
	created := provider
	created.Name = name
	f.providers[name] = created
	return created, nil
}

func (f *fakeWIF) GetServiceAccount(_ context.Context, name string) (WIFServiceAccount, error) {
	sa, ok := f.accounts[name]
	if !ok {
		return WIFServiceAccount{}, ErrWIFNotFound
	}
	return sa, nil
}

func (f *fakeWIF) CreateServiceAccount(_ context.Context, projectID, accountID, _, description string) (WIFServiceAccount, error) {
	f.createSACalls++
	email := accountID + "@" + projectID + ".iam.gserviceaccount.com"
	name := "projects/" + projectID + "/serviceAccounts/" + email
	sa := WIFServiceAccount{Name: name, Email: email, Description: description}
	f.accounts[name] = sa
	return sa, nil
}

func (f *fakeWIF) GetServiceAccountIAMPolicy(_ context.Context, resource string) (*iam.Policy, error) {
	policy := f.policies[resource]
	if policy == nil {
		return &iam.Policy{Version: 1, Etag: "wif-initial"}, nil
	}
	return cloneIAMPolicy(policy), nil
}

func (f *fakeWIF) SetServiceAccountIAMPolicy(_ context.Context, resource string, policy *iam.Policy) error {
	f.setPolicyCalls++
	f.policies[resource] = cloneIAMPolicy(policy)
	return nil
}

func TestBuildIdentityPlanWIF(t *testing.T) {
	t.Parallel()
	plan, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "magelift", GitHubRepo: "magelift",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.PoolID != "ml-shop-preview" || plan.ProviderID != "github" {
		t.Fatalf("ids=%q/%q", plan.PoolID, plan.ProviderID)
	}
	if plan.IssuerURI != githubOIDCIssuer {
		t.Fatalf("issuer=%q", plan.IssuerURI)
	}
	if plan.AttributeMapping["attribute.environment"] != "assertion.environment" {
		t.Fatalf("mapping=%v", plan.AttributeMapping)
	}
	if !strings.Contains(plan.AttributeCondition, "assertion.repository == 'magelift/magelift'") {
		t.Fatalf("condition=%q", plan.AttributeCondition)
	}
	if !strings.Contains(plan.AttributeCondition, "assertion.workflow_ref == 'magelift/magelift/.github/workflows/magelift.yml@refs/heads/main'") {
		t.Fatalf("condition=%q", plan.AttributeCondition)
	}
	if !strings.HasSuffix(plan.ProviderName, "/providers/github") {
		t.Fatalf("provider=%q", plan.ProviderName)
	}
	if plan.ServiceAccountEmail != "ml-shop-preview-ci@example-gcp-project.iam.gserviceaccount.com" {
		t.Fatalf("sa=%q", plan.ServiceAccountEmail)
	}
	if plan.Role != identityRoleDeploy || plan.GitHubEnvironment != "preview" {
		t.Fatalf("role=%q env=%q", plan.Role, plan.GitHubEnvironment)
	}
}

func TestBuildIdentityPlanBuildAndDeployDistinct(t *testing.T) {
	t.Parallel()
	base := IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "magelift", GitHubRepo: "magelift",
	}
	deploy, err := BuildIdentityPlan(base)
	if err != nil {
		t.Fatal(err)
	}
	build, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: base.Project, Environment: base.Environment,
		GCPProject: base.GCPProject, GitHubOwner: base.GitHubOwner, GitHubRepo: base.GitHubRepo,
		Role: identityRoleBuild,
	})
	if err != nil {
		t.Fatal(err)
	}
	if deploy.ServiceAccountID == build.ServiceAccountID {
		t.Fatalf("sa ids should differ deploy=%q build=%q", deploy.ServiceAccountID, build.ServiceAccountID)
	}
	if build.ServiceAccountID != "ml-shop-preview-build" {
		t.Fatalf("build sa=%q", build.ServiceAccountID)
	}
	if deploy.PoolID != build.PoolID || deploy.ProviderName != build.ProviderName || deploy.AttributeCondition != build.AttributeCondition {
		t.Fatalf("shared provider mismatch deploy=%#v build=%#v", deploy, build)
	}
	deployMember := workloadIdentityPrincipal(deploy.PoolID, "1", deploy.GitHubEnvironment)
	buildMember := workloadIdentityPrincipal(build.PoolID, "1", build.GitHubEnvironment)
	if deployMember == buildMember {
		t.Fatalf("members should differ deploy=%q build=%q", deployMember, buildMember)
	}
}

func TestBuildIdentityPlanLongNamesDeterministic(t *testing.T) {
	t.Parallel()
	spec := IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: "very-long-magento-project-name", Environment: "high-availability",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "magelift", GitHubRepo: "magelift",
	}
	first, err := BuildIdentityPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildIdentityPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	if first.PoolID != second.PoolID || first.ServiceAccountID != second.ServiceAccountID {
		t.Fatalf("nondeterministic ids pool=%q/%q sa=%q/%q", first.PoolID, second.PoolID, first.ServiceAccountID, second.ServiceAccountID)
	}
	if len(first.PoolID) > maxWorkloadIdentityPoolIDLen || len(first.ServiceAccountID) > maxServiceAccountIDLen {
		t.Fatalf("length pool=%d sa=%d", len(first.PoolID), len(first.ServiceAccountID))
	}
}

func TestBuildIdentityPlanAcceptsGitHubNameVariants(t *testing.T) {
	t.Parallel()
	plan, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "My-Agency", GitHubRepo: "Shop.Extension",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Repository != "My-Agency/Shop.Extension" {
		t.Fatalf("repository=%q", plan.Repository)
	}
}

func TestGitHubProviderTransmitsTheRegularWorkflowClaimsContract(t *testing.T) {
	plan, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7, Project: "shop", Environment: "staging", GCPProject: "example-gcp-project", GitHubOwner: "magelift", GitHubRepo: "magelift"})
	if err != nil {
		t.Fatal(err)
	}
	// Standard workflow_ref exists on normal jobs; job_workflow_ref is reusable-workflow-only.
	want := "assertion.repository == 'magelift/magelift' && assertion.ref == 'refs/heads/main' && assertion.workflow_ref == 'magelift/magelift/.github/workflows/magelift.yml@refs/heads/main' && (assertion.event_name == 'push' || assertion.event_name == 'workflow_dispatch') && (assertion.environment == 'build' || assertion.environment == 'staging') && assertion.repository_id == '42' && assertion.repository_owner_id == '7'"
	fake := newFakeWIF("123456789012")
	boot, err := NewIdentityFromClient(fake)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boot.Ensure(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if fake.providers[plan.ProviderName].AttributeCondition != want {
		t.Fatalf("provider condition=%s", fake.providers[plan.ProviderName].AttributeCondition)
	}
}

func TestEnsureIdentityCreatesPoolProviderAndBinding(t *testing.T) {
	t.Parallel()
	plan, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "magelift", GitHubRepo: "magelift",
	})
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeWIF("123456789012")
	boot, err := NewIdentityFromClient(fake)
	if err != nil {
		t.Fatal(err)
	}
	result, err := boot.Ensure(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if fake.createPoolCalls != 1 || fake.createProviderCalls != 1 || fake.createSACalls != 1 || fake.setPolicyCalls != 1 {
		t.Fatalf("creates pool=%d provider=%d sa=%d setPolicy=%d", fake.createPoolCalls, fake.createProviderCalls, fake.createSACalls, fake.setPolicyCalls)
	}
	wantProvider := "projects/123456789012/locations/global/workloadIdentityPools/ml-shop-preview/providers/github"
	if result.ProviderResource != wantProvider {
		t.Fatalf("provider=%q want=%q", result.ProviderResource, wantProvider)
	}
	wantMember := workloadIdentityPrincipal("ml-shop-preview", "123456789012", "preview")
	policy := fake.policies[result.Plan.ServiceAccountName]
	if policy == nil {
		t.Fatal("policy missing")
	}
	var found bool
	for _, binding := range policy.Bindings {
		if binding != nil && binding.Role == workloadIdentityUserRole && binding.Condition == nil && containsString(binding.Members, wantMember) {
			found = true
		}
	}
	if !found {
		t.Fatalf("bindings=%#v", policy.Bindings)
	}

	if _, err := boot.Ensure(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if fake.createPoolCalls != 1 || fake.createProviderCalls != 1 || fake.createSACalls != 1 {
		t.Fatalf("idempotent creates pool=%d provider=%d sa=%d", fake.createPoolCalls, fake.createProviderCalls, fake.createSACalls)
	}
}

func TestEnsureBuildAndDeployUseDistinctBindings(t *testing.T) {
	t.Parallel()
	spec := IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "magelift", GitHubRepo: "magelift",
	}
	deployPlan, err := BuildIdentityPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	buildPlan, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: spec.Project, Environment: spec.Environment,
		GCPProject: spec.GCPProject, GitHubOwner: spec.GitHubOwner, GitHubRepo: spec.GitHubRepo,
		Role: identityRoleBuild,
	})
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeWIF("123456789012")
	boot, err := NewIdentityFromClient(fake)
	if err != nil {
		t.Fatal(err)
	}
	deployResult, err := boot.Ensure(context.Background(), deployPlan)
	if err != nil {
		t.Fatal(err)
	}
	buildResult, err := boot.Ensure(context.Background(), buildPlan)
	if err != nil {
		t.Fatal(err)
	}
	if fake.createProviderCalls != 1 {
		t.Fatalf("provider should be shared, create calls=%d", fake.createProviderCalls)
	}
	deployMember := workloadIdentityPrincipal(deployPlan.PoolID, "123456789012", deployPlan.GitHubEnvironment)
	buildMember := workloadIdentityPrincipal(buildPlan.PoolID, "123456789012", buildPlan.GitHubEnvironment)
	if deployMember == buildMember {
		t.Fatal("members must differ")
	}
	if !policyHasMember(fake.policies[deployResult.Plan.ServiceAccountName], deployMember) {
		t.Fatalf("deploy policy=%#v", fake.policies[deployResult.Plan.ServiceAccountName])
	}
	if !policyHasMember(fake.policies[buildResult.Plan.ServiceAccountName], buildMember) {
		t.Fatalf("build policy=%#v", fake.policies[buildResult.Plan.ServiceAccountName])
	}
}

func TestEnsureRejectsForeignPoolOwnership(t *testing.T) {
	t.Parallel()
	plan, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "magelift", GitHubRepo: "magelift",
	})
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeWIF("1")
	fake.pools[plan.PoolName] = WIFPool{Name: plan.PoolName, DisplayName: "legacy pool"}
	boot, err := NewIdentityFromClient(fake)
	if err != nil {
		t.Fatal(err)
	}
	_, err = boot.Ensure(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "ownership marker") {
		t.Fatalf("expected ownership refusal, got %v", err)
	}
	if fake.createProviderCalls != 0 || fake.createSACalls != 0 {
		t.Fatalf("must not mutate foreign resources provider=%d sa=%d", fake.createProviderCalls, fake.createSACalls)
	}
}

func TestEnsurePreservesConditionalBindingsOnIAMUpdate(t *testing.T) {
	t.Parallel()
	plan, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "magelift", GitHubRepo: "magelift",
	})
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeWIF("123456789012")
	fake.pools[plan.PoolName] = WIFPool{Name: plan.PoolName, DisplayName: plan.SharedOwnershipMarker, Description: plan.SharedOwnershipMarker}
	fake.providers[plan.ProviderName] = WIFProvider{
		Name: plan.ProviderName, DisplayName: plan.SharedOwnershipMarker, Description: plan.SharedOwnershipMarker,
		IssuerURI: plan.IssuerURI, AttributeMapping: plan.AttributeMapping, AttributeCondition: plan.AttributeCondition,
	}
	fake.accounts[plan.ServiceAccountName] = WIFServiceAccount{
		Name: plan.ServiceAccountName, Email: plan.ServiceAccountEmail, Description: plan.ServiceAccountMarker,
	}
	fake.policies[plan.ServiceAccountName] = &iam.Policy{
		Version: 3,
		Etag:    "abc",
		AuditConfigs: []*iam.AuditConfig{{
			Service:         "allServices",
			AuditLogConfigs: []*iam.AuditLogConfig{{LogType: "ADMIN_READ"}},
		}},
		Bindings: []*iam.Binding{{
			Role:      workloadIdentityUserRole,
			Members:   []string{"user:admin@example.com"},
			Condition: &iam.Expr{Expression: "request.time < timestamp('2026-01-01T00:00:00Z')"},
		}},
	}
	boot, err := NewIdentityFromClient(fake)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := boot.Ensure(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	policy := fake.policies[plan.ServiceAccountName]
	if policy.Version != 3 || policy.Etag != "abc" || len(policy.AuditConfigs) != 1 {
		t.Fatalf("policy metadata dropped: %#v", policy)
	}
	if len(policy.Bindings) != 2 {
		t.Fatalf("expected conditional + unconditional bindings, got %#v", policy.Bindings)
	}
	if policy.Bindings[0].Condition == nil || policy.Bindings[0].Condition.Expression == "" {
		t.Fatalf("conditional binding mutated: %#v", policy.Bindings[0])
	}
}

func TestEnsureIdentityPropagatesLoudFailures(t *testing.T) {
	t.Parallel()
	plan, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "magelift", GitHubRepo: "magelift",
	})
	if err != nil {
		t.Fatal(err)
	}
	fake := newFakeWIF("1")
	fake.failGetPool = errors.New("iam permission denied")
	boot, err := NewIdentityFromClient(fake)
	if err != nil {
		t.Fatal(err)
	}
	_, err = boot.Ensure(context.Background(), plan)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected loud failure, got %v", err)
	}
}

func TestBuildIdentityPlanRejectsWildcardRepo(t *testing.T) {
	t.Parallel()
	_, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "magelift", GitHubRepo: "*",
	})
	if err == nil {
		t.Fatal("wildcard repo was accepted")
	}
}

func TestWaitWIFResourceRetriesIAMVisibility(t *testing.T) {
	t.Parallel()
	attempts := 0
	pool, err := waitWIFResource(context.Background(), 0, func(_ context.Context, _ string) (WIFPool, error) {
		attempts++
		if attempts < 3 {
			return WIFPool{}, ErrWIFNotFound
		}
		return WIFPool{Name: "visible"}, nil
	}, "pool")
	if err != nil {
		t.Fatal(err)
	}
	if pool.Name != "visible" || attempts != 3 {
		t.Fatalf("pool=%#v attempts=%d", pool, attempts)
	}
}

func TestWaitWIFResourcePropagatesNonNotFound(t *testing.T) {
	t.Parallel()
	want := errors.New("permission denied")
	_, err := waitWIFResource(context.Background(), 0, func(_ context.Context, _ string) (WIFPool, error) {
		return WIFPool{}, want
	}, "pool")
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want=%v", err, want)
	}
}

func policyHasMember(policy *iam.Policy, member string) bool {
	if policy == nil {
		return false
	}
	for _, binding := range policy.Bindings {
		if binding != nil && binding.Role == workloadIdentityUserRole && containsString(binding.Members, member) {
			return true
		}
	}
	return false
}

func TestWIFRefusesOwnershipMarkerPrefixCollisions(t *testing.T) {
	plan, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7, Project: "shop", Environment: "test", GCPProject: "example-gcp-project", GitHubOwner: "magelift", GitHubRepo: "magelift"})
	if err != nil {
		t.Fatal(err)
	}
	for _, description := range []string{plan.SharedOwnershipMarker + "-other", "unmanaged " + plan.SharedOwnershipMarker} {
		fake := newFakeWIF("123456789012")
		fake.pools[plan.PoolName] = WIFPool{Name: plan.PoolName, DisplayName: plan.SharedOwnershipMarker, Description: description}
		boot, _ := NewIdentityFromClient(fake)
		if _, err := boot.Ensure(context.Background(), plan); err == nil {
			t.Fatal("must not adopt a prefix/suffix ownership match")
		}
		if fake.createPoolCalls != 0 || fake.createProviderCalls != 0 || fake.createSACalls != 0 || fake.setPolicyCalls != 0 {
			t.Fatal("foreign pool caused mutations")
		}
	}
	if err := validateServiceAccountOwnership(plan.ServiceAccountMarker, plan.ServiceAccountMarker+"-other"); err == nil {
		t.Fatal("must not adopt SA description prefix")
	}
}

func TestWIFReservesBuildEnvironmentForRoleSeparation(t *testing.T) {
	_, err := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7, Project: "shop", Environment: "build", GCPProject: "example-gcp-project", GitHubOwner: "magelift", GitHubRepo: "magelift"})
	if err == nil {
		t.Fatal("build and deploy environments must remain distinct")
	}
}

func TestIdentityPlanRequiresImmutableRepositoryAndOwnerIDs(t *testing.T) {
	for _, ids := range [][2]int64{{0, 7}, {42, 0}, {-1, 7}, {42, -1}} {
		_, err := BuildIdentityPlan(IdentitySpec{Project: "shop", Environment: "test", GCPProject: "example-gcp-project", GitHubOwner: "agency", GitHubRepo: "shop", GitHubRepositoryID: ids[0], GitHubOwnerID: ids[1]})
		if err == nil {
			t.Fatalf("accepted unpinned repository IDs %v", ids)
		}
	}
}
