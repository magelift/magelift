package paasimport

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
)

func TestMapACCSupportedFixture(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "acc", "supported")
	result, err := MapACC(root)
	if err != nil {
		t.Fatalf("MapACC: %v", err)
	}
	if len(result.Unmapped) != 0 {
		t.Fatalf("supported ACC must have zero unmapped, got %#v", result.Unmapped)
	}
	if strings.Contains(string(result.YAML), "synthetic-crypt-must-not-leak") {
		t.Fatal("crypt plaintext leaked into generated magelift.yaml")
	}
	file, err := config.Load(result.YAML)
	if err != nil {
		t.Fatalf("config.Load: %v\nYAML:\n%s", err, result.YAML)
	}
	envs := file.Environments()
	if len(envs) == 0 {
		t.Fatal("expected at least one environment")
	}
	if _, err := file.ResolveBuild(); err != nil {
		t.Fatalf("ResolveBuild: %v", err)
	}
	var effective config.Effective
	for _, env := range envs {
		resolved, err := file.Resolve(env, config.ResolveOptions{})
		if err != nil {
			t.Fatalf("Resolve(%s): %v", env, err)
		}
		effective = resolved
	}
	text := string(result.YAML)
	for _, want := range []string{
		"name: sample-shop",
		`php: "8.5"`,
		"strategy: compact",
		"encryptionKeySecretArn:",
		"domain: staging.example.com",
		"frontName: backstage",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated YAML missing %q:\n%s", want, text)
		}
	}
	magento := effective.Config.Application.Magento
	if magento.FrontName != "backstage" || magento.CookieDomain != ".shop.example" {
		t.Fatalf("magento overlays = %#v", magento)
	}
	if magento.Consumers.Mode != "processes" || len(magento.Consumers.Names) == 0 {
		t.Fatalf("consumers = %#v", magento.Consumers)
	}
	if len(magento.CORSOrigins) == 0 || magento.CORSOrigins[0] != "https://storefront.example" {
		t.Fatalf("cors = %#v", magento.CORSOrigins)
	}
}

func TestMapUpsunSupportedFixture(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "upsun", "supported")
	result, err := MapUpsun(root)
	if err != nil {
		t.Fatalf("MapUpsun: %v", err)
	}
	if len(result.Unmapped) != 0 {
		t.Fatalf("supported Upsun fixture must have zero unmapped keys, got %#v", result.Unmapped)
	}
	if strings.Contains(string(result.YAML), "upsun-crypt-must-not-leak") {
		t.Fatal("crypt plaintext leaked into generated magelift.yaml")
	}
	file, err := config.Load(result.YAML)
	if err != nil {
		t.Fatalf("config.Load: %v\nYAML:\n%s", err, result.YAML)
	}
	if _, err := file.ResolveBuild(); err != nil {
		t.Fatalf("ResolveBuild: %v", err)
	}
	envs := file.Environments()
	if len(envs) == 0 {
		t.Fatal("expected at least one environment")
	}
	effective, err := file.Resolve(envs[0], config.ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve(%s): %v", envs[0], err)
	}
	text := string(result.YAML)
	for _, want := range []string{
		"name: upsun-sample-shop",
		`php: "8.5"`,
		"strategy: standard",
		"threads: 4",
		"encryptionKeySecretArn:",
		"searchMode:",
		"queueMode:",
		"domain: preview.example.com",
		"frontName: admin_upsun",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated YAML missing %q:\n%s", want, text)
		}
	}
	magento := effective.Config.Application.Magento
	if magento.FrontName != "admin_upsun" || magento.CookieDomain != ".preview.example.com" {
		t.Fatalf("magento overlays = %#v", magento)
	}
	if magento.Consumers.Mode != "both" {
		t.Fatalf("consumers = %#v", magento.Consumers)
	}
	if len(magento.CORSOrigins) == 0 || magento.CORSOrigins[0] != "https://storefront.preview.example.com" {
		t.Fatalf("cors = %#v", magento.CORSOrigins)
	}
}

