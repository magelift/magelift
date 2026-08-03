package ops_test

import (
	"context"
	"errors"
	"testing"

	"github.com/magelift/magelift/internal/cloud/gcp/ops"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type costPlanned struct{}

func (costPlanned) StackName() string                             { return "test" }
func (costPlanned) Provider() sdk.ProviderID                      { return "gcp" }
func (costPlanned) Runtime() sdk.RuntimeID                        { return "gke-autopilot" }
func (costPlanned) Project() string                               { return "shop" }
func (costPlanned) Environment() string                           { return "staging" }
func (costPlanned) Region() string                                { return "europe-west1" }
func (costPlanned) CertificationTier() platform.CertificationTier { return platform.TierExperimental }
func (costPlanned) EnvironmentClass() string                      { return "staging" }
func (costPlanned) Protected() bool                               { return false }
func (costPlanned) ImageDigest() string                           { return "" }
func (costPlanned) WithImageDigest(string) (platform.PlannedStack, error) {
	return costPlanned{}, nil
}
func (costPlanned) TargetDescriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "gcp-gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot"}
}

func TestModuleCostEstimatorAccountFree(t *testing.T) {
	cfg := config.Config{
		Target: config.Target{Provider: "gcp", Runtime: "gke-autopilot", GCP: &config.GCPTarget{
			Project: "example-gcp-project", DesiredWebReplicas: 2,
		}},
		Defaults: config.Defaults{Region: "europe-west1"},
		Preset:   "standard",
	}
	report, err := ops.Module{}.CostEstimator().Estimate(context.Background(), costPlanned{}, cfg, platform.CostOptions{})
	if errors.Is(err, platform.ErrNotSupported) {
		t.Fatal("Module.CostEstimator returned ErrNotSupported")
	}
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "account-free" || len(report.Estimated) == 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.Notice == "" {
		t.Fatal("Notice must be non-empty")
	}
}
