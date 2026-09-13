package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

type scriptedNativeOperations struct {
	start   NativeOperationObservation
	polls   []NativeOperationObservation
	items   []InventoryResource
	err     error
	started bool
}

func (backend *scriptedNativeOperations) Start(_ context.Context, _ sdk.ResilienceOperationRequest) (NativeOperationObservation, error) {
	backend.started = true
	return backend.start, backend.err
}

func (backend *scriptedNativeOperations) Poll(_ context.Context, _ string) (NativeOperationObservation, error) {
	if len(backend.polls) == 0 {
		return NativeOperationObservation{}, errors.New("no scripted poll")
	}
	observation := backend.polls[0]
	backend.polls = backend.polls[1:]
	return observation, nil
}

func (backend *scriptedNativeOperations) Inventory(context.Context, string) ([]InventoryResource, error) {
	return append([]InventoryResource(nil), backend.items...), backend.err
}

func operationRequest() sdk.ResilienceOperationRequest {
	return sdk.ResilienceOperationRequest{Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, FixtureID: "fixture-1", OwnershipMarker: "magelift/test/operations", IdempotencyKey: "operation-1"}
}

func TestNormalizedOperationClientMapsProviderStatesAndInventory(t *testing.T) {
	backend := &scriptedNativeOperations{
		start: NativeOperationObservation{Status: "accepted", Action: sdk.ResilienceBackup, OperationID: "op-1", OwnershipMarker: "magelift/test/operations", OwnershipVerified: true, IdempotencyVerified: true},
		polls: []NativeOperationObservation{{Status: "completed", Action: sdk.ResilienceBackup, OperationID: "op-1", OwnershipMarker: "magelift/test/operations", OwnershipVerified: true, IdempotencyVerified: true, ResourceRefs: []string{"backup:1"}}},
		items: []InventoryResource{{Identity: "backup:1", Owned: true, Live: true}, {Identity: "user:1", Owned: false, Live: true}},
	}
	client, err := NewNormalizedOperationClient(backend)
	if err != nil {
		t.Fatal(err)
	}
	started, err := client.Start(context.Background(), operationRequest())
	if err != nil || started.Status != sdk.ResilienceOperationPending {
		t.Fatalf("start = %#v, err = %v", started, err)
	}
	polled, err := client.Poll(context.Background(), "op-1")
	if err != nil || polled.Status != sdk.ResilienceOperationSucceeded || len(polled.ResourceRefs) != 1 {
		t.Fatalf("poll = %#v, err = %v", polled, err)
	}
	resources, err := client.Inventory(context.Background(), operationRequest().OwnershipMarker)
	if err != nil || len(resources) != 2 || resources[1].Owned {
		t.Fatalf("inventory = %#v, err = %v", resources, err)
	}
}

func TestNormalizedOperationClientFailsClosedOnStatusIdentityAndSecretShapedFailure(t *testing.T) {
	secret := "token=secret-value"
	backend := &scriptedNativeOperations{start: NativeOperationObservation{Status: "failed", Action: sdk.ResilienceBackup, OperationID: "op-1", OwnershipMarker: "magelift/test/operations", OwnershipVerified: true, IdempotencyVerified: true, Detail: secret}}
	client, err := NewNormalizedOperationClient(backend)
	if err != nil {
		t.Fatal(err)
	}
	started, err := client.Start(context.Background(), operationRequest())
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != sdk.ResilienceOperationFailed || started.Detail == secret || strings.Contains(started.Detail, "secret-value") {
		t.Fatalf("normalized failed observation = %#v", started)
	}

	backend.start = NativeOperationObservation{Status: "mystery", Action: sdk.ResilienceBackup, OperationID: "op-1", OwnershipMarker: "magelift/test/operations", OwnershipVerified: true, IdempotencyVerified: true}
	if _, err := client.Start(context.Background(), operationRequest()); err == nil || !strings.Contains(err.Error(), "unknown status") {
		t.Fatalf("unknown status error = %v", err)
	}
	backend.start = NativeOperationObservation{Status: "succeeded", Action: sdk.ResilienceBackup, OperationID: "op-1", OwnershipMarker: "magelift/test/operations", OwnershipVerified: true, IdempotencyVerified: true}
	backend.polls = []NativeOperationObservation{{Status: "succeeded", Action: sdk.ResilienceBackup, OperationID: "other", OwnershipMarker: "magelift/test/operations", OwnershipVerified: true, IdempotencyVerified: true}}
	if _, err := client.Poll(context.Background(), "different"); err == nil || !strings.Contains(err.Error(), "poll returned identity") {
		t.Fatalf("poll identity error = %v", err)
	}
}

func TestNormalizedOperationClientHonorsCancellationBeforeProviderCall(t *testing.T) {
	backend := &scriptedNativeOperations{start: NativeOperationObservation{Status: "succeeded", Action: sdk.ResilienceBackup, OwnershipMarker: "magelift/test/operations", OwnershipVerified: true, IdempotencyVerified: true}}
	client, err := NewNormalizedOperationClient(backend)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Start(ctx, operationRequest()); !errors.Is(err, context.Canceled) || backend.started {
		t.Fatalf("canceled start = %v, started = %v", err, backend.started)
	}
}
