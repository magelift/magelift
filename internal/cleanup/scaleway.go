package cleanup

import (
	"context"
	"fmt"

	"github.com/magelift/magelift/internal/cloud/scaleway/resilience"
	"github.com/magelift/magelift/sdk"
)

// ScalewayDatabaseProvider inventories and deletes ledger-claimed Managed
// Database resources through the provider-local DatabaseAPI. Native SDK types
// stay inside the Scaleway package.
type ScalewayDatabaseProvider struct {
	Database resilience.DatabaseAPI
	Marker   string
}

func (provider ScalewayDatabaseProvider) Inventory(ctx context.Context, request sdk.CleanupInventoryRequest) ([]sdk.CleanupInventoryResource, error) {
	marker := provider.Marker
	if request.Marker != "" {
		marker = request.Marker
	}
	return resilience.InventoryOwnedDatabaseForLedger(ctx, provider.Database, marker)
}

func (provider ScalewayDatabaseProvider) Delete(ctx context.Context, resource sdk.CleanupResource) error {
	return resilience.DeleteOwnedDatabaseForLedger(ctx, provider.Database, provider.Marker, resource)
}

// UnsupportedProvider is returned when a ledger names a provider that has no
// registered restartable cleanup adapter yet.
func UnsupportedProvider(provider string) error {
	return fmt.Errorf("restartable cleanup has no adapter for provider %q", provider)
}
