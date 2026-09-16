package providerhost

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cosign"
)

func TestDiscoverArtifactPaths(t *testing.T) {
	t.Parallel()
	paths := DiscoverArtifactPaths("/opt/magelift/magelift", "gcp")
	if !strings.HasSuffix(paths.Lock, "magelift.providers.lock") || !strings.Contains(paths.Lock, "/opt/magelift/") {
		t.Fatalf("lock = %q", paths.Lock)
	}
	if !strings.HasSuffix(paths.Binary, "magelift-provider-gcp") {
		t.Fatalf("binary = %q", paths.Binary)
	}
	local := DiscoverArtifactPaths("", "gcp")
	if local.Lock != "magelift.providers.lock" || local.Binary != "magelift-provider-gcp" {
		t.Fatalf("local = %#v", local)
	}
}

func TestCachedDialerDialsOnce(t *testing.T) {
	t.Parallel()
	dials := 0
	dialer := &CachedDialer{Dial: func(context.Context) (*Client, error) {
		dials++
		return &Client{}, nil
	}}
	first, err := dialer.Do(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, err := dialer.Do(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if dials != 1 || first != second {
		t.Fatal("dial was not cached")
	}
	failing := &CachedDialer{Dial: func(context.Context) (*Client, error) {
		return nil, errors.New("dial boom")
	}}
	if _, err := failing.Do(context.Background()); err == nil {
		t.Fatal("dial failure was accepted")
	}
	if _, err := (*CachedDialer)(nil).Do(context.Background()); err == nil {
		t.Fatal("nil dialer was accepted")
	}
}

type recordingRunner struct {
	name string
	args []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) error {
	r.name = name
	r.args = append([]string(nil), args...)
	return nil
}

// TestCosignVerifierPassesBundleFirst pins the bundle-first argument order
// through the verifier into cosign. A swap here makes every plugin load
// fail closed with a signature error while manual cosign calls succeed.
func TestCosignVerifierPassesBundleFirst(t *testing.T) {
	runner := &recordingRunner{}
	verifier := cosignVerifier{client: cosign.NewWithRunner(runner)}
	err := verifier.VerifyBlob(context.Background(), "/tmp/x.sigstore.json", "/tmp/x", cosign.VerifyOptions{
		CertificateIdentity: "identity",
		OIDCIssuer:          "https://token.actions.githubusercontent.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"verify-blob", "--bundle", "/tmp/x.sigstore.json", "--certificate-identity", "identity", "--certificate-oidc-issuer", "https://token.actions.githubusercontent.com", "/tmp/x"}
	if runner.name != "cosign" || strings.Join(runner.args, " ") != strings.Join(want, " ") {
		t.Fatalf("argv = %s %v, want cosign %v", runner.name, runner.args, want)
	}
	if err := NewCosignVerifier().VerifyBlob(context.Background(), "", "", cosign.VerifyOptions{}); err == nil {
		t.Fatal("empty paths were accepted")
	}
}
