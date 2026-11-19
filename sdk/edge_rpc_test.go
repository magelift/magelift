package sdk

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEdgeRPCTransportsOpaqueStateFlatAndBound(t *testing.T) {
	envelope := Envelope{Account: "123456789012", Environment: "staging", StackName: "shop-staging"}
	stored := StoredPlan{Provider: "aws", Runtime: "ecs-fargate", StackName: envelope.StackName}
	binding := EdgeRPCBindingFor(envelope, stored)
	plan := EdgePlan{AdapterID: "aws.edge.native", TargetProvider: "aws", TargetRuntime: "ecs-fargate", OwnershipMarker: "magelift/test", Opaque: map[string]string{"state": "planned"}}
	encodedPlan, err := EncodeEdgePlanRPC(plan, binding)
	if err != nil {
		t.Fatal(err)
	}
	var flattened map[string]json.RawMessage
	if err := json.Unmarshal(encodedPlan, &flattened); err != nil {
		t.Fatal(err)
	}
	if flattened["adapterId"] == nil || flattened["plan"] != nil || string(flattened["opaque"]) != `{"state":"planned"}` {
		t.Fatalf("edge plan RPC is not a flat opaque envelope: %s", encodedPlan)
	}
	decodedPlan, decodedBinding, version, err := DecodeEdgePlanRPC(encodedPlan)
	if err != nil || version != EdgeRPCOpaqueVersion || decodedBinding != binding {
		t.Fatalf("decoded edge plan version=%d binding=%#v err=%v", version, decodedBinding, err)
	}
	opaque, ok := decodedPlan.Opaque.(json.RawMessage)
	if !ok || string(opaque) != `{"state":"planned"}` {
		t.Fatalf("decoded opaque state = %#v", decodedPlan.Opaque)
	}
	encodedRequest, err := EncodeEdgeExecutionRPC(EdgeExecutionRequest{
		Plan: decodedPlan, Action: EdgePurge, IdempotencyKey: "purge/staging/nonce", OwnershipMarker: plan.OwnershipMarker,
	}, binding)
	if err != nil {
		t.Fatal(err)
	}
	var flatRequest map[string]json.RawMessage
	if err := json.Unmarshal(encodedRequest, &flatRequest); err != nil {
		t.Fatal(err)
	}
	if flatRequest["plan"] == nil || flatRequest["request"] != nil || !strings.Contains(string(flatRequest["planOpaque"]), `"state":"planned"`) {
		t.Fatalf("edge execution RPC is not a flat opaque envelope: %s", encodedRequest)
	}
	decodedRequest, decodedBinding, version, err := DecodeEdgeExecutionRPC(encodedRequest)
	if err != nil || version != EdgeRPCOpaqueVersion || decodedBinding != binding {
		t.Fatalf("decoded edge request version=%d binding=%#v err=%v", version, decodedBinding, err)
	}
	if got, ok := decodedRequest.Plan.Opaque.(json.RawMessage); !ok || string(got) != `{"state":"planned"}` {
		t.Fatalf("execution opaque state = %#v", decodedRequest.Plan.Opaque)
	}
}

func TestEdgeRPCRejectsUnknownMinorAndMismatchedBinding(t *testing.T) {
	envelope := Envelope{Account: "123456789012", Environment: "staging", StackName: "shop-staging"}
	stored := StoredPlan{Provider: "aws", Runtime: "ecs-fargate", StackName: envelope.StackName}
	binding := EdgeRPCBindingFor(envelope, stored)
	plan, err := EncodeEdgePlanRPC(EdgePlan{AdapterID: "aws.edge.native", TargetProvider: "aws", TargetRuntime: "ecs-fargate", OwnershipMarker: "m"}, binding)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(plan, &wire); err != nil {
		t.Fatal(err)
	}
	wire["rpcVersion"] = json.RawMessage("2")
	unknown, _ := json.Marshal(wire)
	if _, _, _, err := DecodeEdgePlanRPC(unknown); err == nil {
		t.Fatal("unknown edge RPC minor was accepted")
	}
	wrong := binding
	wrong.Account = "999999999999"
	if err := wrong.ValidateFor(envelope, stored); err == nil {
		t.Fatal("edge plan binding accepted another account")
	}
}
