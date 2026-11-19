package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/projectphp"
	"go.yaml.in/yaml/v4"
)

func TestPHPProjectEnvironmentCommandsPreserveAuthoredCode(t *testing.T) {
	var document map[string]any
	if err := yaml.Unmarshal([]byte(starterConfig), &document); err != nil {
		t.Fatal(err)
	}
	data, err := projectphp.Render(document)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "return new Project(", "function shopSettings(): array { return ", 1))
	data = []byte(strings.TrimSuffix(string(data), ");\n") + "; }\n\nreturn new Project(shopSettings());\n")
	path := filepath.Join(t.TempDir(), "magelift.php")
	if err := os.WriteFile(path, data, 0o640); err != nil {
		t.Fatal(err)
	}
	original := string(data)
	var out bytes.Buffer
	run := func(args ...string) error {
		t.Helper()
		o := testOptions(&out, &fakeTerminal{interactive: false})
		o.configPath = path
		cmd := newCommandWithOptions(o)
		cmd.SetArgs(append([]string{"--config", path, "--output", "json"}, args...))
		return cmd.Execute()
	}
	if err := run("env", "create", "preview-42", "--account", "123456789012", "--class", "preview", "--preset", "preview", "--expires-at", "2026-07-19T12:00:00Z", "--monthly-budget-cents", "1000"); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
	if err := run("env", "protect", "staging", "--on"); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != original {
		t.Fatal("authored project changed")
	}
	result, err := projectphp.Evaluate(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	file, err := config.Load(result)
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", config.ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !effective.Config.Protection {
		t.Fatal("staging was not protected")
	}
	if _, err := file.Resolve("preview-42", config.ResolveOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(projectphp.OverlayPath(path)); err != nil {
		t.Fatal(err)
	}
}

func TestPHPProjectEnvironmentInvalidUpdateLeavesNoOverlay(t *testing.T) {
	var document map[string]any
	if err := yaml.Unmarshal([]byte(starterConfig), &document); err != nil {
		t.Fatal(err)
	}
	data, err := projectphp.Render(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "magelift.php")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "env", "create", "broken", "--preset", "garbage"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected invalid environment to fail")
	}
	if _, err := os.Stat(projectphp.OverlayPath(path)); !os.IsNotExist(err) {
		t.Fatalf("overlay exists after failed update: %v", err)
	}
}
