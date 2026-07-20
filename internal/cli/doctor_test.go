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
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, evidence := range []string{`"id": "build"`, `"id": "environment.staging"`, `"status": "ok"`} {
		if !strings.Contains(out.String(), evidence) {
			t.Fatalf("doctor output missing %s: %s", evidence, out.String())
		}
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
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "doctor"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != doctorExitUnhealthy {
		t.Fatalf("doctor error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.Contains(out.String(), `"status": "failed"`) || !strings.Contains(out.String(), `"id": "environment.staging"`) {
		t.Fatalf("doctor did not preserve failure evidence: %s", out.String())
	}
}
