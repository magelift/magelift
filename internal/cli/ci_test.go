package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCIWorkflowFixturesMatchTheRenderer(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	fixtures := []struct {
		name   string
		config string
		output string
	}{
		{
			name:   "aws",
			config: filepath.Join(repositoryRoot, "tests", "fixtures", "ci", "aws", "magelift.php"),
			output: filepath.Join(repositoryRoot, "tests", "fixtures", "ci", "aws", ".github", "workflows", "magelift.yml"),
		},
		{
			name:   "gcp",
			config: filepath.Join(repositoryRoot, "tests", "fixtures", "ci", "gcp", "magelift.php"),
			output: filepath.Join(repositoryRoot, "tests", "fixtures", "ci", "gcp", ".github", "workflows", "magelift.yml"),
		},
		{
			name:   "azure",
			config: filepath.Join(repositoryRoot, "tests", "fixtures", "ci", "azure", "magelift.php"),
			output: filepath.Join(repositoryRoot, "tests", "fixtures", "ci", "azure", ".github", "workflows", "magelift.yml"),
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			got, path, err := expectedWorkflow(&options{configPath: fixture.config}, &ciFlags{version: "v1.0.0-rc.1"})
			if err != nil {
				t.Fatal(err)
			}
			if path != fixture.output {
				t.Fatalf("workflow path = %q, want %q", path, fixture.output)
			}
			if os.Getenv("UPDATE_CLI_FIXTURES") == "1" {
				if err := os.WriteFile(fixture.output, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(fixture.output)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("generated workflow differs from checked-in fixture %q", fixture.output)
			}
			text := string(got)
			start, end := strings.Index(text, "\n  validate:"), strings.Index(text, "\n  preview:")
			if start < 0 || end < start || strings.Contains(text[start:end], "Install Pulumi CLI") {
				t.Fatal("Pulumi must not be installed in validation or build jobs")
			}
			jobs := []string{"preview", "preview-destroy", "preview-sweep", "staging"}
			for i, job := range jobs {
				begin := strings.Index(text, "\n  "+job+":")
				if begin < 0 {
					t.Fatalf("missing job %s", job)
				}
				rest := text[begin+1:]
				if i+1 < len(jobs) {
					next := strings.Index(rest, "\n  "+jobs[i+1]+":")
					if next < 0 {
						t.Fatalf("missing next job %s", jobs[i+1])
					}
					rest = rest[:next]
				}
				for _, required := range []string{"Install Pulumi CLI", "version=v3.277.0", "sha256sum --check -", "x86_64)", "aarch64|arm64)", "pulumi/pulumi"} {
					if !strings.Contains(rest, required) {
						t.Fatalf("job %s missing %s", job, required)
					}
				}
			}
			if bytes.Contains(got, []byte(`test -n "$PULUMI_BACKEND_URL"`)) || bytes.Contains(got, []byte("MAGELIFT_PULUMI_BACKEND_URL")) {
				t.Fatal("generated workflow still requires a Pulumi backend URL")
			}
		})
	}
}

