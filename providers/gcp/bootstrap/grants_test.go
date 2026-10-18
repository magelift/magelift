package bootstrap

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magelift/magelift/providers/gcp/naming"

	artifactregistry "google.golang.org/api/artifactregistry/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iam/v1"
	secretmanager "google.golang.org/api/secretmanager/v1"
	storagev1 "google.golang.org/api/storage/v1"
)

func testCIGrantInputs(t *testing.T) CIGrantInputs {
	spec := testSpec()
	prep, err := BuildPreparationPlan(spec)
	if err != nil {
		t.Fatalf("BuildPreparationPlan: %v", err)
	}
	deploy, err := BuildIdentityPlan(IdentitySpec{
		GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: spec.Project, Environment: spec.Environment, GCPProject: spec.GCPProject,
		GitHubOwner: "magelift", GitHubRepo: "magelift",
	})
	if err != nil {
		t.Fatalf("BuildIdentityPlan deploy: %v", err)
	}
	build, err := BuildIdentityPlan(IdentitySpec{
		GitHubRepositoryID: 42, GitHubOwnerID: 7,
		Project: spec.Project, Environment: spec.Environment, GCPProject: spec.GCPProject,
		GitHubOwner: "magelift", GitHubRepo: "magelift", Role: identityRoleBuild,
	})
	if err != nil {
		t.Fatalf("BuildIdentityPlan build: %v", err)
	}
	return CIGrantInputs{
		Preparation:   prep,
		Deploy:        deploy,
		Build:         build,
		ProjectNumber: "249624521501",
	}
}

func TestBuildCIGrantPlanExcludesBuildFromProjectAndSecret(t *testing.T) {
	t.Parallel()
	inputs := testCIGrantInputs(t)
	plan, err := BuildCIGrantPlan(inputs)
	if err != nil {
		t.Fatal(err)
	}
	buildMember := "serviceAccount:" + inputs.Build.ServiceAccountEmail
	deployMember := "serviceAccount:" + inputs.Deploy.ServiceAccountEmail
	for _, binding := range plan.Bindings {
		if binding.Member == buildMember {
			if binding.Scope == grantScopeProject {
				t.Fatalf("build identity must not receive project grants: %#v", binding)
			}
			if binding.Role == roleSecretAccessor {
				t.Fatalf("build identity must not read state passphrase: %#v", binding)
			}
			if binding.Role != roleArtifactRegistryWriter {
				t.Fatalf("unexpected build binding: %#v", binding)
			}
		}
		if binding.Member == deployMember && binding.Role == roleArtifactRegistryWriter {
			t.Fatalf("deploy must not be registry writer: %#v", binding)
		}
	}
}

func TestBuildCIGrantPlanUsesOwnedNodeServiceAccount(t *testing.T) {
	t.Parallel()
	inputs := testCIGrantInputs(t)
	plan, err := BuildCIGrantPlan(inputs)
	if err != nil {
		t.Fatal(err)
	}
	want := naming.NodeServiceAccountEmail(inputs.Preparation.Base.GCPProject, inputs.Deploy.Project, inputs.Deploy.Environment)
	if plan.NodeServiceAccount != want {
		t.Fatalf("node service account = %q, want %q", plan.NodeServiceAccount, want)
	}
}

