package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/cosign"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type fakeProviderAPI struct {
	execute func(providerhost.ExecuteRequest) (providerhost.ExecuteResult, error)
}

func (f fakeProviderAPI) Ping(context.Context) (string, error) {
	return providerhost.SDKAPIVersion, nil
}

func (f fakeProviderAPI) Describe(context.Context) (providerhost.Identity, error) {
	return providerhost.Identity{APIVersion: providerhost.SDKAPIVersion, Provider: "gcp"}, nil
}

func (f fakeProviderAPI) Plan(_ context.Context, _ sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
	return sdk.ModulePlan{}, errors.New("plan not implemented")
}

func (f fakeProviderAPI) Program(_ context.Context, _ sdk.ModulePlan) (providerhost.ProgramResult, error) {
	return providerhost.ProgramResult{}, errors.New("program not implemented")
}

func (f fakeProviderAPI) Execute(_ context.Context, request providerhost.ExecuteRequest) (providerhost.ExecuteResult, error) {
	if f.execute == nil {
		return providerhost.ExecuteResult{Operation: request.Operation}, nil
	}
	return f.execute(request)
}

type gcpProofPlanned struct {
	stubPlanned
	spec any
}

func (p gcpProofPlanned) OpaquePlanSpec() any { return p.spec }

func testProofPlanned() gcpProofPlanned {
	return gcpProofPlanned{
		stubPlanned: stubPlanned{
			stackName:   "shop-staging",
			provider:    "gcp",
			runtime:     "gke-autopilot",
			project:     "shop",
			environment: "staging",
			region:      "europe-west1",
			envClass:    "staging",
			tier:        platform.TierCertified,
		},
		spec: map[string]any{"marker": "proof-spec"},
	}
}

func testLoaded() providerhost.Loaded {
	return providerhost.Loaded{
		Mode:     providerhost.ModeSubprocess,
		Provider: "gcp",
		Binary:   "/tmp/magelift-provider-gcp",
		Artifact: providerhost.Artifact{
			Name:    "magelift-provider-gcp",
			Version: "0.1.0",
			Digest:  "sha256:abc",
		},
	}
}

