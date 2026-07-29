package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/config"
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
