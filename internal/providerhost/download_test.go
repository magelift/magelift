package providerhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

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
	if !strings.HasSuffix(installed.Binary, filepath.Join("gcp", artifact.Version, ExtractName("gcp"))) {
		t.Fatalf("unexpected binary path %q", installed.Binary)
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
	if got := verifyCalls.Load(); got != 0 {
		t.Fatalf("cache hit re-verified: %d calls", got)
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