func TestBuildCIGrantPlanNodeReaderNotWriter(t *testing.T) {
	t.Parallel()
	plan, err := BuildCIGrantPlan(testCIGrantInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	nodeMember := "serviceAccount:" + plan.NodeServiceAccount
	for _, binding := range plan.Bindings {
		if binding.Member != nodeMember {
			continue
		}
		if binding.Role == roleArtifactRegistryWriter {
			t.Fatalf("node SA must not be registry writer: %#v", binding)
		}
	}
}

func TestCIGrantScopeMismatchFailsBeforeMutation(t *testing.T) {
	t.Parallel()
	inputs := testCIGrantInputs(t)
	inputs.Build.GCPProject = "other-project"
	grantPrep, err := NewCIGrantPreparer(
		&fakeGrantRegistry{}, &fakeGrantSecrets{}, &fakeGrantBuckets{},
		newFakeWIF("249624521501"),
		newFakeRepoIAM(), newFakeSecretIAM(), newFakeBucketIAM(), newFakeProjectIAM(),
	)
	if err != nil {
		t.Fatal(err)
	}
	err = grantPrep.CheckOwnedResources(context.Background(), inputs)
	if err == nil || !strings.Contains(err.Error(), "GCP project") {
		t.Fatalf("expected project mismatch error, got %v", err)
	}
}

type fakeGrantRegistry struct{}

func (f *fakeGrantRegistry) GetRepository(context.Context, string, string, string) (RepositoryMetadata, error) {
	return RepositoryMetadata{Format: "DOCKER", Location: "europe-west1", Labels: preparationLabels(testSpec(), purposeApplicationImages)}, nil
}
func (f *fakeGrantRegistry) CreateRepository(context.Context, string, string, string, map[string]string) (RepositoryMetadata, error) {
	return RepositoryMetadata{}, errors.New("unexpected create")
}
func (f *fakeGrantRegistry) Close() error { return nil }

type fakeGrantSecrets struct{}

func (f *fakeGrantSecrets) GetSecret(context.Context, string, string) (SecretMetadata, error) {
	return SecretMetadata{Labels: preparationLabels(testSpec(), purposePulumiStatePassphrase)}, nil
}
func (f *fakeGrantSecrets) CreateSecret(context.Context, string, string, map[string]string) error {
	return errors.New("unexpected create")
}
func (f *fakeGrantSecrets) GetVersion(context.Context, string, string, string) (SecretVersionMetadata, error) {
	return SecretVersionMetadata{}, nil
}
func (f *fakeGrantSecrets) ListVersions(context.Context, string, string) ([]SecretVersionMetadata, error) {
	return nil, nil
}
func (f *fakeGrantSecrets) AddVersion(context.Context, string, string, []byte) error {
	return errors.New("unexpected add")
}
func (f *fakeGrantSecrets) Close() error { return nil }

type fakeGrantBuckets struct{}

func (f *fakeGrantBuckets) BucketExists(context.Context, string) (bool, BucketMetadata, error) {
	plan, _ := BuildPlan(testSpec())
	return true, BucketMetadata{Location: testSpec().Region, Labels: plan.Labels}, nil
}
func (f *fakeGrantBuckets) CreateBucket(context.Context, string, string, string, map[string]string) error {
	return errors.New("unexpected create")
}
func (f *fakeGrantBuckets) EnsureVersioning(context.Context, string) error                { return nil }
func (f *fakeGrantBuckets) EnsureSoftDelete(context.Context, string, time.Duration) error { return nil }

type fakeRepoIAM struct {
	mu           sync.Mutex
	policy       *artifactregistry.Policy
	sets         int
	conflictOnce bool
}

func newFakeRepoIAM() *fakeRepoIAM {
	return &fakeRepoIAM{policy: &artifactregistry.Policy{Version: 3, Etag: "v1"}}
}

func (f *fakeRepoIAM) GetRepositoryIAMPolicy(context.Context, string) (*artifactregistry.Policy, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneARPolicy(f.policy), nil
}

func (f *fakeRepoIAM) SetRepositoryIAMPolicy(_ context.Context, _ string, policy *artifactregistry.Policy) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets++
	if f.conflictOnce {
		f.conflictOnce = false
		return &googleapi.Error{Code: 409, Message: "concurrent policy change"}
	}
	f.policy = cloneARPolicy(policy)
	return nil
}

func (f *fakeRepoIAM) Close() error { return nil }

type fakeSecretIAM struct {
	policy *secretmanager.Policy
	sets   int
}

func newFakeSecretIAM() *fakeSecretIAM {
	return &fakeSecretIAM{policy: &secretmanager.Policy{Version: 3, Etag: "s1"}}
}

func (f *fakeSecretIAM) GetSecretIAMPolicy(context.Context, string) (*secretmanager.Policy, error) {
	return cloneSecretPolicy(f.policy), nil
}

