package ops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/githubci"
	"github.com/magelift/magelift/internal/platform"
	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
)

func TestBootstrapResolvesNumericGitHubIdentityBeforeCloudMutation(t *testing.T) {
	refused := errors.New("repository metadata access refused")
	b := Bootstrap{
		ResolveGitHubRepository: func(context.Context, string, string) (githubci.Repository, error) {
			return githubci.Repository{}, refused
		},
		NewEnsurer: func(context.Context) (*gcpbootstrap.Bootstrapper, error) {
			t.Fatal("cloud preparation preceded repository validation")
			return nil, nil
		},
	}
	_, err := b.Ensure(context.Background(), gcpTestSpec(), platform.BootstrapRequest{GitHubOwner: "agency", GitHubRepo: "shop"})
	if !errors.Is(err, refused) {
		t.Fatalf("repository failure lost: %v", err)
	}
}

func TestBootstrapPlansSeparateIdentitiesPinnedToRepositoryIDs(t *testing.T) {
	calls := 0
	b := Bootstrap{ResolveGitHubRepository: func(_ context.Context, owner, repo string) (githubci.Repository, error) {
		calls++
		if owner != "agency" || repo != "shop" {
			t.Fatalf("wrong repository %q/%q", owner, repo)
		}
		return githubci.Repository{OwnerLogin: "Agency", Name: "Shop", ID: 42, OwnerID: 7}, nil
	}}
	plans, err := b.planGitHubIdentities(context.Background(), gcpTestSpec(), platform.BootstrapRequest{GitHubOwner: "agency", GitHubRepo: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || calls != 1 || plans[0].ServiceAccountEmail == plans[1].ServiceAccountEmail {
		t.Fatalf("identities=%+v calls=%d", plans, calls)
	}
	for _, plan := range plans {
		if !strings.Contains(plan.AttributeCondition, "assertion.repository_id == '42'") || !strings.Contains(plan.AttributeCondition, "assertion.repository_owner_id == '7'") || plan.Repository != "Agency/Shop" {
			t.Fatalf("unbound trust: %+v", plan)
		}
	}
}

func TestBootstrapLaptopDoesNotRequireGitHubLogin(t *testing.T) {
	b := Bootstrap{ResolveGitHubRepository: func(context.Context, string, string) (githubci.Repository, error) {
		t.Fatal("unexpected GitHub lookup")
		return githubci.Repository{}, nil
	}}
	plans, err := b.planGitHubIdentities(context.Background(), gcpTestSpec(), platform.BootstrapRequest{})
	if err != nil || len(plans) != 0 {
		t.Fatalf("plans=%+v error=%v", plans, err)
	}
}
