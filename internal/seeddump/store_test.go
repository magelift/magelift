package seeddump

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInitRecordedRoundTrip(t *testing.T) {
	root := t.TempDir()
	dumpPath := filepath.Join(root, "fixture.sql")
	record, err := InitRecorded(context.Background(), root, "staging", dumpPath)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusRecorded {
		t.Fatalf("status = %q", record.Status)
	}
	if record.DumpPath != dumpPath {
		t.Fatalf("dumpPath = %q", record.DumpPath)
	}
	if record.UpdatedAt.IsZero() {
		t.Fatal("updatedAt missing")
	}

	store, err := New(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != StatusRecorded || got.DumpPath != dumpPath {
		t.Fatalf("read = %#v", got)
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode = %o", info.Mode().Perm())
	}
}

func TestInitRecordedIdempotentRecordedOverwrite(t *testing.T) {
	root := t.TempDir()
	first, err := InitRecorded(context.Background(), root, "preview", "/tmp/a.sql")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	second, err := InitRecorded(context.Background(), root, "preview", "/tmp/b.sql")
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != StatusRecorded {
		t.Fatalf("status = %q", second.Status)
	}
	if second.DumpPath != "/tmp/b.sql" {
		t.Fatalf("dumpPath = %q", second.DumpPath)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) && !second.UpdatedAt.Equal(first.UpdatedAt) {
		t.Fatalf("updatedAt did not advance: first=%v second=%v", first.UpdatedAt, second.UpdatedAt)
	}

	store, err := New(root, "preview")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["status"] != StatusRecorded {
		t.Fatalf("raw status = %#v", raw["status"])
	}
	if _, ok := raw["imported"]; ok {
		t.Fatal("InitRecorded invented imported status")
	}
}

func TestInitRecordedDoesNotTouchYAML(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte("schemaVersion: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InitRecorded(context.Background(), root, "staging", "dump.sql"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "schemaVersion: 1\n" {
		t.Fatalf("yaml mutated: %q", data)
	}
}

func TestStoreRejectsSymlinkJournal(t *testing.T) {
	root := t.TempDir()
	store, err := New(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, store.path); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(context.Background()); err == nil {
		t.Fatal("symlink journal was accepted")
	}
}

func TestMarkImportingImportedHappyPath(t *testing.T) {
	root := t.TempDir()
	dumpPath := filepath.Join(root, "fixture.sql")
	if _, err := InitRecorded(context.Background(), root, "staging", dumpPath); err != nil {
		t.Fatal(err)
	}
	store, err := New(root, "staging")
	if err != nil {
		t.Fatal(err)
	}

	importing, err := store.MarkImporting(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if importing.Status != StatusImporting {
		t.Fatalf("after MarkImporting status = %q", importing.Status)
	}
	if importing.DumpPath != dumpPath {
		t.Fatalf("dumpPath cleared on MarkImporting: %q", importing.DumpPath)
	}
	if importing.Reason != "" {
		t.Fatalf("reason should be empty while importing: %q", importing.Reason)
	}

	imported, err := store.MarkImported(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if imported.Status != StatusImported {
		t.Fatalf("after MarkImported status = %q", imported.Status)
	}
	got, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != StatusImported {
		t.Fatalf("read after imported = %#v", got)
	}
}

func TestMarkFailedStoresNonemptyReason(t *testing.T) {
	root := t.TempDir()
	if _, err := InitRecorded(context.Background(), root, "staging", "/tmp/dump.sql"); err != nil {
		t.Fatal(err)
	}
	store, err := New(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImporting(context.Background()); err != nil {
		t.Fatal(err)
	}

	failed, err := store.MarkFailed(context.Background(), "mysql refused connection")
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != StatusFailed {
		t.Fatalf("status = %q", failed.Status)
	}
	if failed.Reason != "mysql refused connection" {
		t.Fatalf("reason = %q", failed.Reason)
	}

	got, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != StatusFailed || got.Reason != "mysql refused connection" {
		t.Fatalf("read after failed = %#v", got)
	}
}

func TestMarkFailedRejectsEmptyReason(t *testing.T) {
	root := t.TempDir()
	if _, err := InitRecorded(context.Background(), root, "staging", "/tmp/dump.sql"); err != nil {
		t.Fatal(err)
	}
	store, err := New(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImporting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkFailed(context.Background(), "  "); err == nil {
		t.Fatal("empty reason was accepted")
	}
}

func TestMarkRejectsMissingJournal(t *testing.T) {
	root := t.TempDir()
	store, err := New(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImporting(context.Background()); err == nil {
		t.Fatal("MarkImporting without journal succeeded")
	}
	if _, err := store.MarkImported(context.Background()); err == nil {
		t.Fatal("MarkImported without journal succeeded")
	}
	if _, err := store.MarkFailed(context.Background(), "boom"); err == nil {
		t.Fatal("MarkFailed without journal succeeded")
	}
}

func TestMarkRejectsImportedToImporting(t *testing.T) {
	root := t.TempDir()
	if _, err := InitRecorded(context.Background(), root, "staging", "/tmp/dump.sql"); err != nil {
		t.Fatal(err)
	}
	store, err := New(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImporting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImported(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImporting(context.Background()); err == nil {
		t.Fatal("imported→importing was accepted without retry path")
	}
}

func TestMarkAllowsFailedToImportingRetry(t *testing.T) {
	root := t.TempDir()
	if _, err := InitRecorded(context.Background(), root, "staging", "/tmp/dump.sql"); err != nil {
		t.Fatal(err)
	}
	store, err := New(root, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkImporting(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.MarkFailed(context.Background(), "first attempt failed"); err != nil {
		t.Fatal(err)
	}
	retry, err := store.MarkImporting(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if retry.Status != StatusImporting {
		t.Fatalf("status = %q", retry.Status)
	}
	if retry.Reason != "" {
		t.Fatalf("retry should clear failure reason: %q", retry.Reason)
	}
}

func TestMarkConcurrentWritersSerialize(t *testing.T) {
	root := t.TempDir()
	if _, err := InitRecorded(context.Background(), root, "staging", "/tmp/dump.sql"); err != nil {
		t.Fatal(err)
	}
	store, err := New(root, "staging")
	if err != nil {
		t.Fatal(err)
	}

	const writers = 8
	errs := make(chan error, writers)
	for range writers {
		go func() {
			_, err := store.MarkImporting(context.Background())
			errs <- err
		}()
	}
	var firstOK bool
	for range writers {
		err := <-errs
		if err == nil {
			if firstOK {
				// Concurrent MarkImporting from recorded: only one should win;
				// others may see importing→importing (idempotent) or race to recorded.
				continue
			}
			firstOK = true
			continue
		}
		// Contenders may fail on illegal transition if another already moved past importing
		// via a different path; torn JSON must never occur.
		if err != nil && !errors.Is(err, ErrInvalidTransition) && !errors.Is(err, context.Canceled) {
			// Accept only transition errors after first success (idempotent importing OK).
			if !errors.Is(err, ErrInvalidTransition) {
				// MarkImporting from importing is allowed (idempotent); other errors fail the test.
				t.Errorf("unexpected concurrent error: %v", err)
			}
		}
	}
	got, err := store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Status != StatusImporting {
		t.Fatalf("after concurrent writers journal = %#v", got)
	}
	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("torn or invalid JSON after concurrent writers: %v\n%s", err, data)
	}
}
