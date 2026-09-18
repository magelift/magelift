package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/sumdb/dirhash"
)

func TestStagePublishesThreeCoherentModules(t *testing.T) {
	root := filepath.Join("..", "..")
	proxy := t.TempDir()
	version := "v0.7.0-modproxy-test"
	if err := run(root, "HEAD", version, proxy); err != nil {
		t.Fatal(err)
	}
	modules := map[string]string{
		"github.com/magelift/magelift":               "github.com/magelift/magelift",
		"github.com/magelift/magelift/sdk":           "github.com/magelift/magelift/sdk",
		"github.com/magelift/magelift/providers/gcp": "github.com/magelift/magelift/providers/gcp",
	}
	for module, escaped := range modules {
		atDir := filepath.Join(proxy, filepath.FromSlash(escaped), "@v")
		for _, file := range []string{version + ".info", version + ".mod", version + ".zip"} {
			if _, err := os.Stat(filepath.Join(atDir, file)); err != nil {
				t.Fatalf("%s %s: %v", module, file, err)
			}
		}
		servedMod, err := os.ReadFile(filepath.Join(atDir, version+".mod"))
		if err != nil {
			t.Fatal(err)
		}
		zippedMod := zippedGoMod(t, filepath.Join(atDir, version+".zip"), module+"@"+version+"/go.mod")
		if string(servedMod) != zippedMod {
			t.Fatalf("%s: served .mod diverges from zipped go.mod", module)
		}
	}
	rootZip := filepath.Join(proxy, "github.com/magelift/magelift", "@v", version+".zip")
	assertNoNestedModules(t, rootZip, version)
}

func zippedGoMod(t *testing.T, archive, name string) string {
	t.Helper()
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name != name {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer opened.Close()
		contents, err := io.ReadAll(opened)
		if err != nil {
			t.Fatal(err)
		}
		return string(contents)
	}
	t.Fatalf("%s missing from %s", name, archive)
	return ""
}

func assertNoNestedModules(t *testing.T, archive, version string) {
	t.Helper()
	prefix := "github.com/magelift/magelift@" + version + "/"
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	modCount := 0
	for _, file := range reader.File {
		relative := strings.TrimPrefix(file.Name, prefix)
		top := relative
		if index := strings.IndexByte(relative, '/'); index >= 0 {
			top = relative[:index]
		}
		if top == "providers" || top == "sdk" {
			t.Fatalf("root zip contains nested module path %q", file.Name)
		}
		if strings.HasSuffix(file.Name, "/go.mod") || file.Name == prefix+"go.mod" {
			modCount++
		}
	}
	if modCount != 1 {
		t.Fatalf("root zip holds %d go.mod files, want exactly 1", modCount)
	}
}

func TestRunRequiresFlags(t *testing.T) {
	if err := run("", "HEAD", "v1.0.0", t.TempDir()); err == nil {
		t.Fatal("run accepted an empty root")
	}
}

func TestStageMatchesArchiveContent(t *testing.T) {
	root := filepath.Join("..", "..")
	proxy := t.TempDir()
	version := "v0.7.0-modproxy-test"
	if err := run(root, "HEAD", version, proxy); err != nil {
		t.Fatal(err)
	}
	// Tracked content stages verbatim: website/ must survive (the proxy
	// only prunes nested modules, and an earlier prune list wrongly
	// dropped it).
	zipPath := filepath.Join(proxy, "github.com/magelift/magelift", "@v", version+".zip")
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	prefix := "github.com/magelift/magelift@" + version + "/"
	sawWebsite := false
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, prefix+"website/") {
			sawWebsite = true
			break
		}
	}
	if !sawWebsite {
		t.Fatal("root zip lacks tracked website/ content")
	}
}

func TestNestedModuleInheritsRootLicense(t *testing.T) {
	repo := t.TempDir()
	writeModuleTree(t, repo)
	license := []byte("license-root-only\n")
	if err := os.WriteFile(filepath.Join(repo, "LICENSE"), license, 0o644); err != nil {
		t.Fatal(err)
	}
	commitTree(t, repo)

	proxy := t.TempDir()
	version := "v0.0.0-license"
	if err := run(repo, "HEAD", version, proxy); err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(proxy, "github.com/magelift/magelift/providers/gcp", "@v", version+".zip")
	got := zippedGoMod(t, zipPath, "github.com/magelift/magelift/providers/gcp@"+version+"/LICENSE")
	if got != string(license) {
		t.Fatalf("nested module license = %q, want root LICENSE", got)
	}
}

func TestStageMatchesPublishedRC11Checksums(t *testing.T) {
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	const version = "v0.1.0-alpha.1-rc.11"
	if !tagExists(t, repo, version) {
		t.Skipf("local tag %s missing", version)
	}
	cache := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.Walk(cache, func(path string, _ os.FileInfo, err error) error {
			if err == nil {
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
	})
	published := map[string]string{
		"github.com/magelift/magelift":               "",
		"github.com/magelift/magelift/sdk":           "",
		"github.com/magelift/magelift/providers/gcp": "",
	}
	for module := range published {
		sum, downErr := publishedModuleSum(t, cache, module+"@"+version)
		if downErr != nil {
			if os.Getenv("GITHUB_ACTIONS") != "" {
				t.Fatalf("download %s: %v", module, downErr)
			}
			t.Skipf("published module unavailable: %v", downErr)
		}
		published[module] = sum
	}
	proxy := t.TempDir()
	if err := run(repo, version, version, proxy); err != nil {
		t.Fatal(err)
	}
	for module, want := range published {
		zipPath := filepath.Join(proxy, filepath.FromSlash(module), "@v", version+".zip")
		got, hashErr := dirhash.HashZip(zipPath, dirhash.Hash1)
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		if got != want {
			t.Errorf("%s staged %s, published %s", module, got, want)
		}
	}
}

func writeModuleTree(t *testing.T, repo string) {
	t.Helper()
	for _, spec := range []struct{ dir, body string }{
		{".", "module github.com/magelift/magelift\n\ngo 1.27.0\n"},
		{"sdk", "module github.com/magelift/magelift/sdk\n\ngo 1.27.0\n"},
		{"providers/gcp", "module github.com/magelift/magelift/providers/gcp\n\ngo 1.27.0\n"},
	} {
		dir := filepath.Join(repo, spec.dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(spec.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func commitTree(t *testing.T, repo string) {
	t.Helper()
	commands := [][]string{
		{"git", "init", "-q"},
		{"git", "add", "."},
		{"git", "-c", "user.name=modproxy-test", "-c", "user.email=modproxy-test@example.com", "commit", "-qm", "tree"},
	}
	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

func tagExists(t *testing.T, repo, tag string) bool {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "rev-parse", "-q", "--verify", tag+"^{commit}")
	return cmd.Run() == nil
}

func publishedModuleSum(t *testing.T, cache, spec string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", "mod", "download", "-json", spec)
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOMODCACHE="+cache)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	var result struct {
		Sum string
		Err string
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return "", err
	}
	if result.Err != "" {
		return "", fmt.Errorf("%s", result.Err)
	}
	if result.Sum == "" {
		return "", fmt.Errorf("download %s: empty sum", spec)
	}
	return result.Sum, nil
}
