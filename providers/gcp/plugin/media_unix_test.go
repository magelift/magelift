//go:build unix

package plugin

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/magelift/magelift/sdk"
	"golang.org/x/sys/unix"
)

func TestMediaImportRejectsNonRegularFiles(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	if err := unix.Mkfifo(filepath.Join(src, "pipe"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := &fakeObjectStore{}
	server := &Server{NewMediaStore: func() objectStore { return store }}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	_, operr := server.MediaImport(context.Background(), &sdk.MediaTransferCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan,
		OutputsJSON: mediaTestOutputs(t), LocalDir: src,
	})
	if operr == nil {
		t.Fatal("import accepted a fifo")
	}
	if len(store.uploads) != 0 {
		t.Fatalf("uploads = %#v", store.uploads)
	}
}
