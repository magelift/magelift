//go:build synthetic

// Package synthetic_test proves offline contracts with original synthetic
// fixtures. Mock inventory per step (real code vs fake; nothing here touches
// a network or a cloud API):
//
//   - Fixture load/validate: real internal/config Load plus Resolve.
//   - Routing: real platform.ModuleRegistry dispatch plus real first-party
//     module Plan for in-process targets (spec build and validation only;
//     no Pulumi execution). GCP dispatches to the plugin shim; planning
//     dials the plugin, so the offline suite proves dispatch plus typed
//     error surfacing with scripted sessions.
//   - Contracts: fake sdk.Module implementations through the real
//     RegisterPublicModule bridge plus descriptor validation.
//   - Importer: real paasimport.MapACC/MapUpsun over synthetic inputs; the
//     emitted YAML is validated with real config Load/Resolve.
//   - Local planning: real localdev.Plan plus ComposeTemplateFor over the
//     synthetic GCP preview fixture; no containers start.
//   - Failures: real validation and plan errors asserted by class.
//   - Credentials: refused by denylist before any case runs.
//
// Verdicts here cover routing, contracts, failures, validation, and offline
// planning only. Store-level proof belongs to bounded live acceptance.
package synthetic_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/localdev"
	"github.com/magelift/magelift/internal/paasimport"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/internal/registry"
	"github.com/magelift/magelift/sdk"
)

var deniedEnvVars = []string{
	"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE",
	"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_OAUTH_ACCESS_TOKEN",
	"OVH_API_APPLICATION_KEY", "OVH_API_APPLICATION_SECRET", "OVH_API_CONSUMER_KEY", "OVH_CONFIG_FILE",
	"SCW_ACCESS_KEY", "SCW_SECRET_KEY", "SCW_TOKEN", "SCW_CONFIG_PATH",
	"PULUMI_ACCESS_TOKEN", "PULUMI_CONFIG_PASSPHRASE",
	"MAGELIFT_ACCEPTANCE_TTL_SECONDS", "MAGELIFT_AWS_ACCEPTANCE_KEEP", "MAGELIFT_GCP_ACCEPTANCE_KEEP_ON_FAILURE",
}

func TestMain(m *testing.M) {
	for _, name := range deniedEnvVars {
		if os.Getenv(name) != "" {
			fmt.Fprintf(os.Stderr, "synthetic suite refuses credentials: %s is set\n", name)
			os.Exit(1)
		}
	}
	os.Exit(m.Run())
}

func fixturePath(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join("..", "fixtures", "synthetic", rel)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fixture %s: %v", rel, err)
	}
	return path
}

func loadFixture(t *testing.T, rel string) *config.File {
	t.Helper()
	data, err := os.ReadFile(fixturePath(t, rel))
	if err != nil {
		t.Fatal(err)
	}
	file, err := config.Load(data)
	if err != nil {
		t.Fatalf("load %s: %v", rel, err)
	}
	return file
}

func resolveFixture(t *testing.T, rel, environment string) config.Config {
	t.Helper()
	file := loadFixture(t, rel)
	effective, err := file.Resolve(environment, config.ResolveOptions{})
	if err != nil {
		t.Fatalf("resolve %s:%s: %v", rel, environment, err)
	}
	return effective.Config
}

// gaps maps live-only capabilities to their owning intent. Scenarios with no
// offline path fail through requireOffline instead of passing silently.
var gaps = map[string]string{
	"magento-index-query-reconnect": "reference-store-acceptance",
	"live-credential-expiry-ops":    "reference-store-acceptance",
	"live-backup-restore":           "reference-store-acceptance",
}

func ownerFor(capability string) (string, bool) {
	owner, ok := gaps[capability]
	return owner, ok
}

func requireOffline(t *testing.T, capability string) {
	t.Helper()
	if owner, ok := ownerFor(capability); ok {
		t.Fatalf("gap: %s owned by %s", capability, owner)
	}
	t.Fatalf("gap: %s has no owning intent; file it before merging", capability)
}

func TestGaps(t *testing.T) {
	t.Parallel()
	if len(gaps) == 0 {
		t.Fatal("gaps table must not be empty")
	}
	for capability, owner := range gaps {
		if strings.TrimSpace(owner) == "" {
			t.Errorf("gap %q has no owner", capability)
		}
		if got, ok := ownerFor(capability); !ok || got != owner {
			t.Errorf("ownerFor(%q) = %q, %v", capability, got, ok)
		}
	}
	if _, ok := ownerFor("definitely-not-a-capability"); ok {
		t.Error("ownerFor returned an owner for an unknown capability")
	}
}

func TestRouting_ValidFixturesPlan(t *testing.T) {
	t.Parallel()
	cfg := resolveFixture(t, "aws-preview/magelift.yaml", "preview")
	modules, err := registry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	module, _, err := modules.Plan(cfg, "preview", platform.PlanOptions{})
	if err != nil {
		t.Fatalf("plan aws-preview: %v", err)
	}
	if got := string(module.Descriptor().Provider); got != cfg.Target.Provider {
		t.Errorf("plan aws-preview dispatched to provider %q, want %q", got, cfg.Target.Provider)
	}
}

