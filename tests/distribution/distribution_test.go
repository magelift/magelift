// Package distribution proves the publication mechanics before any tag: an
// SDK consumer and the provider module resolve through a module proxy with
// GOWORK=off, using synthetic versions served from a file proxy staged from
// the working tree. No network beyond file:// is used.
package distribution

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

const (
	sdkModule      = "github.com/magelift/magelift/sdk"
	providerModule = "github.com/magelift/magelift/providers/gcp"
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

// proxyEnv stages a hermetic Go environment: GOWORK=off, GOPROXY pointed at
// dir/proxy, and scratch cache, GOPATH, and HOME so the proof never touches
// the developer's module cache. The read-only module cache needs a
// permission-restoring cleanup for t.TempDir removal.
func proxyEnv(t *testing.T, dir string) (proxy string, env []string) {
	t.Helper()
	proxy = filepath.Join(dir, "proxy")
	home := filepath.Join(dir, "home")
	cache := filepath.Join(dir, "gocache")
	gopath := filepath.Join(dir, "gopath")
	for _, path := range []string{proxy, home, cache, gopath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		filepath.WalkDir(dir, func(path string, _ fs.DirEntry, _ error) error {
			_ = os.Chmod(path, 0o700)
			return nil
		})
	})
	base := os.Environ()
	overrides := map[string]string{
		"GOWORK":      "off",
		"GOPROXY":     "file://" + filepath.ToSlash(proxy),
		"GOSUMDB":     "off",
		"GOTOOLCHAIN": "local",
		"GOCACHE":     cache,
		"GOPATH":      gopath,
		"HOME":        home,
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

func publishModule(t *testing.T, proxy, modulePath, version, srcDir string, rewriteGoMod func([]byte) []byte) {
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
	var archive bytes.Buffer
	if err := zip.CreateFromDir(&archive, module.Version{Path: modulePath, Version: version}, srcDir); err != nil {
		t.Fatalf("zip %s: %v", modulePath, err)
	}
	if err := os.WriteFile(filepath.Join(atDir, escapedVersion+".zip"), archive.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	goMod, err := os.ReadFile(filepath.Join(srcDir, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if rewriteGoMod != nil {
		goMod = rewriteGoMod(goMod)
	}
	if err := os.WriteFile(filepath.Join(atDir, escapedVersion+".mod"), goMod, 0o644); err != nil {
		t.Fatal(err)
	}
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
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		from := filepath.Join(src, entry.Name())
		to := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			copyDir(t, from, to)
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
	proxy, env := proxyEnv(t, work)
	publishModule(t, proxy, sdkModule, proofVersion, filepath.Join(root, "sdk"), nil)

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

func TestProviderModuleIsProxyServable(t *testing.T) {
	root := repositoryRoot(t)
	work := t.TempDir()
	proxy, env := proxyEnv(t, work)

	// Post-tag shape: the provider go.mod carries a versioned SDK require
	// (applied to a temp copy; the tree keeps its pre-tag workspace form).
	copied := filepath.Join(work, "provider")
	copyDir(t, filepath.Join(root, "providers", "gcp"), copied)
	publishModule(t, proxy, providerModule, proofVersion, copied, func(goMod []byte) []byte {
		return append(goMod, "\nrequire "+sdkModule+" "+proofVersion+"\n"...)
	})

	empty := filepath.Join(work, "empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	downloaded := runGo(t, empty, env, "mod", "download", "-json", providerModule+"@"+proofVersion)
	var fetched struct {
		Path    string
		Version string
		GoMod   string
	}
	if err := json.Unmarshal([]byte(downloaded), &fetched); err != nil {
		t.Fatal(err)
	}
	if fetched.Path != providerModule || fetched.Version != proofVersion {
		t.Fatalf("downloaded = %+v", fetched)
	}
	served, err := os.ReadFile(fetched.GoMod)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(served), "require "+sdkModule+" "+proofVersion) {
		t.Fatalf("served provider go.mod lacks the SDK require:\n%s", served)
	}
}
