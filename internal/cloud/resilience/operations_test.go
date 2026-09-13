package resilience_test

import (
	"context"
	"strings"
	"testing"

	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	"github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type nativeOperationAPI struct {
	started    []cloudresilience.NativeOperationRequest
	startCalls int
	pollCalls  int
}

func (api *nativeOperationAPI) Start(_ context.Context, request cloudresilience.NativeOperationRequest) (provider.NativeOperationObservation, error) {
	api.startCalls++
	api.started = append(api.started, request)
	return provider.NativeOperationObservation{
		Status:              "accepted",
		Action:              request.Request.Action,
		OperationID:         "operation-1",
		OwnershipMarker:     request.Request.OwnershipMarker,
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}, nil
}

func (api *nativeOperationAPI) Poll(_ context.Context, operationID string) (provider.NativeOperationObservation, error) {
	api.pollCalls++
	return provider.NativeOperationObservation{Status: "completed", Action: sdk.ResilienceBackup, OperationID: operationID, OwnershipMarker: "magelift/test/operations", OwnershipVerified: true, IdempotencyVerified: true}, nil
}

func (api *nativeOperationAPI) Inventory(context.Context, string) ([]provider.InventoryResource, error) {
	return []provider.InventoryResource{{Identity: "owned/resource", Owned: true, Live: false}}, nil
}

func TestMappedOperationBackendKeepsProviderMappingAtTheEdge(t *testing.T) {
	api := &nativeOperationAPI{}
	backend, err := cloudresilience.NewMappedOperationBackend("gcp", api, func(request sdk.ResilienceOperationRequest) (string, error) {
		return cloudresilience.OperationName("gcp.cloud-sql", request.Action, request.DataClasses)
	})
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, FixtureID: "fixture/known-content",
		OwnershipMarker: "magelift/test/operations", IdempotencyKey: "operation/1",
		ResourceReferences: map[string]string{"database": "opaque/database"},
	}
	started, err := backend.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.ResourceReferences["database"] = "mutated-after-call"
	if len(api.started) != 1 || api.started[0].Operation != "gcp.cloud-sql.backup.database" {
		t.Fatalf("native start request = %#v", api.started)
	}
	if api.started[0].Request.ResourceReferences["database"] != "opaque/database" {
		t.Fatalf("native request was not cloned: %#v", api.started[0].Request.ResourceReferences)
	}
	if started.OperationID != "operation-1" || started.OwnershipMarker != request.OwnershipMarker {
		t.Fatalf("observation = %#v", started)
	}
	if _, err := backend.Poll(context.Background(), "operation-1"); err != nil {
		t.Fatal(err)
	}
	resources, err := backend.Inventory(context.Background(), request.OwnershipMarker)
	if err != nil || len(resources) != 1 || resources[0].Identity != "owned/resource" {
		t.Fatalf("inventory = %#v, err = %v", resources, err)
	}
}

func TestOperationNameIsDeterministicAndRejectsUnsafeInputs(t *testing.T) {
	name, err := cloudresilience.OperationName("aws.rds", sdk.ResilienceRestore, []string{"database", "media"})
	if err != nil || name != "aws.rds.restore.database+media" {
		t.Fatalf("operation name = %q, err = %v", name, err)
	}
	for _, prefix := range []string{"", "aws\nrd"} {
		if _, err := cloudresilience.OperationName(prefix, sdk.ResilienceBackup, []string{"database"}); err == nil {
			t.Fatalf("expected unsafe prefix %q to fail", prefix)
		}
	}
	if _, err := cloudresilience.OperationName("aws.rds", sdk.ResilienceBackup, nil); err == nil {
		t.Fatal("expected missing data class to fail")
	}
	if strings.Contains(name, " ") {
		t.Fatalf("operation name contains whitespace: %q", name)
	}
}

func TestMappedOperationBackendHonorsCanceledContextBeforeNativeCall(t *testing.T) {
	api := &nativeOperationAPI{}
	backend, err := cloudresilience.NewMappedOperationBackend("aws", api, func(sdk.ResilienceOperationRequest) (string, error) {
		return "aws.recovery.backup.database", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = backend.Start(ctx, sdk.ResilienceOperationRequest{Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, OwnershipMarker: "magelift/test/operations", IdempotencyKey: "operation/1"})
	if err == nil || api.startCalls != 0 {
		t.Fatalf("canceled start err = %v, calls = %d", err, api.startCalls)
	}
}
