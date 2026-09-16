package resilience

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/url"
	"sort"
	"strings"
	"testing"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretstypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"

	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	cloudresilience "github.com/magelift/magelift/internal/shared/resilience"
	"github.com/magelift/magelift/sdk"
)

func TestNativeAPIObjectRecoveryIsIdempotentAndResumable(t *testing.T) {
	ctx := context.Background()
	storage := newFakeS3()
	storage.objects[fakeObjectKey("source", "media/a.json")] = fakeObject{body: []byte(`{"id":"a"}`), encryption: s3types.ServerSideEncryptionAes256}
	storage.objects[fakeObjectKey("source", "media/b.json")] = fakeObject{body: []byte(`{"id":"b"}`), encryption: s3types.ServerSideEncryptionAes256}
	native, err := NewNativeAPI(newFakeRDS(), storage, newFakeSecrets(), NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "restore", ArchivePrefix: "snapshots", RestorePrefix: "isolated", RetentionDays: 21})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"media"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/known-content", OwnershipMarker: "magelift/test/aws", IdempotencyKey: "idempotency/media/1",
		ResourceReferences: map[string]string{"media": "s3://source/media"},
	}
	backup, err := client.Start(ctx, request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || len(backup.Evidence) != 1 || backup.Evidence[0].BackupID == "" {
		t.Fatalf("backup observation = %#v", backup)
	}
	copiesAfterFirstBackup := storage.copies
	secondBackup, err := client.Start(ctx, request)
	if err != nil {
		t.Fatalf("idempotent backup: %v", err)
	}
	if secondBackup.Status != sdk.ResilienceOperationSucceeded || storage.copies != copiesAfterFirstBackup {
		t.Fatalf("idempotent backup changed archive: status=%s copies=%d want=%d", secondBackup.Status, storage.copies, copiesAfterFirstBackup)
	}

	restoreRequest := request
	restoreRequest.Action = sdk.ResilienceRestore
	restoreRequest.IdempotencyKey = "idempotency/media/restore/1"
	restoreRequest.BackupReferences = map[string]string{"media": backup.Evidence[0].BackupID}
	restore, err := client.Start(ctx, restoreRequest)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restore.Status != sdk.ResilienceOperationSucceeded || restore.Evidence[0].RestoreID != "aws-s3://restore/isolated/media/"+shortDigest("magelift/test/aws")+"/"+shortDigest(restoreRequest.IdempotencyKey) {
		t.Fatalf("restore observation = %#v", restore)
	}
	resumed, err := client.Poll(ctx, restore.OperationID)
	if err != nil {
		t.Fatalf("resume restore poll: %v", err)
	}
	if resumed.Status != sdk.ResilienceOperationSucceeded || resumed.OperationID != restore.OperationID {
		t.Fatalf("resumed restore = %#v", resumed)
	}

	integrityRequest := restoreRequest
	integrityRequest.Action = sdk.ResilienceIntegrityCheck
	integrityRequest.IdempotencyKey = "idempotency/media/integrity/1"
	integrity, err := client.Start(ctx, integrityRequest)
	if err != nil {
		t.Fatalf("integrity: %v", err)
	}
	if integrity.Status != sdk.ResilienceOperationSucceeded || !integrity.Evidence[0].ManifestVerified || !integrity.Evidence[0].CountsVerified {
		t.Fatalf("integrity observation = %#v", integrity)
	}
}

