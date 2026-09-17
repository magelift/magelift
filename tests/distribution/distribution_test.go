// Package distribution proves publication with GOWORK=off: an SDK consumer
// builds against a file proxy, and the real provider module resolves its
// full graph and compiles. Magelift modules resolve only from the file
// proxy (synthetic versions staged from the working tree); third-party
// modules resolve upstream for the provider build.
package distribution

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

const (
	sdkModule      = "github.com/magelift/magelift/sdk"
	providerModule = "github.com/magelift/magelift/providers/gcp"
	rootModule     = "github.com/magelift/magelift"
	proofVersion   = "v0.7.0-distproof"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
}

// proxyEnv stages a hermetic-resolution Go environment: GOWORK=off,
// GOPROXY pointed at dir/proxy (plus upstream for third-party when
// allowed), sumdb off, local toolchain. Magelift modules resolve only
// from the file proxy. Build and module caches stay shared: they affect
// speed, never resolution, and keep provider compiles affordable.
func proxyEnv(t *testing.T, dir string, upstream bool) (proxy string, env []string) {
	t.Helper()
	proxy = filepath.Join(dir, "proxy")
	if err := os.MkdirAll(proxy, 0o755); err != nil {
		t.Fatal(err)
	}
	proxyURL := "file://" + filepath.ToSlash(proxy)
	if upstream {
		proxyURL += ",https://proxy.golang.org"
	}
	base := os.Environ()
	overrides := map[string]string{
		"GOWORK":      "off",
		"GOPROXY":     proxyURL,
		"GOSUMDB":     "off",
		"GOTOOLCHAIN": "local",
		"GOFLAGS":     "-mod=readonly",
	}
	env = []string{}
	for _, entry := range base {
		name := entry
		if index := strings.IndexByte(entry, '='); index >= 0 {
			name = entry[:index]
		}
		if _, ok := overrides[name]; ok {
			continue
		}
		env = append(env, entry)
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return proxy, env
}

func publishModule(t *testing.T, proxy, modulePath, version, srcDir string) {
	t.Helper()
	escaped, err := module.EscapePath(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	escapedVersion, err := module.EscapeVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	atDir := filepath.Join(proxy, filepath.FromSlash(escaped), "@v")
	if err := os.MkdirAll(atDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(atDir, escapedVersion+".info"), []byte(`{"Version":"`+version+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// The zip and the .mod come from the same source tree, so a published
	// module can never carry divergent manifests.
	var archive bytes.Buffer
	if err := modzip.CreateFromDir(&archive, module.Version{Path: modulePath, Version: version}, srcDir); err != nil {
		t.Fatalf("zip %s: %v", modulePath, err)
	}
	if err := os.WriteFile(filepath.Join(atDir, escapedVersion+".zip"), archive.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	goMod, err := os.ReadFile(filepath.Join(srcDir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(atDir, escapedVersion+".mod"), goMod, 0o644); err != nil {
		t.Fatal(err)
	}
}

// assertProxyCoherent verifies the served .mod matches the go.mod inside
// the served zip: identical manifest content on both paths.
func assertProxyCoherent(t *testing.T, proxy, modulePath, version string) {
	t.Helper()
	escaped, err := module.EscapePath(modulePath)
	if err != nil {
		t.Fatal(err)
	}
	escapedVersion, err := module.EscapeVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	atDir := filepath.Join(proxy, filepath.FromSlash(escaped), "@v")
	servedMod, err := os.ReadFile(filepath.Join(atDir, escapedVersion+".mod"))
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(filepath.Join(atDir, escapedVersion+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, file := range archive.File {
		if !strings.HasSuffix(file.Name, "/go.mod") {
			continue
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		zipped, err := io.ReadAll(opened)
		opened.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(zipped) != string(servedMod) {
			t.Fatal("served .mod differs from the zipped go.mod")
		}
		return
	}
	t.Fatal("zipped go.mod not found")
}

func runGo(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	command := exec.Command("go", args...)
	command.Dir = dir
	command.Env = env
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("go %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return stdout.String()
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	copyDirSkipping(t, src, dst, map[string]bool{})
}

func copyDirSkipping(t *testing.T, src, dst string, skip map[string]bool) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if skip[entry.Name()] {
			continue
		}
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			copyDirSkipping(t, from, to, skip)
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		body, err := os.ReadFile(from)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(to, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSDKConsumerBuildsOutsideWorkspace(t *testing.T) {
	root := repositoryRoot(t)
	work := t.TempDir()
	proxy, env := proxyEnv(t, work, false)
	publishModule(t, proxy, sdkModule, proofVersion, filepath.Join(root, "sdk"))
	assertProxyCoherent(t, proxy, sdkModule, proofVersion)

	consumer := filepath.Join(work, "consumer")
	if err := os.MkdirAll(consumer, 0o755); err != nil {
		t.Fatal(err)
	}
	goMod := "module distproofconsumer\n\ngo 1.27.0\n\nrequire " + sdkModule + " " + proofVersion + "\n"
	if err := os.WriteFile(filepath.Join(consumer, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	main := "package main\n\nimport (\n\t\"fmt\"\n\n\t\"" + sdkModule + "\"\n)\n\nfunc main() { fmt.Println(sdk.HandshakeProtocolVersion) }\n"
	if err := os.WriteFile(filepath.Join(consumer, "main.go"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	// Real consumer flow: tidy writes go.sum from the proxy, then the
	// locked build runs -mod=readonly.
	tidyEnv := append([]string{}, env...)
	for i, entry := range tidyEnv {
		if strings.HasPrefix(entry, "GOFLAGS=") {
			tidyEnv[i] = "GOFLAGS=-mod=mod"
		}
	}
	runGo(t, consumer, tidyEnv, "mod", "tidy")
	listed := runGo(t, consumer, env, "list", "-m", sdkModule)
	if !strings.Contains(listed, proofVersion) {
		t.Fatalf("go list -m = %q, want %s", listed, proofVersion)
	}
	runGo(t, consumer, env, "build", "-o", filepath.Join(consumer, "consumer"), ".")
	command := exec.Command(filepath.Join(consumer, "consumer"))
	command.Env = env
	output, err := command.Output()
	if err != nil {
		t.Fatalf("run consumer: %v", err)
	}
	if strings.TrimSpace(string(output)) != "1" {
		t.Fatalf("consumer output = %q, want 1", output)
	}
}

func TestProviderBuildsOutsideWorkspace(t *testing.T) {
	root := repositoryRoot(t)
	work := t.TempDir()
	// Magelift modules resolve from the file proxy; third-party modules
	// resolve upstream (documented network use: the claim under test is
	// magelift-module resolution plus compilation, not offline vendoring).
	proxy, env := proxyEnv(t, work, true)
	publishModule(t, proxy, sdkModule, proofVersion, filepath.Join(root, "sdk"))
	assertProxyCoherent(t, proxy, sdkModule, proofVersion)

	// Post-tag shape: staged copies carry versioned magelift requires
	// (the tree keeps its pre-tag workspace form until the
	// require-bump commit). The provider under test is the main module
	// on disk; SDK and root resolve from the proxy.
	rootCopy := filepath.Join(work, "root")
	copyDirSkipping(t, root, rootCopy, map[string]bool{
		"providers": true, "sdk": true, ".git": true, "dist": true,
		".venv": true, "node_modules": true, "website": true,
	})
	appendRequire(t, filepath.Join(rootCopy, "go.mod"), sdkModule, proofVersion)
	publishModule(t, proxy, rootModule, proofVersion, rootCopy)
	assertProxyCoherent(t, proxy, rootModule, proofVersion)

	copied := filepath.Join(work, "provider")
	copyDir(t, filepath.Join(root, "providers", "gcp"), copied)
	appendRequire(t, filepath.Join(copied, "go.mod"), sdkModule, proofVersion)
	appendRequire(t, filepath.Join(copied, "go.mod"), rootModule, proofVersion)

	// Resolve the full graph, then compile the real provider: every
	// package including the plugin binary. GOWORK=off throughout.
	tidyEnv := append([]string{}, env...)
	for i, entry := range tidyEnv {
		if strings.HasPrefix(entry, "GOFLAGS=") {
			tidyEnv[i] = "GOFLAGS=-mod=mod"
		}
	}
	runGo(t, copied, tidyEnv, "mod", "tidy")
	listed := runGo(t, copied, env, "list", "-m", "all")
	for _, want := range []string{sdkModule + " " + proofVersion, rootModule + " " + proofVersion} {
		if !strings.Contains(listed, want) {
			t.Fatalf("module graph lacks %s", want)
		}
	}
	runGo(t, copied, env, "build", "./...")
}

func appendRequire(t *testing.T, goModPath, modulePath, version string) {
	t.Helper()
	goMod, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatal(err)
	}
	goMod = append(goMod, "\nrequire "+modulePath+" "+version+"\n"...)
	if err := os.WriteFile(goModPath, goMod, 0o644); err != nil {
		t.Fatal(err)
	}
}
