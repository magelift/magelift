package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestEdgeProxyRoundTrip(t *testing.T) {
	t.Parallel()
	adapter := stubEdgeAdapter{
		plan:   sdk.EdgePlan{AdapterID: "gcp.edge", OwnershipMarker: "magelift/test"},
		result: sdk.EdgeExecutionResult{OperationID: "op-1", OwnershipVerified: true, IdempotencyVerified: true},
	}
	server := &Server{NewEdgeAdapter: func(context.Context, string) (sdk.EdgeAdapter, error) { return adapter, nil }}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	planReq, err := json.Marshal(sdk.EdgePlanRequest{TargetProvider: "gcp", TargetRuntime: "gke-autopilot"})
	if err != nil {
		t.Fatal(err)
	}
	planned, operr := server.EdgePlan(context.Background(), &sdk.EdgePlanCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, RequestJSON: planReq})
	if operr != nil {
		t.Fatal(operr)
	}
	var decoded sdk.EdgePlan
	if err := json.Unmarshal(planned.PlanJSON, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.AdapterID != "gcp.edge" {
		t.Fatalf("plan = %#v", decoded)
	}
	execReq, err := json.Marshal(sdk.EdgeExecutionRequest{Plan: decoded, Action: sdk.EdgePurge, IdempotencyKey: "purge/staging"})
	if err != nil {
		t.Fatal(err)
	}
	executed, operr := server.EdgeExecute(context.Background(), &sdk.EdgeExecuteCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, RequestJSON: execReq})
	if operr != nil {
		t.Fatal(operr)
	}
	var execDecoded sdk.EdgeExecutionResult
	if err := json.Unmarshal(executed.ResultJSON, &execDecoded); err != nil {
		t.Fatal(err)
	}
	if execDecoded.OperationID != "op-1" {
		t.Fatalf("result = %#v", execDecoded)
	}
	if _, operr := server.EdgePlan(context.Background(), &sdk.EdgePlanCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, RequestJSON: []byte("{nope")}); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("malformed request error = %v", operr)
	}
}

func TestEdgeProxyCapabilityErrorIsInvalid(t *testing.T) {
	t.Parallel()
	adapter := stubEdgeAdapter{err: sdk.EdgeCapabilityError{AdapterID: "gcp.edge", Action: sdk.EdgePurge, Status: sdk.EdgeCapabilityUnsupported, Reason: "no cdn"}}
	server := &Server{NewEdgeAdapter: func(context.Context, string) (sdk.EdgeAdapter, error) { return adapter, nil }}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	planReq, err := json.Marshal(sdk.EdgePlanRequest{TargetProvider: "gcp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, operr := server.EdgePlan(context.Background(), &sdk.EdgePlanCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, RequestJSON: planReq}); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("capability error = %v", operr)
	}
}

func TestResilienceProxyRoundTrip(t *testing.T) {
	t.Parallel()
	adapter := stubResilienceAdapter{
		plan:   sdk.ResiliencePlan{AdapterID: "gcp.resilience", OwnershipMarker: "magelift/test"},
		result: sdk.ResilienceExecutionResult{OperationID: "op-2", OwnershipVerified: true, IdempotencyVerified: true},
	}
	server := &Server{NewResilienceAdapter: func(context.Context, string) (sdk.ResilienceAdapter, error) { return adapter, nil }}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	planReq, err := json.Marshal(sdk.ResiliencePlanRequest{OwnershipMarker: "magelift/test"})
	if err != nil {
		t.Fatal(err)
	}
	planned, operr := server.ResiliencePlan(context.Background(), &sdk.ResiliencePlanCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, RequestJSON: planReq})
	if operr != nil {
		t.Fatal(operr)
	}
	var decoded sdk.ResiliencePlan
	if err := json.Unmarshal(planned.PlanJSON, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.AdapterID != "gcp.resilience" {
		t.Fatalf("plan = %#v", decoded)
	}
	execReq, err := json.Marshal(sdk.ResilienceExecutionRequest{Plan: decoded, StageID: "stage-1", IdempotencyKey: "exec/1"})
	if err != nil {
		t.Fatal(err)
	}
	executed, operr := server.ResilienceExecute(context.Background(), &sdk.ResilienceExecuteCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, RequestJSON: execReq})
	if operr != nil {
		t.Fatal(operr)
	}
	var execDecoded sdk.ResilienceExecutionResult
	if err := json.Unmarshal(executed.ResultJSON, &execDecoded); err != nil {
		t.Fatal(err)
	}
	if execDecoded.OperationID != "op-2" {
		t.Fatalf("result = %#v", execDecoded)
	}
	if _, operr := server.ResilienceExecute(context.Background(), &sdk.ResilienceExecuteCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, RequestJSON: []byte("{nope")}); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("malformed request error = %v", operr)
	}
}

func TestResilienceProxyCapabilityErrorIsInvalid(t *testing.T) {
	t.Parallel()
	adapter := stubResilienceAdapter{err: sdk.ResilienceCapabilityError{AdapterID: "gcp.resilience", Operation: sdk.ResilienceFence, Status: sdk.ResilienceCapabilityUnsupported, Reason: "no fence"}}
	server := &Server{NewResilienceAdapter: func(context.Context, string) (sdk.ResilienceAdapter, error) { return adapter, nil }}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	planReq, err := json.Marshal(sdk.ResiliencePlanRequest{OwnershipMarker: "magelift/test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, operr := server.ResiliencePlan(context.Background(), &sdk.ResiliencePlanCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, RequestJSON: planReq}); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("capability error = %v", operr)
	}
}

func TestProxyRequiresProject(t *testing.T) {
	t.Parallel()
	server := &Server{}
	spec := testSpec()
	spec.Identity.GCPProject = ""
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, spec))
	envelope.Project = "shop"
	planReq, err := json.Marshal(sdk.EdgePlanRequest{TargetProvider: "gcp"})
	if err != nil {
		t.Fatal(err)
	}
	if _, operr := server.EdgePlan(context.Background(), &sdk.EdgePlanCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, RequestJSON: planReq}); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("missing project error = %v", operr)
	}
}