func TestMapACCUnmappedFixture(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "acc", "unmapped")
	result, err := MapACC(root)
	if err != nil {
		t.Fatalf("MapACC: %v", err)
	}
	if len(result.Unmapped) == 0 {
		t.Fatal("expected unmapped keys for ACC unmapped fixture")
	}
	if _, err := config.Load(result.YAML); err != nil {
		t.Fatalf("best-effort YAML must still Load: %v\n%s", err, result.YAML)
	}
	report := RenderUnmappedReport(result.Unmapped)
	for _, want := range []string{
		"hooks.build",
		"hooks.deploy",
		"crons.shell-cleanup",
		"CUSTOM_FEATURE_FLAG",
		"WEIRD_DEPLOY_HOOK",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("unmapped report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(string(result.YAML), "synthetic-crypt-must-not-leak") {
		t.Fatal("crypt plaintext leaked")
	}
}

func TestMapUpsunUnmappedFixture(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "upsun", "unmapped")
	result, err := MapUpsun(root)
	if err != nil {
		t.Fatalf("MapUpsun: %v", err)
	}
	if len(result.Unmapped) == 0 {
		t.Fatal("expected unmapped keys")
	}
	report := RenderUnmappedReport(result.Unmapped)
	for _, want := range []string{
		"hooks.build",
		"crons.shell-backup",
		"PLATFORM_CUSTOM_VAR",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("unmapped report missing %q:\n%s", want, report)
		}
	}
}

func TestAllowlistEnvKeys(t *testing.T) {
	for _, key := range []string{
		"CRYPT_KEY",
		"SCD_STRATEGY",
		"SCD_THREADS",
		"UPDATE_URLS",
		"CONFIG__DEFAULT__ADMIN__URL__FRONT_NAME",
		"CONFIG__DEFAULT__WEB__COOKIE__COOKIE_DOMAIN",
		"CONFIG__DEFAULT__WEB__CORS__ORIGINS",
		"CONFIG__DEFAULT__WEB__GRAPHQL__CORS_ORIGINS",
		"CONFIG__DEFAULT__CRON__CONSUMERS_RUNNER__MODE",
		"CONFIG__DEFAULT__CRON__CONSUMERS_RUNNER__CONSUMERS",
	} {
		if !IsAllowlistedEnv(key) {
			t.Fatalf("%s must be on D-07 allowlist", key)
		}
	}
	if IsAllowlistedEnv("CUSTOM_FEATURE_FLAG") {
		t.Fatal("CUSTOM_FEATURE_FLAG must not be allowlisted")
	}
}

func TestFastlyServiceEmitsMigrationIntentWithoutSecrets(t *testing.T) {
	edge, unmapped := fastlyEdgeIntent(".magento/services.yaml", map[string]any{
		"fastly": map[string]any{"type": "fastly:1.0"},
	})
	if edge == nil || edge.ExternalProvider != "fastly" {
		t.Fatalf("edge intent = %#v", edge)
	}
	if len(unmapped) != 2 || unmapped[0].Path != "fastly.serviceId" || unmapped[1].Path != "fastly.tokenSecret" {
		t.Fatalf("unmapped Fastly setup = %#v", unmapped)
	}
	if !serviceMapped("fastly", "fastly:1.0") {
		t.Fatal("Fastly service was not recognized")
	}
}

func TestMapsQualityPatchesFromEnvYaml(t *testing.T) {
	doc := baseDocument("sample-shop", "8.5")
	env := accEnvDocument{}
	env.Stage.Build = map[string]any{
		"QUALITY_PATCHES": []any{"ACSD-123", "MAGETWO-67097"},
		"SCD_STRATEGY":    "compact",
	}
	unmapped := applyAllowlistEnv(&doc, ".magento.env.yaml", env)
	if len(unmapped) != 0 {
		t.Fatalf("unmapped = %#v", unmapped)
	}
	if got := strings.Join(doc.Build.QualityPatches, ","); got != "ACSD-123,MAGETWO-67097" {
		t.Fatalf("qualityPatches = %#v", doc.Build.QualityPatches)
	}
	yamlBytes, err := emitMagelift(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(yamlBytes), "qualityPatches:") || !strings.Contains(string(yamlBytes), "ACSD-123") {
		t.Fatalf("generated YAML missing qualityPatches:\n%s", yamlBytes)
	}
}

