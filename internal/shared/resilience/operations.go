package resilience

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

// NativeOperationRequest is the narrow request a provider package passes to
// its official-SDK translator. Semantic lifecycle fields remain available for
// the translator, while Operation is the provider-owned API operation name.
// No provider SDK request or credential value crosses this boundary.
type NativeOperationRequest struct {
	Provider  sdk.ProviderID
	Operation string
	Request   sdk.ResilienceOperationRequest
}

// NativeOperationAPI is implemented by a provider package around its
// official SDK. The implementation owns request construction and translates
// SDK responses into provider.NativeOperationObservation before returning.
type NativeOperationAPI interface {
	Start(context.Context, NativeOperationRequest) (provider.NativeOperationObservation, error)
	Poll(context.Context, string) (provider.NativeOperationObservation, error)
	Inventory(context.Context, string) ([]provider.InventoryResource, error)
}

// OperationNameFunc maps a semantic recovery request to the provider's
// operation family. It is intentionally provider-owned because equivalent
// actions have different API ownership and safety semantics across clouds.
type OperationNameFunc func(sdk.ResilienceOperationRequest) (string, error)

type mappedOperationBackend struct {
	provider sdk.ProviderID
	api      NativeOperationAPI
	name     OperationNameFunc
}

var _ provider.NativeOperationBackend = (*mappedOperationBackend)(nil)

// NewMappedOperationBackend creates the reusable adapter used by first-party
// provider packages. The provider-specific API remains injected, so tests can
// use deterministic fakes and community implementations can wrap their own
// official SDK client without importing core lifecycle code.
func NewMappedOperationBackend(providerID sdk.ProviderID, api NativeOperationAPI, name OperationNameFunc) (provider.NativeOperationBackend, error) {
	if strings.TrimSpace(string(providerID)) == "" {
		return nil, errors.New("native operation provider is required")
	}
	if api == nil {
		return nil, errors.New("native operation API is required")
	}
	if name == nil {
		return nil, errors.New("native operation name mapper is required")
	}
	return &mappedOperationBackend{provider: providerID, api: api, name: name}, nil
}

func (backend *mappedOperationBackend) Start(ctx context.Context, request sdk.ResilienceOperationRequest) (provider.NativeOperationObservation, error) {
	if ctx == nil {
		return provider.NativeOperationObservation{}, errors.New("native operation context is required")
	}
	if err := ctx.Err(); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	operation, err := backend.name(request)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("map %s resilience operation: %w", backend.provider, err)
	}
	if err := validateOperationName(operation); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return backend.api.Start(ctx, NativeOperationRequest{Provider: backend.provider, Operation: operation, Request: cloneOperationRequest(request)})
}

func (backend *mappedOperationBackend) Poll(ctx context.Context, operationID string) (provider.NativeOperationObservation, error) {
	if ctx == nil {
		return provider.NativeOperationObservation{}, errors.New("native operation context is required")
	}
	if err := ctx.Err(); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if err := validateOpaqueOperationIdentity(operationID); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return backend.api.Poll(ctx, operationID)
}

func (backend *mappedOperationBackend) Inventory(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	if ctx == nil {
		return nil, errors.New("native operation context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateOpaqueOperationIdentity(marker); err != nil {
		return nil, err
	}
	return backend.api.Inventory(ctx, marker)
}

func cloneOperationRequest(request sdk.ResilienceOperationRequest) sdk.ResilienceOperationRequest {
	clone := request
	clone.DataClasses = append([]string(nil), request.DataClasses...)
	clone.ResourceReferences = cloneStringMap(request.ResourceReferences)
	clone.BackupReferences = cloneStringMap(request.BackupReferences)
	return clone
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func validateOperationName(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("native operation name must be a non-empty single-line identity")
	}
	return nil
}

func validateOpaqueOperationIdentity(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("native operation identity must be a non-empty single-line value")
	}
	return nil
}

// OperationName joins data classes deterministically for provider APIs whose
// recovery endpoint is a single workflow command. Provider packages can use
// it as the final part of their own operation family mapping.
func OperationName(prefix string, action sdk.ResilienceAction, dataClasses []string) (string, error) {
	if err := validateOperationName(prefix); err != nil {
		return "", err
	}
	if strings.TrimSpace(string(action)) == "" {
		return "", errors.New("native operation action is required")
	}
	classes := append([]string(nil), dataClasses...)
	sort.Strings(classes)
	if len(classes) == 0 {
		return "", errors.New("native operation requires at least one data class")
	}
	for _, dataClass := range classes {
		if err := validateOperationName(dataClass); err != nil {
			return "", fmt.Errorf("native operation data class: %w", err)
		}
	}
	return prefix + "." + string(action) + "." + strings.Join(classes, "+"), nil
}
