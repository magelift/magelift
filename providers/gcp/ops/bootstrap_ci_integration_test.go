package ops

import (
	"context"
	"errors"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/githubci"
	"github.com/magelift/magelift/internal/platform"
	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
	"google.golang.org/api/iam/v1"
)

const ciIntegrationProjectNumber = "424242424242"

func TestBootstrapEnsurePlansCIPermissionsWithoutInvokingGrants(t *testing.T) {
	b := ciIntegrationBootstrap(t, nil, false)
	result, err := b.Ensure(context.Background(), gcpTestSpec(), ciBootstrapRequest(false))
	if err != nil {
		t.Fatal(err)
	}
	if applied, ok := result.Details["ciPermissionsApplied"].(bool); !ok || applied {
		t.Fatalf("ciPermissionsApplied = %v, details = %#v", result.Details["ciPermissionsApplied"], result.Details)
	}
	plan, ok := result.Details["ciPermissionPlan"].(gcpbootstrap.CIGrantPlan)
	if !ok || len(plan.Bindings) == 0 || plan.ProjectNumber != ciIntegrationProjectNumber {
		t.Fatalf("ciPermissionPlan missing or empty: %#v", result.Details["ciPermissionPlan"])
	}
	if result.Details["ciVariables"] == nil || result.Details["wif"] == nil {
		t.Fatalf("identity details missing: %#v", result.Details)
	}
}

func TestBootstrapEnsureApplyCIPermissionsEnsuresOnceAndCloses(t *testing.T) {
	tracker := &trackingCIGrants{}
	b := ciIntegrationBootstrap(t, tracker, true)
	result, err := b.Ensure(context.Background(), gcpTestSpec(), ciBootstrapRequest(true))
	if err != nil {
		t.Fatal(err)
	}
	if tracker.ensureCalls != 1 || tracker.closeCalls != 1 {
		t.Fatalf("grants ensure=%d close=%d", tracker.ensureCalls, tracker.closeCalls)
	}
	applied, ok := result.Details["ciPermissionsApplied"].(bool)
	if !ok || !applied {
		t.Fatalf("ciPermissionsApplied = %v", result.Details["ciPermissionsApplied"])
	}
	if tracker.lastInputs.ProjectNumber != ciIntegrationProjectNumber {
		t.Fatalf("grant inputs project number = %q", tracker.lastInputs.ProjectNumber)
	}
}

func TestBootstrapEnsureApplyCIPermissionsFailureReturnsError(t *testing.T) {
	grantErr := errors.New("ci grant apply refused")
	tracker := &trackingCIGrants{failEnsure: grantErr}
	b := ciIntegrationBootstrap(t, tracker, true)
	result, err := b.Ensure(context.Background(), gcpTestSpec(), ciBootstrapRequest(true))
	if !errors.Is(err, grantErr) {
		t.Fatalf("error = %v, want %v", err, grantErr)
	}
	if result.BackendURL != "" || len(result.Details) > 0 {
		t.Fatalf("success payload returned on failure: %#v", result)
	}
	if tracker.ensureCalls != 1 || tracker.closeCalls != 1 {
		t.Fatalf("grants ensure=%d close=%d", tracker.ensureCalls, tracker.closeCalls)
	}
}

func ciBootstrapRequest(apply bool) platform.BootstrapRequest {
	return platform.BootstrapRequest{
		GitHubOwner:        "agency",
		GitHubRepo:         "shop",
		ApplyCIPermissions: apply,
	}
}