func TestRouting_GCPDispatchesToPluginShim(t *testing.T) {
	t.Parallel()
	cfg := resolveFixture(t, "gcp-preview/magelift.yaml", "preview")
	modules, err := registry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	module, found := modules.Module("gcp", "gke-autopilot")
	if !found {
		t.Fatal("gcp/gke-autopilot module is not registered")
	}
	if _, ok := module.(*providerhost.ShimModule); !ok {
		t.Fatalf("gcp module = %T, want *providerhost.ShimModule", module)
	}
	if got := string(module.Descriptor().Provider); got != cfg.Target.Provider {
		t.Errorf("gcp dispatched to provider %q, want %q", got, cfg.Target.Provider)
	}
	// Planning dials the plugin; the offline suite proves dispatch only.
	// Provider plan behavior is covered provider-side with fakes.
}

func TestRouting_UnknownTargetFailsClosed(t *testing.T) {
	t.Parallel()
	modules, err := registry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Target: config.Target{Provider: "atlantis", Runtime: "trench-cluster"}}
	_, _, err = modules.Plan(cfg, "preview", platform.PlanOptions{})
	if err == nil || !strings.Contains(err.Error(), "no stack module registered") {
		t.Fatalf("unknown target error = %v, want fail-closed dispatch refusal", err)
	}
}

type fakeModule struct {
	descriptor sdk.ExtensionDescriptor
	plan       func(sdk.ModulePlanRequest) (sdk.ModulePlan, error)
}

func (f fakeModule) Descriptor() sdk.ExtensionDescriptor { return f.descriptor }

func (f fakeModule) Plan(_ context.Context, request sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
	return f.plan(request)
}

func (f fakeModule) Program(sdk.ModulePlan) (any, error) {
	return nil, fmt.Errorf("synthetic fake never programs")
}

func syntheticTarget() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{
		ID:       "synthetic.synthetic-test",
		Provider: "synthetic",
		Runtime:  "synthetic-test",
	}
}

func TestContract_PublicModule(t *testing.T) {
	t.Parallel()
	target := syntheticTarget()
	valid := fakeModule{
		descriptor: sdk.ExtensionDescriptor{
			APIVersion: sdk.ExtensionAPIVersion,
			ID:         "synthetic-test",
			Version:    "0.0.0",
			Source:     "synthetic",
			Tier:       sdk.ExtensionTierExperimental,
			Targets:    []sdk.TargetDescriptor{target},
			OutputKeys: append([]string(nil), platform.RequiredOutputKeys()...),
		},
		plan: func(request sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
			return sdk.ModulePlan{
				StackName:        "synthetic-test",
				Provider:         target.Provider,
				Runtime:          target.Runtime,
				Project:          request.Project,
				Environment:      request.Environment,
				Region:           request.Region,
				EnvironmentClass: request.EnvironmentClass,
				Target:           target,
				Tier:             sdk.ExtensionTierExperimental,
				Opaque:           map[string]any{},
			}, nil
		},
	}
	modules := platform.NewModuleRegistry()
	if err := modules.RegisterPublicModule(valid); err != nil {
		t.Fatalf("register valid fake: %v", err)
	}

	invalidDescriptor := valid
	invalidDescriptor.descriptor.Targets = nil
	if err := platform.NewModuleRegistry().RegisterPublicModule(invalidDescriptor); err == nil {
		t.Fatal("register with zero targets passed, want descriptor rejection")
	}

	mismatched := valid
	mismatched.plan = func(request sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
		plan, err := valid.plan(request)
		if err != nil {
			return plan, err
		}
		plan.Target = sdk.TargetDescriptor{ID: "other.other", Provider: "other", Runtime: "other"}
		return plan, nil
	}
	mismatchedRegistry := platform.NewModuleRegistry()
	if err := mismatchedRegistry.RegisterPublicModule(mismatched); err != nil {
		t.Fatalf("register mismatched fake: %v", err)
	}
	cfg := resolveFixture(t, "gcp-preview/magelift.yaml", "preview")
	cfg.Target.Provider = string(target.Provider)
	cfg.Target.Runtime = string(target.Runtime)
	if _, _, err := mismatchedRegistry.Plan(cfg, "preview", platform.PlanOptions{}); err == nil {
		t.Fatal("mismatched plan passed, want invalid-plan rejection")
	}

	if _, _, err := modules.Plan(cfg, "preview", platform.PlanOptions{}); err != nil {
		t.Fatalf("valid fake plan: %v", err)
	}
}

