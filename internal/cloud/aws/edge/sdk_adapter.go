package edge

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/config"
	composedge "github.com/magelift/magelift/internal/external/edge"
	"github.com/magelift/magelift/internal/platform"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

// NewLifecycleAdapter validates an injected AWS SDK edge adapter at the
// provider boundary. The adapter may be first-party or community-maintained;
// CloudFront/WAF API models never enter the core.
func NewLifecycleAdapter(client sdk.EdgeAdapter) (sdk.EdgeAdapter, error) {
	if err := composedge.ValidateAdapterProvider(client, sdk.ProviderID("aws")); err != nil {
		return nil, err
	}
	return client, nil
}

// NewNativeLifecycleAdapter supplies the reusable SDK lifecycle around an
// AWS-owned CloudFront/WAF API translator. The translator owns AWS SDK types;
// the public lifecycle owns planning, polling, ownership, and idempotency.
func NewNativeLifecycleAdapter(api sdk.EdgeOperationAPI, policy sdk.EdgeOperationPolicy) (sdk.EdgeAdapter, error) {
	return sdk.NewOperationBackedEdgeAdapter(sdk.EdgeAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: "aws.edge.native", Provider: sdk.ProviderID("aws"), Version: "1.0.0",
		Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeFailover, sdk.EdgeRollback, sdk.EdgeDestroy, sdk.EdgePurge},
	}, api, policy)
}

// NewNativeSDKLifecycleAdapter wires the shared lifecycle to the official
// AWS CloudFront SDK translator. AWS SDK options remain provider-owned.
func NewNativeSDKLifecycleAdapter(ctx context.Context, health OriginHealthProbe, policy sdk.EdgeOperationPolicy, opts ...func(*config.LoadOptions) error) (sdk.EdgeAdapter, error) {
	api, err := NewCloudFrontSDKClient(ctx, health, opts...)
	if err != nil {
		return nil, err
	}
	return NewNativeLifecycleAdapter(api, policy)
}

// NewCombinedLifecycleAdapter composes native CloudFront with an independent
// external edge such as Fastly. Reference routing is owned by the shared edge
// composition package.
func NewCombinedLifecycleAdapter(native, external sdk.EdgeAdapter) (sdk.EdgeAdapter, error) {
	if err := composedge.ValidateAdapterProvider(native, sdk.ProviderID("aws")); err != nil {
		return nil, err
	}
	if err := composedge.ValidateAdapterProvider(external, ""); err != nil {
		return nil, err
	}
	return composedge.NewCompositeAdapter(sdk.ProviderID("aws"), map[string]sdk.EdgeAdapter{
		"cloudfront":     native,
		"cloudfront-waf": native,
		string(external.EdgeDescriptor().Provider): external,
	})
}

// NewFromFactory validates the exact AWS target identity before asking the
// provider factory to construct its SDK edge adapter.
func NewFromFactory(ctx context.Context, factory provider.EdgeClientFactory, request provider.ClientRequest) (sdk.EdgeAdapter, error) {
	if err := provider.ValidateClientRequestForProvider(request, sdk.ProviderID("aws")); err != nil {
		return nil, err
	}
	client, err := provider.ConstructEdgeClient(ctx, factory, request)
	if err != nil {
		return nil, err
	}
	return NewLifecycleAdapter(client)
}

// NewLifecycleFactories connects an injected AWS edge SDK adapter to the
// planned module seam without constructing provider clients in the core.
func NewLifecycleFactories(client sdk.EdgeAdapter) platform.LifecycleFactories {
	return platform.LifecycleFactories{Edge: func(context.Context, platform.PlannedStack) (sdk.EdgeAdapter, error) {
		if client == nil {
			return nil, errors.New("AWS edge SDK adapter is required")
		}
		return NewLifecycleAdapter(client)
	}}
}
