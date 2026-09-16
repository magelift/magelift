package observability

import (
	"context"
	"errors"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/internal/platform"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

// NewLifecycleAdapter exposes the OVHcloud MKS audit-subscription translator
// through the provider-neutral SDK lifecycle.
func NewLifecycleAdapter(client providerobservability.LifecycleClient) (sdk.ObservabilityAdapter, error) {
	if client == nil {
		return nil, errors.New("OVH observability lifecycle client is required")
	}
	return providerobservability.NewSDKAdapter(sdk.ProviderID("ovh"), providerobservability.New(), client), nil
}

// NewCombinedLifecycleAdapter composes OVHcloud audit forwarding with an
// external OTLP destination while keeping native subscription operations in
// the OVH adapter.
func NewCombinedLifecycleAdapter(native, external providerobservability.LifecycleClient) (sdk.ObservabilityAdapter, error) {
	return providerobservability.NewCompositeSDKAdapter(sdk.ProviderID("ovh"), map[string]providerobservability.LifecycleClient{"ovh-logs-data-platform": native, "newrelic": external}, "ovh-logs-data-platform")
}

// NewFromFactory validates the OVHcloud identity before constructing a
// provider-owned Logs Data Platform client through the generic factory.
func NewFromFactory(ctx context.Context, factory provider.ObservabilityClientFactory, request provider.ClientRequest) (sdk.ObservabilityAdapter, error) {
	if err := provider.ValidateClientRequestForProvider(request, sdk.ProviderID("ovh")); err != nil {
		return nil, err
	}
	client, err := provider.ConstructObservabilityClient(ctx, factory, request)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// NewLifecycleFactories adapts an injected OVHcloud client to the planned
// module seam without leaking OVH API models into the core.
func NewLifecycleFactories(client providerobservability.LifecycleClient) platform.LifecycleFactories {
	return platform.LifecycleFactories{Observability: func(context.Context, platform.PlannedStack) (sdk.ObservabilityAdapter, error) {
		return NewLifecycleAdapter(client)
	}}
}
