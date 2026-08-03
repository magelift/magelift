package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
)

type fakeTerminal struct {
	interactive bool
	selected    string
	calls       int
}

func (t *fakeTerminal) Interactive() bool { return t.interactive }
func (t *fakeTerminal) SelectEnvironment([]string) (string, error) {
	t.calls++
	return t.selected, nil
}

func testOptions(out *bytes.Buffer, terminal environmentTerminal) *options {
	modules := platform.NewModuleRegistry()
	registerTestModules(modules)
	return &options{
		stdout:        out,
		stderr:        out,
		getenv:        func(string) string { return "" },
		currentBranch: func(string) (string, error) { return "", nil },
		terminal:      terminal,
		modules:       modules,
	}
}

func TestConfigEffectiveJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "magelift.yaml"), []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", filepath.Join(dir, "magelift.yaml"), "--env", "staging", "--output", "json", "config", "effective"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "example-shop"`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestEnvironmentRequiredWithoutPrompt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "magelift.yaml"), []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", filepath.Join(dir, "magelift.yaml"), "config", "effective"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "environment is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d", ExitCode(err))
	}
}

func TestEnvironmentListUsesStructuredOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"name": "staging"`) {
		t.Fatalf("unexpected environment output: %s", out.String())
	}
}

func TestStatusReportsSelectedConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "status"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"environment": "staging"`) || !strings.Contains(out.String(), `"preset": "preview"`) {
		t.Fatalf("unexpected status output: %s", out.String())
	}
}

func TestDeployRequiresRegisteredStackModule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := New()
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "deploy"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), `no stack module registered for target "aws"/"ecs-fargate"`) {
		t.Fatalf("unexpected error/code: %v/%d", err, ExitCode(err))
	}
}

func TestTopLevelCommandNamesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, command := range New().Commands() {
		if seen[command.Name()] {
			t.Fatalf("duplicate top-level command %q", command.Name())
		}
		seen[command.Name()] = true
	}
}

func TestConfigValidateRejectsEnvironmentBuildOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	contents := strings.Replace(starterConfig, "    account: \"123456789012\"", "    account: \"123456789012\"\n    build:\n      php: \"8.4\"", 1)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := New()
	cmd.SetArgs([]string{"--config", path, "config", "validate"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "staging.build") {
		t.Fatalf("unexpected error/code: %v/%d", err, ExitCode(err))
	}
}

func TestEnvironmentSelectionPrecedence(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "magelift.yaml")
	contents := strings.Replace(starterConfig, "  staging:\n", "  staging:\n    branches: [main]\n", 1)
	contents = strings.Replace(contents, "extensions: {}", "  production:\n    account: \"210987654321\"\nextensions: {}", 1)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		flag       string
		env        string
		branch     string
		wantDomain string
	}{
		{name: "flag beats environment and branch", flag: "production", env: "staging", branch: "main", wantDomain: "210987654321"},
		{name: "environment beats branch", env: "production", branch: "main", wantDomain: "210987654321"},
		{name: "branch mapping", branch: "main", wantDomain: "123456789012"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			terminal := &fakeTerminal{interactive: true, selected: "staging"}
			o := testOptions(&out, terminal)
			o.configPath = configPath
			o.environment = tt.flag
			o.getenv = func(string) string { return tt.env }
			o.currentBranch = func(string) (string, error) { return tt.branch, nil }
			file, err := o.load()
			if err != nil {
				t.Fatal(err)
			}
			selected, err := o.selectEnvironment(file)
			if err != nil {
				t.Fatal(err)
			}
			effective, err := file.Resolve(selected, config.ResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if effective.Config.Account != tt.wantDomain {
				t.Fatalf("selected account %q", effective.Config.Account)
			}
			if terminal.calls != 0 {
				t.Fatal("interactive selection was called")
			}
		})
	}
}

func TestEnvironmentSelectionPromptsOnlyOnTTY(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	terminal := &fakeTerminal{interactive: true, selected: "staging"}
	o := testOptions(&out, terminal)
	o.configPath = configPath
	file, err := o.load()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := o.selectEnvironment(file)
	if err != nil || selected != "staging" || terminal.calls != 1 {
		t.Fatalf("selected=%q calls=%d error=%v", selected, terminal.calls, err)
	}

	terminal.interactive = false
	if _, err := o.selectEnvironment(file); err == nil || !strings.Contains(err.Error(), "non-interactive") {
		t.Fatalf("unexpected error: %v", err)
	}
	o.noInteraction = true
	terminal.interactive = true
	if _, err := o.selectEnvironment(file); err == nil || !strings.Contains(err.Error(), "non-interactive") {
		t.Fatalf("unexpected --no-interaction error: %v", err)
	}
}

func TestConfigMigrateNormalizesCurrentSchema(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	input := strings.Replace(starterConfig, "project:\n  name: example-shop", "project: {name: example-shop}", 1)
	if err := os.WriteFile(path, []byte(input), 0o640); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "config", "migrate"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"changed": true`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	out.Reset()
	cmd = newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "config", "migrate"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"changed": false`) {
		t.Fatalf("unexpected idempotent output: %s", out.String())
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("second migration changed the file")
	}
}

func TestConfigMigrateRejectsUnknownSchemaWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	input := strings.Replace(starterConfig, "schemaVersion: 1", "schemaVersion: 2", 1)
	if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := New()
	cmd.SetArgs([]string{"--config", path, "config", "migrate"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "cannot migrate schema version 2") {
		t.Fatalf("unexpected error/code: %v/%d", err, ExitCode(err))
	}
	written, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(written) != input {
		t.Fatal("unsupported migration changed the file")
	}
}
