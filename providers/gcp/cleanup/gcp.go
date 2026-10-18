package cleanup

import (
	"context"

	"github.com/magelift/magelift/providers/gcp/resilience"
	"github.com/magelift/magelift/sdk"
)

// GCPCloudSQLProvider inventories and deletes ledger-claimed Cloud SQL
// instances and backups through the provider-local CloudSQLAPI. Native
// sqladmin models stay inside the resilience package.
type GCPCloudSQLProvider struct {
	SQL     resilience.CloudSQLAPI
	Project string
	Marker  string
}

func (provider GCPCloudSQLProvider) Inventory(ctx context.Context, request sdk.CleanupInventoryRequest) ([]sdk.CleanupInventoryResource, error) {
	marker := provider.Marker
	if request.Marker != "" {
		marker = request.Marker
	}
	return resilience.InventoryOwnedCloudSQLForLedger(ctx, provider.SQL, provider.Project, marker)
}

func (provider GCPCloudSQLProvider) Delete(ctx context.Context, resource sdk.CleanupResource) error {
	return resilience.DeleteOwnedCloudSQLForLedger(ctx, provider.SQL, provider.Project, provider.Marker, resource)
}