func ciIntegrationBootstrap(t *testing.T, tracker *trackingCIGrants, wireGrants bool) Bootstrap {
	t.Helper()
	buckets := newIntegrationBuckets()
	ensurer, err := gcpbootstrap.NewFromClient(buckets)
	if err != nil {
		t.Fatal(err)
	}
	wif := newIntegrationWIF(ciIntegrationProjectNumber)
	identity, err := gcpbootstrap.NewIdentityFromClient(wif)
	if err != nil {
		t.Fatal(err)
	}
	prep := &integrationAccountPrep{t: t, buckets: buckets}
	b := Bootstrap{
		ResolveGitHubRepository: func(_ context.Context, owner, repo string) (githubci.Repository, error) {
			if owner != "agency" || repo != "shop" {
				t.Fatalf("unexpected repository %q/%q", owner, repo)
			}
			return githubci.Repository{OwnerLogin: "Agency", Name: "Shop", ID: 42, OwnerID: 7}, nil
		},
		NewPreparation: func(context.Context) (AccountPreparation, error) { return prep, nil },
		NewEnsurer:     func(context.Context) (*gcpbootstrap.Bootstrapper, error) { return ensurer, nil },
		NewIdentity:    func(context.Context) (*gcpbootstrap.IdentityBootstrapper, error) { return identity, nil },
	}
	if wireGrants {
		if tracker == nil {
			t.Fatal("tracker required when wiring CI grants")
		}
		b.NewCIGrants = func(context.Context) (CIGrantPreparation, error) { return tracker, nil }
	} else {
		b.NewCIGrants = func(context.Context) (CIGrantPreparation, error) {
			t.Fatal("NewCIGrants must not run without ApplyCIPermissions")
			return nil, nil
		}
	}
	return b
}

type trackingCIGrants struct {
	ensureCalls int
	closeCalls  int
	failEnsure  error
	lastInputs  gcpbootstrap.CIGrantInputs
}

func (g *trackingCIGrants) Ensure(_ context.Context, inputs gcpbootstrap.CIGrantInputs) error {
	g.ensureCalls++
	g.lastInputs = inputs
	if g.failEnsure != nil {
		return g.failEnsure
	}
	return nil
}

func (g *trackingCIGrants) Close() error {
	g.closeCalls++
	return nil
}

type integrationAccountPrep struct {
	t         *testing.T
	buckets   *integrationBuckets
	apisReady bool
	keyReady  bool
}

func (p *integrationAccountPrep) EnsureAPIs(context.Context, gcpbootstrap.PreparationPlan, gcpbootstrap.Spec) error {
	p.apisReady = true
	return nil
}

func (p *integrationAccountPrep) EnsureStatePassphrase(ctx context.Context, plan gcpbootstrap.PreparationPlan, spec gcpbootstrap.Spec, owner string) (gcpbootstrap.StatePassphraseRef, error) {
	exists, _, err := p.buckets.BucketExists(ctx, plan.Base.StateBucket)
	if err != nil {
		return gcpbootstrap.StatePassphraseRef{}, err
	}
	if !exists || !p.apisReady || !strings.HasPrefix(owner, "bootstrap-") {
		return gcpbootstrap.StatePassphraseRef{}, errors.New("state initialization preceded prerequisites")
	}
	p.keyReady = true
	return gcpbootstrap.StatePassphraseRef{
		SecretID:    plan.StateSecretID,
		VersionName: "projects/" + spec.GCPProject + "/secrets/" + plan.StateSecretID + "/versions/1",
	}, nil
}

func (p *integrationAccountPrep) EnsureRepository(_ context.Context, plan gcpbootstrap.PreparationPlan, _ gcpbootstrap.Spec) (gcpbootstrap.RepositoryResult, error) {
	if !p.keyReady {
		p.t.Fatal("registry preparation preceded state initialization")
	}
	return gcpbootstrap.RepositoryResult{
		RepositoryID: plan.RegistryID,
		Location:     plan.RegistryLocation,
		ImageBase:    plan.ImageBase,
		RegistryHost: plan.RegistryLocation + "-docker.pkg.dev",
	}, nil
}

func (p *integrationAccountPrep) Close() error { return nil }

type integrationBuckets struct {
	mu      sync.Mutex
	buckets map[string]gcpbootstrap.BucketMetadata
}

func newIntegrationBuckets() *integrationBuckets {
	return &integrationBuckets{buckets: map[string]gcpbootstrap.BucketMetadata{}}
}

func (m *integrationBuckets) BucketExists(_ context.Context, name string) (bool, gcpbootstrap.BucketMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	meta, exists := m.buckets[name]
	meta.Labels = maps.Clone(meta.Labels)
	return exists, meta, nil
}

func (m *integrationBuckets) CreateBucket(_ context.Context, name, _, location string, labels map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buckets[name] = gcpbootstrap.BucketMetadata{Location: location, Labels: maps.Clone(labels)}
	return nil
}

func (m *integrationBuckets) EnsureVersioning(context.Context, string) error { return nil }

