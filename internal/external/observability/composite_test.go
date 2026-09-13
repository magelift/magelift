package observability

import (
	"context"
	"reflect"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

type recordingLifecycleClient struct {
	applyPlans       []Plan
	destroyPlans     []Plan
	destroyRefs      [][]string
	verifiedBindings []SignalBinding
	operations       []Plan
}

func (client *recordingLifecycleClient) Apply(_ context.Context, plan Plan) (LifecycleResult, error) {
	client.applyPlans = append(client.applyPlans, plan)
	result := LifecycleResult{
		OperationID:         "destination-apply",
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}
	if len(plan.Bindings) > 0 {
		result.ResourceRefs = []string{plan.Bindings[0].Destination + ":resource"}
		result.ProofRefs = []string{plan.Bindings[0].Destination + ":proof"}
	}
	return result, nil
}

func (client *recordingLifecycleClient) VerifySignal(_ context.Context, binding SignalBinding) (SignalObservation, error) {
	client.verifiedBindings = append(client.verifiedBindings, binding)
	return SignalObservation{Signal: binding.Signal, Destination: binding.Destination, Delivered: true, LabelsVerified: true, RetentionVerified: true, RedactionVerified: true}, nil
}

func (client *recordingLifecycleClient) VerifyOperations(_ context.Context, plan Plan) (OperationalObservation, error) {
	client.operations = append(client.operations, plan)
	return OperationalObservation{AlertsVerified: true, DashboardsVerified: true, SLOsVerified: true}, nil
}

func (client *recordingLifecycleClient) Destroy(_ context.Context, plan Plan, references []string) (CleanupObservation, error) {
	client.destroyPlans = append(client.destroyPlans, plan)
	client.destroyRefs = append(client.destroyRefs, append([]string(nil), references...))
	return CleanupObservation{Complete: true, UnownedPreserved: true}, nil
}

func (client *recordingLifecycleClient) VerifyCleanup(context.Context, Plan) (CleanupObservation, error) {
	return CleanupObservation{Complete: true, UnownedPreserved: true}, nil
}

func TestNewCompositeLifecycleClientValidatesDestinationOwnership(t *testing.T) {
	client := &recordingLifecycleClient{}
	if _, err := NewCompositeLifecycleClient(nil, ""); err == nil {
		t.Fatal("empty composite was accepted")
	}
	if _, err := NewCompositeLifecycleClient(map[string]LifecycleClient{" ": client}, ""); err == nil {
		t.Fatal("empty destination was accepted")
	}
	if _, err := NewCompositeLifecycleClient(map[string]LifecycleClient{"cloudwatch": client}, "newrelic"); err == nil {
		t.Fatal("missing operation owner was accepted")
	}
	if _, err := NewCompositeLifecycleClient(map[string]LifecycleClient{"cloudwatch": nil}, "cloudwatch"); err == nil {
		t.Fatal("nil destination client was accepted")
	}
	var typedNil *recordingLifecycleClient
	if _, err := NewCompositeLifecycleClient(map[string]LifecycleClient{"cloudwatch": typedNil}, "cloudwatch"); err == nil {
		t.Fatal("typed nil destination client was accepted")
	}
}

func TestCompositeLifecycleClientSplitsBindingsAndKeepsOneOperationOwner(t *testing.T) {
	cloudwatch := &recordingLifecycleClient{}
	newRelic := &recordingLifecycleClient{}
	client, err := NewCompositeLifecycleClient(map[string]LifecycleClient{
		"newrelic":   newRelic,
		"cloudwatch": cloudwatch,
	}, "cloudwatch")
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{
		TargetProvider:  "aws",
		TargetRuntime:   "ecs-fargate",
		OwnershipMarker: "magelift/test/composite",
		Bindings: []SignalBinding{
			{Signal: "metrics", Destination: "cloudwatch", OwnershipMarker: "magelift/test/composite"},
			{Signal: "metrics", Destination: "newrelic", OwnershipMarker: "magelift/test/composite"},
		},
		Alerts: []sdk.AlertIntent{{ID: "availability", Signal: "metrics", Operator: "gt", WindowSeconds: 60, Owner: "oncall", DeduplicationKey: "availability"}},
	}

	result, err := client.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified || !reflect.DeepEqual(result.ResourceRefs, []string{wrapReference("cloudwatch", "cloudwatch:resource"), wrapReference("newrelic", "newrelic:resource")}) {
		t.Fatalf("composite apply result = %#v", result)
	}
	if len(cloudwatch.applyPlans) != 1 || len(cloudwatch.applyPlans[0].Bindings) != 1 || len(cloudwatch.applyPlans[0].Alerts) != 1 {
		t.Fatalf("CloudWatch child plan = %#v", cloudwatch.applyPlans)
	}
	if len(newRelic.applyPlans) != 1 || len(newRelic.applyPlans[0].Bindings) != 1 || len(newRelic.applyPlans[0].Alerts) != 0 {
		t.Fatalf("New Relic child plan = %#v", newRelic.applyPlans)
	}

	observation, err := client.VerifySignal(context.Background(), plan.Bindings[1])
	if err != nil || !observation.Delivered || len(newRelic.verifiedBindings) != 1 {
		t.Fatalf("New Relic signal observation = %#v, err = %v", observation, err)
	}
	if _, err := client.VerifyOperations(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if len(cloudwatch.operations) != 1 || len(newRelic.operations) != 0 || len(cloudwatch.operations[0].Bindings) != 1 {
		t.Fatalf("operation owner calls: cloudwatch=%d newrelic=%d", len(cloudwatch.operations), len(newRelic.operations))
	}

	cloudwatchRef := wrapReference("cloudwatch", "cloudwatch:resource")
	newRelicRef := wrapReference("newrelic", "newrelic:resource")
	cleanup, err := client.Destroy(context.Background(), plan, []string{cloudwatchRef, newRelicRef})
	if err != nil {
		t.Fatal(err)
	}
	if !cleanup.Complete || !cleanup.UnownedPreserved || len(cloudwatch.destroyPlans) != 1 || len(newRelic.destroyPlans) != 1 {
		t.Fatalf("composite cleanup = %#v", cleanup)
	}
	if !reflect.DeepEqual(cloudwatch.destroyRefs, [][]string{{"cloudwatch:resource"}}) || !reflect.DeepEqual(newRelic.destroyRefs, [][]string{{"newrelic:resource"}}) {
		t.Fatalf("destination resource routing = cloudwatch=%v newrelic=%v", cloudwatch.destroyRefs, newRelic.destroyRefs)
	}
}

func TestCompositeLifecycleClientRejectsUnknownBindingBeforeProviderCalls(t *testing.T) {
	cloudwatch := &recordingLifecycleClient{}
	client, err := NewCompositeLifecycleClient(map[string]LifecycleClient{"cloudwatch": cloudwatch}, "cloudwatch")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Apply(context.Background(), Plan{OwnershipMarker: "magelift/test/composite", Bindings: []SignalBinding{{Signal: "metrics", Destination: "unknown"}}})
	if err == nil {
		t.Fatal("unknown destination was accepted")
	}
	if len(cloudwatch.applyPlans) != 0 {
		t.Fatalf("provider was called for unknown destination: %#v", cloudwatch.applyPlans)
	}
}

func TestCompositeLifecycleClientSupportsOperationsWithoutBindings(t *testing.T) {
	owner := &recordingLifecycleClient{}
	other := &recordingLifecycleClient{}
	client, err := NewCompositeLifecycleClient(map[string]LifecycleClient{"owner": owner, "other": other}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Apply(context.Background(), Plan{
		TargetProvider:  "aws",
		TargetRuntime:   "ecs",
		OwnershipMarker: "magelift/test/operations-only",
		Alerts:          []sdk.AlertIntent{{ID: "availability", Signal: "metrics", Operator: "gt", WindowSeconds: 60, Owner: "oncall", DeduplicationKey: "availability"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(owner.applyPlans) != 1 || len(other.applyPlans) != 0 {
		t.Fatalf("operation-only routing = owner=%d other=%d", len(owner.applyPlans), len(other.applyPlans))
	}
}

func TestCompositeLifecycleClientRejectsUnscopedDestroyReference(t *testing.T) {
	client, err := NewCompositeLifecycleClient(map[string]LifecycleClient{"cloudwatch": &recordingLifecycleClient{}}, "cloudwatch")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Destroy(context.Background(), Plan{TargetProvider: "aws", TargetRuntime: "ecs", OwnershipMarker: "magelift/test/composite"}, []string{"cloudwatch:resource"})
	if err == nil {
		t.Fatal("unscoped resource reference was accepted")
	}
}
