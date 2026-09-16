package resilience

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	cloudresilience "github.com/magelift/magelift/internal/shared/resilience"
	"github.com/magelift/magelift/sdk"
)

func TestNativeAPIOVHDatabaseRecoveryUsesProviderBackupsAndIsResumable(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	database := newFakeOVHDatabase()
	database.instances["source"] = DatabaseInstance{ID: "source", Description: "magento", Engine: "postgresql", Version: "17", Region: "GRA", Status: "READY", Plan: "production", Flavor: "b3-8", NodeCount: 2, Encrypted: true, BackupRetentionDays: 14}
	database.backups["source\x00backup"] = DatabaseBackup{ID: "backup", InstanceID: "source", CreatedAt: now.Add(-time.Hour), Status: "READY", Encrypted: true, Regions: []string{"GRA", "DE"}, RetentionDays: 14}
	native, err := NewNativeAPIWithDatabase(newFakeOVHS3(), database, NativeAPIConfig{
		ArchiveBucket: "archive", DatabaseProjectID: "project", DatabaseEngine: "postgresql", DatabaseSourceID: "source",
		RestoreDatabaseRegion: "GRA", RequireDatabaseHA: true, MaxDatabaseBackupAge: 24 * time.Hour,
		DatabaseIPRestrictions: []string{"198.51.100.7/32"},
		Verifier:               fakeOVHDatabaseVerifier{}, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/known-content", OwnershipMarker: "magelift/test/ovh/database", IdempotencyKey: "database/backup/1",
		ResourceReferences: map[string]string{"database": "ovh-database://postgresql/source"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || len(backup.Evidence) != 1 || backup.Evidence[0].ProtectionVerified || !backup.Evidence[0].EncryptionVerified {
		t.Fatalf("backup observation = %#v", backup)
	}
	if database.listBackupsCalls != 1 {
		t.Fatalf("backup list calls = %d, want 1", database.listBackupsCalls)
	}

	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "database/restore/1"
	restore.BackupReferences = map[string]string{"database": backup.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Status != sdk.ResilienceOperationPending {
		t.Fatalf("restore status = %q, want pending while fork is creating", restored.Status)
	}
	if database.created.Description == "" || database.created.NodeCount != 2 || database.created.Region != "GRA" {
		t.Fatalf("restore request = %#v", database.created)
	}
	if len(database.createdRequest.IPRestrictions) != 1 || database.createdRequest.IPRestrictions[0] != "198.51.100.7/32" {
		t.Fatalf("restore IP restrictions = %#v", database.createdRequest.IPRestrictions)
	}
	database.instances[database.created.ID] = database.created
	database.instances[database.created.ID] = DatabaseInstance{ID: database.created.ID, Description: database.created.Description, Engine: "postgresql", Region: "GRA", Status: "READY", NodeCount: 2, Encrypted: true, BackupRetentionDays: 14}
	completed, err := client.Poll(context.Background(), restored.OperationID)
	if err != nil {
		t.Fatalf("poll restore: %v", err)
	}
	if completed.Status != sdk.ResilienceOperationSucceeded || !completed.Evidence[0].ServiceHealthVerified {
		t.Fatalf("completed restore = %#v", completed)
	}
	resumed, err := client.Poll(context.Background(), restored.OperationID)
	if err != nil {
		t.Fatalf("resume restore: %v", err)
	}
	if resumed.OperationID != restored.OperationID || resumed.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("resumed restore = %#v", resumed)
	}

	integrity := restore
	integrity.Action = sdk.ResilienceIntegrityCheck
	integrity.IdempotencyKey = "database/integrity/1"
	integrity.ResourceReferences = map[string]string{"database": "ovh-database://postgresql/" + database.created.ID}
	checked, err := client.Start(context.Background(), integrity)
	if err != nil {
		t.Fatalf("integrity: %v", err)
	}
	if checked.Status != sdk.ResilienceOperationSucceeded || !checked.Evidence[0].ApplicationReadsVerified || !checked.Evidence[0].PermissionsVerified {
		t.Fatalf("integrity observation = %#v", checked)
	}

	inventory, err := client.Inventory(context.Background(), request.OwnershipMarker)
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	if len(inventory) != 2 {
		t.Fatalf("inventory = %#v, want source and restore target", inventory)
	}
}

func TestNativeAPIOVHDatabaseBackupWaitsForLateReadyProviderBackup(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	database := newFakeOVHDatabase()
	database.instances["source"] = DatabaseInstance{ID: "source", Description: "owned", Engine: "mysql", Version: "8.4", Region: "GRA", Status: "READY", Plan: "essential", Flavor: "db1-4", NodeCount: 1, Encrypted: true, BackupRetentionDays: 2}
	native, err := NewNativeAPIWithDatabase(newFakeOVHS3(), database, NativeAPIConfig{
		ArchiveBucket: "archive", DatabaseProjectID: "project", DatabaseEngine: "mysql", DatabaseSourceID: "source", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/late-backup", OwnershipMarker: "magelift/test/ovh/late-backup", IdempotencyKey: "database/backup/late",
		ResourceReferences: map[string]string{"database": "ovh-database://mysql/source"},
	}
	pending, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != sdk.ResilienceOperationPending {
		t.Fatalf("backup status = %q, want pending", pending.Status)
	}
	database.backups["source\x00backup"] = DatabaseBackup{ID: "backup", InstanceID: "source", CreatedAt: now.Add(-time.Minute), Status: "READY", Encrypted: true, Regions: []string{"GRA"}, RetentionDays: 2}
	completed, err := client.Poll(context.Background(), pending.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != sdk.ResilienceOperationSucceeded || completed.OperationID != pending.OperationID || len(completed.Evidence) != 1 {
		t.Fatalf("completed backup = %#v", completed)
	}
}

func TestNativeAPIOVHDatabaseBackupHonorsFixtureBoundary(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	database := newFakeOVHDatabase()
	database.instances["source"] = DatabaseInstance{ID: "source", Description: "owned", Engine: "mysql", Version: "8.4", Region: "GRA", Status: "READY", Plan: "essential", Flavor: "db1-4", NodeCount: 1, Encrypted: true, BackupRetentionDays: 2}
	database.backups["source\x00old"] = DatabaseBackup{ID: "old", InstanceID: "source", CreatedAt: now.Add(-time.Minute), Status: "READY", Encrypted: true, Regions: []string{"GRA"}, RetentionDays: 2}
	native, err := NewNativeAPIWithDatabase(newFakeOVHS3(), database, NativeAPIConfig{
		ArchiveBucket: "archive", DatabaseProjectID: "project", DatabaseEngine: "mysql", DatabaseSourceID: "source",
		DatabaseBackupNotBefore: now, Now: func() time.Time { return now.Add(2 * time.Minute) },
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/boundary", OwnershipMarker: "magelift/test/ovh/boundary", IdempotencyKey: "database/backup/boundary",
		ResourceReferences: map[string]string{"database": "ovh-database://mysql/source"},
	}
	result, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != sdk.ResilienceOperationPending {
		t.Fatalf("backup = %#v", result)
	}
	database.backups["source\x00new"] = DatabaseBackup{ID: "new", InstanceID: "source", CreatedAt: now.Add(time.Minute), Status: "CREATING", Encrypted: true, Regions: []string{"GRA"}, RetentionDays: 2}
	pending, err := client.Poll(context.Background(), result.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != sdk.ResilienceOperationPending {
		t.Fatalf("mixed backup state = %#v", pending)
	}
	database.backups["source\x00new"] = DatabaseBackup{ID: "new", InstanceID: "source", CreatedAt: now.Add(time.Minute), Status: "READY", Encrypted: true, Regions: []string{"GRA"}, RetentionDays: 2}
	completed, err := client.Poll(context.Background(), pending.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != sdk.ResilienceOperationSucceeded || !strings.HasSuffix(completed.Evidence[0].BackupID, "/new") {
		t.Fatalf("completed backup = %#v", completed)
	}
}

func TestNativeAPIOVHDatabasePITRRecoveryUsesProviderPointInTime(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	database := newFakeOVHDatabase()
	database.instances["source"] = DatabaseInstance{
		ID: "source", Description: "owned", Engine: "mysql", Version: "8.4", Region: "GRA", Status: "READY",
		Plan: "essential", Flavor: "db1-4", NodeCount: 1, Encrypted: true, BackupRetentionDays: 2,
		PITRAvailableFrom: now.Add(-time.Hour),
	}
	native, err := NewNativeAPIWithDatabase(newFakeOVHS3(), database, NativeAPIConfig{
		ArchiveBucket: "archive", DatabaseProjectID: "project", DatabaseEngine: "mysql", DatabaseSourceID: "source",
		DatabaseUsePITR: true, DatabaseBackupNotBefore: now.Add(-time.Minute), Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/pitr", OwnershipMarker: "magelift/test/ovh/pitr", IdempotencyKey: "database/pitr",
		ResourceReferences: map[string]string{"database": "ovh-database://mysql/source"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || !strings.Contains(backup.Evidence[0].BackupID, "ovh-database-pitr://source/") {
		t.Fatalf("backup = %#v", backup)
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "database/pitr-restore"
	restore.BackupReferences = map[string]string{"database": backup.Evidence[0].BackupID}
	started, err := client.Start(context.Background(), restore)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if started.Status != sdk.ResilienceOperationPending || database.createdRequest.BackupID != "" || database.createdRequest.PointInTime == nil || !database.createdRequest.PointInTime.Equal(now.Add(-30*time.Second)) {
		t.Fatalf("restore request = %#v observation=%#v", database.createdRequest, started)
	}
	database.instances["restore"] = DatabaseInstance{ID: "restore", Description: database.created.Description, Engine: "mysql", Version: "8.4", Region: "GRA", Status: "READY", Plan: "essential", Flavor: "db1-4", NodeCount: 1, Encrypted: true, BackupRetentionDays: 2}
	completed, err := client.Poll(context.Background(), started.OperationID)
	if err != nil {
		t.Fatalf("poll restore: %v", err)
	}
	if completed.Status != sdk.ResilienceOperationSucceeded || !completed.Evidence[0].ServiceHealthVerified {
		t.Fatalf("completed restore = %#v", completed)
	}
}

func TestNativeAPIOVHDatabaseRefusesUnverifiedBackupAndUnsupportedCacheBackup(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	database := newFakeOVHDatabase()
	database.instances["source"] = DatabaseInstance{ID: "source", Engine: "redis", Region: "GRA", Status: "READY", NodeCount: 1, Encrypted: true, BackupRetentionDays: 1}
	database.backups["source\x00backup"] = DatabaseBackup{ID: "backup", InstanceID: "source", CreatedAt: now.Add(-time.Hour), Status: "READY", Encrypted: false, RetentionDays: 1}
	native, err := NewNativeAPIWithDatabase(newFakeOVHS3(), database, NativeAPIConfig{ArchiveBucket: "archive", DatabaseProjectID: "project", DatabaseEngine: "redis", DatabaseSourceID: "source", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	request := cloudresilience.NativeOperationRequest{Provider: "ovh", Operation: "ovh.managed-database.backup.database", Request: sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, FixtureID: "fixture", OwnershipMarker: "magelift/test/ovh", IdempotencyKey: "backup/1",
		ResourceReferences: map[string]string{"database": "ovh-database://redis/source"},
	}}
	if _, err := native.Start(context.Background(), request); err == nil || !strings.Contains(err.Error(), "encrypted database backup") {
		t.Fatalf("unverified backup error = %v", err)
	}
	cacheRequest := request
	cacheRequest.Operation = "ovh.valkey.backup.cache"
	cacheRequest.Request.DataClasses = []string{"cache"}
	cacheRequest.Request.ResourceReferences = map[string]string{"cache": "ovh-database://redis/source"}
	getCalls := database.getCalls
	listBackupsCalls := database.listBackupsCalls
	createCalls := database.createCalls
	deleteCalls := database.deleteCalls
	var capability sdk.ResilienceCapabilityError
	if _, err := native.Start(context.Background(), cacheRequest); !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported || !strings.Contains(capability.Reason, "source of truth") {
		t.Fatalf("cache backup error = %v, want typed unsupported capability", err)
	}
	if database.getCalls != getCalls || database.listBackupsCalls != listBackupsCalls || database.createCalls != createCalls || database.deleteCalls != deleteCalls {
		t.Fatalf("cache backup touched managed database: %#v", database)
	}
}

func TestNativeAPIOVHDatabaseRestoreCleanupPreservesSourceAndForeignServices(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	database := newFakeOVHDatabase()
	database.instances["source"] = DatabaseInstance{ID: "source", Description: "magento", Engine: "postgresql", Version: "17", Region: "GRA", Status: "READY", Plan: "production", Flavor: "b3-8", NodeCount: 2, Encrypted: true, BackupRetentionDays: 14}
	database.backups["source\x00backup"] = DatabaseBackup{ID: "backup", InstanceID: "source", CreatedAt: now.Add(-time.Hour), Status: "READY", Encrypted: true, Regions: []string{"GRA"}, RetentionDays: 14}
	native, err := NewNativeAPIWithDatabase(newFakeOVHS3(), database, NativeAPIConfig{
		ArchiveBucket: "archive", DatabaseProjectID: "project", DatabaseEngine: "postgresql", DatabaseSourceID: "source", DatabaseDeletePollInterval: time.Millisecond,
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
		Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture/cleanup", OwnershipMarker: "magelift/test/ovh/database-cleanup", IdempotencyKey: "database/backup/1",
		ResourceReferences: map[string]string{"database": "ovh-database://postgresql/source"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "database/restore/1"
	restore.BackupReferences = map[string]string{"database": backup.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	database.instances[database.created.ID] = DatabaseInstance{ID: database.created.ID, Description: database.created.Description, Engine: "postgresql", Region: "GRA", Status: "READY", NodeCount: 2, Encrypted: true, BackupRetentionDays: 14}
	if _, err := client.Poll(context.Background(), restored.OperationID); err != nil {
		t.Fatalf("poll restore: %v", err)
	}
	foreign := DatabaseInstance{ID: "foreign", Description: ovhDatabaseRestoreDescriptionPrefix("other") + strings.Repeat("a", 24), Engine: "postgresql", Region: "GRA", Status: "READY", NodeCount: 2, Encrypted: true}
	database.instances[foreign.ID] = foreign

	cleanup := request
	cleanup.Action = sdk.ResilienceCleanup
	cleanup.IdempotencyKey = "database/cleanup/1"
	cleaned, err := client.Start(context.Background(), cleanup)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if cleaned.Status != sdk.ResilienceOperationSucceeded || len(cleaned.ResourceRefs) != 1 {
		t.Fatalf("cleanup observation = %#v", cleaned)
	}
	if _, ok := database.instances["restore"]; ok {
		t.Fatal("owned restore service remains")
	}
	if _, ok := database.instances["source"]; !ok {
		t.Fatal("cleanup deleted the source service")
	}
	if _, ok := database.instances["foreign"]; !ok {
		t.Fatal("cleanup deleted a foreign service")
	}
	deleteCalls := database.deleteCalls
	repeated, err := client.Start(context.Background(), cleanup)
	if err != nil {
		t.Fatalf("repeated cleanup: %v", err)
	}
	if repeated.Status != sdk.ResilienceOperationSucceeded || database.deleteCalls != deleteCalls {
		t.Fatalf("repeated cleanup = %#v delete calls=%d want=%d", repeated, database.deleteCalls, deleteCalls)
	}
}

func TestNativeAPIOVHDatabaseCleanupPollsDelayedDeletion(t *testing.T) {
	marker := "magelift/test/ovh/database-delayed"
	database := newFakeOVHDatabase()
	database.instances["source"] = DatabaseInstance{ID: "source", Description: "source", Engine: "redis", Region: "GRA", Status: "READY", NodeCount: 1, Encrypted: true}
	database.instances["restore"] = DatabaseInstance{ID: "restore", Description: ovhDatabaseRestoreDescriptionPrefix(marker) + strings.Repeat("b", 24), Engine: "redis", Region: "GRA", Status: "READY", NodeCount: 1, Encrypted: true}
	database.deleteDelay = 2
	native, err := NewNativeAPIWithDatabase(newFakeOVHS3(), database, NativeAPIConfig{ArchiveBucket: "archive", DatabaseEngine: "valkey", DatabaseSourceID: "source", DatabaseDeletePollInterval: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceCleanup, DataClasses: []string{"cache"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture", OwnershipMarker: marker, IdempotencyKey: "cache/cleanup/1",
		ResourceReferences: map[string]string{"cache": "ovh-database://valkey/source"},
	}
	result, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if result.Status != sdk.ResilienceOperationSucceeded || database.deleteCalls != 1 || database.getCalls < 3 {
		t.Fatalf("cleanup = %#v delete calls=%d get calls=%d", result, database.deleteCalls, database.getCalls)
	}
	if _, ok := database.instances["restore"]; ok {
		t.Fatal("delayed owned restore service remains")
	}
}

func TestNativeAPIOVHDatabaseRestoreRefusesForeignIdentityCollision(t *testing.T) {
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	marker := "magelift/test/ovh/database-collision"
	database := newFakeOVHDatabase()
	database.instances["source"] = DatabaseInstance{ID: "source", Description: "source", Engine: "postgresql", Region: "GRA", Status: "READY", NodeCount: 2, Encrypted: true, BackupRetentionDays: 14}
	database.backups["source\x00backup"] = DatabaseBackup{ID: "backup", InstanceID: "source", CreatedAt: now.Add(-time.Hour), Status: "READY", Encrypted: true, Regions: []string{"GRA"}, RetentionDays: 14}
	database.instances["foreign"] = DatabaseInstance{ID: "foreign", Description: ovhDatabaseRestoreDescriptionPrefix(marker) + shortDigest("fixture\x00database/restore/1"), Engine: "postgresql", Region: "GRA", Status: "READY", NodeCount: 2, Encrypted: true}
	native, err := NewNativeAPIWithDatabase(newFakeOVHS3(), database, NativeAPIConfig{ArchiveBucket: "archive", DatabaseEngine: "postgresql", DatabaseSourceID: "source", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceRestore, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture", OwnershipMarker: marker, IdempotencyKey: "database/restore/1",
		ResourceReferences: map[string]string{"database": "ovh-database://postgresql/source"},
		BackupReferences:   map[string]string{"database": "ovh-database-backup://source/backup"},
	}
	if _, err := client.Start(context.Background(), request); err == nil || !strings.Contains(err.Error(), "ownership cannot be proven") {
		t.Fatalf("restore collision error = %v", err)
	}
	if database.createCalls != 0 {
		t.Fatalf("collision triggered restore create: %d", database.createCalls)
	}
	if _, ok := database.instances["foreign"]; !ok {
		t.Fatal("collision handling deleted the foreign service")
	}
}

func TestNativeAPIOVHDatabaseCleanupRefusesDuplicateRestoreIdentity(t *testing.T) {
	marker := "magelift/test/ovh/database-cleanup-collision"
	description := ovhDatabaseRestoreDescriptionPrefix(marker) + strings.Repeat("c", 24)
	database := newFakeOVHDatabase()
	database.instances["source"] = DatabaseInstance{ID: "source", Description: "source", Engine: "postgresql", Region: "GRA", Status: "READY", NodeCount: 2, Encrypted: true}
	database.instances["owned"] = DatabaseInstance{ID: "owned", Description: description, Engine: "postgresql", Region: "GRA", Status: "READY", NodeCount: 2, Encrypted: true}
	database.instances["foreign"] = DatabaseInstance{ID: "foreign", Description: description, Engine: "postgresql", Region: "GRA", Status: "READY", NodeCount: 2, Encrypted: true}
	native, err := NewNativeAPIWithDatabase(newFakeOVHS3(), database, NativeAPIConfig{ArchiveBucket: "archive", DatabaseEngine: "postgresql", DatabaseSourceID: "source"})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceCleanup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion,
		FixtureID: "fixture", OwnershipMarker: marker, IdempotencyKey: "database/cleanup/collision",
		ResourceReferences: map[string]string{"database": "ovh-database://postgresql/source"},
	}
	if _, err := client.Start(context.Background(), request); err == nil || !strings.Contains(err.Error(), "collide") {
		t.Fatalf("cleanup collision error = %v", err)
	}
	if database.deleteCalls != 0 {
		t.Fatalf("cleanup deleted during identity collision: %d", database.deleteCalls)
	}
	if _, ok := database.instances["owned"]; !ok {
		t.Fatal("cleanup deleted the owned service during identity collision")
	}
	if _, ok := database.instances["foreign"]; !ok {
		t.Fatal("cleanup deleted the foreign service during identity collision")
	}
}

type fakeOVHDatabase struct {
	instances        map[string]DatabaseInstance
	backups          map[string]DatabaseBackup
	created          DatabaseInstance
	createdRequest   DatabaseRestoreRequest
	listBackupsCalls int
	createCalls      int
	deleteCalls      int
	getCalls         int
	deleteDelay      int
	pendingDeletes   map[string]int
}

func newFakeOVHDatabase() *fakeOVHDatabase {
	return &fakeOVHDatabase{instances: make(map[string]DatabaseInstance), backups: make(map[string]DatabaseBackup), pendingDeletes: make(map[string]int)}
}

func (fake *fakeOVHDatabase) GetInstance(_ context.Context, _, id string) (DatabaseInstance, error) {
	instance, ok := fake.instances[id]
	if !ok {
		return DatabaseInstance{}, errors.New("not found")
	}
	fake.getCalls++
	if remaining, pending := fake.pendingDeletes[id]; pending {
		if remaining > 0 {
			fake.pendingDeletes[id] = remaining - 1
			return instance, nil
		}
		delete(fake.pendingDeletes, id)
		delete(fake.instances, id)
		return DatabaseInstance{}, errors.New("not found")
	}
	return instance, nil
}

func (fake *fakeOVHDatabase) ListInstances(_ context.Context, _ string) ([]DatabaseInstance, error) {
	instances := make([]DatabaseInstance, 0, len(fake.instances))
	for _, instance := range fake.instances {
		instances = append(instances, instance)
	}
	return instances, nil
}

func (fake *fakeOVHDatabase) ListBackups(_ context.Context, _, instanceID string) ([]DatabaseBackup, error) {
	fake.listBackupsCalls++
	backups := make([]DatabaseBackup, 0)
	for _, backup := range fake.backups {
		if backup.InstanceID == instanceID {
			backups = append(backups, backup)
		}
	}
	return backups, nil
}

func (fake *fakeOVHDatabase) GetBackup(_ context.Context, _, instanceID, backupID string) (DatabaseBackup, error) {
	backup, ok := fake.backups[instanceID+"\x00"+backupID]
	if !ok {
		return DatabaseBackup{}, errors.New("not found")
	}
	return backup, nil
}

func (fake *fakeOVHDatabase) CreateInstanceFromBackup(_ context.Context, request DatabaseRestoreRequest) (DatabaseInstance, error) {
	fake.createCalls++
	fake.createdRequest = request
	fake.created = DatabaseInstance{ID: "restore", Description: request.Description, Engine: request.Engine, Region: request.Region, Status: "CREATING", Plan: request.Plan, Flavor: request.Flavor, Version: request.Version, NodeCount: request.NodeCount, Encrypted: true, BackupRetentionDays: 14}
	return fake.created, nil
}

func (fake *fakeOVHDatabase) DeleteInstance(_ context.Context, _, id string) error {
	if _, ok := fake.instances[id]; !ok {
		return errors.New("not found")
	}
	fake.deleteCalls++
	if fake.deleteDelay > 0 {
		fake.pendingDeletes[id] = fake.deleteDelay
		return nil
	}
	delete(fake.instances, id)
	return nil
}

type fakeOVHDatabaseVerifier struct{}

func (fakeOVHDatabaseVerifier) Verify(context.Context, RecoveryVerificationRequest) (RecoveryVerification, error) {
	return RecoveryVerification{ManifestVerified: true, CountsVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true}, nil
}
