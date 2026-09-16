package sdk

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type cleanupInventoryStub struct {
	resources []CleanupInventoryResource
	err       error
	calls     int
}

func (stub *cleanupInventoryStub) Inventory(context.Context, CleanupInventoryRequest) ([]CleanupInventoryResource, error) {
	stub.calls++
	if stub.err != nil {
		return nil, stub.err
	}
	return append([]CleanupInventoryResource(nil), stub.resources...), nil
}

type cleanupDeleteStub struct {
	deleted []string
	err     error
}

func (stub *cleanupDeleteStub) Delete(_ context.Context, resource CleanupResource) error {
	if stub.err != nil {
		return stub.err
	}
	stub.deleted = append(stub.deleted, cleanupResourceKey(resource))
	return nil
}

type mutatingCleanupInventory struct {
	resources []CleanupInventoryResource
	deleted   []string
}

func (stub *mutatingCleanupInventory) Inventory(context.Context, CleanupInventoryRequest) ([]CleanupInventoryResource, error) {
	return append([]CleanupInventoryResource(nil), stub.resources...), nil
}

func (stub *mutatingCleanupInventory) Delete(_ context.Context, resource CleanupResource) error {
	stub.deleted = append(stub.deleted, cleanupResourceKey(resource))
	remaining := make([]CleanupInventoryResource, 0, len(stub.resources))
	for _, item := range stub.resources {
		if item.Identity == resource.Identity || (resource.Identity == "" && item.Name == resource.Name && item.Kind == resource.Kind) {
			continue
		}
		remaining = append(remaining, item)
	}
	stub.resources = remaining
	return nil
}

func testCleanupLedger(resources ...CleanupResource) CleanupLedger {
	return CleanupLedger{
		Version:   CleanupLedgerVersion,
		RunID:     "run-20260813",
		Marker:    "magelift/scaleway/rdb-recovery/run-20260813",
		Provider:  "scaleway",
		Region:    "fr-par",
		Project:   "11111111-1111-1111-1111-111111111111",
		Profile:   "default",
		ClaimedAt: "2026-08-13T10:00:00Z",
		Resources: resources,
	}
}

