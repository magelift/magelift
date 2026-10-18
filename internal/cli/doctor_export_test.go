package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	doctorExportCanaryToken = "DOCTOR_EXPORT_CANARY_TOKEN_9f3c2a1b"
	doctorExportCanaryPath  = "/secret/canary/DOCTOR_EXPORT_CANARY_PATH_segment"
	doctorExportCanaryURL   = "https://canary.invalid/DOCTOR_EXPORT_CANARY_URL?token=leak"
)

func assertDoctorExportNoCanary(t *testing.T, payload string) {
	t.Helper()
	for _, canary := range []string{doctorExportCanaryToken, doctorExportCanaryPath, doctorExportCanaryURL} {
		if strings.Contains(payload, canary) {
			t.Fatalf("export leaks canary %q: %s", canary, payload)
		}
	}
}

func assertDoctorExportAllowlistShape(t *testing.T, exported []byte) {
	t.Helper()
	var top map[string]json.RawMessage
	if err := json.Unmarshal(exported, &top); err != nil {
		t.Fatal(err)
	}
	for key := range top {
		switch key {
		case "schemaVersion", "cliVersion", "runtime", "status", "target", "checks", "omitted":
		default:
			t.Fatalf("unexpected top-level export key %q", key)
		}
	}
	var checks map[string]json.RawMessage
	if err := json.Unmarshal(top["checks"], &checks); err != nil {
		t.Fatal(err)
	}
	for key := range checks {
		if key != "summary" && key != "items" {
			t.Fatalf("unexpected checks key %q", key)
		}
	}
	var items []map[string]string
	if err := json.Unmarshal(checks["items"], &items); err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		for key := range item {
			if key != "category" && key != "id" && key != "status" && key != "requirement" {
				t.Fatalf("unexpected check item key %q in %#v", key, item)
			}
			if key == "requirement" && item[key] != "required" && item[key] != "optional" {
				t.Fatalf("unexpected requirement value %q", item[key])
			}
		}
	}
}

func TestDoctorLocalRunsWithMissingConfig(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "magelift.yaml")
	var out bytes.Buffer
	runner := &fakeDependencyRunner{}
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = missing, "json"
	o.dependencyRunner = runner
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", missing, "--output", "json", "doctor", "--local"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != doctorExitUnhealthy {
		t.Fatalf("doctor error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.Contains(out.String(), `"id": "config"`) {
		t.Fatalf("doctor output missing config check: %s", out.String())
	}
	for _, call := range runner.calls {
		if strings.Contains(strings.Join(call, " "), "print-access-token") {
			t.Fatalf("local doctor invoked credential probe: %#v", runner.calls)
		}
	}
}

func TestDoctorLocalRunsWithInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte("not: valid: yaml: [[["), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor", "--local"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != doctorExitUnhealthy {
		t.Fatalf("doctor error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.Contains(out.String(), `"id": "config"`) {
		t.Fatalf("doctor output: %s", out.String())
	}
}

func TestDoctorLocalSkipsCredentialProbeOnGCP(t *testing.T) {
	path := writeGCPLifecycleConfig(t, "preview")
	var out bytes.Buffer
	runner := &fakeDependencyRunner{}
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = runner
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor", "--local"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"id": "credentials.gcp"`) || !strings.Contains(out.String(), `"status": "skipped"`) {
		t.Fatalf("doctor output: %s", out.String())
	}
	for _, call := range runner.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "gcloud") && strings.Contains(joined, "print-access-token") {
			t.Fatalf("credential probe ran under --local: %#v", runner.calls)
		}
	}
}

func TestDoctorMissingConfigStillInvalidWithoutLocal(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "magelift.yaml")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = missing, "json"
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", missing, "--output", "json", "doctor"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("doctor error/code = %v/%d", err, ExitCode(err))
	}
}

func TestDoctorExportWrittenOnFailedReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	contents := strings.Replace(starterConfig, "    account: \"123456789012\"", "    account: \"123456789012\"\n    inherits: staging", 1)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(dir, "doctor-export.json")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor", "--export", exportPath})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != doctorExitUnhealthy {
		t.Fatalf("doctor error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.Contains(out.String(), `"status": "failed"`) {
		t.Fatalf("stdout report missing failure: %s", out.String())
	}
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	assertDoctorExportNoCanary(t, string(data))
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("export JSON: %v", err)
	}
	if doc["schemaVersion"] != float64(doctorExportSchemaVersion) || doc["status"] != "failed" {
		t.Fatalf("export document: %#v", doc)
	}
	if _, ok := doc["config"]; ok {
		t.Fatalf("export must not include config path: %#v", doc)
	}
}

