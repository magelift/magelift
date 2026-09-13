package observability

import (
	"context"
	"testing"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
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

type adapterFactory func(context.Context, provider.ClientRequest) (sdk.ObservabilityAdapter, error)

func (factory adapterFactory) NewObservabilityClient(ctx context.Context, request provider.ClientRequest) (sdk.ObservabilityAdapter, error) {
	return factory(ctx, request)
}

func TestSDKAdapterBridgeValidatesScalewayIdentity(t *testing.T) {
	adapter, err := NewLifecycleAdapter(lifecycleStub{})
	if err != nil {
		t.Fatal(err)
	}
	if descriptor := adapter.ObservabilityDescriptor(); descriptor.Provider != "scaleway" || descriptor.ID != "scaleway.observability" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	called := false
	factory := adapterFactory(func(context.Context, provider.ClientRequest) (sdk.ObservabilityAdapter, error) {
		called = true
		return adapter, nil
	})
	request := provider.ClientRequest{TargetID: "scaleway-kapsule", Provider: "scaleway", Runtime: "kapsule", AccountOrProjectRef: "project-id", Region: "fr-par", OwnershipMarker: "magelift/test"}
	if _, err := NewFromFactory(context.Background(), factory, request); err != nil || !called {
		t.Fatalf("valid factory construction = %v, called = %v", err, called)
	}
	request.Provider = "ovh"
	called = false
	if _, err := NewFromFactory(context.Background(), factory, request); err == nil || called {
		t.Fatalf("provider mismatch = %v, called = %v", err, called)
	}
}

func TestCombinedSDKAdapterKeepsScalewayAsOriginProvider(t *testing.T) {
	adapter, err := NewCombinedLifecycleAdapter(lifecycleStub{}, lifecycleStub{})
	if err != nil {
		t.Fatal(err)
	}
	if descriptor := adapter.ObservabilityDescriptor(); descriptor.Provider != "scaleway" || descriptor.ID != "scaleway.observability" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
}
