package providerhost

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/cosign"
	sdk "github.com/magelift/magelift/sdk/v1"
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

func TestDialPingsVerifiedProvider(t *testing.T) {
	binary := buildGCPProvider(t)
	lockPath, bundlePath := writeVerifiedLock(t, binary)
	loaded, err := Load(context.Background(), Options{
		Mode:       ModeSubprocess,
		Provider:   "gcp",
		LockPath:   lockPath,
		BinaryPath: binary,
		BundlePath: bundlePath,
		Verifier:   fakeVerifier{},
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := Dial(context.Background(), loaded.Binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	got, err := session.Ping(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != SDKAPIVersion {
		t.Fatalf("ping = %q", got)
	}
	identity, err := session.Describe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if identity.APIVersion != SDKAPIVersion || identity.Provider != "gcp" || identity.Runtime != "gke-autopilot" || identity.TargetID != "gcp.gke-autopilot" || identity.Tier != "certified" {
		t.Fatalf("identity = %+v", identity)
	}
	if len(identity.OutputKeys) == 0 {
		t.Fatal("outputKeys are empty")
	}
	_, err = session.Plan(context.Background(), sdk.ModulePlanRequest{Environment: "staging"})
	if err == nil {
		t.Fatal("empty plan configuration should fail")
	}
	payload, err := json.Marshal(gcpAutopilotPlanConfig())
	if err != nil {
		t.Fatal(err)
	}
	var configuration map[string]any
	if err := json.Unmarshal(payload, &configuration); err != nil {
		t.Fatal(err)
	}
	plan, err := session.Plan(context.Background(), sdk.ModulePlanRequest{
		Environment:   "staging",
		Configuration: configuration,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Provider != "gcp" || plan.Runtime != "gke-autopilot" || plan.Environment != "staging" || plan.Opaque == nil {
		t.Fatalf("plan = %+v", plan)
	}
	result, err := session.Program(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if result.Kind != ProgramKindPulumiRunFunc {
		t.Fatalf("program = %+v", result)
	}
	if _, err := result.PulumiRunFunc(); !errors.Is(err, ErrProgramNotExecutable) {
		t.Fatalf("kind-only program run error = %v, want %v", err, ErrProgramNotExecutable)
	}
}

func TestDialExecutesOverMockBackend(t *testing.T) {
	binary := buildGCPProvider(t)
	lockPath, bundlePath := writeVerifiedLock(t, binary)
	loaded, err := Load(context.Background(), Options{
		Mode:       ModeSubprocess,
		Provider:   "gcp",
		LockPath:   lockPath,
		BinaryPath: binary,
		BundlePath: bundlePath,
		Verifier:   fakeVerifier{},
	})
	if err != nil {
		t.Fatal(err)
	}
	session, err := Dial(context.Background(), loaded.Binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(session.Close)
	payload, err := json.Marshal(gcpAutopilotPlanConfig())
	if err != nil {
		t.Fatal(err)
	}
	var configuration map[string]any
	if err := json.Unmarshal(payload, &configuration); err != nil {
		t.Fatal(err)
	}
	plan, err := session.Plan(context.Background(), sdk.ModulePlanRequest{
		Environment:   "staging",
		Configuration: configuration,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []ExecuteOperation{ExecutePreview, ExecuteUp, ExecuteDestroy, ExecuteOutputs, ExecuteRedactedOutputs, ExecuteValidateRequest} {
		result, err := session.Execute(context.Background(), ExecuteRequest{
			Operation:  operation,
			Plan:       plan,
			BackendURL: "test://mock",
		})
		if err != nil {
			t.Fatalf("%s: %v", operation, err)
		}
		if result.Operation != operation {
			t.Fatalf("%s: operation echo = %q", operation, result.Operation)
		}
		if len(result.Diagnostics) == 0 || !strings.Contains(result.Diagnostics[0], "mock execute") {
			t.Fatalf("%s: diagnostics = %v", operation, result.Diagnostics)
		}
	}
	preview, err := session.Execute(context.Background(), ExecuteRequest{
		Operation:  ExecutePreview,
		Plan:       plan,
		BackendURL: "test://mock",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Changes["create"] != 1 {
		t.Fatalf("preview changes = %v", preview.Changes)
	}
	outputs, err := session.Execute(context.Background(), ExecuteRequest{
		Operation:  ExecuteOutputs,
		Plan:       plan,
		BackendURL: "test://mock",
	})
	if err != nil {
		t.Fatal(err)
	}
	if outputs.Outputs["mock"] != true {
		t.Fatalf("outputs = %v", outputs.Outputs)
	}
	redacted, err := session.Execute(context.Background(), ExecuteRequest{
		Operation:  ExecuteRedactedOutputs,
		Plan:       plan,
		BackendURL: "test://mock",
	})
	if err != nil {
		t.Fatal(err)
	}
	marker, ok := redacted.Outputs["kubeconfig"].(map[string]any)
	if !ok || marker["secret"] != true {
		t.Fatalf("redacted outputs = %v", redacted.Outputs)
	}
	broken := plan
	broken.Opaque = "not-a-gcp-spec"
	if _, err := session.Execute(context.Background(), ExecuteRequest{
		Operation:  ExecutePreview,
		Plan:       broken,
		BackendURL: "test://mock",
	}); err == nil || !strings.Contains(err.Error(), "decode GCP plan") {
		t.Fatalf("broken plan err = %v, want decode failure", err)
	}
}

func gcpAutopilotPlanConfig() config.Config {
	return config.Config{
		SchemaVersion: 1,
		Project:       config.Project{Name: "shop"},
		Application: config.Application{
			Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm",
		},
		Target: config.Target{
			Provider: "gcp",
			Runtime:  "gke-autopilot",
			GCP: &config.GCPTarget{
				Project:             "example-gcp-project",
				Region:              "europe-west1",
				NetworkCIDR:         "10.20.0.0/16",
				ImageDigest:         "ghcr.io/magelift/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				EncryptionKeySecret: "magento-crypt-key",
			},
		},
		Defaults:      config.Defaults{Region: "europe-west1", Preset: "standard"},
		Class:         "staging",
		Preset:        "standard",
		ExpiresAt:     time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		Compatibility: config.Compatibility{AllowUnsupported: true},
	}
}

func buildGCPProvider(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), ExtractName("gcp"))
	cmd := exec.Command("go", "build", "-o", out, "./cmd/magelift-provider-gcp")
	cmd.Dir = moduleRoot(t)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build magelift-provider-gcp: %v\n%s", err, output)
	}
	return out
}

func writeVerifiedLock(t *testing.T, binary string) (lockPath, bundlePath string) {
	t.Helper()
	payload, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	dir := t.TempDir()
	lockPath = filepath.Join(dir, "magelift.providers.lock")
	if err := os.WriteFile(lockPath, validLockJSON("sha256:"+hex.EncodeToString(sum[:])), 0o600); err != nil {
		t.Fatal(err)
	}
	bundlePath = filepath.Join(dir, "bundle.json")
	if err := os.WriteFile(bundlePath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return lockPath, bundlePath
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
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
	cases := []struct {
		name    string
		options Options
	}{
		{
			name: "missing bundle path",
			options: Options{
				Mode: ModeSubprocess, Provider: "gcp", LockPath: lockPath,
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

func TestCheckAPIVersion(t *testing.T) {
	if err := checkAPIVersion(SDKAPIVersion); err != nil {
		t.Fatalf("host version refused: %v", err)
	}
	for _, version := range []string{"", "v0", "v2"} {
		if err := checkAPIVersion(version); !errors.Is(err, ErrUnsupportedAPI) {
			t.Fatalf("version %q err = %v, want %v", version, err, ErrUnsupportedAPI)
		}
	}
	// Dial enforces checkAPIVersion after Ping; the Dial-level mismatch path
	// would need a dedicated fake plugin binary, so the helper carries the
	// contract and TestDialPingsVerifiedProvider covers the accept path.
}
