package providerhost

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/cosign"
)

type stubVerifier struct {
	calls *atomic.Int32
	err   error
}

func (s stubVerifier) VerifyBlob(_ context.Context, _, _ string, _ cosign.VerifyOptions) error {
	if s.calls != nil {
		s.calls.Add(1)
	}
	return s.err
}

func downloadTestServer(t *testing.T, binary, bundle []byte, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/magelift-provider-gcp", func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		w.Write(binary)
	})
	mux.HandleFunc("/magelift-provider-gcp.sigstore.json", func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			hits.Add(1)
		}
		w.Write(bundle)
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	return server
}

func downloadTestArtifact(serverURL string, binary []byte) Artifact {
	sum := sha256.Sum256(binary)
	return Artifact{
		Name:     "magelift-provider-gcp",
		Version:  "v0.0.0-test.1",
		Protocol: ProtocolV1Marker,
		Digest:   "sha256:" + hex.EncodeToString(sum[:]),
		URL:      serverURL + "/magelift-provider-gcp",
		Cosign: CosignTrust{
			Identity: FirstPartyIdentityPrefix + "v0.0.0-test",
			Issuer:   FirstPartyIssuer,
			Bundle:   serverURL + "/magelift-provider-gcp.sigstore.json",
		},
	}
}

func TestDownloadInstallVerifiesAndCaches(t *testing.T) {
	binary := []byte("fake-provider-binary")
	bundle := []byte("fake-bundle")
	var hits atomic.Int32
	server := downloadTestServer(t, binary, bundle, &hits)
	artifact := downloadTestArtifact(server.URL, binary)

	cache := t.TempDir()
	downloader := &Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: cache}
	installed, err := downloader.Install(context.Background(), "gcp", artifact)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if installed.Cached {
		t.Fatalf("first install should not report Cached")
	}
	wantSuffix := filepath.Join("gcp", artifact.Version, CachePlatform(), ExtractName("gcp"))
	if !strings.HasSuffix(installed.Binary, wantSuffix) {
		t.Fatalf("unexpected binary path %q, want suffix %q", installed.Binary, wantSuffix)
	}
	got, err := os.ReadFile(installed.Binary)
	if err != nil || string(got) != string(binary) {
		t.Fatalf("cached binary = %q, err = %v", got, err)
	}
	if info, err := os.Stat(installed.Binary); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("cached binary is not executable: %v", err)
	}

	var verifyCalls atomic.Int32
	cached, err := (&Downloader{HTTP: server.Client(), Verifier: stubVerifier{calls: &verifyCalls}, CacheDir: cache}).Install(context.Background(), "gcp", artifact)
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if !cached.Cached {
		t.Fatalf("second install should report Cached")
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("cache hit used the network: %d requests", got)
	}
	if got := verifyCalls.Load(); got != 1 {
		t.Fatalf("cache hit did not re-verify signature: %d calls", got)
	}
}

func TestDownloadInstallNoNetworkOnValidCache(t *testing.T) {
	binary := []byte("fake-provider-binary")
	bundle := []byte("fake-bundle")
	var hits atomic.Int32
	server := downloadTestServer(t, binary, bundle, &hits)
	artifact := downloadTestArtifact(server.URL, binary)
	cache := t.TempDir()
	first := &Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: cache}
	if _, err := first.Install(context.Background(), "gcp", artifact); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	broken := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network disabled")
	})}
	cached, err := (&Downloader{HTTP: broken, Verifier: stubVerifier{}, CacheDir: cache}).Install(context.Background(), "gcp", artifact)
	if err != nil {
		t.Fatalf("cached Install: %v", err)
	}
	if !cached.Cached {
		t.Fatal("expected cache hit without network")
	}
	if hits.Load() != 2 {
		t.Fatalf("cache hit used the network: %d requests", hits.Load())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDownloadInstallConcurrentSingleFetch(t *testing.T) {
	binary := []byte("fake-provider-binary")
	bundle := []byte("fake-bundle")
	var hits atomic.Int32
	server := downloadTestServer(t, binary, bundle, &hits)
	artifact := downloadTestArtifact(server.URL, binary)
	cache := t.TempDir()
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := (&Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: cache}).Install(context.Background(), "gcp", artifact)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("Install: %v", err)
		}
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("expected one binary and one bundle fetch, got %d HTTP hits", got)
	}
}

