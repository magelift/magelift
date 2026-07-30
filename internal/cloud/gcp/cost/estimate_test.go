package cost_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	gcpcost "github.com/acourtiol/magelift/internal/cloud/gcp/cost"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

type fakePlanned struct {
	env     string
	region  string
	runtime sdk.RuntimeID
}

func (f fakePlanned) StackName() string        { return "test" }
func (f fakePlanned) Provider() sdk.ProviderID { return "gcp" }
func (f fakePlanned) Runtime() sdk.RuntimeID {
	if f.runtime == "" {
		return "gke-autopilot"
	}
	return f.runtime
}
func (f fakePlanned) Project() string     { return "shop" }
func (f fakePlanned) Environment() string { return f.env }
func (f fakePlanned) Region() string      { return f.region }
func (f fakePlanned) CertificationTier() platform.CertificationTier {
	return platform.TierExperimental
}
func (f fakePlanned) EnvironmentClass() string { return "staging" }
func (f fakePlanned) Protected() bool          { return false }
func (f fakePlanned) ImageDigest() string      { return "" }
func (f fakePlanned) WithImageDigest(string) (platform.PlannedStack, error) {
	return f, nil
}
func (f fakePlanned) TargetDescriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "gcp-gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot"}
}

func TestAccountFreeCostReportClassifiesInputs(t *testing.T) {
	cfg := config.Config{
		Target: config.Target{Provider: "gcp", Runtime: "gke-autopilot", GCP: &config.GCPTarget{
			Project:                "digital-lab-341608",
			Region:                 "europe-west1",
			CloudSQLTier:           "db-custom-2-7680",
			MemorystoreNodeType:    "STANDARD_SMALL",
			AutopilotCPURequest:    "500m",
			AutopilotMemoryRequest: "1Gi",
			DesiredWebReplicas:     2,
			QueueConsumerCount:     1,
		}},
		Defaults: config.Defaults{Region: "europe-west1"}, Preset: "standard", MonthlyBudgetCents: 50000,
	}
	report, err := gcpcost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "staging", region: "europe-west1"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "account-free" || report.MonthlyTotalCents != nil || len(report.Priced) != 0 {
		t.Fatalf("account-free report claimed a price: %#v", report)
	}
	if report.Notice == "" {
		t.Fatal("account-free notice must be non-empty")
	}
	if !strings.Contains(report.Notice, "Catalog") {
		t.Fatalf("notice must mention Catalog: %q", report.Notice)
	}
	if len(report.Estimated) == 0 {
		t.Fatalf("expected non-empty Estimated: %#v", report)
	}
	if len(report.Unsupported) < 2 {
		t.Fatalf("unexpected unsupported: %#v", report.Unsupported)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	if err := json.Unmarshal(data, &shape); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"priced", "estimated", "unsupported", "monthlyTotalCents", "notice"} {
		if _, found := shape[field]; !found {
			t.Fatalf("required field %q is absent from %s", field, data)
		}
	}
}

func TestCostReportWithoutGCPTargetIsExplicit(t *testing.T) {
	cfg := config.Config{Target: config.Target{Provider: "gcp", Runtime: "gke-autopilot"}, Defaults: config.Defaults{Region: "europe-west1", Preset: "preview"}}
	report, err := gcpcost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "preview", region: "europe-west1"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Estimated) != 1 || len(report.Unsupported) != 3 {
		t.Fatalf("missing target.gcp was not classified: %#v", report)
	}
}

func TestEstimateRequiresPlannedStack(t *testing.T) {
	_, err := gcpcost.Estimator{}.Estimate(context.Background(), nil, config.Config{}, platform.CostOptions{})
	if err == nil {
		t.Fatal("nil planned stack was accepted")
	}
}

func TestEstimateRejectsUnsupportedRuntime(t *testing.T) {
	cfg := config.Config{Target: config.Target{Provider: "gcp", Runtime: "gke-autopilot", GCP: &config.GCPTarget{Project: "p"}}}
	_, err := gcpcost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "staging", region: "europe-west1", runtime: "ecs-fargate"}, cfg, platform.CostOptions{})
	if err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("unsupported runtime error = %v", err)
	}
}

func TestLiveCatalogNotWired(t *testing.T) {
	cfg := config.Config{Target: config.Target{Provider: "gcp", Runtime: "gke-autopilot", GCP: &config.GCPTarget{Project: "p"}}}
	_, err := gcpcost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "staging", region: "europe-west1"}, cfg, platform.CostOptions{Live: true})
	if err == nil || !strings.Contains(err.Error(), "--live") {
		t.Fatalf("live error = %v", err)
	}
	if errors.Is(err, platform.ErrNotSupported) {
		t.Fatal("live path must not return ErrNotSupported")
	}
}
