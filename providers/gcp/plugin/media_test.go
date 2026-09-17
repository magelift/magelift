package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func (f *fakeObjectStore) Download(_ context.Context, _ string, object, root, name string) (int64, error) {
	if err := checkContainedName(name); err != nil {
		return 0, err
	}
	body, ok := f.objects[object]
	if !ok {
		return 0, os.ErrNotExist
	}
	dest := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return 0, err
	}
	return int64(len(body)), nil
}

func (f *fakeObjectStore) Upload(_ context.Context, _ string, object string, src *os.File) (int64, error) {
	if src == nil {
		return 0, fmt.Errorf("no open file")
	}
	body, err := io.ReadAll(src)
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

func TestCheckContainedName(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"a.jpg", "catalog/a.jpg", "photo..jpg", "a..b/c..d.jpg", "deep/nested/dir/f.png"} {
		if err := checkContainedName(ok); err != nil {
			t.Errorf("checkContainedName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{"", "   ", "/abs.jpg", "../evil", "a/../../evil", "a/../b", ".", "./a", "a/./b"} {
		if err := checkContainedName(bad); err == nil {
			t.Errorf("checkContainedName(%q) = nil, want error", bad)
		}
	}
}

func TestMediaExportRejectsTraversalObjects(t *testing.T) {
	t.Parallel()
	store := &fakeObjectStore{objects: map[string][]byte{
		"media/../evil.txt": []byte("evil"),
	}}
	server := &Server{NewMediaStore: func() objectStore { return store }}
	parent := t.TempDir()
	dest := filepath.Join(parent, "export")
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	_, operr := server.MediaExport(context.Background(), &sdk.MediaTransferCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan,
		OutputsJSON: mediaTestOutputs(t), LocalDir: dest,
	})
	if operr == nil {
		t.Fatal("export accepted a traversal object")
	}
	if _, err := os.Stat(filepath.Join(parent, "evil.txt")); !os.IsNotExist(err) {
		t.Fatal("traversal object escaped the destination")
	}
	if len(store.uploads) != 0 {
		t.Fatalf("uploads = %#v", store.uploads)
	}
}

func TestMediaExportAcceptsDottedNames(t *testing.T) {
	t.Parallel()
	store := &fakeObjectStore{objects: map[string][]byte{
		"media/photo..jpg": []byte("dots"),
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
	if result.FileCount != 1 {
		t.Fatalf("result = %+v", result)
	}
	if body, err := os.ReadFile(filepath.Join(dest, "photo..jpg")); err != nil || string(body) != "dots" {
		t.Fatalf("dotted name not exported: %v", err)
	}
}

func TestMediaExportRefusesLeafSymlink(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := os.Symlink(target, filepath.Join(dest, "a.jpg")); err != nil {
		t.Fatal(err)
	}
	store := gcsObjectStore{}
	if _, err := store.Download(context.Background(), "bucket", "media/a.jpg", dest, "a.jpg"); err == nil {
		t.Fatal("Download wrote through a leaf symlink")
	} else if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("error = %v, want a symlink refusal", err)
	}
	if body, err := os.ReadFile(target); err != nil || string(body) != "secret" {
		t.Fatalf("symlink target modified: %q %v", body, err)
	}
}

func TestMediaImportRejectsFileSymlink(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.Symlink(target, filepath.Join(src, "a.jpg")); err != nil {
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
		t.Fatal("import accepted a file symlink")
	}
	if len(store.uploads) != 0 {
		t.Fatalf("symlink target uploaded: %#v", store.uploads)
	}
}

func TestMediaImportRejectsParentDirectorySymlink(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(src, "catalog")); err != nil {
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
		t.Fatal("import accepted a directory symlink")
	}
	if len(store.uploads) != 0 {
		t.Fatalf("symlinked directory uploaded: %#v", store.uploads)
	}
}

func TestMediaImportAcceptsDottedNames(t *testing.T) {
	t.Parallel()
	store := &fakeObjectStore{}
	server := &Server{NewMediaStore: func() objectStore { return store }}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "photo..jpg"), []byte("dots"), 0o644); err != nil {
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
	if result.FileCount != 1 || string(store.uploads["media/photo..jpg"]) != "dots" {
		t.Fatalf("result = %+v uploads = %#v", result, store.uploads)
	}
}