func TestReconcileCleanupDeletesSnapshotsBeforeSourceInstances(t *testing.T) {
	ledger := testCleanupLedger(
		CleanupResource{Kind: "rdb-instance", Role: CleanupRoleSource, Name: "magelift-rdb-run", Identity: "instance-1", Rank: CleanupRankSource, Status: CleanupStatusClaimed},
		CleanupResource{Kind: "rdb-snapshot", Role: CleanupRoleSnapshot, Identity: "snapshot-1", Rank: CleanupRankSnapshot, Status: CleanupStatusClaimed},
	)
	inventory := &mutatingCleanupInventory{resources: []CleanupInventoryResource{
		{Kind: "rdb-instance", Role: CleanupRoleSource, Name: "magelift-rdb-run", Identity: "instance-1", Owned: true, Live: true},
		{Kind: "rdb-snapshot", Role: CleanupRoleSnapshot, Identity: "snapshot-1", Owned: true, Live: true},
	}}
	report, err := ReconcileCleanup(context.Background(), ledger, inventory, inventory, CleanupReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "complete" {
		t.Fatalf("status = %q detail=%q", report.Status, report.Detail)
	}
	if len(inventory.deleted) != 2 || inventory.deleted[0] != "rdb-snapshot:snapshot-1" || inventory.deleted[1] != "rdb-instance:instance-1" {
		t.Fatalf("delete order = %#v", inventory.deleted)
	}
}

func TestReconcileCleanupBindsIntendedNameThenDeletesOwnedIdentity(t *testing.T) {
	ledger := testCleanupLedger(CleanupResource{
		Kind: "rdb-instance", Role: CleanupRoleSource, Name: "magelift-rdb-run", Rank: CleanupRankSource, Status: CleanupStatusIntended,
	})
	inventory := &mutatingCleanupInventory{resources: []CleanupInventoryResource{
		{Kind: "rdb-instance", Role: CleanupRoleSource, Name: "magelift-rdb-run", Identity: "instance-1", Owned: true, Live: true},
	}}
	report, err := ReconcileCleanup(context.Background(), ledger, inventory, inventory, CleanupReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "complete" || len(inventory.deleted) != 1 || inventory.deleted[0] != "rdb-instance:instance-1" {
		t.Fatalf("report = %#v deleted=%#v", report, inventory.deleted)
	}
}

func TestReconcileCleanupAdoptsOwnedInventoryMissingFromLedger(t *testing.T) {
	ledger := testCleanupLedger(CleanupResource{
		Kind: "rdb-instance", Role: CleanupRoleSource, Name: "magelift-rdb-run", Identity: "instance-1", Rank: CleanupRankSource, Status: CleanupStatusClaimed,
	})
	inventory := &mutatingCleanupInventory{resources: []CleanupInventoryResource{
		{Kind: "rdb-instance", Role: CleanupRoleSource, Name: "magelift-rdb-run", Identity: "instance-1", Owned: true, Live: true},
		{Kind: "rdb-snapshot", Role: CleanupRoleSnapshot, Name: "magelift-recovery-snap", Identity: "snapshot-2", Owned: true, Live: true},
	}}
	report, err := ReconcileCleanup(context.Background(), ledger, inventory, inventory, CleanupReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "complete" || len(inventory.deleted) != 2 || inventory.deleted[0] != "rdb-snapshot:snapshot-2" {
		t.Fatalf("adopted snapshot was not deleted first: %#v status=%s", inventory.deleted, report.Status)
	}
}

func TestReconcileCleanupRefusesForeignInventory(t *testing.T) {
	ledger := testCleanupLedger(CleanupResource{
		Kind: "rdb-instance", Role: CleanupRoleSource, Identity: "instance-1", Rank: CleanupRankSource, Status: CleanupStatusClaimed,
	})
	inventory := &cleanupInventoryStub{resources: []CleanupInventoryResource{
		{Kind: "rdb-instance", Role: CleanupRoleSource, Identity: "instance-1", Owned: false, Live: true},
	}}
	deleter := &cleanupDeleteStub{}
	report, err := ReconcileCleanup(context.Background(), ledger, inventory, deleter, CleanupReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "failed" || len(deleter.deleted) != 0 || len(report.Refused) != 1 {
		t.Fatalf("foreign resource was mutated: %#v", report)
	}
}

func TestReconcileCleanupTreatsTombstonesAsPendingNotComplete(t *testing.T) {
	ledger := testCleanupLedger(CleanupResource{
		Kind: "rdb-instance", Role: CleanupRoleSource, Identity: "instance-1", Rank: CleanupRankSource, Status: CleanupStatusClaimed,
	})
	inventory := &cleanupInventoryStub{resources: []CleanupInventoryResource{
		{Kind: "rdb-instance", Role: CleanupRoleSource, Identity: "instance-1", Owned: true, Delayed: true},
	}}
	deleter := &cleanupDeleteStub{}
	report, err := ReconcileCleanup(context.Background(), ledger, inventory, deleter, CleanupReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "pending" || len(deleter.deleted) != 0 {
		t.Fatalf("tombstone was treated as complete: %#v deleted=%#v", report, deleter.deleted)
	}
}

func TestReconcileCleanupDryRunDoesNotDelete(t *testing.T) {
	ledger := testCleanupLedger(CleanupResource{
		Kind: "rdb-instance", Role: CleanupRoleSource, Identity: "instance-1", Rank: CleanupRankSource, Status: CleanupStatusClaimed,
	})
	inventory := &cleanupInventoryStub{resources: []CleanupInventoryResource{
		{Kind: "rdb-instance", Role: CleanupRoleSource, Identity: "instance-1", Owned: true, Live: true},
	}}
	deleter := &cleanupDeleteStub{}
	report, err := ReconcileCleanup(context.Background(), ledger, inventory, deleter, CleanupReconcileOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "pending" || len(deleter.deleted) != 0 {
		t.Fatalf("dry-run mutated provider state: %#v deleted=%#v", report, deleter.deleted)
	}
}

type interruptibleCleanupDelete struct {
	inventory *mutatingCleanupInventory
	deleted   []string
	failOn    int
}

func (stub *interruptibleCleanupDelete) Delete(ctx context.Context, resource CleanupResource) error {
	stub.deleted = append(stub.deleted, cleanupResourceKey(resource))
	if stub.failOn > 0 && len(stub.deleted) >= stub.failOn {
		return errors.New("interrupted after first delete")
	}
	return stub.inventory.Delete(ctx, resource)
}

func TestReconcileCleanupPersistsAfterEachDeleteSoInterruptionCanResume(t *testing.T) {
	ledger := testCleanupLedger(
		CleanupResource{Kind: "rdb-snapshot", Role: CleanupRoleSnapshot, Identity: "snapshot-1", Rank: CleanupRankSnapshot, Status: CleanupStatusClaimed},
		CleanupResource{Kind: "rdb-instance", Role: CleanupRoleSource, Identity: "instance-1", Rank: CleanupRankSource, Status: CleanupStatusClaimed},
	)
	inventory := &mutatingCleanupInventory{resources: []CleanupInventoryResource{
		{Kind: "rdb-snapshot", Role: CleanupRoleSnapshot, Identity: "snapshot-1", Owned: true, Live: true},
		{Kind: "rdb-instance", Role: CleanupRoleSource, Identity: "instance-1", Owned: true, Live: true},
	}}
	deleter := &interruptibleCleanupDelete{inventory: inventory, failOn: 2}
	var persisted []CleanupLedger
	_, err := ReconcileCleanup(context.Background(), ledger, inventory, deleter, CleanupReconcileOptions{
		Persist: func(current CleanupLedger) error {
			clone := current
			clone.Resources = append([]CleanupResource(nil), current.Resources...)
			persisted = append(persisted, clone)
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "interrupted after first delete") {
		t.Fatalf("interruption error = %v", err)
	}
	if len(deleter.deleted) != 2 {
		t.Fatalf("deleted before interrupt = %#v", deleter.deleted)
	}
	if len(persisted) == 0 {
		t.Fatal("ledger was not persisted before the failed delete")
	}

	report, err := ReconcileCleanup(context.Background(), persisted[len(persisted)-1], inventory, inventory, CleanupReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "complete" {
		t.Fatalf("resume status = %q", report.Status)
	}
}

func TestReconcileCleanupLeavesUnmaterializedIntendedClaimsComplete(t *testing.T) {
	ledger := testCleanupLedger(CleanupResource{
		Kind: "rdb-instance", Role: CleanupRoleSource, Name: "magelift-rdb-run", Rank: CleanupRankSource, Status: CleanupStatusIntended,
	})
	inventory := &cleanupInventoryStub{}
	report, err := ReconcileCleanup(context.Background(), ledger, inventory, &cleanupDeleteStub{}, CleanupReconcileOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "complete" {
		t.Fatalf("unmaterialized intended claim was not complete: %#v", report)
	}
}

func TestValidateCleanupLedgerRejectsSecretBearingFields(t *testing.T) {
	ledger := testCleanupLedger()
	ledger.Profile = "token-value"
	if err := ValidateCleanupLedger(ledger); err == nil {
		t.Fatal("secret-bearing profile was accepted")
	}
}

func TestReconcileCleanupRequiresInventoryClient(t *testing.T) {
	_, err := ReconcileCleanup(context.Background(), testCleanupLedger(), nil, &cleanupDeleteStub{}, CleanupReconcileOptions{})
	if err == nil || !strings.Contains(err.Error(), "inventory client is required") {
		t.Fatalf("error = %v", err)
	}
}
