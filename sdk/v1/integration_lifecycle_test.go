package v1

import (
	"context"
	"strings"
	"testing"
)

func TestValidateEdgeExecutionResultRequiresOriginHealthProof(t *testing.T) {
	tests := []struct {
		name      string
		action    EdgeAction
		proofRefs []string
		wantError bool
	}{
		{name: "apply without health proof", action: EdgeApply, proofRefs: []string{"aws.cloudfront.ownership"}, wantError: true},
		{name: "verify without health proof", action: EdgeVerify, proofRefs: []string{"gcp.edge.tls"}, wantError: true},
		{name: "failover without health proof", action: EdgeFailover, proofRefs: []string{"fastly:routing"}, wantError: true},
		{name: "rollback without health proof", action: EdgeRollback, proofRefs: []string{"ovh.edge.octavia"}, wantError: true},
		{name: "namespaced health proof", action: EdgeApply, proofRefs: []string{"aws.cloudfront.origin-health"}, wantError: false},
		{name: "destroy does not require origin health", action: EdgeDestroy, proofRefs: []string{"aws.cloudfront.owning-service-inventory-empty"}, wantError: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := EdgeExecutionRequest{
				Action:          test.action,
				OwnershipMarker: "magelift/test/edge",
				Plan:            EdgePlan{OwnershipMarker: "magelift/test/edge"},
			}
			result := EdgeExecutionResult{
				Action:              test.action,
				OperationID:         "edge-operation-1",
				ProofRefs:           test.proofRefs,
				OwnershipMarker:     request.OwnershipMarker,
				OwnershipVerified:   true,
				IdempotencyVerified: true,
			}

			err := ValidateEdgeExecutionResult(request, result)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), EdgeProofOriginHealth) {
					t.Fatalf("ValidateEdgeExecutionResult() error = %v, want an origin-health proof error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateEdgeExecutionResult() error = %v", err)
			}
		})
	}
}

type lifecycleObservabilityAdapter struct {
	descriptor   ObservabilityAdapterDescriptor
	planCalls    int
	executeCalls int
}

func (adapter *lifecycleObservabilityAdapter) ObservabilityDescriptor() ObservabilityAdapterDescriptor {
	return adapter.descriptor
}

func (adapter *lifecycleObservabilityAdapter) PlanObservability(_ context.Context, request ObservabilityPlanRequest) (ObservabilityPlan, error) {
	adapter.planCalls++
	return ObservabilityPlan{
		AdapterID:       adapter.descriptor.ID,
		TargetProvider:  request.TargetProvider,
		TargetRuntime:   request.TargetRuntime,
		OwnershipMarker: request.Intent.OwnershipMarker,
		NativeReference: request.Intent.NativeReference,
	}, nil
}

func (adapter *lifecycleObservabilityAdapter) ExecuteObservability(_ context.Context, request ObservabilityExecutionRequest) (ObservabilityExecutionResult, error) {
	adapter.executeCalls++
	return ObservabilityExecutionResult{
		Action:              request.Action,
		OwnershipMarker:     request.OwnershipMarker,
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}, nil
}

func TestRunObservabilityLifecycleRequiresAdvertisedAction(t *testing.T) {
	tests := []struct {
		name         string
		action       ObservabilityAction
		capabilities []ObservabilityAction
		wantError    bool
	}{
		{name: "advertised apply executes", action: ObservabilityApply, capabilities: []ObservabilityAction{ObservabilityApply}},
		{name: "advertised verify executes", action: ObservabilityVerify, capabilities: []ObservabilityAction{ObservabilityVerify}},
		{name: "advertised destroy executes", action: ObservabilityDestroy, capabilities: []ObservabilityAction{ObservabilityDestroy}},
		{name: "unadvertised apply is blocked", action: ObservabilityApply, capabilities: []ObservabilityAction{ObservabilityVerify}, wantError: true},
		{name: "unadvertised verify is blocked", action: ObservabilityVerify, capabilities: []ObservabilityAction{ObservabilityApply}, wantError: true},
		{name: "unadvertised destroy is blocked", action: ObservabilityDestroy, capabilities: []ObservabilityAction{ObservabilityApply, ObservabilityVerify}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := &lifecycleObservabilityAdapter{descriptor: ObservabilityAdapterDescriptor{
				APIVersion: ExtensionAPIVersion, ID: "test.observability", Provider: "aws", Version: "1.0.0", Capabilities: test.capabilities,
			}}
			request := ObservabilityPlanRequest{
				TargetProvider: "aws", TargetRuntime: "ecs-fargate",
				Intent: ObservabilityIntent{NativeProvider: "cloudwatch", OwnershipMarker: "magelift/test/observability"},
			}

			_, _, err := RunObservabilityLifecycle(context.Background(), adapter, request, test.action, "magelift/test/observability/apply", nil, "")
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "does not declare capability") {
					t.Fatalf("RunObservabilityLifecycle() error = %v, want unsupported-action error", err)
				}
				if adapter.planCalls != 0 || adapter.executeCalls != 0 {
					t.Fatalf("unsupported action reached adapter: plan calls = %d, execute calls = %d", adapter.planCalls, adapter.executeCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("RunObservabilityLifecycle() error = %v", err)
			}
			if adapter.planCalls != 1 || adapter.executeCalls != 1 {
				t.Fatalf("advertised action calls = plan %d, execute %d, want 1 each", adapter.planCalls, adapter.executeCalls)
			}
		})
	}
}
