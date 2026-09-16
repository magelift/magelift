package edge

import (
	"context"
	"errors"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"

	composedge "github.com/magelift/magelift/internal/external/edge"
	"github.com/magelift/magelift/sdk"
)

// NewLifecycleAdapter validates an injected GCP SDK edge adapter. The same
// seam can host Cloud CDN, Cloud Armor, or Google Cloud load-balancing clients.
func NewLifecycleAdapter(client sdk.EdgeAdapter) (sdk.EdgeAdapter, error) {
	if err := composedge.ValidateAdapterProvider(client, sdk.ProviderID("gcp")); err != nil {
		return nil, err
	}
	return client, nil
}

// NativeDescriptor is the static descriptor of the native GCP edge adapter.
// The protocol server advertises it without constructing API clients.
func NativeDescriptor() sdk.EdgeAdapterDescriptor {
	return sdk.EdgeAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: "gcp.edge.native", Provider: sdk.ProviderID("gcp"), Version: "1.0.0",
		Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeFailover, sdk.EdgeRollback, sdk.EdgeDestroy, sdk.EdgePurge},
	}
}

// NewNativeLifecycleAdapter supplies the reusable SDK lifecycle around a
// Google Cloud load-balancing/CDN/Armor API translator. Google SDK and GKE
// resource types remain owned by the injected provider implementation.
func NewNativeLifecycleAdapter(api sdk.EdgeOperationAPI, policy sdk.EdgeOperationPolicy) (sdk.EdgeAdapter, error) {
	return sdk.NewOperationBackedEdgeAdapter(NativeDescriptor(), api, policy)
}

// NewNativeSDKLifecycleAdapter wires the shared lifecycle to the official
// Google Compute API implementation. The runtime still owns the backend
// service and origin health probe; this constructor owns only the edge front
// end and its Cloud CDN/Armor preconditions.
func NewNativeSDKLifecycleAdapter(ctx context.Context, project string, health GoogleOriginHealthProbe, policy sdk.EdgeOperationPolicy, opts ...option.ClientOption) (sdk.EdgeAdapter, error) {
	if ctx == nil {
		return nil, errors.New("GCP edge context is required")
	}
	service, err := compute.NewService(ctx, append([]option.ClientOption{option.WithScopes(compute.CloudPlatformScope)}, opts...)...)
	if err != nil {
		return nil, err
	}
	api, err := newComputeGoogleEdgeAPI(service, project)
	if err != nil {
		return nil, err
	}
	native, err := NewNativeAPI(api, project, health)
	if err != nil {
		return nil, err
	}
	return NewNativeLifecycleAdapter(native, policy)
}

// NewCombinedLifecycleAdapter composes the selected native GCP edge with an
// independent external edge such as Fastly.
func NewCombinedLifecycleAdapter(native, external sdk.EdgeAdapter) (sdk.EdgeAdapter, error) {
	if err := composedge.ValidateAdapterProvider(native, sdk.ProviderID("gcp")); err != nil {
		return nil, err
	}
	if err := composedge.ValidateAdapterProvider(external, ""); err != nil {
		return nil, err
	}
	return composedge.NewCompositeAdapter(sdk.ProviderID("gcp"), map[string]sdk.EdgeAdapter{
		"cloud-cdn":                   native,
		"cloud-armor":                 native,
		"google-cloud-load-balancing": native,
		string(external.EdgeDescriptor().Provider): external,
	})
}
