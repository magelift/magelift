package edge

import (
	"context"
	"errors"

	composedge "github.com/magelift/magelift/internal/external/edge"
	"github.com/magelift/magelift/internal/platform"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

func NewLifecycleAdapter(client sdk.EdgeAdapter) (sdk.EdgeAdapter, error) {
	if err := composedge.ValidateAdapterProvider(client, sdk.ProviderID("scaleway")); err != nil {
		return nil, err
	}
	return client, nil
}

// NewNativeLifecycleAdapter supplies the reusable SDK lifecycle around a
// source-dated Scaleway Load Balancer or Edge Services API translator. A
// provider implementation may return a typed unavailable result where the
// selected product has no lifecycle support.
func NewNativeLifecycleAdapter(api sdk.EdgeOperationAPI, policy sdk.EdgeOperationPolicy) (sdk.EdgeAdapter, error) {
	return sdk.NewOperationBackedEdgeAdapter(sdk.EdgeAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: "scaleway.edge.native", Provider: sdk.ProviderID("scaleway"), Version: "1.0.0",
		Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy},
	}, api, policy)
}

// NewNativeSDKLifecycleAdapter wires the shared lifecycle to the official
// Scaleway Edge Services SDK. The SDK client and origin-health probe remain
// provider-owned; the returned adapter exposes only the public edge port.
func NewNativeSDKLifecycleAdapter(ctx context.Context, project, region string, health OriginHealthProbe, policy sdk.EdgeOperationPolicy, opts ...scw.ClientOption) (sdk.EdgeAdapter, error) {
	api, err := NewScalewayEdgeServicesSDKClient(ctx, project, region, health, opts...)
	if err != nil {
		return nil, err
	}
	return NewNativeLifecycleAdapter(api, policy)
}

func NewCombinedLifecycleAdapter(native, external sdk.EdgeAdapter) (sdk.EdgeAdapter, error) {
	if err := composedge.ValidateAdapterProvider(native, sdk.ProviderID("scaleway")); err != nil {
		return nil, err
	}
	if err := composedge.ValidateAdapterProvider(external, ""); err != nil {
		return nil, err
	}
	return composedge.NewCompositeAdapter(sdk.ProviderID("scaleway"), map[string]sdk.EdgeAdapter{
		"scaleway-edge-services":                   native,
		"scaleway-load-balancer":                   native,
		string(external.EdgeDescriptor().Provider): external,
	})
}

func NewFromFactory(ctx context.Context, factory provider.EdgeClientFactory, request provider.ClientRequest) (sdk.EdgeAdapter, error) {
	if err := provider.ValidateClientRequestForProvider(request, sdk.ProviderID("scaleway")); err != nil {
		return nil, err
	}
	client, err := provider.ConstructEdgeClient(ctx, factory, request)
	if err != nil {
		return nil, err
	}
	return NewLifecycleAdapter(client)
}

func NewLifecycleFactories(client sdk.EdgeAdapter) platform.LifecycleFactories {
	return platform.LifecycleFactories{Edge: func(context.Context, platform.PlannedStack) (sdk.EdgeAdapter, error) {
		if client == nil {
			return nil, errors.New("Scaleway edge SDK adapter is required")
		}
		return NewLifecycleAdapter(client)
	}}
}
