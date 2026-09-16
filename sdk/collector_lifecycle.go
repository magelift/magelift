package sdk

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CollectorProofSignalDelivery is the semantic suffix adapters use for proof
// that the requested signals reached the configured destination.
const CollectorProofSignalDelivery = "signal-delivery"

// RunCollectorDeploymentLifecycle is the shared plan/execute gate for
// provider-owned collector deployment adapters. The adapter owns provider API
// calls; the SDK owns request, plan, ownership, idempotency, and proof checks.
func RunCollectorDeploymentLifecycle(ctx context.Context, adapter CollectorDeploymentAdapter, request CollectorDeploymentPlanRequest, action CollectorDeploymentAction, idempotencyKey string, resourceReferences []string, approvalReference string) (CollectorDeploymentPlan, CollectorDeploymentExecutionResult, error) {
	if ctx == nil {
		return CollectorDeploymentPlan{}, CollectorDeploymentExecutionResult{}, errors.New("collector deployment lifecycle context is required")
	}
	if adapter == nil {
		return CollectorDeploymentPlan{}, CollectorDeploymentExecutionResult{}, errors.New("collector deployment lifecycle adapter is required")
	}
	if err := ValidateCollectorDeploymentPlanRequest(request); err != nil {
		return CollectorDeploymentPlan{}, CollectorDeploymentExecutionResult{}, err
	}

	plan, err := adapter.PlanCollector(ctx, request)
	if err != nil {
		return CollectorDeploymentPlan{}, CollectorDeploymentExecutionResult{}, fmt.Errorf("plan collector deployment lifecycle: %w", err)
	}
	if err := ValidateCollectorDeploymentPlan(plan, request); err != nil {
		return CollectorDeploymentPlan{}, CollectorDeploymentExecutionResult{}, fmt.Errorf("validate collector deployment plan: %w", err)
	}

	executionRequest := CollectorDeploymentExecutionRequest{
		Plan:               plan,
		Action:             action,
		IdempotencyKey:     idempotencyKey,
		OwnershipMarker:    plan.OwnershipMarker,
		ResourceReferences: append([]string(nil), resourceReferences...),
		ApprovalReference:  approvalReference,
	}
	if err := ValidateCollectorDeploymentExecutionRequest(executionRequest); err != nil {
		return CollectorDeploymentPlan{}, CollectorDeploymentExecutionResult{}, err
	}

	result, err := adapter.ExecuteCollector(ctx, executionRequest)
	if err != nil {
		return plan, CollectorDeploymentExecutionResult{}, fmt.Errorf("execute collector deployment lifecycle %q: %w", action, err)
	}
	if err := ValidateCollectorDeploymentExecutionResult(executionRequest, result); err != nil {
		return plan, CollectorDeploymentExecutionResult{}, err
	}
	return plan, result, nil
}

// ValidateCollectorDeploymentExecutionResult verifies that a provider
// returned the requested action and ownership scope, then applies the
// action-specific readiness, health, signal-delivery, rollback, or cleanup
// gates. References remain opaque to the SDK.
func ValidateCollectorDeploymentExecutionResult(request CollectorDeploymentExecutionRequest, result CollectorDeploymentExecutionResult) error {
	if result.Action != request.Action {
		return fmt.Errorf("collector execution result action %q does not match request %q", result.Action, request.Action)
	}
	if result.OwnershipMarker != request.OwnershipMarker {
		return errors.New("collector execution result ownership marker does not match the request")
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified {
		return errors.New("collector execution result must verify ownership and idempotency")
	}
	if result.OperationID != "" {
		if err := validateOpaqueReferences("collector execution result operation ID", []string{result.OperationID}); err != nil {
			return err
		}
	}
	if err := validateOpaqueReferences("collector execution result reference", append(append([]string(nil), result.ResourceRefs...), result.ProofRefs...)); err != nil {
		return err
	}

	switch request.Action {
	case CollectorApply, CollectorVerify:
		if !result.ReadyVerified {
			return fmt.Errorf("collector %q execution result must verify readiness", request.Action)
		}
		if !result.HealthVerified {
			return fmt.Errorf("collector %q execution result must verify health", request.Action)
		}
		if !containsCollectorProof(result.ProofRefs, CollectorProofSignalDelivery) {
			return fmt.Errorf("collector %q execution result must include a %q proof", request.Action, CollectorProofSignalDelivery)
		}
	case CollectorRollback:
		if !result.RollbackVerified {
			return errors.New("collector rollback execution result must verify rollback")
		}
	case CollectorDestroy:
		if !result.CleanupVerified {
			return errors.New("collector destroy execution result must verify cleanup")
		}
	default:
		return fmt.Errorf("invalid collector deployment action %q", request.Action)
	}
	return nil
}

func containsCollectorProof(proofRefs []string, semantic string) bool {
	semantic = strings.ToLower(strings.TrimSpace(semantic))
	for _, reference := range proofRefs {
		reference = strings.ToLower(strings.TrimSpace(reference))
		for _, separator := range []string{".", ":", "/"} {
			if strings.HasSuffix(reference, separator+semantic) {
				return true
			}
		}
	}
	return false
}
