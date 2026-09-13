package resilience

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sdk "github.com/magelift/magelift/sdk/v1"
)

const cleanupKindDatabaseInstance = "database-instance"

// Database returns the injected Managed Database port used by restartable
// cleanup reconciliation. It does not expose provider SDK response models.
func (api *NativeAPI) Database() DatabaseAPI {
	if api == nil {
		return nil
	}
	return api.database
}

// InventoryOwnedDatabaseForLedger lists only OVH Managed Database services
// whose description proves ownership by the exact source marker or the
// deterministic restore description derived from that marker.
func InventoryOwnedDatabaseForLedger(ctx context.Context, database DatabaseAPI, engine, marker string) ([]sdk.CleanupInventoryResource, error) {
	if ctx == nil {
		return nil, errors.New("cleanup inventory context is required")
	}
	if database == nil {
		return nil, errors.New("OVHcloud Managed Database API is required")
	}
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		return nil, errors.New("OVHcloud Managed Database cleanup engine is required")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("cleanup inventory ownership marker is required and must be single-line")
	}
	instances, err := database.ListInstances(ctx, engine)
	if err != nil {
		return nil, fmt.Errorf("list OVHcloud Managed Database instances for cleanup ledger: %w", err)
	}
	resources := make([]sdk.CleanupInventoryResource, 0)
	for _, instance := range instances {
		role, owned := ovhDatabaseLedgerOwnership(instance, marker)
		if !owned {
			continue
		}
		if strings.TrimSpace(instance.ID) == "" {
			return nil, errors.New("refusing to inventory an OVHcloud Managed Database service without an identity")
		}
		resources = append(resources, sdk.CleanupInventoryResource{
			Kind: cleanupKindDatabaseInstance, Role: role, Name: instance.Description,
			Identity: instance.ID, Owned: true, Live: true,
		})
	}
	return resources, nil
}

// DeleteOwnedDatabaseForLedger deletes one provider-owned Managed Database
// service after rechecking the exact source or restore description. The source
// marker is eligible because interrupted source creation is part of the
// wrapper's durable cleanup claim; foreign descriptions are never deleted.
func DeleteOwnedDatabaseForLedger(ctx context.Context, database DatabaseAPI, engine, marker string, resource sdk.CleanupResource) error {
	if ctx == nil {
		return errors.New("cleanup delete context is required")
	}
	if database == nil {
		return errors.New("OVHcloud Managed Database API is required")
	}
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		return errors.New("OVHcloud Managed Database cleanup engine is required")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return errors.New("cleanup delete ownership marker is required and must be single-line")
	}
	if resource.Kind != cleanupKindDatabaseInstance {
		return fmt.Errorf("unsupported OVHcloud cleanup kind %q", resource.Kind)
	}
	id := strings.TrimSpace(resource.Identity)
	if id == "" {
		return errors.New("OVHcloud Managed Database cleanup identity is required")
	}
	instance, err := database.GetInstance(ctx, engine, id)
	if err != nil {
		if isOVHNotFound(err) {
			return nil
		}
		return fmt.Errorf("inspect OVHcloud Managed Database instance %q for cleanup: %w", id, err)
	}
	if _, owned := ovhDatabaseLedgerOwnership(instance, marker); !owned {
		return fmt.Errorf("refusing to delete OVHcloud Managed Database instance %q without the exact ownership marker", instance.ID)
	}
	if err := database.DeleteInstance(ctx, engine, instance.ID); err != nil && !isOVHNotFound(err) {
		return fmt.Errorf("delete owned OVHcloud Managed Database instance %q: %w", instance.ID, err)
	}
	if err := waitForOVHDatabaseGone(ctx, database, engine, instance.ID, time.Second); err != nil {
		return fmt.Errorf("verify deletion of OVHcloud Managed Database instance %q: %w", instance.ID, err)
	}
	return nil
}

func ovhDatabaseLedgerOwnership(instance DatabaseInstance, marker string) (string, bool) {
	if instance.Description == marker {
		return sdk.CleanupRoleSource, true
	}
	if isOVHDatabaseRestoreDescriptionOwned(instance.Description, marker) {
		return sdk.CleanupRoleRestore, true
	}
	return "", false
}
