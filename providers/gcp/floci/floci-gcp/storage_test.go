//go:build floci_gcp

package flocigcp_test

import (
	"context"
	"io"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func TestObjectWriteAndReadAgainstFlociGCP(t *testing.T) {
	host := requireFlociGCP(t)
	t.Setenv("STORAGE_EMULATOR_HOST", host)
	t.Setenv("GOOGLE_CLOUD_PROJECT", "floci-local")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := storage.NewClient(ctx, option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	bucketName := "magelift-floci-gcp-media"
	bucket := client.Bucket(bucketName)
	if err := bucket.Create(ctx, "floci-local", nil); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	defer func() {
		it := bucket.Objects(context.Background(), nil)
		for {
			attrs, listErr := it.Next()
			if listErr == iterator.Done {
				break
			}
			if listErr != nil {
				return
			}
			_ = bucket.Object(attrs.Name).Delete(context.Background())
		}
		_ = bucket.Delete(context.Background())
	}()

	writer := bucket.Object("media/catalog.json").NewWriter(ctx)
	if _, err := io.WriteString(writer, "media-v1"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := bucket.Object("media/catalog.json").NewReader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "media-v1" {
		t.Fatalf("got %q, want media-v1", content)
	}
}
