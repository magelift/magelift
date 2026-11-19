package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	"gopkg.in/yaml.v3"
)

type runtimeObserveEdgeAdapter struct{}

func (runtimeObserveEdgeAdapter) EdgeDescriptor() sdk.EdgeAdapterDescriptor {
	return sdk.EdgeAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: "aws.edge.native", Provider: "aws", Version: "test",
		Capabilities: []sdk.EdgeAction{sdk.EdgePurge},
	}
}
func (runtimeObserveEdgeAdapter) PlanEdge(_ context.Context, request sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	return sdk.EdgePlan{
		AdapterID: "aws.edge.native", TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime,
		OwnershipMarker: request.Intent.OwnershipMarker, Opaque: json.RawMessage(`{"opaque":true}`),
	}, nil
}
func (runtimeObserveEdgeAdapter) ExecuteEdge(_ context.Context, request sdk.EdgeExecutionRequest) (sdk.EdgeExecutionResult, error) {
	return sdk.EdgeExecutionResult{
		Action: request.Action, OperationID: "purge-1", OwnershipMarker: request.OwnershipMarker,
		OwnershipVerified: true, IdempotencyVerified: true,
	}, nil
}

func TestAWSNativeEdgeRPCWorksWithRuntimeObservePlanWithoutDigest(t *testing.T) {
	planRequest := shopRequest(t, "ecs-fargate")
	target, err := parseTarget(planRequest.TargetBlock)
	if err != nil {
		t.Fatal(err)
	}
	target.ImageDigest = ""
	planRequest.TargetBlock, err = yaml.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	planRequest.RuntimeObserve = true
	server := &Server{NewEdgeAdapter: func(context.Context, platform.StackModule, platform.PlannedStack) (sdk.EdgeAdapter, error) {
		return runtimeObserveEdgeAdapter{}, nil
	}}
	var planned sdk.PlanResult
	if err := server.Plan(planRequest, &planned); err != nil || planned.Error != nil {
		t.Fatalf("runtime observe plan: %v %v", err, planned.Error)
	}

	marker := "magelift/shop/pr-23"
	edgeRequest := sdk.EdgePlanRequest{
		TargetProvider: "aws", TargetRuntime: "ecs-fargate",
		Intent: sdk.EdgeIntent{Mode: "native", NativeProvider: "cloudfront-waf", Domains: []string{planRequest.Envelope.Domain}, TLS: true,
			TLSMode: "existing", DNSMode: "existing", OriginHealthRef: "aws/alb", OwnershipMarker: marker},
	}
	requestJSON, err := json.Marshal(edgeRequest)
	if err != nil {
		t.Fatal(err)
	}
	var edgePlan sdk.EdgePlanResult
	if err := server.EdgePlan(&sdk.EdgePlanCall{ProtocolVersion: sdk.ProtocolV1, Envelope: planRequest.Envelope, Plan: planned.Plan, RequestJSON: requestJSON}, &edgePlan); err != nil || edgePlan.Error != nil {
		t.Fatalf("edge plan: %v %v", err, edgePlan.Error)
	}
	decoded, _, _, err := sdk.DecodeEdgePlanRPC(edgePlan.PlanJSON)
	if err != nil {
		t.Fatal(err)
	}
	execJSON, err := sdk.EncodeEdgeExecutionRPC(sdk.EdgeExecutionRequest{
		Plan: decoded, Action: sdk.EdgePurge, IdempotencyKey: "purge/pr-23/one", OwnershipMarker: marker,
	}, sdk.EdgeRPCBindingFor(planRequest.Envelope, planned.Plan))
	if err != nil {
		t.Fatal(err)
	}
	var execution sdk.EdgeExecuteResult
	if err := server.EdgeExecute(&sdk.EdgeExecuteCall{ProtocolVersion: sdk.ProtocolV1, Envelope: planRequest.Envelope, Plan: planned.Plan, RequestJSON: execJSON}, &execution); err != nil || execution.Error != nil {
		t.Fatalf("edge execute: %v %v", err, execution.Error)
	}
}