func TestDownloadInstallRefusesInterruptedPartialCache(t *testing.T) {
	binary := []byte("fake-provider-binary")
	server := downloadTestServer(t, binary, []byte("fake-bundle"), nil)
	artifact := downloadTestArtifact(server.URL, binary)
	cache := t.TempDir()
	paths, err := CachedArtifactPaths(cache, "gcp", artifact.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Binary), 0o755); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(filepath.Dir(paths.Binary), ".download-partial-"+filepath.Base(paths.Binary))
	if err := os.WriteFile(partial, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	downloader := &Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: cache}
	_, err = downloader.Install(context.Background(), "gcp", artifact)
	if !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("err = %v, want corrupt cache", err)
	}
}

func TestDownloadInstallRepairClearsPartialCache(t *testing.T) {
	binary := []byte("fake-provider-binary")
	bundle := []byte("fake-bundle")
	server := downloadTestServer(t, binary, bundle, nil)
	artifact := downloadTestArtifact(server.URL, binary)
	cache := t.TempDir()
	paths, err := CachedArtifactPaths(cache, "gcp", artifact.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Binary), 0o755); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(filepath.Dir(paths.Binary), ".download-partial-"+filepath.Base(paths.Binary))
	if err := os.WriteFile(partial, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	downloader := &Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: cache, Repair: true}
	installed, err := downloader.Install(context.Background(), "gcp", artifact)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if installed.Cached {
		t.Fatal("repair install should fetch fresh artifacts")
	}
	if hasPartialDownloadTemps(filepath.Dir(paths.Binary)) {
		t.Fatal("partial download file remained after repair")
	}
}

func TestDownloadInstallRefusesInvalidBundleCache(t *testing.T) {
	binary := []byte("fake-provider-binary")
	server := downloadTestServer(t, binary, []byte("fake-bundle"), nil)
	artifact := downloadTestArtifact(server.URL, binary)
	cache := t.TempDir()
	paths, err := CachedArtifactPaths(cache, "gcp", artifact.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Binary, binary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CachedBundlePath(paths.Binary), []byte("bad-bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	downloader := &Downloader{
		HTTP:     server.Client(),
		Verifier: stubVerifier{err: errors.New("bad bundle")},
		CacheDir: cache,
	}
	_, err = downloader.Install(context.Background(), "gcp", artifact)
	if !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("err = %v, want corrupt cache", err)
	}
}

func TestDownloadInstallRefusesHalfPairPlatformCache(t *testing.T) {
	binary := []byte("fake-provider-binary")
	server := downloadTestServer(t, binary, []byte("fake-bundle"), nil)
	artifact := downloadTestArtifact(server.URL, binary)
	cache := t.TempDir()
	paths, err := CachedArtifactPaths(cache, "gcp", artifact.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Binary, binary, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = (&Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: cache}).Install(context.Background(), "gcp", artifact)
	if !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("err = %v, want corrupt cache", err)
	}
}

func TestDownloadInstallRefusesLegacyWhenPrimaryCorrupt(t *testing.T) {
	binary := []byte("fake-provider-binary")
	bundle := []byte("fake-bundle")
	server := downloadTestServer(t, binary, bundle, nil)
	artifact := downloadTestArtifact(server.URL, binary)
	cache := t.TempDir()
	primary, err := CachedArtifactPaths(cache, "gcp", artifact.Version)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := legacyCachedArtifactPaths(cache, "gcp", artifact.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(primary.Binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primary.Binary, binary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy.Binary, binary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CachedBundlePath(legacy.Binary), bundle, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = (&Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: cache}).Install(context.Background(), "gcp", artifact)
	if !errors.Is(err, ErrCacheCorrupt) {
		t.Fatalf("err = %v, want corrupt primary without legacy fallback", err)
	}
}

func TestDownloadInstallRefusesLegacyDigestMismatch(t *testing.T) {
	binary := []byte("fake-provider-binary")
	bundle := []byte("fake-bundle")
	server := downloadTestServer(t, binary, bundle, nil)
	artifact := downloadTestArtifact(server.URL, binary)
	cache := t.TempDir()
	legacy, err := legacyCachedArtifactPaths(cache, "gcp", artifact.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacy.Binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy.Binary, []byte("other-binary-bytes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CachedBundlePath(legacy.Binary), bundle, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = (&Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: cache}).Install(context.Background(), "gcp", artifact)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
}

func TestDownloadInstallCancelsActiveDownload(t *testing.T) {
	binary := []byte("fake-provider-binary")
	bundle := []byte("fake-bundle")
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/magelift-provider-gcp", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-release:
		case <-time.After(5 * time.Second):
		}
		w.Write(binary)
	})
	mux.HandleFunc("/magelift-provider-gcp.sigstore.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(bundle)
	})
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	artifact := downloadTestArtifact(server.URL, binary)
	cache := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(75 * time.Millisecond)
		cancel()
	}()
	_, err := (&Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: cache}).Install(ctx, "gcp", artifact)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want canceled download", err)
	}
	paths, err := CachedArtifactPaths(cache, "gcp", artifact.Version)
	if err != nil {
		t.Fatal(err)
	}
	if cachePairComplete(paths.Binary) {
		t.Fatal("canceled download left a complete cache pair")
	}
	if fileExists(paths.Binary) != fileExists(CachedBundlePath(paths.Binary)) {
		t.Fatal("canceled download left a half-installed platform pair")
	}
	close(release)
}

