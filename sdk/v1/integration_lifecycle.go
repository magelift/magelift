package v1

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// RunEdgeLifecycle is the shared plan/execute gate for native, Fastly, and
// community edge adapters. The adapter owns provider API calls; the SDK owns
// descriptor, target, ownership, idempotency, and result validation.
func RunEdgeLifecycle(ctx context.Context, adapter EdgeAdapter, request EdgePlanRequest, action EdgeAction, idempotencyKey string, resourceReferences []string, approvalReference string) (EdgePlan, EdgeExecutionResult, error) {
	if ctx == nil {
		return EdgePlan{}, EdgeExecutionResult{}, errors.New("edge lifecycle context is required")
	}
	if adapter == nil {
		return EdgePlan{}, EdgeExecutionResult{}, errors.New("edge lifecycle adapter is required")
	}
	if err := ValidateEdgeAdapterDescriptor(adapter.EdgeDescriptor()); err != nil {
		return EdgePlan{}, EdgeExecutionResult{}, fmt.Errorf("validate edge adapter descriptor: %w", err)
	}
	if err := ValidateEdgePlanRequest(request); err != nil {
		return EdgePlan{}, EdgeExecutionResult{}, err
	}
	plan, err := adapter.PlanEdge(ctx, request)
	if err != nil {
		return EdgePlan{}, EdgeExecutionResult{}, fmt.Errorf("plan edge lifecycle: %w", err)
	}
	if err := ValidateEdgePlan(plan, request); err != nil {
		return EdgePlan{}, EdgeExecutionResult{}, fmt.Errorf("validate edge plan: %w", err)
	}
	executionRequest := EdgeExecutionRequest{Plan: plan, Action: action, IdempotencyKey: idempotencyKey, OwnershipMarker: plan.OwnershipMarker, ResourceReferences: append([]string(nil), resourceReferences...), ApprovalReference: approvalReference}
	if err := ValidateEdgeExecutionRequest(executionRequest); err != nil {
		return EdgePlan{}, EdgeExecutionResult{}, err
	}
	result, err := adapter.ExecuteEdge(ctx, executionRequest)
	if err != nil {
		return plan, EdgeExecutionResult{}, fmt.Errorf("execute edge lifecycle %q: %w", action, err)
	}
	if err := ValidateEdgeExecutionResult(executionRequest, result); err != nil {
		return plan, EdgeExecutionResult{}, err
	}
	return plan, result, nil
}

