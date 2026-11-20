//go:build synthetic

// Package synthetic_test checks shop configuration, provider dispatch, PaaS
// imports and local Compose planning with fixtures and scripted plugin sessions.
package synthetic_test

import (
	"context"
	"encoding/json"
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
	"github.com/magelift/magelift/internal/projectphp"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/internal/registry"
	"github.com/magelift/magelift/sdk"
)

var deniedEnvVars = []string{
	"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE",
	"GOOGLE_APPLICATION_CREDENTIALS", "GOOGLE_OAUTH_ACCESS_TOKEN",
	"AZURE_CLIENT_SECRET", "AZURE_FEDERATED_TOKEN_FILE", "ARM_CLIENT_SECRET",
	"PULUMI_ACCESS_TOKEN", "PULUMI_CONFIG_PASSPHRASE",
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
	var data []byte
	var err error
	if strings.HasSuffix(rel, ".php") {
		data, err = projectphp.Evaluate(t.Context(), fixturePath(t, rel))
	} else {
		data, err = os.ReadFile(fixturePath(t, rel))
	}
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

func TestRouting_ValidFixturesPlan(t *testing.T) {
	t.Parallel()
	cfg := resolveFixture(t, "aws-preview/magelift.php", "preview")
	modules, err := registry.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(modules.Close)
	module, found := modules.Module("aws", sdk.RuntimeID(cfg.Target.Runtime))
	if !found {
		t.Fatalf("AWS runtime %q is not registered", cfg.Target.Runtime)
	}
	if _, ok := module.(*providerhost.ShimModule); !ok {
		t.Fatalf("AWS module = %T, want *providerhost.ShimModule", module)
	}
	if got := string(module.Descriptor().Provider); got != cfg.Target.Provider {
		t.Errorf("plan aws-preview dispatched to provider %q, want %q", got, cfg.Target.Provider)
	}
	// Planning dials the provider; this offline suite covers dispatch only.
}

func TestRouting_GCPDispatchesToPluginShim(t *testing.T) {
	t.Parallel()
	cfg := resolveFixture(t, "gcp-preview/magelift.php", "preview")
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
	cfg := resolveFixture(t, "gcp-preview/magelift.php", "preview")
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
		{"invalid/unknown-provider.yaml", "target.provider"},
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
	module, err := providerhost.NewShimModule("gcp", "gke-autopilot", client)
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

func importedFixture(t *testing.T, root string, kind paasimport.SourceKind) ([]byte, []paasimport.UnmappedKey, error) {
	t.Helper()
	data, err := projectphp.Evaluate(t.Context(), filepath.Join("..", "fixtures", "ci", "aws", "magelift.php"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	src, err := paasimport.Read(root, kind, "")
	if err != nil {
		return nil, nil, err
	}
	residuals, err := src.Apply(doc)
	if err != nil {
		return nil, nil, err
	}
	data, err = json.Marshal(doc)
	return data, residuals, err
}

func TestValidation_Importer(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		root string
		kind paasimport.SourceKind
	}{
		{"ACC", "imports/acc", paasimport.SourceACC},
		{"Upsun", "imports/upsun", paasimport.SourceUpsun},
	} {
		data, keys, err := importedFixture(t, fixturePath(t, tc.root), tc.kind)
		if err != nil {
			t.Fatalf("%s map: %v", tc.name, err)
		}
		if _, err := config.Load(data); err != nil {
			t.Fatalf("%s emitted YAML invalid: %v", tc.name, err)
		}
		paths := unmappedPaths(t, keys)
		for _, want := range []string{"custom_stage", "relationships.analytics", "analytics"} {
			if !paths[want] {
				t.Errorf("%s unmapped keys %v lack %q", tc.name, keys, want)
			}
		}
	}
}

func TestValidation_ImporterVersions(t *testing.T) {
	t.Parallel()
	badType := t.TempDir()
	writeTestFile(t, badType, ".magento.app.yaml", "name: syntheticbad\ntype: nodejs:20\n")
	if _, _, err := importedFixture(t, badType, paasimport.SourceACC); err == nil || !strings.Contains(err.Error(), "unsupported app type") {
		t.Fatalf("malformed app type error = %v, want unsupported-app-type rejection", err)
	}

	oldPHP := t.TempDir()
	writeTestFile(t, oldPHP, ".magento.app.yaml", "name: synthetic74\ntype: php:7.4\n")
	data, _, err := importedFixture(t, oldPHP, paasimport.SourceACC)
	if err != nil {
		t.Fatalf("php:7.4 map: %v", err)
	}
	file, err := config.Load(data)
	if err != nil {
		t.Fatalf("php:7.4 emitted YAML load: %v", err)
	}
	effective, err := file.Resolve("staging", config.ResolveOptions{})
	if err != nil || effective.Config.Build.PHP != "7.4" {
		t.Fatalf("imported PHP version = %q, error = %v", effective.Config.Build.PHP, err)
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
	data, err := projectphp.Evaluate(t.Context(), fixturePath(t, "gcp-preview/magelift.php"))
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
