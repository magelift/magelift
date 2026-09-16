package resilience

import (
	"context"
	"errors"
	"fmt"
	"strings"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

// ProjectionOperation owns the provider-neutral execution and proof mapping
// for reconstructible search indexes and caches. Provider packages only
// select the operation prefix and encode their own resumable state; they do
// not get separate meanings for application reads, cache loss, or search
// completeness.
type ProjectionOperation struct {
	provider  sdk.ProviderID
	lifecycle *cloudrecovery.ProjectionLifecycle
}

// NewProjectionOperation constructs the shared projection operation boundary
// around an injected runtime adapter. Runtime-specific command transports and
// verifiers remain outside this package.
func NewProjectionOperation(providerID sdk.ProviderID, lifecycle *cloudrecovery.ProjectionLifecycle) (*ProjectionOperation, error) {
	if strings.TrimSpace(string(providerID)) == "" {
		return nil, errors.New("projection provider is required")
	}
	if lifecycle == nil {
		return nil, errors.New("projection lifecycle is required")
	}
	return &ProjectionOperation{provider: providerID, lifecycle: lifecycle}, nil
}

// Execute runs the one supported reconstructible action through the shared
// lifecycle. Backup is intentionally not accepted for either data class.
func (operation *ProjectionOperation) Execute(ctx context.Context, state cloudrecovery.OperationState) (cloudrecovery.ProjectionResult, error) {
	if operation == nil || operation.lifecycle == nil {
		return cloudrecovery.ProjectionResult{}, errors.New("projection operation is not configured")
	}
	request := cloudrecovery.ProjectionRequest{
		Action:          state.Action,
		DataClass:       state.DataClass,
		SourceReference: state.Resource,
		TargetReference: state.Target,
		OperationID:     state.OperationRef,
		Destination:     state.Destination,
		FixtureID:       state.FixtureID,
		OwnershipMarker: state.OwnershipMarker,
		IdempotencyKey:  state.IdempotencyKey,
	}
	switch state.Action {
	case sdk.ResilienceRestore:
		result, err := operation.lifecycle.Rebuild(ctx, request)
		if err != nil {
			return cloudrecovery.ProjectionResult{}, fmt.Errorf("%s projection rebuild: %w", operation.provider, err)
		}
		return result, nil
	case sdk.ResilienceIntegrityCheck:
		result, err := operation.lifecycle.Verify(ctx, request)
		if err != nil {
			return cloudrecovery.ProjectionResult{}, fmt.Errorf("%s projection integrity check: %w", operation.provider, err)
		}
		return result, nil
	default:
		return cloudrecovery.ProjectionResult{}, fmt.Errorf("%s projection does not implement action %q", operation.provider, state.Action)
	}
}

// Observation normalizes one projection result into the provider operation
// observation used by the shared SDK adapter. It never promotes a projection
// to durable backup evidence: encryption, retention, and protection remain
// intentionally absent from ProjectionProof.
func (operation *ProjectionOperation) Observation(
	state cloudrecovery.OperationState,
	operationID string,
	result cloudrecovery.ProjectionResult,
) provider.NativeOperationObservation {
	refs := make([]string, 0, 1)
	if strings.TrimSpace(result.ResourceReference) != "" {
		refs = append(refs, result.ResourceReference)
	}
	proofRefs := append([]string(nil), result.ProofReferences...)
	proofRefs = append(proofRefs, string(operation.provider)+"."+state.DataClass+".projection")
	evidence := make([]sdk.ResilienceProofEvidence, 0, 1)
	if result.Status == sdk.ResilienceOperationSucceeded {
		evidence = append(evidence, cloudrecovery.ProjectionProof(cloudrecovery.ProjectionRequest{
			DataClass:       state.DataClass,
			Destination:     state.Destination,
			FixtureID:       state.FixtureID,
			OwnershipMarker: state.OwnershipMarker,
		}, result))
	}
	return provider.NativeOperationObservation{
		Status:              string(result.Status),
		Action:              state.Action,
		OperationID:         operationID,
		ResourceRefs:        refs,
		ProofRefs:           proofRefs,
		OwnershipMarker:     state.OwnershipMarker,
		OwnershipVerified:   result.OwnershipMarker == state.OwnershipMarker,
		IdempotencyVerified: result.IdempotencyVerified,
		Evidence:            evidence,
		Detail:              result.Reason,
	}
}
