package resilience

import (
	"context"
	"errors"
	"strings"

	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func (api *NativeAPI) startProjectionClass(ctx context.Context, state operationState, _ string) (provider.NativeOperationObservation, error) {
	operation, err := api.projectionOperation(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if strings.TrimSpace(state.Target) == "" {
		state.Target = projectionTarget(state)
	}
	operationID, err := cloudrecovery.EncodeOperationID(ovhRecoveryOperationPrefix, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	result, err := operation.Execute(ctx, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if strings.TrimSpace(result.OperationID) != "" {
		state.OperationRef = result.OperationID
		operationID, err = cloudrecovery.EncodeOperationID(ovhRecoveryOperationPrefix, state)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
	}
	return operation.Observation(state, operationID, result), nil
}

func (api *NativeAPI) pollProjectionClass(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
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
		return provider.NativeOperationObservation{}, errors.New("OVHcloud projection poll returned a different native operation identity")
	}
	return operation.Observation(state, operationID, result), nil
}

func (api *NativeAPI) projectionOperation(state operationState) (*cloudresilience.ProjectionOperation, error) {
	if api == nil || api.config.Projection == nil {
		return nil, ovhCapabilityError(state, "OVHcloud projection recovery requires an injected MKS/workload projection adapter")
	}
	return cloudresilience.NewProjectionOperation(sdk.ProviderID("ovh"), api.config.Projection)
}

func projectionTarget(state operationState) string {
	return "ovh-projection://" + state.DataClass + "/" + shortDigest(state.OwnershipMarker+"\x00"+state.IdempotencyKey+"\x00"+state.Resource)
}
