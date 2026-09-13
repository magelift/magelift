package resilience

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	cleanupKindRDBInstance = "rdb-instance"
	cleanupKindRDBSnapshot = "rdb-snapshot"
)

// Database returns the injected Managed Database port. It is used by the
// restartable cleanup ledger so source instances claimed before create can be
// inventoried and deleted independently of recovery-output cleanup.
func (api *NativeAPI) Database() DatabaseAPI {
	if api == nil {
		return nil
	}
	return api.database
}

// InventoryOwnedDatabaseForLedger lists marker-owned source and restore
// instances plus snapshots of those instances. Recovery-output-only cleanup
// remains a separate, stricter path.
func InventoryOwnedDatabaseForLedger(ctx context.Context, database DatabaseAPI, marker string) ([]sdk.CleanupInventoryResource, error) {
	if ctx == nil {
		return nil, errors.New("cleanup inventory context is required")
	}
	if database == nil {
		return nil, errors.New("Scaleway Managed Database API is required")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("cleanup inventory ownership marker is required and must be single-line")
	}
	instances, err := database.ListInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Scaleway Managed Database instances for cleanup ledger: %w", err)
	}
	ownedIDs := make(map[string]DatabaseInstance)
	resources := make([]sdk.CleanupInventoryResource, 0)
	for _, instance := range instances {
		if !scalewayDatabaseTagsMatch(instance.Tags, marker, "database") {
			continue
		}
		role := sdk.CleanupRoleSource
		if strings.HasPrefix(instance.Name, scalewayDatabaseRestorePrefix) {
			role = sdk.CleanupRoleRestore
		}
		ownedIDs[instance.ID] = instance
		resources = append(resources, sdk.CleanupInventoryResource{
			Kind:     cleanupKindRDBInstance,
			Role:     role,
			Name:     instance.Name,
			Identity: instance.ID,
			Owned:    true,
			Live:     true,
		})
	}
	snapshots, err := database.ListSnapshots(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Scaleway Managed Database snapshots for cleanup ledger: %w", err)
	}
	for _, snapshot := range snapshots {
		_, sourceOwned := ownedIDs[snapshot.InstanceID]
		if !sourceOwned && !scalewayDatabaseSnapshotOwned(snapshot, marker) {
			continue
		}
		resources = append(resources, sdk.CleanupInventoryResource{
			Kind:     cleanupKindRDBSnapshot,
			Role:     sdk.CleanupRoleSnapshot,
			Name:     snapshot.Name,
			Identity: snapshot.ID,
			Owned:    true,
			Live:     true,
		})
	}
	return resources, nil
}

// DeleteOwnedDatabaseForLedger deletes one ledger-claimed resource after a
// direct ownership check. Source instances are eligible; unmarked resources
// are refused.
func DeleteOwnedDatabaseForLedger(ctx context.Context, database DatabaseAPI, marker string, resource sdk.CleanupResource) error {
	if ctx == nil {
		return errors.New("cleanup delete context is required")
	}
	if database == nil {
		return errors.New("Scaleway Managed Database API is required")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return errors.New("cleanup delete ownership marker is required and must be single-line")
	}
	switch resource.Kind {
	case cleanupKindRDBInstance:
		return deleteOwnedDatabaseInstanceForLedger(ctx, database, marker, resource)
	case cleanupKindRDBSnapshot:
		return deleteOwnedDatabaseSnapshotForLedger(ctx, database, marker, resource)
	default:
		return fmt.Errorf("unsupported Scaleway cleanup kind %q", resource.Kind)
	}
}

func deleteOwnedDatabaseInstanceForLedger(ctx context.Context, database DatabaseAPI, marker string, resource sdk.CleanupResource) error {
	id := strings.TrimSpace(resource.Identity)
	if id == "" {
		id = strings.TrimSpace(resource.Name)
	}
	if id == "" {
		return errors.New("Scaleway Managed Database cleanup identity is required")
	}
	instance, err := database.GetInstance(ctx, id)
	if err != nil {
		if isScalewayNotFound(err) {
			return nil
		}
		return fmt.Errorf("inspect Scaleway Managed Database instance %q for cleanup: %w", id, err)
	}
	if !scalewayDatabaseTagsMatch(instance.Tags, marker, "database") {
		return fmt.Errorf("refusing to delete Scaleway Managed Database instance %q without the exact ownership marker", instance.ID)
	}
	if err := database.DeleteInstance(ctx, instance.ID); err != nil && !isScalewayNotFound(err) {
		return fmt.Errorf("delete owned Scaleway Managed Database instance %q: %w", instance.ID, err)
	}
	if err := waitForScalewayDatabaseInstanceGone(ctx, database, instance.ID); err != nil {
		return fmt.Errorf("verify deletion of Scaleway Managed Database instance %q: %w", instance.ID, err)
	}
	return nil
}

func deleteOwnedDatabaseSnapshotForLedger(ctx context.Context, database DatabaseAPI, marker string, resource sdk.CleanupResource) error {
	id := strings.TrimSpace(resource.Identity)
	if id == "" {
		return errors.New("Scaleway Managed Database snapshot identity is required")
	}
	snapshot, err := database.GetSnapshot(ctx, id)
	if err != nil {
		if isScalewayNotFound(err) {
			return nil
		}
		return fmt.Errorf("inspect Scaleway Managed Database snapshot %q for cleanup: %w", id, err)
	}
	instances, err := database.ListInstances(ctx)
	if err != nil {
		return fmt.Errorf("list Scaleway Managed Database instances for snapshot cleanup: %w", err)
	}
	sourceOwned := false
	for _, instance := range instances {
		if instance.ID == snapshot.InstanceID && scalewayDatabaseTagsMatch(instance.Tags, marker, "database") {
			sourceOwned = true
			break
		}
	}
	if !sourceOwned && !scalewayDatabaseSnapshotOwned(snapshot, marker) {
		return fmt.Errorf("refusing to delete Scaleway Managed Database snapshot %q without source ownership", snapshot.ID)
	}
	if err := database.DeleteSnapshot(ctx, snapshot.ID); err != nil && !isScalewayNotFound(err) {
		return fmt.Errorf("delete owned Scaleway Managed Database snapshot %q: %w", snapshot.ID, err)
	}
	if err := waitForScalewayDatabaseSnapshotGone(ctx, database, snapshot.ID); err != nil {
		return fmt.Errorf("verify deletion of Scaleway Managed Database snapshot %q: %w", snapshot.ID, err)
	}
	return nil
}