func TestDownloadInstallConcurrentSubprocessSingleFetch(t *testing.T) {
	if os.Getenv("MAGELIFT_PROVIDER_CACHE_CHILD") == "1" {
		cache := os.Getenv("MAGELIFT_PROVIDER_CACHE_DIR")
		serverURL := os.Getenv("MAGELIFT_PROVIDER_CACHE_SERVER")
		binary := []byte("fake-provider-binary")
		artifact := downloadTestArtifact(serverURL, binary)
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
		_, err := (&Downloader{HTTP: client, Verifier: stubVerifier{}, CacheDir: cache}).Install(context.Background(), "gcp", artifact)
		if err != nil {
			t.Fatalf("child Install: %v", err)
		}
		return
	}
	binary := []byte("fake-provider-binary")
	bundle := []byte("fake-bundle")
	var hits atomic.Int32
	server := downloadTestServer(t, binary, bundle, &hits)
	cache := t.TempDir()
	const workers = 4
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd := exec.Command(os.Args[0], "-test.run=^TestDownloadInstallConcurrentSubprocessSingleFetch$", "-test.count=1")
			cmd.Env = append(os.Environ(),
				"MAGELIFT_PROVIDER_CACHE_CHILD=1",
				"MAGELIFT_PROVIDER_CACHE_DIR="+cache,
				"MAGELIFT_PROVIDER_CACHE_SERVER="+server.URL,
			)
			out, err := cmd.CombinedOutput()
			if err != nil {
				errs <- fmt.Errorf("%w: %s", err, out)
				return
			}
			errs <- nil
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := hits.Load(); got != 2 {
		t.Fatalf("expected one binary and one bundle fetch across subprocesses, got %d HTTP hits", got)
	}
}

func TestDownloadInstallRepairPreservesInstallLock(t *testing.T) {
	binary := []byte("fake-provider-binary")
	bundle := []byte("fake-bundle")
	server := downloadTestServer(t, binary, bundle, nil)
	artifact := downloadTestArtifact(server.URL, binary)
	cache := t.TempDir()
	versionDir, err := cacheVersionDir(cache, "gcp", artifact.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(versionDir, cacheInstallLockName)
	if err := os.WriteFile(lockPath, []byte("held"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := CachedArtifactPaths(cache, "gcp", artifact.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(paths.Binary), 0o755); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(filepath.Dir(paths.Binary), ".download-partial-"+filepath.Base(paths.Binary))
	if err := os.WriteFile(partial, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, err := (&Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: cache, Repair: true}).Install(context.Background(), "gcp", artifact)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if installed.Cached {
		t.Fatal("repair install should fetch fresh artifacts")
	}
	after, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if before.Size() != after.Size() || !os.SameFile(before, after) {
		t.Fatal("repair removed or replaced the version install lock")
	}
	if sysBefore, ok := before.Sys().(*syscall.Stat_t); ok {
		sysAfter, ok := after.Sys().(*syscall.Stat_t)
		if !ok || sysBefore.Ino != sysAfter.Ino {
			t.Fatal("repair changed install lock inode")
		}
	}
}

func TestDownloadInstallAcceptsLegacyCacheLayout(t *testing.T) {
	binary := []byte("fake-provider-binary")
	bundle := []byte("fake-bundle")
	var hits atomic.Int32
	server := downloadTestServer(t, binary, bundle, &hits)
	artifact := downloadTestArtifact(server.URL, binary)
	cache := t.TempDir()
	legacy, err := legacyCachedArtifactPaths(cache, "gcp", artifact.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(legacy.Binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy.Binary, binary, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CachedBundlePath(legacy.Binary), bundle, 0o644); err != nil {
		t.Fatal(err)
	}
	broken := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network disabled")
	})}
	installed, err := (&Downloader{HTTP: broken, Verifier: stubVerifier{}, CacheDir: cache}).Install(context.Background(), "gcp", artifact)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !installed.Cached || installed.Binary != legacy.Binary {
		t.Fatalf("installed = %+v, want legacy cache hit", installed)
	}
}

func TestDownloadInstallRefusesTamperedBinary(t *testing.T) {
	binary := []byte("fake-provider-binary")
	artifact := downloadTestArtifact("https://example.invalid", []byte("different-bytes"))
	server := downloadTestServer(t, binary, []byte("fake-bundle"), nil)
	artifact.URL = server.URL + "/magelift-provider-gcp"
	artifact.Cosign.Bundle = server.URL + "/magelift-provider-gcp.sigstore.json"

	downloader := &Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: t.TempDir()}
	_, err := downloader.Install(context.Background(), "gcp", artifact)
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
}

