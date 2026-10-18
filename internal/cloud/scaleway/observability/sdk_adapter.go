package observability

import (
	"context"
	"errors"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/internal/platform"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

// NewLifecycleAdapter exposes the Scaleway Cockpit translator through the
// provider-neutral SDK lifecycle. Unsupported Cockpit capabilities remain
// explicit in the shared plan and verification evidence.
func NewLifecycleAdapter(client providerobservability.LifecycleClient) (sdk.ObservabilityAdapter, error) {
	if client == nil {
		return nil, errors.New("Scaleway observability lifecycle client is required")
	}
	return providerobservability.NewSDKAdapter(sdk.ProviderID("scaleway"), providerobservability.New(), client), nil
}

// NewCombinedLifecycleAdapter composes Scaleway Cockpit with an external
// OTLP destination without pretending that Cockpit owns external objects.
func NewCombinedLifecycleAdapter(native, external providerobservability.LifecycleClient) (sdk.ObservabilityAdapter, error) {
	return providerobservability.NewCompositeSDKAdapter(sdk.ProviderID("scaleway"), map[string]providerobservability.LifecycleClient{"scaleway-cockpit": native, "newrelic": external}, "scaleway-cockpit")
}

// NewFromFactory validates the Scaleway identity before constructing a
// provider-owned Cockpit client through the generic factory contract.
func NewFromFactory(ctx context.Context, factory provider.ObservabilityClientFactory, request provider.ClientRequest) (sdk.ObservabilityAdapter, error) {
	if err := provider.ValidateClientRequestForProvider(request, sdk.ProviderID("scaleway")); err != nil {
		return nil, err
	}
	client, err := provider.ConstructObservabilityClient(ctx, factory, request)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// NewLifecycleFactories adapts an injected Scaleway client to the planned
// module seam while keeping Scaleway SDK types provider-local.
func NewLifecycleFactories(client providerobservability.LifecycleClient) platform.LifecycleFactories {
	return platform.LifecycleFactories{Observability: func(context.Context, platform.PlannedStack) (sdk.ObservabilityAdapter, error) {
		return NewLifecycleAdapter(client)
	}}
}