func (f *fakeSecretIAM) SetSecretIAMPolicy(_ context.Context, _ string, policy *secretmanager.Policy) error {
	f.sets++
	f.policy = cloneSecretPolicy(policy)
	return nil
}

func (f *fakeSecretIAM) Close() error { return nil }

type fakeBucketIAM struct {
	policy *storagev1.Policy
	sets   int
}

func newFakeBucketIAM() *fakeBucketIAM {
	return &fakeBucketIAM{policy: &storagev1.Policy{
		Version: 3,
		Etag:    "b1",
		Bindings: []*storagev1.PolicyBindings{{
			Role:      "roles/storage.objectViewer",
			Members:   []string{"user:admin@example.com"},
			Condition: &storagev1.Expr{Title: "keep", Expression: "request.time < timestamp('2099-01-01T00:00:00Z')"},
		}},
	}}
}

func (f *fakeBucketIAM) GetBucketIAMPolicy(context.Context, string) (*storagev1.Policy, error) {
	return cloneStoragePolicy(f.policy), nil
}

func (f *fakeBucketIAM) SetBucketIAMPolicy(_ context.Context, _ string, policy *storagev1.Policy) error {
	f.sets++
	f.policy = cloneStoragePolicy(policy)
	return nil
}

func (f *fakeBucketIAM) Close() error { return nil }

type fakeProjectIAM struct {
	policy *cloudresourcemanager.Policy
	sets   int
}

func newFakeProjectIAM() *fakeProjectIAM {
	return &fakeProjectIAM{policy: &cloudresourcemanager.Policy{Version: 3, Etag: "p1"}}
}

func (f *fakeProjectIAM) GetProjectIAMPolicy(context.Context, string) (*cloudresourcemanager.Policy, error) {
	return cloneCRMPolicy(f.policy), nil
}

func (f *fakeProjectIAM) SetProjectIAMPolicy(_ context.Context, _ string, policy *cloudresourcemanager.Policy) error {
	f.sets++
	f.policy = cloneCRMPolicy(policy)
	return nil
}

func (f *fakeProjectIAM) Close() error { return nil }

func seedOwnedNodeServiceAccount(wif *fakeWIF, spec Spec) {
	plan, err := BuildNodeIdentityPlan(spec)
	if err != nil {
		panic(err)
	}
	wif.accounts[plan.ServiceAccountName] = WIFServiceAccount{
		Name:        plan.ServiceAccountName,
		Email:       plan.ServiceAccountEmail,
		Description: plan.ServiceAccountMarker,
	}
}

func testGrantPreparer(t *testing.T, wif *fakeWIF) (*CIGrantPreparer, *fakeRepoIAM, *fakeBucketIAM, *fakeProjectIAM) {
	if wif == nil {
		wif = newFakeWIF("249624521501")
	}
	seedOwnedNodeServiceAccount(wif, testSpec())
	marker := wifServiceAccountOwnershipMarker(testSpec().Project, testSpec().Environment, identityRoleDeploy)
	buildMarker := wifServiceAccountOwnershipMarker(testSpec().Project, testSpec().Environment, identityRoleBuild)
	deployPlan, _ := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7, Project: testSpec().Project, Environment: testSpec().Environment, GCPProject: testSpec().GCPProject, GitHubOwner: "magelift", GitHubRepo: "magelift"})
	buildPlan, _ := BuildIdentityPlan(IdentitySpec{GitHubRepositoryID: 42, GitHubOwnerID: 7, Project: testSpec().Project, Environment: testSpec().Environment, GCPProject: testSpec().GCPProject, GitHubOwner: "magelift", GitHubRepo: "magelift", Role: identityRoleBuild})
	wif.accounts[deployPlan.ServiceAccountName] = WIFServiceAccount{Name: deployPlan.ServiceAccountName, Email: deployPlan.ServiceAccountEmail, Description: marker}
	wif.accounts[buildPlan.ServiceAccountName] = WIFServiceAccount{Name: buildPlan.ServiceAccountName, Email: buildPlan.ServiceAccountEmail, Description: buildMarker}
	repoIAM := newFakeRepoIAM()
	bucketIAM := newFakeBucketIAM()
	projectIAM := newFakeProjectIAM()
	prep, err := NewCIGrantPreparer(&fakeGrantRegistry{}, &fakeGrantSecrets{}, &fakeGrantBuckets{}, wif, repoIAM, newFakeSecretIAM(), bucketIAM, projectIAM)
	if err != nil {
		t.Fatal(err)
	}
	return prep, repoIAM, bucketIAM, projectIAM
}

