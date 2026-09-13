package certification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestDatabaseRecoveryCellPassesRestoreIdentityToIntegrityAndCleansSource(t *testing.T) {
	fixture, err := BuildRecoveryFixtureManifest("fixture-db", "scrubbed", []RecoveryFixtureMaterial{{
		Name: "database", ExpectedRecords: 2, ExpectedObjects: 0, Manifest: []byte("manifest"), Content: []byte("content"), Permissions: []byte("permissions"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	operations := &databaseRecoveryCellOperations{}
	store := &databaseRecoveryCellStore{}
	cell, err := NewDatabaseRecoveryCell(DatabaseRecoveryCellConfig{
		Operations: operations, FixtureStore: store, OperationPolicy: sdk.ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2},
		Fixture: fixture, ResourceReference: "db://source", Marker: "marker",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := cell.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, err := cell.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.RestoreID != "db://restore" || operations.integrityResource != "db://restore" || operations.restoreApproval != "" || !result.SourceInventoryVerified || !result.OutputInventoryVerified {
		t.Fatalf("result=%#v operations=%#v", result, operations)
	}
	if result.CorruptionInjected {
		t.Fatal("isolated restore unexpectedly injected source corruption")
	}
	if err := cell.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !store.deleted {
		t.Fatal("source fixture was not deleted")
	}
}

func TestDatabaseRecoveryCellRejectsIncompleteApplicationEvidence(t *testing.T) {
	fixture, err := BuildRecoveryFixtureManifest("fixture-db", "scrubbed", []RecoveryFixtureMaterial{{
		Name: "database", Manifest: []byte("manifest"), Content: []byte("content"), Permissions: []byte("permissions"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	operations := &databaseRecoveryCellOperations{incompleteIntegrity: true}
	cell, err := NewDatabaseRecoveryCell(DatabaseRecoveryCellConfig{
		Operations: operations, FixtureStore: &databaseRecoveryCellStore{}, OperationPolicy: sdk.ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2},
		Fixture: fixture, ResourceReference: "db://source", Marker: "marker",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cell.Run(context.Background()); err == nil {
		t.Fatal("incomplete application evidence passed")
	}
}

func TestDatabaseRecoveryCellRejectsIncompleteInventory(t *testing.T) {
	fixture, err := BuildRecoveryFixtureManifest("fixture-db", "scrubbed", []RecoveryFixtureMaterial{{
		Name: "database", ExpectedRecords: 2, Manifest: []byte("manifest"), Content: []byte("content"), Permissions: []byte("permissions"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	operations := &databaseRecoveryCellOperations{emptyInventory: true}
	cell, err := NewDatabaseRecoveryCell(DatabaseRecoveryCellConfig{
		Operations: operations, FixtureStore: &databaseRecoveryCellStore{}, OperationPolicy: sdk.ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2},
		Fixture: fixture, ResourceReference: "db://source", Marker: "marker",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cell.Run(context.Background()); err == nil {
		t.Fatal("incomplete source/restore inventory passed")
	}
}

func TestDatabaseRecoveryCellAllowsExplicitlyOptionalProtection(t *testing.T) {
	fixture, err := BuildRecoveryFixtureManifest("fixture-db", "scrubbed", []RecoveryFixtureMaterial{{
		Name: "database", ExpectedRecords: 2, Manifest: []byte("manifest"), Content: []byte("content"), Permissions: []byte("permissions"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	operations := &databaseRecoveryCellOperations{unprotected: true}
	cell, err := NewDatabaseRecoveryCell(DatabaseRecoveryCellConfig{
		Operations: operations, FixtureStore: &databaseRecoveryCellStore{}, OperationPolicy: sdk.ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2},
		Fixture: fixture, ResourceReference: "db://source", Marker: "marker", Protection: DatabaseRecoveryProtectionOptional,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := cell.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	result, err := cell.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.IntegrityEvidence.ProtectionVerified {
		t.Fatal("optional protection unexpectedly claimed as verified")
	}
}

func TestDatabaseRecoveryCellPassesApprovalAndSourceIdentityForInPlaceRestore(t *testing.T) {
	fixture, err := BuildRecoveryFixtureManifest("fixture-db", "scrubbed", []RecoveryFixtureMaterial{{
		Name: "database", ExpectedRecords: 2, ExpectedObjects: 0, Manifest: []byte("manifest"), Content: []byte("content"), Permissions: []byte("permissions"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	operations := &databaseRecoveryCellOperations{inPlace: true}
	store := &databaseRecoveryCellOverwriteStore{}
	operations.store = store
	cell, err := NewDatabaseRecoveryCell(DatabaseRecoveryCellConfig{
		Operations: operations, FixtureStore: store, OperationPolicy: sdk.ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2},
		Fixture: fixture, ResourceReference: "db://source", Marker: "marker", Destination: sdk.RecoverySameRegion, ApprovalReference: "fence-approved",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := cell.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.RestoreID != "db://source" || operations.integrityResource != "db://source" || operations.restoreApproval != "fence-approved" {
		t.Fatalf("result=%#v operations=%#v", result, operations)
	}
	if !result.CorruptionInjected || !store.overwritten || store.payload != DatabaseRecoveryCorruptPayload || store.overwriteAfterBackup != true {
		t.Fatalf("in-place corruption was not injected after backup: result=%#v store=%#v operations=%#v", result, store, operations)
	}
}

func TestDatabaseRecoveryCellRejectsInPlaceWithoutOverwriter(t *testing.T) {
	fixture, err := BuildRecoveryFixtureManifest("fixture-db", "scrubbed", []RecoveryFixtureMaterial{{
		Name: "database", ExpectedRecords: 2, Manifest: []byte("manifest"), Content: []byte("content"), Permissions: []byte("permissions"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewDatabaseRecoveryCell(DatabaseRecoveryCellConfig{
		Operations: &databaseRecoveryCellOperations{inPlace: true}, FixtureStore: &databaseRecoveryCellStore{}, OperationPolicy: sdk.ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2},
		Fixture: fixture, ResourceReference: "db://source", Marker: "marker", Destination: sdk.RecoverySameRegion, ApprovalReference: "fence-approved",
	})
	if err == nil || !strings.Contains(err.Error(), "overwrite") {
		t.Fatalf("error = %v", err)
	}
}

type databaseRecoveryCellStore struct{ deleted bool }

func (s *databaseRecoveryCellStore) Prepare(context.Context, string, RecoveryFixtureManifest) error {
	return nil
}
func (s *databaseRecoveryCellStore) Delete(context.Context, string) error {
	s.deleted = true
	return nil
}

type databaseRecoveryCellOverwriteStore struct {
	databaseRecoveryCellStore
	overwritten          bool
	payload              string
	overwriteAfterBackup bool
	sawBackupBeforeWrite bool
}

func (s *databaseRecoveryCellOverwriteStore) Overwrite(_ context.Context, _ string, payload string) error {
	s.overwritten = true
	s.payload = payload
	s.overwriteAfterBackup = s.sawBackupBeforeWrite
	return nil
}

type databaseRecoveryCellOperations struct {
	integrityResource   string
	restoreApproval     string
	incompleteIntegrity bool
	emptyInventory      bool
	unprotected         bool
	inPlace             bool
	store               *databaseRecoveryCellOverwriteStore
}

func (o *databaseRecoveryCellOperations) Start(_ context.Context, request sdk.ResilienceOperationRequest) (sdk.ResilienceOperationObservation, error) {
	switch request.Action {
	case sdk.ResilienceBackup:
		if o.store != nil {
			o.store.sawBackupBeforeWrite = true
		}
		return databaseRecoveryObservation(request, sdk.ResilienceOperationSucceeded, sdk.ResilienceProofEvidence{DataClass: "database", Destination: string(request.Destination), FixtureID: request.FixtureID, BackupID: "db://backup", EncryptionVerified: true, ProtectionVerified: !o.unprotected}), nil
	case sdk.ResilienceRestore:
		o.restoreApproval = request.ApprovalReference
		restoreID := "db://restore"
		if o.inPlace {
			restoreID = request.ResourceReferences["database"]
		}
		return databaseRecoveryObservation(request, sdk.ResilienceOperationSucceeded, sdk.ResilienceProofEvidence{DataClass: "database", Destination: string(request.Destination), FixtureID: request.FixtureID, BackupID: "db://backup", RestoreID: restoreID, ProtectionVerified: !o.unprotected, ServiceHealthVerified: true}), nil
	case sdk.ResilienceIntegrityCheck:
		o.integrityResource = request.ResourceReferences["database"]
		evidence := sdk.ResilienceProofEvidence{DataClass: "database", Destination: string(request.Destination), FixtureID: request.FixtureID, BackupID: "db://backup", EncryptionVerified: !o.incompleteIntegrity, ProtectionVerified: !o.unprotected, ManifestVerified: !o.incompleteIntegrity, CountsVerified: !o.incompleteIntegrity, ApplicationReadsVerified: !o.incompleteIntegrity, PermissionsVerified: !o.incompleteIntegrity, ServiceHealthVerified: !o.incompleteIntegrity}
		return databaseRecoveryObservation(request, sdk.ResilienceOperationSucceeded, evidence), nil
	case sdk.ResilienceCleanup:
		return databaseRecoveryObservation(request, sdk.ResilienceOperationSucceeded), nil
	default:
		return sdk.ResilienceOperationObservation{}, errors.New("unexpected database recovery action")
	}
}

func (o *databaseRecoveryCellOperations) Poll(context.Context, string) (sdk.ResilienceOperationObservation, error) {
	return sdk.ResilienceOperationObservation{}, errors.New("no pending operation expected")
}

func (o *databaseRecoveryCellOperations) Inventory(_ context.Context, _ string) ([]sdk.ResilienceInventoryResource, error) {
	if o.emptyInventory {
		return []sdk.ResilienceInventoryResource{{Identity: "db://source", Owned: true, Live: true}}, nil
	}
	if o.inPlace {
		return []sdk.ResilienceInventoryResource{{Identity: "db://source", Owned: true, Live: true}}, nil
	}
	return []sdk.ResilienceInventoryResource{{Identity: "db://source", Owned: true, Live: true}, {Identity: "db://restore", Owned: true, Live: true}}, nil
}

func databaseRecoveryObservation(request sdk.ResilienceOperationRequest, status sdk.ResilienceOperationStatus, evidence ...sdk.ResilienceProofEvidence) sdk.ResilienceOperationObservation {
	return sdk.ResilienceOperationObservation{Status: status, Action: request.Action, OperationID: "db-op-" + string(request.Action), OwnershipMarker: request.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true, Evidence: evidence}
}
