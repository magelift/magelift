package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/magelift/magelift/internal/cosign"
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
  provider: gcp
  runtime: gke-autopilot
  gcp:
    project: example-gcp-project
    region: europe-west1
defaults:
  region: eu-west-3
  preset: standard
environments:
  staging:
    account: "123456789012"
extensions: {}
`

const providersTestLock = `{"schemaVersion":1,"sdkAPIVersion":"v1","providers":{"gcp":{"name":"magelift-provider-gcp","version":"v0.0.0-test","protocol":"magelift-v1","digest":"sha256:b5d54c39e66671c9731b9f471e585d8262cd4f54963f0c93082d8dcf334d4c78","cosign":{"identity":"https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/v0.0.0-test","issuer":"https://token.actions.githubusercontent.com","bundle":"bundle.json"}}}}`

func writeProvidersFixture(t *testing.T) (configPath, lockPath string) {
	t.Helper()
	dir := t.TempDir()
	configPath = filepath.Join(dir, "magelift.php")
	lockPath = filepath.Join(dir, "magelift.providers.lock")
	if err := writeTestProjectFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(lockPath, []byte(providersTestLock), 0o600); err != nil {
		t.Fatal(err)
	}
	return configPath, lockPath
}

func TestProvidersInstallReportsBundledBinary(t *testing.T) {
	configPath, lockPath := writeProvidersFixture(t)
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "magelift-provider-gcp")
	if err := writeTestProjectFile(binary, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(filepath.Join(filepath.Dir(configPath), "bundle.json"), []byte("signature fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(binDir, "magelift"), nil }
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "/cache/cosign", nil }
	o.providerVerifier = providersStubVerifier{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "--output", "json", "providers", "install", "--lockfile", lockPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"bundled": true`) || !strings.Contains(out.String(), `"provider": "gcp"`) {
		t.Fatalf("unexpected output: %s", out.String())
	}
}

func TestProvidersInstallRefusesUnknownProvider(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "magelift.php")
	gcpConfig := providersTestConfig
	if err := writeTestProjectFile(configPath, []byte(gcpConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "magelift.providers.lock")
	if err := writeTestProjectFile(lockPath, []byte(strings.ReplaceAll(providersTestLock, "gcp", "azure")), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "/cache/cosign", nil }
	o.providerVerifier = providersStubVerifier{}
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
	configPath := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
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
	configPath := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "magelift-provider-gcp")
	if err := writeTestProjectFile(binary, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(filepath.Join(filepath.Dir(configPath), "bundle.json"), []byte("signature fixture"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(binDir, "magelift"), nil }
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "/cache/cosign", nil }
	o.providerVerifier = providersStubVerifier{}
	o.fetchProviderLock = func(_ context.Context, version string) (providerhost.Lockfile, error) {
		if version != "v9.9.9" {
			t.Fatalf("version = %q", version)
		}
		return providerhost.ParseLock(strings.NewReader(strings.ReplaceAll(providersTestLock, "v0.0.0-test", "v9.9.9")))
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
	configPath := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
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
	configPath := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	binary := filepath.Join(binDir, "magelift-provider-gcp")
	if err := writeTestProjectFile(binary, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(filepath.Join(filepath.Dir(configPath), "bundle.json"), []byte("signature fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := Version
	Version = "0.1.0-alpha.1-rc.2"
	t.Cleanup(func() { Version = previous })

	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(binDir, "magelift"), nil }
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "/cache/cosign", nil }
	o.providerVerifier = providersStubVerifier{}
	o.fetchProviderLock = func(_ context.Context, version string) (providerhost.Lockfile, error) {
		if version != "v0.1.0-alpha.1-rc.2" {
			t.Fatalf("version = %q", version)
		}
		return providerhost.ParseLock(strings.NewReader(strings.ReplaceAll(providersTestLock, "v0.0.0-test", "v0.1.0-alpha.1-rc.2")))
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
	configPath := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "magelift.providers.lock")
	if err := writeTestProjectFile(lockPath, []byte("{bogus"), 0o600); err != nil {
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

type providersStubVerifier struct{ err error }

func (v providersStubVerifier) VerifyBlob(context.Context, string, string, cosign.VerifyOptions) error {
	return v.err
}

func TestProvidersBootstrapFailureDoesNotPublishProjectLock(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	configPath := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(t.TempDir(), "magelift"), nil }
	o.fetchProviderLock = func(context.Context, string) (providerhost.Lockfile, error) {
		return providerhost.ParseLock(strings.NewReader(strings.ReplaceAll(providersTestLock, "v0.0.0-test", "v9.9.9")))
	}
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "", fmt.Errorf("fixture verifier unavailable") }
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "providers", "install", "--version", "v9.9.9"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "fixture verifier unavailable") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat("magelift.providers.lock"); !os.IsNotExist(err) {
		t.Fatalf("failed install published lock: %v", err)
	}
	temps, err := filepath.Glob(".magelift-providers-lock-*")
	if err != nil || len(temps) != 0 {
		t.Fatalf("staging files: %v, %v", temps, err)
	}
}

func TestProvidersInstallRefusesTamperedBundledBinary(t *testing.T) {
	configPath, lockPath := writeProvidersFixture(t)
	binDir := t.TempDir()
	if err := writeTestProjectFile(filepath.Join(binDir, "magelift-provider-gcp"), []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(binDir, "magelift"), nil }
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "/fixture/cosign", nil }
	o.providerVerifier = providersStubVerifier{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "providers", "install", "--lockfile", lockPath})
	if err := cmd.Execute(); !errors.Is(err, providerhost.ErrChecksumMismatch) {
		t.Fatalf("error = %v", err)
	}
}

