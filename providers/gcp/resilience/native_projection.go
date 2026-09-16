package resilience

import (
	"context"
	"errors"
	"strings"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	cloudresilience "github.com/magelift/magelift/internal/shared/resilience"
	"github.com/magelift/magelift/sdk"
)

func (api *NativeAPI) startProjectionClass(ctx context.Context, state cloudrecovery.OperationState, _ string) (provider.NativeOperationObservation, error) {
	operation, err := api.projectionOperation(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if strings.TrimSpace(state.Target) == "" {
		state.Target = projectionTarget(state)
	}
	operationID, err := cloudrecovery.EncodeOperationID(operationPrefix, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	result, err := operation.Execute(ctx, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if strings.TrimSpace(result.OperationID) != "" {
		state.OperationRef = result.OperationID
		operationID, err = cloudrecovery.EncodeOperationID(operationPrefix, state)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
	}
	return operation.Observation(state, operationID, result), nil
}

func (api *NativeAPI) pollProjectionClass(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	operation, err := api.projectionOperation(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if strings.TrimSpace(state.Target) == "" {
		state.Target = state.Resource
	}
	result, err := operation.Execute(ctx, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if strings.TrimSpace(state.OperationRef) != "" && strings.TrimSpace(result.OperationID) != "" && result.OperationID != state.OperationRef {
		return provider.NativeOperationObservation{}, errors.New("GCP projection poll returned a different native operation identity")
	}
	return operation.Observation(state, operationID, result), nil
}

func (api *NativeAPI) projectionOperation(state cloudrecovery.OperationState) (*cloudresilience.ProjectionOperation, error) {
	if api == nil || api.config.Projection == nil {
		return nil, capabilityError(state, "GCP projection recovery requires an injected GKE/workload projection adapter")
	}
	return cloudresilience.NewProjectionOperation(sdk.ProviderID("gcp"), api.config.Projection)
}

func projectionTarget(state cloudrecovery.OperationState) string {
	return "gcp-projection://" + state.DataClass + "/" + shortDigest(state.OwnershipMarker+"\x00"+state.IdempotencyKey+"\x00"+state.Resource)
}
