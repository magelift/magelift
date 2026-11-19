//go:build floci

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/platform"
	azurestate "github.com/magelift/magelift/providers/azure/state"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestAzureFlociBootstrapCreatesStateContainer(t *testing.T) {
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
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "test-only-passphrase")
	server := &Server{blobFactory: func(context.Context, storedPlan) (*azblob.Client, error) { return client, nil }}
	request := testPlanRequest()
	request.AccountOnly = true
	request.Services = nil
	request.TargetBlock = []byte("subscriptionId: sub\nresourceGroup: group\ntenantId: tenant\n")
	var planned sdk.PlanResult
	if err := server.Plan(request, &planned); err != nil || planned.Error != nil {
		t.Fatalf("account plan: %v / %v", err, planned.Error)
	}
	container := strings.TrimPrefix(planned.Plan.StateBackendURL, "azblob://")
	t.Cleanup(func() { _, _ = client.DeleteContainer(context.Background(), container, nil) })
	var verified sdk.BootstrapVerifyResult
	if err := server.BootstrapVerify(&sdk.BootstrapVerifyCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan}, &verified); err != nil || verified.Error != nil || !verified.Verified {
		t.Fatalf("verify = %#v, error = %v, transport = %v", verified, verified.Error, err)
	}
	call := &sdk.BootstrapEnsureCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan}
	for attempt := 0; attempt < 2; attempt++ {
		var ensured sdk.BootstrapEnsureResult
		if err := server.BootstrapEnsure(call, &ensured); err != nil || ensured.Error != nil {
			t.Fatalf("ensure %d = %#v, %v", attempt, ensured, err)
		}
		var result platform.BootstrapResult
		if err := json.Unmarshal(ensured.ResultJSON, &result); err != nil || result.BackendURL != planned.Plan.StateBackendURL {
			t.Fatalf("result = %#v, %v", result, err)
		}
	}
}

func TestAzureFlociPreviewSweepUsesSharedContainer(t *testing.T) {
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
	t.Setenv("AZURE_STORAGE_CONNECTION_STRING", connection)
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "test-only-passphrase")
	t.Setenv("PULUMI_HOME", t.TempDir())
	server := &Server{stateFactory: func(_ context.Context, plan storedPlan) (*azurestate.Manager, error) {
		return azurestate.NewManager(client, strings.TrimPrefix(plan.BackendURL, "azblob://"), plan.Spec.Project, plan.Spec.Environment)
	}}
	base := testPlanRequest()
	base.TargetBlock = []byte("subscriptionId: sub\nresourceGroup: group\ntenantId: tenant\n")
	base.AccountOnly = true
	var account sdk.PlanResult
	if err := server.Plan(base, &account); err != nil || account.Error != nil {
		t.Fatalf("base plan: %v %v", err, account.Error)
	}
	container := strings.TrimPrefix(account.Plan.StateBackendURL, "azblob://")
	ctx := context.Background()
	if _, err := client.CreateContainer(ctx, container, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = client.DeleteContainer(context.Background(), container, nil) })
	var unlocks []sdk.StateUnlockCall
	for i := 1; i <= 2; i++ {
		request := testPlanRequest()
		request.TargetBlock = base.TargetBlock
		request.StateEnvironment = "preview"
		request.AllowExpiredPreview = true
		request.Envelope.Environment = fmt.Sprintf("preview-pr-%d", i)
		request.Envelope.StackName = fmt.Sprintf("shop-preview-pr-%d-azure-aks", i)
		var planned sdk.PlanResult
		if err := server.Plan(request, &planned); err != nil || planned.Error != nil {
			t.Fatalf("PR plan: %v %v", err, planned.Error)
		}
		if planned.Plan.StateBackendURL != account.Plan.StateBackendURL {
			t.Fatalf("unexpected backend %s", planned.Plan.StateBackendURL)
		}
		var locked sdk.StateLockResult
		lockCall := &sdk.StateLockCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan, Owner: fmt.Sprintf("owner-%d", i)}
		if err := server.StateLock(lockCall, &locked); err != nil || locked.Error != nil || !locked.Locked {
			t.Fatalf("preview lock: %v %#v", err, locked)
		}
		unlocks = append(unlocks, sdk.StateUnlockCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan})
		stack, err := automation.NewInlineStackWithBackend(ctx, planned.Plan.StackName, func(*pulumi.Context) error { return nil }, planned.Plan.StateBackendURL)
		if err != nil {
			t.Fatalf("create test stack: %v", err)
		}
		metadata := automation.PreviewMetadata{Project: "shop", Repository: "example/shop", PullRequest: int64(i), Environment: request.Envelope.Environment, StackKey: planned.Plan.StackName, Owner: "test", Generation: 1, ExpiresAt: "2020-01-01T00:00:00Z"}
		payload, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		if err := stack.SetTag(ctx, "magelift:preview:identity", string(payload)); err != nil {
			t.Fatalf("set preview tag: %v", err)
		}
	}
	records, err := automation.ListPreviewRecords(ctx, account.Plan.StateBackendURL, "shop")
	if err != nil {
		t.Fatalf("discover previews in bootstrapped container: %v", err)
	}
	if len(records) != 2 || records[0].StackName == records[1].StackName {
		t.Fatalf("discovered previews: %#v", records)
	}
	for _, call := range unlocks {
		var result sdk.StateUnlockResult
		if err := server.StateUnlock(&call, &result); err != nil || result.Error != nil {
			t.Fatalf("preview unlock: %v %#v", err, result)
		}
	}
}
