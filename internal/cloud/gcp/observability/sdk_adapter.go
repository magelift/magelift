package observability

import (
	"context"
	"errors"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/internal/platform"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// NewLifecycleAdapter exposes the Google Cloud Operations translator through
// the provider-neutral SDK lifecycle.
func NewLifecycleAdapter(client providerobservability.LifecycleClient) (sdk.ObservabilityAdapter, error) {
	if client == nil {
		return nil, errors.New("GCP observability lifecycle client is required")
	}
	return providerobservability.NewSDKAdapter(sdk.ProviderID("gcp"), providerobservability.New(), client), nil
}

// NewCombinedLifecycleAdapter composes Google Cloud Operations with an
// external OTLP destination while keeping native operational objects owned by
// Google Cloud.
func NewCombinedLifecycleAdapter(native, external providerobservability.LifecycleClient) (sdk.ObservabilityAdapter, error) {
	return providerobservability.NewCompositeSDKAdapter(sdk.ProviderID("gcp"), map[string]providerobservability.LifecycleClient{"google-cloud-operations": native, "newrelic": external}, "google-cloud-operations")
}

// NewFromFactory validates the GCP identity before constructing the official
// Google SDK-backed lifecycle client through the injected factory.
func NewFromFactory(ctx context.Context, factory provider.ObservabilityClientFactory, request provider.ClientRequest) (sdk.ObservabilityAdapter, error) {
	if err := provider.ValidateClientRequestForProvider(request, sdk.ProviderID("gcp")); err != nil {
		return nil, err
	}
	client, err := provider.ConstructObservabilityClient(ctx, factory, request)
	if err != nil {
		return nil, err
	}
	return client, nil
}

// NewLifecycleFactories adapts an injected GCP client to the planned module
// seam without exposing Google API types to the platform package.
func NewLifecycleFactories(client providerobservability.LifecycleClient) platform.LifecycleFactories {
	return platform.LifecycleFactories{Observability: func(context.Context, platform.PlannedStack) (sdk.ObservabilityAdapter, error) {
		return NewLifecycleAdapter(client)
	}}
}
