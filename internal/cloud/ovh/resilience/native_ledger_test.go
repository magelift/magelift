package resilience

import (
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestInventoryOwnedDatabaseForLedgerUsesSourceAndRestoreDescriptions(t *testing.T) {
	marker := "magelift/ovh/database-recovery/ledger"
	database := newFakeOVHDatabase()
	database.instances["source"] = DatabaseInstance{ID: "source", Description: marker, Engine: "mysql"}
	database.instances["restore"] = DatabaseInstance{ID: "restore", Description: ovhDatabaseRestoreDescriptionPrefix(marker) + strings.Repeat("a", 24), Engine: "mysql"}
	database.instances["foreign"] = DatabaseInstance{ID: "foreign", Description: marker + "/foreign", Engine: "mysql"}

	resources, err := InventoryOwnedDatabaseForLedger(context.Background(), database, "mysql", marker)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 2 {
		t.Fatalf("resources = %#v, want source and restore", resources)
	}
	seen := make(map[string]sdk.CleanupInventoryResource, len(resources))
	for _, resource := range resources {
		seen[resource.Identity] = resource
	}
	if seen["source"].Role != sdk.CleanupRoleSource || seen["restore"].Role != sdk.CleanupRoleRestore || seen["foreign"].Identity != "" {
		t.Fatalf("ownership inventory = %#v", seen)
	}
}

func TestDeleteOwnedDatabaseForLedgerRechecksDescription(t *testing.T) {
	marker := "magelift/ovh/database-recovery/ledger-delete"
	database := newFakeOVHDatabase()
	database.instances["owned"] = DatabaseInstance{ID: "owned", Description: marker, Engine: "mysql"}
	database.instances["foreign"] = DatabaseInstance{ID: "foreign", Description: "unrelated", Engine: "mysql"}

	if err := DeleteOwnedDatabaseForLedger(context.Background(), database, "mysql", marker, sdk.CleanupResource{Kind: "database-instance", Identity: "foreign"}); err == nil || !strings.Contains(err.Error(), "exact ownership marker") {
		t.Fatalf("foreign delete error = %v", err)
	}
	if err := DeleteOwnedDatabaseForLedger(context.Background(), database, "mysql", marker, sdk.CleanupResource{Kind: "database-instance", Identity: "owned"}); err != nil {
		t.Fatal(err)
	}
	if database.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", database.deleteCalls)
	}
}
