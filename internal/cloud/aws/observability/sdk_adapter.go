package observability

import (
	"context"
	"errors"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/internal/platform"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// NewLifecycleAdapter exposes the AWS CloudWatch translator through the
// provider-neutral SDK lifecycle. The shared planner and evidence rules stay
// in internal/external/observability.
func NewLifecycleAdapter(client providerobservability.LifecycleClient) (sdk.ObservabilityAdapter, error) {
	if client == nil {
		return nil, errors.New("AWS observability lifecycle client is required")
	}
	return providerobservability.NewSDKAdapter(sdk.ProviderID("aws"), providerobservability.New(), client), nil
}

// NewCombinedLifecycleAdapter composes CloudWatch with an external OTLP
// destination while keeping CloudWatch as the sole owner of native alerts,
// dashboards, and SLO-like operational objects.
func NewCombinedLifecycleAdapter(native, external providerobservability.LifecycleClient) (sdk.ObservabilityAdapter, error) {
	return providerobservability.NewCompositeSDKAdapter(sdk.ProviderID("aws"), map[string]providerobservability.LifecycleClient{"cloudwatch": native, "newrelic": external}, "cloudwatch")
}

// NewFromFactory validates the AWS identity before asking an injected
// provider factory to construct the SDK-backed lifecycle client.
func NewFromFactory(ctx context.Context, factory provider.ObservabilityClientFactory, request provider.ClientRequest) (sdk.ObservabilityAdapter, error) {
	if err := provider.ValidateClientRequestForProvider(request, sdk.ProviderID("aws")); err != nil {
		return nil, err
	}
	client, err := provider.ConstructObservabilityClient(ctx, factory, request)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// NewLifecycleFactories adapts one injected AWS client to the planned module
// seam. Client construction remains outside the core and can use either the
// official AWS SDK config or a community implementation.
func NewLifecycleFactories(client providerobservability.LifecycleClient) platform.LifecycleFactories {
	return platform.LifecycleFactories{Observability: func(context.Context, platform.PlannedStack) (sdk.ObservabilityAdapter, error) {
		return NewLifecycleAdapter(client)
	}}
}