func TestNativeAPIObjectCleanupRemovesOwnedOutputsAndPreservesSource(t *testing.T) {
	ctx := context.Background()
	storage := newFakeS3()
	storage.objects[fakeObjectKey("source", "media/fixture.json")] = fakeObject{body: []byte(`{"id":"fixture"}`), encryption: s3types.ServerSideEncryptionAes256}
	native, err := NewNativeAPI(newFakeRDS(), storage, newFakeSecrets(), NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "archive", ArchivePrefix: "snapshots", RestorePrefix: "isolated", RetentionDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"media"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/cleanup", OwnershipMarker: "magelift/test/aws-cleanup", IdempotencyKey: "live/media/backup",
		ResourceReferences: map[string]string{"media": "s3://source/media"},
	}
	backup, err := client.Start(ctx, request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "live/media/restore"
	restore.BackupReferences = map[string]string{"media": backup.Evidence[0].BackupID}
	if _, err := client.Start(ctx, restore); err != nil {
		t.Fatalf("restore: %v", err)
	}
	cleanup := request
	cleanup.Action = sdk.ResilienceCleanup
	cleanup.IdempotencyKey = "live/media/cleanup"
	result, err := client.Start(ctx, cleanup)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.Status != sdk.ResilienceOperationSucceeded || len(result.ResourceRefs) == 0 {
		t.Fatalf("cleanup observation = %#v", result)
	}
	remaining, err := native.Inventory(ctx, request.OwnershipMarker)
	if err != nil {
		t.Fatalf("inventory after cleanup: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("owned objects remain after cleanup: %#v", remaining)
	}
	if _, ok := storage.objects[fakeObjectKey("source", "media/fixture.json")]; !ok {
		t.Fatal("cleanup deleted the source fixture")
	}
}

func TestNativeAPISecretRecoveryAndCleanupPreserveSource(t *testing.T) {
	ctx := context.Background()
	storage := newFakeS3()
	secrets := newFakeSecrets()
	marker := "magelift/test/aws-secret"
	fixture := "fixture/known-secret"
	state := operationState{OwnershipMarker: marker, DataClass: "configuration-secrets", IdempotencyKey: "source"}
	secrets.descriptions["source"] = secretsmanager.DescribeSecretOutput{Name: aws.String("source"), Tags: secretTagsFor(state)}
	secrets.values["source"] = secretsmanager.GetSecretValueOutput{SecretString: aws.String("synthetic-secret")}
	native, err := NewNativeAPI(newFakeRDS(), storage, secrets, NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "archive", ArchivePrefix: "snapshots", RestorePrefix: "isolated", RetentionDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"configuration-secrets"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: fixture, OwnershipMarker: marker, IdempotencyKey: "live/configuration-secrets/backup",
		ResourceReferences: map[string]string{"configuration-secrets": "aws-secretsmanager://source"},
	}
	backup, err := client.Start(ctx, request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || backup.Evidence[0].BackupID == "" || !backup.Evidence[0].SecretReferencesVerified {
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
	if observation, err := client.Start(ctx, cleanup); err != nil || observation.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("cleanup = %#v, err=%v", observation, err)
	}
	remaining, err := native.Inventory(ctx, marker)
	if err != nil {
		t.Fatalf("inventory after cleanup: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("owned outputs remain after cleanup: %#v", remaining)
	}
	if _, ok := secrets.values["source"]; !ok {
		t.Fatal("cleanup deleted source secret")
	}
}

func TestNativeAPIRDSBackupAndRestorePollAfterInterruption(t *testing.T) {
	ctx := context.Background()
	rdsClient := newFakeRDS()
	rdsClient.instances["orders"] = rdstypes.DBInstance{
		DBInstanceIdentifier: aws.String("orders"), DBInstanceArn: aws.String("arn:aws:rds:eu-west-1:123456789012:db:orders"),
		DBInstanceStatus: aws.String("available"), Engine: aws.String("postgres"), EngineVersion: aws.String("17"),
		StorageEncrypted: aws.Bool(true), DeletionProtection: aws.Bool(true), DBInstanceClass: aws.String("db.t4g.small"),
	}
	rdsClient.tags["arn:aws:rds:eu-west-1:123456789012:db:orders"] = rdsTags("magelift/test/aws", "database")
	native, err := NewNativeAPI(rdsClient, newFakeS3(), newFakeSecrets(), NativeAPIConfig{
		ArchiveBucket: "archive", RestorePubliclyAccessible: true, RestoreVPCSecurityGroupIDs: []string{"sg-restore"},
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/known-content", OwnershipMarker: "magelift/test/aws", IdempotencyKey: "idempotency/database/1",
		ResourceReferences: map[string]string{"database": "aws-rds://instance/orders"},
	}
	backup, err := client.Start(ctx, request)
	if err != nil {
		t.Fatalf("start database backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationPending || backup.OperationID == "" {
		t.Fatalf("backup start = %#v", backup)
	}
	completed, err := client.Poll(ctx, backup.OperationID)
	if err != nil {
		t.Fatalf("poll database backup: %v", err)
	}
	if completed.Status != sdk.ResilienceOperationSucceeded || completed.Evidence[0].BackupID == "" {
		t.Fatalf("backup completion = %#v", completed)
	}
	if completed.Evidence[0].ManifestVerified || completed.Evidence[0].CountsVerified || completed.Evidence[0].ApplicationReadsVerified || completed.Evidence[0].PermissionsVerified || completed.Evidence[0].SecretReferencesVerified {
		t.Fatalf("RDS backup control-plane observation claimed application evidence: %#v", completed.Evidence[0])
	}

	restoreRequest := request
	restoreRequest.Action = sdk.ResilienceRestore
	restoreRequest.IdempotencyKey = "idempotency/database/restore/1"
	restoreRequest.BackupReferences = map[string]string{"database": completed.Evidence[0].BackupID}
	restore, err := client.Start(ctx, restoreRequest)
	if err != nil {
		t.Fatalf("start database restore: %v", err)
	}
	if restore.Status != sdk.ResilienceOperationPending {
		t.Fatalf("restore start = %#v", restore)
	}
	if rdsClient.lastRestoreInput == nil || !aws.ToBool(rdsClient.lastRestoreInput.PubliclyAccessible) || len(rdsClient.lastRestoreInput.VpcSecurityGroupIds) != 1 || rdsClient.lastRestoreInput.VpcSecurityGroupIds[0] != "sg-restore" {
		t.Fatalf("restore network settings = %#v", rdsClient.lastRestoreInput)
	}
	completedRestore, err := client.Poll(ctx, restore.OperationID)
	if err != nil {
		t.Fatalf("poll database restore: %v", err)
	}
	if completedRestore.Status != sdk.ResilienceOperationSucceeded || completedRestore.Evidence[0].RestoreID == "" || !completedRestore.Evidence[0].ServiceHealthVerified {
		t.Fatalf("restore completion = %#v", completedRestore)
	}
	if completedRestore.Evidence[0].ManifestVerified || completedRestore.Evidence[0].CountsVerified || completedRestore.Evidence[0].ApplicationReadsVerified || completedRestore.Evidence[0].PermissionsVerified || completedRestore.Evidence[0].SecretReferencesVerified {
		t.Fatalf("RDS restore control-plane observation claimed application evidence: %#v", completedRestore.Evidence[0])
	}
}

func TestNativeAPIAuroraClusterBackupRestorePollAndCleanup(t *testing.T) {
	ctx := context.Background()
	marker := "magelift/test/aws-aurora"
	rdsClient, client := newAuroraRecoveryTestSetup(t, marker)

	foreignARN := "arn:aws:rds:eu-west-1:123456789012:cluster:foreign"
	rdsClient.clusters["foreign"] = rdstypes.DBCluster{
		DBClusterIdentifier: aws.String("foreign"), DBClusterArn: aws.String(foreignARN),
		Status: aws.String("available"), Engine: aws.String("aurora-mysql"),
		StorageEncrypted: aws.Bool(true), DeletionProtection: aws.Bool(true),
	}
	rdsClient.tags[foreignARN] = rdsTags("magelift/test/other-aurora", "database")

	backupRequest := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/aurora-known-content", OwnershipMarker: marker, IdempotencyKey: "aurora/backup/1",
		ResourceReferences: map[string]string{"database": "aws-rds://cluster/orders"},
	}
	backupStart, err := client.Start(ctx, backupRequest)
	if err != nil {
		t.Fatalf("start Aurora backup: %v", err)
	}
	if backupStart.Status != sdk.ResilienceOperationPending || backupStart.OperationID == "" {
		t.Fatalf("Aurora backup start = %#v", backupStart)
	}
	backup, err := client.Poll(ctx, backupStart.OperationID)
	if err != nil {
		t.Fatalf("poll Aurora backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || len(backup.Evidence) != 1 {
		t.Fatalf("Aurora backup completion = %#v", backup)
	}
	backupEvidence := backup.Evidence[0]
	expectedSnapshotID := strings.ToLower("magelift-cluster-snapshot-" + shortDigest(marker+"\x00"+backupRequest.IdempotencyKey+"\x00"+backupRequest.ResourceReferences["database"]))
	if backupEvidence.BackupID != "aws-rds://cluster-snapshot/"+expectedSnapshotID {
		t.Fatalf("Aurora backup identity = %q, want aws-rds://cluster-snapshot/%s", backupEvidence.BackupID, expectedSnapshotID)
	}
	if !backupEvidence.EncryptionVerified || !backupEvidence.ProtectionVerified || !backupEvidence.ServiceHealthVerified {
		t.Fatalf("Aurora backup control-plane evidence = %#v", backupEvidence)
	}
	assertRDSControlPlaneOnlyEvidence(t, backupEvidence)
	snapshot, ok := rdsClient.clusterSnapshots[expectedSnapshotID]
	if !ok || !rdsTagsMatch(snapshot.TagList, marker, "database") {
		t.Fatalf("Aurora cluster snapshot ownership = %#v, want marker %q and database class", snapshot.TagList, marker)
	}

	restoreRequest := backupRequest
	restoreRequest.Action = sdk.ResilienceRestore
	restoreRequest.IdempotencyKey = "aurora/restore/1"
	restoreRequest.BackupReferences = map[string]string{"database": backupEvidence.BackupID}
	restoreStart, err := client.Start(ctx, restoreRequest)
	if err != nil {
		t.Fatalf("start Aurora restore: %v", err)
	}
	if restoreStart.Status != sdk.ResilienceOperationPending || restoreStart.OperationID == "" {
		t.Fatalf("Aurora restore start = %#v", restoreStart)
	}
	expectedTargetID := strings.ToLower("magelift-cluster-restore-" + shortDigest("orders\x00"+marker+"\x00"+restoreRequest.IdempotencyKey))
	state, err := decodeOperationState(restoreStart.OperationID)
	if err != nil {
		t.Fatalf("decode Aurora restore identity: %v", err)
	}
	if state.Target != "aws-rds://cluster/"+expectedTargetID {
		t.Fatalf("Aurora restore operation target = %q, want aws-rds://cluster/%s", state.Target, expectedTargetID)
	}
	restoredCluster, ok := rdsClient.clusters[expectedTargetID]
	if !ok || aws.ToString(restoredCluster.Status) != "creating" || aws.ToString(restoredCluster.Engine) != "aurora-mysql" || !aws.ToBool(restoredCluster.DeletionProtection) {
		t.Fatalf("Aurora restored cluster after start = %#v", restoredCluster)
	}
	if !rdsTagsMatch(rdsClient.tags[aws.ToString(restoredCluster.DBClusterArn)], marker, "database") {
		t.Fatalf("Aurora restored cluster ownership = %#v", rdsClient.tags[aws.ToString(restoredCluster.DBClusterArn)])
	}
	if rdsClient.lastRestoreClusterInput == nil || len(rdsClient.lastRestoreClusterInput.VpcSecurityGroupIds) != 1 || rdsClient.lastRestoreClusterInput.VpcSecurityGroupIds[0] != "sg-aurora" {
		t.Fatalf("Aurora restore network settings = %#v", rdsClient.lastRestoreClusterInput)
	}
	if _, ok := rdsClient.instances[expectedTargetID+"-instance"]; ok {
		t.Fatal("Aurora restore created a member before the cluster became available")
	}

	clusterPending, err := client.Poll(ctx, restoreStart.OperationID)
	if err != nil {
		t.Fatalf("poll creating Aurora cluster: %v", err)
	}
	if clusterPending.Status != sdk.ResilienceOperationPending || clusterPending.OperationID != restoreStart.OperationID {
		t.Fatalf("creating Aurora cluster poll = %#v", clusterPending)
	}
	rdsClient.clusters[expectedTargetID] = func() rdstypes.DBCluster {
		cluster := rdsClient.clusters[expectedTargetID]
		cluster.Status = aws.String("available")
		return cluster
	}()

	memberPending, err := client.Poll(ctx, restoreStart.OperationID)
	if err != nil {
		t.Fatalf("create Aurora restore member: %v", err)
	}
	if memberPending.Status != sdk.ResilienceOperationPending || memberPending.OperationID != restoreStart.OperationID {
		t.Fatalf("Aurora member creation poll = %#v", memberPending)
	}
	memberID := expectedTargetID + "-instance"
	member, ok := rdsClient.instances[memberID]
	if !ok || aws.ToString(member.DBInstanceStatus) != "creating" || aws.ToString(member.DBClusterIdentifier) != expectedTargetID || aws.ToString(member.DBInstanceClass) != "db.r6g.large" {
		t.Fatalf("Aurora restored member after create = %#v", member)
	}
	if !rdsTagsMatch(rdsClient.tags[aws.ToString(member.DBInstanceArn)], marker, "database") {
		t.Fatalf("Aurora restored member ownership = %#v", rdsClient.tags[aws.ToString(member.DBInstanceArn)])
	}
	if rdsClient.lastCreateInstanceInput == nil || !aws.ToBool(rdsClient.lastCreateInstanceInput.PubliclyAccessible) {
		t.Fatalf("Aurora restored member reachability = %#v", rdsClient.lastCreateInstanceInput)
	}
	if len(rdsClient.clusters[expectedTargetID].DBClusterMembers) != 1 || aws.ToString(rdsClient.clusters[expectedTargetID].DBClusterMembers[0].DBInstanceIdentifier) != memberID {
		t.Fatalf("Aurora cluster members = %#v, want one restored member", rdsClient.clusters[expectedTargetID].DBClusterMembers)
	}

	rdsClient.instances[memberID] = func() rdstypes.DBInstance {
		instance := rdsClient.instances[memberID]
		instance.DBInstanceStatus = aws.String("available")
		return instance
	}()
	restored, err := client.Poll(ctx, restoreStart.OperationID)
	if err != nil {
		t.Fatalf("poll available Aurora restore: %v", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || len(restored.Evidence) != 1 {
		t.Fatalf("Aurora restore completion = %#v", restored)
	}
	restoreEvidence := restored.Evidence[0]
	if restoreEvidence.BackupID != backupEvidence.BackupID || restoreEvidence.RestoreID != "aws-rds://cluster/"+expectedTargetID || !restoreEvidence.EncryptionVerified || !restoreEvidence.ProtectionVerified || !restoreEvidence.ServiceHealthVerified {
		t.Fatalf("Aurora restore evidence = %#v", restoreEvidence)
	}
	assertRDSControlPlaneOnlyEvidence(t, restoreEvidence)
	if len(restored.ResourceRefs) != 1 || restored.ResourceRefs[0] != "aws-rds://cluster/"+expectedTargetID {
		t.Fatalf("Aurora restore resource refs = %#v", restored.ResourceRefs)
	}
	repeated, err := client.Poll(ctx, restoreStart.OperationID)
	if err != nil {
		t.Fatalf("repeat Aurora restore poll: %v", err)
	}
	if repeated.Status != sdk.ResilienceOperationSucceeded || len(repeated.Evidence) != 1 || repeated.Evidence[0].RestoreID != restoreEvidence.RestoreID {
		t.Fatalf("repeat Aurora restore poll = %#v", repeated)
	}

	owned, err := client.Inventory(ctx, marker)
	if err != nil {
		t.Fatalf("Aurora inventory before cleanup: %v", err)
	}
	for _, identity := range []string{
		"aws-rds://cluster/orders", "aws-rds://cluster/" + expectedTargetID,
		"aws-rds://cluster-snapshot/" + expectedSnapshotID, "aws-rds://instance/orders-instance", "aws-rds://instance/" + memberID,
	} {
		if !containsInventoryIdentity(owned, identity) {
			t.Fatalf("Aurora inventory missing %q: %#v", identity, owned)
		}
	}

	cleanup := backupRequest
	cleanup.Action = sdk.ResilienceCleanup
	cleanup.IdempotencyKey = "aurora/cleanup/1"
	cleanupStart, err := client.Start(ctx, cleanup)
	if err != nil {
		t.Fatalf("start Aurora cleanup: %v", err)
	}
	if cleanupStart.Status != sdk.ResilienceOperationPending {
		t.Fatalf("Aurora cleanup start = %#v", cleanupStart)
	}
	cleanupComplete, err := client.Poll(ctx, cleanupStart.OperationID)
	if err != nil {
		t.Fatalf("poll Aurora cleanup: %v", err)
	}
	if cleanupComplete.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("Aurora cleanup completion = %#v", cleanupComplete)
	}
	remaining, err := client.Inventory(ctx, marker)
	if err != nil {
		t.Fatalf("Aurora inventory after cleanup: %v", err)
	}
	if len(remaining) != 2 || !containsInventoryIdentity(remaining, "aws-rds://cluster/orders") || !containsInventoryIdentity(remaining, "aws-rds://instance/orders-instance") {
		t.Fatalf("Aurora cleanup removed or hid source cluster: %#v", remaining)
	}
	foreign, err := client.Inventory(ctx, "magelift/test/other-aurora")
	if err != nil {
		t.Fatalf("foreign Aurora inventory after cleanup: %v", err)
	}
	if len(foreign) != 1 || foreign[0].Identity != "aws-rds://cluster/foreign" {
		t.Fatalf("Aurora cleanup touched foreign cluster: %#v", foreign)
	}
	if _, ok := rdsClient.instances[memberID]; ok {
		t.Fatal("Aurora cleanup left restored member")
	}
	if _, ok := rdsClient.clusters[expectedTargetID]; ok {
		t.Fatal("Aurora cleanup left restored cluster")
	}
	if _, ok := rdsClient.clusterSnapshots[expectedSnapshotID]; ok {
		t.Fatal("Aurora cleanup left owned cluster snapshot")
	}
	wantEvents := []string{
		"create-cluster-snapshot:" + expectedSnapshotID,
		"restore-cluster:" + expectedTargetID,
		"create-instance:" + memberID,
		"delete-instance:" + memberID,
		"modify-cluster:" + expectedTargetID,
		"delete-cluster:" + expectedTargetID,
		"delete-cluster-snapshot:" + expectedSnapshotID,
	}
	if got, want := strings.Join(rdsClient.events, "|"), strings.Join(wantEvents, "|"); got != want {
		t.Fatalf("Aurora RDS mutation order = %q, want %q", got, want)
	}
}

func TestNativeAPIAuroraRecoveryRejectsForeignSnapshotsAndCollisions(t *testing.T) {
	tests := []struct {
		name    string
		action  sdk.ResilienceAction
		prepare func(*fakeRDS, string, string, string)
		wantErr string
	}{
		{
			name:   "foreign cluster snapshot on restore",
			action: sdk.ResilienceRestore,
			prepare: func(rdsClient *fakeRDS, marker, snapshotID, _ string) {
				rdsClient.clusterSnapshots[snapshotID] = fakeAuroraClusterSnapshot(snapshotID, "magelift/test/foreign-snapshot", "database")
			},
			wantErr: "AWS Aurora restore snapshot is missing, unowned, or not available",
		},
		{
			name:   "foreign cluster collision on restore",
			action: sdk.ResilienceRestore,
			prepare: func(rdsClient *fakeRDS, _ string, snapshotID, targetID string) {
				rdsClient.clusterSnapshots[snapshotID] = fakeAuroraClusterSnapshot(snapshotID, "magelift/test/aws-aurora-collision", "database")
				arn := "arn:aws:rds:eu-west-1:123456789012:cluster:" + targetID
				rdsClient.clusters[targetID] = fakeAuroraCluster(targetID, arn, "available", nil)
				rdsClient.tags[arn] = rdsTags("magelift/test/foreign-cluster", "database")
			},
			wantErr: "refusing to reuse an unowned AWS Aurora restore cluster",
		},
		{
			name:   "foreign member on owned cluster",
			action: sdk.ResilienceRestore,
			prepare: func(rdsClient *fakeRDS, marker, snapshotID, targetID string) {
				rdsClient.clusterSnapshots[snapshotID] = fakeAuroraClusterSnapshot(snapshotID, marker, "database")
				clusterARN := "arn:aws:rds:eu-west-1:123456789012:cluster:" + targetID
				memberID := targetID + "-instance"
				memberARN := "arn:aws:rds:eu-west-1:123456789012:db:" + memberID
				rdsClient.clusters[targetID] = fakeAuroraCluster(targetID, clusterARN, "available", []rdstypes.DBClusterMember{{DBInstanceIdentifier: aws.String(memberID)}})
				rdsClient.tags[clusterARN] = rdsTags(marker, "database")
				rdsClient.instances[memberID] = rdstypes.DBInstance{
					DBInstanceIdentifier: aws.String(memberID), DBInstanceArn: aws.String(memberARN), DBInstanceStatus: aws.String("available"),
					Engine: aws.String("aurora-mysql"), DBClusterIdentifier: aws.String(targetID), StorageEncrypted: aws.Bool(true), DeletionProtection: aws.Bool(true),
				}
				rdsClient.tags[memberARN] = rdsTags("magelift/test/foreign-member", "database")
			},
			wantErr: "AWS Aurora restore DB instance is not owned by this operation",
		},
		{
			name:   "foreign cluster snapshot collision on backup",
			action: sdk.ResilienceBackup,
			prepare: func(rdsClient *fakeRDS, _ string, snapshotID, _ string) {
				rdsClient.clusterSnapshots[snapshotID] = fakeAuroraClusterSnapshot(snapshotID, "magelift/test/foreign-backup", "database")
			},
			wantErr: "AWS RDS cluster snapshot exists but is not owned by this operation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marker := "magelift/test/aws-aurora-cases"
			rdsClient, client := newAuroraRecoveryTestSetup(t, marker)
			idempotencyKey := "aurora/restore/negative"
			if tt.action == sdk.ResilienceBackup {
				idempotencyKey = "aurora/backup/negative"
			}
			request := sdk.ResilienceOperationRequest{
				Action: tt.action, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegionIsolated,
				FixtureID: "fixture/aurora-negative", OwnershipMarker: marker, IdempotencyKey: idempotencyKey,
				ResourceReferences: map[string]string{"database": "aws-rds://cluster/orders"},
			}
			snapshotID := "aurora-negative-snapshot"
			if tt.action == sdk.ResilienceBackup {
				snapshotID = strings.ToLower("magelift-cluster-snapshot-" + shortDigest(marker+"\x00"+idempotencyKey+"\x00"+request.ResourceReferences["database"]))
			} else {
				request.BackupReferences = map[string]string{"database": "aws-rds://cluster-snapshot/" + snapshotID}
			}
			targetID := strings.ToLower("magelift-cluster-restore-" + shortDigest("orders\x00"+marker+"\x00"+idempotencyKey))
			tt.prepare(rdsClient, marker, snapshotID, targetID)

			_, err := client.Start(context.Background(), request)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
			if len(rdsClient.events) != 0 {
				t.Fatalf("negative Aurora case mutated RDS = %#v", rdsClient.events)
			}
		})
	}
}

func newAuroraRecoveryTestSetup(t *testing.T, marker string) (*fakeRDS, sdk.ResilienceOperationClient) {
	t.Helper()
	rdsClient := newFakeRDS()
	sourceARN := "arn:aws:rds:eu-west-1:123456789012:cluster:orders"
	sourceMemberID := "orders-instance"
	rdsClient.clusters["orders"] = fakeAuroraCluster("orders", sourceARN, "available", []rdstypes.DBClusterMember{{DBInstanceIdentifier: aws.String(sourceMemberID), IsClusterWriter: aws.Bool(true)}})
	rdsClient.tags[sourceARN] = rdsTags(marker, "database")
	sourceMemberARN := "arn:aws:rds:eu-west-1:123456789012:db:" + sourceMemberID
	rdsClient.instances[sourceMemberID] = rdstypes.DBInstance{
		DBInstanceIdentifier: aws.String(sourceMemberID), DBInstanceArn: aws.String(sourceMemberARN), DBInstanceStatus: aws.String("available"),
		Engine: aws.String("aurora-mysql"), StorageEncrypted: aws.Bool(true), DeletionProtection: aws.Bool(true), DBClusterIdentifier: aws.String("orders"),
	}
	rdsClient.tags[sourceMemberARN] = rdsTags(marker, "database")
	native, err := NewNativeAPI(rdsClient, newFakeS3(), newFakeSecrets(), NativeAPIConfig{
		ArchiveBucket: "archive", RestoreAuroraInstanceClass: "db.r6g.large", RestoreAuroraSubnetGroup: "aurora-subnets", RestorePubliclyAccessible: true, RestoreVPCSecurityGroupIDs: []string{"sg-aurora"}, RetentionDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	return rdsClient, client
}

func fakeAuroraCluster(id, arn, status string, members []rdstypes.DBClusterMember) rdstypes.DBCluster {
	return rdstypes.DBCluster{
		DBClusterIdentifier: aws.String(id), DBClusterArn: aws.String(arn), DBClusterMembers: members,
		Status: aws.String(status), Engine: aws.String("aurora-mysql"), EngineVersion: aws.String("8.0.mysql_aurora.3.08.0"),
		StorageEncrypted: aws.Bool(true), DeletionProtection: aws.Bool(true), BackupRetentionPeriod: aws.Int32(7),
	}
}

func fakeAuroraClusterSnapshot(id, marker, class string) rdstypes.DBClusterSnapshot {
	return rdstypes.DBClusterSnapshot{
		DBClusterSnapshotIdentifier: aws.String(id), DBClusterSnapshotArn: aws.String("arn:aws:rds:eu-west-1:123456789012:cluster-snapshot:" + id),
		Status: aws.String("available"), StorageEncrypted: aws.Bool(true), Engine: aws.String("aurora-mysql"), TagList: rdsTags(marker, class),
	}
}

func assertRDSControlPlaneOnlyEvidence(t *testing.T, evidence sdk.ResilienceProofEvidence) {
	t.Helper()
	if evidence.ManifestVerified || evidence.CountsVerified || evidence.ApplicationReadsVerified || evidence.PermissionsVerified || evidence.SecretReferencesVerified {
		t.Fatalf("RDS control-plane evidence claimed application verification: %#v", evidence)
	}
}

func containsInventoryIdentity(resources []sdk.ResilienceInventoryResource, want string) bool {
	for _, resource := range resources {
		if resource.Identity == want {
			return true
		}
	}
	return false
}

func TestNativeAPIRDSIntegrityRequiresVerifierAndAcceptsVerifiedEvidence(t *testing.T) {
	ctx := context.Background()
	rdsClient := newFakeRDS()
	arn := "arn:aws:rds:eu-west-1:123456789012:db:orders"
	rdsClient.instances["orders"] = rdstypes.DBInstance{
		DBInstanceIdentifier: aws.String("orders"), DBInstanceArn: aws.String(arn), DBInstanceStatus: aws.String("available"),
		Engine: aws.String("postgres"), StorageEncrypted: aws.Bool(true), DeletionProtection: aws.Bool(true),
	}
	rdsClient.tags[arn] = rdsTags("magelift/test/aws-integrity", "database")
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceIntegrityCheck, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/rds-integrity", OwnershipMarker: "magelift/test/aws-integrity", IdempotencyKey: "rds/integrity/1",
		ResourceReferences: map[string]string{"database": "aws-rds://instance/orders"},
	}

	withoutVerifier, err := NewNativeAPI(rdsClient, newFakeS3(), newFakeSecrets(), NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	withoutVerifierClient, err := NewNativeResilienceClient(withoutVerifier)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := withoutVerifierClient.Start(ctx, request); err == nil || !strings.Contains(err.Error(), "application-owned recovery verifier") {
		t.Fatalf("integrity without verifier error = %v", err)
	}

	withVerifier, err := NewNativeAPI(rdsClient, newFakeS3(), newFakeSecrets(), NativeAPIConfig{ArchiveBucket: "archive", Verifier: fakeRDSRecoveryVerifier{}})
	if err != nil {
		t.Fatal(err)
	}
	withVerifierClient, err := NewNativeResilienceClient(withVerifier)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := withVerifierClient.Start(ctx, request)
	if err != nil {
		t.Fatalf("integrity with verifier: %v", err)
	}
	if observation.Status != sdk.ResilienceOperationSucceeded || !observation.Evidence[0].ManifestVerified || !observation.Evidence[0].CountsVerified || !observation.Evidence[0].ApplicationReadsVerified || !observation.Evidence[0].PermissionsVerified || !observation.Evidence[0].SecretReferencesVerified {
		t.Fatalf("integrity evidence = %#v", observation)
	}
}

func TestNativeAPIRDSRefusesUnsupportedDestinationBeforeMutation(t *testing.T) {
	native, err := NewNativeAPI(newFakeRDS(), newFakeS3(), newFakeSecrets(), NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Start(context.Background(), sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoveryAlternateRegion,
		FixtureID: "fixture/rds-destination", OwnershipMarker: "magelift/test/aws-destination", IdempotencyKey: "rds/destination/1",
		ResourceReferences: map[string]string{"database": "aws-rds://instance/orders"},
	})
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported || capability.DataClass != "database" {
		t.Fatalf("error = %v, want typed unsupported destination refusal", err)
	}
}

func TestDescriptorRDSDatabaseDoesNotAdvertiseAlternateRegion(t *testing.T) {
	for _, capability := range Descriptor().DataClasses {
		if capability.Name != "database" {
			continue
		}
		for _, destination := range capability.Destinations {
			if destination == sdk.RecoveryAlternateRegion {
				t.Fatal("AWS RDS database descriptor advertises an unimplemented alternate-region destination")
			}
		}
		return
	}
	t.Fatal("AWS RDS database capability is missing")
}

func TestNativeAPIRDSInventoryAndCleanupPreserveSourceAndForeignResources(t *testing.T) {
	ctx := context.Background()
	marker := "magelift/test/aws-rds-cleanup"
	rdsClient := newFakeRDS()
	sourceARN := "arn:aws:rds:eu-west-1:123456789012:db:orders"
	rdsClient.instances["orders"] = rdstypes.DBInstance{
		DBInstanceIdentifier: aws.String("orders"), DBInstanceArn: aws.String(sourceARN),
		DBInstanceStatus: aws.String("available"), Engine: aws.String("mysql"), EngineVersion: aws.String("8.0"),
		StorageEncrypted: aws.Bool(true), DeletionProtection: aws.Bool(true),
	}
	rdsClient.tags[sourceARN] = rdsTags(marker, "database")
	foreignARN := "arn:aws:rds:eu-west-1:123456789012:db:foreign"
	rdsClient.instances["foreign"] = rdstypes.DBInstance{
		DBInstanceIdentifier: aws.String("foreign"), DBInstanceArn: aws.String(foreignARN),
		DBInstanceStatus: aws.String("available"), StorageEncrypted: aws.Bool(true), DeletionProtection: aws.Bool(true),
	}
	rdsClient.tags[foreignARN] = rdsTags("magelift/test/foreign", "database")
	native, err := NewNativeAPI(rdsClient, newFakeS3(), newFakeSecrets(), NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	base := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/rds-cleanup", OwnershipMarker: marker, IdempotencyKey: "rds/cleanup/backup",
		ResourceReferences: map[string]string{"database": "aws-rds://instance/orders"},
	}
	backup, err := client.Start(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if backup.Status != sdk.ResilienceOperationPending {
		t.Fatalf("backup start = %#v", backup)
	}
	backup, err = client.Poll(ctx, backup.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	restore := base
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "rds/cleanup/restore"
	restore.BackupReferences = map[string]string{"database": backup.Evidence[0].BackupID}
	restored, err := client.Start(ctx, restore)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Poll(ctx, restored.OperationID); err != nil {
		t.Fatal(err)
	}
	owned, err := native.Inventory(ctx, marker)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 3 {
		t.Fatalf("owned RDS inventory = %#v, want source, snapshot, and restore", owned)
	}
	foreign, err := native.Inventory(ctx, "magelift/test/foreign")
	if err != nil {
		t.Fatal(err)
	}
	if len(foreign) != 1 || foreign[0].Identity != "aws-rds://instance/foreign" {
		t.Fatalf("foreign RDS inventory = %#v", foreign)
	}

	cleanup := base
	cleanup.Action = sdk.ResilienceCleanup
	cleanup.IdempotencyKey = "rds/cleanup/delete"
	firstCleanup, err := client.Start(ctx, cleanup)
	if err != nil {
		t.Fatal(err)
	}
	if firstCleanup.Status != sdk.ResilienceOperationPending {
		t.Fatalf("cleanup start = %#v", firstCleanup)
	}
	completedCleanup, err := client.Poll(ctx, firstCleanup.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if completedCleanup.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("cleanup completion = %#v", completedCleanup)
	}
	remaining, err := native.Inventory(ctx, marker)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Identity != "aws-rds://instance/orders" {
		t.Fatalf("RDS cleanup deleted or hid the source: %#v", remaining)
	}
	foreign, err = native.Inventory(ctx, "magelift/test/foreign")
	if err != nil {
		t.Fatal(err)
	}
	if len(foreign) != 1 || foreign[0].Identity != "aws-rds://instance/foreign" {
		t.Fatalf("RDS cleanup touched foreign resources: %#v", foreign)
	}
}

func TestNativeAPIRejectsUnsupportedQueueBeforeMutation(t *testing.T) {
	native, err := NewNativeAPI(newFakeRDS(), newFakeS3(), newFakeSecrets(), NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), cloudresilience.NativeOperationRequest{
		Provider: "aws", Operation: "aws.queue.backup.queue", Request: sdk.ResilienceOperationRequest{
			Action: sdk.ResilienceBackup, DataClasses: []string{"queue"}, FixtureID: "fixture", OwnershipMarker: "marker", IdempotencyKey: "key",
			ResourceReferences: map[string]string{"queue": "aws-sqs://queue"},
		},
	})
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
	native, err := NewNativeAPI(newFakeRDS(), newFakeS3(), newFakeSecrets(), NativeAPIConfig{ArchiveBucket: "archive", Projection: projection})
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
			FixtureID: "fixture/projection", OwnershipMarker: "magelift/test/aws/projection", IdempotencyKey: "projection/" + dataClass,
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
	result := cloudrecovery.ProjectionResult{
		Status: sdk.ResilienceOperationSucceeded, ResourceReference: request.TargetReference,
		FixtureID: request.FixtureID, OwnershipMarker: request.OwnershipMarker, IdempotencyVerified: true,
		CountsVerified:           request.DataClass == cloudrecovery.ProjectionSearchIndex,
		ApplicationReadsVerified: true, PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		CacheLossClassified: request.DataClass == cloudrecovery.ProjectionCache, RestoreDurationSeconds: 3,
		Reason: "fake runtime verified known projection content",
	}
	return result
}

type fakeObject struct {
	body       []byte
	metadata   map[string]string
	encryption s3types.ServerSideEncryption
}

type fakeS3 struct {
	objects map[string]fakeObject
	copies  int
}

func newFakeS3() *fakeS3 { return &fakeS3{objects: make(map[string]fakeObject)} }

func fakeObjectKey(bucket, key string) string { return bucket + "\x00" + key }

func (f *fakeS3) ListObjectsV2(_ context.Context, input *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	prefix := aws.ToString(input.Prefix)
	keys := make([]string, 0)
	for composite := range f.objects {
		parts := strings.SplitN(composite, "\x00", 2)
		if len(parts) == 2 && parts[0] == aws.ToString(input.Bucket) && strings.HasPrefix(parts[1], prefix) {
			keys = append(keys, parts[1])
		}
	}
	sort.Strings(keys)
	contents := make([]s3types.Object, 0, len(keys))
	for _, key := range keys {
		object := f.objects[fakeObjectKey(aws.ToString(input.Bucket), key)]
		contents = append(contents, s3types.Object{Key: aws.String(key), Size: aws.Int64(int64(len(object.body))), ETag: aws.String("etag")})
	}
	return &s3.ListObjectsV2Output{Contents: contents, IsTruncated: aws.Bool(false)}, nil
}

func (f *fakeS3) HeadObject(_ context.Context, input *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	object, ok := f.objects[fakeObjectKey(aws.ToString(input.Bucket), aws.ToString(input.Key))]
	if !ok {
		return nil, errors.New("NotFound")
	}
	return &s3.HeadObjectOutput{ContentLength: aws.Int64(int64(len(object.body))), Metadata: cloneMetadata(object.metadata), ServerSideEncryption: object.encryption}, nil
}

func (f *fakeS3) GetObject(_ context.Context, input *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	object, ok := f.objects[fakeObjectKey(aws.ToString(input.Bucket), aws.ToString(input.Key))]
	if !ok {
		return nil, errors.New("NoSuchKey")
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(object.body))}, nil
}

func (f *fakeS3) PutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	body, err := io.ReadAll(input.Body)
	if err != nil {
		return nil, err
	}
	encryption := input.ServerSideEncryption
	if encryption == "" {
		encryption = s3types.ServerSideEncryptionAes256
	}
	f.objects[fakeObjectKey(aws.ToString(input.Bucket), aws.ToString(input.Key))] = fakeObject{body: body, metadata: cloneMetadata(input.Metadata), encryption: encryption}
	return &s3.PutObjectOutput{}, nil
}

func (f *fakeS3) CopyObject(_ context.Context, input *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	source, err := url.PathUnescape(aws.ToString(input.CopySource))
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(source, "/", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid copy source")
	}
	object, ok := f.objects[fakeObjectKey(parts[0], parts[1])]
	if !ok {
		return nil, errors.New("NoSuchKey")
	}
	encryption := input.ServerSideEncryption
	if encryption == "" {
		encryption = s3types.ServerSideEncryptionAes256
	}
	f.objects[fakeObjectKey(aws.ToString(input.Bucket), aws.ToString(input.Key))] = fakeObject{body: append([]byte(nil), object.body...), metadata: cloneMetadata(input.Metadata), encryption: encryption}
	f.copies++
	return &s3.CopyObjectOutput{}, nil
}

func (f *fakeS3) DeleteObject(_ context.Context, input *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	delete(f.objects, fakeObjectKey(aws.ToString(input.Bucket), aws.ToString(input.Key)))
	return &s3.DeleteObjectOutput{}, nil
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	clone := make(map[string]string, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

type fakeRDS struct {
	instances               map[string]rdstypes.DBInstance
	clusters                map[string]rdstypes.DBCluster
	snapshots               map[string]rdstypes.DBSnapshot
	clusterSnapshots        map[string]rdstypes.DBClusterSnapshot
	tags                    map[string][]rdstypes.Tag
	events                  []string
	lastRestoreInput        *rds.RestoreDBInstanceFromDBSnapshotInput
	lastRestoreClusterInput *rds.RestoreDBClusterFromSnapshotInput
	lastCreateInstanceInput *rds.CreateDBInstanceInput
}

type fakeRDSRecoveryVerifier struct{}

func (fakeRDSRecoveryVerifier) Verify(context.Context, RecoveryVerificationRequest) (RecoveryVerification, error) {
	return RecoveryVerification{
		ManifestVerified: true, CountsVerified: true, ApplicationReadsVerified: true,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
	}, nil
}

func newFakeRDS() *fakeRDS {
	return &fakeRDS{instances: make(map[string]rdstypes.DBInstance), clusters: make(map[string]rdstypes.DBCluster), snapshots: make(map[string]rdstypes.DBSnapshot), clusterSnapshots: make(map[string]rdstypes.DBClusterSnapshot), tags: make(map[string][]rdstypes.Tag)}
}

func (f *fakeRDS) CreateDBSnapshot(_ context.Context, input *rds.CreateDBSnapshotInput, _ ...func(*rds.Options)) (*rds.CreateDBSnapshotOutput, error) {
	instance, ok := f.instances[aws.ToString(input.DBInstanceIdentifier)]
	if !ok {
		return nil, errors.New("DBInstanceNotFound")
	}
	snapshot := rdstypes.DBSnapshot{DBSnapshotIdentifier: input.DBSnapshotIdentifier, DBSnapshotArn: aws.String("arn:aws:rds:eu-west-1:123456789012:snapshot:" + aws.ToString(input.DBSnapshotIdentifier)), Status: aws.String("available"), Encrypted: instance.StorageEncrypted, Engine: instance.Engine, TagList: input.Tags}
	f.snapshots[aws.ToString(input.DBSnapshotIdentifier)] = snapshot
	return &rds.CreateDBSnapshotOutput{DBSnapshot: &snapshot}, nil
}

func (f *fakeRDS) DescribeDBSnapshots(_ context.Context, input *rds.DescribeDBSnapshotsInput, _ ...func(*rds.Options)) (*rds.DescribeDBSnapshotsOutput, error) {
	if aws.ToString(input.DBSnapshotIdentifier) == "" {
		values := make([]rdstypes.DBSnapshot, 0, len(f.snapshots))
		for _, snapshot := range f.snapshots {
			values = append(values, snapshot)
		}
		sort.Slice(values, func(i, j int) bool {
			return aws.ToString(values[i].DBSnapshotIdentifier) < aws.ToString(values[j].DBSnapshotIdentifier)
		})
		return &rds.DescribeDBSnapshotsOutput{DBSnapshots: values}, nil
	}
	snapshot, ok := f.snapshots[aws.ToString(input.DBSnapshotIdentifier)]
	if !ok {
		return nil, errors.New("DBSnapshotNotFound")
	}
	return &rds.DescribeDBSnapshotsOutput{DBSnapshots: []rdstypes.DBSnapshot{snapshot}}, nil
}

func (f *fakeRDS) DeleteDBSnapshot(_ context.Context, input *rds.DeleteDBSnapshotInput, _ ...func(*rds.Options)) (*rds.DeleteDBSnapshotOutput, error) {
	id := aws.ToString(input.DBSnapshotIdentifier)
	snapshot, ok := f.snapshots[id]
	if !ok {
		return nil, errors.New("DBSnapshotNotFound")
	}
	delete(f.snapshots, id)
	return &rds.DeleteDBSnapshotOutput{DBSnapshot: &snapshot}, nil
}

func (f *fakeRDS) CreateDBClusterSnapshot(_ context.Context, input *rds.CreateDBClusterSnapshotInput, _ ...func(*rds.Options)) (*rds.CreateDBClusterSnapshotOutput, error) {
	cluster, ok := f.clusters[aws.ToString(input.DBClusterIdentifier)]
	if !ok {
		return nil, errors.New("DBClusterNotFound")
	}
	snapshot := rdstypes.DBClusterSnapshot{DBClusterSnapshotIdentifier: input.DBClusterSnapshotIdentifier, DBClusterSnapshotArn: aws.String("arn:aws:rds:eu-west-1:123456789012:cluster-snapshot:" + aws.ToString(input.DBClusterSnapshotIdentifier)), Status: aws.String("available"), StorageEncrypted: cluster.StorageEncrypted, Engine: cluster.Engine, TagList: input.Tags}
	f.clusterSnapshots[aws.ToString(input.DBClusterSnapshotIdentifier)] = snapshot
	f.events = append(f.events, "create-cluster-snapshot:"+aws.ToString(input.DBClusterSnapshotIdentifier))
	return &rds.CreateDBClusterSnapshotOutput{DBClusterSnapshot: &snapshot}, nil
}

func (f *fakeRDS) DescribeDBClusterSnapshots(_ context.Context, input *rds.DescribeDBClusterSnapshotsInput, _ ...func(*rds.Options)) (*rds.DescribeDBClusterSnapshotsOutput, error) {
	if aws.ToString(input.DBClusterSnapshotIdentifier) == "" {
		values := make([]rdstypes.DBClusterSnapshot, 0, len(f.clusterSnapshots))
		for _, snapshot := range f.clusterSnapshots {
			values = append(values, snapshot)
		}
		sort.Slice(values, func(i, j int) bool {
			return aws.ToString(values[i].DBClusterSnapshotIdentifier) < aws.ToString(values[j].DBClusterSnapshotIdentifier)
		})
		return &rds.DescribeDBClusterSnapshotsOutput{DBClusterSnapshots: values}, nil
	}
	snapshot, ok := f.clusterSnapshots[aws.ToString(input.DBClusterSnapshotIdentifier)]
	if !ok {
		return nil, errors.New("DBClusterSnapshotNotFound")
	}
	return &rds.DescribeDBClusterSnapshotsOutput{DBClusterSnapshots: []rdstypes.DBClusterSnapshot{snapshot}}, nil
}

func (f *fakeRDS) DeleteDBClusterSnapshot(_ context.Context, input *rds.DeleteDBClusterSnapshotInput, _ ...func(*rds.Options)) (*rds.DeleteDBClusterSnapshotOutput, error) {
	id := aws.ToString(input.DBClusterSnapshotIdentifier)
	snapshot, ok := f.clusterSnapshots[id]
	if !ok {
		return nil, errors.New("DBClusterSnapshotNotFound")
	}
	delete(f.clusterSnapshots, id)
	f.events = append(f.events, "delete-cluster-snapshot:"+id)
	return &rds.DeleteDBClusterSnapshotOutput{DBClusterSnapshot: &snapshot}, nil
}

func (f *fakeRDS) DescribeDBInstances(_ context.Context, input *rds.DescribeDBInstancesInput, _ ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error) {
	if aws.ToString(input.DBInstanceIdentifier) == "" {
		values := make([]rdstypes.DBInstance, 0, len(f.instances))
		for _, instance := range f.instances {
			values = append(values, instance)
		}
		sort.Slice(values, func(i, j int) bool {
			return aws.ToString(values[i].DBInstanceIdentifier) < aws.ToString(values[j].DBInstanceIdentifier)
		})
		return &rds.DescribeDBInstancesOutput{DBInstances: values}, nil
	}
	instance, ok := f.instances[aws.ToString(input.DBInstanceIdentifier)]
	if !ok {
		return nil, errors.New("DBInstanceNotFound")
	}
	return &rds.DescribeDBInstancesOutput{DBInstances: []rdstypes.DBInstance{instance}}, nil
}

func (f *fakeRDS) DeleteDBInstance(_ context.Context, input *rds.DeleteDBInstanceInput, _ ...func(*rds.Options)) (*rds.DeleteDBInstanceOutput, error) {
	id := aws.ToString(input.DBInstanceIdentifier)
	instance, ok := f.instances[id]
	if !ok {
		return nil, errors.New("DBInstanceNotFound")
	}
	f.events = append(f.events, "delete-instance:"+id)
	if aws.ToBool(instance.DeletionProtection) {
		return nil, errors.New("InvalidDBInstanceState: deletion protection is enabled")
	}
	delete(f.instances, id)
	delete(f.tags, aws.ToString(instance.DBInstanceArn))
	clusterID := aws.ToString(instance.DBClusterIdentifier)
	if cluster, ok := f.clusters[clusterID]; ok {
		members := make([]rdstypes.DBClusterMember, 0, len(cluster.DBClusterMembers))
		for _, member := range cluster.DBClusterMembers {
			if aws.ToString(member.DBInstanceIdentifier) != id {
				members = append(members, member)
			}
		}
		cluster.DBClusterMembers = members
		f.clusters[clusterID] = cluster
	}
	return &rds.DeleteDBInstanceOutput{DBInstance: &instance}, nil
}

func (f *fakeRDS) ModifyDBInstance(_ context.Context, input *rds.ModifyDBInstanceInput, _ ...func(*rds.Options)) (*rds.ModifyDBInstanceOutput, error) {
	id := aws.ToString(input.DBInstanceIdentifier)
	instance, ok := f.instances[id]
	if !ok {
		return nil, errors.New("DBInstanceNotFound")
	}
	f.events = append(f.events, "modify-instance:"+id)
	instance.DeletionProtection = input.DeletionProtection
	f.instances[id] = instance
	return &rds.ModifyDBInstanceOutput{DBInstance: &instance}, nil
}

func (f *fakeRDS) DescribeDBClusters(_ context.Context, input *rds.DescribeDBClustersInput, _ ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error) {
	if aws.ToString(input.DBClusterIdentifier) == "" {
		values := make([]rdstypes.DBCluster, 0, len(f.clusters))
		for _, cluster := range f.clusters {
			values = append(values, cluster)
		}
		sort.Slice(values, func(i, j int) bool {
			return aws.ToString(values[i].DBClusterIdentifier) < aws.ToString(values[j].DBClusterIdentifier)
		})
		return &rds.DescribeDBClustersOutput{DBClusters: values}, nil
	}
	cluster, ok := f.clusters[aws.ToString(input.DBClusterIdentifier)]
	if !ok {
		return nil, errors.New("DBClusterNotFound")
	}
	return &rds.DescribeDBClustersOutput{DBClusters: []rdstypes.DBCluster{cluster}}, nil
}

func (f *fakeRDS) DeleteDBCluster(_ context.Context, input *rds.DeleteDBClusterInput, _ ...func(*rds.Options)) (*rds.DeleteDBClusterOutput, error) {
	id := aws.ToString(input.DBClusterIdentifier)
	cluster, ok := f.clusters[id]
	if !ok {
		return nil, errors.New("DBClusterNotFound")
	}
	f.events = append(f.events, "delete-cluster:"+id)
	if aws.ToBool(cluster.DeletionProtection) {
		return nil, errors.New("InvalidDBClusterState: deletion protection is enabled")
	}
	if len(cluster.DBClusterMembers) > 0 {
		return nil, errors.New("InvalidDBClusterState: DB cluster still has members")
	}
	delete(f.clusters, id)
	delete(f.tags, aws.ToString(cluster.DBClusterArn))
	return &rds.DeleteDBClusterOutput{DBCluster: &cluster}, nil
}

func (f *fakeRDS) ModifyDBCluster(_ context.Context, input *rds.ModifyDBClusterInput, _ ...func(*rds.Options)) (*rds.ModifyDBClusterOutput, error) {
	id := aws.ToString(input.DBClusterIdentifier)
	cluster, ok := f.clusters[id]
	if !ok {
		return nil, errors.New("DBClusterNotFound")
	}
	f.events = append(f.events, "modify-cluster:"+id)
	cluster.DeletionProtection = input.DeletionProtection
	f.clusters[id] = cluster
	return &rds.ModifyDBClusterOutput{DBCluster: &cluster}, nil
}

func (f *fakeRDS) RestoreDBInstanceFromDBSnapshot(_ context.Context, input *rds.RestoreDBInstanceFromDBSnapshotInput, _ ...func(*rds.Options)) (*rds.RestoreDBInstanceFromDBSnapshotOutput, error) {
	f.lastRestoreInput = input
	snapshot, ok := f.snapshots[aws.ToString(input.DBSnapshotIdentifier)]
	if !ok {
		return nil, errors.New("DBSnapshotNotFound")
	}
	arn := "arn:aws:rds:eu-west-1:123456789012:db:" + aws.ToString(input.DBInstanceIdentifier)
	f.instances[aws.ToString(input.DBInstanceIdentifier)] = rdstypes.DBInstance{DBInstanceIdentifier: input.DBInstanceIdentifier, DBInstanceArn: aws.String(arn), DBInstanceStatus: aws.String("available"), Engine: snapshot.Engine, StorageEncrypted: snapshot.Encrypted, DeletionProtection: input.DeletionProtection}
	f.tags[arn] = input.Tags
	instance := f.instances[aws.ToString(input.DBInstanceIdentifier)]
	return &rds.RestoreDBInstanceFromDBSnapshotOutput{DBInstance: &instance}, nil
}

func (f *fakeRDS) RestoreDBClusterFromSnapshot(_ context.Context, input *rds.RestoreDBClusterFromSnapshotInput, _ ...func(*rds.Options)) (*rds.RestoreDBClusterFromSnapshotOutput, error) {
	f.lastRestoreClusterInput = input
	snapshotID := aws.ToString(input.SnapshotIdentifier)
	snapshot, ok := f.clusterSnapshots[snapshotID]
	if !ok {
		return nil, errors.New("DBClusterSnapshotNotFound")
	}
	id := aws.ToString(input.DBClusterIdentifier)
	if _, ok := f.clusters[id]; ok {
		return nil, errors.New("DBClusterAlreadyExists")
	}
	arn := "arn:aws:rds:eu-west-1:123456789012:cluster:" + id
	cluster := rdstypes.DBCluster{
		DBClusterIdentifier: aws.String(id), DBClusterArn: aws.String(arn), Status: aws.String("creating"),
		Engine: snapshot.Engine, EngineVersion: snapshot.EngineVersion, StorageEncrypted: snapshot.StorageEncrypted,
		DeletionProtection: input.DeletionProtection, DBSubnetGroup: input.DBSubnetGroupName, BackupRetentionPeriod: input.BackupRetentionPeriod,
	}
	f.clusters[id] = cluster
	f.tags[arn] = append([]rdstypes.Tag(nil), input.Tags...)
	f.events = append(f.events, "restore-cluster:"+id)
	return &rds.RestoreDBClusterFromSnapshotOutput{DBCluster: &cluster}, nil
}

func (f *fakeRDS) CreateDBInstance(_ context.Context, input *rds.CreateDBInstanceInput, _ ...func(*rds.Options)) (*rds.CreateDBInstanceOutput, error) {
	f.lastCreateInstanceInput = input
	id := aws.ToString(input.DBInstanceIdentifier)
	clusterID := aws.ToString(input.DBClusterIdentifier)
	cluster, ok := f.clusters[clusterID]
	if !ok {
		return nil, errors.New("DBClusterNotFound")
	}
	if _, ok := f.instances[id]; ok {
		return nil, errors.New("DBInstanceAlreadyExists")
	}
	arn := "arn:aws:rds:eu-west-1:123456789012:db:" + id
	instance := rdstypes.DBInstance{
		DBInstanceIdentifier: aws.String(id), DBInstanceArn: aws.String(arn), DBInstanceStatus: aws.String("creating"),
		Engine: input.Engine, EngineVersion: cluster.EngineVersion, DBClusterIdentifier: input.DBClusterIdentifier,
		DBInstanceClass: input.DBInstanceClass, StorageEncrypted: cluster.StorageEncrypted, DeletionProtection: input.DeletionProtection,
	}
	f.instances[id] = instance
	members := append([]rdstypes.DBClusterMember(nil), cluster.DBClusterMembers...)
	members = append(members, rdstypes.DBClusterMember{DBInstanceIdentifier: aws.String(id), IsClusterWriter: aws.Bool(true)})
	cluster.DBClusterMembers = members
	f.clusters[clusterID] = cluster
	f.tags[arn] = append([]rdstypes.Tag(nil), input.Tags...)
	f.events = append(f.events, "create-instance:"+id)
	return &rds.CreateDBInstanceOutput{DBInstance: &instance}, nil
}

func (f *fakeRDS) ListTagsForResource(_ context.Context, input *rds.ListTagsForResourceInput, _ ...func(*rds.Options)) (*rds.ListTagsForResourceOutput, error) {
	return &rds.ListTagsForResourceOutput{TagList: append([]rdstypes.Tag(nil), f.tags[aws.ToString(input.ResourceName)]...)}, nil
}

type fakeSecrets struct {
	values       map[string]secretsmanager.GetSecretValueOutput
	descriptions map[string]secretsmanager.DescribeSecretOutput
}

func newFakeSecrets() *fakeSecrets {
	return &fakeSecrets{values: make(map[string]secretsmanager.GetSecretValueOutput), descriptions: make(map[string]secretsmanager.DescribeSecretOutput)}
}

func (f *fakeSecrets) GetSecretValue(_ context.Context, input *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	value, ok := f.values[aws.ToString(input.SecretId)]
	if !ok {
		return nil, errors.New("ResourceNotFoundException")
	}
	return &value, nil
}

func (f *fakeSecrets) DescribeSecret(_ context.Context, input *secretsmanager.DescribeSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error) {
	description, ok := f.descriptions[aws.ToString(input.SecretId)]
	if !ok {
		return nil, errors.New("ResourceNotFoundException")
	}
	return &description, nil
}

func (f *fakeSecrets) CreateSecret(_ context.Context, input *secretsmanager.CreateSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error) {
	name := aws.ToString(input.Name)
	f.descriptions[name] = secretsmanager.DescribeSecretOutput{Name: aws.String(name), Tags: input.Tags}
	f.values[name] = secretsmanager.GetSecretValueOutput{SecretString: input.SecretString, SecretBinary: input.SecretBinary}
	return &secretsmanager.CreateSecretOutput{Name: aws.String(name)}, nil
}

func (f *fakeSecrets) PutSecretValue(_ context.Context, input *secretsmanager.PutSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error) {
	f.values[aws.ToString(input.SecretId)] = secretsmanager.GetSecretValueOutput{SecretString: input.SecretString, SecretBinary: input.SecretBinary}
	return &secretsmanager.PutSecretValueOutput{}, nil
}

func (f *fakeSecrets) DeleteSecret(_ context.Context, input *secretsmanager.DeleteSecretInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error) {
	name := aws.ToString(input.SecretId)
	if _, ok := f.descriptions[name]; !ok {
		return nil, errors.New("ResourceNotFoundException")
	}
	delete(f.descriptions, name)
	delete(f.values, name)
	return &secretsmanager.DeleteSecretOutput{Name: aws.String(name)}, nil
}

func (f *fakeSecrets) ListSecrets(context.Context, *secretsmanager.ListSecretsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error) {
	names := make([]string, 0, len(f.descriptions))
	for name := range f.descriptions {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([]secretstypes.SecretListEntry, 0, len(names))
	for _, name := range names {
		description := f.descriptions[name]
		entries = append(entries, secretstypes.SecretListEntry{Name: aws.String(name), ARN: aws.String("arn:aws:secretsmanager:eu-west-1:123456789012:secret:" + name), Tags: description.Tags})
	}
	return &secretsmanager.ListSecretsOutput{SecretList: entries}, nil
}

func rdsTags(marker, class string) []rdstypes.Tag {
	return []rdstypes.Tag{{Key: aws.String(ownershipTagKey), Value: aws.String(marker)}, {Key: aws.String(classTagKey), Value: aws.String(class)}}
}
