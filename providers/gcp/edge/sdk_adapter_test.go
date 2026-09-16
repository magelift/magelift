package edge

import (
	"context"
	"testing"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
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
	adapter, err := NewLifecycleAdapter(client)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor := adapter.EdgeDescriptor(); descriptor.Provider != "gcp" || descriptor.ID != "gcp.edge" {
		t.Fatalf("descriptor = %#v", descriptor)
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
