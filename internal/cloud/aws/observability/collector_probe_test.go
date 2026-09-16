package observability

import (
	"context"
	"testing"

	"github.com/magelift/magelift/internal/external/newrelic"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
)

func TestECSNewRelicSignalProbeQueriesEveryRequestedSignal(t *testing.T) {
	query := &fakeECSMarkerQueryer{result: newrelic.MarkerQueryResult{Count: 1, LabelsVerified: true}}
	probe, err := NewECSNewRelicSignalProbe(query, "newrelic-acceptance://user-key")
	if err != nil {
		t.Fatal(err)
	}
	plan := ecsCollectorPlanRequest()
	resource := providerobservability.CollectorResource{Identity: "service:cluster/web|task:collector|previous:original", OwnershipMarker: plan.OwnershipMarker, Owned: true}
	delivered, err := probe.VerifyCollectorSignals(context.Background(), sdk.CollectorDeploymentPlan{
		TargetProvider: plan.TargetProvider, TargetRuntime: plan.TargetRuntime, Workload: plan.Workload, Distribution: plan.Distribution,
		CredentialRef: plan.CredentialRef, Endpoint: plan.Endpoint, NativeReference: plan.NativeReference, OwnershipMarker: plan.OwnershipMarker, Signals: plan.Signals,
	}, resource)
	if err != nil || !delivered {
		t.Fatalf("delivery = %t, err = %v", delivered, err)
	}
	if len(query.requests) != len(plan.Signals) {
		t.Fatalf("query requests = %#v", query.requests)
	}
	for _, request := range query.requests {
		if request.CredentialRef != "newrelic-acceptance://user-key" || request.OwnershipMarker != plan.OwnershipMarker {
			t.Fatalf("query request crossed credential or marker boundary: %#v", request)
		}
	}
}

func TestECSNewRelicSignalProbeReturnsUnprovenForEmptyResult(t *testing.T) {
	query := &fakeECSMarkerQueryer{result: newrelic.MarkerQueryResult{LabelsVerified: false}}
	probe, err := NewECSNewRelicSignalProbe(query, "newrelic-acceptance://user-key")
	if err != nil {
		t.Fatal(err)
	}
	plan := ecsCollectorPlanRequest()
	resource := providerobservability.CollectorResource{Identity: "service:cluster/web|task:collector|previous:original", OwnershipMarker: plan.OwnershipMarker, Owned: true}
	delivered, err := probe.VerifyCollectorSignals(context.Background(), sdk.CollectorDeploymentPlan{
		TargetProvider: plan.TargetProvider, TargetRuntime: plan.TargetRuntime, Workload: plan.Workload, Distribution: plan.Distribution,
		CredentialRef: plan.CredentialRef, Endpoint: plan.Endpoint, NativeReference: plan.NativeReference, OwnershipMarker: plan.OwnershipMarker, Signals: plan.Signals,
	}, resource)
	if err != nil {
		t.Fatal(err)
	}
	if delivered {
		t.Fatal("empty marker result was reported as delivered")
	}
}

func TestNewECSNewRelicSignalProbeValidatesDependencies(t *testing.T) {
	if _, err := NewECSNewRelicSignalProbe(nil, "newrelic-acceptance://user-key"); err == nil {
		t.Fatal("nil query verifier was accepted")
	}
	query := &fakeECSMarkerQueryer{}
	if _, err := NewECSNewRelicSignalProbe(query, "not a credential"); err == nil {
		t.Fatal("invalid query credential reference was accepted")
	}
}

type fakeECSMarkerQueryer struct {
	result   newrelic.MarkerQueryResult
	requests []newrelic.MarkerQueryRequest
}

func (queryer *fakeECSMarkerQueryer) QueryMarker(_ context.Context, request newrelic.MarkerQueryRequest) (newrelic.MarkerQueryResult, error) {
	queryer.requests = append(queryer.requests, request)
	return queryer.result, nil
}
