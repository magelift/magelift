package observability

import (
	"errors"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
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