func TestDoctorExportOmitsSensitiveReportFields(t *testing.T) {
	path := writeGCPLifecycleConfig(t, "preview")
	exportPath := filepath.Join(t.TempDir(), "export.json")
	adc := filepath.Join(t.TempDir(), "adc-"+doctorExportCanaryToken+".json")
	if err := os.WriteFile(adc, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	runner := &fakeDependencyRunner{missing: map[string]bool{"gcloud": true}}
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = runner
	o.getenv = func(key string) string {
		if key == "GOOGLE_APPLICATION_CREDENTIALS" {
			return adc
		}
		return ""
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor", "--export", exportPath})
	_ = cmd.Execute()
	stdout := out.String()
	if !strings.Contains(stdout, doctorExportCanaryToken) || !strings.Contains(stdout, `"installHint"`) {
		t.Fatalf("stdout should still carry rich diagnostics: %s", stdout)
	}
	exported, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	payload := string(exported)
	assertDoctorExportNoCanary(t, payload)
	assertDoctorExportAllowlistShape(t, exported)
	var doc doctorExportDocument
	if err := json.Unmarshal(exported, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Omitted.Contract == "" || len(doc.Omitted.Fields) == 0 {
		t.Fatalf("missing omission contract: %#v", doc.Omitted)
	}
	if doc.Runtime.GOOS == "" || doc.Runtime.GOARCH == "" {
		t.Fatalf("runtime metadata missing: %#v", doc.Runtime)
	}
}

func TestDoctorExportFileModeAndRefuseOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(dir, "doctor-safe.json")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor", "--export", exportPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("export mode = %#o, want 0600", info.Mode().Perm())
	}
	cmd = newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor", "--export", exportPath})
	err = cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "overwrite") {
		t.Fatalf("overwrite error = %v", err)
	}
}

func TestDoctorExportRefusesSymlinkTarget(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("symlink refusal is exercised on Unix-like systems")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(dir, "real-export.json")
	link := filepath.Join(dir, "linked-export.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor", "--export", link})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("symlink export error = %v", err)
	}
}

func TestDoctorExportWithLocalBadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	bad := "project:\n  name: " + doctorExportCanaryToken + "\naccount: " + doctorExportCanaryURL + "\n"
	if err := os.WriteFile(path, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	exportPath := filepath.Join(dir, "local-export.json")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor", "--local", "--export", exportPath})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != doctorExitUnhealthy {
		t.Fatalf("doctor error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.Contains(out.String(), `"id": "config"`) {
		t.Fatalf("stdout should include config failure: %s", out.String())
	}
	exported, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	assertDoctorExportNoCanary(t, string(exported))
}

func TestDoctorExportCategoricalFieldsRejectPrivateValues(t *testing.T) {
	const privateName = "private-token-like-project"
	report := doctorReport{Status: privateName, Checks: []doctorCheck{
		{ID: "environment." + privateName, Status: "ok", Message: privateName},
		{ID: "dependency." + privateName, Status: privateName},
		{ID: privateName, Status: privateName},
	}}
	doc, err := buildDoctorExport(report, nil)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), privateName) {
		t.Fatalf("private categorical value exported: %s", payload)
	}
	if doc.Status != "failed" || doc.Checks.Summary.Failed != 2 || doc.Checks.Items[0].ID != "environment" {
		t.Fatalf("unexpected safe summary: %#v", doc)
	}
	if exportableDoctorTarget(privateName, "gke-autopilot") != nil || exportableDoctorTarget("gcp", privateName) != nil {
		t.Fatal("arbitrary target accepted")
	}
	for _, value := range []string{privateName, "1.2.3-" + privateName, "1.2.3+" + privateName} {
		if doctorExportVersion(value) != "" {
			t.Fatalf("private version exported: %q", value)
		}
	}
	for _, value := range []string{"dev", "v1.2.3", "v0.1.0-alpha.1", "v0.0.0-canary." + strings.Repeat("a", 40)} {
		if doctorExportVersion(value) != value {
			t.Fatalf("known release version omitted: %q", value)
		}
	}
}

func TestDoctorExportMissingConfigPreservesInvalidExitCode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.yaml")
	exportPath := filepath.Join(dir, "export.json")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.dependencyRunner = &fakeDependencyRunner{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor", "--export", exportPath})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("invalid config code changed: %v", err)
	}
	payload, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), path) {
		t.Fatalf("path exported: %s", payload)
	}
	var doc doctorExportDocument
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Status != "failed" || doc.Checks.Summary.Failed < 1 {
		t.Fatalf("missing config failure omitted: %#v", doc)
	}
}

func TestDoctorExportPreservesKnownOptionalDependencyStatus(t *testing.T) {
	doc, err := buildDoctorExport(doctorReport{Status: "ok", Checks: []doctorCheck{
		{ID: "dependency.docker", Status: "missing", Requirement: "optional"},
		{ID: "dependency.gcloud", Status: "unreachable", Requirement: doctorExportCanaryToken},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Status != "ok" || doc.Checks.Summary.Failed != 0 || doc.Checks.Summary.Missing != 1 || doc.Checks.Summary.Unreachable != 1 {
		t.Fatalf("optional tool state was changed: %#v", doc)
	}
	if doc.Checks.Items[0].Requirement != "optional" || doc.Checks.Items[1].Requirement != "" {
		t.Fatalf("requirement allowlist failed: %#v", doc.Checks.Items)
	}
}
