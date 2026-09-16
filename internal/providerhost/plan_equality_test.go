package providerhost

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	gcpstack "github.com/magelift/magelift/internal/cloud/gcp/stack"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

// dialEqualityConfig mirrors the gcp stack package's deployment fixture. It
// is replicated (not imported) because test helpers do not cross packages;
// if PlanFromConfig gains required fields this test fails loudly, which is
// the point of the pin.
func dialEqualityConfig() config.Config {
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

func TestDialPlanEqualsInProcessPlan(t *testing.T) {
	cfg := dialEqualityConfig()
	// Configuration carries the JSON form of config.Config, matching the
	// platform.configMap contract the host uses to build requests.
	payload, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var configuration map[string]any
	if err := json.Unmarshal(payload, &configuration); err != nil {
		t.Fatal(err)
	}
	request := sdk.ModulePlanRequest{
		Project: "shop", Environment: "staging", Region: "europe-west1",
		Configuration: configuration,
	}

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
	got, err := session.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	planned, err := gcpstack.Module{}.Plan(cfg, "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		t.Fatalf("in-process plan type = %T", planned)
	}
	tier := sdk.ExtensionTierExperimental
	if planned.CertificationTier() == platform.TierCertified {
		tier = sdk.ExtensionTierCertified
	}
	want := sdk.ModulePlan{
		StackName:        planned.StackName(),
		Provider:         planned.Provider(),
		Runtime:          planned.Runtime(),
		Project:          planned.Project(),
		Environment:      planned.Environment(),
		Region:           planned.Region(),
		EnvironmentClass: planned.EnvironmentClass(),
		Protected:        planned.Protected(),
		ImageDigest:      planned.ImageDigest(),
		Target:           planned.TargetDescriptor(),
		Tier:             tier,
		Opaque:           gcpPlanned.Spec,
	}

	// Compare as decoded JSON: the subprocess Opaque arrives as a map
	// while the in-process one is a struct, so direct DeepEqual would
	// false-fail on Go types that encode identically.
	decode := func(t *testing.T, value sdk.ModulePlan) any {
		t.Helper()
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		return decoded
	}
	wantAny, gotAny := decode(t, want), decode(t, got)
	if !reflect.DeepEqual(wantAny, gotAny) {
		wantJSON, _ := json.MarshalIndent(wantAny, "", "  ")
		gotJSON, _ := json.MarshalIndent(gotAny, "", "  ")
		t.Fatalf("subprocess plan differs from in-process plan:\nwant: %s\ngot: %s", wantJSON, gotJSON)
	}
}
