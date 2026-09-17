package cli

import (
	"bytes"
	"errors"
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

func TestDoctorCredentialsGCPMintsToken(t *testing.T) {
	path := writeGCPLifecycleConfig(t, "preview")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id": "credentials.gcp"`) || !strings.Contains(out.String(), "ADC mints tokens") {
		t.Fatalf("doctor lacks the ADC check: %s", out.String())
	}
}

func TestDoctorCredentialsGCPExplainsLoginGap(t *testing.T) {
	path := writeGCPLifecycleConfig(t, "preview")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{fail: map[string]error{"gcloud auth application-default print-access-token": errors.New("exit 1")}}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != doctorExitUnhealthy {
		t.Fatalf("doctor error/code = %v/%d", err, ExitCode(err))
	}
	for _, want := range []string{`"id": "credentials.gcp"`, "gcloud auth login alone is not enough", `"next": "gcloud auth application-default login"`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("doctor output missing %q: %s", want, out.String())
		}
	}
}

func TestDoctorCredentialsGCPUsesADCFileWithoutGcloud(t *testing.T) {
	path := writeGCPLifecycleConfig(t, "preview")
	adc := filepath.Join(t.TempDir(), "adc.json")
	if err := os.WriteFile(adc, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.getenv = func(key string) string {
		if key == "GOOGLE_APPLICATION_CREDENTIALS" {
			return adc
		}
		return ""
	}
	o.dependencyRunner = &fakeDependencyRunner{missing: map[string]bool{"gcloud": true}}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ADC file present") {
		t.Fatalf("doctor output: %s", out.String())
	}
}

func TestDoctorCredentialsGCPSkipsOnCI(t *testing.T) {
	path := writeGCPLifecycleConfig(t, "preview")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.getenv = func(key string) string {
		if key == "CI" {
			return "true"
		}
		return ""
	}
	o.dependencyRunner = &fakeDependencyRunner{missing: map[string]bool{"gcloud": true}}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status": "skipped"`) || !strings.Contains(out.String(), "workload identity") {
		t.Fatalf("doctor output: %s", out.String())
	}
}

func TestDoctorOmitsCredentialsForNonGCP(t *testing.T) {
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
	if strings.Contains(out.String(), `"id": "credentials.`) {
		t.Fatalf("non-GCP report carries credential checks: %s", out.String())
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
