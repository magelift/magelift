package state

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memObjects struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMem() *memObjects { return &memObjects{data: map[string][]byte{}} }

func (m *memObjects) Get(_ context.Context, _, key string) ([]byte, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	body, ok := m.data[key]
	if !ok {
		return nil, "", ErrNotLocked
	}
	return append([]byte(nil), body...), "1", nil
}
func (m *memObjects) PutIfAbsent(_ context.Context, _, key string, body []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.data[key]; ok {
		return "", ErrLocked
	}
	m.data[key] = append([]byte(nil), body...)
	return "1", nil
}
func (m *memObjects) Put(_ context.Context, _, key string, body []byte) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = append([]byte(nil), body...)
	return "1", nil
}
func (m *memObjects) Delete(_ context.Context, _, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
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
	if err := handle.Release(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Status(context.Background()); !errors.Is(err, ErrNotLocked) {
		t.Fatalf("expected unlocked, got %v", err)
	}
}
