package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
)

type scriptedCaller struct {
	respond func(method string, reply any) error
	calls   []string
}

func (s *scriptedCaller) Call(method string, _ any, reply any) error {
	s.calls = append(s.calls, method)
	return s.respond(method, reply)
}

func testDescribe() *sdk.DescribeResponse {
	operations := make([]sdk.OperationVersion, 0, len(sdk.PluginMethods))
	for operation := range sdk.PluginMethods {
		operations = append(operations, sdk.OperationVersion{Name: string(operation), Version: "1.0"})
	}
	return &sdk.DescribeResponse{
		ProtocolVersion: sdk.ProtocolV1, ProviderID: "gcp", ProviderVersion: "v0.0.0-test",
		Operations: operations,
		Runtimes: []sdk.RuntimeAdvertisement{
			{Runtime: "gke-autopilot", Tier: sdk.ExtensionTierBuiltIn},
			{Runtime: "gke-standard", Tier: sdk.ExtensionTierExperimental},
		},
	}
}

func testShimPlan(t *testing.T, client *providerhost.Client) platform.PlannedStack {
	t.Helper()
	module, err := providerhost.NewShimModule("gcp", "gke-autopilot", client)
	if err != nil {
		t.Fatal(err)
	}
	planned, err := module.Plan(gcpShimConfig(), "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return planned
}

func gcpShimConfig() config.Config {
	return config.Config{
		SchemaVersion: 1,
		Project:       config.Project{Name: "shop"},
		Application:   config.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Target: config.Target{
			Provider: "gcp", Runtime: "gke-autopilot",
			GCP: &config.GCPTarget{
				Project:             "example-gcp-project",
				Region:              "europe-west1",
				NetworkCIDR:         "10.20.0.0/16",
				ImageDigest:         "ghcr.io/magelift/magento@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				EncryptionKeySecret: "magento-crypt-key",
			},
		},
		Defaults:  config.Defaults{Region: "europe-west1", Preset: "standard"},
		Class:     "staging",
		Preset:    "standard",
		ExpiresAt: "2027-01-01T00:00:00Z",
	}
}

func testLoaded() providerhost.Loaded {
	return providerhost.Loaded{
		Mode:     providerhost.ModeSubprocess,
		Provider: "gcp",
		Binary:   "/tmp/magelift-provider-gcp",
		Artifact: providerhost.Artifact{
			Name:     "magelift-provider-gcp",
			Version:  "0.1.0",
			Protocol: "magelift-v1",
			Digest:   "sha256:abc",
		},
	}
}

func TestDefaultNewBackendUsesPluginForGCP(t *testing.T) {
	caller := &scriptedCaller{respond: func(method string, reply any) error {
		switch method {
		case "Plugin.Plan":
			*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Plan: sdk.StoredPlan{
				StackName: "shop-staging", Provider: "gcp", Runtime: "gke-autopilot",
				ImageDigest: "digest", Opaque: []byte(`{"stored":true}`),
			}}
		case "Plugin.Preview":
			*(reply.(*sdk.LifecycleResult)) = sdk.LifecycleResult{Summary: sdk.ChangeSummary{Create: 3}}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	}}
	client := providerhost.NewTestClient(caller, testDescribe())
	o := &options{
		modules: platform.NewModuleRegistry(),
		loadProvider: func(context.Context, string) (providerhost.Loaded, error) {
			t.Fatal("planned provider must not be acquired a second time")
			return providerhost.Loaded{}, nil
		},
	}
	backend, err := o.defaultNewBackend(context.Background(), testShimPlan(t, client), "file:///state")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := backend.(*providerhost.PluginBackend); !ok {
		t.Fatalf("backend = %T, want *PluginBackend", backend)
	}
	changes, err := backend.Preview(context.Background(), automation.Request{Target: sdk.TargetDescriptor{Provider: "gcp", Runtime: "gke-autopilot"}}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if changes["create"] != 3 {
		t.Fatalf("changes = %v", changes)
	}
}

func TestDefaultNewBackendReusesPlannedProviderSession(t *testing.T) {
	var stderr bytes.Buffer
	o := &options{
		stderr:  &stderr,
		modules: platform.NewModuleRegistry(),
		loadProvider: func(context.Context, string) (providerhost.Loaded, error) {
			t.Fatal("planned provider must not be loaded a second time")
			return providerhost.Loaded{}, nil
		},
	}
	planClient := providerhost.NewTestClient(&scriptedCaller{respond: func(method string, reply any) error {
		switch method {
		case "Plugin.Plan":
			*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Plan: sdk.StoredPlan{
				StackName: "shop-staging", Provider: "gcp", Runtime: "gke-autopilot", Opaque: []byte(`{}`),
			}}
		case "Plugin.Preview":
			*(reply.(*sdk.LifecycleResult)) = sdk.LifecycleResult{Summary: sdk.ChangeSummary{Create: 1}}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	}}, testDescribe())
	backend, err := o.defaultNewBackend(context.Background(), testShimPlan(t, planClient), "file:///state")
	if err != nil {
		t.Fatalf("defaultNewBackend: %v", err)
	}
	if _, err := backend.Preview(context.Background(), automation.Request{Target: sdk.TargetDescriptor{Provider: "gcp", Runtime: "gke-autopilot"}}, io.Discard); err != nil {
		t.Fatalf("Preview on planned session: %v", err)
	}
	if strings.Contains(stderr.String(), "using in-process backend") {
		t.Fatalf("stderr = %q, want no fallback notice", stderr.String())
	}
}

func TestDefaultNewBackendSkipsPluginForOtherCells(t *testing.T) {
	var stderr bytes.Buffer
	o := &options{
		stderr:  &stderr,
		modules: platform.NewModuleRegistry(),
		loadProvider: func(context.Context, string) (providerhost.Loaded, error) {
			t.Fatal("loader must not run for non-GCP cells")
			return providerhost.Loaded{}, nil
		},
	}
	other := stubPlanned{stackName: "shop-prod", provider: "aws", runtime: "eks", tier: platform.TierBuiltIn}
	_, err := o.defaultNewBackend(context.Background(), other, "file:///state")
	if err == nil || !strings.Contains(err.Error(), "no stack module") {
		t.Fatalf("err = %v, want in-process path", err)
	}
	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want no plugin notice", stderr.String())
	}
}

func TestSubprocessProvenance(t *testing.T) {
	withArtifact := &options{
		inspectProvider: func(context.Context, string) (providerhost.Loaded, error) { return testLoaded(), nil },
		loadProvider: func(context.Context, string) (providerhost.Loaded, error) {
			t.Fatal("listing installed providers tried to acquire one")
			return providerhost.Loaded{}, nil
		},
	}
	provenance := withArtifact.subprocessProvenance(context.Background())
	if len(provenance) != 3 || provenance[0].Name != "magelift-provider-aws" || provenance[0].Mode != "subprocess" || provenance[2].Name != "magelift-provider-azure" || provenance[2].Mode != "subprocess" {
		t.Fatalf("provenance = %v", provenance)
	}
	got := provenance[1]
	if got.Name != "magelift-provider-gcp" || got.Version != "0.1.0" || got.Digest != "sha256:abc" || got.Mode != "subprocess" {
		t.Fatalf("provenance = %+v", got)
	}
	withoutArtifact := &options{
		inspectProvider: func(_ context.Context, name string) (providerhost.Loaded, error) {
			if name == "azure" {
				return providerhost.Loaded{}, providerhost.ErrChecksumMismatch
			}
			return providerhost.Loaded{}, providerhost.ErrNoLockfile
		},
	}
	provenance = withoutArtifact.subprocessProvenance(context.Background())
	if len(provenance) != 3 || provenance[0].Name != "magelift-provider-aws" || provenance[0].Mode != "not-installed" || provenance[0].Version != "" || provenance[1].Mode != "not-installed" || provenance[2].Mode != "unverified" {
		t.Fatalf("provenance = %v", provenance)
	}
}

func TestExtensionsListShowsProviderProvenance(t *testing.T) {
	var stdout bytes.Buffer
	o := &options{
		output:          "json",
		stdout:          &stdout,
		stderr:          io.Discard,
		modules:         platform.NewModuleRegistry(),
		inspectProvider: func(context.Context, string) (providerhost.Loaded, error) { return testLoaded(), nil },
	}
	root := extensionsCommand(o)
	root.SetArgs([]string{"list"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, want := range []string{`"providers"`, "magelift-provider-aws", "magelift-provider-gcp", "magelift-provider-azure", `"mode": "subprocess"`, `"version": "0.1.0"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

// TestOutputsDisplayRedactsThroughPluginBackend pins the display
// boundary: `magelift outputs` through a dialed session must serve the
// secret-flagged op, never decrypted day-2 outputs.
func TestOutputsDisplayRedactsThroughPluginBackend(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	var stdout, stderr bytes.Buffer
	var methods []string
	caller := &scriptedCaller{respond: func(method string, reply any) error {
		methods = append(methods, method)
		*(reply.(*sdk.OutputsResult)) = sdk.OutputsResult{
			ValuesJSON: []byte(`{"kubeconfig":"DECRYPTED-KUBECONFIG-SENTINEL"}`),
			SecretKeys: []string{"kubeconfig"},
		}
		return nil
	}}
	client := providerhost.NewTestClient(caller, testDescribe())
	o := testOptions(&stdout, &fakeTerminal{interactive: false})
	o.stderr = &stderr
	o.configPath, o.environment = path, "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		backend, err := providerhost.NewPluginBackend(client, sdk.Envelope{}, sdk.StoredPlan{Opaque: []byte(`{}`)})
		if err != nil {
			t.Fatal(err)
		}
		return backend, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "outputs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(methods) != 1 || methods[0] != "Plugin.Outputs" {
		t.Fatalf("methods = %v, want [Plugin.Outputs]", methods)
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
	if err == nil || !strings.Contains(err.Error(), "magelift.providers.lock") {
		t.Fatalf("err = %v, want lockfile failure", err)
	}
}
