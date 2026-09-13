package cost_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	ovhcost "github.com/magelift/magelift/internal/cloud/ovh/cost"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type fakePlanned struct {
	env     string
	region  string
	runtime sdk.RuntimeID
}

func (f fakePlanned) StackName() string        { return "test" }
func (f fakePlanned) Provider() sdk.ProviderID { return "ovh" }
func (f fakePlanned) Runtime() sdk.RuntimeID {
	if f.runtime == "" {
		return "mks"
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
	return sdk.TargetDescriptor{ID: "ovh.mks", Provider: "ovh", Runtime: "mks"}
}

func TestAccountFreeCostReportClassifiesOVHCapacity(t *testing.T) {
	cfg := config.Config{
		Target: config.Target{Provider: "ovh", Runtime: "mks", OVH: &config.OVHTarget{
			Region:             "EU-WEST-PAR",
			MKSPlan:            "standard",
			Zones:              []string{"eu-west-par-a", "eu-west-par-b"},
			NodeFlavor:         "b3-8",
			NodeCount:          3,
			CPURequest:         "500m",
			MemoryRequest:      "1Gi",
			DesiredWebReplicas: 2,
			DatabaseFlavor:     "b3-8",
			DatabasePlan:       "production",
			DatabaseVersion:    "8.4",
			DatabaseNodeCount:  2,
			ValkeyFlavor:       "b3-8",
			ValkeyPlan:         "production",
			ValkeyVersion:      "8.1",
			ValkeyNodeCount:    2,
			QueueConsumerCount: 1,
			AttachFloatingIPs:  true,
		}},
		Defaults:           config.Defaults{Region: "EU-WEST-PAR"},
		Preset:             "standard",
		MonthlyBudgetCents: 42000,
	}

	report, err := (ovhcost.Estimator{}).Estimate(context.Background(), fakePlanned{env: "staging", region: "EU-WEST-PAR"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Environment != "staging" || report.Provider != "ovh" || report.Region != "EU-WEST-PAR" || report.Preset != "standard" {
		t.Fatalf("unexpected report identity: %#v", report)
	}
	if report.Mode != "account-free" || report.Currency != "EUR" || report.MonthlyBudgetCents != 42000 || report.MonthlyTotalCents != nil || len(report.Priced) != 0 {
		t.Fatalf("account-free report claimed a price: %#v", report)
	}
	if len(report.Estimated) != 6 || len(report.Unsupported) != 2 {
		t.Fatalf("unexpected classifications: %#v", report)
	}
	for _, resource := range []string{
		"OVH MKS control plane",
		"OVH MKS worker nodes",
		"OVH Managed MySQL",
		"OVH Managed Valkey",
		"OVH Kubernetes LoadBalancer",
		"Magento queue",
	} {
		if !hasEstimatedResource(report, resource) {
			t.Fatalf("missing estimated resource %q: %#v", resource, report.Estimated)
		}
	}
	worker := estimatedResource(report, "OVH MKS worker nodes")
	if !strings.Contains(worker.Configuration, "3 x b3-8") || !strings.Contains(worker.Configuration, "2 availability zone(s)") || !strings.Contains(worker.Configuration, "floating IPs attached") {
		t.Fatalf("worker capacity lost: %#v", worker)
	}
	queue := estimatedResource(report, "Magento queue")
	if queue.Configuration != "database-backed; 1 consumer replicas" {
		t.Fatalf("queue configuration = %q", queue.Configuration)
	}
	if report.Unsupported[0].Resource != "OVHcloud unit prices" || report.Unsupported[1].Resource != "usage-based services" {
		t.Fatalf("unexpected unsupported items: %#v", report.Unsupported)
	}
}

func TestAccountFreeCostReportWithoutOVHTargetIsExplicit(t *testing.T) {
	cfg := config.Config{
		Target:   config.Target{Provider: "ovh", Runtime: "mks"},
		Defaults: config.Defaults{Region: "EU-WEST-PAR", Preset: "preview"},
	}
	report, err := (ovhcost.Estimator{}).Estimate(context.Background(), fakePlanned{env: "preview", region: "EU-WEST-PAR"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Estimated) != 1 || len(report.Unsupported) != 3 {
		t.Fatalf("missing target.ovh was not classified: %#v", report)
	}
	if !strings.Contains(report.Unsupported[2].Reason, "target.ovh") {
		t.Fatalf("missing target reason: %#v", report.Unsupported)
	}
}

func TestLivePricingFailsClosedBeforeAnyProviderWork(t *testing.T) {
	cfg := config.Config{Target: config.Target{Provider: "ovh", Runtime: "mks", OVH: &config.OVHTarget{Region: "EU-WEST-PAR"}}}
	_, err := (ovhcost.Estimator{}).Estimate(context.Background(), fakePlanned{env: "preview", region: "EU-WEST-PAR"}, cfg, platform.CostOptions{Live: true})
	if err == nil || !strings.Contains(err.Error(), "--live") {
		t.Fatalf("live pricing error = %v", err)
	}
	if errors.Is(err, platform.ErrNotSupported) {
		t.Fatal("live pricing refusal should identify the unimplemented OVH pricing path")
	}
}

func TestBudgetFlagReportsUnsupportedProviderBudget(t *testing.T) {
	cfg := config.Config{Target: config.Target{Provider: "ovh", Runtime: "mks", OVH: &config.OVHTarget{ServiceName: "shop"}}}
	report, err := (ovhcost.Estimator{}).Estimate(context.Background(), fakePlanned{env: "staging", region: "EU-WEST-PAR"}, cfg, platform.CostOptions{Budget: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Budget == nil || report.Budget.State != platform.CostBudgetUnavailable || report.Budget.Scope != "ovh/service/shop" {
		t.Fatalf("budget = %#v", report.Budget)
	}
	if !strings.Contains(report.Budget.Notice, "monthlyBudgetCents") {
		t.Fatalf("budget notice = %q", report.Budget.Notice)
	}
}

func TestEstimateRejectsMissingPlanAndUnsupportedRuntime(t *testing.T) {
	if _, err := (ovhcost.Estimator{}).Estimate(context.Background(), nil, config.Config{}, platform.CostOptions{}); err == nil {
		t.Fatal("nil planned stack was accepted")
	}
	cfg := config.Config{Target: config.Target{Provider: "ovh", Runtime: "mks", OVH: &config.OVHTarget{Region: "EU-WEST-PAR"}}}
	_, err := (ovhcost.Estimator{}).Estimate(context.Background(), fakePlanned{runtime: "gke-autopilot"}, cfg, platform.CostOptions{})
	if err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("unsupported runtime error = %v", err)
	}
}

func hasEstimatedResource(report platform.CostReport, resource string) bool {
	for _, item := range report.Estimated {
		if item.Resource == resource {
			return true
		}
	}
	return false
}

func estimatedResource(report platform.CostReport, resource string) platform.CostEstimatedItem {
	for _, item := range report.Estimated {
		if item.Resource == resource {
			return item
		}
	}
	return platform.CostEstimatedItem{}
}
