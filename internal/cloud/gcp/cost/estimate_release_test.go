package cost_test

import (
	"context"
	"strings"
	"testing"

	gcpcost "github.com/magelift/magelift/internal/cloud/gcp/cost"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
)

func TestAccountFreeUsesReleaseAwareCloudSQLDefault(t *testing.T) {
	cfg := config.Config{
		Application: config.Application{Version: "2.4.9"},
		Target:      config.Target{Provider: "gcp", Runtime: "gke-autopilot", GCP: &config.GCPTarget{Project: "p"}},
		Defaults:    config.Defaults{Region: "europe-west1"},
		Preset:      "standard",
	}
	report, err := gcpcost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "staging", region: "europe-west1"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Estimated {
		if item.Resource == "Cloud SQL MySQL" && !strings.Contains(item.Configuration, "db-perf-optimized-N-2") {
			t.Fatalf("Cloud SQL estimate = %q", item.Configuration)
		}
	}
}
