package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
)

func TestInitFromAccWritesLoadValidConfig(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(dir, "magelift.yaml")
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init", "--from-acc"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --from-acc: %v\n%s", err, out.String())
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "synthetic-crypt-must-not-leak") {
		t.Fatal("crypt plaintext leaked into magelift.yaml")
	}
	file, err := config.Load(data)
	if err != nil {
		t.Fatalf("config.Load: %v\nYAML:\n%s", err, data)
	}
	if len(file.Environments()) == 0 {
		t.Fatal("expected environments")
	}
	if _, err := file.ResolveBuild(); err != nil {
		t.Fatalf("ResolveBuild: %v", err)
	}
	for _, env := range file.Environments() {
		if _, err := file.Resolve(env, config.ResolveOptions{}); err != nil {
			t.Fatalf("Resolve(%s): %v", env, err)
		}
	}

	// Same validation path as `magelift config validate`.
	var validateOut bytes.Buffer
	validate := newCommand(&validateOut, &validateOut, nil)
	validate.SetArgs([]string{"--config", configPath, "config", "validate"})
	if err := validate.Execute(); err != nil {
		t.Fatalf("config validate: %v\n%s", err, validateOut.String())
	}
}

func TestInitFromAccRefusesExistingConfig(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	configPath := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init", "--from-acc"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected refuse-if-exists")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2", ExitCode(err))
	}
}

func TestInitFromAccAndFromUpsunMutualExclusion(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "magelift.yaml")
	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init", "--from-acc", "--from-upsun"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected mutual exclusion error")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2; err=%v out=%s", ExitCode(err), err, out.String())
	}
	if !strings.Contains(err.Error(), "from-acc") || !strings.Contains(err.Error(), "from-upsun") {
		t.Fatalf("error should mention both flags: %v", err)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("should not write config on mutual exclusion: %v", err)
	}
}

func TestInitFromAccOverwriteWithYes(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	configPath := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "--yes", "init", "--from-acc"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --from-acc --yes: %v\n%s", err, out.String())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == starterConfig {
		t.Fatal("expected overwrite of existing starter config")
	}
	if _, err := config.Load(data); err != nil {
		t.Fatalf("config.Load after overwrite: %v", err)
	}
}

func TestInitConfigOutWritesSideFileLeavesDefault(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	defaultPath := filepath.Join(dir, "magelift.yaml")
	sidePath := filepath.Join(dir, "review.magelift.yaml")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", defaultPath, "init", "--from-acc", "--config-out", sidePath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --config-out: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(defaultPath); !os.IsNotExist(err) {
		t.Fatalf("default magelift.yaml should remain absent: %v", err)
	}
	data, err := os.ReadFile(sidePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(data); err != nil {
		t.Fatalf("config.Load side file: %v", err)
	}
}

func TestInitConfigOutRefusesExistingWithoutYes(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	sidePath := filepath.Join(dir, "review.magelift.yaml")
	if err := os.WriteFile(sidePath, []byte("existing: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"init", "--from-acc", "--config-out", sidePath})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected refuse existing --config-out path")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2; err=%v", ExitCode(err), err)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want already-exists message, got %v", err)
	}
}

func TestInitFromUpsunAccepted(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "upsun", "supported")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	configPath := filepath.Join(dir, "magelift.yaml")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init", "--from-upsun"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --from-upsun: %v\n%s", err, out.String())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(data); err != nil {
		t.Fatalf("config.Load: %v\nYAML:\n%s", err, data)
	}
	if _, err := os.Stat(filepath.Join(dir, "magelift.unmapped.md")); !os.IsNotExist(err) {
		t.Fatalf("supported Upsun import must not write sidecar: %v", err)
	}
}

func TestInitFromAccUnmappedWritesSidecarAndExitsNonZero(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "unmapped")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	configPath := filepath.Join(dir, "magelift.yaml")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "init", "--from-acc"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit on unmapped keys")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2; err=%v", ExitCode(err), err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("YAML must still be written: %v", err)
	}
	sidecar := filepath.Join(dir, "magelift.unmapped.md")
	report, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatalf("sidecar missing: %v", err)
	}
	for _, want := range []string{"hooks.build", "crons.shell-cleanup", "CUSTOM_FEATURE_FLAG"} {
		if !strings.Contains(string(report), want) {
			t.Fatalf("sidecar missing %q:\n%s", want, report)
		}
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(data); err != nil {
		t.Fatalf("written YAML must Load: %v", err)
	}
}

func TestInitConfigOutUnmappedSidecarUsesStem(t *testing.T) {
	fixture := filepath.Join(cliRepoRoot(t), "testdata", "fixtures", "acc", "unmapped")
	dir := t.TempDir()
	copyTree(t, fixture, dir)
	sidePath := filepath.Join(dir, "review.magelift.yaml")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"init", "--from-acc", "--config-out", sidePath})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected non-zero exit on unmapped keys")
	}
	if ExitCode(err) != 2 {
		t.Fatalf("exit code = %d, want 2", ExitCode(err))
	}
	sidecar := filepath.Join(dir, "review.magelift.unmapped.md")
	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("stem sidecar missing: %v", err)
	}
}

func TestInitBareWithYesOverwritesStarter(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "magelift.yaml")
	old := starterConfig + "# marker\n"
	if err := os.WriteFile(configPath, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := newCommand(&out, &out, nil)
	cmd.SetArgs([]string{"--config", configPath, "--yes", "init"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init --yes: %v\n%s", err, out.String())
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != starterConfig {
		t.Fatalf("expected starter rewrite; got %q", data)
	}
}

func cliRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s: %v", root, err)
	}
	return root
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
	if err != nil {
		t.Fatalf("copyTree: %v", err)
	}
}
