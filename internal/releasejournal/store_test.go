package releasejournal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStoreAppendsAtomicallyAndPreservesSequence(t *testing.T) {
	store, err := New(t.TempDir(), "staging")
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC) }
	const count = 8
	var wait sync.WaitGroup
	errors := make(chan error, count)
	for index := range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.Append(context.Background(), Entry{Action: ActionPromote, Environment: "staging", DigestReference: fmt.Sprintf("registry.example.invalid/shop@sha256:%064x", index+1), SignatureIdentity: "identity", SignatureIssuer: "https://issuer.example.invalid", ForwardOnly: true})
			errors <- err
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	entries, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != count {
		t.Fatalf("entries = %d", len(entries))
	}
	for index, entry := range entries {
		if entry.Sequence != index+1 || !entry.RecordedAt.Equal(store.now()) {
			t.Fatalf("entry %d = %#v", index, entry)
		}
	}
	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("journal mode = %o", info.Mode().Perm())
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
	if _, err := store.List(context.Background()); err == nil {
		t.Fatal("symlink journal was accepted")
	}
}
