package ops_test

import (
	"context"
	"errors"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	gcpcost "github.com/magelift/magelift/providers/gcp/cost"
	providerschema "github.com/magelift/magelift/providers/gcp/schema"
	"github.com/magelift/magelift/sdk"
)

func TestEstimatorAccountFree(t *testing.T) {
	in := gcpcost.EstimateInputs{
		Envelope: sdk.Envelope{Environment: "staging", Region: "europe-west1", Preset: "standard"},
		Target:   &providerschema.GCPTarget{Project: "example-gcp-project", DesiredWebReplicas: 2},
	}
	report, err := gcpcost.Estimator{}.Estimate(context.Background(), in)
	if errors.Is(err, platform.ErrNotSupported) {
		t.Fatal("Estimator returned ErrNotSupported")
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
