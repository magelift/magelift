package bootstrap

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/googleapi"
)

func testNodeIdentityPreparer(t *testing.T, wif WIFAPI, repoIAM *fakeRepoIAM, projectIAM *fakeProjectIAM) *NodeIdentityPreparer {
	if wif == nil {
		wif = newFakeWIF("249624521501")
	}
	if repoIAM == nil {
		repoIAM = newFakeRepoIAM()
	}
	if projectIAM == nil {
		projectIAM = newFakeProjectIAM()
	}
	prep, err := NewNodeIdentityPreparer(wif, &fakeGrantRegistry{}, repoIAM, projectIAM)
	if err != nil {
		t.Fatal(err)
	}
	return prep
}

func TestNodeIdentityEnsureCreatesOwnedServiceAccountAndGrants(t *testing.T) {
	t.Parallel()
	wif := newFakeWIF("249624521501")
	repoIAM := newFakeRepoIAM()
	projectIAM := newFakeProjectIAM()
	prep := testNodeIdentityPreparer(t, wif, repoIAM, projectIAM)
	spec := testSpec()
	plan, err := BuildPreparationPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	result, err := prep.Ensure(context.Background(), plan, spec)
	if err != nil {
		t.Fatal(err)
	}
	wantPlan, err := BuildNodeIdentityPlan(spec)
	if err != nil {
		t.Fatal(err)
	}
	if result.ServiceAccountEmail != wantPlan.ServiceAccountEmail || result.ServiceAccountName != wantPlan.ServiceAccountName {
		t.Fatalf("result=%#v want=%#v", result, wantPlan)
	}
	if wif.createSACalls != 1 {
		t.Fatalf("createSACalls=%d", wif.createSACalls)
	}
	if repoIAM.sets != 1 || projectIAM.sets != 1 {
		t.Fatalf("repo sets=%d project sets=%d", repoIAM.sets, projectIAM.sets)
	}
	member := "serviceAccount:" + result.ServiceAccountEmail
	registryReader, runtimeRole := false, false
	for _, binding := range repoIAM.policy.Bindings {
		for _, got := range binding.Members {
			if got == member && binding.Role != roleArtifactRegistryReader {
				t.Fatalf("node received registry privilege %q", binding.Role)
			}
			registryReader = registryReader || got == member
		}
	}
	for _, binding := range projectIAM.policy.Bindings {
		for _, got := range binding.Members {
			if got == member && binding.Role != roleContainerDefaultNodeSA {
				t.Fatalf("node received project privilege %q", binding.Role)
			}
			runtimeRole = runtimeRole || got == member
		}
	}
	if !registryReader || !runtimeRole {
		t.Fatal("owned node runtime grants missing")
	}
}

func TestNodeIdentityEnsureRerunDoesNotGrantAgain(t *testing.T) {
	t.Parallel()
	wif := newFakeWIF("249624521501")
	repoIAM := newFakeRepoIAM()
	projectIAM := newFakeProjectIAM()
	prep := testNodeIdentityPreparer(t, wif, repoIAM, projectIAM)
	spec := testSpec()
	plan, _ := BuildPreparationPlan(spec)
	if _, err := prep.Ensure(context.Background(), plan, spec); err != nil {
		t.Fatal(err)
	}
	firstRepo, firstProject := repoIAM.sets, projectIAM.sets
	if _, err := prep.Ensure(context.Background(), plan, spec); err != nil {
		t.Fatal(err)
	}
	if wif.createSACalls != 1 {
		t.Fatalf("createSACalls=%d", wif.createSACalls)
	}
	if repoIAM.sets != firstRepo || projectIAM.sets != firstProject {
		t.Fatalf("extra IAM writes: repo %d->%d project %d->%d", firstRepo, repoIAM.sets, firstProject, projectIAM.sets)
	}
}

func TestNodeIdentityForeignServiceAccountRejectedBeforeIAM(t *testing.T) {
	t.Parallel()
	wif := newFakeWIF("249624521501")
	repoIAM := newFakeRepoIAM()
	projectIAM := newFakeProjectIAM()
	prep := testNodeIdentityPreparer(t, wif, repoIAM, projectIAM)
	spec := testSpec()
	plan, _ := BuildPreparationPlan(spec)
	want, _ := BuildNodeIdentityPlan(spec)
	wif.accounts[want.ServiceAccountName] = WIFServiceAccount{Name: want.ServiceAccountName, Email: want.ServiceAccountEmail, Description: "not-owned"}
	_, err := prep.Ensure(context.Background(), plan, spec)
	if err == nil || !strings.Contains(err.Error(), "ownership marker") {
		t.Fatalf("expected ownership refusal, got %v", err)
	}
	if repoIAM.sets != 0 || projectIAM.sets != 0 {
		t.Fatal("IAM mutated after foreign node rejection")
	}
}