func TestCIGenerateIsDeterministicAndKeepsWorkflowOffStdout(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.php")
	if err := writeTestProjectFile(configPath, []byte(starterConfig), 0o600); err != nil {
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
	for _, required := range []string{checkoutAction, setupBuildxAction, cosignInstallerAction, "MAGELIFT_VERSION: v1.2.3", "cosign verify-blob", "checksums.txt.sigstore.json", "sha256sum -c -", "Install MageLift provider", "providers install", "Build and sign immutable image", "environment: staging", "MAGELIFT_BUILD_ROLE_ARN", `- "staging"`, "--env \"${{ matrix.environment }}\"", "deploy --digest"} {
		if !strings.Contains(text, required) {
			t.Fatalf("workflow lacks %q:\n%s", required, text)
		}
	}
	for _, banned := range []string{"go install", "setup-go", "go-version-file"} {
		if strings.Contains(text, banned) {
			t.Fatalf("workflow still contains %q", banned)
		}
	}
}

func TestCIValidateDetectsWorkflowAndConfigDrift(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.php")
	if err := writeTestProjectFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	executeCI(t, configPath, "generate")
	executeCIWithArgs(t, configPath, "validate", "--output", "json")

	workflowPath := filepath.Join(directory, ".github", "workflows", "magelift.yml")
	workflow, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	customized := strings.Replace(string(workflow), "# v7.0.1", "# customized-checkout-pin", 1)
	customized += "\n      - name: Agency custom check\n        run: echo ok\n"
	if err := os.WriteFile(workflowPath, []byte(customized), 0o644); err != nil {
		t.Fatal(err)
	}
	executeCIWithArgs(t, configPath, "validate", "--output", "json")

	err = executeCIErrorWithArgs(configPath, "validate", "--check-template")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "does not match the current MageLift template") {
		t.Fatalf("template freshness error = %v", err)
	}

	err = executeCIErrorWithArgs(configPath, "generate")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "differs from the MageLift template") || !strings.Contains(err.Error(), "--replace") {
		t.Fatalf("generate without replace error = %v", err)
	}
	stillCustom, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stillCustom, []byte(customized)) {
		t.Fatal("custom workflow was modified without --replace")
	}
	candidates, err := filepath.Glob(workflowPath + ".candidate-*")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidate files = %v", candidates)
	}
	candidate, err := os.ReadFile(candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(candidate, stillCustom) {
		t.Fatal("candidate should carry the rendered template, not the customized workflow")
	}

	executeCIWithArgs(t, configPath, "generate", "--replace")
	replaced, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(replaced, stillCustom) {
		t.Fatal("expected --replace to adopt the template")
	}

	updated := strings.Replace(starterConfig, "  production:\n    inherits: staging\n    account: \"210987654321\"\n    class: production\n    preset: high-availability\n    domain: example.com\n    protection: true\n", "", 1)
	if err := writeTestProjectFile(configPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	executeCIWithArgs(t, configPath, "validate", "--output", "json")
	err = executeCIErrorWithArgs(configPath, "validate", "--check-template")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "does not match the current MageLift template") {
		t.Fatalf("config drift template freshness error = %v", err)
	}
}

