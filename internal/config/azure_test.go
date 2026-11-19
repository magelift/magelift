package config

import (
	"strings"
	"testing"
)

func TestResolveAzureTarget(t *testing.T) {
	input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.9, mode: integrated}
build: {php: "8.5"}
target:
  provider: azure
  runtime: aks
  azure: {subscriptionId: example-subscription, resourceGroup: example-rg}
defaults: {region: francecentral, preset: preview}
environments: {preview: {class: preview}}
`
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := file.Resolve("preview", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Config.Target.Azure == nil || resolved.Config.Target.Azure.SubscriptionID != "example-subscription" || resolved.Config.Target.Azure.ResourceGroup != "example-rg" {
		t.Fatalf("resolved Azure target = %#v", resolved.Config.Target.Azure)
	}
	if resolved.Config.Target.AWS != nil || resolved.Config.Target.GCP != nil {
		t.Fatalf("unexpected cloud target: %#v", resolved.Config.Target)
	}
}

func TestResolveAzureRejectsMismatchedAndMissingTargets(t *testing.T) {
	for _, test := range []struct {
		name, target, want string
	}{
		{"missing account", "azure: {}", "subscriptionId"},
		{"wrong block", "gcp: {project: example}", "target.gcp is not valid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.9, mode: integrated}
build: {php: "8.5"}
target:
  provider: azure
  runtime: aks
  ` + test.target + `
defaults: {region: francecentral, preset: preview}
environments: {preview: {class: preview}}
`
			file, err := Load([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			_, err = file.Resolve("preview", ResolveOptions{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAzureServiceCapabilities(t *testing.T) {
	c := Config{}
	c.Target.Provider = "azure"
	c.Target.Runtime = "aks"
	c.Services = map[string]ServiceBinding{
		"database": {Family: "mysql", Ownership: "managed", Version: "8.0.21"},
		"cache":    {Family: "valkey", Ownership: "self-hosted", Image: "valkey:9"},
		"search":   {Family: "opensearch", Ownership: "self-hosted", Image: "opensearch:3"},
		"queue":    {Family: "database"},
	}
	if err := c.CheckServiceCapabilities(); err != nil {
		t.Fatalf("valid selections: %v", err)
	}
	c.Services["cache"] = ServiceBinding{Family: "redis", Ownership: "external", Endpoint: "cache.example", Port: 6379}
	if err := c.CheckServiceCapabilities(); err != nil {
		t.Fatalf("external cache: %v", err)
	}
	c.Services["cache"] = ServiceBinding{Family: "redis", Ownership: "external", Endpoint: "cache.example", Port: 6380}
	if err := c.CheckServiceCapabilities(); err == nil || !strings.Contains(err.Error(), "6379") {
		t.Fatalf("unsupported port: %v", err)
	}
}
