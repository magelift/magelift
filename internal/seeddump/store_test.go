package seeddump

import (
	"context"
	"encoding/json"
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
