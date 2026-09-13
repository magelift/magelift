package resilience

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestInventoryOwnedDatabaseForLedgerIncludesSourceAndRefusesForeign(t *testing.T) {
	database := newFakeScalewayDatabase()
	database.instances["source"] = DatabaseInstance{
		ID: "source", Name: "magelift-rdb-run", Region: "fr-par", Status: "ready",
		Tags: []string{scalewayDatabaseOwnershipTag + "=owner", scalewayDatabaseClassTag + "=database"},
	}
	database.instances["foreign"] = DatabaseInstance{
		ID: "foreign", Name: "other", Region: "fr-par", Status: "ready",
		Tags: []string{scalewayDatabaseOwnershipTag + "=other", scalewayDatabaseClassTag + "=database"},
	}
	database.snapshots["snap"] = DatabaseSnapshot{ID: "snap", InstanceID: "source", Name: "magelift-recovery-snap", Region: "fr-par", Status: "ready"}
	database.snapshots["foreign-snap"] = DatabaseSnapshot{ID: "foreign-snap", InstanceID: "foreign", Name: "other-snap", Region: "fr-par", Status: "ready"}

	resources, err := InventoryOwnedDatabaseForLedger(context.Background(), database, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 2 {
		t.Fatalf("resources = %#v", resources)
	}
	if err := DeleteOwnedDatabaseForLedger(context.Background(), database, "owner", sdk.CleanupResource{Kind: "rdb-snapshot", Identity: "snap"}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteOwnedDatabaseForLedger(context.Background(), database, "owner", sdk.CleanupResource{Kind: "rdb-instance", Identity: "source"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := database.instances["source"]; ok {
		t.Fatal("owned source instance remained")
	}
	if _, ok := database.instances["foreign"]; !ok {
		t.Fatal("foreign instance was deleted")
	}
	if err := DeleteOwnedDatabaseForLedger(context.Background(), database, "owner", sdk.CleanupResource{Kind: "rdb-instance", Identity: "foreign"}); err == nil || !strings.Contains(err.Error(), "without the exact ownership marker") {
		t.Fatalf("foreign delete error = %v", err)
	}
}
