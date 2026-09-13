package resilience

import (
	"context"
	"errors"
	"strings"
	"testing"

	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type scalewayProjectionBackend struct{}

func (scalewayProjectionBackend) Rebuild(_ context.Context, request cloudrecovery.ProjectionRequest) (cloudrecovery.ProjectionResult, error) {
	return scalewayProjectionResult(request), nil
}

func (scalewayProjectionBackend) Verify(_ context.Context, request cloudrecovery.ProjectionRequest) (cloudrecovery.ProjectionResult, error) {
	return scalewayProjectionResult(request), nil
}

func scalewayProjectionResult(request cloudrecovery.ProjectionRequest) cloudrecovery.ProjectionResult {
	return cloudrecovery.ProjectionResult{
		Status:                   sdk.ResilienceOperationSucceeded,
		OperationID:              "scaleway-runtime-projection-1",
		ResourceReference:        request.TargetReference,
		FixtureID:                request.FixtureID,
		OwnershipMarker:          request.OwnershipMarker,
		IdempotencyVerified:      true,
		ApplicationReadsVerified: true,
		PermissionsVerified:      true,
		SecretReferencesVerified: true,
		ServiceHealthVerified:    true,
		CacheLossClassified:      request.DataClass == cloudrecovery.ProjectionCache,
		RestoreDurationSeconds:   1,
		Reason:                   "Scaleway workload projection proof completed",
	}
}

func TestNativeAPIScalewayCacheProjectionUsesSharedLifecycle(t *testing.T) {
	t.Parallel()
	lifecycle, err := cloudrecovery.NewProjectionLifecycle(scalewayProjectionBackend{})
	if err != nil {
		t.Fatal(err)
	}
	native, err := NewNativeAPI(newFakeScalewayS3(), NativeAPIConfig{
		ArchiveBucket: "archive", Projection: lifecycle,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceRestore, DataClasses: []string{cloudrecovery.ProjectionCache},
		Destination: sdk.RecoverySameRegionIsolated, FixtureID: "fixture/cache",
		OwnershipMarker: "magelift/test/scaleway/projection", IdempotencyKey: "cache/restore/1",
		ResourceReferences: map[string]string{cloudrecovery.ProjectionCache: "scaleway-redis://cache/source"},
	}
	operation, err := scalewayOperationName(request)
	if err != nil {
		t.Fatal(err)
	}
	started, err := native.Start(context.Background(), cloudresilience.NativeOperationRequest{
		Provider: "scaleway", Operation: operation, Request: request,
	})
	if err != nil {
		t.Fatalf("start cache projection: %v", err)
	}
	if started.Status != string(sdk.ResilienceOperationSucceeded) || !started.OwnershipVerified || !started.IdempotencyVerified {
		t.Fatalf("started observation = %#v", started)
	}
	if len(started.Evidence) != 1 || started.Evidence[0].DataClass != cloudrecovery.ProjectionCache || started.Evidence[0].RetentionDays != 0 {
		t.Fatalf("started evidence = %#v", started.Evidence)
	}
	resumed, err := native.Poll(context.Background(), started.OperationID)
	if err != nil {
		t.Fatalf("poll cache projection: %v", err)
	}
	if resumed.Status != string(sdk.ResilienceOperationSucceeded) || resumed.OperationID != started.OperationID {
		t.Fatalf("resumed observation = %#v", resumed)
	}
}

func TestNativeAPIScalewayProjectionRequiresInjectedAdapter(t *testing.T) {
	t.Parallel()
	native, err := NewNativeAPI(newFakeScalewayS3(), NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceRestore, DataClasses: []string{cloudrecovery.ProjectionCache},
		Destination: sdk.RecoverySameRegionIsolated, FixtureID: "fixture/cache",
		OwnershipMarker: "magelift/test/scaleway/projection", IdempotencyKey: "cache/restore/2",
		ResourceReferences: map[string]string{cloudrecovery.ProjectionCache: "scaleway-redis://cache/source"},
	}
	operation, err := scalewayOperationName(request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), cloudresilience.NativeOperationRequest{Provider: "scaleway", Operation: operation, Request: request})
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported || !strings.Contains(capability.Reason, "injected Kapsule") {
		t.Fatalf("error = %v, want typed projection capability refusal", err)
	}
}
