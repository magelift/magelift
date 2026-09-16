package main

import (
	"context"
	"testing"
	"time"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
)

func TestAcceptancePlanUsesOneOwnedGoogleCloudOperationsCell(t *testing.T) {
	plan := acceptancePlan("magelift/gcp/observability/live-test")
	if plan.TargetProvider != "gcp" || plan.TargetRuntime != "google-cloud-operations-live-cell" {
		t.Fatalf("plan target = %#v", plan)
	}
	if len(plan.Bindings) != 2 || len(plan.Alerts) != 1 || len(plan.Dashboards) != 1 {
		t.Fatalf("plan shape = %#v", plan)
	}
	if plan.Alerts[0].Owner == "" || plan.Alerts[0].RunbookURL == "" || plan.Alerts[0].DeduplicationKey == "" || plan.Dashboards[0].Owner == "" {
		t.Fatalf("acceptance plan dropped actionable ownership metadata: %#v", plan)
	}
	for _, binding := range plan.Bindings {
		if binding.Destination != "google-cloud-operations" || binding.Mode != "native" || binding.OwnershipMarker != plan.OwnershipMarker {
			t.Fatalf("binding = %#v", binding)
		}
	}
	if plan.Bindings[0].RetentionDays != 1 || plan.Bindings[1].RetentionDays != 0 {
		t.Fatalf("retention plan = %#v", plan.Bindings)
	}
}

func TestLifecycleTimeoutAllowsExplicitLongRoutingBudget(t *testing.T) {
	if got, want := lifecycleTimeout(4*time.Minute), 12*time.Minute; got != want {
		t.Fatalf("default lifecycle timeout = %s, want %s", got, want)
	}
	if got, want := lifecycleTimeout(time.Hour), time.Hour+5*time.Minute; got != want {
		t.Fatalf("long-budget lifecycle timeout = %s, want %s", got, want)
	}
}

func TestVerifySignalsRequiresAllPortableProofBits(t *testing.T) {
	client := fakeLifecycleClient{observations: map[string]providerobservability.SignalObservation{
		"logs":    {Signal: "logs", Destination: "google-cloud-operations", Delivered: true, LabelsVerified: true, RetentionVerified: true, RedactionVerified: true},
		"metrics": {Signal: "metrics", Destination: "google-cloud-operations", Delivered: true, LabelsVerified: true, RetentionVerified: true, RedactionVerified: false},
	}}
	all, failure := verifySignals(context.Background(), &client, acceptancePlan("marker").Bindings)
	if all || failure == "" {
		t.Fatalf("verifySignals = all=%t failure=%q", all, failure)
	}
}

func TestVerifyDeliveryRepublishesDataPlaneProbesWithoutNewResources(t *testing.T) {
	client := fakeLifecycleClient{observations: map[string]providerobservability.SignalObservation{
		"logs":    {Signal: "logs", Destination: "google-cloud-operations", Delivered: true, LabelsVerified: true, RetentionVerified: true, RedactionVerified: true},
		"metrics": {Signal: "metrics", Destination: "google-cloud-operations", Delivered: true, LabelsVerified: true, RetentionVerified: true, RedactionVerified: true},
	}}
	if err := verifyDelivery(context.Background(), &client, acceptancePlan("marker"), time.Second); err != nil {
		t.Fatal(err)
	}
	if len(client.published) != 2 {
		t.Fatalf("published probes = %#v, want one per binding", client.published)
	}
}

type fakeLifecycleClient struct {
	observations map[string]providerobservability.SignalObservation
	published    []providerobservability.SignalBinding
}

func (f *fakeLifecycleClient) Apply(context.Context, providerobservability.Plan) (providerobservability.LifecycleResult, error) {
	return providerobservability.LifecycleResult{}, nil
}

func (f *fakeLifecycleClient) VerifySignal(_ context.Context, binding providerobservability.SignalBinding) (providerobservability.SignalObservation, error) {
	return f.observations[binding.Signal], nil
}

func (f *fakeLifecycleClient) PublishSignalProbe(_ context.Context, binding providerobservability.SignalBinding) error {
	f.published = append(f.published, binding)
	return nil
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
