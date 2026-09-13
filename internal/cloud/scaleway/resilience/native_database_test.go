package resilience

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestNativeAPIDatabaseRecoveryIsResumableIdempotentAndOwned(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	database := newFakeScalewayDatabase()
	database.instances["source"] = DatabaseInstance{
		ID: "source", Name: "source", Region: "fr-par", Status: "ready", NodeType: "db-gp1-small",
		VolumeType: "bssd", IsHA: true, EncryptionEnabled: true,
		Tags: []string{scalewayDatabaseOwnershipTag + "=owner", scalewayDatabaseClassTag + "=database"},
	}
	native, err := NewNativeAPIWithDatabase(newFakeScalewayS3(), database, NativeAPIConfig{
		ArchiveBucket: "archive", DatabaseRegion: "fr-par", RequireDatabaseHA: true, RetentionDays: 7,
		Now: func() time.Time { return now },
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
		FixtureID: "fixture/known-content", OwnershipMarker: "owner", IdempotencyKey: "database/backup/1",
		ResourceReferences: map[string]string{"database": "scaleway-rdb://source"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup start: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationPending || database.createSnapshots != 1 {
		t.Fatalf("backup start = %#v creates=%d", backup, database.createSnapshots)
	}
	snapshot := database.snapshots["snapshot-1"]
	snapshot.Status = "ready"
	database.snapshots[snapshot.ID] = snapshot
	completed, err := client.Poll(context.Background(), backup.OperationID)
	if err != nil {
		t.Fatalf("backup poll: %v", err)
	}
	if completed.Status != sdk.ResilienceOperationSucceeded || len(completed.Evidence) != 1 || !completed.Evidence[0].EncryptionVerified || completed.Evidence[0].ProtectionVerified {
		t.Fatalf("backup completion = %#v", completed)
	}
	second, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("idempotent backup: %v", err)
	}
	if second.Status != sdk.ResilienceOperationSucceeded || database.createSnapshots != 1 {
		t.Fatalf("idempotent backup = %#v creates=%d", second, database.createSnapshots)
	}

	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "database/restore/1"
	restore.BackupReferences = map[string]string{"database": completed.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil {
		t.Fatalf("restore start: %v", err)
	}
	if restored.Status != sdk.ResilienceOperationPending || database.createRestores != 1 {
		t.Fatalf("restore start = %#v restores=%d", restored, database.createRestores)
	}
	targetID := database.lastRestoreID
	database.instances[targetID] = DatabaseInstance{
		ID: targetID, Name: database.instances[targetID].Name, Region: "fr-par", Status: "ready", NodeType: "db-gp1-small",
		VolumeType: "bssd", IsHA: true, EncryptionEnabled: true,
		Tags: scalewayDatabaseTags(nil, operationState{OwnershipMarker: "owner", DataClass: "database"}),
	}
	restored, err = client.Poll(context.Background(), restored.OperationID)
	if err != nil {
		t.Fatalf("restore poll: %v", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || !restored.Evidence[0].ServiceHealthVerified || restored.Evidence[0].ProtectionVerified {
		t.Fatalf("restore completion = %#v", restored)
	}

	verifier := fakeScalewayRecoveryVerifier{}
	native.config.Verifier = verifier
	integrity := restore
	integrity.Action = sdk.ResilienceIntegrityCheck
	integrity.IdempotencyKey = "database/integrity/1"
	integrity.ResourceReferences = map[string]string{"database": "scaleway-rdb://" + targetID}
	checked, err := client.Start(context.Background(), integrity)
	if err != nil {
		t.Fatalf("integrity: %v", err)
	}
	if checked.Status != sdk.ResilienceOperationSucceeded || !checked.Evidence[0].ApplicationReadsVerified || !checked.Evidence[0].PermissionsVerified {
		t.Fatalf("integrity = %#v", checked)
	}

	inventory, err := client.Inventory(context.Background(), "owner")
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	if len(inventory) < 2 {
		t.Fatalf("inventory = %#v", inventory)
	}
}

func TestNativeAPIDatabaseRejectsLogicalBackupAndAlternateRegion(t *testing.T) {
	database := newFakeScalewayDatabase()
	database.instances["source"] = DatabaseInstance{
		ID: "source", Name: "source", Region: "fr-par", Status: "ready", VolumeType: "lssd", EncryptionEnabled: true,
		Tags: []string{scalewayDatabaseOwnershipTag + "=owner", scalewayDatabaseClassTag + "=database"},
	}
	native, err := NewNativeAPIWithDatabase(newFakeScalewayS3(), database, NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	request := cloudresilience.NativeOperationRequest{
		Provider: "scaleway", Operation: "scaleway.managed-database.backup.database",
		Request: sdk.ResilienceOperationRequest{Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, FixtureID: "fixture", OwnershipMarker: "owner", IdempotencyKey: "backup", ResourceReferences: map[string]string{"database": "scaleway-rdb://source"}},
	}
	_, err = native.Start(context.Background(), request)
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("logical backup error = %v, want unsupported capability", err)
	}
	if database.createSnapshots != 0 {
		t.Fatalf("logical backup mutated snapshots: %d", database.createSnapshots)
	}

	source := database.instances["source"]
	source.VolumeType = "bssd"
	source.EncryptionEnabled = true
	database.instances["source"] = source
	database.snapshots["snapshot-1"] = DatabaseSnapshot{ID: "snapshot-1", InstanceID: "source", Name: scalewayDatabaseNamePrefix + shortDigest("owner") + "-backup", Region: "fr-par", Status: "ready", VolumeType: "bssd", ExpiresAt: time.Now().Add(24 * time.Hour)}
	restore := request
	restore.Operation = "scaleway.managed-database.restore.database"
	restore.Request.Action = sdk.ResilienceRestore
	restore.Request.Destination = sdk.RecoveryAlternateRegion
	restore.Request.IdempotencyKey = "restore"
	restore.Request.BackupReferences = map[string]string{"database": "scaleway-rdb-snapshot://snapshot-1"}
	_, err = native.Start(context.Background(), restore)
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("alternate-region restore error = %v, want unsupported capability", err)
	}
}

func TestNativeAPIDatabaseReconcilesAmbiguousRestoreCreation(t *testing.T) {
	database := newFakeScalewayDatabase()
	database.instances["source"] = DatabaseInstance{
		ID: "source", Name: "source", Region: "fr-par", Status: "ready", VolumeType: "bssd", EncryptionEnabled: true,
		Tags: []string{scalewayDatabaseOwnershipTag + "=owner", scalewayDatabaseClassTag + "=database"},
	}
	database.snapshots["snapshot-1"] = DatabaseSnapshot{
		ID: "snapshot-1", InstanceID: "source", Name: scalewayDatabaseNamePrefix + shortDigest("owner") + "-backup",
		Region: "fr-par", Status: "ready", VolumeType: "bssd", ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	database.createRestoreErr = context.Canceled
	native, err := NewNativeAPIWithDatabase(newFakeScalewayS3(), database, NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}

	request := cloudresilience.NativeOperationRequest{
		Provider: "scaleway", Operation: "scaleway.managed-database.restore.database",
		Request: sdk.ResilienceOperationRequest{
			Action: sdk.ResilienceRestore, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegionIsolated,
			FixtureID: "fixture", OwnershipMarker: "owner", IdempotencyKey: "restore-ambiguous",
			ResourceReferences: map[string]string{"database": "scaleway-rdb://source"},
			BackupReferences:   map[string]string{"database": "scaleway-rdb-snapshot://snapshot-1"},
		},
	}
	observation, err := native.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("ambiguous restore: %v", err)
	}
	if observation.Status != string(sdk.ResilienceOperationPending) {
		t.Fatalf("ambiguous restore status = %s, want pending", observation.Status)
	}
	target := database.instances[database.lastRestoreID]
	if !scalewayDatabaseRestoreOwned(target, operationState{OwnershipMarker: "owner", DataClass: "database"}) {
		t.Fatalf("ambiguous restore target was not claimed: %#v", target)
	}
}

func TestNativeAPIDatabaseRestoreStaysPendingWhileTagWouldHitTransientState(t *testing.T) {
	database := newFakeScalewayDatabase()
	database.instances["source"] = DatabaseInstance{
		ID: "source", Name: "source", Region: "fr-par", Status: "ready", VolumeType: "bssd", EncryptionEnabled: true,
		Tags: []string{scalewayDatabaseOwnershipTag + "=owner", scalewayDatabaseClassTag + "=database"},
	}
	database.snapshots["snapshot-1"] = DatabaseSnapshot{
		ID: "snapshot-1", InstanceID: "source", Name: scalewayDatabaseNamePrefix + shortDigest("owner") + "-backup",
		Region: "fr-par", Status: "ready", VolumeType: "bssd", ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	database.updateTagsErr = errors.New("scaleway-sdk-go: resource instance with ID restore-1 is in a transient state: provisioning")
	native, err := NewNativeAPIWithDatabase(newFakeScalewayS3(), database, NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceRestore, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture", OwnershipMarker: "owner", IdempotencyKey: "restore-transient",
		ResourceReferences: map[string]string{"database": "scaleway-rdb://source"},
		BackupReferences:   map[string]string{"database": "scaleway-rdb-snapshot://snapshot-1"},
	}
	started, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("provisioning restore start: %v", err)
	}
	if started.Status != sdk.ResilienceOperationPending {
		t.Fatalf("provisioning restore status = %s, want pending", started.Status)
	}
	target := database.instances[database.lastRestoreID]
	target.Status = "ready"
	target.EncryptionEnabled = true
	database.instances[target.ID] = target
	database.updateTagsErr = nil
	completed, err := client.Poll(context.Background(), started.OperationID)
	if err != nil {
		t.Fatalf("restore poll after ready: %v", err)
	}
	if completed.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("restore poll status = %s detail=%q", completed.Status, completed.Detail)
	}
	if !scalewayDatabaseRestoreOwned(database.instances[target.ID], operationState{OwnershipMarker: "owner", DataClass: "database"}) {
		t.Fatalf("restore poll did not tag the ready target: %#v", database.instances[target.ID])
	}
}

func TestNativeAPIDatabaseBackupKeepsRetentionAfterElapsedPoll(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	database := newFakeScalewayDatabase()
	database.instances["source"] = DatabaseInstance{
		ID: "source", Name: "source", Region: "fr-par", Status: "ready", VolumeType: "bssd", EncryptionEnabled: true,
		Tags: []string{scalewayDatabaseOwnershipTag + "=owner", scalewayDatabaseClassTag + "=database"},
	}
	native, err := NewNativeAPIWithDatabase(newFakeScalewayS3(), database, NativeAPIConfig{
		ArchiveBucket: "archive", DatabaseRegion: "fr-par", RetentionDays: 1,
		Now: func() time.Time { return now },
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
		FixtureID: "fixture", OwnershipMarker: "owner", IdempotencyKey: "backup-elapsed",
		ResourceReferences: map[string]string{"database": "scaleway-rdb://source"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup start: %v", err)
	}
	snapshot := database.snapshots["snapshot-1"]
	snapshot.Status = "ready"
	snapshot.CreatedAt = time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	database.snapshots[snapshot.ID] = snapshot
	now = now.Add(10 * time.Minute)
	completed, err := client.Poll(context.Background(), backup.OperationID)
	if err != nil {
		t.Fatalf("elapsed backup poll: %v", err)
	}
	if completed.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("elapsed backup poll status = %s detail=%q", completed.Status, completed.Detail)
	}
}

func TestNativeAPIDatabaseBackupPollFailsClosedOnShortRetention(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	database := newFakeScalewayDatabase()
	database.instances["source"] = DatabaseInstance{
		ID: "source", Name: "source", Region: "fr-par", Status: "ready", VolumeType: "bssd", EncryptionEnabled: true,
		Tags: []string{scalewayDatabaseOwnershipTag + "=owner", scalewayDatabaseClassTag + "=database"},
	}
	native, err := NewNativeAPIWithDatabase(newFakeScalewayS3(), database, NativeAPIConfig{
		ArchiveBucket: "archive", DatabaseRegion: "fr-par", RetentionDays: 1,
		Now: func() time.Time { return now },
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
		FixtureID: "fixture", OwnershipMarker: "owner", IdempotencyKey: "backup-short-retention",
		ResourceReferences: map[string]string{"database": "scaleway-rdb://source"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup start: %v", err)
	}
	snapshot := database.snapshots["snapshot-1"]
	snapshot.Status = "ready"
	snapshot.CreatedAt = now
	snapshot.ExpiresAt = now.Add(time.Hour)
	database.snapshots[snapshot.ID] = snapshot
	polled, err := client.Poll(context.Background(), backup.OperationID)
	if err != nil {
		t.Fatalf("short retention poll error = %v", err)
	}
	if polled.Status != sdk.ResilienceOperationFailed {
		t.Fatalf("short retention poll = status %s detail=%q", polled.Status, polled.Detail)
	}
}

func TestNativeAPIDatabaseReconcilesAmbiguousSnapshotCreation(t *testing.T) {
	database := newFakeScalewayDatabase()
	database.instances["source"] = DatabaseInstance{
		ID: "source", Name: "source", Region: "fr-par", Status: "ready", VolumeType: "bssd", EncryptionEnabled: true,
		Tags: []string{scalewayDatabaseOwnershipTag + "=owner", scalewayDatabaseClassTag + "=database"},
	}
	database.createSnapshotErr = context.Canceled
	native, err := NewNativeAPIWithDatabase(newFakeScalewayS3(), database, NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}

	request := cloudresilience.NativeOperationRequest{
		Provider: "scaleway", Operation: "scaleway.managed-database.backup.database",
		Request: sdk.ResilienceOperationRequest{
			Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, FixtureID: "fixture",
			OwnershipMarker: "owner", IdempotencyKey: "backup-ambiguous",
			ResourceReferences: map[string]string{"database": "scaleway-rdb://source"},
		},
	}
	observation, err := native.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("ambiguous snapshot: %v", err)
	}
	if observation.Status != string(sdk.ResilienceOperationPending) {
		t.Fatalf("ambiguous snapshot status = %s, want pending", observation.Status)
	}
	if _, ok := database.snapshots["snapshot-1"]; !ok {
		t.Fatal("ambiguous snapshot was not recovered from inventory")
	}
}

func TestDatabaseOnlyInventoryDoesNotRequireObjectStorage(t *testing.T) {
	database := newFakeScalewayDatabase()
	database.instances["source"] = DatabaseInstance{
		ID: "source", Name: "source", Region: "fr-par", Status: "ready", EncryptionEnabled: true,
		Tags: []string{scalewayDatabaseOwnershipTag + "=owner", scalewayDatabaseClassTag + "=database"},
	}
	native := &NativeAPI{
		database: database,
		config:   normalizeScalewayConfig(NativeAPIConfig{DatabaseRegion: "fr-par"}),
	}

	inventory, err := native.Inventory(context.Background(), "owner")
	if err != nil {
		t.Fatalf("database-only inventory: %v", err)
	}
	if len(inventory) != 1 || inventory[0].Identity != "scaleway-rdb://source" {
		t.Fatalf("database-only inventory = %#v", inventory)
	}
}

type fakeScalewayDatabase struct {
	instances         map[string]DatabaseInstance
	snapshots         map[string]DatabaseSnapshot
	createSnapshots   int
	createRestores    int
	lastRestoreID     string
	createRestoreErr  error
	createSnapshotErr error
	updateTagsErr     error
}

func newFakeScalewayDatabase() *fakeScalewayDatabase {
	return &fakeScalewayDatabase{instances: make(map[string]DatabaseInstance), snapshots: make(map[string]DatabaseSnapshot)}
}

func (fake *fakeScalewayDatabase) GetInstance(_ context.Context, id string) (DatabaseInstance, error) {
	if instance, ok := fake.instances[id]; ok {
		return cloneScalewayDatabaseInstance(instance), nil
	}
	for _, instance := range fake.instances {
		if instance.Name == id {
			return cloneScalewayDatabaseInstance(instance), nil
		}
	}
	return DatabaseInstance{}, errors.New("not found")
}

func (fake *fakeScalewayDatabase) ListInstances(context.Context) ([]DatabaseInstance, error) {
	instances := make([]DatabaseInstance, 0, len(fake.instances))
	for _, instance := range fake.instances {
		instances = append(instances, cloneScalewayDatabaseInstance(instance))
	}
	sort.Slice(instances, func(i, j int) bool { return instances[i].ID < instances[j].ID })
	return instances, nil
}

func (fake *fakeScalewayDatabase) DeleteInstance(_ context.Context, id string) error {
	if _, ok := fake.instances[id]; !ok {
		return errors.New("not found")
	}
	delete(fake.instances, id)
	return nil
}

func (fake *fakeScalewayDatabase) CreateSnapshot(_ context.Context, instanceID, name string, expiresAt time.Time) (DatabaseSnapshot, error) {
	fake.createSnapshots++
	snapshot := DatabaseSnapshot{ID: "snapshot-1", InstanceID: instanceID, Name: name, Region: "fr-par", Status: "creating", NodeType: "db-gp1-small", VolumeType: "bssd", ExpiresAt: expiresAt}
	fake.snapshots[snapshot.ID] = snapshot
	if fake.createSnapshotErr != nil {
		return DatabaseSnapshot{}, fake.createSnapshotErr
	}
	return snapshot, nil
}

func (fake *fakeScalewayDatabase) GetSnapshot(_ context.Context, id string) (DatabaseSnapshot, error) {
	snapshot, ok := fake.snapshots[id]
	if !ok {
		return DatabaseSnapshot{}, errors.New("not found")
	}
	return snapshot, nil
}

func (fake *fakeScalewayDatabase) ListSnapshots(context.Context) ([]DatabaseSnapshot, error) {
	snapshots := make([]DatabaseSnapshot, 0, len(fake.snapshots))
	for _, snapshot := range fake.snapshots {
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].ID < snapshots[j].ID })
	return snapshots, nil
}

func (fake *fakeScalewayDatabase) DeleteSnapshot(_ context.Context, id string) error {
	if _, ok := fake.snapshots[id]; !ok {
		return errors.New("not found")
	}
	delete(fake.snapshots, id)
	return nil
}

func (fake *fakeScalewayDatabase) CreateInstanceFromSnapshot(_ context.Context, _ string, name, nodeType string, isHA bool) (DatabaseInstance, error) {
	fake.createRestores++
	fake.lastRestoreID = "restore-1"
	target := DatabaseInstance{ID: fake.lastRestoreID, Name: name, Region: "fr-par", Status: "provisioning", NodeType: nodeType, VolumeType: "bssd", IsHA: isHA, EncryptionEnabled: true}
	fake.instances[target.ID] = target
	return target, fake.createRestoreErr
}

func (fake *fakeScalewayDatabase) UpdateInstanceTags(_ context.Context, id string, tags []string) (DatabaseInstance, error) {
	if fake.updateTagsErr != nil {
		return DatabaseInstance{}, fake.updateTagsErr
	}
	instance := fake.instances[id]
	instance.ID = id
	instance.Tags = append([]string(nil), tags...)
	fake.instances[id] = instance
	return cloneScalewayDatabaseInstance(instance), nil
}

type fakeScalewayRecoveryVerifier struct{}

func (fakeScalewayRecoveryVerifier) Verify(context.Context, RecoveryVerificationRequest) (RecoveryVerification, error) {
	return RecoveryVerification{ManifestVerified: true, CountsVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true}, nil
}

func cloneScalewayDatabaseInstance(instance DatabaseInstance) DatabaseInstance {
	instance.Tags = append([]string(nil), instance.Tags...)
	return instance
}
