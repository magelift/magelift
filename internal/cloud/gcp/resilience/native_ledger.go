package resilience

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	cleanupKindCloudSQLInstance = "cloudsql-instance"
	cleanupKindCloudSQLBackup   = "cloudsql-backup"
)

// SQL returns the injected Cloud SQL port used by restartable cleanup
// reconciliation. It does not expose sqladmin SDK models.
func (api *NativeAPI) SQL() CloudSQLAPI {
	if api == nil {
		return nil
	}
	return api.sql
}

// InventoryOwnedCloudSQLForLedger lists marker-owned Cloud SQL instances and
// backups that either name those instances or carry the Magelift recovery
// description prefix. Foreign instances are never returned.
func InventoryOwnedCloudSQLForLedger(ctx context.Context, sql CloudSQLAPI, project, marker string) ([]sdk.CleanupInventoryResource, error) {
	if ctx == nil {
		return nil, errors.New("cleanup inventory context is required")
	}
	if sql == nil {
		return nil, errors.New("GCP Cloud SQL API is required")
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, errors.New("GCP Cloud SQL cleanup project is required")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("cleanup inventory ownership marker is required and must be single-line")
	}
	instances, err := sql.ListInstances(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("list GCP Cloud SQL instances for cleanup ledger: %w", err)
	}
	ownedNames := make(map[string]CloudSQLInstance)
	resources := make([]sdk.CleanupInventoryResource, 0)
	for _, instance := range instances {
		if !cloudSQLInstanceOwnedForLedger(instance, marker) {
			continue
		}
		if strings.TrimSpace(instance.Name) == "" {
			return nil, errors.New("refusing to inventory a GCP Cloud SQL instance without a name")
		}
		ownedNames[instance.Name] = instance
		resources = append(resources, sdk.CleanupInventoryResource{
			Kind:      cleanupKindCloudSQLInstance,
			Role:      cloudSQLInstanceLedgerRole(instance),
			Name:      instance.Name,
			Identity:  instance.Name,
			Owned:     true,
			Live:      true,
			Protected: instance.DeletionProtection,
		})
	}
	backups, err := sql.ListBackups(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("list GCP Cloud SQL backups for cleanup ledger: %w", err)
	}
	prefix := cloudSQLOwnedBackupPrefix(marker)
	for _, backup := range backups {
		if strings.TrimSpace(backup.Name) == "" {
			continue
		}
		if !cloudSQLBackupOwnedForLedger(backup, ownedNames, prefix) {
			continue
		}
		resources = append(resources, sdk.CleanupInventoryResource{
			Kind:     cleanupKindCloudSQLBackup,
			Role:     sdk.CleanupRoleSnapshot,
			Name:     backup.Name,
			Identity: backup.Name,
			Owned:    true,
			Live:     true,
		})
	}
	return resources, nil
}

// DeleteOwnedCloudSQLForLedger deletes one ledger-claimed Cloud SQL resource
// after a direct ownership check. Deletion-protected instances are refused.
// Unmarked resources are never deleted.
func DeleteOwnedCloudSQLForLedger(ctx context.Context, sql CloudSQLAPI, project, marker string, resource sdk.CleanupResource) error {
	if ctx == nil {
		return errors.New("cleanup delete context is required")
	}
	if sql == nil {
		return errors.New("GCP Cloud SQL API is required")
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return errors.New("GCP Cloud SQL cleanup project is required")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return errors.New("cleanup delete ownership marker is required and must be single-line")
	}
	switch resource.Kind {
	case cleanupKindCloudSQLInstance:
		return deleteOwnedCloudSQLInstanceForLedger(ctx, sql, project, marker, resource)
	case cleanupKindCloudSQLBackup:
		return deleteOwnedCloudSQLBackupForLedger(ctx, sql, project, marker, resource)
	default:
		return fmt.Errorf("unsupported GCP Cloud SQL cleanup kind %q", resource.Kind)
	}
}

func deleteOwnedCloudSQLInstanceForLedger(ctx context.Context, sql CloudSQLAPI, project, marker string, resource sdk.CleanupResource) error {
	name := firstNonEmpty(resource.Identity, resource.Name)
	if name == "" {
		return errors.New("GCP Cloud SQL cleanup identity is required")
	}
	instance, err := sql.GetInstance(ctx, project, name)
	if err != nil {
		if isCloudSQLNotFound(err) {
			return nil
		}
		return fmt.Errorf("inspect GCP Cloud SQL instance %q for cleanup: %w", name, err)
	}
	if !cloudSQLInstanceOwnedForLedger(instance, marker) {
		return fmt.Errorf("refusing to delete GCP Cloud SQL instance %q without the exact ownership marker", instance.Name)
	}
	if instance.DeletionProtection {
		return fmt.Errorf("refusing to delete GCP Cloud SQL instance %q because deletion protection is enabled", instance.Name)
	}
	op, err := sql.DeleteInstance(ctx, project, instance.Name)
	if err != nil {
		if isCloudSQLNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete owned GCP Cloud SQL instance %q: %w", instance.Name, err)
	}
	if err := waitCloudSQLAPIOperation(ctx, sql, project, op); err != nil {
		return fmt.Errorf("wait for deletion of GCP Cloud SQL instance %q: %w", instance.Name, err)
	}
	if err := waitForCloudSQLInstanceGone(ctx, sql, project, instance.Name); err != nil {
		return fmt.Errorf("verify deletion of GCP Cloud SQL instance %q: %w", instance.Name, err)
	}
	return nil
}

