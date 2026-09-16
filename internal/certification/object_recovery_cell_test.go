package certification

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestObjectRecoveryCellRunsPortableSequenceAndCleansExactFixture(t *testing.T) {
	store := newObjectRecoveryCellTestStore()
	operations := &objectRecoveryCellTestOperations{}
	cell, err := NewObjectRecoveryCell(ObjectRecoveryCellConfig{
		Operations:        operations,
		Store:             store,
		Bucket:            "bucket",
		ResourceReference: "test-object://bucket/fixture",
		DataClass:         "object-data",
		Marker:            "magelift/test/object-cell",
		FixtureID:         "fixture/known-content",
		SourcePrefix:      "fixture/known-content",
		ParseReference: func(reference string) (string, string, error) {
			if !strings.HasPrefix(reference, "test-object://bucket/") {
				return "", "", errors.New("invalid test reference")
			}
			return "bucket", strings.TrimPrefix(reference, "test-object://bucket/"), nil
		},
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
	if result.ObjectCount != 3 || result.BackupID == "" || result.RestoreID == "" || result.FixtureFingerprint == "" {
		t.Fatalf("result = %#v", result)
	}
	if len(operations.started) != 3 || operations.started[0].Action != sdk.ResilienceBackup || operations.started[1].Action != sdk.ResilienceRestore || operations.started[2].Action != sdk.ResilienceIntegrityCheck {
		t.Fatalf("started = %#v", operations.started)
	}
	if operations.started[1].Destination != sdk.RecoverySameRegionIsolated || operations.started[1].ApprovalReference != "" {
		t.Fatalf("isolated restore request = %#v", operations.started[1])
	}
	for _, request := range operations.started {
		if len(request.DataClasses) != 1 || request.DataClasses[0] != "object-data" || request.ResourceReferences["object-data"] != "test-object://bucket/fixture" {
			t.Fatalf("request = %#v", request)
		}
	}
	if err := cell.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(operations.started) != 4 || operations.started[3].Action != sdk.ResilienceCleanup || len(store.objects) != 0 {
		t.Fatalf("cleanup started=%#v objects=%#v", operations.started, store.objects)
	}
}

func TestObjectRecoveryCellPassesApprovalAndSourceIdentityForInPlaceRestore(t *testing.T) {
	store := newObjectRecoveryCellTestStore()
	operations := &objectRecoveryCellTestOperations{store: store}
	cell, err := NewObjectRecoveryCell(ObjectRecoveryCellConfig{
		Operations:        operations,
		Store:             store,
		Bucket:            "bucket",
		ResourceReference: "test-object://bucket/fixture/known-content",
		DataClass:         "object-data",
		Marker:            "magelift/test/object-cell-inplace",
		FixtureID:         "fixture/known-content",
		SourcePrefix:      "fixture/known-content",
		Destination:       sdk.RecoverySameRegion,
		ApprovalReference: "fence-approved",
		ParseReference: func(reference string) (string, string, error) {
			if !strings.HasPrefix(reference, "test-object://bucket/") {
				return "", "", errors.New("invalid test reference")
			}
			return "bucket", strings.TrimPrefix(reference, "test-object://bucket/"), nil
		},
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
	if result.RestoreID != "test-object://bucket/fixture/known-content" || operations.started[1].ApprovalReference != "fence-approved" || operations.started[1].Destination != sdk.RecoverySameRegion {
		t.Fatalf("result=%#v started=%#v", result, operations.started)
	}
}

func TestObjectRecoveryCellMaterializesInfrastructureStateFixture(t *testing.T) {
	store := newObjectRecoveryCellTestStore()
	operations := &objectRecoveryCellTestOperations{}
	cell, err := NewObjectRecoveryCell(ObjectRecoveryCellConfig{
		Operations:        operations,
		Store:             store,
		Bucket:            "bucket",
		ResourceReference: "test-object://bucket/state/fixture",
		DataClass:         "infrastructure-state",
		Marker:            "magelift/test/state-cell",
		FixtureID:         "fixture/known-state",
		SourcePrefix:      "state/fixture",
		ParseReference: func(reference string) (string, string, error) {
			if !strings.HasPrefix(reference, "test-object://bucket/") {
				return "", "", errors.New("invalid test reference")
			}
			return "bucket", strings.TrimPrefix(reference, "test-object://bucket/"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := cell.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.objects["bucket/state/fixture/stack.json"]; !ok {
		t.Fatalf("state fixture missing: %#v", store.objects)
	}
	if _, ok := store.objects["bucket/state/fixture/catalog.json"]; ok {
		t.Fatal("infrastructure-state fixture used media object keys")
	}
}

func TestObjectRecoveryCellMaterializesAuditEvidenceFixture(t *testing.T) {
	store := newObjectRecoveryCellTestStore()
	operations := &objectRecoveryCellTestOperations{}
	cell, err := NewObjectRecoveryCell(ObjectRecoveryCellConfig{
		Operations:        operations,
		Store:             store,
		Bucket:            "bucket",
		ResourceReference: "test-object://bucket/audit/fixture",
		DataClass:         "audit-evidence",
		Marker:            "magelift/test/audit-cell",
		FixtureID:         "fixture/known-audit",
		SourcePrefix:      "audit/fixture",
		ParseReference: func(reference string) (string, string, error) {
			if !strings.HasPrefix(reference, "test-object://bucket/") {
				return "", "", errors.New("invalid test reference")
			}
			return "bucket", strings.TrimPrefix(reference, "test-object://bucket/"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := cell.Prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.objects["bucket/audit/fixture/events.jsonl"]; !ok {
		t.Fatalf("audit fixture missing: %#v", store.objects)
	}
	if _, ok := store.objects["bucket/audit/fixture/catalog.json"]; ok {
		t.Fatal("audit-evidence fixture used media object keys")
	}
	if _, ok := store.objects["bucket/audit/fixture/stack.json"]; ok {
		t.Fatal("audit-evidence fixture used infrastructure-state object keys")
	}
}

func TestObjectRecoveryCellRejectsMissingProviderBoundary(t *testing.T) {
	if _, err := NewObjectRecoveryCell(ObjectRecoveryCellConfig{}); err == nil || !strings.Contains(err.Error(), "operation client") {
		t.Fatalf("error = %v", err)
	}
}

type objectRecoveryCellTestStore struct {
	objects map[string][]byte
}

func newObjectRecoveryCellTestStore() *objectRecoveryCellTestStore {
	return &objectRecoveryCellTestStore{objects: make(map[string][]byte)}
}

func (store *objectRecoveryCellTestStore) Put(_ context.Context, bucket string, object ObjectRecoveryFixture) error {
	store.objects[bucket+"/"+object.Key] = append([]byte(nil), object.Body...)
	return nil
}

func (store *objectRecoveryCellTestStore) Delete(_ context.Context, bucket, key string) error {
	delete(store.objects, bucket+"/"+key)
	return nil
}

func (store *objectRecoveryCellTestStore) ReadPrefix(_ context.Context, bucket, prefix string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	for key, body := range store.objects {
		key = strings.TrimPrefix(key, bucket+"/")
		if strings.HasPrefix(key, prefix+"/") {
			result[key] = append([]byte(nil), body...)
		}
	}
	if strings.HasPrefix(prefix, "restore") {
		for key, body := range store.objects {
			key = strings.TrimPrefix(key, bucket+"/")
			if strings.HasPrefix(key, "fixture/known-content/") {
				relative := strings.TrimPrefix(key, "fixture/known-content/")
				result[prefix+"/"+relative] = append([]byte(nil), body...)
			}
		}
	}
	return result, nil
}

type objectRecoveryCellTestOperations struct {
	started []sdk.ResilienceOperationRequest
	store   *objectRecoveryCellTestStore
}

func (operations *objectRecoveryCellTestOperations) Start(_ context.Context, request sdk.ResilienceOperationRequest) (sdk.ResilienceOperationObservation, error) {
	operations.started = append(operations.started, request)
	restoreID := "test-object://bucket/restore"
	if request.Destination == sdk.RecoverySameRegion {
		restoreID = request.ResourceReferences[request.DataClasses[0]]
		if operations.store != nil && request.Action == sdk.ResilienceRestore {
			for _, object := range knownObjectRecoveryFixture("fixture/known-content") {
				_ = operations.store.Put(context.Background(), "bucket", object)
			}
		}
	}
	evidence := sdk.ResilienceProofEvidence{DataClass: request.DataClasses[0], FixtureID: request.FixtureID, BackupID: "test-object://bucket/archive", RestoreID: restoreID, EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true}
	if request.Action == sdk.ResilienceCleanup {
		return sdk.ResilienceOperationObservation{Status: sdk.ResilienceOperationSucceeded, Action: request.Action, OperationID: "cleanup", OwnershipMarker: request.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true}, nil
	}
	return sdk.ResilienceOperationObservation{Status: sdk.ResilienceOperationSucceeded, Action: request.Action, OperationID: string(request.Action), OwnershipMarker: request.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true, Evidence: []sdk.ResilienceProofEvidence{evidence}}, nil
}

func (operations *objectRecoveryCellTestOperations) Poll(_ context.Context, _ string) (sdk.ResilienceOperationObservation, error) {
	return sdk.ResilienceOperationObservation{}, errors.New("poll is not expected in the synchronous test")
}

func (operations *objectRecoveryCellTestOperations) Inventory(_ context.Context, _ string) ([]sdk.ResilienceInventoryResource, error) {
	return []sdk.ResilienceInventoryResource{{Identity: "test-object://bucket/archive", Owned: true, Live: true}}, nil
}
