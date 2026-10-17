package providerhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func verifierTestPin(t *testing.T, body []byte) {
	t.Helper()
	key := runtime.GOOS + "/" + runtime.GOARCH
	original, ok := verifierPins[key]
	if !ok {
		t.Skipf("no verifier pin for %s", key)
	}
	sum := sha256.Sum256(body)
	verifierPins[key] = hex.EncodeToString(sum[:])
	t.Cleanup(func() { verifierPins[key] = original })
}

func TestEnsureVerifierCachesAndSkipsRedownload(t *testing.T) {
	fixture := []byte("fixture-cosign")
	verifierTestPin(t, fixture)
	var hits atomic.Int32
	file := "cosign-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		file += ".exe"
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if !strings.HasSuffix(r.URL.Path, "/"+VerifierVersion+"/"+file) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write(fixture)
	}))
	t.Cleanup(server.Close)

	cache := t.TempDir()
	installer := &VerifierInstaller{HTTP: server.Client(), BaseURL: server.URL, CacheDir: cache}
	path, err := installer.Ensure(context.Background())
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !strings.HasPrefix(path, filepath.Join(cache, "_verifier")) {
		t.Fatalf("path = %q", path)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("verifier is not executable: %v", err)
	}
	second, err := installer.Ensure(context.Background())
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if second != path {
		t.Fatalf("second path = %q, want %q", second, path)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("cache hit used the network: %d requests", got)
	}
	if cached, err := CachedVerifierPath(cache); err != nil || cached != path {
		t.Fatalf("cached = %q, err = %v", cached, err)
	}
}

func TestEnsureVerifierRefusesChecksumMismatch(t *testing.T) {
	if _, ok := verifierPins[runtime.GOOS+"/"+runtime.GOARCH]; !ok {
		t.Skipf("no verifier pin for this platform")
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("tampered"))
	}))
	t.Cleanup(server.Close)

	cache := t.TempDir()
	installer := &VerifierInstaller{HTTP: server.Client(), BaseURL: server.URL, CacheDir: cache}
	if _, err := installer.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
	if _, err := CachedVerifierPath(cache); err == nil {
		t.Fatal("tampered verifier is treated as cached")
	}
}

func TestCachedVerifierPathMissesEmptyCache(t *testing.T) {
	if _, err := CachedVerifierPath(t.TempDir()); err == nil {
		t.Fatal("empty cache reported a verifier")
	}
}

func TestMaxVerifierBytesFitsPinnedCosignRelease(t *testing.T) {
	// GitHub asset sizes for Cosign VerifierVersion, fetched 2026-09-18.
	const (
		cosignLinuxAmd64   = 141178250
		cosignWindowsAmd64 = 198819314
	)
	if maxVerifierBytes <= cosignLinuxAmd64 {
		t.Fatalf("maxVerifierBytes = %d; Cosign %s linux/amd64 is %d", maxVerifierBytes, VerifierVersion, cosignLinuxAmd64)
	}
	if maxVerifierBytes <= cosignWindowsAmd64 {
		t.Fatalf("maxVerifierBytes = %d; Cosign %s windows/amd64 is %d", maxVerifierBytes, VerifierVersion, cosignWindowsAmd64)
	}
}

func TestEnsureVerifierRejectsCorruptCacheWithoutNetwork(t *testing.T) {
	fixture := []byte("verified fixture")
	verifierTestPin(t, fixture)
	cache := t.TempDir()
	path, err := VerifierPath(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0o755); err != nil {
		t.Fatal(err)
	}
	var hits atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits.Add(1); w.Write(fixture) }))
	defer server.Close()
	installer := &VerifierInstaller{HTTP: server.Client(), BaseURL: server.URL, CacheDir: cache}
	if _, err := installer.Ensure(context.Background()); err == nil || !strings.Contains(err.Error(), "cached verifier is corrupt") {
		t.Fatalf("error = %v", err)
	}
	if hits.Load() != 0 {
		t.Fatal("corrupt cache triggered download")
	}
	installer.Repair = true
	if _, err := installer.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatal("explicit repair did not download")
	}
}

func TestVerifierLegacyCacheRemainsUsableOffline(t *testing.T) {
	fixture := []byte("legacy verified fixture")
	verifierTestPin(t, fixture)
	cache := t.TempDir()
	path, err := VerifierPath(cache)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, runtime.GOOS+"_"+runtime.GOARCH) {
		t.Fatalf("platform absent from %s", path)
	}
	legacy := filepath.Join(filepath.Dir(filepath.Dir(path)), filepath.Base(path))
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := (&VerifierInstaller{CacheDir: cache, BaseURL: "https://invalid.invalid"}).Ensure(context.Background())
	if err != nil || got != legacy {
		t.Fatalf("legacy = %q, %v", got, err)
	}
	if got, err := CachedVerifierPath(cache); err != nil || got != legacy {
		t.Fatalf("cached = %q, %v", got, err)
	}
}
