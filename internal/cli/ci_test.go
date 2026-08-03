package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCIGenerateIsDeterministicAndKeepsWorkflowOffStdout(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	workflowPath := filepath.Join(directory, ".github", "workflows", "magelift.yml")
	for run := range 2 {
		var stdout, stderr bytes.Buffer
		command := newCommand(&stdout, &stderr, nil)
		command.SetArgs([]string{"--config", configPath, "--output", "json", "ci", "generate", "--magelift-version", "v1.2.3"})
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		if stderr.Len() != 0 {
			t.Fatalf("diagnostics = %q", stderr.String())
		}
		wantChanged := `"changed": true`
		if run == 1 {
			wantChanged = `"changed": false`
		}
		if !strings.Contains(stdout.String(), wantChanged) || strings.Contains(stdout.String(), "actions/checkout") {
			t.Fatalf("stdout = %s", stdout.String())
		}
	}
	workflow, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{checkoutAction, setupGoAction, setupQemuAction, setupBuildxAction, cosignInstallerAction, "github.com/magelift/magelift/cmd/magelift@v1.2.3", "Build and sign immutable image", "environment: staging", "MAGELIFT_BUILD_ROLE_ARN", `- "staging"`, "--env \"${{ matrix.environment }}\"", "deploy --digest"} {
		if !strings.Contains(text, required) {
			t.Fatalf("workflow lacks %q:\n%s", required, text)
		}
	}
}

func TestCIValidateDetectsWorkflowAndConfigDrift(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	executeCI(t, configPath, "generate")
	executeCI(t, configPath, "validate")

	workflowPath := filepath.Join(directory, ".github", "workflows", "magelift.yml")
	workflow, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(workflow), "# v7.0.0", "# v4.0.0", 1)
	if err := os.WriteFile(workflowPath, []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	err = executeCIError(configPath, "validate")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "out of date") {
		t.Fatalf("workflow drift error = %v", err)
	}

	executeCI(t, configPath, "generate")
	updated := strings.Replace(starterConfig, "extensions: {}", "  production:\n    account: \"210987654321\"\nextensions: {}", 1)
	if err := os.WriteFile(configPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	err = executeCIError(configPath, "validate")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "out of date") {
		t.Fatalf("config drift error = %v", err)
	}
}

func TestCIGenerateRejectsMutableToolVersion(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	command := newCommand(&output, &output, nil)
	command.SetArgs([]string{"--config", configPath, "ci", "generate", "--magelift-version", "latest"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "immutable MageLift release") {
		t.Fatalf("error = %v", err)
	}
}

func TestCIGenerateIncludesProductionApprovalAndPromotion(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	config := strings.Replace(starterConfig, "  staging:\n    account: \"123456789012\"\n", "  staging:\n    account: \"123456789012\"\n  production:\n    account: \"210987654321\"\n    class: production\n", 1)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	executeCI(t, configPath, "generate")
	workflow, err := os.ReadFile(filepath.Join(directory, ".github", "workflows", "magelift.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{"name: Promote and deploy production", "environment: production", "--certificate-identity", "--yes promote", "deploy --digest"} {
		if !strings.Contains(text, required) {
			t.Fatalf("production workflow lacks %q:\n%s", required, text)
		}
	}
}

func TestCIGenerateIncludesClosedPreviewCleanup(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	config := strings.Replace(starterConfig, "environments:\n  staging:\n", "environments:\n  preview:\n    account: \"123456789012\"\n    monthlyBudgetCents: 10000\n    expiresAt: \"2026-07-19T00:00:00Z\"\n  staging:\n", 1)
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	executeCI(t, configPath, "generate")
	workflow, err := os.ReadFile(filepath.Join(directory, ".github", "workflows", "magelift.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{"name: Destroy closed preview", "environment: preview", "types: [opened, synchronize, reopened, labeled, closed]", "schedule:", "name: Sweep expired previews", "github.event.action == 'closed'", "--yes destroy", "env sweep"} {
		if !strings.Contains(text, required) {
			t.Fatalf("preview cleanup workflow lacks %q:\n%s", required, text)
		}
	}
}

func executeCI(t *testing.T, configPath, action string) {
	t.Helper()
	if err := executeCIError(configPath, action); err != nil {
		t.Fatal(err)
	}
}

func executeCIError(configPath, action string) error {
	var stdout, stderr bytes.Buffer
	command := newCommand(&stdout, &stderr, nil)
	command.SetArgs([]string{"--config", configPath, "ci", action, "--magelift-version", "v1.2.3"})
	return command.Execute()
}