func deleteOwnedCloudSQLBackupForLedger(ctx context.Context, sql CloudSQLAPI, project, marker string, resource sdk.CleanupResource) error {
	name := strings.TrimSpace(resource.Identity)
	if name == "" {
		return errors.New("GCP Cloud SQL backup identity is required")
	}
	backup, err := sql.GetBackup(ctx, name)
	if err != nil {
		if isCloudSQLNotFound(err) {
			return nil
		}
		return fmt.Errorf("inspect GCP Cloud SQL backup %q for cleanup: %w", name, err)
	}
	owned, err := cloudSQLBackupStillOwnedForLedger(ctx, sql, project, marker, backup)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("refusing to delete GCP Cloud SQL backup %q without source ownership", backup.Name)
	}
	op, err := sql.DeleteBackup(ctx, backup.Name)
	if err != nil {
		if isCloudSQLNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete owned GCP Cloud SQL backup %q: %w", backup.Name, err)
	}
	if err := waitCloudSQLAPIOperation(ctx, sql, project, op); err != nil {
		return fmt.Errorf("wait for deletion of GCP Cloud SQL backup %q: %w", backup.Name, err)
	}
	return nil
}

func cloudSQLInstanceOwnedForLedger(instance CloudSQLInstance, marker string) bool {
	return secretLabelsMatch(instance.UserLabels, marker, "database")
}

func cloudSQLInstanceLedgerRole(instance CloudSQLInstance) string {
	prefix := "magelift-recovery"
	if strings.HasPrefix(instance.Name, prefix) {
		return sdk.CleanupRoleRestore
	}
	return sdk.CleanupRoleSource
}

func cloudSQLOwnedBackupPrefix(marker string) string {
	return "magelift-recovery:" + shortDigest(marker) + ":"
}

func cloudSQLBackupOwnedForLedger(backup CloudSQLBackup, ownedInstances map[string]CloudSQLInstance, prefix string) bool {
	if strings.HasPrefix(backup.Description, prefix) {
		return true
	}
	for name := range ownedInstances {
		if cloudSQLBackupBelongsToInstance(backup, name) {
			return true
		}
	}
	return false
}

func cloudSQLBackupStillOwnedForLedger(ctx context.Context, sql CloudSQLAPI, project, marker string, backup CloudSQLBackup) (bool, error) {
	if strings.HasPrefix(backup.Description, cloudSQLOwnedBackupPrefix(marker)) {
		return true, nil
	}
	instanceName := strings.TrimSpace(backup.Instance)
	if strings.Contains(instanceName, "/instances/") {
		instanceName = instanceName[strings.LastIndex(instanceName, "/")+1:]
	}
	if instanceName == "" {
		return false, nil
	}
	instance, err := sql.GetInstance(ctx, project, instanceName)
	if err != nil {
		if isCloudSQLNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect GCP Cloud SQL instance %q for backup cleanup: %w", instanceName, err)
	}
	return cloudSQLInstanceOwnedForLedger(instance, marker), nil
}

func waitCloudSQLAPIOperation(ctx context.Context, sql CloudSQLAPI, project string, operation CloudSQLOperation) error {
	if strings.TrimSpace(operation.Name) == "" {
		if operation.Status == "succeeded" || operation.Status == "done" || operation.Status == "completed" {
			return nil
		}
		return errors.New("GCP Cloud SQL cleanup API returned no operation identity")
	}
	for {
		switch strings.ToLower(operation.Status) {
		case "succeeded", "done", "completed":
			return nil
		case "failed", "error":
			if operation.Error == "" {
				return errors.New("GCP Cloud SQL cleanup operation failed")
			}
			return errors.New(operation.Error)
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
		var err error
		operation, err = sql.GetOperation(ctx, project, operation.Name)
		if err != nil {
			return fmt.Errorf("poll GCP Cloud SQL cleanup operation: %w", err)
		}
	}
}

func waitForCloudSQLInstanceGone(ctx context.Context, sql CloudSQLAPI, project, name string) error {
	for {
		_, err := sql.GetInstance(ctx, project, name)
		if isCloudSQLNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
