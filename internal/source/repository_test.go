package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitInspectorReturnsCleanRevisionAndRoot(t *testing.T) {
	root := cleanRepository(t)
	nested := filepath.Join(root, "app", "code")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "Module.php"), []byte("<?php\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	repository, err := (GitInspector{}).Inspect(context.Background(), nested)
	if err == nil || !strings.Contains(err.Error(), "untracked changes") {
		t.Fatalf("untracked directory should fail inspection: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "app")); err != nil {
		t.Fatal(err)
	}
	repository, err = (GitInspector{}).Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if repository.Root != root || !revisionPattern.MatchString(repository.Revision) {
		t.Fatalf("unexpected repository: %#v", repository)
	}
}

func TestGitInspectorReturnsOptionalOriginURL(t *testing.T) {
	root := cleanRepository(t)
	runGit(t, root, "remote", "add", "origin", "https://github.com/acourtiol/shop.git")

	repository, err := (GitInspector{}).Inspect(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if repository.OriginURL != "https://github.com/acourtiol/shop.git" {
		t.Fatalf("unexpected origin URL: %q", repository.OriginURL)
	}
}

func TestGitInspectorRejectsTrackedAndUntrackedChanges(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
	}{
		{name: "tracked", path: "README.md"},
		{name: "untracked", path: "new.txt"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := cleanRepository(t)
			if err := os.WriteFile(filepath.Join(root, test.path), []byte("changed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := (GitInspector{}).Inspect(context.Background(), root)
			if err == nil || !strings.Contains(err.Error(), "tracked or untracked changes") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestGitInspectorRejectsNonRepository(t *testing.T) {
	_, err := (GitInspector{}).Inspect(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "locate Git repository") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func cleanRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "config", "user.name", "MageLift Tests")
	runGit(t, root, "config", "user.email", "tests@magelift.invalid")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "--quiet", "-m", "fixture")
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", directory}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
