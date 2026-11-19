//go:build floci

package floci_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/magelift/magelift/providers/azure/state"
)

func TestAzureStateLockExcludesConcurrentOperations(t *testing.T) {
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
	container := fmt.Sprintf("magelift-lock-%d", time.Now().UnixNano())
	if _, err := client.CreateContainer(ctx, container, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = client.DeleteContainer(context.Background(), container, nil) })
	first, err := state.NewManager(client, container, "shop", "preview")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := state.NewManager(client, container, "shop", "preview")
	handle, err := first.Acquire(ctx, "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Acquire(ctx, "second"); !errors.Is(err, state.ErrLocked) {
		t.Fatalf("second acquisition = %v, want locked", err)
	}
	info, err := second.Status(ctx)
	if err != nil || info.Owner != "first" {
		t.Fatalf("status = %#v, %v", info, err)
	}
	if _, err := second.Unlock(ctx); err != nil {
		t.Fatal(err)
	}
	successor, err := second.Acquire(ctx, "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Release(ctx); err == nil {
		t.Fatal("stale lock handle removed the successor")
	}
	if err := successor.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Status(ctx); !errors.Is(err, state.ErrNotLocked) {
		t.Fatalf("final status = %v", err)
	}
}
