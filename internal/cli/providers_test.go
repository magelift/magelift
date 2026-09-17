package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/providerhost"
)

const providersTestConfig = `schemaVersion: 1
project:
  name: example-shop
application:
  edition: open-source
  version: 2.4.9
  mode: integrated
  webRuntime: nginx-fpm
build:
  php: "8.5"
target:
  provider: aws
  runtime: ecs-fargate
  aws: {}
defaults:
  region: eu-west-3
  preset: standard
environments:
  staging:
    account: "123456789012"
extensions: {}
`

const providersTestLock = `{"schemaVersion":1,"sdkAPIVersion":"v1","providers":{"aws":{"name":"magelift-provider-aws","version":"v0.0.0-test","protocol":"magelift-v1","digest":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","cosign":{"identity":"https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/v0.0.0-test","issuer":"https://token.actions.githubusercontent.com","bundle":"bundle.json"}}}}`

func writeProvidersFixture(t *testing.T) (configPath, lockPath string) {
	t.Helper()
	dir := t.TempDir()
	configPath = filepath.Join(dir, "magelift.yaml")
	lockPath = filepath.Join(dir, "magelift.providers.lock")
	if err := os.WriteFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte(providersTestLock), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath, lockPath
}

func TestProvidersInstallReportsBundledBinary(t *testing.T) {
	configPath, lockPath := writeProvidersFixture(t)
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "magelift-provider-aws")
	if err := os.WriteFile(binary, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(binDir, "magelift"), nil }
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "/cache/cosign", nil }
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "--output", "json", "providers", "install", "--lockfile", lockPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"bundled": true`) || !strings.Contains(out.String(), `"provider": "aws"`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestProvidersInstallRefusesUnknownProvider(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "magelift.yaml")
	gcpConfig := strings.Replace(providersTestConfig, "provider: aws\n  runtime: ecs-fargate\n  aws: {}", "provider: gcp\n  runtime: gke-autopilot\n  gcp:\n    project: example-gcp-project\n    region: europe-west1", 1)
	if err := os.WriteFile(configPath, []byte(gcpConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "magelift.providers.lock")
	if err := os.WriteFile(lockPath, []byte(providersTestLock), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "/cache/cosign", nil }
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "providers", "install", "--lockfile", lockPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not in magelift.providers.lock") {
		t.Fatalf("unexpected error: %v", err)
	}
	if ExitCode(err) != 3 {
		t.Fatalf("exit code = %d", ExitCode(err))
	}
}

func TestProvidersInstallRequiresLockfile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(t.TempDir(), "magelift"), nil }
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "providers", "install", "--lockfile", filepath.Join(dir, "missing.lock")})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "missing.lock") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvidersInstallBootstrapsMissingLockfile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	configPath := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "magelift-provider-aws")
	if err := os.WriteFile(binary, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(binDir, "magelift"), nil }
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "/cache/cosign", nil }
	o.fetchProviderLock = func(_ context.Context, version string) (providerhost.Lockfile, error) {
		if version != "v9.9.9" {
			t.Fatalf("version = %q", version)
		}
		return providerhost.ParseLock(strings.NewReader(providersTestLock))
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "--output", "json", "providers", "install", "--version", "v9.9.9"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"bootstrapped": true`) {
		t.Fatalf("output lacks bootstrap flag: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "magelift.providers.lock")); err != nil {
		t.Fatalf("bootstrapped lockfile missing: %v", err)
	}
}

func TestProvidersInstallRefusesRebootstrapOverExistingLock(t *testing.T) {
	configPath, lockPath := writeProvidersFixture(t)

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "providers", "install", "--lockfile", lockPath, "--version", "v9.9.9"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvidersInstallNeedsVersionWithoutLock(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	configPath := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(t.TempDir(), "magelift"), nil }
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "providers", "install"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProvidersInstallRegistersFlags(t *testing.T) {
	out := &bytes.Buffer{}
	root := newCommandWithOptions(testOptions(out, &fakeTerminal{interactive: false}))
	found, _, err := root.Find([]string{"providers", "install"})
	if err != nil {
		t.Fatalf("find providers install: %v", err)
	}
	for _, name := range []string{"lockfile", "cache-dir", "version"} {
		if found.Flags().Lookup(name) == nil {
			t.Errorf("providers install is missing --%s", name)
		}
	}
	if found.Flags().Lookup("provider") != nil {
		t.Errorf("providers install must not accept --provider (YAML-driven)")
	}
}

func TestProvidersInstallBootstrapsFromReleaseShapedVersion(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	configPath := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "magelift-provider-aws")
	if err := os.WriteFile(binary, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	previous := Version
	Version = "0.1.0-alpha.1-rc.2"
	t.Cleanup(func() { Version = previous })

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(binDir, "magelift"), nil }
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "/cache/cosign", nil }
	o.fetchProviderLock = func(_ context.Context, version string) (providerhost.Lockfile, error) {
		if version != "v0.1.0-alpha.1-rc.2" {
			t.Fatalf("version = %q", version)
		}
		return providerhost.ParseLock(strings.NewReader(providersTestLock))
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "--output", "json", "providers", "install"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"bootstrapped": true`) {
		t.Fatalf("output lacks bootstrap flag: %s", out.String())
	}
}

func TestProvidersInstallRefusesToReplaceInvalidLock(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	configPath := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "magelift.providers.lock")
	if err := os.WriteFile(lockPath, []byte("{bogus"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.fetchProviderLock = func(_ context.Context, _ string) (providerhost.Lockfile, error) {
		t.Fatal("bootstrap must not run over an invalid lock")
		return providerhost.Lockfile{}, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "providers", "install"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("install succeeded over an invalid lock")
	}
	contents, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "{bogus" {
		t.Fatalf("invalid lock was replaced: %q", contents)
	}
}
