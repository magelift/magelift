package cost_test

import (
	"context"
	"strings"
	"testing"

	gcpcost "github.com/magelift/magelift/providers/gcp/cost"
	providerschema "github.com/magelift/magelift/providers/gcp/schema"
	"github.com/magelift/magelift/sdk"
)

func TestAccountFreeUsesReleaseAwareCloudSQLDefault(t *testing.T) {
	in := gcpcost.EstimateInputs{
		Envelope: sdk.Envelope{Environment: "staging", Region: "europe-west1", Preset: "standard", AppVersion: "2.4.9"},
		Target:   &providerschema.GCPTarget{Project: "p"},
	}
	report, err := gcpcost.Estimator{}.Estimate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range report.Estimated {
		if item.Resource == "Cloud SQL MySQL" && !strings.Contains(item.Configuration, "db-perf-optimized-N-2") {
			t.Fatalf("Cloud SQL estimate = %q", item.Configuration)
		}
	}
}
