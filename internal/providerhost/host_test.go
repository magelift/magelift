package providerhost

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cosign"
)

const testDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func validLockJSON(digest string) []byte {
	if digest == "" {
		digest = testDigest
	}
	return []byte(`{
  "schemaVersion": 1,
  "sdkAPIVersion": "v1",
  "providers": {
    "gcp": {
      "name": "magelift-provider-gcp",
      "version": "0.1.0",
      "protocol": "magelift-v1",
      "digest": "` + digest + `",
      "url": "https://github.com/magelift/magelift/releases/download/v0.1.0/magelift-provider-gcp",
      "cosign": {
        "identity": "https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/v0.1.0",
        "issuer": "https://token.actions.githubusercontent.com",
        "bundle": "magelift-provider-gcp.sigstore.json"
      }
    }
  }
}`)
}

func TestParseLockfile(t *testing.T) {
	lock, err := ParseLock(bytes.NewReader(validLockJSON("")))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := lock.Artifact("gcp")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Name != "magelift-provider-gcp" || artifact.Digest != testDigest {
		t.Fatalf("artifact = %+v", artifact)
	}
}

func TestParseLockfileRefuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		body    string
		wantErr error
	}{
		{
			name:    "unsigned",
			body:    strings.Replace(string(validLockJSON("")), `"identity": "https://github.com/magelift/magelift/.github/workflows/release.yml@refs/tags/v0.1.0"`, `"identity": ""`, 1),
			wantErr: ErrUnsigned,
		},
		{
			name:    "missing digest",
			body:    strings.Replace(string(validLockJSON("")), `"digest": "`+testDigest+`"`, `"digest": ""`, 1),
			wantErr: ErrDigestRequired,
		},
		{
			name:    "bad schema",
			body:    strings.Replace(string(validLockJSON("")), `"schemaVersion": 1`, `"schemaVersion": 2`, 1),
			wantErr: ErrUnsupportedSchema,
		},
		{
			name:    "schema zero refused",
			body:    strings.Replace(string(validLockJSON("")), `"schemaVersion": 1`, `"schemaVersion": 0`, 1),
			wantErr: ErrUnsupportedSchema,
		},
		{
			name:    "missing protocol",
			body:    strings.Replace(string(validLockJSON("")), `"protocol": "magelift-v1",`, ``, 1),
			wantErr: ErrUnsupportedProtocol,
		},
		{
			name:    "wrong protocol",
			body:    strings.Replace(string(validLockJSON("")), `"protocol": "magelift-v1"`, `"protocol": "not-a-protocol"`, 1),
			wantErr: ErrUnsupportedProtocol,
		},
		{
			name:    "bad api",
			body:    strings.Replace(string(validLockJSON("")), `"sdkAPIVersion": "v1"`, `"sdkAPIVersion": "v0"`, 1),
			wantErr: ErrUnsupportedAPI,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseLock(strings.NewReader(tc.body))
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestLoadInProcessDoesNotReadLock(t *testing.T) {
	loaded, err := Load(context.Background(), Options{Mode: ModeInProcess, Provider: "gcp", LockPath: "/no/such/lock"})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mode != ModeInProcess || loaded.Binary != "" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestLoadSubprocessRefusesChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "magelift-provider-gcp")
	if err := os.WriteFile(binary, []byte("provider-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "magelift.providers.lock")
	if err := os.WriteFile(lockPath, validLockJSON(""), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(context.Background(), Options{
		Mode:       ModeSubprocess,
		Provider:   "gcp",
		LockPath:   lockPath,
		BinaryPath: binary,
		BundlePath: filepath.Join(dir, "bundle.json"),
		Verifier:   fakeVerifier{},
	})
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadSubprocessVerifiesDigestAndSignature(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("provider-bytes")
	sum := sha256.Sum256(payload)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	binary := filepath.Join(dir, "magelift-provider-gcp")
	if err := os.WriteFile(binary, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "magelift.providers.lock")
	if err := os.WriteFile(lockPath, validLockJSON(digest), 0o600); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(dir, "bundle.json")
	if err := os.WriteFile(bundle, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(context.Background(), Options{
		Mode:       ModeSubprocess,
		Provider:   "gcp",
		LockPath:   lockPath,
		BinaryPath: binary,
		BundlePath: bundle,
		Verifier:   fakeVerifier{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Mode != ModeSubprocess || loaded.Binary != binary || loaded.Artifact.Name != ExtractName("gcp") {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestExtractName(t *testing.T) {
	if got := ExtractName("gcp"); got != "magelift-provider-gcp" {
		t.Fatalf("got %q", got)
	}
}

type fakeVerifier struct {
	err error
}

func (f fakeVerifier) VerifyBlob(context.Context, string, string, cosign.VerifyOptions) error {
	return f.err
}

func TestLoadSubprocessRefusesUnknownProvider(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "magelift.providers.lock")
	if err := os.WriteFile(lockPath, validLockJSON(""), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(context.Background(), Options{
		Mode:       ModeSubprocess,
		Provider:   "aws",
		LockPath:   lockPath,
		BinaryPath: filepath.Join(dir, "magelift-provider-aws"),
		BundlePath: filepath.Join(dir, "bundle.json"),
		Verifier:   fakeVerifier{},
	})
	if !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("err = %v, want %v", err, ErrUnknownProvider)
	}
}

func TestLoadSubprocessRefusesUnsignedVerify(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("provider-bytes")
	sum := sha256.Sum256(payload)
	digest := "sha256:" + hex.EncodeToString(sum[:])
	binary := filepath.Join(dir, "magelift-provider-gcp")
	if err := os.WriteFile(binary, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(dir, "magelift.providers.lock")
	if err := os.WriteFile(lockPath, validLockJSON(digest), 0o600); err != nil {
		t.Fatal(err)
	}
	lockWithoutBundle := filepath.Join(dir, "magelift-no-bundle.providers.lock")
	bundleless := strings.Replace(string(validLockJSON(digest)), `"bundle": "magelift-provider-gcp.sigstore.json"`, `"bundle": ""`, 1)
	if err := os.WriteFile(lockWithoutBundle, []byte(bundleless), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		options Options
	}{
		{
			name: "missing bundle name",
			options: Options{
				Mode: ModeSubprocess, Provider: "gcp", LockPath: lockWithoutBundle,
				BinaryPath: binary, BundlePath: "", Verifier: fakeVerifier{},
			},
		},
		{
			name: "nil verifier",
			options: Options{
				Mode: ModeSubprocess, Provider: "gcp", LockPath: lockPath,
				BinaryPath: binary, BundlePath: filepath.Join(dir, "bundle.json"), Verifier: nil,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(context.Background(), tc.options)
			if !errors.Is(err, ErrUnsigned) {
				t.Fatalf("err = %v, want %v", err, ErrUnsigned)
			}
		})
	}
}
