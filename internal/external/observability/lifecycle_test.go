package observability

import (
	"context"
	"strings"
	"testing"
)

type lifecycleBackend struct {
	applyResult LifecycleResult
	resources   []InventoryResource
	applyCalls  int
	destroyErr  error
}

func (backend *lifecycleBackend) Apply(context.Context, Plan) (LifecycleResult, error) {
	backend.applyCalls++
	return backend.applyResult, nil
}

func (*lifecycleBackend) VerifySignal(_ context.Context, binding SignalBinding) (SignalObservation, error) {
	return SignalObservation{Signal: binding.Signal, Destination: binding.Destination}, nil
}

func (*lifecycleBackend) VerifyOperations(context.Context, Plan) (OperationalObservation, error) {
	return OperationalObservation{}, nil
}

func (backend *lifecycleBackend) Destroy(context.Context, Plan, []string) error {
	return backend.destroyErr
}

func (backend *lifecycleBackend) Inventory(context.Context, string) ([]InventoryResource, error) {
	return append([]InventoryResource(nil), backend.resources...), nil
}

func TestManagedLifecycleClientChecksApplyResultAndInventory(t *testing.T) {
	backend := &lifecycleBackend{applyResult: LifecycleResult{
		OperationID: "apply-1", ResourceRefs: []string{"telemetry:group"}, ProofRefs: []string{"telemetry:ownership"}, OwnershipVerified: true, IdempotencyVerified: true,
	}, resources: []InventoryResource{
		{Identity: "telemetry:unowned", OwnershipMarker: "other", Live: true},
		{Identity: "telemetry:deleted", OwnershipMarker: "magelift/test", Owned: true, Live: false},
	}}
	client, err := NewManagedLifecycleClient(backend)
	if err != nil {
		t.Fatal(err)
	}
	plan := Plan{OwnershipMarker: "magelift/test"}
	result, err := client.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationID != "apply-1" || backend.applyCalls != 1 {
		t.Fatalf("apply result/calls = %#v/%d", result, backend.applyCalls)
	}
	cleanup, err := client.VerifyCleanup(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !cleanup.Complete || !cleanup.UnownedPreserved || len(cleanup.ResourceRefs) != 1 || cleanup.ResourceRefs[0] != "telemetry:unowned" {
		t.Fatalf("cleanup = %#v", cleanup)
	}
}

func TestManagedLifecycleClientKeepsDelayedOwnedDeletionIncomplete(t *testing.T) {
	backend := &lifecycleBackend{resources: []InventoryResource{{Identity: "telemetry:owned", OwnershipMarker: "magelift/test", Owned: true, Live: true}}}
	client, err := NewManagedLifecycleClient(backend)
	if err != nil {
		t.Fatal(err)
	}
	cleanup, err := client.Destroy(context.Background(), Plan{OwnershipMarker: "magelift/test"}, []string{"telemetry:owned"})
	if err != nil {
		t.Fatal(err)
	}
	if cleanup.Complete || !strings.Contains(cleanup.Reason, "remain") {
		t.Fatalf("delayed cleanup = %#v", cleanup)
	}
}

func TestManagedLifecycleClientRejectsUnsafeBackendResultBeforeCleanup(t *testing.T) {
	backend := &lifecycleBackend{applyResult: LifecycleResult{OperationID: "apply-1", ResourceRefs: []string{"secret\nvalue"}, OwnershipVerified: true, IdempotencyVerified: true}}
	client, err := NewManagedLifecycleClient(backend)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Apply(context.Background(), Plan{OwnershipMarker: "magelift/test"})
	if err == nil || !strings.Contains(err.Error(), "single-line") {
		t.Fatalf("unsafe result error = %v", err)
	}
}
