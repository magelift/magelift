package edge

import (
	"context"
	"testing"

	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type sdkEdgeStub struct{ descriptor sdk.EdgeAdapterDescriptor }

func (stub sdkEdgeStub) EdgeDescriptor() sdk.EdgeAdapterDescriptor { return stub.descriptor }
func (stub sdkEdgeStub) PlanEdge(context.Context, sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	return sdk.EdgePlan{AdapterID: stub.descriptor.ID}, nil
}
func (stub sdkEdgeStub) ExecuteEdge(context.Context, sdk.EdgeExecutionRequest) (sdk.EdgeExecutionResult, error) {
	return sdk.EdgeExecutionResult{}, nil
}

type sdkEdgeFactory func(context.Context, provider.ClientRequest) (sdk.EdgeAdapter, error)

func (factory sdkEdgeFactory) NewEdgeClient(ctx context.Context, request provider.ClientRequest) (sdk.EdgeAdapter, error) {
	return factory(ctx, request)
}

func TestSDKEdgeAdapterBridgeValidatesGCPIdentity(t *testing.T) {
	client := sdkEdgeStub{descriptor: sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "gcp.edge", Provider: "gcp", Version: "1.0.0", Capabilities: []sdk.EdgeAction{sdk.EdgeApply}}}
	if _, err := NewLifecycleAdapter(client); err != nil {
		t.Fatal(err)
	}
	factoryCalled := false
	factory := sdkEdgeFactory(func(context.Context, provider.ClientRequest) (sdk.EdgeAdapter, error) {
		factoryCalled = true
		return client, nil
	})
	request := provider.ClientRequest{TargetID: "gcp-gke", Provider: "gcp", Runtime: "gke-standard", AccountOrProjectRef: "project", Region: "europe-west1", OwnershipMarker: "magelift/test"}
	if _, err := NewFromFactory(context.Background(), factory, request); err != nil || !factoryCalled {
		t.Fatalf("valid factory construction = %v, called = %v", err, factoryCalled)
	}
	request.Provider = "aws"
	factoryCalled = false
	if _, err := NewFromFactory(context.Background(), factory, request); err == nil || factoryCalled {
		t.Fatalf("provider mismatch = %v, called = %v", err, factoryCalled)
	}
}

func TestCombinedSDKEdgeAdapterKeepsGCPAsOriginProvider(t *testing.T) {
	native := sdkEdgeStub{descriptor: sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "gcp.edge", Provider: "gcp", Version: "1.0.0", Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy}}}
	external := sdkEdgeStub{descriptor: sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "fastly.edge", Provider: "fastly", Version: "1.0.0", Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy}}}
	adapter, err := NewCombinedLifecycleAdapter(native, external)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor := adapter.EdgeDescriptor(); descriptor.Provider != "gcp" || descriptor.ID != "gcp.edge.composite" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
}
