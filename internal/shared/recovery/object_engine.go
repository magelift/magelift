package recovery

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ObjectInfo is the provider-neutral listing result required by the archive
// engine. Validator is opaque because S3 ETags and GCS ETags have different
// guarantees, but it is still useful for detecting an archive mutation.
type ObjectInfo struct {
	Key       string
	Size      int64
	Validator string
}

type ObjectMetadata struct {
	Size          int64
	Validator     string
	Metadata      map[string]string
	Encryption    string
	EncryptionKey string
	Protected     bool
}

// ObjectStore is the only storage port needed by the provider-neutral object
// archive algorithm. Provider packages adapt official SDK clients to it.
type ObjectStore interface {
	List(context.Context, string, string) ([]ObjectInfo, error)
	Head(context.Context, string, string) (ObjectMetadata, error)
	Read(context.Context, string, string) ([]byte, error)
	Put(context.Context, string, string, []byte, map[string]string) error
	Copy(context.Context, string, string, string, string, map[string]string) error
}

type ObjectArchiveScope struct {
	DataClass       string
	FixtureID       string
	OwnershipMarker string
	IdempotencyKey  string
	SourceBucket    string
	SourcePrefix    string
}

type ObjectArchiveConfig struct {
	ArchiveBucket      string
	ArchivePrefix      string
	RestoreBucket      string
	RestorePrefix      string
	IsNotFound         func(error) bool
	EncryptionVerified func(ObjectMetadata) bool
	ProtectionVerified func(ObjectMetadata) bool
}

type ObjectArchiveEngine struct {
	store  ObjectStore
	config ObjectArchiveConfig
}

func NewObjectArchiveEngine(store ObjectStore, config ObjectArchiveConfig) (*ObjectArchiveEngine, error) {
	if store == nil {
		return nil, errors.New("object archive store is required")
	}
	if strings.TrimSpace(config.ArchiveBucket) == "" || strings.TrimSpace(config.ArchivePrefix) == "" || strings.TrimSpace(config.RestoreBucket) == "" || strings.TrimSpace(config.RestorePrefix) == "" {
		return nil, errors.New("object archive buckets and prefixes are required")
	}
	if config.IsNotFound == nil || config.EncryptionVerified == nil || config.ProtectionVerified == nil {
		return nil, errors.New("object archive provider policy callbacks are required")
	}
	return &ObjectArchiveEngine{store: store, config: config}, nil
}

