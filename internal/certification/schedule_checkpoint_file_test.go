package certification

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileScheduleCheckpointStoreMergesAndPersistsUnitWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoints.json")
	first, err := NewFileScheduleCheckpointStore(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFileScheduleCheckpointStore(path)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := reusableFingerprint()
	digest, err := fingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := SchedulerCheckpoint{
		CellID: "baseline", Fingerprint: digest, ReuseFingerprint: &fingerprint,
		OwnershipMarker: fingerprint.OwnershipMarker, Status: "IN_PROGRESS", UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := first.Save(context.Background(), []SchedulerCheckpoint{checkpoint}); err != nil {
		t.Fatal(err)
	}
	checkpoint.Status = "PASS"
	checkpoint.StackID = "stack-1"
	if err := first.Save(context.Background(), []SchedulerCheckpoint{checkpoint}); err != nil {
		t.Fatal(err)
	}
	other := checkpoint
	other.CellID = "transition"
	if err := first.Save(context.Background(), []SchedulerCheckpoint{other}); err != nil {
		t.Fatal(err)
	}
	loaded, err := second.Load(context.Background())
	if err != nil || len(loaded) != 2 || loaded[0].CellID != "baseline" || loaded[0].Status != "PASS" || loaded[1].CellID != "transition" {
		t.Fatalf("loaded checkpoints = %#v, err=%v", loaded, err)
	}
	conflict := checkpoint
	conflict.Fingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := second.Save(context.Background(), []SchedulerCheckpoint{conflict}); err == nil {
		t.Fatal("stale checkpoint identity was accepted")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file schedule checkpoint mode = %o, want 600", info.Mode().Perm())
	}
}

func TestFileScheduleCheckpointStoreHonorsContextWhileWaitingForLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoints.json")
	store, err := NewFileScheduleCheckpointStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".lock", 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path + ".lock")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, loadErr := store.Load(ctx)
		result <- loadErr
	}()
	time.Sleep(2 * store.lockPoll)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled checkpoint load = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("file schedule checkpoint load did not honor cancellation while waiting for lock")
	}
}
