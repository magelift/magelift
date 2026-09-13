package main

import (
	"context"
	"testing"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
)

func TestAcceptancePlanUsesOneOwnedNativeCloudWatchCell(t *testing.T) {
	plan := acceptancePlan("magelift/aws/observability/live-test", 1)
	if plan.TargetProvider != "aws" || plan.TargetRuntime != "cloudwatch-live-cell" {
		t.Fatalf("plan target = %#v", plan)
	}
	if len(plan.Bindings) != 2 || len(plan.Alerts) != 1 || len(plan.Dashboards) != 1 {
		t.Fatalf("plan shape = %#v", plan)
	}
	if plan.Alerts[0].Owner == "" || plan.Alerts[0].RunbookURL == "" || plan.Alerts[0].DeduplicationKey == "" || plan.Dashboards[0].Owner == "" {
		t.Fatalf("acceptance plan dropped actionable ownership metadata: %#v", plan)
	}
	for _, binding := range plan.Bindings {
		if binding.Destination != "cloudwatch" || binding.Mode != "native" || binding.OwnershipMarker != plan.OwnershipMarker {
			t.Fatalf("binding = %#v", binding)
		}
	}
}

func TestVerifySignalsRequiresAllPortableProofBits(t *testing.T) {
	client := fakeLifecycleClient{observations: map[string]providerobservability.SignalObservation{
		"logs":    {Signal: "logs", Destination: "cloudwatch", Delivered: true, LabelsVerified: true, RetentionVerified: true, RedactionVerified: true},
		"metrics": {Signal: "metrics", Destination: "cloudwatch", Delivered: true, LabelsVerified: true, RetentionVerified: true, RedactionVerified: false},
	}}
	plan := acceptancePlan("marker", 1)
	all, failure := verifySignals(context.Background(), &client, plan.Bindings)
	if all || failure == "" {
		t.Fatalf("verifySignals = all=%t failure=%q", all, failure)
	}
}

type fakeLifecycleClient struct {
	observations map[string]providerobservability.SignalObservation
}

func (f *fakeLifecycleClient) Apply(_ context.Context, _ providerobservability.Plan) (providerobservability.LifecycleResult, error) {
	return providerobservability.LifecycleResult{}, nil
}

func (f *fakeLifecycleClient) VerifySignal(_ context.Context, binding providerobservability.SignalBinding) (providerobservability.SignalObservation, error) {
	return f.observations[binding.Signal], nil
}

func (*fakeLifecycleClient) VerifyOperations(context.Context, providerobservability.Plan) (providerobservability.OperationalObservation, error) {
	return providerobservability.OperationalObservation{AlertsVerified: true, DashboardsVerified: true, SLOsVerified: true}, nil
}

func (*fakeLifecycleClient) VerifyCleanup(context.Context, providerobservability.Plan) (providerobservability.CleanupObservation, error) {
	return providerobservability.CleanupObservation{}, nil
}

func (*fakeLifecycleClient) Destroy(context.Context, providerobservability.Plan, []string) (providerobservability.CleanupObservation, error) {
	return providerobservability.CleanupObservation{}, nil
}
