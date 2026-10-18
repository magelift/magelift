package provider

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/magelift/magelift/sdk"
)

// ValidateClientRequestForProvider adds the provider identity check that the
// generic descriptor validator cannot infer from arbitrary community target
// IDs. Provider factories should call it before constructing SDK clients.
func ValidateClientRequestForProvider(request ClientRequest, expected sdk.ProviderID) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Provider != expected {
		return fmt.Errorf("provider client request targets %q, want %q", request.Provider, expected)
	}
	return nil
}

// ConstructResilienceClient validates the shared request before delegating to
// a provider factory. The factory is the only place allowed to construct a
// cloud SDK client; this helper never mutates provider state.
func ConstructResilienceClient(ctx context.Context, factory ResilienceClientFactory, request ClientRequest) (sdk.ResilienceOperationClient, error) {
	return construct(ctx, "resilience", factory, request, func() (sdk.ResilienceOperationClient, error) {
		return factory.NewResilienceClient(ctx, request)
	})
}

// ConstructEdgeClient validates and constructs a provider-owned edge adapter.
func ConstructEdgeClient(ctx context.Context, factory EdgeClientFactory, request ClientRequest) (sdk.EdgeAdapter, error) {
	return construct(ctx, "edge", factory, request, func() (sdk.EdgeAdapter, error) {
		return factory.NewEdgeClient(ctx, request)
	})
}

// ConstructObservabilityClient validates and constructs a provider-owned
// observability adapter.
func ConstructObservabilityClient(ctx context.Context, factory ObservabilityClientFactory, request ClientRequest) (sdk.ObservabilityAdapter, error) {
	return construct(ctx, "observability", factory, request, func() (sdk.ObservabilityAdapter, error) {
		return factory.NewObservabilityClient(ctx, request)
	})
}

// ConstructCredentialLifecycleClient validates and constructs a provider-owned
// credential lifecycle adapter without accepting a secret value.
func ConstructCredentialLifecycleClient(ctx context.Context, factory CredentialLifecycleClientFactory, request ClientRequest) (sdk.CredentialLifecycleAdapter, error) {
	return construct(ctx, "credential lifecycle", factory, request, func() (sdk.CredentialLifecycleAdapter, error) {
		return factory.NewCredentialLifecycleClient(ctx, request)
	})
}

// ConstructInventoryClient validates the generic request before constructing
// the owning-service inventory boundary.
func ConstructInventoryClient(ctx context.Context, factory InventoryClientFactory, request ClientRequest) (InventoryClient, error) {
	return construct(ctx, "owning-service inventory", factory, request, func() (InventoryClient, error) {
		return factory.NewInventoryClient(ctx, request)
	})
}

func construct[T any](ctx context.Context, kind string, factory any, request ClientRequest, create func() (T, error)) (T, error) {
	var zero T
	if ctx == nil {
		return zero, errors.New("provider client context is required")
	}
	if factory == nil {
		return zero, fmt.Errorf("provider %s factory is required", kind)
	}
	if err := request.Validate(); err != nil {
		return zero, fmt.Errorf("validate provider %s client request: %w", kind, err)
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	client, err := create()
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return zero, contextErr
		}
		return zero, fmt.Errorf("construct provider %s client: provider construction failed", kind)
	}
	if isNilValue(client) {
		return zero, fmt.Errorf("construct provider %s client: factory returned nil", kind)
	}
	return client, nil
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
