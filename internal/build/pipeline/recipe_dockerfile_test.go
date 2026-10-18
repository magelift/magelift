package pipeline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	buildcontext "github.com/magelift/magelift"
)

func TestEmbeddedApplicationFragmentUsesRuntimeStage(t *testing.T) {
	fragment, err := embeddedApplicationFragment("abc123")
	if err != nil {
		t.Fatal(err)
	}
	body := string(fragment)
	if strings.Contains(body, "RUNTIME_BASE") || strings.Contains(body, "# syntax=") {
		t.Fatalf("fragment still references external runtime base:\n%s", body)
	}
	if !strings.Contains(body, "FROM runtime") {
		t.Fatal("fragment must continue from the embedded runtime stage")
	}
	if !strings.Contains(body, "io.magelift.recipe.sha256") {
		t.Fatal("fragment must declare recipe trace label")
	}
}

func TestMaterializeEmbeddedRecipeDockerfileKeepsPinnedBases(t *testing.T) {
	contextDir := t.TempDir()
	if err := buildcontext.WriteContext(contextDir); err != nil {
		t.Fatal(err)
	}
	path, err := materializeEmbeddedRecipeDockerfile(contextDir, "deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(contents)
	if !strings.Contains(dockerfile, filepath.Join("images", "php-nginx", "Dockerfile")) && !strings.Contains(dockerfile, "docker.io/library/php:8.5-fpm-trixie@sha256:") {
		t.Fatal("composed Dockerfile must include pinned upstream bases")
	}
	if strings.Count(dockerfile, "# syntax=docker/dockerfile:") != 1 {
		t.Fatal("composed Dockerfile must keep a single syntax directive")
	}
}
