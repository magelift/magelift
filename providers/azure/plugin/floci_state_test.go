//go:build floci_azure

package plugin

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	azurestate "github.com/magelift/magelift/providers/azure/state"
	"github.com/magelift/magelift/sdk"
)

func TestAzureFlociPluginStateOperations(t *testing.T) {
	if os.Getenv("MAGELIFT_FLOCI_AZURE") != "1" {
		t.Skip("Azure emulator is required")
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
	container := fmt.Sprintf("magelift-plugin-%d", time.Now().UnixNano())
	if _, err := client.CreateContainer(ctx, container, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = client.DeleteContainer(context.Background(), container, nil) })
	server := &Server{stateFactory: func(_ context.Context, plan storedPlan) (*azurestate.Manager, error) {
		return azurestate.NewManager(client, container, plan.Spec.Project, plan.Spec.Environment)
	}}
	request := testPlanRequest()
	var planned sdk.PlanResult
	if err := server.Plan(request, &planned); err != nil || planned.Error != nil {
		t.Fatalf("plan: %v / %v", err, planned.Error)
	}
	lockCall := &sdk.StateLockCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan, Owner: "test-owner"}
	var locked sdk.StateLockResult
	if err := server.StateLock(lockCall, &locked); err != nil || locked.Error != nil || !locked.Locked {
		t.Fatalf("lock = %#v, %v", locked, err)
	}
	var second sdk.StateLockResult
	if err := server.StateLock(lockCall, &second); err != nil || second.Error == nil || second.Error.Code != sdk.ErrCodeConflict {
		t.Fatalf("second lock = %#v, %v", second, err)
	}
	var status sdk.StateStatusResult
	if err := server.StateStatus(&sdk.StateStatusCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan}, &status); err != nil || status.Error != nil || !status.Locked || !strings.Contains(string(status.InfoJSON), "test-owner") {
		t.Fatalf("status = %#v, %v", status, err)
	}
	var unlocked sdk.StateUnlockResult
	if err := server.StateUnlock(&sdk.StateUnlockCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan}, &unlocked); err != nil || unlocked.Error != nil {
		t.Fatalf("unlock = %#v, %v", unlocked, err)
	}
	status = sdk.StateStatusResult{}
	if err := server.StateStatus(&sdk.StateStatusCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan}, &status); err != nil || status.Error != nil || status.Locked {
		t.Fatalf("unlocked status = %#v, %v", status, err)
	}
}

func TestAzureFlociStaleHandleDoesNotDeleteSuccessor(t *testing.T) {
	if os.Getenv("MAGELIFT_FLOCI_AZURE") != "1" {
		t.Skip("Azure emulator is required")
	}
	endpoint := os.Getenv("MAGELIFT_AZURE_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:4577"
	}
	connection := "DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=" + strings.TrimRight(endpoint, "/") + "/devstoreaccount1;"
	client, err := azblob.NewClientFromConnectionString(connection, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	container := fmt.Sprintf("magelift-stale-%d", time.Now().UnixNano())
	if _, err := client.CreateContainer(ctx, container, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = client.DeleteContainer(context.Background(), container, nil) })
	request := testPlanRequest()
	server := &Server{stateFactory: func(_ context.Context, plan storedPlan) (*azurestate.Manager, error) {
		return azurestate.NewManager(client, container, plan.Spec.Project, plan.Spec.Environment)
	}}
	var plan sdk.PlanResult
	if err := server.Plan(request, &plan); err != nil || plan.Error != nil {
		t.Fatalf("plan: %v %#v", err, plan.Error)
	}
	call := &sdk.StateLockCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: plan.Plan, Owner: "first"}
	var locked sdk.StateLockResult
	if err := server.StateLock(call, &locked); err != nil || locked.Error != nil {
		t.Fatalf("first lock: %v %#v", err, locked)
	}
	manager, err := azurestate.NewManager(client, container, "shop", "preview")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Unlock(ctx); err != nil {
		t.Fatal(err)
	}
	successor, err := manager.Acquire(ctx, "successor")
	if err != nil {
		t.Fatal(err)
	}
	unlock := &sdk.StateUnlockCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: plan.Plan}
	var unlocked sdk.StateUnlockResult
	if err := server.StateUnlock(unlock, &unlocked); err != nil || unlocked.Error == nil || unlocked.Error.Code != sdk.ErrCodeConflict {
		t.Fatalf("stale release: %v %#v", err, unlocked)
	}
	status, err := manager.Status(ctx)
	if err != nil || status.Owner != "successor" {
		t.Fatalf("successor changed: %v %#v", err, status)
	}
	if err := successor.Release(ctx); err != nil {
		t.Fatal(err)
	}
	locked = sdk.StateLockResult{}
	if err := server.StateLock(call, &locked); err != nil || locked.Error != nil {
		t.Fatalf("reacquire after stale handle: %v %#v", err, locked)
	}
	if _, err := manager.Unlock(ctx); err != nil {
		t.Fatal(err)
	}
	unlocked = sdk.StateUnlockResult{}
	if err := server.StateUnlock(unlock, &unlocked); err != nil || unlocked.Error != nil {
		t.Fatalf("missing lock cleanup: %v %#v", err, unlocked)
	}
	locked = sdk.StateLockResult{}
	if err := server.StateLock(call, &locked); err != nil || locked.Error != nil {
		t.Fatalf("reacquire after remote deletion: %v %#v", err, locked)
	}
}
