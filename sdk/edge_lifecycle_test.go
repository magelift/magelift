package sdk

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type scriptedEdgeOperations struct {
	plan           EdgePlan
	start          []EdgeOperationRequest
	polls          int
	pollStatus     EdgeOperationStatus
	pollIdentity   string
	inventoryCalls int
	clearAfter     int
	inventory      []EdgeInventoryResource
}

func (api *scriptedEdgeOperations) Plan(context.Context, EdgePlanRequest) (EdgePlan, error) {
	return api.plan, nil
}

func (api *scriptedEdgeOperations) Start(_ context.Context, request EdgeOperationRequest) (EdgeOperationObservation, error) {
	api.start = append(api.start, request)
	return EdgeOperationObservation{
		Status:              EdgeOperationPending,
		Action:              request.Action,
		OperationID:         "edge-operation-1",
		OwnershipMarker:     request.Request.OwnershipMarker,
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}, nil
}

func (api *scriptedEdgeOperations) Poll(_ context.Context, operationID string) (EdgeOperationObservation, error) {
	api.polls++
	observation := EdgeOperationObservation{
		Status:              EdgeOperationSucceeded,
		Action:              EdgeApply,
		OperationID:         operationID,
		ResourceRefs:        []string{"cloudfront:distribution"},
		ProofRefs:           []string{"cloudfront:origin-health"},
		OwnershipMarker:     "magelift/test/edge",
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}
	if api.polls == 1 {
		observation.Status = EdgeOperationRunning
	}
	if api.pollStatus != "" {
		observation.Status = api.pollStatus
	}
	if api.pollIdentity != "" {
		observation.OperationID = api.pollIdentity
	}
	return observation, nil
}

func (api *scriptedEdgeOperations) Inventory(context.Context, string) ([]EdgeInventoryResource, error) {
	api.inventoryCalls++
	if api.clearAfter > 0 && api.inventoryCalls >= api.clearAfter {
		return nil, nil
	}
	return append([]EdgeInventoryResource(nil), api.inventory...), nil
}

func testEdgePlanRequest() EdgePlanRequest {
	return EdgePlanRequest{
		TargetProvider: "aws",
		TargetRuntime:  "ecs-fargate",
		Intent: EdgeIntent{
			Mode:            "native",
			NativeProvider:  "cloudfront",
			OriginHealthRef: "health:origin",
			OwnershipMarker: "magelift/test/edge",
		},
	}
}

func testEdgeDescriptor() EdgeAdapterDescriptor {
	return EdgeAdapterDescriptor{
		APIVersion: ExtensionAPIVersion,
		ID:         "aws.edge.native",
		Provider:   "aws",
		Version:    "1.0.0",
		Capabilities: []EdgeAction{
			EdgeApply, EdgeVerify, EdgeDestroy,
		},
	}
}