func TestCIGenerateRejectsMutableToolVersion(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.php")
	if err := writeTestProjectFile(configPath, []byte(starterConfig), 0o600); err != nil {
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
	configPath := filepath.Join(directory, "magelift.php")
	// The default starter already carries staging plus production.
	config := starterConfig
	if err := writeTestProjectFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	executeCI(t, configPath, "generate")
	workflow, err := os.ReadFile(filepath.Join(directory, ".github", "workflows", "magelift.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{"name: Promote and deploy production", "environment: production", "--from staging", "--yes promote", "deploy --digest"} {
		if !strings.Contains(text, required) {
			t.Fatalf("production workflow lacks %q:\n%s", required, text)
		}
	}
	for _, forbidden := range []string{`test -n "$PULUMI_BACKEND_URL"`, "MAGELIFT_PULUMI_BACKEND_URL"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("production workflow still requires cloud-devops %q:\n%s", forbidden, text)
		}
	}
	// The application producer policy is independent of the deployment login.
	for _, required := range []string{"MAGELIFT_IMAGE_SIGNING_IDENTITY", "MAGELIFT_IMAGE_SIGNING_ISSUER", `--certificate-identity "$SIGNING_IDENTITY"`, `--certificate-oidc-issuer "$SIGNING_ISSUER"`} {
		if !strings.Contains(text, required) {
			t.Fatalf("producer trust policy missing %q", required)
		}
	}
}

func TestCIGenerateIncludesClosedPreviewCleanup(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.php")
	// The default starter already carries a preview env.
	config := starterConfig
	if err := writeTestProjectFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	executeCI(t, configPath, "generate")
	workflow, err := os.ReadFile(filepath.Join(directory, ".github", "workflows", "magelift.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{"name: Remove preview", "environment: preview", "types: [opened, synchronize, reopened]", "pull_request_target:", "types: [labeled, unlabeled, closed]", "github.event.action == 'unlabeled'", "schedule:", "name: Sweep expired previews", "github.event.action == 'closed'", "--yes destroy", "env sweep"} {
		if !strings.Contains(text, required) {
			t.Fatalf("preview cleanup workflow lacks %q:\n%s", required, text)
		}
	}
}

func TestCIGenerateGCPUsesFederationAndPRScopedIdentity(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.php")
	// The GCP starter already carries preview, staging, and production.
	config := strings.Replace(starterGCPConfig, "example-gcp-project", "example-gcp", 1)
	if err := writeTestProjectFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	executeCI(t, configPath, "generate")
	workflow, err := os.ReadFile(filepath.Join(directory, ".github", "workflows", "magelift.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(workflow)
	for _, required := range []string{
		"# target: gcp/gke-autopilot",
		gcpAuthAction,
		gcpSetupAction,
		"MAGELIFT_GCP_WORKLOAD_IDENTITY_PROVIDER",
		"MAGELIFT_GCP_BUILD_SERVICE_ACCOUNT",
		"MAGELIFT_GCP_STAGING_SERVICE_ACCOUNT",
		"--preview-repository",
		"--preview-number",
		"--preview-commit",
		"--preview-generation",
		"group: magelift-preview-${{ github.repository }}-${{ github.event.pull_request.number }}",
		"cancel-in-progress: false",
		"--preview-repository \"${{ github.repository }}\" --no-interaction --yes env sweep",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("GCP workflow lacks %q:\n%s", required, text)
		}
	}
	for _, forbidden := range []string{configureAWSAction, "MAGELIFT_AWS_REGION", "MAGELIFT_BUILD_ROLE_ARN", "service-account-key", "private_key"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("GCP workflow contains forbidden AWS or key material %q:\n%s", forbidden, text)
		}
	}
	executeCI(t, configPath, "validate")
}

func TestCIGenerateRejectsRemovedCloudProvider(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.php")
	config := strings.Replace(starterConfig, "  provider: aws\n  runtime: ecs-fargate\n  aws:", "  provider: ovh\n  runtime: mks\n  ovh:", 1)
	if err := writeTestProjectFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	err := executeCIError(configPath, "generate")
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "field ovh not found") {
		t.Fatalf("unsupported provider error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(directory, ".github", "workflows", "magelift.yml")); !os.IsNotExist(statErr) {
		t.Fatalf("unsupported target wrote a workflow: stat error = %v", statErr)
	}
}

func executeCI(t *testing.T, configPath, action string) {
	t.Helper()
	executeCIWithArgs(t, configPath, action)
}

func executeCIWithArgs(t *testing.T, configPath, action string, extra ...string) {
	t.Helper()
	if err := executeCIErrorWithArgs(configPath, action, extra...); err != nil {
		t.Fatal(err)
	}
}

func executeCIError(configPath, action string) error {
	return executeCIErrorWithArgs(configPath, action)
}

func executeCIErrorWithArgs(configPath, action string, extra ...string) error {
	var stdout, stderr bytes.Buffer
	command := newCommand(&stdout, &stderr, nil)
	args := []string{"--config", configPath, "ci", action, "--magelift-version", "v1.2.3"}
	args = append(args, extra...)
	command.SetArgs(args)
	return command.Execute()
}

func TestCIWorkflowNormalizesInstalledReleaseVersion(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.php")
	if err := writeTestProjectFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"1.2.3", "1.2.3-rc.1", "0.0.0-canary.2e75d313b03d53dd7cfe694297032bf8d69cc2ed"} {
		t.Run(version, func(t *testing.T) {
			actual, _, err := expectedWorkflow(&options{configPath: configPath}, &ciFlags{version: version})
			if err != nil {
				t.Fatal(err)
			}
			tagged, _, err := expectedWorkflow(&options{configPath: configPath}, &ciFlags{version: "v" + version})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actual, tagged) {
				t.Fatal("installed and tag versions produce different workflows")
			}
			if !bytes.Contains(actual, []byte("MAGELIFT_VERSION: v"+version)) {
				t.Fatal("workflow does not install the immutable release tag")
			}
		})
	}
}

func TestWorkflowProviderInstallUsesSelectedConfig(t *testing.T) {
	dir := t.TempDir()
	defaultPath := filepath.Join(dir, "magelift.php")
	configPath := filepath.Join(dir, "custom-config.php")
	if err := writeTestProjectFile(defaultPath, []byte(starterConfig), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(defaultPath, configPath); err != nil {
		t.Fatal(err)
	}
	workflow, _, err := expectedWorkflow(&options{configPath: configPath}, &ciFlags{version: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	want := "run: magelift --config " + shellQuote("custom-config.php") + " providers install"
	if got, total := strings.Count(string(workflow), want), strings.Count(string(workflow), "providers install"); got == 0 || got != total {
		t.Fatalf("correct install commands %d of %d", got, total)
	}
}
