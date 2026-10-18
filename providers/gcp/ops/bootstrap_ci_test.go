package ops

import (
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
)

func TestBootstrapRefusesCIApplyWithoutRepositoryBeforePreparation(t *testing.T) {
	b := Bootstrap{NewPreparation: func(context.Context) (AccountPreparation, error) {
		t.Fatal("prepared cloud resources without a repository")
		return nil, nil
	}}
	_, err := b.Ensure(context.Background(), gcpTestSpec(), platform.BootstrapRequest{ApplyCIPermissions: true})
	if err == nil || !strings.Contains(err.Error(), "--github-owner") {
		t.Fatalf("missing repository accepted: %v", err)
	}
}

func TestCIVariablesDeriveProducerIdentityWithoutDeploymentLogin(t *testing.T) {
	spec := gcpTestSpec()
	prep, err := gcpbootstrap.BuildPreparationPlan(gcpbootstrap.Spec{Project: spec.Identity.Project, Environment: spec.Identity.Environment, GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region})
	if err != nil {
		t.Fatal(err)
	}
	var identities []gcpbootstrap.IdentityPlan
	for _, role := range []string{"deploy", "build"} {
		plan, err := gcpbootstrap.BuildIdentityPlan(gcpbootstrap.IdentitySpec{Project: spec.Identity.Project, Environment: spec.Identity.Environment, GCPProject: spec.Identity.GCPProject, GitHubOwner: "agency", GitHubRepo: "shop", GitHubRepositoryID: 42, GitHubOwnerID: 7, Role: role})
		if err != nil {
			t.Fatal(err)
		}
		identities = append(identities, plan)
	}
	provider := "projects/123/locations/global/workloadIdentityPools/" + identities[0].PoolID + "/providers/github"
	inputs, err := ciGrantInputs(prep, identities, provider)
	if err != nil {
		t.Fatal(err)
	}
	if inputs.ProjectNumber != "123" {
		t.Fatal("verified project number lost")
	}
	if _, err := ciGrantInputs(prep, identities, strings.Replace(provider, identities[0].PoolID, "foreign-pool", 1)); err == nil {
		t.Fatal("foreign federation reference accepted")
	}
	vars := ciVariables(prep, identities, provider)
	if vars["MAGELIFT_IMAGE_SIGNING_IDENTITY"] != "https://github.com/agency/shop/.github/workflows/magelift.yml@refs/heads/main" || vars["MAGELIFT_IMAGE_SIGNING_ISSUER"] != "https://token.actions.githubusercontent.com" {
		t.Fatal("producer policy was derived from deployment credentials")
	}
	if vars["MAGELIFT_GCP_BUILD_SERVICE_ACCOUNT"] == identities[0].ServiceAccountEmail || vars["MAGELIFT_RELEASE_IMAGE"] != prep.ImageBase {
		t.Fatal("build identity or destination lost")
	}
}
