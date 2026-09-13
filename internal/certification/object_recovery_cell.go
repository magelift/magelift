package certification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// ObjectRecoveryFixture is a scrubbed known-content object used by the
// provider-neutral recovery cell. Providers decide how to put, read, and
// delete it; the operation sequence and proof requirements live here once.
type ObjectRecoveryFixture struct {
	Key  string
	Body []byte
}

// ObjectRecoveryStore is the smallest provider adapter required by the live
// object recovery cell. It deliberately carries no SDK models or cloud
// endpoint details across the boundary.
type ObjectRecoveryStore interface {
	Put(context.Context, string, ObjectRecoveryFixture) error
	Delete(context.Context, string, string) error
	ReadPrefix(context.Context, string, string) (map[string][]byte, error)
}

// ObjectRecoveryCellConfig binds one disposable object-storage cell to the
// normalized operation client. Bucket creation and deletion remain outside
// this core runner because owning CLIs and deletion semantics differ by
// provider.
type ObjectRecoveryCellConfig struct {
	Operations        sdk.ResilienceOperationClient
	Store             ObjectRecoveryStore
	Bucket            string
	ResourceReference string
	DataClass         string
	Marker            string
	FixtureID         string
	SourcePrefix      string
	Destination       sdk.RecoveryDestination
	ApprovalReference string
	ParseReference    func(string) (bucket, prefix string, err error)
}

// ObjectRecoveryCell is reusable by AWS S3, GCP Cloud Storage, Scaleway
// Object Storage, OVHcloud Object Storage, and community S3-compatible
// adapters. Only the provider-local store and reference parser vary.
type ObjectRecoveryCell struct {
	config  ObjectRecoveryCellConfig
	fixture []ObjectRecoveryFixture
}

type ObjectRecoveryCellResult struct {
	ObjectCount        int
	BackupID           string
	RestoreID          string
	FixtureFingerprint string
}

func NewObjectRecoveryCell(config ObjectRecoveryCellConfig) (*ObjectRecoveryCell, error) {
	if config.Operations == nil {
		return nil, errors.New("object recovery cell operation client is required")
	}
	if config.Store == nil {
		return nil, errors.New("object recovery cell object store is required")
	}
	if config.ParseReference == nil {
		return nil, errors.New("object recovery cell object-reference parser is required")
	}
	for _, part := range []struct {
		value string
		name  string
	}{
		{config.Bucket, "bucket"}, {config.ResourceReference, "resource reference"}, {config.Marker, "ownership marker"}, {config.FixtureID, "fixture ID"}, {config.SourcePrefix, "source prefix"},
	} {
		if strings.TrimSpace(part.value) == "" || strings.ContainsAny(part.value, "\r\n\x00") {
			return nil, fmt.Errorf("object recovery cell %s is required and must be single-line", part.name)
		}
	}
	if strings.TrimSpace(config.DataClass) == "" {
		config.DataClass = "media"
	}
	if strings.ContainsAny(config.DataClass, "\r\n\x00") {
		return nil, errors.New("object recovery cell data class must be single-line")
	}
	if config.Destination == "" {
		config.Destination = sdk.RecoverySameRegionIsolated
	}
	return &ObjectRecoveryCell{config: config, fixture: knownObjectRecoveryFixtureFor(config.DataClass, config.SourcePrefix)}, nil
}

// Fixtures returns a defensive copy of the source fixture definition.
func (cell *ObjectRecoveryCell) Fixtures() []ObjectRecoveryFixture {
	if cell == nil {
		return nil
	}
	result := make([]ObjectRecoveryFixture, len(cell.fixture))
	for index, object := range cell.fixture {
		result[index] = ObjectRecoveryFixture{Key: object.Key, Body: bytes.Clone(object.Body)}
	}
	return result
}

// Prepare writes only the core's known-content objects. The caller owns the
// disposable bucket and must invoke Cleanup even when this method fails.
func (cell *ObjectRecoveryCell) Prepare(ctx context.Context) error {
	if err := validateCellContext(ctx); err != nil {
		return err
	}
	for _, object := range cell.fixture {
		if err := cell.config.Store.Put(ctx, cell.config.Bucket, object); err != nil {
			return fmt.Errorf("put object recovery fixture %q: %w", object.Key, err)
		}
	}
	return nil
}

