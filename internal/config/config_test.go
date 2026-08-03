package config

import (
	"strings"
	"testing"
)

const base = `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8-p5, mode: integrated}
build:
  php: "8.3"
  composer: {credentials: aws-secrets-manager://composer/auth}
  staticContent: {locales: [en_US, fr_FR], strategy: compact}
target: {provider: aws, runtime: ecs-fargate}
defaults: {region: eu-west-3, preset: preview}
environments:
  shared:
    build:
      staticContent: {strategy: standard}
  staging:
    inherits: shared
    account: "123"
    build:
      staticContent: {locales: [de_DE], strategy: null}
extensions:
  vendor.example: {anything: true}
`

func TestResolveMergeAndProvenance(t *testing.T) {
	f, err := Load([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{
		Builtins:  map[string]any{"defaults": map[string]any{"region": "us-east-1"}},
		Presets:   map[string]map[string]any{"preview": {"build": map[string]any{"staticContent": map[string]any{"locales": []any{"en_US"}}}}},
		Overrides: map[string]any{"defaults": map[string]any{"region": "us-west-2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := effective.Config.Defaults.Region; got != "us-west-2" {
		t.Fatalf("region = %q", got)
	}
	if got := effective.Config.Application.WebRuntime; got != "nginx-fpm" {
		t.Fatalf("web runtime default = %q", got)
	}
	if got := effective.Provenance["application.webRuntime"].Source; got != "built-in defaults" {
		t.Fatalf("web runtime provenance = %q", got)
	}
	locales := effective.Config.Build.StaticContent.Locales
	if len(locales) != 1 || locales[0] != "de_DE" {
		t.Fatalf("lists were not replaced: %#v", locales)
	}
	if effective.Config.Build.StaticContent.Strategy != "" {
		t.Fatal("null did not remove inherited value")
	}
	if got := effective.Provenance["defaults.region"].Source; got != "CLI override" {
		t.Fatalf("provenance = %q", got)
	}
	if !effective.Provenance["build.staticContent.strategy"].Removed {
		t.Fatal("removal provenance missing")
	}
}

func TestResolveAppliesBoundedDefaultsWhenAWSTargetIsConfigured(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	aws := effective.Config.Target.AWS
	if aws == nil || aws.Catalog.Versions.OpenSearch != "OpenSearch_3.1" || aws.Catalog.Versions.Valkey != "8.1" {
		t.Fatalf("compatibility defaults = %#v", aws)
	}
	if aws.Catalog.Fargate.DesiredCount != 1 || !aws.Catalog.Search.AcceptColdStarts {
		t.Fatalf("preview defaults = %#v", aws.Catalog)
	}
	if got := effective.Provenance["target.aws.catalog.versions.openSearch"].Source; got != "compatibility defaults" {
		t.Fatalf("compatibility provenance = %q", got)
	}
	if got := effective.Provenance["target.aws.catalog.fargate.desiredCount"].Source; got != "preset preview" {
		t.Fatalf("preset provenance = %q", got)
	}
}

func TestResolveDoesNotInventAWSTargetForMinimalConfig(t *testing.T) {
	f, err := Load([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Target.AWS != nil {
		t.Fatal("minimal config unexpectedly received an AWS target")
	}
}

func TestResolvePreservesExplicitOptionsWhenUsingDefaultPresets(t *testing.T) {
	f, err := Load([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{
		Builtins:      map[string]any{"defaults": map[string]any{"region": "us-east-1"}},
		Compatibility: map[string]any{"compatibility": map[string]any{"allowUnsupported": true}},
		Overrides:     map[string]any{"defaults": map[string]any{"region": "us-west-2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Defaults.Region != "us-west-2" || !effective.Config.Compatibility.AllowUnsupported {
		t.Fatalf("explicit resolver options were lost: %#v", effective.Config)
	}
}

func TestStrictUnknownKey(t *testing.T) {
	_, err := Load([]byte(strings.Replace(base, "project: {name: shop}", "project: {name: shop, typo: true}", 1)))
	if err == nil || !strings.Contains(err.Error(), "field typo not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSeedDumpPathLoadsAndResolves(t *testing.T) {
	input := strings.Replace(base, "    account: \"123\"\n", "    account: \"123\"\n    seedDump: /tmp/fixture.sql\n", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatalf("Load with seedDump: %v", err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve with seedDump: %v", err)
	}
	if effective.Config.SeedDump != "/tmp/fixture.sql" {
		t.Fatalf("SeedDump = %q", effective.Config.SeedDump)
	}
}

func TestSeedDumpStatusYAMLFieldRejected(t *testing.T) {
	input := strings.Replace(base, "    account: \"123\"\n", "    account: \"123\"\n    seedDumpStatus: recorded\n", 1)
	_, err := Load([]byte(input))
	if err == nil || !strings.Contains(err.Error(), "field seedDumpStatus not found") {
		t.Fatalf("seedDumpStatus must remain journal-only, got: %v", err)
	}
}

func TestUnknownExtensionKeysAllowed(t *testing.T) {
	if _, err := Load([]byte(base)); err != nil {
		t.Fatal(err)
	}
}

func TestBuildHooksAreTypedAndValidated(t *testing.T) {
	input := strings.Replace(base, "  shared:\n    build:\n      staticContent: {strategy: standard}\n", "  shared: {}\n", 1)
	input = strings.Replace(input, "    build:\n      staticContent: {locales: [de_DE], strategy: null}\n", "", 1)
	input = strings.Replace(input, "build:\n  php: \"8.3\"", "build:\n  php: \"8.3\"\n  hooks:\n    build.prepare:\n      phase: build\n      relationship: before\n      target: build\n      command:\n        executable: composer\n        arguments: [run-script, prepare]", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := f.ResolveBuild()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Build.Hooks["build.prepare"].Command == nil {
		t.Fatal("hook command was not decoded")
	}
}

func TestBuildHooksRejectUnsafeCommandsAndRetries(t *testing.T) {
	input := strings.Replace(base, "  shared:\n    build:\n      staticContent: {strategy: standard}\n", "  shared: {}\n", 1)
	input = strings.Replace(input, "    build:\n      staticContent: {locales: [de_DE], strategy: null}\n", "", 1)
	input = strings.Replace(input, "build:\n  php: \"8.3\"", "build:\n  php: \"8.3\"\n  hooks:\n    build.prepare:\n      phase: build\n      relationship: before\n      target: build\n      command:\n        executable: sh\n        arguments: [\"echo unsafe\"]\n      retries:\n        maxAttempts: 2", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.ResolveBuild(); err == nil || !strings.Contains(err.Error(), "executable") || !strings.Contains(err.Error(), "idempotent") {
		t.Fatalf("unexpected hook validation error: %v", err)
	}
}

func TestRejectsUnnamespacedExtension(t *testing.T) {
	f, err := Load([]byte(strings.Replace(base, "vendor.example", "example", 1)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "namespaced identifier") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInheritanceCycle(t *testing.T) {
	input := strings.Replace(base, "  shared:\n", "  loop: {inherits: staging}\n  shared:\n    inherits: loop\n", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRejectsPlaintextSecret(t *testing.T) {
	f, err := Load([]byte(strings.Replace(base, "aws-secrets-manager://composer/auth", "hunter2", 1)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "plaintext") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRejectsUnsupportedOrMalformedSecretReference(t *testing.T) {
	for _, reference := range []string{"vault-secrets://composer/auth", "ssm://parameter?withDecryption=false"} {
		f, err := Load([]byte(strings.Replace(base, "aws-secrets-manager://composer/auth", reference, 1)))
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Resolve("staging", ResolveOptions{})
		if err == nil || !strings.Contains(err.Error(), "valid secret reference") {
			t.Fatalf("reference %q returned %v", reference, err)
		}
	}
}

func TestRejectsGCPSecretSchemeOnAWSTarget(t *testing.T) {
	f, err := Load([]byte(strings.Replace(base, "aws-secrets-manager://composer/auth", "gcp-secret-manager://projects/p/secrets/s/versions/latest", 1)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "aws-secrets-manager:// or ssm://") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAcceptsGCPSecretSchemeOnGCPTarget(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: gcp, runtime: gke-autopilot, gcp: {project: p, region: europe-west1}}", 1)
	input = strings.Replace(input, "aws-secrets-manager://composer/auth", "gcp-secret-manager://projects/p/secrets/s/versions/latest", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Resolve("staging", ResolveOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRejectsAWSSecretSchemeOnGCPTarget(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: gcp, runtime: gke-autopilot, gcp: {project: p, region: europe-west1}}", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "gcp-secret-manager://") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnvironmentNamesSorted(t *testing.T) {
	f, _ := Load([]byte(base))
	got := strings.Join(f.Environments(), ",")
	if got != "shared,staging" {
		t.Fatalf("got %q", got)
	}
}

func TestEnvironmentForBranch(t *testing.T) {
	f, err := Load([]byte(strings.Replace(base, "  staging:\n", "  staging:\n    branches: [main]\n", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if got, found, err := f.EnvironmentForBranch("main"); err != nil || !found || got != "staging" {
		t.Fatalf("got environment=%q found=%v error=%v", got, found, err)
	}
	if _, found, err := f.EnvironmentForBranch("feature/no-map"); err != nil || found {
		t.Fatalf("unexpected match: found=%v error=%v", found, err)
	}
}

func TestEnvironmentForBranchRejectsAmbiguousMapping(t *testing.T) {
	input := strings.Replace(base, "  shared:\n", "  shared:\n    branches: [main]\n", 1)
	input = strings.Replace(input, "  staging:\n", "  staging:\n    branches: [main]\n", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.EnvironmentForBranch("main"); err == nil || !strings.Contains(err.Error(), "multiple environments") {
		t.Fatalf("unexpected error: %v", err)
	}
}
