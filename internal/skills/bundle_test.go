package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBundledManifestIsCompleteAndStable(t *testing.T) {
	manifest, err := BundledManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest) != 5 {
		t.Fatalf("bundled skill count = %d, want 5 user skills", len(manifest))
	}
	for _, name := range []string{"magelift-configure", "magelift-dependencies", "magelift-local-runtime", "magelift-migrate", "magelift-operate"} {
		found := false
		for _, entry := range manifest {
			if entry.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("user skill %q is missing from the bundle", name)
		}
	}
	for _, entry := range manifest {
		if entry.Name == "" || entry.Description == "" || entry.Version == "" {
			t.Fatalf("incomplete manifest entry: %#v", entry)
		}
		if len(entry.Digest) != 64 {
			t.Fatalf("digest for %q = %q", entry.Name, entry.Digest)
		}
	}
}

func TestReleaseManifestMatchesEmbeddedBundle(t *testing.T) {
	want, err := BundledManifest()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "agents", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got []ManifestEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("release manifest differs from embedded bundle:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestInstallCopyAndVerifyDetectsDrift(t *testing.T) {
	project := t.TempDir()
	installed, err := Install(InstallOptions{
		ProjectRoot: project,
		HomeRoot:    t.TempDir(),
		Agent:       "claude",
		Scope:       "project",
		Mode:        "copy",
		Skills:      []string{"magelift-operate"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 1 || installed[0].Mode != "copy" {
		t.Fatalf("installed = %#v", installed)
	}
	report, err := Verify(InstallOptions{ProjectRoot: project, HomeRoot: t.TempDir(), Agent: "claude", Scope: "project", Skills: []string{"magelift-operate"}})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Clean {
		t.Fatalf("fresh installation is not clean: %#v", report)
	}
	path := filepath.Join(project, ".claude", "skills", "magelift-operate", "SKILL.md")
	if err := os.WriteFile(path, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err = Verify(InstallOptions{ProjectRoot: project, HomeRoot: t.TempDir(), Agent: "claude", Scope: "project", Skills: []string{"magelift-operate"}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Clean || report.Items[0].Status != "drift" {
		t.Fatalf("drift was not reported: %#v", report)
	}
}

func TestInstallRefusesUnrelatedDestination(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".agents", "skills", "magelift-operate")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("someone else's skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Install(InstallOptions{ProjectRoot: project, Agent: "generic", Scope: "project", Mode: "copy", Skills: []string{"magelift-operate"}})
	if err == nil || !strings.Contains(err.Error(), "--replace") {
		t.Fatalf("unexpected conflict result: %v", err)
	}
	if _, err := Install(InstallOptions{ProjectRoot: project, Agent: "generic", Scope: "project", Mode: "copy", Replace: true, Skills: []string{"magelift-operate"}}); err != nil {
		t.Fatal(err)
	}
}

func TestInstallSymlinkUsesVerifiedLocalSource(t *testing.T) {
	project := t.TempDir()
	sourceRoot := FindSourceRoot(project)
	if sourceRoot != "" {
		t.Fatal("temporary project unexpectedly found a MageLift source root")
	}
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot = FindSourceRoot(working)
	if sourceRoot == "" {
		t.Fatal("repository source root was not found")
	}
	installed, err := Install(InstallOptions{
		ProjectRoot: project,
		SourceRoot:  sourceRoot,
		Agent:       "codex",
		Scope:       "project",
		Mode:        "symlink",
		Skills:      []string{"magelift-configure"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if installed[0].Mode != "symlink" {
		t.Fatalf("mode = %q", installed[0].Mode)
	}
	info, err := os.Lstat(installed[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("installation is not a symlink")
	}
	report, err := Verify(InstallOptions{ProjectRoot: project, Agent: "codex", Scope: "project", Skills: []string{"magelift-configure"}})
	if err != nil || !report.Clean {
		t.Fatalf("symlink verification failed: %v %#v", err, report)
	}
}

func TestInstallSupportsAgentAndScopePaths(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	tests := []struct {
		name  string
		agent string
		scope string
		want  string
	}{
		{name: "codex project", agent: "codex", scope: "project", want: filepath.Join(".agents", "skills")},
		{name: "codex global", agent: "codex", scope: "global", want: filepath.Join(".codex", "skills")},
		{name: "claude code global", agent: "claude-code", scope: "global", want: filepath.Join(".claude", "skills")},
		{name: "cursor project", agent: "cursor", scope: "project", want: filepath.Join(".cursor", "skills")},
		{name: "generic global", agent: "generic", scope: "global", want: filepath.Join(".agents", "skills")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := Install(InstallOptions{
				ProjectRoot: project,
				HomeRoot:    home,
				Agent:       tt.agent,
				Scope:       tt.scope,
				Mode:        "copy",
				Skills:      []string{"magelift-operate"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(results) != 1 || results[0].Path != filepath.Join(map[string]string{"project": project, "global": home}[tt.scope], tt.want, "magelift-operate") {
				t.Fatalf("installation = %#v", results)
			}
			report, err := Verify(InstallOptions{
				ProjectRoot: project,
				HomeRoot:    home,
				Agent:       tt.agent,
				Scope:       tt.scope,
				Skills:      []string{"magelift-operate"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if !report.Clean {
				t.Fatalf("installation is not clean: %#v", report)
			}
		})
	}
}

func TestSkillsCLIArgsAreShellSafe(t *testing.T) {
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	sourceRoot := FindSourceRoot(working)
	args, err := SkillsCLIArgs(sourceRoot, "claude", "global", "copy", []string{"magelift-operate"})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "sh -c") || !strings.Contains(joined, "--global") || !strings.Contains(joined, "--copy") {
		t.Fatalf("unexpected args: %#v", args)
	}
}