func TestProvidersInstallRefusesInvalidBundledSignature(t *testing.T) {
	configPath, lockPath := writeProvidersFixture(t)
	binDir := t.TempDir()
	if err := writeTestProjectFile(filepath.Join(binDir, "magelift-provider-gcp"), []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(filepath.Join(filepath.Dir(configPath), "bundle.json"), []byte("signature fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(binDir, "magelift"), nil }
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "/fixture/cosign", nil }
	o.providerVerifier = providersStubVerifier{err: errors.New("fixture invalid signature")}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "providers", "install", "--lockfile", lockPath})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "fixture invalid signature") {
		t.Fatalf("error = %v", err)
	}
}

func TestProvidersBootstrapPreservesConcurrentProjectPin(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	configPath := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(configPath, []byte(providersTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile("bundle.json", []byte("signature fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	if err := writeTestProjectFile(filepath.Join(binDir, "magelift-provider-gcp"), []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(binDir, "magelift"), nil }
	o.fetchProviderLock = func(context.Context, string) (providerhost.Lockfile, error) {
		return providerhost.ParseLock(strings.NewReader(strings.ReplaceAll(providersTestLock, "v0.0.0-test", "v9.9.9")))
	}
	o.ensureProviderVerifier = func(context.Context, string) (string, error) {
		return "/fixture/cosign", writeTestProjectFile("magelift.providers.lock", []byte("concurrent project pin"), 0o600)
	}
	o.providerVerifier = providersStubVerifier{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "providers", "install", "--version", "v9.9.9"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "without replacing an existing pin") {
		t.Fatalf("error = %v", err)
	}
	body, err := os.ReadFile("magelift.providers.lock")
	if err != nil || string(body) != "concurrent project pin" {
		t.Fatalf("project pin = %q, %v", body, err)
	}
}

func TestDefaultLoadProviderAcquiresPinnedGCPFromOfflineCache(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	cache := t.TempDir()
	t.Setenv("MAGELIFT_PROVIDER_CACHE_DIR", cache)
	lock := strings.ReplaceAll(providersTestLock, "gcp", "gcp")
	if err := writeTestProjectFile("magelift.providers.lock", []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	paths, err := providerhost.CachedArtifactPaths(cache, "gcp", "v0.0.0-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(paths.Binary, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(providerhost.CachedBundlePath(paths.Binary), []byte("signature fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.providerVerifier = providersStubVerifier{}
	o.fetchProviderLock = func(context.Context, string) (providerhost.Lockfile, error) {
		t.Fatal("project pin must not bootstrap")
		return providerhost.Lockfile{}, nil
	}
	loaded, err := o.defaultLoadProvider(context.Background(), "gcp")
	if err != nil || loaded.Binary != paths.Binary || loaded.Artifact.Version != "v0.0.0-test" {
		t.Fatalf("loaded=%+v, error=%v", loaded, err)
	}
}

func TestProvidersInstallBuiltinOnlyHasNoSideEffects(t *testing.T) {
	for _, provider := range []struct{ name, runtime, settings string }{
		{"aws", "ecs-fargate", "aws: {}"}, {"ovh", "mks", "ovh:\n    serviceName: example-service"}, {"scaleway", "kapsule", "scaleway:\n    projectId: example-project"},
	} {
		t.Run(provider.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			configPath := filepath.Join(dir, "magelift.php")
			target := "provider: " + provider.name + "\n  runtime: " + provider.runtime + "\n  " + provider.settings
			text := strings.Replace(providersTestConfig, "provider: gcp\n  runtime: gke-autopilot\n  gcp:\n    project: example-gcp-project\n    region: europe-west1", target, 1)
			if err := writeTestProjectFile(configPath, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			o := testOptions(&out, &fakeTerminal{interactive: false})
			o.executable = func() (string, error) { t.Fatal("builtin target looked up executable"); return "", nil }
			o.fetchProviderLock = func(context.Context, string) (providerhost.Lockfile, error) {
				t.Fatal("builtin target fetched lock")
				return providerhost.Lockfile{}, nil
			}
			o.ensureProviderVerifier = func(context.Context, string) (string, error) {
				t.Fatal("builtin target installed verifier")
				return "", nil
			}
			cmd := newCommandWithOptions(o)
			cmd.SetArgs([]string{"--config", configPath, "--output", "json", "providers", "install"})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat("magelift.providers.lock"); !os.IsNotExist(err) {
				t.Fatalf("unexpected lock: %v", err)
			}
			if !strings.Contains(out.String(), `"providers": []`) {
				t.Fatalf("output: %s", out.String())
			}
		})
	}
}

func TestProvidersBootstrapRejectsMismatchedReleaseBeforeSideEffects(t *testing.T) {
	for _, field := range []string{"version", "identity", "extra-provider"} {
		t.Run(field, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			if err := writeTestProjectFile("magelift.php", []byte(providersTestConfig), 0600); err != nil {
				t.Fatal(err)
			}
			o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
			o.executable = func() (string, error) { return filepath.Join(dir, "no-binary", "magelift"), nil }
			o.fetchProviderLock = func(context.Context, string) (providerhost.Lockfile, error) {
				lock, err := providerhost.ParseLock(strings.NewReader(strings.ReplaceAll(providersTestLock, "v0.0.0-test", "v9.9.9")))
				if err != nil {
					return lock, err
				}
				artifact := lock.Providers["gcp"]
				switch field {
				case "version":
					artifact.Version = "v1.0.0"
				case "identity":
					artifact.Cosign.Identity = providerhost.FirstPartyIdentityPrefix + "v1.0.0"
				case "extra-provider":
					artifact.Version = "v1.0.0"
					lock.Providers["azure"] = artifact
					return lock, nil
				}
				lock.Providers["gcp"] = artifact
				return lock, nil
			}
			o.ensureProviderVerifier = func(context.Context, string) (string, error) {
				t.Fatal("verifier invoked on mismatched lock")
				return "", nil
			}
			cmd := newCommandWithOptions(o)
			cmd.SetArgs([]string{"--config", "magelift.php", "providers", "install", "--version", "v9.9.9"})
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "does not match release tag") {
				t.Fatalf("error: %v", err)
			}
			if _, err := os.Stat("magelift.providers.lock"); !os.IsNotExist(err) {
				t.Fatalf("lock published: %v", err)
			}
			matches, _ := filepath.Glob(".magelift-providers-lock-*")
			if len(matches) > 0 {
				t.Fatalf("staged locks: %v", matches)
			}
		})
	}
}

func TestProvidersInstallMixedBuiltinAndPlugin(t *testing.T) {
	dir := t.TempDir()
	text := strings.Replace(providersTestConfig, "  staging:\n    account:", "  aws-preview:\n    target:\n      provider: aws\n      runtime: ecs-fargate\n      aws: {}\n      gcp: null\n  staging:\n    account:", 1)
	configPath := filepath.Join(dir, "magelift.php")
	lockPath := filepath.Join(dir, "magelift.providers.lock")
	if err := writeTestProjectFile(configPath, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(lockPath, []byte(providersTestLock), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(filepath.Join(dir, "bundle.json"), []byte("signature fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeTestProjectFile(filepath.Join(dir, "magelift-provider-gcp"), []byte("fake"), 0755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.executable = func() (string, error) { return filepath.Join(dir, "magelift"), nil }
	o.ensureProviderVerifier = func(context.Context, string) (string, error) { return "/fixture/cosign", nil }
	o.providerVerifier = providersStubVerifier{}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "--output", "json", "providers", "install", "--lockfile", lockPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"provider": "gcp"`) || strings.Contains(out.String(), `"provider": "aws"`) {
		t.Fatalf("output: %s", out.String())
	}
}
