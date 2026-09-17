package providerhost

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resolveTestLock(t *testing.T, dir, version, digest string) string {
	t.Helper()
	lock := `{"schemaVersion":1,"sdkAPIVersion":"v1","providers":{"gcp":{"name":"magelift-provider-gcp","version":"` + version + `","protocol":"magelift-v1","digest":"` + digest + `","cosign":{"identity":"` + FirstPartyIdentityPrefix + version + `","issuer":"` + FirstPartyIssuer + `","bundle":"bundle.json"}}}}`
	path := filepath.Join(dir, "magelift.providers.lock")
	if err := os.WriteFile(path, []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func resolveTestCache(t *testing.T, cache, version string, binary []byte) (string, string) {
	t.Helper()
	sum := sha256.Sum256(binary)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	paths, err := CachedArtifactPaths(cache, "gcp", version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Binary, binary, 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := CachedBundlePath(paths.Binary)
	if err := os.WriteFile(bundle, []byte("bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	return digest, paths.Binary
}

func TestResolveUsesCacheForProjectLock(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	cache := t.TempDir()
	digest, binary := resolveTestCache(t, cache, "v9", []byte("cached-binary"))
	resolveTestLock(t, project, "v9", digest)

	resolved, err := Resolve("gcp", ResolveOptions{CacheDir: cache})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !resolved.Cached || resolved.Binary != binary {
		t.Fatalf("resolved = %+v, want cached %q", resolved, binary)
	}
	if resolved.LockPath != "magelift.providers.lock" {
		t.Fatalf("lock = %q", resolved.LockPath)
	}
}

func TestResolveProjectLockControlsVersion(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	cache := t.TempDir()
	_, _ = resolveTestCache(t, cache, "v1", []byte("old-binary"))
	digest, binary := resolveTestCache(t, cache, "v9", []byte("new-binary"))
	resolveTestLock(t, project, "v9", digest)

	resolved, err := Resolve("gcp", ResolveOptions{CacheDir: cache})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Binary != binary || resolved.Artifact.Version != "v9" {
		t.Fatalf("resolved = %+v, want v9", resolved)
	}
}

func TestResolveUsesBesideCLIForBundledLock(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	binDir := t.TempDir()
	sum := sha256.Sum256([]byte("bundled"))
	resolveTestLock(t, binDir, "v1", "sha256:"+hex.EncodeToString(sum[:]))
	binary := filepath.Join(binDir, ExtractName("gcp"))
	if err := os.WriteFile(binary, []byte("bundled"), 0o755); err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve("gcp", ResolveOptions{ExecutablePath: filepath.Join(binDir, "magelift"), CacheDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Cached || resolved.Binary != binary {
		t.Fatalf("resolved = %+v, want beside-CLI", resolved)
	}
}

func TestResolveNamesInstallOnCacheMiss(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	resolveTestLock(t, project, "v9", "sha256:"+strings.Repeat("b", 64))

	_, err := Resolve("gcp", ResolveOptions{CacheDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "magelift providers install") {
		t.Fatalf("err = %v, want install hint", err)
	}
}

func TestResolveNamesBootstrapWithoutLock(t *testing.T) {
	t.Chdir(t.TempDir())
	exe := filepath.Join(t.TempDir(), "magelift")

	_, err := Resolve("gcp", ResolveOptions{ExecutablePath: exe, CacheDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "bootstrap one") {
		t.Fatalf("err = %v, want bootstrap hint", err)
	}
}

func TestResolveHonorsExplicitLock(t *testing.T) {
	t.Chdir(t.TempDir())
	cache := t.TempDir()
	digest, binary := resolveTestCache(t, cache, "v3", []byte("explicit-binary"))
	lockPath := resolveTestLock(t, t.TempDir(), "v3", digest)

	resolved, err := Resolve("gcp", ResolveOptions{LockPath: lockPath, CacheDir: cache})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !resolved.Cached || resolved.Binary != binary || resolved.LockPath != lockPath {
		t.Fatalf("resolved = %+v", resolved)
	}
}

func TestResolveRejectsForeignPublisher(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	lock := `{"schemaVersion":1,"sdkAPIVersion":"v1","providers":{"gcp":{"name":"magelift-provider-gcp","version":"v9","protocol":"magelift-v1","digest":"sha256:` + strings.Repeat("b", 64) + `","cosign":{"identity":"https://evil.example/wolf","issuer":"https://token.actions.githubusercontent.com","bundle":"bundle.json"}}}}`
	if err := os.WriteFile(filepath.Join(project, "magelift.providers.lock"), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolve("gcp", ResolveOptions{CacheDir: t.TempDir()}); err == nil {
		t.Fatal("foreign publisher was accepted")
	}
}