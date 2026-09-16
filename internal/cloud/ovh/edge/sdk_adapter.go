package edge

import (
	"context"
	"errors"

	composedge "github.com/magelift/magelift/internal/external/edge"
	"github.com/magelift/magelift/internal/platform"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
	"k8s.io/client-go/kubernetes"
)

func NewLifecycleAdapter(client sdk.EdgeAdapter) (sdk.EdgeAdapter, error) {
	if err := composedge.ValidateAdapterProvider(client, sdk.ProviderID("ovh")); err != nil {
		return nil, err
	}
	return client, nil
}

// NewNativeLifecycleAdapter supplies the reusable SDK lifecycle around an
// OVHcloud Load Balancer/CDN API translator. OVH SDK/API models stay in the
// injected implementation and unsupported product paths remain explicit.
func NewNativeLifecycleAdapter(api sdk.EdgeOperationAPI, policy sdk.EdgeOperationPolicy) (sdk.EdgeAdapter, error) {
	return sdk.NewOperationBackedEdgeAdapter(sdk.EdgeAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: "ovh.edge.native", Provider: sdk.ProviderID("ovh"), Version: "1.0.0",
		Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy},
	}, api, policy)
}

// NewKubernetesLifecycleAdapter constructs the OVH MKS Load Balancer adapter
// from the official client-go boundary. The shared SDK still owns planning,
// polling, ownership, and cleanup; this package owns the Octavia annotation
// and Kubernetes Service translation.
func NewKubernetesLifecycleAdapter(client kubernetes.Interface, health OriginHealthProbe, policy sdk.EdgeOperationPolicy) (sdk.EdgeAdapter, error) {
	api, err := NewKubernetesServiceAPI(client)
	if err != nil {
		return nil, err
	}
	native, err := NewNativeAPI(api, health)
	if err != nil {
		return nil, err
	}
	return NewNativeLifecycleAdapter(native, policy)
}

func NewCombinedLifecycleAdapter(native, external sdk.EdgeAdapter) (sdk.EdgeAdapter, error) {
	if err := composedge.ValidateAdapterProvider(native, sdk.ProviderID("ovh")); err != nil {
		return nil, err
	}
	if err := composedge.ValidateAdapterProvider(external, ""); err != nil {
		return nil, err
	}
	return composedge.NewCompositeAdapter(sdk.ProviderID("ovh"), map[string]sdk.EdgeAdapter{
		"ovh-cdn":                                  native,
		"ovh-public-cloud-load-balancer":           native,
		string(external.EdgeDescriptor().Provider): external,
	})
}

func NewFromFactory(ctx context.Context, factory provider.EdgeClientFactory, request provider.ClientRequest) (sdk.EdgeAdapter, error) {
	if err := provider.ValidateClientRequestForProvider(request, sdk.ProviderID("ovh")); err != nil {
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
			return nil, errors.New("OVH edge SDK adapter is required")
		}
		return NewLifecycleAdapter(client)
	}}
}
