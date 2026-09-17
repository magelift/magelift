package observability

import (
	"context"
	"testing"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
)

type lifecycleStub struct{}

func (lifecycleStub) Apply(context.Context, providerobservability.Plan) (providerobservability.LifecycleResult, error) {
	return providerobservability.LifecycleResult{OperationID: "apply", OwnershipVerified: true, IdempotencyVerified: true}, nil
}
func (lifecycleStub) VerifySignal(_ context.Context, binding providerobservability.SignalBinding) (providerobservability.SignalObservation, error) {
	return providerobservability.SignalObservation{Signal: binding.Signal, Destination: binding.Destination}, nil
}
func (lifecycleStub) VerifyOperations(context.Context, providerobservability.Plan) (providerobservability.OperationalObservation, error) {
	return providerobservability.OperationalObservation{}, nil
}
func (lifecycleStub) Destroy(context.Context, providerobservability.Plan, []string) (providerobservability.CleanupObservation, error) {
	return providerobservability.CleanupObservation{Complete: true, UnownedPreserved: true}, nil
}
func (lifecycleStub) VerifyCleanup(context.Context, providerobservability.Plan) (providerobservability.CleanupObservation, error) {
	return providerobservability.CleanupObservation{Complete: true, UnownedPreserved: true}, nil
}

func TestSDKAdapterBridgeValidatesGCPIdentity(t *testing.T) {
	adapter, err := NewLifecycleAdapter(lifecycleStub{})
	if err != nil {
		t.Fatal(err)
	}
	if descriptor := adapter.ObservabilityDescriptor(); descriptor.Provider != "gcp" || descriptor.ID != "gcp.observability" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
}

func TestCombinedSDKAdapterKeepsGCPAsOriginProvider(t *testing.T) {
	adapter, err := NewCombinedLifecycleAdapter(lifecycleStub{}, lifecycleStub{})
	if err != nil {
		t.Fatal(err)
	}
	if descriptor := adapter.ObservabilityDescriptor(); descriptor.Provider != "gcp" || descriptor.ID != "gcp.observability" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
}