func TestNodeIdentityPreparationScopeMismatchRejected(t *testing.T) {
	t.Parallel()
	prep := testNodeIdentityPreparer(t, nil, nil, nil)
	spec := testSpec()
	plan, _ := BuildPreparationPlan(spec)
	plan.RegistryID = "foreign"
	_, err := prep.Ensure(context.Background(), plan, spec)
	if err == nil || !strings.Contains(err.Error(), "preparation plan does not match spec") {
		t.Fatalf("expected scope mismatch, got %v", err)
	}
}

func TestNodeIdentityProjectIAMCASPreservesConditionalBinding(t *testing.T) {
	t.Parallel()
	wif := newFakeWIF("249624521501")
	repoIAM := newFakeRepoIAM()
	projectIAM := newFakeProjectIAM()
	projectIAM.policy.Bindings = []*cloudresourcemanager.Binding{{
		Role:      roleContainerDefaultNodeSA,
		Members:   []string{"user:admin@example.com"},
		Condition: &cloudresourcemanager.Expr{Title: "keep", Expression: "request.time < timestamp('2099-01-01T00:00:00Z')"},
	}}
	prep := testNodeIdentityPreparer(t, wif, repoIAM, projectIAM)
	spec := testSpec()
	plan, _ := BuildPreparationPlan(spec)
	if _, err := prep.Ensure(context.Background(), plan, spec); err != nil {
		t.Fatal(err)
	}
	if len(projectIAM.policy.Bindings) < 2 || projectIAM.policy.Bindings[0].Condition == nil {
		t.Fatalf("conditional binding lost: %#v", projectIAM.policy.Bindings)
	}
}

func TestNodeIdentityRepositoryCASRetry(t *testing.T) {
	t.Parallel()
	wif := newFakeWIF("249624521501")
	repoIAM := newFakeRepoIAM()
	repoIAM.conflictOnce = true
	projectIAM := newFakeProjectIAM()
	prep := testNodeIdentityPreparer(t, wif, repoIAM, projectIAM)
	spec := testSpec()
	plan, _ := BuildPreparationPlan(spec)
	if _, err := prep.Ensure(context.Background(), plan, spec); err != nil {
		t.Fatal(err)
	}
	if repoIAM.sets < 2 {
		t.Fatalf("expected CAS retry, sets=%d", repoIAM.sets)
	}
}

type conflictCreateWIF struct {
	*fakeWIF
}

func (c *conflictCreateWIF) CreateServiceAccount(_ context.Context, projectID, accountID, _, description string) (WIFServiceAccount, error) {
	email := accountID + "@" + projectID + ".iam.gserviceaccount.com"
	name := "projects/" + projectID + "/serviceAccounts/" + email
	sa := WIFServiceAccount{Name: name, Email: email, Description: description}
	c.accounts[name] = sa
	return WIFServiceAccount{}, &googleapi.Error{Code: 409, Message: "already exists"}
}

func TestNodeIdentityCreateConflictRefetchesOwnedAccount(t *testing.T) {
	t.Parallel()
	base := newFakeWIF("249624521501")
	wif := &conflictCreateWIF{fakeWIF: base}
	repoIAM := newFakeRepoIAM()
	projectIAM := newFakeProjectIAM()
	prep := testNodeIdentityPreparer(t, wif, repoIAM, projectIAM)
	spec := testSpec()
	plan, _ := BuildPreparationPlan(spec)
	want, _ := BuildNodeIdentityPlan(spec)
	result, err := prep.Ensure(context.Background(), plan, spec)
	if err != nil {
		t.Fatal(err)
	}
	if result.ServiceAccountEmail != want.ServiceAccountEmail {
		t.Fatalf("result=%#v", result)
	}
}

func TestNodeIdentityRefusesForeignReturnedResourceBeforeIAM(t *testing.T) {
	wif := newFakeWIF("123456789012")
	repo := newFakeRepoIAM()
	project := newFakeProjectIAM()
	preparer := testNodeIdentityPreparer(t, wif, repo, project)
	spec := testSpec()
	prep, _ := BuildPreparationPlan(spec)
	plan, _ := BuildNodeIdentityPlan(spec)
	wif.accounts[plan.ServiceAccountName] = WIFServiceAccount{Name: "projects/foreign-project/serviceAccounts/foreign", Email: plan.ServiceAccountEmail, Description: plan.ServiceAccountMarker}
	_, err := preparer.Ensure(context.Background(), prep, spec)
	if err == nil || repo.sets != 0 || project.sets != 0 {
		t.Fatalf("foreign node reference accepted: %v", err)
	}
}

func TestNodeIAMForbiddenNamesAdministratorPrerequisite(t *testing.T) {
	cause := &googleapi.Error{Code: 403, Message: "permission denied"}
	err := nodeIAMPreparationError(cause, "prepare node role", "resourcemanager.projects.setIamPolicy")
	if !strings.Contains(err.Error(), "administrator") || !strings.Contains(err.Error(), "resourcemanager.projects.setIamPolicy") || !strings.Contains(err.Error(), "rerun magelift bootstrap") {
		t.Fatalf("missing administrative next action: %v", err)
	}
}
