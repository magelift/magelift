package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

type fakeDependencyRunner struct {
	missing map[string]bool
	fail    map[string]error
	calls   [][]string
}

func (r *fakeDependencyRunner) LookPath(command string) (string, error) {
	if r.missing[command] {
		return "", errors.New("not found")
	}
	return "/test/bin/" + command, nil
}

func (r *fakeDependencyRunner) Run(_ context.Context, command string, args ...string) ([]byte, error) {
	argv := append([]string{command}, args...)
	r.calls = append(r.calls, append([]string(nil), argv...))
	key := strings.Join(argv, " ")
	if err := r.fail[key]; err != nil {
		return nil, err
	}
	if command == "brew" && len(args) >= 2 && args[0] == "install" {
		packageName := args[len(args)-1]
		switch packageName {
		case "docker-desktop":
			delete(r.missing, "docker")
		case "pulumi":
			delete(r.missing, "pulumi")
		}
	}
	return []byte("test-version\n"), nil
}

func TestDevUpStopsBeforeComposeWhenDockerIsUnavailable(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, ".magelift"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".magelift", "compose.local.yml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeDependencyRunner{missing: map[string]bool{"docker": true}}
	called := false
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.output, o.dependencyRunner = path, "json", runner
	o.runCompose = func(context.Context, string, []string, []string, io.Writer, io.Writer) error {
		called = true
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "local", "up"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "local Docker runtime is unavailable") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
	if called {
		t.Fatal("Compose ran after the dependency preflight failed")
	}
}

func TestDevSeedChecksDockerBeforeWritingCredentials(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, ".magelift"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".magelift", "compose.local.yml"), []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "bin", "magento"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runner := &fakeDependencyRunner{missing: map[string]bool{"docker": true}}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.output, o.dependencyRunner = path, "json", runner
	o.getenv = func(key string) string {
		if key == "MAGELIFT_LOCAL_ADMIN_PASSWORD" {
			return "Alocaladminpassword123"
		}
		return ""
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "local", "seed"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 3 {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
	if _, statErr := os.Stat(filepath.Join(directory, ".magelift", ".env.local")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("credentials file exists after preflight: %v", statErr)
	}
}

func TestDoctorDependencyInstallRequiresExplicitConfirmation(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeDependencyRunner{missing: map[string]bool{"docker": true}}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.output, o.dependencyRunner = path, "json", runner
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--output", "json", "doctor", "--install-dependencies"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
	for _, call := range runner.calls {
		if call[0] == "brew" {
			t.Fatalf("package manager ran without confirmation: %#v", runner.calls)
		}
	}
}

func TestDoctorDependencyInstallRechecksAfterExplicitInstall(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeDependencyRunner{missing: map[string]bool{"docker": true}}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.output, o.dependencyRunner, o.yes = path, "json", runner, true
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--output", "json", "doctor", "--install-dependencies"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"id": "dependency.docker"`) || !strings.Contains(output.String(), `"status": "ok"`) {
		t.Fatalf("dependency output = %s", output.String())
	}
	installed := false
	for _, call := range runner.calls {
		if strings.Join(call, " ") == "brew install --cask docker-desktop" {
			installed = true
		}
	}
	if !installed {
		t.Fatalf("install calls = %#v", runner.calls)
	}
}

func TestRunExecTargetPreflightsLauncherBeforeStartingSession(t *testing.T) {
	temporary := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(temporary, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeDependencyRunner{missing: map[string]bool{"kubectl": true}}
	called := false
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.dependencyRunner = runner
	o.runCommand = func(context.Context, string, []string, io.Writer, io.Writer) error {
		called = true
		return nil
	}
	err := o.runExecTarget(context.Background(), platform.ExecTarget{Launcher: "kubectl", CleanupPaths: []string{temporary}})
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "kubectl") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
	if called {
		t.Fatal("remote launcher started after preflight failure")
	}
	if _, statErr := os.Stat(temporary); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("temporary credential was not cleaned up: %v", statErr)
	}
}

func TestInfrastructureDoesNotRequirePulumiCLI(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	runner := &fakeDependencyRunner{missing: map[string]bool{"pulumi": true}}
	backendCalled := false
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.dependencyRunner = path, "staging", runner
	o.newBackend = func(context.Context, platform.PlannedStack, string) (infrastructureBackend, error) {
		backendCalled = true
		return &fakeInfrastructureBackend{}, nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "--yes", "deploy"})
	err := command.Execute()
	if !backendCalled {
		t.Fatalf("deploy stopped before the infrastructure backend because pulumi was absent: %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "pulumi") {
		t.Fatalf("deploy still requires the pulumi CLI: %v", err)
	}
}

func TestSignedReleasePreflightsCosignBeforeJournalRead(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	runner := &fakeDependencyRunner{missing: map[string]bool{"cosign": true}}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.dependencyRunner = path, "staging", runner
	o.newReleaseStore = func(string, string) (releaseStore, error) {
		t.Fatal("release journal was read after cosign preflight failure")
		return nil, nil
	}
	err := o.requireSignedRelease(context.Background(), "staging", releaseOne)
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "cosign") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
}
