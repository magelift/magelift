package state

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type memObjects struct {
	mu   sync.Mutex
	data map[string][]byte
	gen  map[string]int64
}

func newMem() *memObjects { return &memObjects{data: map[string][]byte{}, gen: map[string]int64{}} }

func (m *memObjects) nextGen(key string) string {
	m.gen[key]++
	return strconv.FormatInt(m.gen[key], 10)
}

func (m *memObjects) Get(_ context.Context, _, key string) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, ok := m.data[key]
	if !ok {
		return nil, "", ErrNotLocked
	}
	return append([]byte(nil), body...), strconv.FormatInt(m.gen[key], 10), nil
}
func (m *memObjects) PutIfAbsent(_ context.Context, _, key string, body []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; ok {
		return "", ErrLocked
	}
	m.data[key] = append([]byte(nil), body...)
	return m.nextGen(key), nil
}
func (m *memObjects) Put(_ context.Context, _, key string, body []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = append([]byte(nil), body...)
	return m.nextGen(key), nil
}
func (m *memObjects) Delete(_ context.Context, _, key string) error {
	return m.DeleteGeneration(context.Background(), "", key, "")
}
func (m *memObjects) DeleteGeneration(_ context.Context, _, key, generation string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; !ok {
		return nil
	}
	if generation != "" && strconv.FormatInt(m.gen[key], 10) != generation {
		return errors.New("conditionNotMet: generation mismatch")
	}
	delete(m.data, key)
	delete(m.gen, key)
	return nil
}
func (m *memObjects) Copy(_ context.Context, _, srcKey, _, dstKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, ok := m.data[srcKey]
	if !ok {
		return ErrNotLocked
	}
	m.data[dstKey] = append([]byte(nil), body...)
	return nil
}

func (m *memObjects) List(_ context.Context, _, prefix string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	keys := make([]string, 0, len(m.data))
	for key := range m.data {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

func TestAcquireLockAndRelease(t *testing.T) {
	t.Parallel()
	manager, err := NewManager(newMem(), "magelift-state-bucket-test", "shop", "preview")
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC) }
	handle, err := manager.Acquire(context.Background(), "shop", "preview", "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Acquire(context.Background(), "shop", "preview", "owner-2")
	if !errors.Is(err, ErrLocked) && err == nil {
		t.Fatalf("expected lock conflict, got %v", err)
	}
	if err := handle.Release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Status(context.Background()); !errors.Is(err, ErrNotLocked) {
		t.Fatalf("expected unlocked, got %v", err)
	}
}

func TestReleaseDoesNotDeleteSuccessorLock(t *testing.T) {
	t.Parallel()
	objects := newMem()
	manager, err := NewManager(objects, "magelift-state-bucket-test", "shop", "preview")
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC) }
	handle, err := manager.Acquire(context.Background(), "shop", "preview", "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	manager.now = func() time.Time { return time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC) }
	if _, err := objects.Put(context.Background(), manager.bucket, manager.key, []byte(`{"project":"shop","environment":"preview","owner":"owner-2","acquiredAt":"2026-07-21T00:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
	if err := handle.Release(context.Background()); err == nil || !strings.Contains(err.Error(), "ownership changed") {
		t.Fatalf("release error = %v", err)
	}
	if _, ok := objects.data[manager.key]; !ok {
		t.Fatal("successor lock object was deleted")
	}
	current, err := manager.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Owner != "owner-2" {
		t.Fatalf("successor owner = %q", current.Owner)
	}
}

func TestArchiveCreatesCompleteSnapshotAndRestoresIt(t *testing.T) {
	t.Parallel()
	objects := newMem()
	objects.data["stacks/shop.json"] = []byte("state")
	objects.data["stacks/old.json"] = []byte("old")
	objects.data["locks/shop/preview.json"] = []byte("lock")
	archive, err := NewArchive(objects, "magelift-state-bucket-test", "shop", "preview")
	if err != nil {
		t.Fatal(err)
	}
	archive.now = func() time.Time { return time.Date(2026, 7, 20, 0, 0, 0, 123456789, time.UTC) }
	backup, err := archive.Backup(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if backup.ID != "20260720T000000.123456789Z" || backup.Objects != 2 {
		t.Fatalf("backup=%#v", backup)
	}
	if _, ok := objects.data["backups/shop/preview/20260720T000000.123456789Z/.magelift-complete"]; !ok {
		t.Fatal("backup completion marker is missing")
	}
	objects.data["stacks/shop.json"] = []byte("changed")
	objects.data["stacks/stale.json"] = []byte("stale")
	restored, err := archive.Restore(context.Background(), backup.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Objects != 2 || string(objects.data["stacks/shop.json"]) != "state" || string(objects.data["stacks/old.json"]) != "old" {
		t.Fatalf("restore=%#v objects=%#v", restored, objects.data)
	}
	if _, ok := objects.data["stacks/stale.json"]; ok {
		t.Fatal("stale state object was not removed")
	}
	if _, ok := objects.data["backups/shop/preview/20260720T000000.123456789Z/locks/shop/preview.json"]; ok {
		t.Fatal("lock was copied into backup")
	}
}

func TestArchiveRejectsIncompleteSnapshotBeforeMutation(t *testing.T) {
	t.Parallel()
	objects := newMem()
	objects.data["stacks/shop.json"] = []byte("current")
	objects.data["backups/shop/preview/20260720T000000.123456789Z/stacks/shop.json"] = []byte("partial")
	archive, err := NewArchive(objects, "magelift-state-bucket-test", "shop", "preview")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Restore(context.Background(), "20260720T000000.123456789Z"); err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("error=%v", err)
	}
	if string(objects.data["stacks/shop.json"]) != "current" {
		t.Fatal("restore mutated current state before verifying completion")
	}
}
