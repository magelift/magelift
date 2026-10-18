package cost_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	gcpcost "github.com/magelift/magelift/providers/gcp/cost"
	providerschema "github.com/magelift/magelift/providers/gcp/schema"
	"github.com/magelift/magelift/sdk"
)

func TestAccountFreeCostReportClassifiesInputs(t *testing.T) {
	in := gcpcost.EstimateInputs{
		Envelope: sdk.Envelope{Environment: "staging", Region: "europe-west1", Preset: "standard", MonthlyBudgetCents: 50000},
		Target: &providerschema.GCPTarget{
			Project:                "example-gcp-project",
			Region:                 "europe-west1",
			CloudSQLTier:           "db-custom-2-7680",
			MemorystoreNodeType:    "STANDARD_SMALL",
			AutopilotCPURequest:    "500m",
			AutopilotMemoryRequest: "1Gi",
			DesiredWebReplicas:     2,
			QueueConsumerCount:     1,
		},
	}
	report, err := gcpcost.Estimator{}.Estimate(context.Background(), in)
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
	in := gcpcost.EstimateInputs{Envelope: sdk.Envelope{Environment: "preview", Region: "europe-west1", Preset: "preview"}}
	report, err := gcpcost.Estimator{}.Estimate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Estimated) != 1 || len(report.Unsupported) != 3 {
		t.Fatalf("missing target.gcp was not classified: %#v", report)
	}
}

func TestEstimateRequiresEnvelope(t *testing.T) {
	_, err := gcpcost.Estimator{}.Estimate(context.Background(), gcpcost.EstimateInputs{})
	if err == nil {
		t.Fatal("empty envelope was accepted")
	}
}

func TestEstimateRejectsUnsupportedRuntime(t *testing.T) {
	in := gcpcost.EstimateInputs{Envelope: sdk.Envelope{Environment: "staging"}, Runtime: "ecs-fargate", Target: &providerschema.GCPTarget{Project: "p"}}
	_, err := gcpcost.Estimator{}.Estimate(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "runtime") {
		t.Fatalf("unsupported runtime error = %v", err)
	}
}

func TestLiveCatalogNotWired(t *testing.T) {
	in := gcpcost.EstimateInputs{Envelope: sdk.Envelope{Environment: "staging"}, Live: true, Target: &providerschema.GCPTarget{Project: "p"}}
	_, err := gcpcost.Estimator{}.Estimate(context.Background(), in)
	if err == nil || !strings.Contains(err.Error(), "--live") {
		t.Fatalf("live error = %v", err)
	}
	if errors.Is(err, platform.ErrNotSupported) {
		t.Fatal("live path must not return ErrNotSupported")
	}
}