func ValidateEdgeExecutionResult(request EdgeExecutionRequest, result EdgeExecutionResult) error {
	if result.Action != request.Action {
		return fmt.Errorf("edge execution result action %q does not match request %q", result.Action, request.Action)
	}
	if result.OwnershipMarker != request.OwnershipMarker {
		return errors.New("edge execution result ownership marker does not match the request")
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified {
		return errors.New("edge execution result must verify ownership and idempotency")
	}
	if err := validateOpaqueReferences("edge execution result resource reference", append(append([]string(nil), result.ResourceRefs...), result.ProofRefs...)); err != nil {
		return err
	}
	return ValidateEdgeSafetyProofs(request.Action, result.ProofRefs)
}

// ValidateEdgeSafetyProofs is the shared fail-closed gate for traffic-bearing
// edge actions. A configured origin-health reference is intent only; an
// execution result must carry a namespaced origin-health proof from the
// adapter after its provider-owned health check. Destroy is exempt because it
// must preserve cleanup safety even when the origin is unavailable.
func ValidateEdgeSafetyProofs(action EdgeAction, proofRefs []string) error {
	if err := validateOpaqueReferences("edge safety proof reference", proofRefs); err != nil {
		return err
	}
	switch action {
	case EdgeApply, EdgeVerify, EdgeFailover, EdgeRollback:
		if !containsEdgeProof(proofRefs, EdgeProofOriginHealth) {
			return fmt.Errorf("edge %q result must include an %q proof", action, EdgeProofOriginHealth)
		}
	case EdgeDestroy, EdgePurge:
		return nil
	default:
		return fmt.Errorf("invalid edge action %q", action)
	}
	return nil
}

func containsEdgeProof(proofRefs []string, semantic string) bool {
	semantic = strings.ToLower(strings.TrimSpace(semantic))
	for _, reference := range proofRefs {
		reference = strings.ToLower(strings.TrimSpace(reference))
		if reference == semantic {
			return true
		}
		for _, separator := range []string{".", ":", "/"} {
			if strings.HasSuffix(reference, separator+semantic) {
				return true
			}
		}
	}
	return false
}

// RunObservabilityLifecycle is the observability equivalent of
// RunEdgeLifecycle. It keeps native CloudWatch/Google Cloud/Cockpit/OVH
// schemas and New Relic object models behind the same public port.
func RunObservabilityLifecycle(ctx context.Context, adapter ObservabilityAdapter, request ObservabilityPlanRequest, action ObservabilityAction, idempotencyKey string, resourceReferences []string, approvalReference string) (ObservabilityPlan, ObservabilityExecutionResult, error) {
	if ctx == nil {
		return ObservabilityPlan{}, ObservabilityExecutionResult{}, errors.New("observability lifecycle context is required")
	}
	if adapter == nil {
		return ObservabilityPlan{}, ObservabilityExecutionResult{}, errors.New("observability lifecycle adapter is required")
	}
	descriptor := adapter.ObservabilityDescriptor()
	if err := ValidateObservabilityAdapterDescriptor(descriptor); err != nil {
		return ObservabilityPlan{}, ObservabilityExecutionResult{}, fmt.Errorf("validate observability adapter descriptor: %w", err)
	}
	if !observabilityActionSupported(descriptor.Capabilities, action) {
		return ObservabilityPlan{}, ObservabilityExecutionResult{}, fmt.Errorf("observability adapter %q does not declare capability %q", descriptor.ID, action)
	}
	if err := ValidateObservabilityPlanRequest(request); err != nil {
		return ObservabilityPlan{}, ObservabilityExecutionResult{}, err
	}
	plan, err := adapter.PlanObservability(ctx, request)
	if err != nil {
		return ObservabilityPlan{}, ObservabilityExecutionResult{}, fmt.Errorf("plan observability lifecycle: %w", err)
	}
	if err := ValidateObservabilityPlan(plan, request); err != nil {
		return ObservabilityPlan{}, ObservabilityExecutionResult{}, fmt.Errorf("validate observability plan: %w", err)
	}
	executionRequest := ObservabilityExecutionRequest{Plan: plan, Action: action, IdempotencyKey: idempotencyKey, OwnershipMarker: plan.OwnershipMarker, ResourceReferences: append([]string(nil), resourceReferences...), ApprovalReference: approvalReference}
	if err := ValidateObservabilityExecutionRequest(executionRequest); err != nil {
		return ObservabilityPlan{}, ObservabilityExecutionResult{}, err
	}
	if action == ObservabilityApply {
		if err := ValidateObservabilityApplyPlan(plan); err != nil {
			return plan, ObservabilityExecutionResult{}, fmt.Errorf("observability apply blocked before mutation: %w", err)
		}
	}
	result, err := adapter.ExecuteObservability(ctx, executionRequest)
	if err != nil {
		return plan, ObservabilityExecutionResult{}, fmt.Errorf("execute observability lifecycle %q: %w", action, err)
	}
	if err := ValidateObservabilityExecutionResult(executionRequest, result); err != nil {
		return plan, ObservabilityExecutionResult{}, err
	}
	return plan, result, nil
}

func ValidateObservabilityExecutionResult(request ObservabilityExecutionRequest, result ObservabilityExecutionResult) error {
	if result.Action != request.Action {
		return fmt.Errorf("observability execution result action %q does not match request %q", result.Action, request.Action)
	}
	if result.OwnershipMarker != request.OwnershipMarker {
		return errors.New("observability execution result ownership marker does not match the request")
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified {
		return errors.New("observability execution result must verify ownership and idempotency")
	}
	return validateOpaqueReferences("observability execution result resource reference", append(append([]string(nil), result.ResourceRefs...), result.ProofRefs...))
}
