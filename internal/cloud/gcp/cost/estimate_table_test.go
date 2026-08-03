package cost_test

import (
	"context"
	"strings"
	"testing"

	gcpcost "github.com/acourtiol/magelift/internal/cloud/gcp/cost"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/platform"
)

func TestAccountFreePresetTable(t *testing.T) {
	tests := []struct {
		name             string
		preset           string
		gcp              *config.GCPTarget
		wantMinEstimated int
		wantContains     string
		wantNotice       bool
	}{
		{
			name:             "configured standard catalog",
			preset:           "standard",
			gcp:              &config.GCPTarget{Project: "p", DesiredWebReplicas: 2, CloudSQLTier: "db-custom-2-7680", MemorystoreNodeType: "STANDARD_SMALL"},
			wantMinEstimated: 1,
			wantContains:     "GKE Autopilot web",
			wantNotice:       true,
		},
		{
			name:             "empty gcp fields use preview defaults",
			preset:           "preview",
			gcp:              &config.GCPTarget{Project: "p"},
			wantMinEstimated: 1,
			wantContains:     "database-backed",
			wantNotice:       true,
		},
		{
			name:             "missing gcp target",
			preset:           "standard",
			gcp:              nil,
			wantMinEstimated: 1,
			wantContains:     "exact capacity is not configured",
			wantNotice:       true,
		},
		{
			name:             "high-availability replicas",
			preset:           "high-availability",
			gcp:              &config.GCPTarget{Project: "p"},
			wantMinEstimated: 1,
			wantContains:     "3 replicas",
			wantNotice:       true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{
				Target:   config.Target{Provider: "gcp", Runtime: "gke-autopilot", GCP: tc.gcp},
				Defaults: config.Defaults{Region: "europe-west1"},
				Preset:   tc.preset,
			}
			report, err := gcpcost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "staging", region: "europe-west1"}, cfg, platform.CostOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if report.Mode != "account-free" {
				t.Fatalf("mode = %q", report.Mode)
			}
			if len(report.Estimated) < tc.wantMinEstimated {
				t.Fatalf("Estimated len = %d, want >= %d: %#v", len(report.Estimated), tc.wantMinEstimated, report.Estimated)
			}
			if tc.wantNotice && report.Notice == "" {
				t.Fatal("Notice must be non-empty")
			}
			joined := ""
			for _, item := range report.Estimated {
				joined += item.Resource + " " + item.Configuration + "\n"
			}
			if !strings.Contains(joined, tc.wantContains) {
				t.Fatalf("Estimated missing %q in %q", tc.wantContains, joined)
			}
		})
	}
}
