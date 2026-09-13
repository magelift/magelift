package cli

import (
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

type plannedStateBackend struct {
	stubPlanned
	url string
}

func (p plannedStateBackend) StateBackendURL() string { return p.url }

func TestInfrastructureBackendURLPrefersEnvThenPlanned(t *testing.T) {
	planned := plannedStateBackend{url: "gs://magelift-example-europe-west1-shop-preview-state"}
	t.Run("env wins", func(t *testing.T) {
		o := testOptions(nil, &fakeTerminal{})
		o.getenv = func(key string) string {
			if key == "PULUMI_BACKEND_URL" {
				return "s3://override-state"
			}
			return ""
		}
		if got := o.infrastructureBackendURL(planned); got != "s3://override-state" {
			t.Fatalf("backend = %q", got)
		}
	})
	t.Run("planned when env empty", func(t *testing.T) {
		o := testOptions(nil, &fakeTerminal{})
		if got := o.infrastructureBackendURL(planned); got != planned.url {
			t.Fatalf("backend = %q", got)
		}
	})
	t.Run("empty without planned url", func(t *testing.T) {
		o := testOptions(nil, &fakeTerminal{})
		var planned platform.PlannedStack = stubPlanned{provider: "aws", runtime: "ecs-fargate"}
		if got := o.infrastructureBackendURL(planned); got != "" {
			t.Fatalf("backend = %q", got)
		}
	})
}

func TestBootstrapGitHubIdentityIsOptional(t *testing.T) {
	owner, repo, requested, err := platform.BootstrapRequest{}.GitHubIdentity()
	if err != nil || requested || owner != "" || repo != "" {
		t.Fatalf("empty = %q %q %v %v", owner, repo, requested, err)
	}
	_, _, _, err = (platform.BootstrapRequest{GitHubOwner: "acourtiol"}).GitHubIdentity()
	if err == nil {
		t.Fatal("partial GitHub request must fail")
	}
	owner, repo, requested, err = (platform.BootstrapRequest{GitHubOwner: "acourtiol", GitHubRepo: "shop"}).GitHubIdentity()
	if err != nil || !requested || owner != "acourtiol" || repo != "shop" {
		t.Fatalf("full = %q %q %v %v", owner, repo, requested, err)
	}
}