func TestDefaultNewBackendUsesSubprocessForProofCell(t *testing.T) {
	var stderr bytes.Buffer
	var got providerhost.ExecuteRequest
	api := fakeProviderAPI{execute: func(request providerhost.ExecuteRequest) (providerhost.ExecuteResult, error) {
		got = request
		return providerhost.ExecuteResult{Operation: request.Operation, Changes: map[string]int{"create": 3}}, nil
	}}
	o := &options{
		stderr:       &stderr,
		modules:      platform.NewModuleRegistry(),
		loadProvider: func(context.Context, string) (providerhost.Loaded, error) { return testLoaded(), nil },
		dialProvider: func(context.Context, string) (providerhost.API, error) { return api, nil },
	}
	backend, err := o.defaultNewBackend(context.Background(), testProofPlanned(), "file:///state")
	if err != nil {
		t.Fatal(err)
	}
	subprocess, ok := backend.(*providerhost.SubprocessBackend)
	if !ok {
		t.Fatalf("backend = %T, want *SubprocessBackend", backend)
	}
	if !strings.Contains(stderr.String(), "using subprocess provider magelift-provider-gcp 0.1.0") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if subprocess.Plan().Opaque.(map[string]any)["marker"] != "proof-spec" {
		t.Fatalf("plan opaque = %v", subprocess.Plan().Opaque)
	}
	if subprocess.Plan().Tier != sdk.ExtensionTierCertified {
		t.Fatalf("plan tier = %q", subprocess.Plan().Tier)
	}
	changes, err := backend.Preview(context.Background(), automation.Request{Target: sdk.TargetDescriptor{Provider: "gcp", Runtime: "gke-autopilot"}}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if changes["create"] != 3 || got.BackendURL != "file:///state" || got.Plan.StackName != "shop-staging" {
		t.Fatalf("changes = %v, request = %+v", changes, got)
	}
}

func TestDefaultNewBackendFallsBackWhenProviderMissing(t *testing.T) {
	var stderr bytes.Buffer
	o := &options{
		stderr:  &stderr,
		modules: platform.NewModuleRegistry(),
		loadProvider: func(context.Context, string) (providerhost.Loaded, error) {
			return providerhost.Loaded{}, errors.New("no lockfile")
		},
		dialProvider: func(context.Context, string) (providerhost.API, error) {
			t.Fatal("dial must not run when loading fails")
			return nil, nil
		},
	}
	_, err := o.defaultNewBackend(context.Background(), testProofPlanned(), "file:///state")
	if err == nil || !strings.Contains(err.Error(), "no stack module") {
		t.Fatalf("err = %v, want in-process fallback failure (empty registry, no Pulumi)", err)
	}
	if !strings.Contains(stderr.String(), "subprocess provider magelift-provider-gcp unavailable (no lockfile); using in-process backend") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestDefaultNewBackendSkipsSubprocessForOtherCells(t *testing.T) {
	var stderr bytes.Buffer
	o := &options{
		stderr:  &stderr,
		modules: platform.NewModuleRegistry(),
		loadProvider: func(context.Context, string) (providerhost.Loaded, error) {
			t.Fatal("loader must not run for non-proof cells")
			return providerhost.Loaded{}, nil
		},
		dialProvider: func(context.Context, string) (providerhost.API, error) {
			t.Fatal("dial must not run for non-proof cells")
			return nil, nil
		},
	}
	other := stubPlanned{stackName: "shop-prod", provider: "aws", runtime: "eks", tier: platform.TierCertified}
	_, err := o.defaultNewBackend(context.Background(), other, "file:///state")
	if err == nil || !strings.Contains(err.Error(), "no stack module") {
		t.Fatalf("err = %v, want in-process path", err)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want no subprocess notice", stderr.String())
	}
}

func TestSubprocessProvenance(t *testing.T) {
	withArtifact := &options{
		loadProvider: func(context.Context, string) (providerhost.Loaded, error) { return testLoaded(), nil },
	}
	provenance := withArtifact.subprocessProvenance(context.Background())
	if len(provenance) != 1 {
		t.Fatalf("provenance = %v", provenance)
	}
	got := provenance[0]
	if got.Name != "magelift-provider-gcp" || got.Version != "0.1.0" || got.Digest != "sha256:abc" || got.Mode != "subprocess" {
		t.Fatalf("provenance = %+v", got)
	}
	withoutArtifact := &options{
		loadProvider: func(context.Context, string) (providerhost.Loaded, error) {
			return providerhost.Loaded{}, errors.New("no lockfile")
		},
	}
	provenance = withoutArtifact.subprocessProvenance(context.Background())
	if len(provenance) != 1 || provenance[0].Mode != "in-process" || provenance[0].Version != "" {
		t.Fatalf("provenance = %v", provenance)
	}
}

func TestExtensionsListShowsProviderProvenance(t *testing.T) {
	var stdout bytes.Buffer
	o := &options{
		output:       "json",
		stdout:       &stdout,
		stderr:       io.Discard,
		modules:      platform.NewModuleRegistry(),
		loadProvider: func(context.Context, string) (providerhost.Loaded, error) { return testLoaded(), nil },
	}
	root := extensionsCommand(o)
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, want := range []string{`"providers"`, "magelift-provider-gcp", `"mode": "subprocess"`, `"version": "0.1.0"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

// TestOutputsDisplayRedactsThroughSubprocessBackend pins the display
// boundary: `magelift outputs` through a Dialed session must serve the
// redacted op, never the decrypted day-2 outputs. SubprocessBackend must
// keep its RedactedOutputs method or this falls through to decrypted
// Outputs and leaks kubeconfig to stdout.
func TestOutputsDisplayRedactsThroughSubprocessBackend(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	var stdout, stderr bytes.Buffer
	var got []providerhost.ExecuteOperation
	api := fakeProviderAPI{execute: func(request providerhost.ExecuteRequest) (providerhost.ExecuteResult, error) {
		got = append(got, request.Operation)
		if request.Operation == providerhost.ExecuteRedactedOutputs {
			return providerhost.ExecuteResult{Operation: request.Operation, Outputs: map[string]any{"kubeconfig": map[string]any{"secret": true}}}, nil
		}
		return providerhost.ExecuteResult{Operation: request.Operation, Outputs: map[string]any{"kubeconfig": "DECRYPTED-KUBECONFIG-SENTINEL"}}, nil
	}}
	o := testOptions(&stdout, &fakeTerminal{interactive: false})
	o.stderr = &stderr
	o.configPath, o.environment = path, "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return providerhost.NewSubprocessBackend(api, sdk.ModulePlan{StackName: "shop-staging"}, ""), nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "outputs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != providerhost.ExecuteRedactedOutputs {
		t.Fatalf("operations = %v, want [redacted-outputs]", got)
	}
	out := stdout.String()
	if !strings.Contains(out, `"secret"`) {
		t.Fatalf("display must carry the redacted marker:\n%s", out)
	}
	if strings.Contains(out, "DECRYPTED-KUBECONFIG-SENTINEL") {
		t.Fatalf("display leaked decrypted kubeconfig:\n%s", out)
	}
}

func TestDefaultLoadProviderRefusesWithoutLockfile(t *testing.T) {
	dir := t.TempDir()
	o := &options{executable: func() (string, error) { return dir + "/magelift", nil }}
	_, err := o.defaultLoadProvider(context.Background(), "gcp")
	if err == nil || !strings.Contains(err.Error(), "lockfile") {
		t.Fatalf("err = %v, want lockfile failure", err)
	}
}

func TestWarnProviderSkew(t *testing.T) {
	previous := Version
	t.Cleanup(func() { Version = previous })
	for _, tc := range []struct {
		name        string
		cliVersion  string
		lockVersion string
		wantWarning bool
	}{
		{"skew warns", "v1.0.0-rc.1", "v1.0.0-rc.2", true},
		{"match quiet", "v1.0.0-rc.1", "v1.0.0-rc.1", false},
		{"v prefix match quiet", "v1.0.0", "1.0.0", false},
		{"dev quiet", "dev", "v1.0.0", false},
		{"versionless lock quiet", "v1.0.0", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			Version = tc.cliVersion
			var stderr bytes.Buffer
			warnProviderSkew(&stderr, "gcp", tc.lockVersion)
			if tc.wantWarning {
				want := "provider gcp version " + tc.lockVersion + " differs from CLI version " + tc.cliVersion
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("stderr = %q, want %q", stderr.String(), want)
				}
				return
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want quiet", stderr.String())
			}
		})
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

// TestCosignBlobVerifierPassesBundleFirst pins the bundle-first
// argument order through the providerhost verifier into cosign.
// A swap here makes every subprocess load fail closed with a
// signature error while manual cosign calls succeed.
func TestCosignBlobVerifierPassesBundleFirst(t *testing.T) {
	runner := &recordingRunner{}
	verifier := cosignBlobVerifier{client: cosign.NewWithRunner(runner)}
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
}
