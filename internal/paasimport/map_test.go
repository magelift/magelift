package paasimport

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/config"
)

func TestMapACCSupportedFixture(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "acc", "supported")
	data, err := MapACC(root)
	if err != nil {
		t.Fatalf("MapACC: %v", err)
	}
	if strings.Contains(string(data), "synthetic-crypt-must-not-leak") {
		t.Fatal("crypt plaintext leaked into generated magelift.yaml")
	}
	file, err := config.Load(data)
	if err != nil {
		t.Fatalf("config.Load: %v\nYAML:\n%s", err, data)
	}
	envs := file.Environments()
	if len(envs) == 0 {
		t.Fatal("expected at least one environment")
	}
	if _, err := file.ResolveBuild(); err != nil {
		t.Fatalf("ResolveBuild: %v", err)
	}
	for _, env := range envs {
		if _, err := file.Resolve(env, config.ResolveOptions{}); err != nil {
			t.Fatalf("Resolve(%s): %v", env, err)
		}
	}
	text := string(data)
	for _, want := range []string{
		"name: sample-shop",
		`php: "8.5"`,
		"strategy: compact",
		"encryptionKeySecretArn:",
		"domain: staging.example.com",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated YAML missing %q:\n%s", want, text)
		}
	}
}

func TestDetectACCRequiresAppYAML(t *testing.T) {
	dir := t.TempDir()
	if err := DetectACC(dir); err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func repoRoot(t *testing.T) string {
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