// Backup copies an ownership-scoped source prefix into a deterministic
// archive, then seals a manifest. Existing owned objects and manifests are
// reused so a retry after interruption does not duplicate work.
func (e *ObjectArchiveEngine) Backup(ctx context.Context, scope ObjectArchiveScope) (ObjectArchiveManifest, error) {
	if err := validateScope(scope); err != nil {
		return ObjectArchiveManifest{}, err
	}
	archivePrefix := e.archivePrefix(scope)
	if scope.SourceBucket == e.config.ArchiveBucket && prefixesOverlap(archivePrefix, scope.SourcePrefix) {
		return ObjectArchiveManifest{}, errors.New("object archive prefix must not be inside its source prefix")
	}
	objects, err := e.store.List(ctx, scope.SourceBucket, scope.SourcePrefix)
	if err != nil {
		return ObjectArchiveManifest{}, fmt.Errorf("list object archive source: %w", err)
	}
	if len(objects) == 0 {
		return ObjectArchiveManifest{}, fmt.Errorf("object archive source %q contains no objects", scope.SourcePrefix)
	}
	manifestKey := e.operationPrefix(scope) + "/manifest.json"
	if manifest, ok, err := e.readManifest(ctx, scope, manifestKey); err != nil {
		return ObjectArchiveManifest{}, err
	} else if ok {
		return manifest, nil
	}

	entries := make([]ObjectArchiveEntry, 0, len(objects))
	for _, object := range objects {
		key := strings.TrimSpace(object.Key)
		if key == "" || object.Size < 0 {
			return ObjectArchiveManifest{}, errors.New("object archive source returned an invalid object")
		}
		targetKey := SafeArchiveKey(e.operationPrefix(scope)+"/objects", RelativeObjectKey(key, scope.SourcePrefix))
		if targetKey == "" {
			return ObjectArchiveManifest{}, fmt.Errorf("object archive source key %q cannot be archived safely", key)
		}
		owned, err := e.owned(ctx, e.config.ArchiveBucket, targetKey, scope)
		if err != nil {
			return ObjectArchiveManifest{}, err
		}
		if !owned {
			if err := e.store.Copy(ctx, scope.SourceBucket, key, e.config.ArchiveBucket, targetKey, MetadataFor(scope, false)); err != nil {
				return ObjectArchiveManifest{}, fmt.Errorf("copy object archive source %q: %w", key, err)
			}
			if err := e.verify(ctx, e.config.ArchiveBucket, targetKey, scope); err != nil {
				return ObjectArchiveManifest{}, fmt.Errorf("verify archived object %q: %w", key, err)
			}
		}
		archived, err := e.store.Head(ctx, e.config.ArchiveBucket, targetKey)
		if err != nil {
			return ObjectArchiveManifest{}, fmt.Errorf("inspect archived object %q: %w", key, err)
		}
		entries = append(entries, ObjectArchiveEntry{SourceKey: key, ArchiveKey: targetKey, Size: object.Size, ETag: archived.Validator})
	}
	manifest := ObjectArchiveManifest{Version: OperationVersion, DataClass: scope.DataClass, FixtureID: scope.FixtureID, OwnershipMarker: scope.OwnershipMarker, SourceBucket: scope.SourceBucket, SourcePrefix: scope.SourcePrefix, ArchiveBucket: e.config.ArchiveBucket, ArchivePrefix: e.operationPrefix(scope), Entries: entries}
	body, _, err := SealObjectManifest(manifest)
	if err != nil {
		return ObjectArchiveManifest{}, fmt.Errorf("seal object archive manifest: %w", err)
	}
	if existing, headErr := e.store.Head(ctx, e.config.ArchiveBucket, manifestKey); headErr == nil {
		if !e.matches(existing, scope) || !e.policySatisfied(existing) {
			return ObjectArchiveManifest{}, errors.New("object archive manifest appeared with an unexpected owner or policy")
		}
		return ObjectArchiveManifest{}, errors.New("object archive manifest appeared concurrently; retry the idempotent operation")
	} else if !e.config.IsNotFound(headErr) {
		return ObjectArchiveManifest{}, fmt.Errorf("inspect object archive manifest: %w", headErr)
	}
	if err := e.store.Put(ctx, e.config.ArchiveBucket, manifestKey, body, MetadataFor(scope, true)); err != nil {
		return ObjectArchiveManifest{}, fmt.Errorf("write object archive manifest: %w", err)
	}
	if err := e.verify(ctx, e.config.ArchiveBucket, manifestKey, scope); err != nil {
		return ObjectArchiveManifest{}, fmt.Errorf("verify object archive manifest: %w", err)
	}
	return manifest, nil
}

func (e *ObjectArchiveEngine) ReadManifest(ctx context.Context, scope ObjectArchiveScope, bucket, key string) (ObjectArchiveManifest, error) {
	if err := validateScope(scope); err != nil {
		return ObjectArchiveManifest{}, err
	}
	metadata, err := e.store.Head(ctx, bucket, key)
	if err != nil {
		return ObjectArchiveManifest{}, fmt.Errorf("inspect object archive manifest: %w", err)
	}
	if !e.matches(metadata, scope) || !e.policySatisfied(metadata) {
		return ObjectArchiveManifest{}, errors.New("object archive manifest does not satisfy ownership or protection policy")
	}
	body, err := e.store.Read(ctx, bucket, key)
	if err != nil {
		return ObjectArchiveManifest{}, fmt.Errorf("read object archive manifest: %w", err)
	}
	manifest, _, err := OpenObjectManifest(body)
	if err != nil {
		return ObjectArchiveManifest{}, fmt.Errorf("decode object archive manifest: %w", err)
	}
	if err := ValidateObjectManifest(manifest, scope.DataClass, scope.FixtureID, scope.OwnershipMarker); err != nil {
		return ObjectArchiveManifest{}, fmt.Errorf("validate object archive manifest: %w", err)
	}
	if manifest.ArchiveBucket != bucket {
		return ObjectArchiveManifest{}, errors.New("object archive manifest bucket does not match the backup reference")
	}
	return manifest, nil
}

// Restore copies all manifest entries to a deterministic isolated prefix.
func (e *ObjectArchiveEngine) Restore(ctx context.Context, scope ObjectArchiveScope, manifest ObjectArchiveManifest) (string, error) {
	return e.restoreInto(ctx, scope, manifest, e.config.RestoreBucket, e.restorePrefix(scope), false)
}

// RestoreInPlace copies all manifest entries onto the source prefix, overwriting
// existing objects in that prefix. Callers must require an operator approval
// before invoking this path.
func (e *ObjectArchiveEngine) RestoreInPlace(ctx context.Context, scope ObjectArchiveScope, manifest ObjectArchiveManifest) (string, error) {
	return e.restoreInto(ctx, scope, manifest, scope.SourceBucket, strings.Trim(scope.SourcePrefix, "/"), true)
}

