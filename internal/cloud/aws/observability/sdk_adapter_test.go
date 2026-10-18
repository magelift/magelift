package observability

import (
	"context"
	"testing"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
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

func TestSDKAdapterBridgeValidatesAWSIdentity(t *testing.T) {
	adapter, err := NewLifecycleAdapter(lifecycleStub{})
	if err != nil {
		t.Fatal(err)
	}
	if descriptor := adapter.ObservabilityDescriptor(); descriptor.Provider != "aws" || descriptor.ID != "aws.observability" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	called := false
	factory := adapterFactory(func(context.Context, provider.ClientRequest) (sdk.ObservabilityAdapter, error) {
		called = true
		return adapter, nil
	})
	request := provider.ClientRequest{TargetID: "aws-ecs", Provider: "aws", Runtime: "ecs-fargate", AccountOrProjectRef: "123456789012", Region: "eu-west-1", OwnershipMarker: "magelift/test"}
	if _, err := NewFromFactory(context.Background(), factory, request); err != nil || !called {
		t.Fatalf("valid factory construction = %v, called = %v", err, called)
	}
	request.Provider = "gcp"
	called = false
	if _, err := NewFromFactory(context.Background(), factory, request); err == nil || called {
		t.Fatalf("provider mismatch = %v, called = %v", err, called)
	}
}

func TestCombinedSDKAdapterKeepsAWSAsOriginProvider(t *testing.T) {
	adapter, err := NewCombinedLifecycleAdapter(lifecycleStub{}, lifecycleStub{})
	if err != nil {
		t.Fatal(err)
	}
	if descriptor := adapter.ObservabilityDescriptor(); descriptor.Provider != "aws" || descriptor.ID != "aws.observability" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
}
