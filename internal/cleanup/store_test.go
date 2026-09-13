package cleanup

import (
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func testStoreLedger() sdk.CleanupLedger {
	return sdk.CleanupLedger{
		Version:   sdk.CleanupLedgerVersion,
		RunID:     "run-store",
		Marker:    "magelift/scaleway/rdb-recovery/run-store",
		Provider:  "scaleway",
		Region:    "fr-par",
		Project:   "11111111-1111-1111-1111-111111111111",
		Profile:   "default",
		ClaimedAt: "2026-08-13T10:00:00Z",
	}
}

func TestSaveLoadRoundTripAndClaimBeforeIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledgers", "run-store.json")
	ledger, err := Claim(path, testStoreLedger(), sdk.CleanupResource{
		Kind: "rdb-instance", Role: sdk.CleanupRoleSource, Name: "magelift-rdb-run", Status: sdk.CleanupStatusIntended,
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Resources) != 1 || loaded.Resources[0].Status != sdk.CleanupStatusIntended || loaded.Resources[0].Rank != sdk.CleanupRankSource {
		t.Fatalf("loaded = %#v", loaded)
	}
	updated, err := RecordIdentity(path, ledger, "rdb-instance", "magelift-rdb-run", "instance-1")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Resources[0].Status != sdk.CleanupStatusClaimed || updated.Resources[0].Identity != "instance-1" {
		t.Fatalf("recorded = %#v", updated.Resources[0])
	}
}

func TestListDirFailsClosedOnCorruptLedger(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ListDir(dir); err == nil {
		t.Fatal("corrupt ledger was skipped")
	}
}

func TestListDirTreatsMissingDirectoryAsEmpty(t *testing.T) {
	paths, ledgers, err := ListDir(filepath.Join(t.TempDir(), "missing"))
	if err != nil || len(paths) != 0 || len(ledgers) != 0 {
		t.Fatalf("paths=%v ledgers=%v err=%v", paths, ledgers, err)
	}
}
