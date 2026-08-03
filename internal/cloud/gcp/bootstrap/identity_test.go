package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeWIF struct {
	projectNumber string
	pools         map[string]WIFPool
	providers     map[string]WIFProvider
	accounts      map[string]WIFServiceAccount
	policies      map[string][]WIFBinding

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
		policies:      map[string][]WIFBinding{},
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
	created := WIFPool{Name: name, DisplayName: pool.DisplayName}
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

func (f *fakeWIF) CreateServiceAccount(_ context.Context, projectID, accountID, _ string) (WIFServiceAccount, error) {
	f.createSACalls++
	email := accountID + "@" + projectID + ".iam.gserviceaccount.com"
	name := "projects/" + projectID + "/serviceAccounts/" + email
	sa := WIFServiceAccount{Name: name, Email: email}
	f.accounts[name] = sa
	return sa, nil
}

func (f *fakeWIF) GetServiceAccountIAMPolicy(_ context.Context, resource string) ([]WIFBinding, error) {
	return append([]WIFBinding(nil), f.policies[resource]...), nil
}

func (f *fakeWIF) SetServiceAccountIAMPolicy(_ context.Context, resource string, bindings []WIFBinding) error {
	f.setPolicyCalls++
	copied := make([]WIFBinding, len(bindings))
	copy(copied, bindings)
	f.policies[resource] = copied
	return nil
}

func TestBuildIdentityPlanWIF(t *testing.T) {
	t.Parallel()
	plan, err := BuildIdentityPlan(IdentitySpec{
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "acourtiol", GitHubRepo: "magelift",
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
	if plan.AttributeMapping["attribute.repository"] != "assertion.repository" {
		t.Fatalf("mapping=%v", plan.AttributeMapping)
	}
	if plan.AttributeCondition != "assertion.repository == 'magelift/magelift'" {
		t.Fatalf("condition=%q", plan.AttributeCondition)
	}
	if !strings.HasSuffix(plan.ProviderName, "/providers/github") {
		t.Fatalf("provider=%q", plan.ProviderName)
	}
	if plan.ServiceAccountEmail != "ml-shop-preview-ci@example-gcp-project.iam.gserviceaccount.com" {
		t.Fatalf("sa=%q", plan.ServiceAccountEmail)
	}
}

func TestEnsureIdentityCreatesPoolProviderAndBinding(t *testing.T) {
	t.Parallel()
	plan, err := BuildIdentityPlan(IdentitySpec{
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "acourtiol", GitHubRepo: "magelift",
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
	if result.ProviderResource == "deferred" || strings.Contains(result.ProviderResource, "deferred") {
		t.Fatal("provider must not be deferred")
	}
	wantMember := "principalSet://iam.googleapis.com/projects/123456789012/locations/global/workloadIdentityPools/ml-shop-preview/attribute.repository/magelift/magelift"
	bindings := fake.policies[result.Plan.ServiceAccountName]
	if len(bindings) != 1 || bindings[0].Role != workloadIdentityUserRole || !containsString(bindings[0].Members, wantMember) {
		t.Fatalf("bindings=%#v", bindings)
	}

	// Second Ensure is idempotent — no extra creates when resources exist.
	if _, err := boot.Ensure(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if fake.createPoolCalls != 1 || fake.createProviderCalls != 1 || fake.createSACalls != 1 {
		t.Fatalf("idempotent creates pool=%d provider=%d sa=%d", fake.createPoolCalls, fake.createProviderCalls, fake.createSACalls)
	}
}

func TestEnsureIdentityPropagatesLoudFailures(t *testing.T) {
	t.Parallel()
	plan, err := BuildIdentityPlan(IdentitySpec{
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "acourtiol", GitHubRepo: "magelift",
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
	_, err := BuildIdentityPlan(IdentitySpec{
		Project: "shop", Environment: "preview",
		GCPProject:  "example-gcp-project",
		GitHubOwner: "acourtiol", GitHubRepo: "*",
	})
	if err == nil {
		t.Fatal("wildcard repo was accepted")
	}
}