func TestFailure_InvalidFixtures(t *testing.T) {
	t.Parallel()
	// Mirrors the CLI validate path: Load, then ResolveBuild. Each fixture
	// must fail with exactly its documented class.
	cases := []struct {
		rel     string
		wantErr string
	}{
		{"invalid/unknown-provider.yaml", "target.provider must be aws, gcp, ovh, or scaleway"},
		{"invalid/unsupported-version.yaml", "must be an exact Magento release"},
	}
	for _, tc := range cases {
		data, err := os.ReadFile(fixturePath(t, tc.rel))
		if err != nil {
			t.Fatal(err)
		}
		file, err := config.Load(data)
		if err != nil {
			t.Fatalf("%s load error = %v", tc.rel, err)
		}
		_, err = file.ResolveBuild()
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s ResolveBuild error = %v, want %q", tc.rel, err, tc.wantErr)
		}
	}
}

func TestCoreAcceptsSemanticallyEmptyGCPTarget(t *testing.T) {
	t.Parallel()
	// Split enforcement: core config accepts structural presence; the
	// provider's ValidateConfig rejects the missing project (provider
	// schema suite covers the rejection on the equivalent target).
	data, err := os.ReadFile(fixturePath(t, "invalid/missing-required.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := config.Load(data)
	if err != nil {
		t.Fatalf("load error = %v", err)
	}
	if _, err := file.ResolveBuild(); err != nil {
		t.Fatalf("core rejected provider-owned semantics: %v", err)
	}
}

func TestFailure_ExpiredPreviewRefusesPlan(t *testing.T) {
	t.Parallel()
	cfg := resolveFixture(t, "invalid/expired-preview.yaml", "preview")
	// The plugin refuses expired previews server-side; the shim surfaces
	// the typed refusal without interpreting it.
	client := providerhost.NewTestClient(scriptedCaller(func(method string, reply any) error {
		if method != "Plugin.Plan" {
			return errors.New("unexpected method " + method)
		}
		*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Error: &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: "preview expired at 2020-01-01"}}
		return nil
	}), nil)
	module, err := providerhost.NewShimModule("gke-autopilot", client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = module.Plan(cfg, "preview", platform.PlanOptions{})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "expired") {
		t.Fatalf("expired preview plan error = %v, want expiry refusal", err)
	}
}

type scriptedCaller func(method string, reply any) error

func (f scriptedCaller) Call(method string, _ any, reply any) error {
	return f(method, reply)
}

func unmappedPaths(t *testing.T, keys []paasimport.UnmappedKey) map[string]bool {
	t.Helper()
	paths := map[string]bool{}
	for _, key := range keys {
		paths[key.Path] = true
	}
	return paths
}

func TestValidation_Importer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		root  string
		mapFn func(string) (paasimport.Result, error)
	}{
		{"ACC", "imports/acc", paasimport.MapACC},
		{"Upsun", "imports/upsun", paasimport.MapUpsun},
	} {
		result, err := tc.mapFn(fixturePath(t, tc.root))
		if err != nil {
			t.Fatalf("%s map: %v", tc.name, err)
		}
		if _, err := config.Load(result.YAML); err != nil {
			t.Fatalf("%s emitted YAML invalid: %v", tc.name, err)
		}
		paths := unmappedPaths(t, result.Unmapped)
		for _, want := range []string{"custom_stage", "relationships.analytics", "analytics"} {
			if !paths[want] {
				t.Errorf("%s unmapped keys %v lack %q", tc.name, result.Unmapped, want)
			}
		}
	}
}

func TestValidation_ImporterVersions(t *testing.T) {
	t.Parallel()
	badType := t.TempDir()
	writeTestFile(t, badType, ".magento.app.yaml", "name: syntheticbad\ntype: nodejs:20\n")
	if _, err := paasimport.MapACC(badType); err == nil || !strings.Contains(err.Error(), "unsupported app type") {
		t.Fatalf("malformed app type error = %v, want unsupported-app-type rejection", err)
	}

	oldPHP := t.TempDir()
	writeTestFile(t, oldPHP, ".magento.app.yaml", "name: synthetic74\ntype: php:7.4\n")
	result, err := paasimport.MapACC(oldPHP)
	if err != nil {
		t.Fatalf("php:7.4 map: %v", err)
	}
	file, err := config.Load(result.YAML)
	if err != nil {
		t.Fatalf("php:7.4 emitted YAML load: %v", err)
	}
	if _, err := file.Resolve("staging", config.ResolveOptions{}); err == nil || !strings.Contains(err.Error(), "build.php") {
		t.Fatalf("php:7.4 resolve error = %v, want version rejection (nothing substituted)", err)
	}
}

func writeTestFile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLocalPlanning(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(fixturePath(t, "gcp-preview/magelift.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := config.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := file.ResolveBuild()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := localdev.Plan(spec)
	if err != nil {
		t.Fatalf("localdev plan: %v", err)
	}
	if !strings.Contains(plan.AppImage, "php-nginx") {
		t.Errorf("AppImage = %q, want the nginx runtime image", plan.AppImage)
	}
	rendered := localdev.ComposeTemplateFor(plan)
	for _, want := range []string{"mailpit", "MAGELIFT_LOCAL_EMAIL_DISABLE", "magelift/php-nginx"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendered compose lacks %q", want)
		}
	}
}
