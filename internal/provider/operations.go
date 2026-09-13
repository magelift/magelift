package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// NativeOperationObservation is the small translation target used by a
// provider SDK wrapper. Cloud-specific operation states stay inside the
// provider implementation; only these normalized states cross the boundary.
type NativeOperationObservation struct {
	Status              string
	Action              sdk.ResilienceAction
	OperationID         string
	ResourceRefs        []string
	ProofRefs           []string
	OwnershipMarker     string
	OwnershipVerified   bool
	IdempotencyVerified bool
	Evidence            []sdk.ResilienceProofEvidence
	Detail              string
}

// NativeOperationBackend is implemented by a provider package around its
// official SDK. It deliberately returns provider-neutral value objects rather
// than SDK response models.
type NativeOperationBackend interface {
	Start(context.Context, sdk.ResilienceOperationRequest) (NativeOperationObservation, error)
	Poll(context.Context, string) (NativeOperationObservation, error)
	Inventory(context.Context, string) ([]InventoryResource, error)
}

// NormalizedOperationClient adapts a provider backend to the public SDK port.
// The SDK owns the bounded wait policy; this adapter owns status, identity,
// ownership, and inventory normalization exactly once for every provider.
type NormalizedOperationClient struct {
	backend NativeOperationBackend
}

var _ sdk.ResilienceOperationClient = (*NormalizedOperationClient)(nil)

func NewNormalizedOperationClient(backend NativeOperationBackend) (*NormalizedOperationClient, error) {
	if backend == nil {
		return nil, errors.New("native operation backend is required")
	}
	return &NormalizedOperationClient{backend: backend}, nil
}

func (client *NormalizedOperationClient) Start(ctx context.Context, request sdk.ResilienceOperationRequest) (sdk.ResilienceOperationObservation, error) {
	if err := validateOperationContext(ctx); err != nil {
		return sdk.ResilienceOperationObservation{}, err
	}
	if err := validateOperationRequest(request); err != nil {
		return sdk.ResilienceOperationObservation{}, err
	}
	observation, err := client.backend.Start(ctx, request)
	if err != nil {
		return sdk.ResilienceOperationObservation{}, fmt.Errorf("provider resilience operation start failed: %w", err)
	}
	normalized, err := normalizeOperationObservation(observation)
	if err != nil {
		return sdk.ResilienceOperationObservation{}, err
	}
	if normalized.OwnershipMarker != request.OwnershipMarker || !normalized.OwnershipVerified {
		return sdk.ResilienceOperationObservation{}, errors.New("provider resilience operation did not verify ownership scope")
	}
	if !normalized.IdempotencyVerified {
		return sdk.ResilienceOperationObservation{}, errors.New("provider resilience operation did not verify idempotency")
	}
	if normalized.Action != request.Action {
		return sdk.ResilienceOperationObservation{}, fmt.Errorf("provider resilience operation action %q does not match request %q", normalized.Action, request.Action)
	}
	return normalized, nil
}

func (client *NormalizedOperationClient) Poll(ctx context.Context, operationID string) (sdk.ResilienceOperationObservation, error) {
	if err := validateOperationContext(ctx); err != nil {
		return sdk.ResilienceOperationObservation{}, err
	}
	if err := validateOpaqueIdentity("provider resilience operation ID", operationID); err != nil {
		return sdk.ResilienceOperationObservation{}, err
	}
	observation, err := client.backend.Poll(ctx, operationID)
	if err != nil {
		return sdk.ResilienceOperationObservation{}, fmt.Errorf("provider resilience operation poll failed: %w", err)
	}
	normalized, err := normalizeOperationObservation(observation)
	if err != nil {
		return sdk.ResilienceOperationObservation{}, err
	}
	if normalized.OperationID != operationID {
		return sdk.ResilienceOperationObservation{}, fmt.Errorf("provider resilience operation poll returned identity %q, want %q", normalized.OperationID, operationID)
	}
	return normalized, nil
}