// Run executes backup, isolated restore, independent content verification,
// and archive-integrity verification. It intentionally does not clean up so
// callers can inspect the pre-cleanup inventory and report provider failures.
func (cell *ObjectRecoveryCell) Run(ctx context.Context) (ObjectRecoveryCellResult, error) {
	if cell == nil {
		return ObjectRecoveryCellResult{}, errors.New("object recovery cell is required")
	}
	if err := validateCellContext(ctx); err != nil {
		return ObjectRecoveryCellResult{}, err
	}
	base := sdk.ResilienceOperationRequest{
		DataClasses:        []string{cell.config.DataClass},
		Destination:        cell.config.Destination,
		FixtureID:          cell.config.FixtureID,
		OwnershipMarker:    cell.config.Marker,
		ResourceReferences: map[string]string{cell.config.DataClass: cell.config.ResourceReference},
		ApprovalReference:  cell.config.ApprovalReference,
	}

	backupRequest := base
	backupRequest.Action = sdk.ResilienceBackup
	backupRequest.IdempotencyKey = "live/" + cell.config.DataClass + "/backup"
	backup, err := cell.config.Operations.Start(ctx, backupRequest)
	if err != nil {
		return ObjectRecoveryCellResult{}, fmt.Errorf("start object recovery backup: %w", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || len(backup.Evidence) != 1 || backup.Evidence[0].BackupID == "" || !backup.Evidence[0].EncryptionVerified || !backup.Evidence[0].ProtectionVerified || !backup.Evidence[0].ManifestVerified {
		return ObjectRecoveryCellResult{}, fmt.Errorf("object recovery backup evidence is incomplete: %#v", backup.Evidence)
	}

	if cell.config.Destination == sdk.RecoverySameRegion {
		for _, object := range cell.fixture {
			if err := cell.config.Store.Delete(ctx, cell.config.Bucket, object.Key); err != nil {
				return ObjectRecoveryCellResult{}, fmt.Errorf("clear object recovery source fixture %q before in-place restore: %w", object.Key, err)
			}
		}
	}

	restoreRequest := base
	restoreRequest.Action = sdk.ResilienceRestore
	restoreRequest.IdempotencyKey = "live/" + cell.config.DataClass + "/restore"
	restoreRequest.BackupReferences = map[string]string{cell.config.DataClass: backup.Evidence[0].BackupID}
	restored, err := cell.config.Operations.Start(ctx, restoreRequest)
	if err != nil {
		return ObjectRecoveryCellResult{}, fmt.Errorf("restore object recovery backup: %w", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || len(restored.Evidence) != 1 || restored.Evidence[0].RestoreID == "" || !restored.Evidence[0].CountsVerified || !restored.Evidence[0].PermissionsVerified || !restored.Evidence[0].ServiceHealthVerified {
		return ObjectRecoveryCellResult{}, fmt.Errorf("object recovery restore evidence is incomplete: %#v", restored.Evidence)
	}
	restoreBucket, restorePrefix, err := cell.config.ParseReference(restored.Evidence[0].RestoreID)
	if err != nil {
		return ObjectRecoveryCellResult{}, fmt.Errorf("parse object recovery restore reference: %w", err)
	}
	if restoreBucket != cell.config.Bucket {
		return ObjectRecoveryCellResult{}, fmt.Errorf("object recovery restore bucket %q does not match source bucket %q", restoreBucket, cell.config.Bucket)
	}
	var source map[string][]byte
	if cell.config.Destination == sdk.RecoverySameRegion {
		expected := make(map[string][]byte, len(cell.fixture))
		for _, object := range cell.fixture {
			expected[object.Key] = append([]byte(nil), object.Body...)
		}
		restoredObjects, err := cell.config.Store.ReadPrefix(ctx, restoreBucket, restorePrefix)
		if err != nil {
			return ObjectRecoveryCellResult{}, fmt.Errorf("read object recovery in-place restore: %w", err)
		}
		if err := compareObjectRecoveryObjects(expected, restoredObjects, cell.config.SourcePrefix, restorePrefix); err != nil {
			return ObjectRecoveryCellResult{}, err
		}
		source = expected
	} else {
		var err error
		source, err = cell.config.Store.ReadPrefix(ctx, cell.config.Bucket, cell.config.SourcePrefix)
		if err != nil {
			return ObjectRecoveryCellResult{}, fmt.Errorf("read object recovery source fixture: %w", err)
		}
		restoredObjects, err := cell.config.Store.ReadPrefix(ctx, restoreBucket, restorePrefix)
		if err != nil {
			return ObjectRecoveryCellResult{}, fmt.Errorf("read object recovery isolated restore: %w", err)
		}
		if err := compareObjectRecoveryObjects(source, restoredObjects, cell.config.SourcePrefix, restorePrefix); err != nil {
			return ObjectRecoveryCellResult{}, err
		}
	}

	integrityRequest := base
	integrityRequest.Action = sdk.ResilienceIntegrityCheck
	integrityRequest.IdempotencyKey = "live/" + cell.config.DataClass + "/integrity"
	integrityRequest.BackupReferences = map[string]string{cell.config.DataClass: backup.Evidence[0].BackupID}
	integrity, err := cell.config.Operations.Start(ctx, integrityRequest)
	if err != nil {
		return ObjectRecoveryCellResult{}, fmt.Errorf("verify object recovery archive integrity: %w", err)
	}
	if integrity.Status != sdk.ResilienceOperationSucceeded || len(integrity.Evidence) != 1 || !integrity.Evidence[0].ManifestVerified || !integrity.Evidence[0].CountsVerified || !integrity.Evidence[0].PermissionsVerified || !integrity.Evidence[0].ServiceHealthVerified {
		return ObjectRecoveryCellResult{}, fmt.Errorf("object recovery integrity evidence is incomplete: %#v", integrity.Evidence)
	}
	inventory, err := cell.config.Operations.Inventory(ctx, cell.config.Marker)
	if err != nil {
		return ObjectRecoveryCellResult{}, fmt.Errorf("inventory object recovery outputs: %w", err)
	}
	if len(inventory) == 0 {
		return ObjectRecoveryCellResult{}, errors.New("object recovery inventory was empty before cleanup")
	}
	return ObjectRecoveryCellResult{ObjectCount: len(source), BackupID: backup.Evidence[0].BackupID, RestoreID: restored.Evidence[0].RestoreID, FixtureFingerprint: objectRecoveryFingerprint(source)}, nil
}

// Cleanup executes the provider-neutral cleanup operation and removes the
// exact source fixture objects. The wrapper remains responsible for deleting
// the bucket or container after this returns.
func (cell *ObjectRecoveryCell) Cleanup(ctx context.Context) error {
	if cell == nil {
		return errors.New("object recovery cell is required")
	}
	if err := validateCellContext(ctx); err != nil {
		return err
	}
	var cleanupErrors []error
	_, err := cell.config.Operations.Start(ctx, sdk.ResilienceOperationRequest{
		Action:             sdk.ResilienceCleanup,
		DataClasses:        []string{cell.config.DataClass},
		Destination:        cell.config.Destination,
		FixtureID:          cell.config.FixtureID,
		OwnershipMarker:    cell.config.Marker,
		IdempotencyKey:     "live/" + cell.config.DataClass + "/cleanup",
		ResourceReferences: map[string]string{cell.config.DataClass: cell.config.ResourceReference},
		ApprovalReference:  cell.config.ApprovalReference,
	})
	if err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("clean object recovery outputs: %w", err))
	}
	for _, object := range cell.fixture {
		if err := cell.config.Store.Delete(ctx, cell.config.Bucket, object.Key); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete object recovery source fixture %q: %w", object.Key, err))
		}
	}
	return errors.Join(cleanupErrors...)
}

func knownObjectRecoveryFixture(prefix string) []ObjectRecoveryFixture {
	return knownObjectRecoveryFixtureFor("media", prefix)
}

func knownObjectRecoveryFixtureFor(dataClass, prefix string) []ObjectRecoveryFixture {
	switch dataClass {
	case "infrastructure-state":
		return []ObjectRecoveryFixture{
			{Key: prefix + "/stack.json", Body: []byte(`{"checkpoint":"fixture-stack","version":1}`)},
			{Key: prefix + "/lock.json", Body: []byte(`{"owner":"magelift-fixture"}`)},
			{Key: prefix + "/nested/checksum.bin", Body: []byte{0x00, 0x01, 0x02, 0x03, 0xfe, 0xff}},
		}
	case "audit-evidence":
		return []ObjectRecoveryFixture{
			{Key: prefix + "/events.jsonl", Body: []byte("{\"event\":\"fixture-audit\",\"n\":1}\n")},
			{Key: prefix + "/digest.sha256", Body: []byte("known-audit-digest\n")},
			{Key: prefix + "/nested/checksum.bin", Body: []byte{0x00, 0x01, 0x02, 0x03, 0xfe, 0xff}},
		}
	default:
		return []ObjectRecoveryFixture{
			{Key: prefix + "/catalog.json", Body: []byte(`{"sku":"fixture-sku","version":1}`)},
			{Key: prefix + "/media/product.txt", Body: []byte("known-content-product\n")},
			{Key: prefix + "/nested/checksum.bin", Body: []byte{0x00, 0x01, 0x02, 0x03, 0xfe, 0xff}},
		}
	}
}

func compareObjectRecoveryObjects(source, restored map[string][]byte, sourcePrefix, restorePrefix string) error {
	if len(source) != len(restored) {
		return fmt.Errorf("object recovery isolated restore object count %d does not match source count %d", len(restored), len(source))
	}
	for sourceKey, sourceBody := range source {
		relative := strings.TrimPrefix(sourceKey, strings.TrimSuffix(sourcePrefix, "/")+"/")
		restoredKey := strings.TrimSuffix(restorePrefix, "/") + "/" + relative
		restoredBody, ok := restored[restoredKey]
		if !ok || !bytes.Equal(sourceBody, restoredBody) {
			return fmt.Errorf("object recovery isolated restore content mismatch for %q", sourceKey)
		}
	}
	return nil
}

func objectRecoveryFingerprint(objects map[string][]byte) string {
	keys := make([]string, 0, len(objects))
	for key := range objects {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	input := make([]byte, 0)
	for _, key := range keys {
		input = append(input, key...)
		input = append(input, 0)
		input = append(input, objects[key]...)
		input = append(input, 0)
	}
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:])[:16]
}

func validateCellContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("object recovery cell context is required")
	}
	return ctx.Err()
}
