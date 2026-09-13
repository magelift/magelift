package certification

import (
	"context"
	"errors"
	"strings"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestSecretRecoveryCellRunsPortableSequenceAndCleansExactFixture(t *testing.T) {
	store := newSecretRecoveryCellTestStore()
	operations := &secretRecoveryCellTestOperations{}
	cell, err := NewSecretRecoveryCell(SecretRecoveryCellConfig{
		Operations:        operations,
		Store:             store,
		ResourceReference: "test-secret://source",
		Marker:            "magelift/test/secret-cell",
		FixtureID:         "fixture/known-secret",
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
	if result.BackupID == "" || result.RestoreID == "" || result.ValueFingerprint == "" {
		t.Fatalf("result = %#v", result)
	}
	if len(operations.started) != 3 || operations.started[0].Action != sdk.ResilienceBackup || operations.started[1].Action != sdk.ResilienceRestore || operations.started[2].Action != sdk.ResilienceIntegrityCheck {
		t.Fatalf("started = %#v", operations.started)
	}
	if operations.started[1].Destination != sdk.RecoverySameRegionIsolated || operations.started[1].ApprovalReference != "" {
		t.Fatalf("isolated restore request = %#v", operations.started[1])
	}
	for _, request := range operations.started {
		if len(request.DataClasses) != 1 || request.DataClasses[0] != "configuration-secrets" || request.ResourceReferences["configuration-secrets"] != "test-secret://source" {
			t.Fatalf("request = %#v", request)
		}
	}
	if err := cell.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(operations.started) != 4 || operations.started[3].Action != sdk.ResilienceCleanup || len(store.values) != 0 {
		t.Fatalf("cleanup started=%#v values=%#v", operations.started, store.values)
	}
}

func TestSecretRecoveryCellReturnsOnlyValueFingerprint(t *testing.T) {
	store := newSecretRecoveryCellTestStore()
	operations := &secretRecoveryCellTestOperations{}
	cell, err := NewSecretRecoveryCell(SecretRecoveryCellConfig{
		Operations:        operations,
		Store:             store,
		ResourceReference: "test-secret://source",
		Marker:            "marker",
		FixtureID:         "fixture",
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
	if len(result.ValueFingerprint) != 64 || result.ValueFingerprint == string(cell.value) || strings.Contains(result.ValueFingerprint, cell.config.FixtureID) {
		t.Fatalf("result exposes more than a digest: %#v", result)
	}
}

func TestSecretRecoveryCellPassesApprovalAndSourceIdentityForInPlaceRestore(t *testing.T) {
	store := newSecretRecoveryCellTestStore()
	operations := &secretRecoveryCellTestOperations{store: store}
	cell, err := NewSecretRecoveryCell(SecretRecoveryCellConfig{
		Operations:        operations,
		Store:             store,
		ResourceReference: "test-secret://source",
		Marker:            "magelift/test/secret-cell-inplace",
		FixtureID:         "fixture/known-secret",
		Destination:       sdk.RecoverySameRegion,
		ApprovalReference: "fence-approved",
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
	if result.RestoreID != "test-secret://source" || operations.started[1].ApprovalReference != "fence-approved" || operations.started[1].Destination != sdk.RecoverySameRegion {
		t.Fatalf("result=%#v started=%#v", result, operations.started)
	}
}

func TestSecretRecoveryCellRejectsInPlaceWithoutOverwriter(t *testing.T) {
	operations := &secretRecoveryCellTestOperations{}
	if _, err := NewSecretRecoveryCell(SecretRecoveryCellConfig{
		Operations:        operations,
		Store:             isolatedOnlySecretStore{},
		ResourceReference: "test-secret://source",
		Marker:            "marker",
		FixtureID:         "fixture",
		Destination:       sdk.RecoverySameRegion,
		ApprovalReference: "fence-approved",
	}); err == nil || !strings.Contains(err.Error(), "overwrite") {
		t.Fatalf("error = %v", err)
	}
}

func TestSecretRecoveryCellRejectsMissingProviderBoundary(t *testing.T) {
	if _, err := NewSecretRecoveryCell(SecretRecoveryCellConfig{}); err == nil || !strings.Contains(err.Error(), "operation client") {
		t.Fatalf("error = %v", err)
	}
}

type secretRecoveryCellTestStore struct {
	values map[string][]byte
}

func newSecretRecoveryCellTestStore() *secretRecoveryCellTestStore {
	return &secretRecoveryCellTestStore{values: make(map[string][]byte)}
}

func (store *secretRecoveryCellTestStore) Put(_ context.Context, reference string, value []byte) error {
	store.values[reference] = append([]byte(nil), value...)
	store.values["test-secret://restore"] = append([]byte(nil), value...)
	return nil
}

func (store *secretRecoveryCellTestStore) Overwrite(_ context.Context, reference string, value []byte) error {
	store.values[reference] = append([]byte(nil), value...)
	return nil
}

func (store *secretRecoveryCellTestStore) Read(_ context.Context, reference string) ([]byte, error) {
	value, ok := store.values[reference]
	if !ok {
		return nil, errors.New("secret not found")
	}
	return append([]byte(nil), value...), nil
}

func (store *secretRecoveryCellTestStore) Delete(_ context.Context, reference string) error {
	delete(store.values, reference)
	delete(store.values, "test-secret://restore")
	return nil
}

type isolatedOnlySecretStore struct{}

func (isolatedOnlySecretStore) Put(context.Context, string, []byte) error { return nil }
func (isolatedOnlySecretStore) Read(context.Context, string) ([]byte, error) {
	return nil, errors.New("not used")
}
func (isolatedOnlySecretStore) Delete(context.Context, string) error { return nil }

type secretRecoveryCellTestOperations struct {
	started []sdk.ResilienceOperationRequest
	store   *secretRecoveryCellTestStore
}

func (operations *secretRecoveryCellTestOperations) Start(_ context.Context, request sdk.ResilienceOperationRequest) (sdk.ResilienceOperationObservation, error) {
	operations.started = append(operations.started, request)
	if request.Action == sdk.ResilienceCleanup {
		return sdk.ResilienceOperationObservation{Status: sdk.ResilienceOperationSucceeded, Action: request.Action, OperationID: "cleanup", OwnershipMarker: request.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true}, nil
	}
	restoreID := "test-secret://restore"
	if request.Destination == sdk.RecoverySameRegion {
		restoreID = request.ResourceReferences[request.DataClasses[0]]
		if operations.store != nil && request.Action == sdk.ResilienceRestore {
			operations.store.values[restoreID] = knownSecretRecoveryValue(request.FixtureID)
		}
	}
	evidence := sdk.ResilienceProofEvidence{DataClass: request.DataClasses[0], FixtureID: request.FixtureID, BackupID: "test-secret://archive", RestoreID: restoreID, EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true}
	return sdk.ResilienceOperationObservation{Status: sdk.ResilienceOperationSucceeded, Action: request.Action, OperationID: string(request.Action), OwnershipMarker: request.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true, Evidence: []sdk.ResilienceProofEvidence{evidence}}, nil
}

func (operations *secretRecoveryCellTestOperations) Poll(_ context.Context, _ string) (sdk.ResilienceOperationObservation, error) {
	return sdk.ResilienceOperationObservation{}, errors.New("poll is not expected in the synchronous test")
}

func (operations *secretRecoveryCellTestOperations) Inventory(_ context.Context, _ string) ([]sdk.ResilienceInventoryResource, error) {
	return []sdk.ResilienceInventoryResource{{Identity: "test-secret://archive", Owned: true, Live: true}}, nil
}
