package stack

import (
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/config"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestPlanFromConfigMapsExplicitGCPInputs(t *testing.T) {
	cfg := gcpDeploymentConfig()
	spec, err := PlanFromConfig(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.Validate(); err != nil {
		t.Fatalf("planned spec is invalid: %v", err)
	}
	if spec.Identity.Preset != sdk.PresetStandard || spec.Identity.GCPProject != cfg.Target.GCP.Project {
		t.Fatalf("identity was not mapped: %#v", spec.Identity)
	}
	if spec.Identity.Region != "europe-west1" {
		t.Fatalf("region = %q", spec.Identity.Region)
	}
	if spec.Artifact.ImageDigest != cfg.Target.GCP.ImageDigest {
		t.Fatalf("image digest = %q", spec.Artifact.ImageDigest)
	}
	if spec.Policy.NetworkCIDR != "10.20.0.0/16" {
		t.Fatalf("network CIDR = %q", spec.Policy.NetworkCIDR)
	}
	if len(spec.Policy.Zones) != 2 || spec.Policy.Zones[0] != "europe-west1-b" {
		t.Fatalf("zones = %#v", spec.Policy.Zones)
	}
	if spec.Catalog.CloudSQLTier != "db-custom-2-7680" || spec.Catalog.DesiredWebReplicas != 2 {
		t.Fatalf("catalog was not mapped: %#v", spec.Catalog)
	}
	if spec.Catalog.CloudSQLAvailability != "REGIONAL" || spec.Catalog.QueueMode != "rabbitmq" {
		t.Fatalf("standard production catalog = %#v", spec.Catalog)
	}
	if spec.Dependencies.DatabaseName != "magento" || spec.Dependencies.MasterUsername != "magento" {
		t.Fatalf("dependencies were not mapped: %#v", spec.Dependencies)
	}
}

func TestPlanFromConfigRejectsMissingGCPBlock(t *testing.T) {
	cfg := gcpDeploymentConfig()
	cfg.Target.GCP = nil
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "target.gcp is required") {
		t.Fatalf("unexpected missing GCP input error: %v", err)
	}
}

func TestPlanFromConfigRejectsGuessedDeploymentInputs(t *testing.T) {
	cfg := gcpDeploymentConfig()
	cfg.Class = ""
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "class must be explicit") {
		t.Fatalf("missing class was accepted: %v", err)
	}

	cfg = gcpDeploymentConfig()
	cfg.ExpiresAt = "not-a-timestamp"
	if _, err := PlanFromConfig(cfg, "staging"); err == nil || !strings.Contains(err.Error(), "RFC3339") {
		t.Fatalf("invalid expiration was accepted: %v", err)
	}
}

func TestPlanFromConfigAllowsExpiredPreviewOnlyForDestroy(t *testing.T) {
	cfg := gcpDeploymentConfig()
	cfg.Class = "preview"
	cfg.Preset = "preview"
	cfg.Defaults.Preset = "preview"
	cfg.ExpiresAt = "2020-01-01T00:00:00Z"
	if _, err := PlanFromConfig(cfg, "preview"); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired preview was accepted for normal planning: %v", err)
	}
	if _, err := PlanFromConfigWithOptions(cfg, "preview", PlanOptions{AllowExpiredPreview: true}); err != nil {
		t.Fatalf("expired preview was not accepted for destroy planning: %v", err)
	}
}

func TestPlanFromConfigUsesResolvedDefaultPresetWhenEnvironmentDoesNotOverride(t *testing.T) {
	cfg := gcpDeploymentConfig()
	cfg.Preset = ""
	spec, err := PlanFromConfig(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Identity.Preset != sdk.PresetStandard {
		t.Fatalf("preset = %q", spec.Identity.Preset)
	}
}

func gcpDeploymentConfig() config.Config {
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
				Project:     "example-gcp-project",
				Region:      "europe-west1",
				NetworkCIDR: "10.20.0.0/16",
				ImageDigest: "ghcr.io/magelift/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		},
		Defaults:  config.Defaults{Region: "europe-west1", Preset: "standard"},
		Class:     "staging",
		Preset:    "standard",
		ExpiresAt: time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
	}
}
