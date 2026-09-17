package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStagePublishesThreeCoherentModules(t *testing.T) {
	root := filepath.Join("..", "..")
	proxy := t.TempDir()
	version := "v0.7.0-modproxy-test"
	if err := run(root, version, proxy); err != nil {
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
	if err := run("", "v1.0.0", t.TempDir()); err == nil {
		t.Fatal("run accepted an empty root")
	}
}
