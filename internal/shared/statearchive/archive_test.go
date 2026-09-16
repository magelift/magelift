package statearchive

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"
)

type memoryStore struct {
	objects map[string][]byte
}

func (s *memoryStore) List(_ context.Context, prefix string) ([]string, error) {
	keys := make([]string, 0, len(s.objects))
	for key := range s.objects {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func (s *memoryStore) Read(_ context.Context, key string) ([]byte, error) {
	body, ok := s.objects[key]
	if !ok {
		return nil, context.Canceled
	}
	return append([]byte(nil), body...), nil
}

func (s *memoryStore) Copy(_ context.Context, source, target string) error {
	body, ok := s.objects[source]
	if !ok {
		return context.Canceled
	}
	s.objects[target] = append([]byte(nil), body...)
	return nil
}

func (s *memoryStore) Put(_ context.Context, key string, body []byte) error {
	s.objects[key] = append([]byte(nil), body...)
	return nil
}

func (s *memoryStore) Delete(_ context.Context, key string) error {
	delete(s.objects, key)
	return nil
}

func TestArchiveBackupAndRestoreUseOneCompleteSnapshotProtocol(t *testing.T) {
	store := &memoryStore{objects: map[string][]byte{
		"stacks/shop.json":        []byte("state"),
		"stacks/stale.json":       []byte("stale"),
		"locks/shop/preview.json": []byte("lock"),
	}}
	archive, err := New(store, "backups/shop/preview/", "gs://state/")
	if err != nil {
		t.Fatal(err)
	}
	archive.SetClock(func() time.Time { return time.Date(2026, 7, 20, 0, 0, 0, 123456789, time.UTC) })
	backup, err := archive.Backup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if backup.ID != "20260720T000000.123456789Z" || backup.Objects != 2 {
		t.Fatalf("backup=%#v", backup)
	}
	if backup.Bytes != int64(len("state")+len("stale")) || backup.ManifestDigest == "" {
		t.Fatalf("backup integrity metadata=%#v", backup)
	}
	if _, ok := store.objects["backups/shop/preview/20260720T000000.123456789Z/"+CompleteMarker]; !ok {
		t.Fatal("completion marker is missing")
	}
	if _, ok := store.objects["backups/shop/preview/20260720T000000.123456789Z/"+ManifestObject]; !ok {
		t.Fatal("backup manifest is missing")
	}
	store.objects["stacks/shop.json"] = []byte("changed")
	store.objects["stacks/new.json"] = []byte("new")
	restored, err := archive.Restore(context.Background(), backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Objects != 2 || string(store.objects["stacks/shop.json"]) != "state" || string(store.objects["stacks/stale.json"]) != "stale" {
		t.Fatalf("restore=%#v objects=%#v", restored, store.objects)
	}
	if _, ok := store.objects["stacks/new.json"]; ok {
		t.Fatal("stale object survived restore")
	}
}

func TestArchiveRejectsTamperedSnapshotBeforeMutation(t *testing.T) {
	store := &memoryStore{objects: map[string][]byte{"stacks/shop.json": []byte("current")}}
	archive, err := New(store, "backups/shop/preview/", "gs://state/")
	if err != nil {
		t.Fatal(err)
	}
	archive.SetClock(func() time.Time { return time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC) })
	backup, err := archive.Backup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	store.objects["backups/shop/preview/"+backup.ID+"/stacks/shop.json"] = []byte("tampered")
	store.objects["stacks/shop.json"] = []byte("changed")
	store.objects["stacks/new.json"] = []byte("new")
	if _, err := archive.Restore(context.Background(), backup.ID); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("error=%v", err)
	}
	if string(store.objects["stacks/shop.json"]) != "changed" || string(store.objects["stacks/new.json"]) != "new" {
		t.Fatal("tampered restore mutated current state")
	}
}

func TestArchiveRejectsIncompleteSnapshotBeforeMutation(t *testing.T) {
	store := &memoryStore{objects: map[string][]byte{
		"stacks/shop.json": []byte("current"),
		"backups/shop/preview/20260720T000000.123456789Z/stacks/shop.json": []byte("partial"),
	}}
	archive, err := New(store, "backups/shop/preview/", "gs://state/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Restore(context.Background(), "20260720T000000.123456789Z"); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("error=%v", err)
	}
	if string(store.objects["stacks/shop.json"]) != "current" {
		t.Fatal("restore mutated state before completion validation")
	}
}

func TestArchiveRejectsInvalidCompletionMarkerBeforeMutation(t *testing.T) {
	store := &memoryStore{objects: map[string][]byte{"stacks/shop.json": []byte("current")}}
	archive, err := New(store, "backups/shop/preview/", "gs://state/")
	if err != nil {
		t.Fatal(err)
	}
	archive.SetClock(func() time.Time { return time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC) })
	backup, err := archive.Backup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	markerKey := "backups/shop/preview/" + backup.ID + "/" + CompleteMarker
	store.objects[markerKey] = []byte("not-complete\n")
	store.objects["stacks/shop.json"] = []byte("changed")
	if _, err := archive.Restore(context.Background(), backup.ID); err == nil || !strings.Contains(err.Error(), "completion marker") {
		t.Fatalf("error=%v", err)
	}
	if string(store.objects["stacks/shop.json"]) != "changed" {
		t.Fatal("invalid completion marker mutated current state")
	}
}

func TestArchiveRequiresContexts(t *testing.T) {
	store := &memoryStore{objects: map[string][]byte{"stacks/shop.json": []byte("state")}}
	archive, err := New(store, "backups/shop/preview/", "gs://state/")
	if err != nil {
		t.Fatal(err)
	}
	var nilContext context.Context
	if _, err := archive.Backup(nilContext); err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("nil backup context error=%v", err)
	}
	if _, err := archive.Restore(nilContext, "20260720T000000.000000000Z"); err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("nil restore context error=%v", err)
	}
}
