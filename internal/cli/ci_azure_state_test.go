package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"go.yaml.in/yaml/v4"
)

func TestAzureDeployJobsReceiveStateSettings(t *testing.T) {
	cfg := config.Config{}
	cfg.Target.Provider = "azure"
	rendered := renderWorkflow("magelift.php", "v1.2.3", []string{"test", "preview", "staging", "uat", "production"}, azureCIWorkflowGenerator{}, cfg, "azure/aks")
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string            `yaml:"name"`
				Uses string            `yaml:"uses"`
				Run  string            `yaml:"run"`
				Env  map[string]string `yaml:"env"`
				With map[string]string `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(rendered, &workflow); err != nil {
		t.Fatal(err)
	}
	var stateScript string
	for _, name := range []string{"test", "preview", "preview-destroy", "preview-sweep", "staging", "uat", "production"} {
		job, ok := workflow.Jobs[name]
		if !ok {
			t.Fatalf("missing job %s", name)
		}
		state, login, kubelogin := -1, -1, -1
		for i, step := range job.Steps {
			switch step.Name {
			case "Validate Azure state configuration":
				state = i
				stateScript = step.Run
				for key, value := range map[string]string{"AZURE_STORAGE_ACCOUNT": "${{ vars.MAGELIFT_AZURE_STORAGE_ACCOUNT }}", "AZURE_TENANT_ID": "${{ vars.MAGELIFT_AZURE_TENANT_ID }}", "PULUMI_CONFIG_PASSPHRASE": "${{ secrets.MAGELIFT_PULUMI_CONFIG_PASSPHRASE }}"} {
					if step.Env[key] != value {
						t.Fatalf("%s %s = %q", name, key, step.Env[key])
					}
				}
			case "Sign in to Azure with federated credentials":
				login = i
			case "Install kubelogin":
				kubelogin = i
				if step.Uses != strings.Fields(azureKubeloginAction)[0] || step.With["kubelogin-version"] != "v0.2.22" {
					t.Fatalf("kubelogin = %+v", step)
				}
			}
		}
		if state < 0 || login <= state || kubelogin <= login {
			t.Fatalf("%s state=%d login=%d kubelogin=%d", name, state, login, kubelogin)
		}
	}
	for _, name := range []string{"build", "validate"} {
		for _, step := range workflow.Jobs[name].Steps {
			if step.Name == "Validate Azure state configuration" || step.Name == "Install kubelogin" {
				t.Fatalf("%s receives deploy state setup", name)
			}
		}
	}
	dir := t.TempDir()
	envFile := filepath.Join(dir, "github-env")
	cmd := exec.Command("bash", "-c", stateScript)
	cmd.Env = append(os.Environ(), "GITHUB_ENV="+envFile, "AZURE_STORAGE_ACCOUNT=examplestate", "AZURE_TENANT_ID=tenant-example", "PULUMI_CONFIG_PASSPHRASE=first line\nsecond line")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("state setup: %v: %s", err, output)
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"AZURE_STORAGE_ACCOUNT=examplestate\n", "AZURE_TENANT_ID=tenant-example\n", "PULUMI_CONFIG_PASSPHRASE<<MAGELIFT_", "first line\nsecond line\n"} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("missing %q from GitHub environment", expected)
		}
	}
	bad := exec.Command("bash", "-c", stateScript)
	bad.Env = append(os.Environ(), "GITHUB_ENV="+filepath.Join(dir, "bad-env"), "AZURE_STORAGE_ACCOUNT=bad;account", "AZURE_TENANT_ID=tenant", "PULUMI_CONFIG_PASSPHRASE=secret")
	if err := bad.Run(); err == nil {
		t.Fatal("invalid account accepted")
	}
	for _, fields := range []struct{ tenant, passphrase string }{{"tenant\nOTHER=value", "secret"}, {"tenant", "   "}} {
		cmd := exec.Command("bash", "-c", stateScript)
		cmd.Env = append(os.Environ(), "GITHUB_ENV="+filepath.Join(dir, "rejected-env"), "AZURE_STORAGE_ACCOUNT=examplestate", "AZURE_TENANT_ID="+fields.tenant, "PULUMI_CONFIG_PASSPHRASE="+fields.passphrase)
		if err := cmd.Run(); err == nil {
			t.Fatal("invalid tenant or passphrase accepted")
		}
	}
}
