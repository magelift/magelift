package resilience

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	cloudresilience "github.com/magelift/magelift/internal/shared/resilience"
	"github.com/magelift/magelift/sdk"
)

type ovhProjectionBackend struct{}

func (ovhProjectionBackend) Rebuild(_ context.Context, request cloudrecovery.ProjectionRequest) (cloudrecovery.ProjectionResult, error) {
	return ovhProjectionResult(request), nil
}

func (ovhProjectionBackend) Verify(_ context.Context, request cloudrecovery.ProjectionRequest) (cloudrecovery.ProjectionResult, error) {
	return ovhProjectionResult(request), nil
}

func ovhProjectionResult(request cloudrecovery.ProjectionRequest) cloudrecovery.ProjectionResult {
	return cloudrecovery.ProjectionResult{
		Status:                   sdk.ResilienceOperationSucceeded,
		OperationID:              "ovh-runtime-projection-1",
		ResourceReference:        request.TargetReference,
		FixtureID:                request.FixtureID,
		OwnershipMarker:          request.OwnershipMarker,
		IdempotencyVerified:      true,
		CountsVerified:           request.DataClass == cloudrecovery.ProjectionSearchIndex,
		ApplicationReadsVerified: true,
		PermissionsVerified:      true,
		SecretReferencesVerified: true,
		ServiceHealthVerified:    true,
		CacheLossClassified:      request.DataClass == cloudrecovery.ProjectionCache,
		RestoreDurationSeconds:   1,
		Reason:                   "OVHcloud workload projection proof completed",
	}
}

func TestNativeAPIOVHSearchProjectionUsesSharedLifecycle(t *testing.T) {
	t.Parallel()
	lifecycle, err := cloudrecovery.NewProjectionLifecycle(ovhProjectionBackend{})
	if err != nil {
		t.Fatal(err)
	}
	native, err := NewNativeAPI(newFakeOVHS3(), NativeAPIConfig{
		ArchiveBucket: "archive", Projection: lifecycle,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceRestore, DataClasses: []string{cloudrecovery.ProjectionSearchIndex},
		Destination: sdk.RecoverySameRegionIsolated, FixtureID: "fixture/search",
		OwnershipMarker: "magelift/test/ovh/projection", IdempotencyKey: "search/restore/1",
		ResourceReferences: map[string]string{cloudrecovery.ProjectionSearchIndex: "ovh-search://workload/source"},
	}
	operation, err := ovhOperationName(request)
	if err != nil {
		t.Fatal(err)
	}
	started, err := native.Start(context.Background(), cloudresilience.NativeOperationRequest{
		Provider: "ovh", Operation: operation, Request: request,
	})
	if err != nil {
		t.Fatalf("start search projection: %v", err)
	}
	if started.Status != string(sdk.ResilienceOperationSucceeded) || !started.OwnershipVerified || !started.IdempotencyVerified {
		t.Fatalf("started observation = %#v", started)
	}
	if len(started.Evidence) != 1 || !started.Evidence[0].CountsVerified || started.Evidence[0].RetentionDays != 0 {
		t.Fatalf("started evidence = %#v", started.Evidence)
	}
	resumed, err := native.Poll(context.Background(), started.OperationID)
	if err != nil {
		t.Fatalf("poll search projection: %v", err)
	}
	if resumed.Status != string(sdk.ResilienceOperationSucceeded) || resumed.OperationID != started.OperationID {
		t.Fatalf("resumed observation = %#v", resumed)
	}
}

func TestNativeAPIOVHProjectionRequiresInjectedAdapter(t *testing.T) {
	t.Parallel()
	native, err := NewNativeAPI(newFakeOVHS3(), NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceRestore, DataClasses: []string{cloudrecovery.ProjectionSearchIndex},
		Destination: sdk.RecoverySameRegionIsolated, FixtureID: "fixture/search",
		OwnershipMarker: "magelift/test/ovh/projection", IdempotencyKey: "search/restore/2",
		ResourceReferences: map[string]string{cloudrecovery.ProjectionSearchIndex: "ovh-search://workload/source"},
	}
	operation, err := ovhOperationName(request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), cloudresilience.NativeOperationRequest{Provider: "ovh", Operation: operation, Request: request})
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported || !strings.Contains(capability.Reason, "injected MKS") {
		t.Fatalf("error = %v, want typed projection capability refusal", err)
	}
}

