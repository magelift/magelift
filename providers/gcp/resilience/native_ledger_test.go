package resilience

import (
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestInventoryOwnedCloudSQLForLedgerIncludesSourceAndRefusesForeign(t *testing.T) {
	sql := newFakeCloudSQL()
	sql.instances["demo\x00orders"] = CloudSQLInstance{
		Project: "demo", Name: "orders", State: "RUNNABLE",
		UserLabels: map[string]string{ownershipLabelKey: "owner", classLabelKey: "database"},
	}
	sql.instances["demo\x00magelift-recovery-abcd"] = CloudSQLInstance{
		Project: "demo", Name: "magelift-recovery-abcd", State: "RUNNABLE",
		UserLabels: map[string]string{ownershipLabelKey: "owner", classLabelKey: "database"},
	}
	sql.instances["demo\x00foreign"] = CloudSQLInstance{
		Project: "demo", Name: "foreign", State: "RUNNABLE",
		UserLabels: map[string]string{ownershipLabelKey: "other", classLabelKey: "database"},
	}
	sql.backups["projects/demo/backups/owned"] = CloudSQLBackup{Name: "projects/demo/backups/owned", Instance: "orders", Description: "other"}
	sql.backups["projects/demo/backups/prefixed"] = CloudSQLBackup{
		Name: "projects/demo/backups/prefixed", Instance: "gone", Description: cloudSQLOwnedBackupPrefix("owner") + "fixture",
	}
	sql.backups["projects/demo/backups/foreign"] = CloudSQLBackup{Name: "projects/demo/backups/foreign", Instance: "foreign", Description: "nope"}

	resources, err := InventoryOwnedCloudSQLForLedger(context.Background(), sql, "demo", "owner")
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]sdk.CleanupInventoryResource, len(resources))
	for _, resource := range resources {
		seen[resource.Identity] = resource
	}
	if seen["orders"].Role != sdk.CleanupRoleSource || seen["magelift-recovery-abcd"].Role != sdk.CleanupRoleRestore {
		t.Fatalf("instance roles = %#v", seen)
	}
	if _, ok := seen["foreign"]; ok {
		t.Fatalf("foreign instance was inventoried: %#v", seen)
	}
	if _, ok := seen["projects/demo/backups/owned"]; !ok {
		t.Fatalf("owned-instance backup missing: %#v", seen)
	}
	if _, ok := seen["projects/demo/backups/prefixed"]; !ok {
		t.Fatalf("prefixed backup missing: %#v", seen)
	}
	if _, ok := seen["projects/demo/backups/foreign"]; ok {
		t.Fatalf("foreign backup was inventoried: %#v", seen)
	}
}

func TestDeleteOwnedCloudSQLForLedgerRechecksOwnership(t *testing.T) {
	sql := newFakeCloudSQL()
	sql.instances["demo\x00orders"] = CloudSQLInstance{
		Project: "demo", Name: "orders", State: "RUNNABLE",
		UserLabels: map[string]string{ownershipLabelKey: "owner", classLabelKey: "database"},
	}
	sql.instances["demo\x00foreign"] = CloudSQLInstance{
		Project: "demo", Name: "foreign", State: "RUNNABLE",
		UserLabels: map[string]string{ownershipLabelKey: "other", classLabelKey: "database"},
	}
	sql.instances["demo\x00protected"] = CloudSQLInstance{
		Project: "demo", Name: "protected", State: "RUNNABLE", DeletionProtection: true,
		UserLabels: map[string]string{ownershipLabelKey: "owner", classLabelKey: "database"},
	}
	sql.backups["projects/demo/backups/owned"] = CloudSQLBackup{Name: "projects/demo/backups/owned", Instance: "orders"}

	if err := DeleteOwnedCloudSQLForLedger(context.Background(), sql, "demo", "owner", sdk.CleanupResource{Kind: cleanupKindCloudSQLInstance, Identity: "foreign"}); err == nil || !strings.Contains(err.Error(), "without the exact ownership marker") {
		t.Fatalf("foreign delete error = %v", err)
	}
	if err := DeleteOwnedCloudSQLForLedger(context.Background(), sql, "demo", "owner", sdk.CleanupResource{Kind: cleanupKindCloudSQLInstance, Identity: "protected"}); err == nil || !strings.Contains(err.Error(), "deletion protection") {
		t.Fatalf("protected delete error = %v", err)
	}
	if err := DeleteOwnedCloudSQLForLedger(context.Background(), sql, "demo", "owner", sdk.CleanupResource{Kind: cleanupKindCloudSQLBackup, Identity: "projects/demo/backups/owned"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := sql.backups["projects/demo/backups/owned"]; ok {
		t.Fatal("owned backup remained")
	}
	if err := DeleteOwnedCloudSQLForLedger(context.Background(), sql, "demo", "owner", sdk.CleanupResource{Kind: cleanupKindCloudSQLInstance, Identity: "orders"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := sql.instances["demo\x00orders"]; ok {
		t.Fatal("owned instance remained")
	}
	if _, ok := sql.instances["demo\x00foreign"]; !ok {
		t.Fatal("foreign instance was deleted")
	}
}
