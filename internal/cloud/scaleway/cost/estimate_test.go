package cost_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	scalewaycost "github.com/magelift/magelift/internal/cloud/scaleway/cost"
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
func (f fakePlanned) Provider() sdk.ProviderID { return "scaleway" }
func (f fakePlanned) Runtime() sdk.RuntimeID {
	if f.runtime == "" {
		return "kapsule"
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
	return sdk.TargetDescriptor{ID: "scaleway-kapsule", Provider: "scaleway", Runtime: "kapsule"}
}

func TestAccountFreeCostReportClassifiesScalewayCapacity(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Target: config.Target{Provider: "scaleway", Runtime: "kapsule", Scaleway: &config.ScalewayTarget{
			Region: "fr-par", Zones: []string{"fr-par-1", "fr-par-2"},
			KapsuleVersion: "1.36.1", NodeType: "GP1-M", NodeCount: 3,
			CPURequest: "1", MemoryRequest: "2Gi", DesiredWebReplicas: 4,
			DatabaseNodeType: "DB-GP-M", DatabaseHighAvailability: true,
			RedisNodeType: "RED1-S", RedisVersion: "8.6.3", RedisClusterSize: 2,
			QueueConsumerCount: 2,
		}},
		Defaults: config.Defaults{Region: "fr-par", Preset: "standard"},
		Preset:   "standard", MonthlyBudgetCents: 50000,
	}
	report, err := scalewaycost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "staging", region: "fr-par"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Environment != "staging" || report.Provider != "scaleway" || report.Region != "fr-par" || report.Preset != "standard" {
		t.Fatalf("unexpected report identity: %#v", report)
	}
	if report.Mode != "account-free" || report.Currency != "EUR" || report.MonthlyTotalCents != nil || len(report.Priced) != 0 {
		t.Fatalf("account-free report claimed a price: %#v", report)
	}
	if len(report.Estimated) != 7 || len(report.Unsupported) != 3 {
		t.Fatalf("unexpected classifications: %#v", report)
	}
	for _, test := range []struct {
		resource string
		contains string
	}{
		{resource: "Scaleway Kapsule control plane", contains: "1.36.1"},
		{resource: "Scaleway Kapsule worker pool", contains: "3 x GP1-M nodes across fr-par-1, fr-par-2"},
		{resource: "Kapsule web workload", contains: "4 replicas, 1 CPU, 2Gi memory each"},
		{resource: "Scaleway Managed Database for MySQL", contains: "DB-GP-M, high availability"},
		{resource: "Scaleway Managed Redis", contains: "RED1-S, 2 nodes, Redis 8.6.3"},
		{resource: "Scaleway load balancer", contains: "LoadBalancer"},
		{resource: "Magento queue", contains: "database-backed, 2 consumer replicas"},
	} {
		t.Run(test.resource, func(t *testing.T) {
			t.Parallel()
			assertEstimatedConfiguration(t, report, test.resource, test.contains)
		})
	}
	if report.Notice == "" || !strings.Contains(report.Notice, "Scaleway") {
		t.Fatalf("account-free notice is not explicit: %q", report.Notice)
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	if err := json.Unmarshal(data, &shape); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"environment", "provider", "region", "preset", "mode", "currency", "priced", "estimated", "unsupported", "monthlyTotalCents", "notice"} {
		if _, found := shape[field]; !found {
			t.Fatalf("provider-neutral field %q is absent from %s", field, data)
		}
	}
}

func TestAccountFreeAppliesScalewayPresetDefaults(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Target:   config.Target{Provider: "scaleway", Runtime: "kapsule", Scaleway: &config.ScalewayTarget{Region: "fr-par"}},
		Defaults: config.Defaults{Region: "fr-par", Preset: "high-availability"},
		Preset:   "high-availability",
	}
	report, err := scalewaycost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "production", region: "fr-par"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertEstimatedConfiguration(t, report, "Scaleway Kapsule worker pool", "2 x DEV1-M nodes across fr-par-1")
	assertEstimatedConfiguration(t, report, "Kapsule web workload", "3 replicas, 500m CPU, 2Gi memory each")
	assertEstimatedConfiguration(t, report, "Scaleway Managed Database for MySQL", "DB-GP-S, single instance")
	assertEstimatedConfiguration(t, report, "Scaleway Managed Redis", "RED1-S, 2 nodes, Redis 8.6.3")
}

func TestAccountFreeDoesNotRequireCloudContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := config.Config{Target: config.Target{Provider: "scaleway", Runtime: "kapsule", Scaleway: &config.ScalewayTarget{Region: "fr-par"}}}
	if _, err := (scalewaycost.Estimator{}).Estimate(ctx, fakePlanned{env: "preview", region: "fr-par"}, cfg, platform.CostOptions{}); err != nil {
		t.Fatalf("account-free estimation used the canceled context: %v", err)
	}
}

func TestCostReportWithoutScalewayTargetIsExplicit(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Target: config.Target{Provider: "scaleway", Runtime: "kapsule"}, Defaults: config.Defaults{Region: "fr-par", Preset: "preview"}}
	report, err := scalewaycost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "preview", region: "fr-par"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Estimated) != 1 || len(report.Unsupported) != 4 {
		t.Fatalf("missing target.scaleway was not classified: %#v", report)
	}
	if !strings.Contains(report.Estimated[0].Configuration, "preview") {
		t.Fatalf("missing target report lost preset: %#v", report.Estimated[0])
	}
}

func TestBudgetFlagReportsUnsupportedProviderBudget(t *testing.T) {
	cfg := config.Config{Target: config.Target{Provider: "scaleway", Runtime: "kapsule", Scaleway: &config.ScalewayTarget{ProjectID: "project-123"}}}
	report, err := scalewaycost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "staging", region: "fr-par"}, cfg, platform.CostOptions{Budget: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Budget == nil || report.Budget.State != platform.CostBudgetUnavailable || report.Budget.Scope != "scaleway/project/project-123" {
		t.Fatalf("budget = %#v", report.Budget)
	}
	if !strings.Contains(report.Budget.Notice, "monthlyBudgetCents") {
		t.Fatalf("budget notice = %q", report.Budget.Notice)
	}
}

func TestEstimateRequiresPlannedStack(t *testing.T) {
	t.Parallel()

	_, err := scalewaycost.Estimator{}.Estimate(context.Background(), nil, config.Config{}, platform.CostOptions{})
	if err == nil {
		t.Fatal("nil planned stack was accepted")
	}
}

func TestEstimateRejectsUnsupportedRuntime(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Target: config.Target{Provider: "scaleway", Runtime: "kapsule", Scaleway: &config.ScalewayTarget{Region: "fr-par"}}}
	_, err := scalewaycost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "staging", region: "fr-par", runtime: "gke-autopilot"}, cfg, platform.CostOptions{})
	if err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("unsupported runtime error = %v", err)
	}
}

func TestLivePricingNotImplemented(t *testing.T) {
	t.Parallel()

	cfg := config.Config{Target: config.Target{Provider: "scaleway", Runtime: "kapsule", Scaleway: &config.ScalewayTarget{Region: "fr-par"}}}
	_, err := scalewaycost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "staging", region: "fr-par"}, cfg, platform.CostOptions{Live: true})
	if err == nil || !strings.Contains(err.Error(), "--live") {
		t.Fatalf("live error = %v", err)
	}
	if errors.Is(err, platform.ErrNotSupported) {
		t.Fatal("live pricing refusal must remain distinct from an unavailable account-free estimator")
	}
}

func assertEstimatedConfiguration(t *testing.T, report platform.CostReport, resource, contains string) {
	t.Helper()
	for _, item := range report.Estimated {
		if item.Resource == resource {
			if !strings.Contains(item.Configuration, contains) {
				t.Fatalf("%s configuration = %q, want substring %q", resource, item.Configuration, contains)
			}
			return
		}
	}
	t.Fatalf("missing estimated resource %q: %#v", resource, report.Estimated)
}
