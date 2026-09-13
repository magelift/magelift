package resilience

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestNativeAPIObjectRecoveryIsIdempotentAndResumable(t *testing.T) {
	storage := newFakeGCS()
	storage.putSource("source", "media/a.json", []byte(`{"id":"a"}`))
	storage.putSource("source", "media/b.json", []byte(`{"id":"b"}`))
	native, err := NewNativeAPI(storage, newFakeSecretAPI(), NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "restore", ArchivePrefix: "snapshots", RestorePrefix: "isolated"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"media"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/known-content", OwnershipMarker: "magelift/test/gcp", IdempotencyKey: "idempotency/media/1",
		ResourceReferences: map[string]string{"media": "gs://source/media"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || len(backup.Evidence) != 1 {
		t.Fatalf("backup observation = %#v", backup)
	}
	copies := storage.copies
	second, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("idempotent backup: %v", err)
	}
	if second.Status != sdk.ResilienceOperationSucceeded || storage.copies != copies {
		t.Fatalf("idempotent backup changed archive: %#v copies=%d want=%d", second, storage.copies, copies)
	}

	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "idempotency/media/restore/1"
	restore.BackupReferences = map[string]string{"media": backup.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || !strings.HasPrefix(restored.Evidence[0].RestoreID, "gcp-storage://restore/isolated/media/") {
		t.Fatalf("restore observation = %#v", restored)
	}
	resumed, err := client.Poll(context.Background(), restored.OperationID)
	if err != nil {
		t.Fatalf("resume restore: %v", err)
	}
	if resumed.Status != sdk.ResilienceOperationSucceeded || resumed.OperationID != restored.OperationID {
		t.Fatalf("resumed restore = %#v", resumed)
	}

	integrity := restore
	integrity.Action = sdk.ResilienceIntegrityCheck
	integrity.IdempotencyKey = "idempotency/media/integrity/1"
	checked, err := client.Start(context.Background(), integrity)
	if err != nil {
		t.Fatalf("integrity: %v", err)
	}
	if checked.Status != sdk.ResilienceOperationSucceeded || !checked.Evidence[0].ManifestVerified || !checked.Evidence[0].CountsVerified {
		t.Fatalf("integrity observation = %#v", checked)
	}
}

func TestNativeAPIObjectInPlaceRestoreOverwritesSourcePrefix(t *testing.T) {
	storage := newFakeGCS()
	storage.putSource("source", "media/a.json", []byte(`{"id":"a"}`))
	native, err := NewNativeAPI(storage, newFakeSecretAPI(), NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "restore", ArchivePrefix: "snapshots", RestorePrefix: "isolated"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"media"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/inplace", OwnershipMarker: "owner", IdempotencyKey: "media/backup/inplace",
		ResourceReferences: map[string]string{"media": "gs://source/media"}, ApprovalReference: "fence-approved",
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil || backup.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("backup = %#v err=%v", backup, err)
	}
	storage.putSource("source", "media/a.json", []byte(`{"id":"corrupt"}`))
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "media/restore/inplace"
	restore.BackupReferences = map[string]string{"media": backup.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil || restored.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("in-place restore = %#v err=%v", restored, err)
	}
	if restored.Evidence[0].RestoreID != "gcp-storage://source/media" {
		t.Fatalf("in-place restore target = %q", restored.Evidence[0].RestoreID)
	}
	body, err := storage.Read(context.Background(), "source", "media/a.json")
	if err != nil || string(body) != `{"id":"a"}` {
		t.Fatalf("in-place restore body = %q err=%v", body, err)
	}
}

func TestNativeAPIInfrastructureStateInPlaceRestoreOverwritesSourcePrefix(t *testing.T) {
	storage := newFakeGCS()
	storage.putSource("source", "state/stack.json", []byte(`{"checkpoint":"a"}`))
	native, err := NewNativeAPI(storage, newFakeSecretAPI(), NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "restore", ArchivePrefix: "snapshots", RestorePrefix: "isolated"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"infrastructure-state"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/inplace-state", OwnershipMarker: "owner", IdempotencyKey: "state/backup/inplace",
		ResourceReferences: map[string]string{"infrastructure-state": "gs://source/state"}, ApprovalReference: "fence-approved",
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil || backup.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("backup = %#v err=%v", backup, err)
	}
	storage.putSource("source", "state/stack.json", []byte(`{"checkpoint":"corrupt"}`))
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "state/restore/inplace"
	restore.BackupReferences = map[string]string{"infrastructure-state": backup.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil || restored.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("in-place restore = %#v err=%v", restored, err)
	}
	if restored.Evidence[0].DataClass != "infrastructure-state" || restored.Evidence[0].RestoreID != "gcp-storage://source/state" {
		t.Fatalf("in-place restore evidence = %#v", restored.Evidence[0])
	}
	body, err := storage.Read(context.Background(), "source", "state/stack.json")
	if err != nil || string(body) != `{"checkpoint":"a"}` {
		t.Fatalf("in-place restore body = %q err=%v", body, err)
	}
}

func TestNativeAPIAuditEvidenceInPlaceRestoreOverwritesSourcePrefix(t *testing.T) {
	storage := newFakeGCS()
	storage.putSource("source", "audit/events.jsonl", []byte("{\"event\":\"a\"}\n"))
	native, err := NewNativeAPI(storage, newFakeSecretAPI(), NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "restore", ArchivePrefix: "snapshots", RestorePrefix: "isolated"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"audit-evidence"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/inplace-audit", OwnershipMarker: "owner", IdempotencyKey: "audit/backup/inplace",
		ResourceReferences: map[string]string{"audit-evidence": "gs://source/audit"}, ApprovalReference: "fence-approved",
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil || backup.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("backup = %#v err=%v", backup, err)
	}
	storage.putSource("source", "audit/events.jsonl", []byte("{\"event\":\"corrupt\"}\n"))
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "audit/restore/inplace"
	restore.BackupReferences = map[string]string{"audit-evidence": backup.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil || restored.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("in-place restore = %#v err=%v", restored, err)
	}
	if restored.Evidence[0].DataClass != "audit-evidence" || restored.Evidence[0].RestoreID != "gcp-storage://source/audit" {
		t.Fatalf("in-place restore evidence = %#v", restored.Evidence[0])
	}
	body, err := storage.Read(context.Background(), "source", "audit/events.jsonl")
	if err != nil || string(body) != "{\"event\":\"a\"}\n" {
		t.Fatalf("in-place restore body = %q err=%v", body, err)
	}
}

func TestNativeAPIObjectInPlaceRestoreRequiresApproval(t *testing.T) {
	storage := newFakeGCS()
	storage.putSource("source", "media/a.json", []byte(`{"id":"a"}`))
	native, err := NewNativeAPI(storage, newFakeSecretAPI(), NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "restore", ArchivePrefix: "snapshots", RestorePrefix: "isolated"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"media"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/inplace-no-approval", OwnershipMarker: "owner", IdempotencyKey: "media/backup/inplace-no-approval",
		ResourceReferences: map[string]string{"media": "gs://source/media"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "media/restore/inplace-no-approval"
	restore.BackupReferences = map[string]string{"media": backup.Evidence[0].BackupID}
	_, err = client.Start(context.Background(), restore)
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("error = %v, want unsupported capability", err)
	}
}

func TestNativeAPIObjectCleanupRemovesOwnedArchiveAndRestore(t *testing.T) {
	storage := newFakeGCS()
	storage.putSource("source", "media/a.json", []byte(`{"id":"a"}`))
	native, err := NewNativeAPI(storage, nil, NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "restore", ArchivePrefix: "snapshots", RestorePrefix: "isolated"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"media"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/cleanup", OwnershipMarker: "magelift/test/gcp-cleanup", IdempotencyKey: "cleanup/media/backup",
		ResourceReferences: map[string]string{"media": "gs://source/media"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil || backup.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("backup = %#v err=%v", backup, err)
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "cleanup/media/restore"
	restore.BackupReferences = map[string]string{"media": backup.Evidence[0].BackupID}
	if restored, restoreErr := client.Start(context.Background(), restore); restoreErr != nil || restored.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("restore = %#v err=%v", restored, restoreErr)
	}
	cleanup := request
	cleanup.Action = sdk.ResilienceCleanup
	cleanup.IdempotencyKey = "cleanup/media/cleanup"
	cleaned, err := client.Start(context.Background(), cleanup)
	if err != nil || cleaned.Status != sdk.ResilienceOperationSucceeded || len(cleaned.ResourceRefs) == 0 {
		t.Fatalf("cleanup = %#v err=%v", cleaned, err)
	}
	if _, err := storage.Head(context.Background(), "source", "media/a.json"); err != nil {
		t.Fatalf("cleanup deleted source object: %v", err)
	}
	owned, err := native.Inventory(context.Background(), request.OwnershipMarker)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 0 {
		t.Fatalf("owned recovery objects remain: %#v", owned)
	}
}

func TestNativeAPISecretRecoveryAndCleanupPreserveSource(t *testing.T) {
	ctx := context.Background()
	storage := newFakeGCS()
	secrets := newFakeSecretAPI()
	marker := "magelift/test/gcp-secret"
	project := "demo"
	source := "projects/" + project + "/secrets/source-secret"
	secrets.secrets[source] = fakeSecret{value: []byte("synthetic-secret"), labels: secretManagerLabelsFor(cloudrecovery.OperationState{OwnershipMarker: marker, DataClass: "configuration-secrets"})}
	native, err := NewNativeAPI(storage, secrets, NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "archive", Project: project, ArchivePrefix: "snapshots", RestorePrefix: "isolated"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"configuration-secrets"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/known-secret", OwnershipMarker: marker, IdempotencyKey: "live/configuration-secrets/backup",
		ResourceReferences: map[string]string{"configuration-secrets": "gcp-secret-manager://" + source},
	}
	backup, err := client.Start(ctx, request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || backup.Evidence[0].BackupID == "" {
		t.Fatalf("backup = %#v", backup)
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "live/configuration-secrets/restore"
	restore.BackupReferences = map[string]string{"configuration-secrets": backup.Evidence[0].BackupID}
	restored, err := client.Start(ctx, restore)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || restored.Evidence[0].RestoreID == "" {
		t.Fatalf("restore = %#v", restored)
	}
	if !bytes.Equal(secrets.secrets[strings.TrimPrefix(restored.Evidence[0].RestoreID, "gcp-secret-manager://")].value, secrets.secrets[source].value) {
		t.Fatal("restored secret value differs from source")
	}
	owned, err := native.Inventory(ctx, marker)
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	if len(owned) != 2 {
		t.Fatalf("owned inventory = %#v", owned)
	}
	cleanup := request
	cleanup.Action = sdk.ResilienceCleanup
	cleanup.IdempotencyKey = "live/configuration-secrets/cleanup"
	result, err := client.Start(ctx, cleanup)
	if err != nil || result.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("cleanup = %#v err=%v", result, err)
	}
	remaining, err := native.Inventory(ctx, marker)
	if err != nil {
		t.Fatalf("inventory after cleanup: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("owned outputs remain after cleanup: %#v", remaining)
	}
	if _, ok := secrets.secrets[source]; !ok {
		t.Fatal("cleanup deleted source secret")
	}
}

func TestNativeAPISecretRecoveryDoesNotExposeValue(t *testing.T) {
	secrets := newFakeSecretAPI()
	secrets.secrets["projects/demo/secrets/app"] = fakeSecret{value: []byte("do-not-log"), labels: secretManagerLabelsFor(cloudrecovery.OperationState{OwnershipMarker: "owner", DataClass: "configuration-secrets"})}
	native, err := NewNativeAPI(newFakeGCS(), secrets, NativeAPIConfig{ArchiveBucket: "archive", Project: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"configuration-secrets"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/secret", OwnershipMarker: "owner", IdempotencyKey: "secret/backup/1",
		ResourceReferences: map[string]string{"configuration-secrets": "gcp-secret-manager://projects/demo/secrets/app"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	for _, evidence := range backup.Evidence {
		if strings.Contains(evidence.Reason, "do-not-log") || strings.Contains(evidence.BackupID, "do-not-log") {
			t.Fatalf("secret value leaked in evidence: %#v", evidence)
		}
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "secret/restore/1"
	restore.BackupReferences = map[string]string{"configuration-secrets": backup.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || len(secrets.created) != 1 {
		t.Fatalf("restore = %#v created=%v", restored, secrets.created)
	}
	if !bytes.Equal(secrets.secrets[restored.Evidence[0].RestoreID[len("gcp-secret-manager://"):]].value, []byte("do-not-log")) {
		t.Fatalf("restored secret value was not preserved")
	}
}

func TestNativeAPISecretInPlaceRestoreOverwritesSource(t *testing.T) {
	ctx := context.Background()
	storage := newFakeGCS()
	secrets := newFakeSecretAPI()
	marker := "magelift/test/gcp-secret-inplace"
	project := "demo"
	source := "projects/" + project + "/secrets/source-secret"
	original := []byte("synthetic-secret")
	secrets.secrets[source] = fakeSecret{value: append([]byte(nil), original...), labels: secretManagerLabelsFor(cloudrecovery.OperationState{OwnershipMarker: marker, DataClass: "configuration-secrets"})}
	native, err := NewNativeAPI(storage, secrets, NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "archive", Project: project, ArchivePrefix: "snapshots"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"configuration-secrets"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/known-secret", OwnershipMarker: marker, IdempotencyKey: "live/configuration-secrets/backup",
		ResourceReferences: map[string]string{"configuration-secrets": "gcp-secret-manager://" + source}, ApprovalReference: "fence-approved",
	}
	backup, err := client.Start(ctx, request)
	if err != nil || backup.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("backup = %#v err=%v", backup, err)
	}
	if err := secrets.AddVersion(ctx, source, []byte("corrupt")); err != nil {
		t.Fatal(err)
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "live/configuration-secrets/restore"
	restore.BackupReferences = map[string]string{"configuration-secrets": backup.Evidence[0].BackupID}
	restored, err := client.Start(ctx, restore)
	if err != nil || restored.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("in-place restore = %#v err=%v", restored, err)
	}
	if restored.Evidence[0].RestoreID != "gcp-secret-manager://"+source {
		t.Fatalf("in-place restore target = %q", restored.Evidence[0].RestoreID)
	}
	if len(secrets.created) != 0 {
		t.Fatalf("in-place restore created isolated secrets: %v", secrets.created)
	}
	if !bytes.Equal(secrets.secrets[source].value, original) {
		t.Fatalf("in-place restore value = %q", secrets.secrets[source].value)
	}
	owned, err := native.Inventory(ctx, marker)
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	for _, resource := range owned {
		if strings.HasPrefix(resource.Identity, "gcp-secret-manager://") {
			t.Fatalf("in-place restore left an isolated recovery secret: %#v", owned)
		}
	}
	if len(owned) == 0 {
		t.Fatal("archive inventory was empty after in-place restore")
	}
	cleanup := request
	cleanup.Action = sdk.ResilienceCleanup
	cleanup.IdempotencyKey = "live/configuration-secrets/cleanup"
	if result, err := client.Start(ctx, cleanup); err != nil || result.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("cleanup = %#v err=%v", result, err)
	}
	if _, ok := secrets.secrets[source]; !ok {
		t.Fatal("cleanup deleted source secret")
	}
}

func TestNativeAPISecretInPlaceRestoreRequiresApproval(t *testing.T) {
	ctx := context.Background()
	secrets := newFakeSecretAPI()
	source := "projects/demo/secrets/source-secret"
	secrets.secrets[source] = fakeSecret{value: []byte("synthetic-secret"), labels: secretManagerLabelsFor(cloudrecovery.OperationState{OwnershipMarker: "owner", DataClass: "configuration-secrets"})}
	native, err := NewNativeAPI(newFakeGCS(), secrets, NativeAPIConfig{ArchiveBucket: "archive", Project: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"configuration-secrets"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/inplace-no-approval", OwnershipMarker: "owner", IdempotencyKey: "secret/backup/inplace-no-approval",
		ResourceReferences: map[string]string{"configuration-secrets": "gcp-secret-manager://" + source},
	}
	backup, err := client.Start(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "secret/restore/inplace-no-approval"
	restore.BackupReferences = map[string]string{"configuration-secrets": backup.Evidence[0].BackupID}
	_, err = client.Start(ctx, restore)
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("error = %v, want unsupported capability", err)
	}
}

func TestNativeAPISecretInPlaceRestoreRecreatesMissingSource(t *testing.T) {
	ctx := context.Background()
	secrets := newFakeSecretAPI()
	marker := "owner"
	source := "projects/demo/secrets/source-secret"
	original := []byte("synthetic-secret")
	secrets.secrets[source] = fakeSecret{value: append([]byte(nil), original...), labels: secretManagerLabelsFor(cloudrecovery.OperationState{OwnershipMarker: marker, DataClass: "configuration-secrets"})}
	native, err := NewNativeAPI(newFakeGCS(), secrets, NativeAPIConfig{ArchiveBucket: "archive", Project: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"configuration-secrets"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/recreate", OwnershipMarker: marker, IdempotencyKey: "secret/backup/recreate",
		ResourceReferences: map[string]string{"configuration-secrets": "gcp-secret-manager://" + source}, ApprovalReference: "fence-approved",
	}
	backup, err := client.Start(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := secrets.Delete(ctx, source); err != nil {
		t.Fatal(err)
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "secret/restore/recreate"
	restore.BackupReferences = map[string]string{"configuration-secrets": backup.Evidence[0].BackupID}
	restored, err := client.Start(ctx, restore)
	if err != nil || restored.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("recreate restore = %#v err=%v", restored, err)
	}
	if restored.Evidence[0].RestoreID != "gcp-secret-manager://"+source {
		t.Fatalf("recreate restore target = %q", restored.Evidence[0].RestoreID)
	}
	if !bytes.Equal(secrets.secrets[source].value, original) {
		t.Fatalf("recreated secret value = %q", secrets.secrets[source].value)
	}
}

func TestNativeAPICloudSQLBackupRestoreAndIntegrityPoll(t *testing.T) {
	sql := newFakeCloudSQL()
	sql.instances["demo\x00orders"] = CloudSQLInstance{Project: "demo", Name: "orders", State: "RUNNABLE", Region: "europe-west1", DatabaseVersion: "POSTGRES_17", Tier: "db-custom-2-7680", AvailabilityType: "REGIONAL", DeletionProtection: true, BackupsEnabled: true, UserLabels: secretLabelsFor(cloudrecoveryState("fixture/sql", "owner"))}
	verifier := fakeRecoveryVerifier{}
	native, err := NewNativeAPIWithSQL(newFakeGCS(), newFakeSecretAPI(), sql, NativeAPIConfig{ArchiveBucket: "archive", Project: "demo", RestoreProject: "recovery-project", RestoreRegion: "europe-west4", RequireRegionalHA: true, RequireDeletionProtect: true, Verifier: verifier})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoveryAlternateRegion, FixtureID: "fixture/sql", OwnershipMarker: "owner", IdempotencyKey: "sql/backup/1", ResourceReferences: map[string]string{"database": "gcp-cloud-sql://projects/demo/instances/orders"}}
	backup, err := client.Start(context.Background(), request)
	if err != nil || backup.Status != sdk.ResilienceOperationPending {
		t.Fatalf("backup start = %#v err=%v", backup, err)
	}
	completed, err := client.Poll(context.Background(), backup.OperationID)
	if err != nil || completed.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("backup poll = %#v err=%v", completed, err)
	}
	if completed.Evidence[0].ManifestVerified || completed.Evidence[0].CountsVerified || completed.Evidence[0].ApplicationReadsVerified || completed.Evidence[0].PermissionsVerified || completed.Evidence[0].SecretReferencesVerified {
		t.Fatalf("Cloud SQL backup control-plane observation claimed application evidence: %#v", completed.Evidence[0])
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "sql/restore/1"
	restore.BackupReferences = map[string]string{"database": completed.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil || restored.Status != sdk.ResilienceOperationPending {
		t.Fatalf("restore start = %#v err=%v", restored, err)
	}
	restoredDone, err := client.Poll(context.Background(), restored.OperationID)
	if err != nil || restoredDone.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("restore poll = %#v err=%v", restoredDone, err)
	}
	if !strings.HasPrefix(restoredDone.Evidence[0].RestoreID, "gcp-cloud-sql://projects/recovery-project/instances/") {
		t.Fatalf("alternate-region restore target = %q", restoredDone.Evidence[0].RestoreID)
	}
	if restoredDone.Evidence[0].ManifestVerified || restoredDone.Evidence[0].CountsVerified || restoredDone.Evidence[0].ApplicationReadsVerified || restoredDone.Evidence[0].PermissionsVerified || restoredDone.Evidence[0].SecretReferencesVerified {
		t.Fatalf("Cloud SQL restore control-plane observation claimed application evidence: %#v", restoredDone.Evidence[0])
	}
	integrity := request
	integrity.Action = sdk.ResilienceIntegrityCheck
	integrity.IdempotencyKey = "sql/integrity/1"
	integrity.BackupReferences = map[string]string{"database": completed.Evidence[0].BackupID}
	checked, err := client.Start(context.Background(), integrity)
	if err != nil || checked.Status != sdk.ResilienceOperationSucceeded || !checked.Evidence[0].ApplicationReadsVerified {
		t.Fatalf("integrity = %#v err=%v", checked, err)
	}
	if !checked.Evidence[0].ManifestVerified || !checked.Evidence[0].CountsVerified || !checked.Evidence[0].PermissionsVerified || !checked.Evidence[0].SecretReferencesVerified || !checked.Evidence[0].ServiceHealthVerified {
		t.Fatalf("Cloud SQL verifier evidence = %#v", checked.Evidence[0])
	}
}

func TestNativeAPICloudSQLRejectsInPlaceRestoreBeforeMutation(t *testing.T) {
	sql := newFakeCloudSQL()
	sql.instances["demo\x00orders"] = CloudSQLInstance{Project: "demo", Name: "orders", State: "RUNNABLE", Region: "europe-west1", DatabaseVersion: "POSTGRES_17", Tier: "db-custom-2-7680", AvailabilityType: "REGIONAL", DeletionProtection: true, BackupsEnabled: true, UserLabels: secretLabelsFor(cloudrecoveryState("fixture/sql-inplace", "owner"))}
	sql.backups["projects/demo/backups/owned"] = CloudSQLBackup{Name: "projects/demo/backups/owned", Instance: "orders", Description: "owned", State: "SUCCESSFUL"}
	native, err := NewNativeAPIWithSQL(newFakeGCS(), newFakeSecretAPI(), sql, NativeAPIConfig{ArchiveBucket: "archive", Project: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), cloudresilience.NativeOperationRequest{
		Provider: "gcp", Operation: "gcp.cloud-sql.restore.database",
		Request: sdk.ResilienceOperationRequest{
			Action: sdk.ResilienceRestore, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion,
			FixtureID: "fixture/sql-inplace", OwnershipMarker: "owner", IdempotencyKey: "sql/restore/inplace",
			ResourceReferences: map[string]string{"database": "gcp-cloud-sql://projects/demo/instances/orders"},
			BackupReferences:   map[string]string{"database": "gcp-cloud-sql-backup://projects/demo/backups/owned"},
		},
	})
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("error = %v, want unsupported capability", err)
	}
	if !strings.Contains(capability.Reason, "operator approval") {
		t.Fatalf("reason = %q", capability.Reason)
	}
	if len(sql.operations) != 0 {
		t.Fatalf("in-place restore mutated Cloud SQL: %#v", sql.operations)
	}
}

func TestNativeAPICloudSQLInPlaceRestoreUsesSourceInstance(t *testing.T) {
	sql := newFakeCloudSQL()
	sql.instances["demo\x00orders"] = CloudSQLInstance{Project: "demo", Name: "orders", State: "RUNNABLE", Region: "europe-west1", DatabaseVersion: "POSTGRES_17", Tier: "db-custom-2-7680", AvailabilityType: "REGIONAL", DeletionProtection: true, BackupsEnabled: true, UserLabels: secretLabelsFor(cloudrecoveryState("fixture/sql-inplace", "owner"))}
	sql.backups["projects/demo/backups/owned"] = CloudSQLBackup{Name: "projects/demo/backups/owned", Instance: "orders", Description: "owned", State: "SUCCESSFUL"}
	native, err := NewNativeAPIWithSQL(newFakeGCS(), newFakeSecretAPI(), sql, NativeAPIConfig{ArchiveBucket: "archive", Project: "demo", RequireRegionalHA: true, RequireDeletionProtect: true})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	started, err := client.Start(context.Background(), sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceRestore, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/sql-inplace", OwnershipMarker: "owner", IdempotencyKey: "sql/restore/inplace/ok", ApprovalReference: "fence-approved",
		ResourceReferences: map[string]string{"database": "gcp-cloud-sql://projects/demo/instances/orders"},
		BackupReferences:   map[string]string{"database": "gcp-cloud-sql-backup://projects/demo/backups/owned"},
	})
	if err != nil || started.Status != sdk.ResilienceOperationPending {
		t.Fatalf("in-place start = %#v err=%v", started, err)
	}
	if len(sql.operations) != 1 {
		t.Fatalf("in-place restore operations = %#v", sql.operations)
	}
	done, err := client.Poll(context.Background(), started.OperationID)
	if err != nil || done.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("in-place poll = %#v err=%v", done, err)
	}
	if done.Evidence[0].RestoreID != "gcp-cloud-sql://projects/demo/instances/orders" {
		t.Fatalf("in-place restore target = %q", done.Evidence[0].RestoreID)
	}
	if done.Evidence[0].Destination != string(sdk.RecoverySameRegion) {
		t.Fatalf("destination = %q", done.Evidence[0].Destination)
	}
}

func TestNativeAPICloudSQLInPlaceRestoreRejectsRestoreProject(t *testing.T) {
	sql := newFakeCloudSQL()
	sql.instances["demo\x00orders"] = CloudSQLInstance{Project: "demo", Name: "orders", State: "RUNNABLE", Region: "europe-west1", AvailabilityType: "REGIONAL", DeletionProtection: true, BackupsEnabled: true, UserLabels: secretLabelsFor(cloudrecoveryState("fixture/sql-inplace-project", "owner"))}
	sql.backups["projects/demo/backups/owned"] = CloudSQLBackup{Name: "projects/demo/backups/owned", Instance: "orders", State: "SUCCESSFUL"}
	native, err := NewNativeAPIWithSQL(newFakeGCS(), newFakeSecretAPI(), sql, NativeAPIConfig{ArchiveBucket: "archive", Project: "demo", RestoreProject: "recovery-project"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), cloudresilience.NativeOperationRequest{
		Provider: "gcp", Operation: "gcp.cloud-sql.restore.database",
		Request: sdk.ResilienceOperationRequest{
			Action: sdk.ResilienceRestore, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion,
			FixtureID: "fixture/sql-inplace-project", OwnershipMarker: "owner", IdempotencyKey: "sql/restore/inplace/project", ApprovalReference: "fence-approved",
			ResourceReferences: map[string]string{"database": "gcp-cloud-sql://projects/demo/instances/orders"},
			BackupReferences:   map[string]string{"database": "gcp-cloud-sql-backup://projects/demo/backups/owned"},
		},
	})
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("error = %v, want unsupported capability", err)
	}
	if len(sql.operations) != 0 {
		t.Fatalf("retargeted in-place restore mutated Cloud SQL: %#v", sql.operations)
	}
}

func TestNativeAPICloudSQLIntegrityRequiresVerifier(t *testing.T) {
	sql := newFakeCloudSQL()
	sql.instances["demo\x00orders"] = CloudSQLInstance{
		Project: "demo", Name: "orders", State: "RUNNABLE", Region: "europe-west1", AvailabilityType: "REGIONAL",
		DeletionProtection: true, BackupsEnabled: true, UserLabels: secretLabelsFor(cloudrecoveryState("fixture/sql-integrity", "owner")),
	}
	native, err := NewNativeAPIWithSQL(newFakeGCS(), newFakeSecretAPI(), sql, NativeAPIConfig{ArchiveBucket: "archive", Project: "demo", RequireRegionalHA: true, RequireDeletionProtect: true})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Start(context.Background(), sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceIntegrityCheck, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/sql-integrity", OwnershipMarker: "owner", IdempotencyKey: "sql/integrity/no-verifier",
		ResourceReferences: map[string]string{"database": "gcp-cloud-sql://projects/demo/instances/orders"},
	})
	if err == nil || !strings.Contains(err.Error(), "application recovery verifier") {
		t.Fatalf("integrity without verifier error = %v", err)
	}
}

func TestDeleteLeftoverCloudSQLBackupsForInstancePreservesOtherInstances(t *testing.T) {
	sql := newFakeCloudSQL()
	instance := "shop-staging-sql"
	sql.backups["projects/demo/backups/final"] = CloudSQLBackup{
		Name: "projects/demo/backups/final", Instance: instance, Type: "FINAL", State: "SUCCESSFUL",
	}
	sql.backups["projects/demo/backups/full-name"] = CloudSQLBackup{
		Name: "projects/demo/backups/full-name", Instance: "projects/demo/instances/" + instance, Type: "AUTOMATED", State: "SUCCESSFUL",
	}
	sql.backups["projects/demo/backups/other"] = CloudSQLBackup{
		Name: "projects/demo/backups/other", Instance: "other-sql", Type: "FINAL", State: "SUCCESSFUL",
	}
	sql.backups["projects/demo/backups/unscoped"] = CloudSQLBackup{
		Name: "projects/demo/backups/unscoped", Description: "no instance", State: "SUCCESSFUL",
	}
	native, err := NewNativeAPIWithSQL(nil, nil, sql, NativeAPIConfig{Project: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := native.DeleteLeftoverCloudSQLBackupsForInstance(context.Background(), instance)
	if err != nil {
		t.Fatalf("delete leftover backups: %v", err)
	}
	if len(deleted) != 2 || deleted[0] != "projects/demo/backups/final" || deleted[1] != "projects/demo/backups/full-name" {
		t.Fatalf("deleted = %#v", deleted)
	}
	if _, exists := sql.backups["projects/demo/backups/other"]; !exists {
		t.Fatal("backup for another instance was deleted")
	}
	if _, exists := sql.backups["projects/demo/backups/unscoped"]; !exists {
		t.Fatal("backup without an instance name was deleted")
	}
	listed, err := native.LeftoverCloudSQLBackupsForInstance(context.Background(), instance)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("leftovers after delete = %#v", listed)
	}
}

func TestDeleteLeftoverCloudSQLBackupsForInstanceRefusesEmptyName(t *testing.T) {
	native, err := NewNativeAPIWithSQL(nil, nil, newFakeCloudSQL(), NativeAPIConfig{Project: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := native.DeleteLeftoverCloudSQLBackupsForInstance(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "single Magelift instance name") {
		t.Fatalf("empty instance error = %v", err)
	}
	if _, err := native.DeleteLeftoverCloudSQLBackupsForInstance(context.Background(), "shop/*"); err == nil || !strings.Contains(err.Error(), "single Magelift instance name") {
		t.Fatalf("wildcard instance error = %v", err)
	}
}

func TestDeleteOwnedCloudSQLBackupsPreservesForeignBackups(t *testing.T) {
	sql := newFakeCloudSQL()
	owner := "magelift/test/cloud-sql-cleanup"
	sql.backups["projects/demo/backups/owned"] = CloudSQLBackup{
		Name: "projects/demo/backups/owned", Description: "magelift-recovery:" + shortDigest(owner) + ":fixture", State: "SUCCESSFUL",
	}
	sql.backups["projects/demo/backups/foreign"] = CloudSQLBackup{
		Name: "projects/demo/backups/foreign", Description: "operator backup", State: "SUCCESSFUL",
	}
	native, err := NewNativeAPIWithSQL(nil, nil, sql, NativeAPIConfig{Project: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := native.DeleteOwnedCloudSQLBackups(context.Background(), owner); err != nil {
		t.Fatalf("delete owned backups: %v", err)
	}
	if _, exists := sql.backups["projects/demo/backups/owned"]; exists {
		t.Fatal("owned backup remains")
	}
	if _, exists := sql.backups["projects/demo/backups/foreign"]; !exists {
		t.Fatal("foreign backup was deleted")
	}
}

func TestNativeAPIRejectsUnsupportedQueueBeforeMutation(t *testing.T) {
	native, err := NewNativeAPI(newFakeGCS(), newFakeSecretAPI(), NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), cloudresilience.NativeOperationRequest{Provider: "gcp", Operation: "gcp.pubsub.backup.queue", Request: sdk.ResilienceOperationRequest{Action: sdk.ResilienceBackup, DataClasses: []string{"queue"}, FixtureID: "fixture", OwnershipMarker: "owner", IdempotencyKey: "queue/1", ResourceReferences: map[string]string{"queue": "gcp-pubsub://projects/demo/topics/orders"}}})
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("error = %v, want unsupported capability", err)
	}
}

func TestNativeAPIProjectionRecoveryUsesSharedProofContract(t *testing.T) {
	projection, err := cloudrecovery.NewProjectionLifecycle(fakeProjectionBackend{})
	if err != nil {
		t.Fatal(err)
	}
	native, err := NewNativeAPIWithSQLAndProjection(newFakeGCS(), newFakeSecretAPI(), nil, NativeAPIConfig{ArchiveBucket: "archive", Project: "demo"}, projection)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	for _, dataClass := range []string{"search-index", "cache"} {
		request := sdk.ResilienceOperationRequest{
			Action: sdk.ResilienceRestore, DataClasses: []string{dataClass}, Destination: sdk.RecoverySameRegionIsolated,
			FixtureID: "fixture/projection", OwnershipMarker: "magelift/test/gcp/projection", IdempotencyKey: "projection/" + dataClass,
			ResourceReferences: map[string]string{dataClass: "runtime://source/" + dataClass},
		}
		observation, err := client.Start(context.Background(), request)
		if err != nil {
			t.Fatalf("%s projection restore: %v", dataClass, err)
		}
		if observation.Status != sdk.ResilienceOperationSucceeded || len(observation.Evidence) != 1 || observation.Evidence[0].RestoreID == "" {
			t.Fatalf("%s projection observation = %#v", dataClass, observation)
		}
		resumed, err := client.Poll(context.Background(), observation.OperationID)
		if err != nil {
			t.Fatalf("%s projection poll: %v", dataClass, err)
		}
		if resumed.Status != sdk.ResilienceOperationSucceeded || resumed.OperationID != observation.OperationID {
			t.Fatalf("%s projection resumed observation = %#v", dataClass, resumed)
		}
	}
}

type fakeProjectionBackend struct{}

func (fakeProjectionBackend) Rebuild(_ context.Context, request cloudrecovery.ProjectionRequest) (cloudrecovery.ProjectionResult, error) {
	return fakeProjectionResult(request), nil
}

func (fakeProjectionBackend) Verify(_ context.Context, request cloudrecovery.ProjectionRequest) (cloudrecovery.ProjectionResult, error) {
	return fakeProjectionResult(request), nil
}

func fakeProjectionResult(request cloudrecovery.ProjectionRequest) cloudrecovery.ProjectionResult {
	return cloudrecovery.ProjectionResult{
		Status: sdk.ResilienceOperationSucceeded, ResourceReference: request.TargetReference,
		FixtureID: request.FixtureID, OwnershipMarker: request.OwnershipMarker, IdempotencyVerified: true,
		CountsVerified:           request.DataClass == cloudrecovery.ProjectionSearchIndex,
		ApplicationReadsVerified: true, PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		CacheLossClassified: request.DataClass == cloudrecovery.ProjectionCache, RestoreDurationSeconds: 3,
		Reason: "fake runtime verified known projection content",
	}
}

type fakeGCSObject struct {
	body      []byte
	metadata  map[string]string
	kmsKey    string
	etag      string
	retention string
}

type fakeGCS struct {
	objects map[string]fakeGCSObject
	copies  int
}

func newFakeGCS() *fakeGCS { return &fakeGCS{objects: make(map[string]fakeGCSObject)} }

func (f *fakeGCS) putSource(bucket, key string, body []byte) {
	f.objects[bucket+"\x00"+key] = fakeGCSObject{body: append([]byte(nil), body...), etag: "etag-" + key}
}

func (f *fakeGCS) List(_ context.Context, bucket, prefix string) ([]GCSObject, error) {
	objects := make([]GCSObject, 0)
	for composite, value := range f.objects {
		parts := strings.SplitN(composite, "\x00", 2)
		if len(parts) == 2 && parts[0] == bucket && strings.HasPrefix(parts[1], prefix) {
			objects = append(objects, GCSObject{Key: parts[1], Size: int64(len(value.body)), ETag: value.etag})
		}
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

func (f *fakeGCS) Head(_ context.Context, bucket, key string) (GCSObjectMetadata, error) {
	value, ok := f.objects[bucket+"\x00"+key]
	if !ok {
		return GCSObjectMetadata{}, errors.New("object not found")
	}
	return GCSObjectMetadata{Size: int64(len(value.body)), ETag: value.etag, Metadata: cloneMetadata(value.metadata), KMSKeyName: value.kmsKey, RetentionMode: value.retention}, nil
}

func (f *fakeGCS) Read(_ context.Context, bucket, key string) ([]byte, error) {
	value, ok := f.objects[bucket+"\x00"+key]
	if !ok {
		return nil, errors.New("object not found")
	}
	return append([]byte(nil), value.body...), nil
}

func (f *fakeGCS) Delete(_ context.Context, bucket, key string) error {
	composite := bucket + "\x00" + key
	if _, ok := f.objects[composite]; !ok {
		return errors.New("object not found")
	}
	delete(f.objects, composite)
	return nil
}

func (f *fakeGCS) Put(_ context.Context, bucket, key string, body []byte, metadata map[string]string, kmsKey string) error {
	f.objects[bucket+"\x00"+key] = fakeGCSObject{body: append([]byte(nil), body...), metadata: cloneMetadata(metadata), kmsKey: kmsKey, etag: "etag-" + key}
	return nil
}

func (f *fakeGCS) Copy(_ context.Context, sourceBucket, sourceKey, targetBucket, targetKey string, metadata map[string]string, kmsKey string) error {
	source, ok := f.objects[sourceBucket+"\x00"+sourceKey]
	if !ok {
		return errors.New("source object not found")
	}
	f.copies++
	f.objects[targetBucket+"\x00"+targetKey] = fakeGCSObject{body: append([]byte(nil), source.body...), metadata: cloneMetadata(metadata), kmsKey: kmsKey, etag: "etag-" + targetKey}
	return nil
}

type fakeSecret struct {
	value  []byte
	labels map[string]string
}

type fakeSecretAPI struct {
	secrets map[string]fakeSecret
	created []string
}

func newFakeSecretAPI() *fakeSecretAPI { return &fakeSecretAPI{secrets: make(map[string]fakeSecret)} }

func (f *fakeSecretAPI) Get(_ context.Context, name string) (SecretValue, error) {
	value, ok := f.secrets[name]
	if !ok {
		return SecretValue{}, errors.New("secret not found")
	}
	return SecretValue{Data: append([]byte{}, value.value...)}, nil
}

func (f *fakeSecretAPI) Describe(_ context.Context, name string) (SecretMetadata, error) {
	value, ok := f.secrets[name]
	if !ok {
		return SecretMetadata{}, errors.New("secret not found")
	}
	return SecretMetadata{Name: name, Labels: cloneMetadata(value.labels)}, nil
}

func (f *fakeSecretAPI) List(_ context.Context, project string) ([]SecretMetadata, error) {
	prefix := "projects/" + project + "/secrets/"
	result := make([]SecretMetadata, 0)
	for name, secret := range f.secrets {
		if strings.HasPrefix(name, prefix) {
			result = append(result, SecretMetadata{Name: name, Labels: cloneMetadata(secret.labels)})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (f *fakeSecretAPI) Create(_ context.Context, project, secretID string, value []byte, labels map[string]string) error {
	name := "projects/" + project + "/secrets/" + secretID
	f.secrets[name] = fakeSecret{value: append([]byte{}, value...), labels: cloneMetadata(labels)}
	f.created = append(f.created, name)
	return nil
}

func (f *fakeSecretAPI) AddVersion(_ context.Context, name string, value []byte) error {
	secret, ok := f.secrets[name]
	if !ok {
		return errors.New("secret not found")
	}
	secret.value = append([]byte{}, value...)
	f.secrets[name] = secret
	return nil
}

func (f *fakeSecretAPI) Delete(_ context.Context, name string) error {
	if _, ok := f.secrets[name]; !ok {
		return errors.New("secret not found")
	}
	delete(f.secrets, name)
	return nil
}

type fakeCloudSQLOperation struct {
	operation CloudSQLOperation
	target    CloudSQLInstance
	backup    CloudSQLBackup
}

type fakeCloudSQL struct {
	instances  map[string]CloudSQLInstance
	backups    map[string]CloudSQLBackup
	operations map[string]fakeCloudSQLOperation
	next       int
}

func newFakeCloudSQL() *fakeCloudSQL {
	return &fakeCloudSQL{instances: make(map[string]CloudSQLInstance), backups: make(map[string]CloudSQLBackup), operations: make(map[string]fakeCloudSQLOperation)}
}

func (f *fakeCloudSQL) CreateBackup(_ context.Context, project, instance, description string, _ int64) (CloudSQLOperation, error) {
	f.next++
	name := "projects/" + project + "/backups/" + string(rune('0'+f.next))
	opName := "projects/" + project + "/operations/backup-" + string(rune('0'+f.next))
	backup := CloudSQLBackup{Name: name, Instance: instance, Description: description, State: "RUNNING", KMSKey: "", ExpiryTime: time.Now().Add(24 * time.Hour)}
	f.backups[name] = backup
	op := CloudSQLOperation{Name: opName, Status: "running", ResourceName: name}
	f.operations[opName] = fakeCloudSQLOperation{operation: op, backup: backup}
	return op, nil
}

func (f *fakeCloudSQL) DeleteBackup(_ context.Context, name string) (CloudSQLOperation, error) {
	if _, exists := f.backups[name]; !exists {
		return CloudSQLOperation{}, errors.New("backup not found")
	}
	delete(f.backups, name)
	return CloudSQLOperation{Status: "succeeded"}, nil
}

func (f *fakeCloudSQL) ListBackups(_ context.Context, _ string) ([]CloudSQLBackup, error) {
	values := make([]CloudSQLBackup, 0, len(f.backups))
	for _, backup := range f.backups {
		values = append(values, backup)
	}
	return values, nil
}

func (f *fakeCloudSQL) GetBackup(_ context.Context, name string) (CloudSQLBackup, error) {
	backup, ok := f.backups[name]
	if !ok {
		return CloudSQLBackup{}, errors.New("backup not found")
	}
	return backup, nil
}

func (f *fakeCloudSQL) RestoreBackup(_ context.Context, project, target, backup string, settings CloudSQLRestoreSettings) (CloudSQLOperation, error) {
	f.next++
	opName := "projects/" + project + "/operations/restore-" + string(rune('0'+f.next))
	op := CloudSQLOperation{Name: opName, Status: "running", ResourceName: target}
	restored := CloudSQLInstance{Project: project, Name: target, Region: settings.Region, State: "PENDING_CREATE", DatabaseVersion: settings.DatabaseVersion, Tier: settings.Tier, AvailabilityType: settings.AvailabilityType, KMSKey: settings.KMSKey, DeletionProtection: settings.DeletionProtection, UserLabels: cloneMetadata(settings.UserLabels)}
	if existing, ok := f.instances[project+"\x00"+target]; ok {
		restored = existing
		restored.State = "PENDING_CREATE"
		if settings.Region != "" {
			restored.Region = settings.Region
		}
		if settings.DatabaseVersion != "" {
			restored.DatabaseVersion = settings.DatabaseVersion
		}
		if settings.Tier != "" {
			restored.Tier = settings.Tier
		}
		if settings.AvailabilityType != "" {
			restored.AvailabilityType = settings.AvailabilityType
		}
		if settings.KMSKey != "" {
			restored.KMSKey = settings.KMSKey
		}
		if settings.DeletionProtection {
			restored.DeletionProtection = true
		}
		if len(settings.UserLabels) > 0 {
			restored.UserLabels = cloneMetadata(settings.UserLabels)
		}
	}
	f.operations[opName] = fakeCloudSQLOperation{operation: op, target: restored}
	_ = backup
	return op, nil
}

func (f *fakeCloudSQL) GetInstance(_ context.Context, project, instance string) (CloudSQLInstance, error) {
	value, ok := f.instances[project+"\x00"+instance]
	if !ok {
		for _, operation := range f.operations {
			if operation.target.Project == project && operation.target.Name == instance && operation.operation.Status == "succeeded" {
				value = operation.target
				value.State = "RUNNABLE"
				f.instances[project+"\x00"+instance] = value
				return value, nil
			}
		}
		return CloudSQLInstance{}, errors.New("instance not found")
	}
	return value, nil
}

func (f *fakeCloudSQL) ListInstances(_ context.Context, _ string) ([]CloudSQLInstance, error) {
	values := make([]CloudSQLInstance, 0, len(f.instances))
	for _, instance := range f.instances {
		values = append(values, instance)
	}
	return values, nil
}

func (f *fakeCloudSQL) DeleteInstance(_ context.Context, project, instance string) (CloudSQLOperation, error) {
	key := project + "\x00" + instance
	if _, ok := f.instances[key]; !ok {
		return CloudSQLOperation{}, errors.New("instance not found")
	}
	delete(f.instances, key)
	return CloudSQLOperation{Status: "succeeded"}, nil
}

func (f *fakeCloudSQL) GetOperation(_ context.Context, _ string, name string) (CloudSQLOperation, error) {
	operation, ok := f.operations[name]
	if !ok {
		return CloudSQLOperation{}, errors.New("operation not found")
	}
	operation.operation.Status = "succeeded"
	if operation.backup.Name != "" {
		operation.backup.State = "SUCCESSFUL"
		f.backups[operation.backup.Name] = operation.backup
	}
	if operation.target.Name != "" {
		operation.target.State = "RUNNABLE"
		f.instances[operation.target.Project+"\x00"+operation.target.Name] = operation.target
	}
	f.operations[name] = operation
	return operation.operation, nil
}

type fakeRecoveryVerifier struct{}

func (fakeRecoveryVerifier) Verify(context.Context, RecoveryVerificationRequest) (RecoveryVerification, error) {
	return RecoveryVerification{ManifestVerified: true, CountsVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true}, nil
}

func cloudrecoveryState(fixture, marker string) cloudrecovery.OperationState {
	return cloudrecovery.OperationState{Version: cloudrecovery.OperationVersion, DataClass: "database", FixtureID: fixture, OwnershipMarker: marker}
}

func TestNativeAPIRefusesFenceFailoverAndFailbackBeforeMutation(t *testing.T) {
	native, err := NewNativeAPI(nil, nil, NativeAPIConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []sdk.ResilienceAction{sdk.ResilienceFence, sdk.ResilienceFailover, sdk.ResilienceFailback} {
		_, err := native.Start(context.Background(), cloudresilience.NativeOperationRequest{
			Provider: "gcp",
			Request: sdk.ResilienceOperationRequest{
				Action: action, DataClasses: []string{"database", "media"}, Destination: sdk.RecoveryAlternateRegion,
				FixtureID: "fixture/fence", OwnershipMarker: "owner", IdempotencyKey: "runtime/" + string(action),
			},
		})
		var capability sdk.ResilienceCapabilityError
		if !errors.As(err, &capability) || capability.Operation != action || capability.Status != sdk.ResilienceCapabilityUnsupported || capability.AdapterID != AdapterID {
			t.Fatalf("%s error = %v, typed = %#v", action, err, capability)
		}
		if !strings.Contains(capability.Reason, "has not opted into") {
			t.Fatalf("%s reason = %q", action, capability.Reason)
		}
	}
}
