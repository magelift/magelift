package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestNativeEdgeRPCWorksWithRuntimeObservePlanWithoutDigest(t *testing.T) {
	request := testPlanRequest()
	request.RuntimeObserve = true
	request.TargetBlock = []byte("project: example-gcp-project\nregion: europe-west1\nnetworkCidr: 10.20.0.0/16\nencryptionKeySecret: magento-crypt-key\n")
	adapter := stubEdgeAdapter{
		plan:   sdk.EdgePlan{AdapterID: "gcp.edge", TargetProvider: "gcp", TargetRuntime: "gke-autopilot", OwnershipMarker: "magelift/shop/staging", Opaque: json.RawMessage(`{"opaque":true}`)},
		result: sdk.EdgeExecutionResult{Action: sdk.EdgePurge, OperationID: "purge-1", OwnershipMarker: "magelift/shop/staging", OwnershipVerified: true, IdempotencyVerified: true},
	}
	server := &Server{
		Admission:      stubAdmission{},
		NewEdgeAdapter: func(context.Context, string) (sdk.EdgeAdapter, error) { return adapter, nil },
	}
	planned, operr := server.Plan(context.Background(), request)
	if operr != nil {
		t.Fatalf("runtime observe plan: %v", operr)
	}
	if planned.Plan.ImageDigest != "" {
		t.Fatalf("runtime observe plan has unexpected image digest %q", planned.Plan.ImageDigest)
	}

	edgeRequest, err := json.Marshal(sdk.EdgePlanRequest{TargetProvider: "gcp", TargetRuntime: "gke-autopilot"})
	if err != nil {
		t.Fatal(err)
	}
	edgePlan, operr := server.EdgePlan(context.Background(), &sdk.EdgePlanCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan, RequestJSON: edgeRequest,
	})
	if operr != nil {
		t.Fatalf("edge plan: %v", operr)
	}
	decoded, _, _, err := sdk.DecodeEdgePlanRPC(edgePlan.PlanJSON)
	if err != nil {
		t.Fatal(err)
	}
	executionJSON, err := sdk.EncodeEdgeExecutionRPC(sdk.EdgeExecutionRequest{
		Plan: decoded, Action: sdk.EdgePurge, IdempotencyKey: "purge/staging/one", OwnershipMarker: adapter.plan.OwnershipMarker,
	}, sdk.EdgeRPCBindingFor(request.Envelope, planned.Plan))
	if err != nil {
		t.Fatal(err)
	}
	if _, operr := server.EdgeExecute(context.Background(), &sdk.EdgeExecuteCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan, RequestJSON: executionJSON,
	}); operr != nil {
		t.Fatalf("edge execute: %v", operr)
	}

	request.RuntimeObserve = false
	if _, operr := server.Plan(context.Background(), request); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("deploy plan without image digest error = %v", operr)
	}
}
