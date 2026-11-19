//go:build floci_azure

package floci_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/magelift/magelift/providers/azure/storage"
)

func TestBlobLifecycle(t *testing.T) {
	if os.Getenv("MAGELIFT_FLOCI_AZURE") != "1" {
		t.Skip("set MAGELIFT_FLOCI_AZURE=1 to run against the Azure emulator")
	}
	endpoint := os.Getenv("MAGELIFT_AZURE_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:4577"
	}
	connection := "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;" +
		"AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;" +
		"BlobEndpoint=" + strings.TrimRight(endpoint, "/") + "/devstoreaccount1;"
	client, err := azblob.NewClientFromConnectionString(connection, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	container := fmt.Sprintf("magelift-%d", time.Now().UnixNano())
	if _, err := client.CreateContainer(ctx, container, nil); err != nil {
		t.Fatalf("create container: %v", err)
	}
	t.Cleanup(func() { _, _ = client.DeleteContainer(context.Background(), container, nil) })
	store, err := storage.NewBlobStoreWithConnectionString(connection, container)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "catalog/product/example.txt", strings.NewReader("media-v1")); err != nil {
		t.Fatal(err)
	}
	body, err := store.Open(ctx, "catalog/product/example.txt")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil || string(data) != "media-v1" {
		t.Fatalf("read media = %q, %v", data, err)
	}
	if err := store.Delete(ctx, "catalog/product/example.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(ctx, "catalog/product/example.txt"); err == nil {
		t.Fatal("deleted object remains readable")
	}
}