func TestCIGrantEnsurePreservesConditionalBindingsAndAudit(t *testing.T) {
	t.Parallel()
	prep, _, bucketIAM, _ := testGrantPreparer(t, nil)
	if err := prep.Ensure(context.Background(), testCIGrantInputs(t)); err != nil {
		t.Fatal(err)
	}
	if bucketIAM.policy.Bindings[0].Condition == nil || bucketIAM.policy.Bindings[0].Members[0] != "user:admin@example.com" {
		t.Fatalf("conditional binding changed: %#v", bucketIAM.policy.Bindings[0])
	}
}

func TestCIGrantEnsureRerunDoesNotSetAgain(t *testing.T) {
	t.Parallel()
	prep, repoIAM, _, projectIAM := testGrantPreparer(t, nil)
	inputs := testCIGrantInputs(t)
	if err := prep.Ensure(context.Background(), inputs); err != nil {
		t.Fatal(err)
	}
	firstRepoSets := repoIAM.sets
	firstProjectSets := projectIAM.sets
	if err := prep.Ensure(context.Background(), inputs); err != nil {
		t.Fatal(err)
	}
	if repoIAM.sets != firstRepoSets || projectIAM.sets != firstProjectSets {
		t.Fatalf("expected no extra sets: repo %d->%d project %d->%d", firstRepoSets, repoIAM.sets, firstProjectSets, projectIAM.sets)
	}
}

func TestCIGrantRepositoryCASRetryPreservesConcurrentBinding(t *testing.T) {
	t.Parallel()
	repoIAM := newFakeRepoIAM()
	repoIAM.policy.Bindings = []*artifactregistry.Binding{{
		Role: roleArtifactRegistryReader, Members: []string{"user:concurrent@example.com"},
	}}
	repoIAM.conflictOnce = true
	wif := newFakeWIF("249624521501")
	prep, _, _, _ := testGrantPreparer(t, wif)
	prep.repoIAM = repoIAM
	if err := prep.Ensure(context.Background(), testCIGrantInputs(t)); err != nil {
		t.Fatal(err)
	}
	if repoIAM.sets < 2 {
		t.Fatalf("expected CAS retry, sets=%d", repoIAM.sets)
	}
	for _, binding := range repoIAM.policy.Bindings {
		if binding.Role == roleArtifactRegistryReader && containsString(binding.Members, "user:concurrent@example.com") {
			return
		}
	}
	t.Fatalf("concurrent binding lost: %#v", repoIAM.policy.Bindings)
}

func TestMergeUnconditionalARMemberPreservesConditionalBinding(t *testing.T) {
	t.Parallel()
	policy := &artifactregistry.Policy{
		Version: 3,
		Bindings: []*artifactregistry.Binding{{
			Role:      roleArtifactRegistryReader,
			Members:   []string{"user:admin@example.com"},
			Condition: &artifactregistry.Expr{Title: "time", Expression: "request.time < timestamp('2099-01-01T00:00:00Z')"},
		}},
	}
	updated, changed := mergeUnconditionalARMember(policy, roleArtifactRegistryReader, "serviceAccount:ci@example.iam.gserviceaccount.com")
	if !changed || len(updated.Bindings) != 2 {
		t.Fatalf("bindings=%#v changed=%v", updated.Bindings, changed)
	}
	if updated.Bindings[0].Condition == nil {
		t.Fatal("conditional binding was converted")
	}
}

