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
