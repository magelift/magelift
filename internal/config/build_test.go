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
