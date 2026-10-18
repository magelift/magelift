package cost_test

import (
	"context"
	"strings"
	"testing"

	gcpcost "github.com/magelift/magelift/providers/gcp/cost"
	providerschema "github.com/magelift/magelift/providers/gcp/schema"
	"github.com/magelift/magelift/sdk"
)

func TestAccountFreePresetTable(t *testing.T) {
	tests := []struct {
		name             string
		preset           string
		gcp              *providerschema.GCPTarget
		wantMinEstimated int
		wantContains     string
		wantNotice       bool
	}{
		{
			name:             "configured standard catalog",
			preset:           "standard",
			gcp:              &providerschema.GCPTarget{Project: "p", DesiredWebReplicas: 2, CloudSQLTier: "db-custom-2-7680", MemorystoreNodeType: "STANDARD_SMALL"},
			wantMinEstimated: 1,
			wantContains:     "GKE Autopilot web",
			wantNotice:       true,
		},
		{
			name:             "empty gcp fields use preview defaults",
			preset:           "preview",
			gcp:              &providerschema.GCPTarget{Project: "p"},
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
			gcp:              &providerschema.GCPTarget{Project: "p"},
			wantMinEstimated: 1,
			wantContains:     "3 replicas",
			wantNotice:       true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := gcpcost.EstimateInputs{
				Envelope: sdk.Envelope{Environment: "staging", Region: "europe-west1", Preset: tc.preset},
				Target:   tc.gcp,
			}
			report, err := gcpcost.Estimator{}.Estimate(context.Background(), in)
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