func TestDownloadInstallRefusesBadSignature(t *testing.T) {
	binary := []byte("fake-provider-binary")
	server := downloadTestServer(t, binary, []byte("fake-bundle"), nil)
	artifact := downloadTestArtifact(server.URL, binary)

	downloader := &Downloader{HTTP: server.Client(), Verifier: stubVerifier{err: errors.New("bad bundle")}, CacheDir: t.TempDir()}
	_, err := downloader.Install(context.Background(), "gcp", artifact)
	if err == nil || !strings.Contains(err.Error(), "verify provider signature") {
		t.Fatalf("err = %v, want signature failure", err)
	}
}

func TestDownloadInstallRefusesUnsignedEntry(t *testing.T) {
	artifact := Artifact{Name: "magelift-provider-gcp", Version: "v1", Protocol: ProtocolV1Marker, Digest: "sha256:" + strings.Repeat("a", 64)}
	downloader := &Downloader{Verifier: stubVerifier{}, CacheDir: t.TempDir()}
	_, err := downloader.Install(context.Background(), "gcp", artifact)
	if !errors.Is(err, ErrUnsigned) {
		t.Fatalf("err = %v, want unsigned refusal", err)
	}
}

func TestDownloadInstallRefusesMissingURL(t *testing.T) {
	sum := sha256.Sum256([]byte("x"))
	artifact := Artifact{
		Name: "magelift-provider-gcp", Version: "v1", Protocol: ProtocolV1Marker,
		Digest: "sha256:" + hex.EncodeToString(sum[:]),
		Cosign: CosignTrust{Identity: FirstPartyIdentityPrefix + "v0.0.0-test", Issuer: FirstPartyIssuer, Bundle: "bundle.json"},
	}
	downloader := &Downloader{Verifier: stubVerifier{}, CacheDir: t.TempDir()}
	_, err := downloader.Install(context.Background(), "gcp", artifact)
	if !errors.Is(err, ErrNotDownloadable) {
		t.Fatalf("err = %v, want not-downloadable", err)
	}
}

func TestDownloadInstallRefusesUnsafeVersion(t *testing.T) {
	artifact := Artifact{
		Name: "magelift-provider-gcp", Version: "../escape", Protocol: ProtocolV1Marker,
		Digest: "sha256:" + strings.Repeat("a", 64), URL: "https://example.invalid/x",
		Cosign: CosignTrust{Identity: FirstPartyIdentityPrefix + "v0.0.0-test", Issuer: FirstPartyIssuer, Bundle: "https://example.invalid/x.sigstore.json"},
	}
	downloader := &Downloader{Verifier: stubVerifier{}, CacheDir: t.TempDir()}
	_, err := downloader.Install(context.Background(), "gcp", artifact)
	if err == nil || !strings.Contains(err.Error(), "safe path segment") {
		t.Fatalf("err = %v, want path traversal refusal", err)
	}
}

func TestDownloadInstallAcceptsLockfileRelativeBundle(t *testing.T) {
	binary := []byte("fake-provider-binary")
	bundle := []byte("fake-bundle")
	server := downloadTestServer(t, binary, bundle, nil)
	artifact := downloadTestArtifact(server.URL, binary)

	lockDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(lockDir, "gcp.sigstore.json"), bundle, 0o644); err != nil {
		t.Fatal(err)
	}
	artifact.Cosign.Bundle = "gcp.sigstore.json"

	downloader := &Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: t.TempDir(), LockDir: lockDir}
	installed, err := downloader.Install(context.Background(), "gcp", artifact)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !fileExists(installed.Bundle) {
		t.Fatalf("bundle was not installed to the cache")
	}
}

