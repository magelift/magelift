package plugin

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

type fakeObjectStore struct {
	objects map[string][]byte
	uploads map[string][]byte
}

func (f *fakeObjectStore) List(_ context.Context, _ string, prefix string) ([]string, error) {
	var names []string
	for name := range f.objects {
		if strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	return names, nil
}

func (f *fakeObjectStore) Download(_ context.Context, _ string, object, dest string) (int64, error) {
	body, ok := f.objects[object]
	if !ok {
		return 0, os.ErrNotExist
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return 0, err
	}
	return int64(len(body)), nil
}

func (f *fakeObjectStore) Upload(_ context.Context, _ string, object, src string) (int64, error) {
	body, err := os.ReadFile(src)
	if err != nil {
		return 0, err
	}
	if f.uploads == nil {
		f.uploads = map[string][]byte{}
	}
	f.uploads[object] = body
	return int64(len(body)), nil
}

func mediaTestOutputs(t *testing.T) []byte {
	t.Helper()
	outputs, err := json.Marshal(map[string]any{"mediaBucket": "shop-media"})
	if err != nil {
		t.Fatal(err)
	}
	return outputs
}

func TestMediaExportDownloadsPrefixTree(t *testing.T) {
	t.Parallel()
	store := &fakeObjectStore{objects: map[string][]byte{
		"media/catalog/a.jpg": []byte("aaa"),
		"media/d.jpg":         []byte("dddd"),
		"other/skip.txt":      []byte("skip"),
	}}
	server := &Server{NewMediaStore: func() objectStore { return store }}
	dest := t.TempDir()
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	result, operr := server.MediaExport(context.Background(), &sdk.MediaTransferCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan,
		OutputsJSON: mediaTestOutputs(t), LocalDir: dest,
	})
	if operr != nil {
		t.Fatal(operr)
	}
	if result.FileCount != 2 || result.ByteCount != 7 {
		t.Fatalf("result = %+v", result)
	}
	if body, err := os.ReadFile(filepath.Join(dest, "catalog", "a.jpg")); err != nil || string(body) != "aaa" {
		t.Fatalf("exported tree wrong: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "other")); !os.IsNotExist(err) {
		t.Fatal("unprefixed objects exported")
	}
}

func TestMediaImportUploadsTree(t *testing.T) {
	t.Parallel()
	store := &fakeObjectStore{}
	server := &Server{NewMediaStore: func() objectStore { return store }}
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "catalog"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "catalog", "b.jpg"), []byte("bb"), 0o644); err != nil {
		t.Fatal(err)
	}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	result, operr := server.MediaImport(context.Background(), &sdk.MediaTransferCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan,
		OutputsJSON: mediaTestOutputs(t), LocalDir: src,
	})
	if operr != nil {
		t.Fatal(operr)
	}
	if result.FileCount != 1 || result.ByteCount != 2 {
		t.Fatalf("result = %+v", result)
	}
	if got := store.uploads["media/catalog/b.jpg"]; string(got) != "bb" {
		t.Fatalf("uploads = %#v", store.uploads)
	}
}

func TestMediaTransferValidatesInput(t *testing.T) {
	t.Parallel()
	server := &Server{NewMediaStore: func() objectStore { return &fakeObjectStore{} }}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	if _, operr := server.MediaExport(context.Background(), nil); operr == nil {
		t.Fatal("nil call accepted")
	}
	relative := &sdk.MediaTransferCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, OutputsJSON: mediaTestOutputs(t), LocalDir: "relative/path"}
	if _, operr := server.MediaExport(context.Background(), relative); operr == nil {
		t.Fatal("relative dir accepted")
	}
	missing := &sdk.MediaTransferCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, OutputsJSON: mediaTestOutputs(t), LocalDir: filepath.Join(t.TempDir(), "absent")}
	if _, operr := server.MediaImport(context.Background(), missing); operr == nil {
		t.Fatal("absent import dir accepted")
	}
	nobucket := &sdk.MediaTransferCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, OutputsJSON: []byte(`{}`), LocalDir: t.TempDir()}
	if _, operr := server.MediaExport(context.Background(), nobucket); operr == nil {
		t.Fatal("missing bucket accepted")
	}
}