func TestCIGrantPlanRejectsForgedResourceAndServiceAccountNames(t *testing.T) {
	for _, change := range []func(*CIGrantInputs){
		func(in *CIGrantInputs) { in.Preparation.RegistryID = "foreign-repository" },
		func(in *CIGrantInputs) { in.Preparation.StateSecretID = "foreign-key" },
		func(in *CIGrantInputs) {
			in.Deploy.ServiceAccountID = "foreign-account"
			in.Deploy.ServiceAccountEmail = "foreign-account@" + in.Deploy.GCPProject + ".iam.gserviceaccount.com"
		},
	} {
		inputs := testCIGrantInputs(t)
		change(&inputs)
		if _, err := BuildCIGrantPlan(inputs); err == nil {
			t.Fatal("accepted a forged resource ownership plan")
		}
	}
}

func TestCIGrantsVerifyProjectNumberBeforeAnyPolicyWrite(t *testing.T) {
	preparer, repo, _, project := testGrantPreparer(t, newFakeWIF("another-project-number"))
	err := preparer.Ensure(context.Background(), testCIGrantInputs(t))
	if err == nil || !strings.Contains(err.Error(), "project number differs") {
		t.Fatalf("wrong project was not refused: %v", err)
	}
	if repo.sets != 0 || project.sets != 0 {
		t.Fatal("IAM mutated before project-number verification")
	}
}

func TestCIGrantMissingPolicyEtagRefusesBlindOverwrite(t *testing.T) {
	preparer, repo, _, _ := testGrantPreparer(t, nil)
	repo.policy.Etag = ""
	if err := preparer.Ensure(context.Background(), testCIGrantInputs(t)); err == nil || !strings.Contains(err.Error(), "etag") {
		t.Fatalf("expected missing-etag refusal: %v", err)
	}
	if repo.sets != 0 {
		t.Fatal("policy written without etag")
	}
}

func TestCIGrantRejectsForeignNodeServiceAccountBeforeMutation(t *testing.T) {
	t.Parallel()
	wif := newFakeWIF("249624521501")
	preparer, repoIAM, _, projectIAM := testGrantPreparer(t, wif)
	plan, _ := BuildNodeIdentityPlan(testSpec())
	wif.accounts[plan.ServiceAccountName] = WIFServiceAccount{
		Name: plan.ServiceAccountName, Email: plan.ServiceAccountEmail, Description: "foreign ownership",
	}
	err := preparer.Ensure(context.Background(), testCIGrantInputs(t))
	if err == nil || !strings.Contains(err.Error(), "ownership marker") {
		t.Fatalf("expected foreign node refusal, got %v", err)
	}
	if repoIAM.sets != 0 || projectIAM.sets != 0 {
		t.Fatal("IAM mutated after foreign node rejection")
	}
}

func TestCIGrantsKeepNodeImpersonationOnExactNodeAccount(t *testing.T) {
	inputs := testCIGrantInputs(t)
	nodePlan, err := BuildNodeIdentityPlan(Spec{
		Project: inputs.Deploy.Project, Environment: inputs.Deploy.Environment, GCPProject: inputs.Preparation.Base.GCPProject,
	})
	if err != nil {
		t.Fatal(err)
	}
	node := nodePlan.ServiceAccountName
	wif := newFakeWIF(inputs.ProjectNumber)
	wif.policies[node] = &iam.Policy{Version: 3, Etag: "node-policy", Bindings: []*iam.Binding{{Role: roleNodeServiceAccountUser, Members: []string{"user:existing@example.invalid"}, Condition: &iam.Expr{Expression: "request.time < timestamp('2027-01-01T00:00:00Z')"}}}}
	preparer, _, _, _ := testGrantPreparer(t, wif)
	if err := preparer.Ensure(context.Background(), inputs); err != nil {
		t.Fatal(err)
	}
	policy := wif.policies[node]
	if len(policy.Bindings) != 2 || policy.Bindings[0].Condition == nil || policy.Etag != "node-policy" {
		t.Fatal("foreign conditional node policy was changed")
	}
	got := policy.Bindings[1]
	if got.Role != roleNodeServiceAccountUser || got.Condition != nil || len(got.Members) != 1 || got.Members[0] != "serviceAccount:"+inputs.Deploy.ServiceAccountEmail {
		t.Fatal("node impersonation was not scoped to deploy")
	}
}
