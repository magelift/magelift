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

func TestSDKEdgeAdapterBridgeValidatesAWSIdentity(t *testing.T) {
	client := sdkEdgeStub{descriptor: sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "aws.edge", Provider: "aws", Version: "1.0.0", Capabilities: []sdk.EdgeAction{sdk.EdgeApply}}}
	if _, err := NewLifecycleAdapter(client); err != nil {
		t.Fatal(err)
	}
	called := false
	factory := sdkEdgeFactory(func(context.Context, provider.ClientRequest) (sdk.EdgeAdapter, error) {
		called = true
		return client, nil
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

func TestCombinedSDKEdgeAdapterKeepsAWSAsOriginProvider(t *testing.T) {
	native := sdkEdgeStub{descriptor: sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "aws.edge", Provider: "aws", Version: "1.0.0", Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy}}}
	external := sdkEdgeStub{descriptor: sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "fastly.edge", Provider: "fastly", Version: "1.0.0", Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy}}}
	adapter, err := NewCombinedLifecycleAdapter(native, external)
	if err != nil {
		t.Fatal(err)
	}
	if descriptor := adapter.EdgeDescriptor(); descriptor.Provider != "aws" || descriptor.ID != "aws.edge.composite" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
}