func (m *integrationBuckets) EnsureSoftDelete(context.Context, string, time.Duration) error {
	return nil
}

// integrationWIF is a native WIFAPI fake with in-memory owned resources for identity Ensure.
type integrationWIF struct {
	projectNumber string
	pools         map[string]gcpbootstrap.WIFPool
	providers     map[string]gcpbootstrap.WIFProvider
	accounts      map[string]gcpbootstrap.WIFServiceAccount
	policies      map[string]*iam.Policy
}

func newIntegrationWIF(projectNumber string) *integrationWIF {
	return &integrationWIF{
		projectNumber: projectNumber,
		pools:         map[string]gcpbootstrap.WIFPool{},
		providers:     map[string]gcpbootstrap.WIFProvider{},
		accounts:      map[string]gcpbootstrap.WIFServiceAccount{},
		policies:      map[string]*iam.Policy{},
	}
}

func (f *integrationWIF) ProjectNumber(context.Context, string) (string, error) {
	return f.projectNumber, nil
}

func (f *integrationWIF) GetPool(_ context.Context, name string) (gcpbootstrap.WIFPool, error) {
	pool, ok := f.pools[name]
	if !ok {
		return gcpbootstrap.WIFPool{}, gcpbootstrap.ErrWIFNotFound
	}
	return pool, nil
}

func (f *integrationWIF) CreatePool(_ context.Context, parent, poolID string, pool gcpbootstrap.WIFPool) (gcpbootstrap.WIFPool, error) {
	name := parent + "/workloadIdentityPools/" + poolID
	created := gcpbootstrap.WIFPool{Name: name, DisplayName: pool.DisplayName, Description: pool.Description}
	f.pools[name] = created
	return created, nil
}

func (f *integrationWIF) GetProvider(_ context.Context, name string) (gcpbootstrap.WIFProvider, error) {
	provider, ok := f.providers[name]
	if !ok {
		return gcpbootstrap.WIFProvider{}, gcpbootstrap.ErrWIFNotFound
	}
	return provider, nil
}

func (f *integrationWIF) CreateProvider(_ context.Context, parent, providerID string, provider gcpbootstrap.WIFProvider) (gcpbootstrap.WIFProvider, error) {
	name := parent + "/providers/" + providerID
	created := provider
	created.Name = name
	f.providers[name] = created
	return created, nil
}

func (f *integrationWIF) GetServiceAccount(_ context.Context, name string) (gcpbootstrap.WIFServiceAccount, error) {
	sa, ok := f.accounts[name]
	if !ok {
		return gcpbootstrap.WIFServiceAccount{}, gcpbootstrap.ErrWIFNotFound
	}
	return sa, nil
}

func (f *integrationWIF) CreateServiceAccount(_ context.Context, projectID, accountID, _, description string) (gcpbootstrap.WIFServiceAccount, error) {
	email := accountID + "@" + projectID + ".iam.gserviceaccount.com"
	name := "projects/" + projectID + "/serviceAccounts/" + email
	sa := gcpbootstrap.WIFServiceAccount{Name: name, Email: email, Description: description}
	f.accounts[name] = sa
	return sa, nil
}

func (f *integrationWIF) GetServiceAccountIAMPolicy(_ context.Context, resource string) (*iam.Policy, error) {
	policy := f.policies[resource]
	if policy == nil {
		return &iam.Policy{Version: 1, Etag: "wif-initial"}, nil
	}
	return cloneIntegrationIAMPolicy(policy), nil
}

func (f *integrationWIF) SetServiceAccountIAMPolicy(_ context.Context, resource string, policy *iam.Policy) error {
	f.policies[resource] = cloneIntegrationIAMPolicy(policy)
	return nil
}

func cloneIntegrationIAMPolicy(policy *iam.Policy) *iam.Policy {
	if policy == nil {
		return nil
	}
	out := *policy
	if len(policy.Bindings) > 0 {
		out.Bindings = make([]*iam.Binding, len(policy.Bindings))
		for i, binding := range policy.Bindings {
			if binding == nil {
				continue
			}
			copied := *binding
			copied.Members = append([]string(nil), binding.Members...)
			out.Bindings[i] = &copied
		}
	}
	return &out
}