func (client *NormalizedOperationClient) Inventory(ctx context.Context, marker string) ([]sdk.ResilienceInventoryResource, error) {
	if err := validateOperationContext(ctx); err != nil {
		return nil, err
	}
	if err := validateOpaqueIdentity("provider resilience ownership marker", marker); err != nil {
		return nil, err
	}
	resources, err := client.backend.Inventory(ctx, marker)
	if err != nil {
		return nil, fmt.Errorf("provider resilience owning-service inventory failed: %w", err)
	}
	result := make([]sdk.ResilienceInventoryResource, 0, len(resources))
	seen := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		if err := validateOpaqueIdentity("provider inventory resource identity", resource.Identity); err != nil {
			return nil, err
		}
		if _, exists := seen[resource.Identity]; exists {
			return nil, fmt.Errorf("provider inventory returned duplicate resource %q", resource.Identity)
		}
		seen[resource.Identity] = struct{}{}
		result = append(result, sdk.ResilienceInventoryResource{Identity: resource.Identity, Owned: resource.Owned, Live: resource.Live})
	}
	return result, nil
}

func normalizeOperationObservation(observation NativeOperationObservation) (sdk.ResilienceOperationObservation, error) {
	status, err := normalizeOperationStatus(observation.Status)
	if err != nil {
		return sdk.ResilienceOperationObservation{}, err
	}
	if observation.OperationID == "" {
		if status != "succeeded" {
			return sdk.ResilienceOperationObservation{}, errors.New("provider resilience operation identity is required before completion")
		}
	} else if err := validateOpaqueIdentity("provider resilience operation ID", observation.OperationID); err != nil {
		return sdk.ResilienceOperationObservation{}, err
	}
	if err := validateOpaqueIdentity("provider resilience ownership marker", observation.OwnershipMarker); err != nil {
		return sdk.ResilienceOperationObservation{}, err
	}
	for _, reference := range append(append([]string(nil), observation.ResourceRefs...), observation.ProofRefs...) {
		if err := validateOpaqueIdentity("provider resilience reference", reference); err != nil {
			return sdk.ResilienceOperationObservation{}, err
		}
	}
	return sdk.ResilienceOperationObservation{
		Status: observationStatus(status), Action: observation.Action, OperationID: observation.OperationID,
		ResourceRefs: append([]string(nil), observation.ResourceRefs...), ProofRefs: append([]string(nil), observation.ProofRefs...),
		OwnershipMarker: observation.OwnershipMarker, OwnershipVerified: observation.OwnershipVerified,
		IdempotencyVerified: observation.IdempotencyVerified, Evidence: append([]sdk.ResilienceProofEvidence(nil), observation.Evidence...),
		Detail: safeOperationDetail(status, observation.Detail),
	}, nil
}

func normalizeOperationStatus(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending", "queued", "accepted":
		return "pending", nil
	case "running", "in_progress", "in-progress":
		return "running", nil
	case "succeeded", "success", "completed", "complete":
		return "succeeded", nil
	case "failed", "error", "cancelled", "canceled":
		return "failed", nil
	default:
		return "", fmt.Errorf("provider resilience operation returned unknown status %q", value)
	}
}

func observationStatus(value string) sdk.ResilienceOperationStatus {
	switch value {
	case "pending":
		return sdk.ResilienceOperationPending
	case "running":
		return sdk.ResilienceOperationRunning
	case "succeeded":
		return sdk.ResilienceOperationSucceeded
	default:
		return sdk.ResilienceOperationFailed
	}
}

func safeOperationDetail(status, detail string) string {
	if status == "failed" {
		return "provider resilience operation failed"
	}
	if strings.ContainsAny(detail, "\r\n\x00") {
		return ""
	}
	return detail
}

func validateOperationContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("provider resilience operation context is required")
	}
	return ctx.Err()
}

func validateOperationRequest(request sdk.ResilienceOperationRequest) error {
	if request.Action == "" || strings.TrimSpace(request.OwnershipMarker) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		return errors.New("provider resilience operation action, ownership marker, and idempotency key are required")
	}
	if err := validateOpaqueIdentity("provider resilience ownership marker", request.OwnershipMarker); err != nil {
		return err
	}
	if err := validateOpaqueIdentity("provider resilience idempotency key", request.IdempotencyKey); err != nil {
		return err
	}
	return nil
}

func validateOpaqueIdentity(name, value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s must be a non-empty single-line identity", name)
	}
	return nil
}
