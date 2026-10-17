package providerhost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cosign"
)

func TestEnsureTrustedRootMaterializesPinnedBytes(t *testing.T) {
	cache := t.TempDir()
	installer := &VerifierInstaller{CacheDir: cache}
	path, err := installer.ensureTrustedRoot(context.Background(), cache)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, embeddedTrustedRootDigest+"/"+trustedRootFileName) {
		t.Fatalf("path = %q", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyDigestBytes(body, "sha256:"+embeddedTrustedRootDigest); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(string(body), string(embeddedTrustedRoot)) {
		t.Fatal("materialized trusted root differs from embedded asset")
	}
	second, err := installer.ensureTrustedRoot(context.Background(), cache)
	if err != nil || second != path {
		t.Fatalf("second = %q, %v", second, err)
	}
}

func TestCachedTrustedRootRejectsCorruptWithoutRepair(t *testing.T) {
	cache := t.TempDir()
	path, err := TrustedRootPath(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CachedTrustedRootPath(cache); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("err = %v", err)
	}
	installer := &VerifierInstaller{CacheDir: cache}
	if _, err := installer.ensureTrustedRoot(context.Background(), cache); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("Ensure without repair: %v", err)
	}
	installer.Repair = true
	fixed, err := installer.ensureTrustedRoot(context.Background(), cache)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := CachedTrustedRootPath(cache); err != nil || got != fixed {
		t.Fatalf("cached = %q, %v", got, err)
	}
}

func TestCachedVerifierPathFailsClosedOnCorruptTrustedRoot(t *testing.T) {
	fixture := []byte("fixture-cosign")
	verifierTestPin(t, fixture)
	cache := t.TempDir()
	path, err := VerifierPath(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	rootPath, err := TrustedRootPath(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(rootPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rootPath, []byte("bad-root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CachedVerifierPath(cache); err == nil {
		t.Fatal("corrupt trusted root left cached verifier usable")
	}
}

func TestNewPreferredVerifierPassesPinnedTrustedRoot(t *testing.T) {
	fixture := []byte("fixture-cosign")
	verifierTestPin(t, fixture)
	cache := t.TempDir()
	path, err := VerifierPath(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (&VerifierInstaller{CacheDir: cache}).ensureTrustedRoot(context.Background(), cache); err != nil {
		t.Fatal(err)
	}
	root, err := CachedTrustedRootPath(cache)
	if err != nil {
		t.Fatal(err)
	}
	runner := &cosignRecordingRunner{}
	verifier := cosignVerifier{
		client:      cosign.NewWithRunner(runner),
		trustedRoot: root,
	}
	if err := verifier.VerifyBlob(context.Background(), "/tmp/bundle.json", "/tmp/binary", cosign.VerifyOptions{
		CertificateIdentity: "identity",
		OIDCIssuer:          "https://token.actions.githubusercontent.com",
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(runner.lastArgs, " "), "--trusted-root") {
		t.Fatalf("args = %v", runner.lastArgs)
	}
	if !strings.Contains(strings.Join(runner.lastArgs, " "), root) {
		t.Fatalf("args = %v", runner.lastArgs)
	}
}

type cosignRecordingRunner struct {
	lastArgs []string
}

func (r *cosignRecordingRunner) Run(_ context.Context, name string, args ...string) error {
	r.lastArgs = append([]string{name}, args...)
	return nil
}

func TestVerifierEnsureRepairsCorruptRootExplicitly(t *testing.T) {
	fixture := []byte("pinned cosign fixture")
	verifierTestPin(t, fixture)
	cache := t.TempDir()
	path, err := VerifierPath(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := TrustedRootPath(cache)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(root, []byte("bad-root"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (&VerifierInstaller{CacheDir: cache}).Ensure(context.Background()); err == nil {
		t.Fatal("automatic root repair accepted")
	}
	if err := NewPreferredVerifier(cache).VerifyBlob(context.Background(), "bundle", "binary", cosign.VerifyOptions{}); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("corrupt root selected fallback verifier: %v", err)
	}
	if _, err := (&VerifierInstaller{CacheDir: cache, Repair: true}).Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := CachedTrustedRootPath(cache); err != nil {
		t.Fatal(err)
	}
}
