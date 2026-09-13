package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorChecksBuildAndEveryEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []string{`"id": "build"`, `"id": "environment.preview"`, `"id": "environment.staging"`, `"id": "environment.production"`, `"id": "local.edge"`, `"status": "ok"`, `"next": "magelift bootstrap --env preview"`, `CloudFront, Cloud Armor, and Fastly are cloud-only`} {
		if !strings.Contains(out.String(), evidence) {
			t.Fatalf("doctor output missing %s: %s", evidence, out.String())
		}
	}
	if strings.Contains(out.String(), `"id": "runtime.observe"`) || strings.Contains(out.String(), `"mode": "runtime"`) {
		t.Fatalf("doctor queried cloud runtime health: %s", out.String())
	}
	if strings.Contains(out.String(), "pulumi up") {
		t.Fatalf("doctor next-step documented pulumi up: %s", out.String())
	}
}

func TestDoctorPrintsFailedChecksAndUsesStableExitCode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	contents := strings.Replace(starterConfig, "    account: \"123456789012\"", "    account: \"123456789012\"\n    inherits: staging", 1)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != doctorExitUnhealthy {
		t.Fatalf("doctor error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.Contains(out.String(), `"status": "failed"`) || !strings.Contains(out.String(), `"id": "environment.staging"`) {
		t.Fatalf("doctor did not preserve failure evidence: %s", out.String())
	}
	if !strings.Contains(out.String(), `"next": "magelift config validate"`) {
		t.Fatalf("doctor did not name the fix: %s", out.String())
	}
}

func TestDoctorNextPointsAtValidateOnBuildFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	contents := strings.Replace(starterConfig, `php: "8.5"`, `php: "7.0"`, 1)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected doctor to fail on bad php version")
	}
	if !strings.Contains(out.String(), `"next": "magelift config validate"`) {
		t.Fatalf("doctor next = %s, want config validate", out.String())
	}
}

func TestDoctorNextPointsAtInstallerOnMissingDependency(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{missing: map[string]bool{"pulumi": true}}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected doctor to fail on missing pulumi")
	}
	if !strings.Contains(out.String(), `"next": "magelift doctor --install-dependencies"`) {
		t.Fatalf("doctor next = %s, want installer flag", out.String())
	}
}
