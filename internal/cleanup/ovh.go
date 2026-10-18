package cleanup

import (
	"context"

	"github.com/magelift/magelift/internal/cloud/ovh/resilience"
	"github.com/magelift/magelift/sdk"
)

// OVHDatabaseProvider inventories and deletes ledger-claimed Managed Database
// services through the provider-local DatabaseAPI. Native OVH SDK models stay
// inside the resilience package.
type OVHDatabaseProvider struct {
	Database resilience.DatabaseAPI
	Engine   string
	Marker   string
}

func (provider OVHDatabaseProvider) Inventory(ctx context.Context, request sdk.CleanupInventoryRequest) ([]sdk.CleanupInventoryResource, error) {
	marker := provider.Marker
	if request.Marker != "" {
		marker = request.Marker
	}
	return resilience.InventoryOwnedDatabaseForLedger(ctx, provider.Database, provider.Engine, marker)
}

func (provider OVHDatabaseProvider) Delete(ctx context.Context, resource sdk.CleanupResource) error {
	return resilience.DeleteOwnedDatabaseForLedger(ctx, provider.Database, provider.Engine, provider.Marker, resource)
}
