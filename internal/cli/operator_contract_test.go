package cli

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/health"
	"github.com/magelift/magelift/internal/platform"
	"go.yaml.in/yaml/v4"
)

func TestHealthReportOutputSnapshots(t *testing.T) {
	observedAt := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	report := health.Report{
		Environment: "staging",
		Target:      "gcp.gke-autopilot",
		Mode:        "runtime",
		Status:      health.StatusDegraded,
		Checks: []health.Check{{
			ID:         "runtime.web",
			Layer:      health.LayerMagento,
			Status:     health.StatusDegraded,
			Message:    "one replica is unavailable",
			Source:     "kubernetes",
			ObservedAt: observedAt,
			Freshness:  health.FreshnessCurrent,
		}},
		ObservedAt: observedAt,
		Source:     "runtime",
		Freshness:  health.FreshnessCurrent,
	}

	jsonValue, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := `{
  "environment": "staging",
  "target": "gcp.gke-autopilot",
  "mode": "runtime",
  "status": "degraded",
  "checks": [
    {
      "id": "runtime.web",
      "layer": "magento",
      "status": "degraded",
      "message": "one replica is unavailable",
      "source": "kubernetes",
      "observedAt": "2026-08-14T12:00:00Z",
      "freshness": "current"
    }
  ],
  "observedAt": "2026-08-14T12:00:00Z",
  "source": "runtime",
  "freshness": "current"
}
`
	if got := string(jsonValue) + "\n"; got != wantJSON {
		t.Fatalf("JSON snapshot drifted:\n%s", got)
	}

	yamlValue, err := yaml.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	const wantYAML = `environment: staging
target: gcp.gke-autopilot
mode: runtime
status: degraded
checks:
    - id: runtime.web
      layer: magento
      status: degraded
      message: one replica is unavailable
      source: kubernetes
      observedAt: 2026-08-14T12:00:00Z
      freshness: current
observedAt: 2026-08-14T12:00:00Z
source: runtime
freshness: current
`
	if string(yamlValue) != wantYAML {
		t.Fatalf("YAML snapshot drifted:\n%s", yamlValue)
	}

	var table bytes.Buffer
	if err := (&options{output: "table", stdout: &table}).write(report); err != nil {
		t.Fatal(err)
	}
	if table.String() != wantYAML {
		t.Fatalf("table snapshot drifted:\n%s", table.String())
	}
}

func TestCostReportNormalizationSnapshotFields(t *testing.T) {
	observedAt := time.Date(2026, time.August, 14, 12, 0, 0, 0, time.UTC)
	report := platform.NormalizeCostReport(platform.CostReport{
		Environment: "staging",
		Provider:    "gcp",
		Region:      "europe-west1",
		Mode:        "account-free",
		Currency:    "USD",
		Estimated: []platform.CostEstimatedItem{{
			Resource: "GKE Autopilot",
		}},
	}, observedAt)
	if report.Evidence != platform.CostEvidenceEstimate || report.Source != "configuration" || report.Freshness != platform.CostFreshnessCurrent {
		t.Fatalf("report evidence = %#v", report)
	}
	if report.Scope != "gcp/europe-west1/staging" || report.Estimated[0].Evidence != platform.CostEvidenceEstimate || report.Estimated[0].Source != "configuration" {
		t.Fatalf("report scope/items = %#v", report)
	}
}

func TestCostReportNormalizesActualForecastAndBudgetEvidence(t *testing.T) {
	observedAt := time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC)
	report := platform.NormalizeCostReport(platform.CostReport{
		Provider: "gcp", Region: "europe-west1", Environment: "staging",
		Actual:   &platform.CostMoney{Units: 4, Currency: "USD"},
		Forecast: &platform.CostMoney{Units: 7, Currency: "USD"},
		Budget:   &platform.CostBudgetReport{Budgets: []platform.CostBudget{{Amount: platform.CostMoney{Units: 10}}}},
	}, observedAt)
	if report.Actual == nil || report.Actual.ObservedAt != observedAt || report.Actual.Source != "configuration" || report.Actual.Freshness != platform.CostFreshnessCurrent {
		t.Fatalf("actual = %#v", report.Actual)
	}
	if report.Forecast == nil || report.Forecast.ObservedAt != observedAt {
		t.Fatalf("forecast = %#v", report.Forecast)
	}
	if report.Budget == nil || report.Budget.ObservedAt != observedAt || report.Budget.Source != "configuration" || report.Budget.Budgets[0].Amount.ObservedAt != observedAt {
		t.Fatalf("budget = %#v", report.Budget)
	}
}
