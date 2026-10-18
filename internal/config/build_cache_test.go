package config

import (
	"strings"
	"testing"
)

func TestValidateRegistryCacheReferenceAcceptsCommonRegistries(t *testing.T) {
	for _, reference := range []string{
		"registry.example.invalid/acme/shop:buildkit-cache",
		"localhost:5000/acme/shop:buildkit-cache",
		"123456789012.dkr.ecr.us-east-1.amazonaws.com/acme/shop:buildkit-cache",
		"us-docker.pkg.dev/acme-project/magelift/shop:buildkit-cache",
		"ghcr.io/acme/shop:buildkit-cache",
		"ghcr.io/acme/secret-shop:cache",
	} {
		if err := ValidateRegistryCacheReference("build.cache.to", reference); err != nil {
			t.Fatalf("%q: %v", reference, err)
		}
	}
}

func TestValidateRegistryCacheReferenceRejectsUnsafeInput(t *testing.T) {
	tests := []struct {
		reference string
		message   string
	}{
		{reference: "registry.example.invalid/acme/shop", message: "explicit tag"},
		{reference: "registry.example.invalid/acme/shop@sha256:deadbeef", message: "digest"},
		{reference: "https://registry.example.invalid/acme/shop:tag", message: "URL"},
		{reference: "registry.example.invalid/acme/shop:tag?pull=always", message: "URL"},
		{reference: "user:pass@registry.example.invalid/acme/shop:tag", message: "explicit tag"},
		{reference: "registry.example.invalid/acme/shop:tag,mode=max", message: "BuildKit cache options"},
		{reference: "type=registry,ref=registry.example.invalid/acme/shop:tag", message: "BuildKit cache options"},
		{reference: "registry.example.invalid/acme/shop:tag ", message: "whitespace"},
		{reference: "registry.example.invalid/acme/shop:tag,other", message: "explicit tag"},
	}
	for _, test := range tests {
		t.Run(test.reference, func(t *testing.T) {
			err := ValidateRegistryCacheReference("build.cache.to", test.reference)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error=%v want %q", err, test.message)
			}
		})
	}
}

func TestResolveRejectsInvalidBuildCache(t *testing.T) {
	tests := []struct {
		name    string
		overlay string
		message string
	}{
		{
			name:    "mode without export",
			overlay: "  cache: {mode: max}\n",
			message: "build.cache.mode requires build.cache.to",
		},
		{
			name:    "ignore without export",
			overlay: "  cache: {ignoreExportErrors: true}\n",
			message: "build.cache.ignoreExportErrors requires build.cache.to",
		},
		{
			name:    "invalid export reference",
			overlay: "  cache: {to: acme/shop:cache}\n",
			message: "registry host",
		},
		{
			name:    "duplicate import",
			overlay: "  cache: {from: [registry.example.invalid/acme/cache:import, registry.example.invalid/acme/cache:import]}\n",
			message: "duplicate",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := strings.Replace(base, "build:\n  php:", "build:\n"+test.overlay+"  php:", 1)
			file, err := Load([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			_, err = file.Resolve("staging", ResolveOptions{})
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error=%v want %q", err, test.message)
			}
		})
	}
}

func TestResolveAcceptsValidBuildCache(t *testing.T) {
	input := strings.Replace(base, "build:\n  php:", `build:
  cache:
    from:
      - registry.example.invalid/acme/cache:import
    to: registry.example.invalid/acme/cache:export
    mode: min
  php:`, 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Config.Build.Cache == nil || effective.Config.Build.Cache.To == "" || len(effective.Config.Build.Cache.From) != 1 {
		t.Fatalf("cache = %#v", effective.Config.Build.Cache)
	}
}
