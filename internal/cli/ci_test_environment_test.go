package cli

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"go.yaml.in/yaml/v4"
)

func TestSingleTestWorkflowBuildsAndDeploysWithoutProductionInputs(t *testing.T) {
	for _, provider := range []string{"gcp", "aws"} {
		t.Run(provider, func(t *testing.T) {
			var generator ciWorkflowGenerator = awsCIWorkflowGenerator{}
			cfg := config.Config{}
			cfg.Target.Provider = provider
			if provider == "gcp" {
				generator = gcpCIWorkflowGenerator{}
				cfg.Target.GCP = &config.GCPTarget{Project: "example-gcp-project", Region: "us-central1"}
			}
			rendered := renderWorkflow("magelift.yaml", "v1.2.3", []string{"test"}, generator, cfg, provider)
			var workflow struct {
				On   map[string]any `yaml:"on"`
				Jobs map[string]struct {
					Environment string
					If          string
					Concurrency struct {
						CancelInProgress bool `yaml:"cancel-in-progress"`
						Group            string
					}
					Permissions map[string]string
					Steps       []struct {
						Name string
						Run  string
					}
				}
			}
			if err := yaml.Unmarshal(rendered, &workflow); err != nil {
				t.Fatal(err)
			}
			if len(workflow.Jobs) != 3 {
				t.Fatalf("jobs: %v", workflow.Jobs)
			}
			for _, job := range []string{"validate", "build", "test"} {
				if _, ok := workflow.Jobs[job]; !ok {
					t.Fatalf("missing %s", job)
				}
			}
			if _, ok := workflow.On["schedule"]; ok {
				t.Fatal("test-only flow has a preview schedule")
			}
			if strings.Contains(string(rendered), "release_digest:") || strings.Contains(string(rendered), "artifact_run_id:") || strings.Contains(string(rendered), "inputs.") {
				t.Fatal("single test dispatch needs no production/environment inputs")
			}
			deploy := workflow.Jobs["test"]
			if deploy.Environment != "test" || deploy.Concurrency.Group == "" || deploy.Concurrency.CancelInProgress {
				t.Fatalf("test deployment is not serialized: %+v", deploy)
			}
			if strings.Contains(deploy.If, "pull_request") || !strings.Contains(deploy.If, "needs.build.result == 'success'") {
				t.Fatalf("unsafe deployment condition: %s", deploy.If)
			}
			if workflow.Jobs["validate"].Permissions["id-token"] != "" {
				t.Fatal("PR validation has cloud privilege")
			}
			foundHandoff, foundDeploy := false, false
			for _, step := range deploy.Steps {
				foundHandoff = foundHandoff || step.Name == "Verify artifact handoff"
				foundDeploy = foundDeploy || strings.Contains(step.Run, "--env test --no-interaction --yes deploy --digest")
			}
			if !foundHandoff || !foundDeploy {
				t.Fatal("test job must deploy the verified existing build")
			}
			if strings.Contains(string(rendered), "MAGELIFT_TEST_ENV") {
				t.Fatal("literal test environment needs no duplicated variable")
			}
			if provider == "gcp" {
				if workflow.Jobs["build"].Environment != "build" || !strings.Contains(string(rendered), "MAGELIFT_GCP_TEST_SERVICE_ACCOUNT") {
					t.Fatal("GCP build and test identities need distinct environment references")
				}
			}
		})
	}
}
