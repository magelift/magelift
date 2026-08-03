package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	"go.yaml.in/yaml/v4"
)

func TestEnvProtectUpdatesOnlySelectedOverlay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o640); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "protect", "staging", "--on"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "protection: true") || !strings.Contains(out.String(), `"protection": true`) {
		t.Fatalf("protection update missing: config=%s output=%s", data, out.String())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("config mode changed to %o", info.Mode().Perm())
	}
}

func TestEnvProtectRequiresExplicitChoiceAndProductionApproval(t *testing.T) {
	contents := strings.Replace(starterConfig, "extensions: {}", "  production:\n    account: \"123456789012\"\n    class: production\n    domain: example.com\n    monthlyBudgetCents: 1000\nextensions: {}", 1)
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--config", path, "env", "protect", "staging"},
		{"--config", path, "env", "protect", "production", "--off"},
	} {
		cmd := New()
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil || ExitCode(err) != 2 {
			t.Fatalf("args %v returned error/code %v/%d", args, err, ExitCode(err))
		}
	}
}

func TestEnvCreateAddsValidatedOverlayAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o640); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "create", "preview-42", "--account", "123456789012", "--class", "preview", "--preset", "preview", "--expires-at", "2026-07-19T12:00:00Z", "--monthly-budget-cents", "1000"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "preview-42:") || !strings.Contains(out.String(), `"created": true`) {
		t.Fatalf("environment was not created: config=%s output=%s", data, out.String())
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("config mode changed to %o", info.Mode().Perm())
	}

	cmd = New()
	cmd.SetArgs([]string{"--config", path, "env", "create", "preview-42"})
	err = cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("duplicate create error/code = %v/%d", err, ExitCode(err))
	}
}

func TestEnvDestroyDestroysInfrastructureBeforeRemovingOverlay(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newLock = func(context.Context, platform.PlannedStack) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--yes", "--output", "json", "env", "destroy", "staging"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"preview", "destroy"}) {
		t.Fatalf("backend calls = %v", backend.calls)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "staging:") || !strings.Contains(out.String(), `"removed": true`) {
		t.Fatalf("environment overlay was not removed: config=%s output=%s", data, out.String())
	}
}

func TestEnvSweepDryRunReportsOnlyExpiredPreviewOverlays(t *testing.T) {
	path := writeLifecycleConfig(t, "preview", false)
	setEnvironmentExpiration(t, path, "staging", "2020-01-01T00:00:00Z")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "env", "sweep", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"action": "would-destroy"`) {
		t.Fatalf("expired preview was not reported: %s", out.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "staging:") {
		t.Fatal("dry run changed the configuration")
	}
}

func TestEnvSweepDestroysExpiredPreviewAndRemovesOverlay(t *testing.T) {
	path := writeLifecycleConfig(t, "preview", false)
	setEnvironmentExpiration(t, path, "staging", "2020-01-01T00:00:00Z")
	backend := &fakeInfrastructureBackend{}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newLock = func(context.Context, platform.PlannedStack) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "--yes", "env", "sweep"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"preview", "destroy"}) {
		t.Fatalf("backend calls = %v", backend.calls)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "staging:") || !strings.Contains(out.String(), `"action": "destroyed"`) {
		t.Fatalf("expired preview was not destroyed: config=%s output=%s", data, out.String())
	}
}

func TestEnvSweepSkipsProtectedExpiredPreview(t *testing.T) {
	path := writeLifecycleConfig(t, "preview", true)
	setEnvironmentExpiration(t, path, "staging", "2020-01-01T00:00:00Z")
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--output", "json", "--yes", "env", "sweep"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"reason": "environment is protected"`) {
		t.Fatalf("protected preview was not classified: %s", out.String())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "staging:") {
		t.Fatal("protected preview configuration was removed")
	}
}

func setEnvironmentExpiration(t *testing.T, path, name, expiresAt string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	environments := document["environments"].(map[string]any)
	overlay := environments[name].(map[string]any)
	overlay["expiresAt"] = expiresAt
	environments[name] = overlay
	updated, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		t.Fatal(err)
	}
}