func TestDownloadInstallRefusesHTTPURL(t *testing.T) {
	sum := sha256.Sum256([]byte("x"))
	artifact := Artifact{
		Name: "magelift-provider-gcp", Version: "v1", Protocol: ProtocolV1Marker,
		Digest: "sha256:" + hex.EncodeToString(sum[:]), URL: "http://example.invalid/x",
		Cosign: CosignTrust{Identity: FirstPartyIdentityPrefix + "v0.0.0-test", Issuer: FirstPartyIssuer, Bundle: "https://example.invalid/x.sigstore.json"},
	}
	downloader := &Downloader{Verifier: stubVerifier{}, CacheDir: t.TempDir()}
	_, err := downloader.Install(context.Background(), "gcp", artifact)
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("err = %v, want HTTPS refusal", err)
	}
}

func TestResolveCachedFindsInstalledEntry(t *testing.T) {
	binary := []byte("fake-provider-binary")
	server := downloadTestServer(t, binary, []byte("fake-bundle"), nil)
	artifact := downloadTestArtifact(server.URL, binary)

	cache := t.TempDir()
	installed, err := (&Downloader{HTTP: server.Client(), Verifier: stubVerifier{}, CacheDir: cache}).Install(context.Background(), "gcp", artifact)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	lockDir := t.TempDir()
	lockPath := filepath.Join(lockDir, "magelift.providers.lock")
	lockJSON := `{"schemaVersion":1,"sdkAPIVersion":"v1","providers":{"gcp":{"name":"magelift-provider-gcp","version":"` + artifact.Version + `","protocol":"magelift-v1","digest":"` + artifact.Digest + `","url":"` + artifact.URL + `","cosign":{"identity":"https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/v0.0.0-test","issuer":"https://token.actions.githubusercontent.com","bundle":"` + artifact.Cosign.Bundle + `"}}}}`
	if err := os.WriteFile(lockPath, []byte(lockJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	gotBinary, gotBundle, err := ResolveCached(lockPath, "gcp", cache)
	if err != nil {
		t.Fatalf("ResolveCached: %v", err)
	}
	if gotBinary != installed.Binary || gotBundle != installed.Bundle {
		t.Fatalf("ResolveCached = %q %q, want %q %q", gotBinary, gotBundle, installed.Binary, installed.Bundle)
	}
}

func TestResolveCachedMissingEntryIsNotExist(t *testing.T) {
	lockDir := t.TempDir()
	lockPath := filepath.Join(lockDir, "magelift.providers.lock")
	lockJSON := `{"schemaVersion":1,"sdkAPIVersion":"v1","providers":{"gcp":{"name":"magelift-provider-gcp","version":"v9","protocol":"magelift-v1","digest":"sha256:` + strings.Repeat("b", 64) + `","cosign":{"identity":"https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/v0.0.0-test","issuer":"https://token.actions.githubusercontent.com","bundle":"bundle.json"}}}}`
	if err := os.WriteFile(lockPath, []byte(lockJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := ResolveCached(lockPath, "gcp", t.TempDir())
	if !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("err = %v, want cache miss", err)
	}
}

func TestDefaultCacheDirResolves(t *testing.T) {
	dir, err := DefaultCacheDir()
	if err != nil {
		t.Fatalf("DefaultCacheDir: %v", err)
	}
	if !strings.HasSuffix(dir, filepath.Join("magelift", "providers")) {
		t.Fatalf("unexpected cache dir %q", dir)
	}
}

func TestCachedArtifactPathsRejectsProviderTraversal(t *testing.T) {
	for _, provider := range []string{"", ".", "..", "../foreign", `..\foreign`, "/absolute"} {
		t.Run(provider, func(t *testing.T) {
			if _, err := CachedArtifactPaths(t.TempDir(), provider, "v1.0.0"); err == nil {
				t.Fatalf("accepted provider %q", provider)
			}
			if _, err := legacyCachedArtifactPaths(t.TempDir(), provider, "v1.0.0"); err == nil {
				t.Fatalf("accepted legacy provider %q", provider)
			}
		})
	}
}
