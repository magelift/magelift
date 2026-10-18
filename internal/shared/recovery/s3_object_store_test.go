package recovery

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type fakeS3ObjectAPI struct {
	pages       []S3ObjectPage
	listTokens  []string
	putOptions  []S3ObjectWriteOptions
	copyOptions []S3ObjectWriteOptions
	putMetadata []map[string]string
}

func (fake *fakeS3ObjectAPI) List(_ context.Context, _, _ string, token string) (S3ObjectPage, error) {
	fake.listTokens = append(fake.listTokens, token)
	if len(fake.pages) == 0 {
		return S3ObjectPage{}, errors.New("no fake page")
	}
	page := fake.pages[0]
	fake.pages = fake.pages[1:]
	return page, nil
}

func (fake *fakeS3ObjectAPI) Head(context.Context, string, string) (ObjectMetadata, error) {
	return ObjectMetadata{}, nil
}

func (fake *fakeS3ObjectAPI) Read(context.Context, string, string) ([]byte, error) {
	return nil, nil
}

func (fake *fakeS3ObjectAPI) Put(_ context.Context, _, _ string, _ []byte, metadata map[string]string, options S3ObjectWriteOptions) error {
	fake.putMetadata = append(fake.putMetadata, metadata)
	fake.putOptions = append(fake.putOptions, options)
	return nil
}

func (fake *fakeS3ObjectAPI) Copy(_ context.Context, _, _, _, _ string, _ map[string]string, options S3ObjectWriteOptions) error {
	fake.copyOptions = append(fake.copyOptions, options)
	return nil
}

func TestS3ObjectStorePaginatesAndRejectsStalledPages(t *testing.T) {
	fake := &fakeS3ObjectAPI{pages: []S3ObjectPage{
		{Objects: []ObjectInfo{{Key: "one", Size: 1}}, Truncated: true, NextContinuationToken: "next"},
		{Objects: []ObjectInfo{{Key: "two", Size: 2}}},
	}}
	store, err := NewS3ObjectStore(fake, S3ObjectWriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	objects, err := store.List(context.Background(), "bucket", "prefix/")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(objects), 2; got != want {
		t.Fatalf("got %d objects, want %d", got, want)
	}
	if want := []string{"", "next"}; !reflect.DeepEqual(fake.listTokens, want) {
		t.Fatalf("got continuation tokens %v, want %v", fake.listTokens, want)
	}

	stalled := &fakeS3ObjectAPI{pages: []S3ObjectPage{{Truncated: true, NextContinuationToken: ""}}}
	stalledStore, err := NewS3ObjectStore(stalled, S3ObjectWriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stalledStore.List(context.Background(), "bucket", "prefix/"); err == nil {
		t.Fatal("expected stalled pagination error")
	}
}

func TestS3ObjectStoreCopiesMetadataAndWritePolicy(t *testing.T) {
	retainUntil := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	options := S3ObjectWriteOptions{Encryption: "kms", EncryptionKey: "key-1", ObjectLockMode: "COMPLIANCE", RetainUntil: retainUntil}
	fake := &fakeS3ObjectAPI{}
	store, err := NewS3ObjectStore(fake, options)
	if err != nil {
		t.Fatal(err)
	}
	metadata := map[string]string{"owner": "marker"}
	if err := store.Put(context.Background(), "bucket", "manifest", []byte("body"), metadata); err != nil {
		t.Fatal(err)
	}
	metadata["owner"] = "mutated"
	if err := store.Copy(context.Background(), "source", "key", "bucket", "copy", metadata); err != nil {
		t.Fatal(err)
	}
	if got, want := fake.putMetadata[0]["owner"], "marker"; got != want {
		t.Fatalf("put metadata was aliased: got %q, want %q", got, want)
	}
	if !reflect.DeepEqual(fake.putOptions, []S3ObjectWriteOptions{options}) || !reflect.DeepEqual(fake.copyOptions, []S3ObjectWriteOptions{options}) {
		t.Fatalf("write policy was not forwarded: put=%v copy=%v", fake.putOptions, fake.copyOptions)
	}
}

func TestNewS3ObjectStoreRejectsUnsafeWriteOptions(t *testing.T) {
	fake := &fakeS3ObjectAPI{}
	if _, err := NewS3ObjectStore(fake, S3ObjectWriteOptions{Encryption: "bad\nvalue"}); err == nil {
		t.Fatal("expected control-data rejection")
	}
}
