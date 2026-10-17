package ops

import (
	"context"

	"github.com/magelift/magelift/internal/githubci"
	"github.com/magelift/magelift/internal/platform"
	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
)

// Resolve immutable repository metadata before any cloud mutation.
func (b Bootstrap) planGitHubIdentities(ctx context.Context, spec gcpstack.Spec, req platform.BootstrapRequest) ([]gcpbootstrap.IdentityPlan, error) {
	owner, repo, wanted, err := req.GitHubIdentity()
	if err != nil || !wanted {
		return nil, err
	}
	resolve := b.ResolveGitHubRepository
	if resolve == nil {
		client, err := githubci.NewFromEnvironment(ctx)
		if err != nil {
			return nil, err
		}
		resolve = client.ResolveRepository
	}
	repository, err := resolve(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	plans := make([]gcpbootstrap.IdentityPlan, 0, 2)
	for _, role := range []string{"deploy", "build"} {
		plan, err := gcpbootstrap.BuildIdentityPlan(gcpbootstrap.IdentitySpec{
			Project: spec.Identity.Project, Environment: spec.Identity.Environment, GCPProject: spec.Identity.GCPProject,
			GitHubOwner: repository.OwnerLogin, GitHubRepo: repository.Name,
			GitHubRepositoryID: repository.ID, GitHubOwnerID: repository.OwnerID, Role: role,
		})
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}
