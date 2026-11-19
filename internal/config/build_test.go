package config

import (
	"strings"
	"testing"
)

func TestResolveBuildUsesProjectInputsWithoutEnvironment(t *testing.T) {
	input := strings.Replace(base, "  shared:\n    build:\n      staticContent: {strategy: standard}\n", "  shared: {}\n", 1)
	input = strings.Replace(input, "    build:\n      staticContent: {locales: [de_DE], strategy: null}\n", "", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	spec, err := f.ResolveBuild()
	if err != nil {
		t.Fatal(err)
	}
	if spec.Application.WebRuntime != "nginx-fpm" {
		t.Fatalf("web runtime = %q", spec.Application.WebRuntime)
	}
	if spec.Build.PHP != "8.3" || spec.Compatibility.Status != CompatibilitySupported {
		t.Fatalf("unexpected build spec: %#v", spec)
	}
}

func TestProjectDefaultsResolveWithoutEnvironment(t *testing.T) {
	input := strings.Replace(base, "  shared:\n    build:\n      staticContent: {strategy: standard}\n", "  shared: {}\n", 1)
	input = strings.Replace(input, "    build:\n      staticContent: {locales: [de_DE], strategy: null}\n", "", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := f.ProjectDefaults()
	if err != nil {
		t.Fatal(err)
	}
	if defaults.Region != "eu-west-3" || defaults.Preset != "preview" {
		t.Fatalf("defaults = %#v", defaults)
	}
}

func TestResolveBuildRejectsEnvironmentArtifactOverrides(t *testing.T) {
	f, err := Load([]byte(base))
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.ResolveBuild()
	if err == nil || !strings.Contains(err.Error(), "shared.build") || !strings.Contains(err.Error(), "staging.build") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveBuildDefaultsNullComposerVersion(t *testing.T) {
	file, err := Load([]byte(`schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.9, mode: integrated}
build: {php: "8.5", composer: {version: null}}
target: {provider: aws, runtime: ecs-fargate}
defaults: {region: eu-west-3, preset: preview}
environments: {staging: {account: "123456789012"}}
`))
	if err != nil {
		t.Fatal(err)
	}
	spec, err := file.ResolveBuild()
	if err != nil || spec.Build.Composer.Version != "2.10+" {
		t.Fatalf("version %q, error %v", spec.Build.Composer.Version, err)
	}
}