func (e *ObjectArchiveEngine) restoreInto(ctx context.Context, scope ObjectArchiveScope, manifest ObjectArchiveManifest, bucket, targetPrefix string, overwrite bool) (string, error) {
	if err := ValidateObjectManifest(manifest, scope.DataClass, scope.FixtureID, scope.OwnershipMarker); err != nil {
		return "", err
	}
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(targetPrefix) == "" {
		return "", errors.New("object restore target bucket and prefix are required")
	}
	for _, entry := range manifest.Entries {
		targetKey := SafeArchiveKey(targetPrefix, RelativeObjectKey(entry.ArchiveKey, manifest.ArchivePrefix+"/objects"))
		if targetKey == "" {
			return "", fmt.Errorf("object restore target for %q is unsafe", entry.ArchiveKey)
		}
		if !overwrite {
			owned, err := e.owned(ctx, bucket, targetKey, scope)
			if err != nil {
				return "", err
			}
			if owned {
				continue
			}
		}
		if err := e.store.Copy(ctx, manifest.ArchiveBucket, entry.ArchiveKey, bucket, targetKey, MetadataFor(scope, false)); err != nil {
			return "", fmt.Errorf("restore object %q: %w", entry.ArchiveKey, err)
		}
		if err := e.verify(ctx, bucket, targetKey, scope); err != nil {
			return "", fmt.Errorf("verify restored object %q: %w", entry.ArchiveKey, err)
		}
	}
	return targetPrefix, nil
}

func (e *ObjectArchiveEngine) VerifyRestore(ctx context.Context, scope ObjectArchiveScope, manifest ObjectArchiveManifest) (string, error) {
	return e.verifyRestoreInto(ctx, scope, manifest, e.config.RestoreBucket, e.restorePrefix(scope))
}

func (e *ObjectArchiveEngine) VerifyRestoreInPlace(ctx context.Context, scope ObjectArchiveScope, manifest ObjectArchiveManifest) (string, error) {
	return e.verifyRestoreInto(ctx, scope, manifest, scope.SourceBucket, strings.Trim(scope.SourcePrefix, "/"))
}

func (e *ObjectArchiveEngine) verifyRestoreInto(ctx context.Context, scope ObjectArchiveScope, manifest ObjectArchiveManifest, bucket, targetPrefix string) (string, error) {
	if err := ValidateObjectManifest(manifest, scope.DataClass, scope.FixtureID, scope.OwnershipMarker); err != nil {
		return "", err
	}
	for _, entry := range manifest.Entries {
		targetKey := SafeArchiveKey(targetPrefix, RelativeObjectKey(entry.ArchiveKey, manifest.ArchivePrefix+"/objects"))
		if targetKey == "" {
			return "", fmt.Errorf("object restore target for %q is unsafe", entry.ArchiveKey)
		}
		owned, err := e.owned(ctx, bucket, targetKey, scope)
		if err != nil {
			return "", err
		}
		if !owned {
			return "", ErrObjectNotReady
		}
	}
	return targetPrefix, nil
}

var ErrObjectNotReady = errors.New("object restore is not complete")

func (e *ObjectArchiveEngine) Integrity(ctx context.Context, scope ObjectArchiveScope, manifest ObjectArchiveManifest) error {
	if err := ValidateObjectManifest(manifest, scope.DataClass, scope.FixtureID, scope.OwnershipMarker); err != nil {
		return err
	}
	for _, entry := range manifest.Entries {
		metadata, err := e.store.Head(ctx, manifest.ArchiveBucket, entry.ArchiveKey)
		if err != nil {
			return fmt.Errorf("inspect object archive entry %q: %w", entry.ArchiveKey, err)
		}
		if !e.matches(metadata, scope) || !e.policySatisfied(metadata) {
			return fmt.Errorf("object archive entry %q failed ownership or policy verification", entry.ArchiveKey)
		}
		if metadata.Size != entry.Size {
			return fmt.Errorf("object archive entry %q size changed", entry.ArchiveKey)
		}
		if entry.ETag != "" && metadata.Validator != "" && entry.ETag != metadata.Validator {
			return fmt.Errorf("object archive entry %q validator changed", entry.ArchiveKey)
		}
	}
	return nil
}

func MetadataFor(scope ObjectArchiveScope, manifest bool) map[string]string {
	metadata := map[string]string{OwnershipMetadataKey: scope.OwnershipMarker, ClassMetadataKey: scope.DataClass, FixtureMetadataKey: scope.FixtureID}
	if manifest {
		metadata[ManifestMetadataKey] = "true"
	}
	return metadata
}

