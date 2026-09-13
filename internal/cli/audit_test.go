package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuditExportsControlPointersWithoutSecretsOrCertificates(t *testing.T) {
	path := writeLifecycleConfig(t, "production", true)
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.output = path, "staging", "json"
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "audit"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	if strings.Contains(got, "database-password") || strings.Contains(got, "super-secret") {
		t.Fatalf("secret leaked: %s", got)
	}
	for _, banned := range []string{"SOC 2 certified", "ISO 27001 certified", "GDPR certified"} {
		if strings.Contains(got, banned) {
			t.Fatalf("audit claimed %q: %s", banned, got)
		}
	}
	if !strings.Contains(got, "does not certify") {
		t.Fatalf("missing disclaimer: %s", got)
	}
	var report auditReport
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("decode: %v\n%s", err, got)
	}
	if report.Environment != "staging" {
		t.Fatalf("environment = %#v", report.Environment)
	}
	found := map[string]bool{}
	for _, control := range report.Controls {
		found[control.ID] = true
		if len(control.Pointers) == 0 && control.ID != "residency" {
			t.Fatalf("control %s has no pointers: %#v", control.ID, control)
		}
	}
	for _, id := range []string{"encryption", "iam", "logging", "backup", "waf", "residency"} {
		if !found[id] {
			t.Fatalf("missing control %s: %#v", id, report.Controls)
		}
	}
}

func TestAuditIsDistinctFromEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	command := newCommand(&output, &output, nil)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	help := output.String()
	if !strings.Contains(help, "  audit ") || !strings.Contains(help, "  evidence ") {
		t.Fatalf("help must list audit and evidence separately:\n%s", help)
	}
}