func TestMapsObservabilityIntentWithoutCopyingCredentials(t *testing.T) {
	doc := baseDocument("sample-shop", "8.5")
	env := accEnvDocument{}
	env.Stage.Deploy = map[string]string{
		"NEW_RELIC_LICENSE_KEY": "synthetic-license-must-not-leak",
		"NEW_RELIC_APP_NAME":    "magento-staging",
	}
	unmapped := applyAllowlistEnv(&doc, ".magento.env.yaml", env)
	if doc.Observability == nil || doc.Observability.ExternalProvider != "newrelic" || !doc.Observability.Metrics || doc.Observability.ServiceName != "magento-staging" {
		t.Fatalf("observability intent = %#v", doc.Observability)
	}
	if len(unmapped) != 1 || unmapped[0].Path != "stage.deploy.NEW_RELIC_LICENSE_KEY" {
		t.Fatalf("credential residual = %#v", unmapped)
	}
	yamlBytes, err := emitMagelift(doc)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(yamlBytes), "synthetic-license-must-not-leak") {
		t.Fatal("observability credential leaked into generated YAML")
	}
}

func TestMapsObservabilityServiceIntent(t *testing.T) {
	intent := observabilityServiceIntent(map[string]any{
		"datadog": map[string]any{"type": "datadog:1"},
	})
	if intent == nil || intent.ExternalProvider != "datadog" || !intent.Logs || !intent.Metrics || !intent.Traces {
		t.Fatalf("service observability intent = %#v", intent)
	}
}

func TestMapsPaaSBuildRequirements(t *testing.T) {
	doc, unmapped, err := mapAppTree(".magento.app.yaml", accAppRaw{
		"name":    "sample-shop",
		"type":    "php:8.5",
		"runtime": map[string]any{"extensions": []any{"redis", "apcu"}},
		"dependencies": map[string]any{
			"php": map[string]any{"composer/composer": "2.10"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(unmapped) != 0 {
		t.Fatalf("build requirements were reported as unmapped: %#v", unmapped)
	}
	if strings.Join(doc.Build.Extensions, ",") != "apcu,redis" || doc.Build.Composer == nil || doc.Build.Composer.Version != "2.10" {
		t.Fatalf("build requirements = %#v", doc.Build)
	}
}

func TestPaaSDisabledExtensionsRemainExplicitlyUnmapped(t *testing.T) {
	_, unmapped, err := mapAppTree(".platform.app.yaml", accAppRaw{
		"name":    "sample-shop",
		"type":    "php:8.5",
		"runtime": map[string]any{"disabled_extensions": []any{"xdebug"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(unmapped) != 1 || unmapped[0].Path != "runtime.disabled_extensions" {
		t.Fatalf("disabled extension residual = %#v", unmapped)
	}
}

func TestSidecarPath(t *testing.T) {
	if got := SidecarPath("/tmp/magelift.yaml"); got != "/tmp/magelift.unmapped.md" {
		t.Fatalf("SidecarPath magelift.yaml = %q", got)
	}
	if got := SidecarPath("/tmp/review.magelift.yaml"); got != "/tmp/review.magelift.unmapped.md" {
		t.Fatalf("SidecarPath config-out = %q", got)
	}
}

func TestFullyMappedNeverWritesSidecarContent(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "acc", "supported")
	result, err := MapACC(root)
	if err != nil {
		t.Fatalf("MapACC: %v", err)
	}
	if result.HasUnmapped() {
		t.Fatal("fully mapped result must not report unmapped keys")
	}
}

func TestDetectACCRequiresAppYAML(t *testing.T) {
	dir := t.TempDir()
	if err := DetectACC(dir); err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestDetectUpsunRequiresAppYAML(t *testing.T) {
	dir := t.TempDir()
	if err := DetectUpsun(dir); err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s: %v", root, err)
	}
	return root
}
