package resilience

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
)

func TestCleanupOwnedRemovesOnlyRecoveryOutputsAcrossScalewayServices(t *testing.T) {
	storage := newFakeScalewayS3()
	owner := "owner"
	ownedMetadata := cloudrecovery.MetadataFor(cloudrecovery.ObjectArchiveScope{DataClass: "media", FixtureID: "fixture", OwnershipMarker: owner}, false)
	archiveKey := "snapshots/" + shortDigest(owner) + "/media/backup/objects/a"
	restoreKey := "isolated/media/" + shortDigest(owner) + "/restore/objects/a"
	storage.objects["archive\x00"+archiveKey] = fakeScalewayObject{body: []byte("archive"), metadata: ownedMetadata, encryption: s3types.ServerSideEncryptionAes256}
	storage.objects["restore\x00"+restoreKey] = fakeScalewayObject{body: []byte("restore"), metadata: ownedMetadata, encryption: s3types.ServerSideEncryptionAes256}
	storage.objects["archive\x00foreign"] = fakeScalewayObject{body: []byte("foreign"), metadata: map[string]string{"unrelated": "true"}, encryption: s3types.ServerSideEncryptionAes256}

	database := newFakeScalewayDatabase()
	database.instances["owned-instance"] = DatabaseInstance{ID: "owned-instance", Name: "magelift-restore-owned", Tags: scalewayDatabaseTags(nil, operationState{OwnershipMarker: owner, DataClass: "database"})}
	database.instances["source-instance"] = DatabaseInstance{ID: "source-instance", Name: "source", Tags: []string{scalewayDatabaseOwnershipTag + "=" + owner, scalewayDatabaseClassTag + "=database"}}
	database.instances["foreign-instance"] = DatabaseInstance{ID: "foreign-instance", Name: "magelift-restore-foreign", Tags: scalewayDatabaseTags(nil, operationState{OwnershipMarker: "other", DataClass: "database"})}
	database.snapshots["owned-snapshot"] = DatabaseSnapshot{ID: "owned-snapshot", Name: scalewayDatabaseNamePrefix + shortDigest(owner) + "-backup"}
	database.snapshots["foreign-snapshot"] = DatabaseSnapshot{ID: "foreign-snapshot", Name: scalewayDatabaseNamePrefix + shortDigest("other") + "-backup"}

	secrets := newFakeScalewaySecrets()
	secrets.secrets["owned-secret"] = SecretMetadata{ID: "owned-secret", Name: "restore-secret-owned", Protected: true, Tags: scalewaySecretTags(operationState{OwnershipMarker: owner, DataClass: "configuration-secrets"})}
	secrets.values["owned-secret"] = []byte("owned")
	secrets.secrets["source-secret"] = SecretMetadata{ID: "source-secret", Name: "app", Protected: true, Tags: []string{scalewayDatabaseOwnershipTag + "=" + owner, scalewayDatabaseClassTag + "=configuration-secrets"}}
	secrets.secrets["foreign-secret"] = SecretMetadata{ID: "foreign-secret", Name: "restore-secret-foreign", Protected: true, Tags: scalewaySecretTags(operationState{OwnershipMarker: "other", DataClass: "configuration-secrets"})}

	native, err := NewNativeAPIWithServices(storage, database, secrets, NativeAPIConfig{ArchiveBucket: "archive", ArchivePrefix: "snapshots", RestoreBucket: "restore", RestorePrefix: "isolated", RestoreSecretPrefix: "restore-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if err := native.CleanupOwned(context.Background(), owner); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	for _, key := range []string{"archive\x00" + archiveKey, "restore\x00" + restoreKey} {
		if _, ok := storage.objects[key]; ok {
			t.Fatalf("owned object %q remains", key)
		}
	}
	if _, ok := storage.objects["archive\x00foreign"]; !ok {
		t.Fatal("cleanup deleted an unmarked object")
	}
	if _, ok := database.instances["owned-instance"]; ok {
		t.Fatal("owned database restore remains")
	}
	if _, ok := database.instances["source-instance"]; !ok {
		t.Fatal("cleanup deleted the source database")
	}
	if _, ok := database.instances["foreign-instance"]; !ok {
		t.Fatal("cleanup deleted a foreign database restore")
	}
	if _, ok := database.snapshots["owned-snapshot"]; ok {
		t.Fatal("owned database snapshot remains")
	}
	if _, ok := database.snapshots["foreign-snapshot"]; !ok {
		t.Fatal("cleanup deleted a foreign database snapshot")
	}
	if _, ok := secrets.secrets["owned-secret"]; ok {
		t.Fatal("owned restore secret remains")
	}
	if _, ok := secrets.secrets["source-secret"]; !ok {
		t.Fatal("cleanup deleted the source secret")
	}
	if _, ok := secrets.secrets["foreign-secret"]; !ok {
		t.Fatal("cleanup deleted a foreign secret")
	}
}

func TestCleanupOwnedReportsObjectLockRetentionWithoutDeleting(t *testing.T) {
	storage := newFakeScalewayS3()
	owner := "owner"
	key := "snapshots/" + shortDigest(owner) + "/media/backup/manifest.json"
	retainUntil := time.Now().Add(time.Hour)
	storage.objects["archive\x00"+key] = fakeScalewayObject{
		body:       []byte("locked"),
		metadata:   cloudrecovery.MetadataFor(cloudrecovery.ObjectArchiveScope{DataClass: "media", FixtureID: "fixture", OwnershipMarker: owner}, true),
		encryption: s3types.ServerSideEncryptionAes256,
		lockMode:   s3types.ObjectLockModeCompliance,
		retain:     &retainUntil,
	}
	native, err := NewNativeAPI(storage, NativeAPIConfig{ArchiveBucket: "archive", ArchivePrefix: "snapshots", RequireObjectLock: true})
	if err != nil {
		t.Fatal(err)
	}
	err = native.CleanupOwned(context.Background(), owner)
	var pending *ScalewayCleanupPendingError
	if !errors.As(err, &pending) || len(pending.Resources) != 1 || !strings.Contains(pending.Resources[0], key) {
		t.Fatalf("cleanup error = %v, want retention pending", err)
	}
	if _, ok := storage.objects["archive\x00"+key]; !ok {
		t.Fatal("cleanup deleted a compliance-retained object")
	}
}

func TestCleanupOwnedRefusesArchiveOwnershipDrift(t *testing.T) {
	storage := newFakeScalewayS3()
	owner := "owner"
	key := "snapshots/" + shortDigest(owner) + "/media/backup/manifest.json"
	storage.objects["archive\x00"+key] = fakeScalewayObject{body: []byte("drift"), metadata: map[string]string{"wrong": "owner"}, encryption: s3types.ServerSideEncryptionAes256}
	native, err := NewNativeAPI(storage, NativeAPIConfig{ArchiveBucket: "archive", ArchivePrefix: "snapshots"})
	if err != nil {
		t.Fatal(err)
	}
	if err := native.CleanupOwned(context.Background(), owner); err == nil || !strings.Contains(err.Error(), "ownership metadata drifted") {
		t.Fatalf("cleanup error = %v, want ownership drift refusal", err)
	}
	if _, ok := storage.objects["archive\x00"+key]; !ok {
		t.Fatal("ownership drift cleanup deleted the object")
	}
}
