package providerhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func acquireTestLock(version, digest, url, bundle string) string {
	if bundle == "" {
		bundle = url + ".sigstore.json"
	}
	return `{"schemaVersion":1,"sdkAPIVersion":"v1","providers":{"gcp":{"name":"magelift-provider-gcp","version":"` + version + `","protocol":"magelift-v1","digest":"` + digest + `","url":"` + url + `","cosign":{"identity":"` + FirstPartyIdentityPrefix + version + `","issuer":"` + FirstPartyIssuer + `","bundle":"` + bundle + `"}}}}`
}

func TestAcquireRejectsNonGCPProvider(t *testing.T) {
	t.Parallel()
	_, err := Acquire(context.Background(), "aws", "v1.0.0", AcquireOptions{})
	if err == nil || !strings.Contains(err.Error(), "only gcp") {
		t.Fatalf("err = %v, want non-gcp refusal", err)
	}
}

func TestAcquireMalformedProjectLockFailsClosed(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	if err := os.WriteFile("magelift.providers.lock", []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Acquire(context.Background(), "gcp", "v1.0.0", AcquireOptions{})
	if err == nil || errors.Is(err, ErrNoLockfile) {
		t.Fatalf("err = %v, want parse failure", err)
	}
}

func TestAcquireRejectsDevCLIVersionWithoutLock(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	_, err := Acquire(context.Background(), "gcp", "dev", AcquireOptions{})
	if err == nil || !strings.Contains(err.Error(), "not a release tag") {
		t.Fatalf("err = %v, want dev refusal", err)
	}
}

func TestAcquireBootstrapsAndCachesReleaseLock(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	cache := t.TempDir()
	binDir := t.TempDir()
	version := "v0.0.0-test.1"
	platform := runtime.GOOS + "_" + runtime.GOARCH

	binary := []byte("acquire-bootstrap-binary")
	sum := sha256.Sum256(binary)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	var fetchHits atomic.Int32
	var baseURL string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/magelift.providers.lock."+platform):
			fetchHits.Add(1)
			w.Write([]byte(acquireTestLock(version, digest, baseURL+"/magelift-provider-gcp", baseURL+"/magelift-provider-gcp.sigstore.json")))
		case r.URL.Path == "/magelift-provider-gcp":
			w.Write(binary)
		case r.URL.Path == "/magelift-provider-gcp.sigstore.json":
			w.Write([]byte("bundle"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	baseURL = server.URL

	opts := AcquireOptions{
		Resolve:        ResolveOptions{ExecutablePath: filepath.Join(binDir, "magelift"), CacheDir: cache},
		ReleaseBaseURL: server.URL,
		HTTP:           server.Client(),
		Verifier:       stubVerifier{},
	}
	loaded, err := Acquire(context.Background(), "gcp", version, opts)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if loaded.Binary == "" || loaded.Provider != "gcp" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if fetchHits.Load() != 1 {
		t.Fatalf("fetch hits = %d, want one lock download", fetchHits.Load())
	}
	lockPath, err := cachedReleaseLockPath(cache, version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("cached release lock missing: %v", err)
	}
	if _, err := os.Stat("magelift.providers.lock"); !os.IsNotExist(err) {
		t.Fatal("project lock must not be written")
	}

	fetchHits.Store(0)
	server.Close() // The second acquisition must work with the server unavailable.
	loaded2, err := Acquire(context.Background(), "gcp", version, opts)
	if err != nil {
		t.Fatalf("second Acquire: %v", err)
	}
	if loaded2.Binary != loaded.Binary {
		t.Fatalf("second binary = %q, want %q", loaded2.Binary, loaded.Binary)
	}
	if fetchHits.Load() != 0 {
		t.Fatalf("second acquire fetched lock again: hits=%d", fetchHits.Load())
	}
}

func TestAcquireCorruptCachedReleaseLockFailsClosed(t *testing.T) {
	cache := t.TempDir()
	version := "v1.2.3"
	lockPath, err := cachedReleaseLockPath(cache, version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lockPath, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	var fetchHits atomic.Int32
	opts := AcquireOptions{
		Resolve: ResolveOptions{CacheDir: cache},
		FetchLock: func(context.Context, string) (Lockfile, error) {
			fetchHits.Add(1)
			return Lockfile{}, errors.New("fetch should not run")
		},
		Verifier: stubVerifier{},
	}
	_, err = Acquire(context.Background(), "gcp", version, opts)
	if err == nil || fetchHits.Load() != 0 {
		t.Fatalf("err = %v, fetch hits = %d, want corrupt cache failure without fetch", err, fetchHits.Load())
	}
}

func TestAcquireDownloadsProviderOnCacheMiss(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	cache := t.TempDir()
	binary := []byte("cached-on-miss")
	sum := sha256.Sum256(binary)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	version := "v0.0.0-test.2"

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/magelift-provider-gcp":
			w.Write(binary)
		case "/magelift-provider-gcp.sigstore.json":
			w.Write([]byte("bundle"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	lock := acquireTestLock(version, digest, server.URL+"/magelift-provider-gcp", server.URL+"/magelift-provider-gcp.sigstore.json")
	if err := os.WriteFile("magelift.providers.lock", []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Acquire(context.Background(), "gcp", "v9.9.9", AcquireOptions{
		Resolve:  ResolveOptions{CacheDir: cache},
		HTTP:     server.Client(),
		Verifier: stubVerifier{},
	})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if loaded.Binary == "" {
		t.Fatal("expected binary path")
	}
	if _, err := os.Stat(loaded.Binary); err != nil {
		t.Fatalf("binary missing: %v", err)
	}
}

func TestAcquireProjectLockPrecedenceOverBootstrap(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	cache := t.TempDir()
	version := "v0.0.0-test.3"
	digest, binary := resolveTestCache(t, cache, version, []byte("project-pin"))
	resolveTestLock(t, project, version, digest)

	var fetchHits atomic.Int32
	opts := AcquireOptions{
		Resolve: ResolveOptions{CacheDir: cache},
		FetchLock: func(context.Context, string) (Lockfile, error) {
			fetchHits.Add(1)
			return Lockfile{}, errors.New("bootstrap must not run")
		},
		Verifier: stubVerifier{},
	}
	loaded, err := Acquire(context.Background(), "gcp", "v9.9.9", opts)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if loaded.Binary != binary {
		t.Fatalf("binary = %q, want %q", loaded.Binary, binary)
	}
	if fetchHits.Load() != 0 {
		t.Fatalf("bootstrap fetch ran with project lock present")
	}
}

func TestAcquireCorruptProviderDoesNotDownloadReplacement(t *testing.T) {
	for _, signatureFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(signatureFailure), func(t *testing.T) {
			project := t.TempDir()
			t.Chdir(project)
			cache := t.TempDir()
			version := "v0.0.0-test.3"
			digest, binary := resolveTestCache(t, cache, version, []byte("project-pin"))
			resolveTestLock(t, project, version, digest)
			verifier := stubVerifier{}
			if signatureFailure {
				verifier.err = errors.New("fixture signature failure")
			} else if err := os.WriteFile(binary, []byte("corrupt"), 0o755); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			_, err := Acquire(context.Background(), "gcp", "v9.9.9", AcquireOptions{
				Resolve: ResolveOptions{CacheDir: cache}, Verifier: verifier,
				FetchLock: func(context.Context, string) (Lockfile, error) {
					calls.Add(1)
					return Lockfile{}, errors.New("unexpected fetch")
				},
			})
			if err == nil || calls.Load() != 0 {
				t.Fatalf("error=%v, release fetches=%d", err, calls.Load())
			}
			body, readErr := os.ReadFile(binary)
			if readErr != nil || (!signatureFailure && string(body) != "corrupt") {
				t.Fatalf("cache was changed: %q, %v", body, readErr)
			}
		})
	}
}

func TestAcquireUnknownLockedProviderDoesNotInstallVerifier(t *testing.T) {
	project := t.TempDir()
	t.Chdir(project)
	resolveTestLock(t, project, "v1.0.0", "sha256:"+strings.Repeat("a", 64))
	path := "magelift.providers.lock"
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(body), `"gcp":`, `"other":`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	called := false
	_, err = Acquire(context.Background(), "gcp", "v1.0.0", AcquireOptions{
		Resolve:        ResolveOptions{CacheDir: t.TempDir()},
		EnsureVerifier: func(context.Context, string) (string, error) { called = true; return "", nil },
	})
	if !errors.Is(err, ErrUnknownProvider) || called {
		t.Fatalf("error=%v, verifier acquired=%v", err, called)
	}
}