const (
	OwnershipMetadataKey = "magelift-ownership"
	ClassMetadataKey     = "magelift-data-class"
	FixtureMetadataKey   = "magelift-fixture"
	ManifestMetadataKey  = "magelift-manifest"
)

func validateScope(scope ObjectArchiveScope) error {
	if strings.TrimSpace(scope.DataClass) == "" || strings.TrimSpace(scope.FixtureID) == "" || strings.TrimSpace(scope.OwnershipMarker) == "" || strings.TrimSpace(scope.IdempotencyKey) == "" || strings.TrimSpace(scope.SourceBucket) == "" || strings.TrimSpace(scope.SourcePrefix) == "" {
		return errors.New("object archive scope is incomplete")
	}
	return nil
}

func (e *ObjectArchiveEngine) archivePrefix(scope ObjectArchiveScope) string {
	return strings.Trim(e.config.ArchivePrefix, "/") + "/" + Digest([]byte(scope.OwnershipMarker))[:24]
}

func (e *ObjectArchiveEngine) operationPrefix(scope ObjectArchiveScope) string {
	return e.archivePrefix(scope) + "/" + scope.DataClass + "/" + Digest([]byte(scope.IdempotencyKey))[:24]
}

func (e *ObjectArchiveEngine) restorePrefix(scope ObjectArchiveScope) string {
	return strings.Trim(e.config.RestorePrefix, "/") + "/" + scope.DataClass + "/" + Digest([]byte(scope.OwnershipMarker))[:24] + "/" + Digest([]byte(scope.IdempotencyKey))[:24]
}

func (e *ObjectArchiveEngine) readManifest(ctx context.Context, scope ObjectArchiveScope, key string) (ObjectArchiveManifest, bool, error) {
	metadata, err := e.store.Head(ctx, e.config.ArchiveBucket, key)
	if err != nil {
		if e.config.IsNotFound(err) {
			return ObjectArchiveManifest{}, false, nil
		}
		return ObjectArchiveManifest{}, false, fmt.Errorf("inspect object archive manifest: %w", err)
	}
	if !e.matches(metadata, scope) || metadata.Metadata[ManifestMetadataKey] != "true" || !e.policySatisfied(metadata) {
		return ObjectArchiveManifest{}, false, errors.New("object archive manifest exists but is not owned by this operation")
	}
	body, err := e.store.Read(ctx, e.config.ArchiveBucket, key)
	if err != nil {
		return ObjectArchiveManifest{}, false, fmt.Errorf("read object archive manifest: %w", err)
	}
	manifest, _, err := OpenObjectManifest(body)
	if err != nil {
		return ObjectArchiveManifest{}, false, fmt.Errorf("decode object archive manifest: %w", err)
	}
	if err := ValidateObjectManifest(manifest, scope.DataClass, scope.FixtureID, scope.OwnershipMarker); err != nil {
		return ObjectArchiveManifest{}, false, fmt.Errorf("validate object archive manifest: %w", err)
	}
	return manifest, true, nil
}

func (e *ObjectArchiveEngine) owned(ctx context.Context, bucket, key string, scope ObjectArchiveScope) (bool, error) {
	metadata, err := e.store.Head(ctx, bucket, key)
	if err != nil {
		if e.config.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("inspect object %q: %w", key, err)
	}
	if !e.matches(metadata, scope) || !e.policySatisfied(metadata) {
		return false, fmt.Errorf("refusing to overwrite an unowned or insufficiently protected object %q", key)
	}
	return true, nil
}

func (e *ObjectArchiveEngine) verify(ctx context.Context, bucket, key string, scope ObjectArchiveScope) error {
	metadata, err := e.store.Head(ctx, bucket, key)
	if err != nil {
		return err
	}
	if !e.matches(metadata, scope) || !e.policySatisfied(metadata) {
		return errors.New("object failed ownership, encryption, or protection verification")
	}
	return nil
}

func (e *ObjectArchiveEngine) matches(metadata ObjectMetadata, scope ObjectArchiveScope) bool {
	return metadata.Metadata[OwnershipMetadataKey] == scope.OwnershipMarker && metadata.Metadata[ClassMetadataKey] == scope.DataClass && metadata.Metadata[FixtureMetadataKey] == scope.FixtureID
}

func (e *ObjectArchiveEngine) policySatisfied(metadata ObjectMetadata) bool {
	return e.config.EncryptionVerified(metadata) && e.config.ProtectionVerified(metadata)
}

func prefixesOverlap(archivePrefix, sourcePrefix string) bool {
	archivePrefix = strings.Trim(archivePrefix, "/")
	sourcePrefix = strings.Trim(sourcePrefix, "/")
	return archivePrefix == sourcePrefix || strings.HasPrefix(archivePrefix, sourcePrefix+"/")
}