func TestNativeAPIOVHCacheProjectionSkipsManagedDatabaseTranslator(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		action sdk.ResilienceAction
		backup bool
	}{
		{name: "restore", action: sdk.ResilienceRestore, backup: true},
		{name: "integrity", action: sdk.ResilienceIntegrityCheck},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			lifecycle, err := cloudrecovery.NewProjectionLifecycle(ovhProjectionBackend{})
			if err != nil {
				t.Fatal(err)
			}
			database := newFakeOVHDatabase()
			database.instances["source"] = DatabaseInstance{ID: "source", Engine: "redis", Region: "GRA", Status: "READY", Encrypted: true}
			database.backups["source\x00backup"] = DatabaseBackup{ID: "backup", InstanceID: "source", CreatedAt: time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC), Status: "READY", Encrypted: true, Regions: []string{"GRA"}, RetentionDays: 14}
			storage := newFakeOVHS3()
			native, err := NewNativeAPIWithDatabase(storage, database, NativeAPIConfig{
				ArchiveBucket: "archive", DatabaseEngine: "redis", DatabaseSourceID: "source", Projection: lifecycle,
				Verifier: fakeOVHDatabaseVerifier{},
			})
			if err != nil {
				t.Fatal(err)
			}
			request := sdk.ResilienceOperationRequest{
				Action: tt.action, DataClasses: []string{cloudrecovery.ProjectionCache}, Destination: sdk.RecoverySameRegionIsolated,
				FixtureID: "fixture/cache", OwnershipMarker: "magelift/test/ovh/cache", IdempotencyKey: "cache/" + tt.name,
				ResourceReferences: map[string]string{cloudrecovery.ProjectionCache: "ovh-valkey://cache/source"},
			}
			if tt.backup {
				request.BackupReferences = map[string]string{cloudrecovery.ProjectionCache: "ovh-database-backup://source/backup"}
			}
			operation, err := ovhOperationName(request)
			if err != nil {
				t.Fatal(err)
			}
			started, err := native.Start(context.Background(), cloudresilience.NativeOperationRequest{Provider: "ovh", Operation: operation, Request: request})
			if err != nil {
				t.Fatalf("start cache %s: %v", tt.name, err)
			}
			if started.Status != string(sdk.ResilienceOperationSucceeded) || len(started.Evidence) != 1 || started.Evidence[0].DataClass != cloudrecovery.ProjectionCache {
				t.Fatalf("started cache %s observation = %#v", tt.name, started)
			}
			resumed, err := native.Poll(context.Background(), started.OperationID)
			if err != nil {
				t.Fatalf("poll cache %s: %v", tt.name, err)
			}
			if resumed.Status != string(sdk.ResilienceOperationSucceeded) || resumed.OperationID != started.OperationID {
				t.Fatalf("resumed cache %s observation = %#v", tt.name, resumed)
			}
			if database.getCalls != 0 || database.listBackupsCalls != 0 || database.createCalls != 0 || database.deleteCalls != 0 {
				t.Fatalf("cache %s used managed database translator: %#v", tt.name, database)
			}
			if len(storage.objects) != 0 || storage.copies != 0 {
				t.Fatalf("cache %s mutated object storage: %#v", tt.name, storage)
			}
		})
	}
}

func TestNativeAPIOVHCacheRecoveryWithoutProjectionRefusesBeforeDatabaseAccess(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		action     sdk.ResilienceAction
		wantReason string
		backup     bool
	}{
		{name: "backup", action: sdk.ResilienceBackup, wantReason: "source of truth", backup: false},
		{name: "restore", action: sdk.ResilienceRestore, wantReason: "injected MKS", backup: true},
		{name: "integrity", action: sdk.ResilienceIntegrityCheck, wantReason: "injected MKS", backup: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			database := newFakeOVHDatabase()
			database.instances["source"] = DatabaseInstance{ID: "source", Engine: "redis", Region: "GRA", Status: "READY", Encrypted: true}
			database.backups["source\x00backup"] = DatabaseBackup{ID: "backup", InstanceID: "source", CreatedAt: time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC), Status: "READY", Encrypted: true, Regions: []string{"GRA"}, RetentionDays: 14}
			storage := newFakeOVHS3()
			native, err := NewNativeAPIWithDatabase(storage, database, NativeAPIConfig{
				ArchiveBucket: "archive", DatabaseEngine: "redis", DatabaseSourceID: "source", Verifier: fakeOVHDatabaseVerifier{},
			})
			if err != nil {
				t.Fatal(err)
			}
			request := sdk.ResilienceOperationRequest{
				Action: tt.action, DataClasses: []string{cloudrecovery.ProjectionCache}, Destination: sdk.RecoverySameRegionIsolated,
				FixtureID: "fixture/cache", OwnershipMarker: "magelift/test/ovh/cache-unsupported", IdempotencyKey: "cache/unsupported/" + tt.name,
				ResourceReferences: map[string]string{cloudrecovery.ProjectionCache: "ovh-valkey://cache/source"},
			}
			if tt.backup {
				request.BackupReferences = map[string]string{cloudrecovery.ProjectionCache: "ovh-database-backup://source/backup"}
			}
			operation, err := ovhOperationName(request)
			if err != nil {
				t.Fatal(err)
			}
			_, err = native.Start(context.Background(), cloudresilience.NativeOperationRequest{Provider: "ovh", Operation: operation, Request: request})
			var capability sdk.ResilienceCapabilityError
			if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported || capability.Operation != tt.action || capability.DataClass != cloudrecovery.ProjectionCache || !strings.Contains(capability.Reason, tt.wantReason) {
				t.Fatalf("cache %s error = %v, want typed unsupported capability", tt.name, err)
			}
			if database.getCalls != 0 || database.listBackupsCalls != 0 || database.createCalls != 0 || database.deleteCalls != 0 {
				t.Fatalf("cache %s attempted managed database access: %#v", tt.name, database)
			}
			if len(storage.objects) != 0 || storage.copies != 0 {
				t.Fatalf("cache %s mutated object storage: %#v", tt.name, storage)
			}
		})
	}
}