func TestOperationBackedEdgeAdapterPlansPollsAndValidatesOwnership(t *testing.T) {
	api := &scriptedEdgeOperations{plan: EdgePlan{
		AdapterID:       "aws.edge.native",
		TargetProvider:  "aws",
		TargetRuntime:   "ecs-fargate",
		OwnershipMarker: "magelift/test/edge",
		Opaque:          map[string]string{"distribution": "cloudfront"},
	}}
	api.inventory = []EdgeInventoryResource{{Identity: "cloudfront:distribution", OwnershipMarker: "magelift/test/edge", Owned: true, Live: true}}
	adapter, err := NewOperationBackedEdgeAdapter(testEdgeDescriptor(), api, EdgeOperationPolicy{Timeout: time.Second, PollInterval: time.Nanosecond, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	request := testEdgePlanRequest()
	plan, err := adapter.PlanEdge(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ExecuteEdge(context.Background(), EdgeExecutionRequest{
		Plan: plan, Action: EdgeApply, IdempotencyKey: "magelift/test/edge/apply", OwnershipMarker: plan.OwnershipMarker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationID != "edge-operation-1" || api.polls != 2 || len(api.start) != 1 {
		t.Fatalf("result or calls = %#v, polls=%d starts=%d", result, api.polls, len(api.start))
	}
	if api.start[0].Provider != "aws" || api.start[0].Request.Plan.Opaque == nil {
		t.Fatalf("provider request = %#v", api.start[0])
	}
	resources, err := adapter.Inventory(context.Background(), plan.OwnershipMarker)
	if err != nil || len(resources) != 1 {
		t.Fatalf("inventory = %#v, error=%v", resources, err)
	}
}

func TestOperationBackedEdgeAdapterRejectsProviderPlanMismatchBeforeMutation(t *testing.T) {
	api := &scriptedEdgeOperations{plan: EdgePlan{
		AdapterID:       "gcp.edge.native",
		TargetProvider:  "aws",
		TargetRuntime:   "ecs-fargate",
		OwnershipMarker: "magelift/test/edge",
	}}
	adapter, err := NewOperationBackedEdgeAdapter(testEdgeDescriptor(), api, EdgeOperationPolicy{Timeout: time.Second, PollInterval: time.Nanosecond, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.PlanEdge(context.Background(), testEdgePlanRequest())
	if err == nil || !strings.Contains(err.Error(), "adapter ID") {
		t.Fatalf("plan error = %v", err)
	}
	if len(api.start) != 0 {
		t.Fatal("provider mutation started after plan rejection")
	}
}

func TestOperationBackedEdgeAdapterReportsTypedUnsupportedAction(t *testing.T) {
	api := &scriptedEdgeOperations{plan: EdgePlan{
		AdapterID:       "aws.edge.native",
		TargetProvider:  "aws",
		TargetRuntime:   "ecs-fargate",
		OwnershipMarker: "magelift/test/edge",
	}}
	adapter, err := NewOperationBackedEdgeAdapter(testEdgeDescriptor(), api, EdgeOperationPolicy{Timeout: time.Second, PollInterval: time.Nanosecond, MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.PlanEdge(context.Background(), testEdgePlanRequest())
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.ExecuteEdge(context.Background(), EdgeExecutionRequest{
		Plan: plan, Action: EdgeFailover, IdempotencyKey: "magelift/test/edge/failover", OwnershipMarker: plan.OwnershipMarker,
	})
	var capabilityErr EdgeCapabilityError
	if !errors.As(err, &capabilityErr) || capabilityErr.Status != EdgeCapabilityUnsupported || capabilityErr.Action != EdgeFailover {
		t.Fatalf("unsupported action error = %v, typed = %#v", err, capabilityErr)
	}
	if len(api.start) != 0 {
		t.Fatal("unsupported edge action reached provider API")
	}
}

func TestValidateEdgeSafetyProofsGatesTrafficActions(t *testing.T) {
	tests := []struct {
		name      string
		action    EdgeAction
		proofs    []string
		wantError bool
	}{
		{name: "apply requires namespaced origin health", action: EdgeApply, proofs: []string{"aws.cloudfront.tls"}, wantError: true},
		{name: "verify requires namespaced origin health", action: EdgeVerify, proofs: []string{"gcp.edge.security-policy"}, wantError: true},
		{name: "failover requires namespaced origin health", action: EdgeFailover, proofs: []string{"fastly.edge.failover"}, wantError: true},
		{name: "rollback requires namespaced origin health", action: EdgeRollback, proofs: []string{"scaleway.edge.rollback"}, wantError: true},
		{name: "namespaced origin health satisfies apply", action: EdgeApply, proofs: []string{"aws.cloudfront.origin-health"}, wantError: false},
		{name: "namespaced origin health satisfies rollback", action: EdgeRollback, proofs: []string{"fastly.edge.origin-health"}, wantError: false},
		{name: "destroy remains exempt", action: EdgeDestroy, proofs: []string{"aws.cloudfront.owning-service-inventory-empty"}, wantError: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateEdgeSafetyProofs(test.action, test.proofs)
			if (err != nil) != test.wantError {
				t.Fatalf("ValidateEdgeSafetyProofs(%q, %v) error = %v, wantError=%t", test.action, test.proofs, err, test.wantError)
			}
			if test.wantError && !strings.Contains(err.Error(), EdgeProofOriginHealth) {
				t.Fatalf("error = %v, want origin-health proof diagnostic", err)
			}
		})
	}
}

func TestWaitForEdgeOperationRejectsUnknownAndMismatchedStates(t *testing.T) {
	api := &scriptedEdgeOperations{}
	policy := EdgeOperationPolicy{Timeout: time.Second, PollInterval: time.Nanosecond, MaxAttempts: 1}
	api.pollStatus = EdgeOperationStatus("unknown")
	if _, err := WaitForEdgeOperation(context.Background(), api, "operation", policy); err == nil {
		t.Fatal("unknown edge operation status was accepted")
	}
	api.polls = 0
	api.pollStatus = ""
	api.pollIdentity = "different-operation"
	if _, err := WaitForEdgeOperation(context.Background(), api, "operation", policy); err == nil {
		t.Fatal("mismatched edge operation identity was accepted")
	}
}

func TestWaitForEdgeCleanupWaitsForOwnedTombstonesAndPreservesUnownedResources(t *testing.T) {
	api := &scriptedEdgeOperations{
		clearAfter: 2,
		inventory: []EdgeInventoryResource{
			{Identity: "cloudfront:distribution", OwnershipMarker: "magelift/test/edge", Owned: true, Live: true},
			{Identity: "cloudfront:customer-distribution", OwnershipMarker: "customer", Owned: false, Live: true},
		},
	}
	resources, err := WaitForEdgeCleanup(context.Background(), api, "magelift/test/edge", EdgeOperationPolicy{Timeout: time.Second, PollInterval: time.Nanosecond, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 0 || api.inventoryCalls != 2 {
		t.Fatalf("cleanup resources=%v inventory calls=%d", resources, api.inventoryCalls)
	}
}
