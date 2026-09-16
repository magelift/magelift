package resilience

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

type gcsObjectStore struct {
	api           GCSAPI
	kmsKey        string
	retentionDays int
	lockRetention bool
	now           func() time.Time
}

func (s gcsObjectStore) List(ctx context.Context, bucket, prefix string) ([]cloudrecovery.ObjectInfo, error) {
	objects, err := s.api.List(ctx, bucket, prefix)
	if err != nil {
		return nil, err
	}
	result := make([]cloudrecovery.ObjectInfo, 0, len(objects))
	for _, object := range objects {
		result = append(result, cloudrecovery.ObjectInfo{Key: object.Key, Size: object.Size, Validator: object.ETag})
	}
	return result, nil
}

func (s gcsObjectStore) Head(ctx context.Context, bucket, key string) (cloudrecovery.ObjectMetadata, error) {
	metadata, err := s.api.Head(ctx, bucket, key)
	if err != nil {
		return cloudrecovery.ObjectMetadata{}, err
	}
	return cloudrecovery.ObjectMetadata{Size: metadata.Size, Validator: metadata.ETag, Metadata: cloneMetadata(metadata.Metadata), Encryption: "cloud-storage", EncryptionKey: metadata.KMSKeyName, Protected: strings.EqualFold(metadata.RetentionMode, "locked")}, nil
}

func (s gcsObjectStore) Read(ctx context.Context, bucket, key string) ([]byte, error) {
	return s.api.Read(ctx, bucket, key)
}

func (s gcsObjectStore) Put(ctx context.Context, bucket, key string, body []byte, metadata map[string]string) error {
	if s.lockRetention {
		retentionAPI, ok := s.api.(GCSRetentionAPI)
		if !ok {
			return errors.New("GCP Cloud Storage adapter cannot set locked object retention")
		}
		return retentionAPI.PutWithRetention(ctx, bucket, key, body, metadata, s.kmsKey, s.retentionUntil())
	}
	return s.api.Put(ctx, bucket, key, body, metadata, s.kmsKey)
}

func (s gcsObjectStore) Copy(ctx context.Context, sourceBucket, sourceKey, targetBucket, targetKey string, metadata map[string]string) error {
	if s.lockRetention {
		retentionAPI, ok := s.api.(GCSRetentionAPI)
		if !ok {
			return errors.New("GCP Cloud Storage adapter cannot set locked object retention")
		}
		return retentionAPI.CopyWithRetention(ctx, sourceBucket, sourceKey, targetBucket, targetKey, metadata, s.kmsKey, s.retentionUntil())
	}
	return s.api.Copy(ctx, sourceBucket, sourceKey, targetBucket, targetKey, metadata, s.kmsKey)
}

func (s gcsObjectStore) retentionUntil() time.Time {
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	return now.Add(time.Duration(s.retentionDays) * 24 * time.Hour)
}

func (api *NativeAPI) objectEngine() (*cloudrecovery.ObjectArchiveEngine, error) {
	if api == nil || api.storage == nil {
		return nil, errors.New("GCP Cloud Storage recovery API is required")
	}
	return cloudrecovery.NewObjectArchiveEngine(gcsObjectStore{api: api.storage, kmsKey: api.config.KMSKeyName, retentionDays: api.retentionDays(), lockRetention: api.config.RequireLockedRetention, now: api.config.Now}, cloudrecovery.ObjectArchiveConfig{
		ArchiveBucket: api.config.ArchiveBucket, ArchivePrefix: api.config.ArchivePrefix, RestoreBucket: api.config.RestoreBucket, RestorePrefix: api.config.RestorePrefix,
		IsNotFound: isGCSNotFound,
		EncryptionVerified: func(metadata cloudrecovery.ObjectMetadata) bool {
			return !api.config.RequireCMEK || metadata.EncryptionKey == api.config.KMSKeyName
		},
		ProtectionVerified: func(metadata cloudrecovery.ObjectMetadata) bool {
			return !api.config.RequireLockedRetention || metadata.Protected
		},
	})
}

func (api *NativeAPI) startObjects(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupObjectsForState(ctx, state, operationID)
	}
	if _, _, err := parseGCSReference(state.Resource); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	engine, err := api.objectEngine()
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	scope, err := gcpObjectScope(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		manifest, err := engine.Backup(ctx, scope)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return objectBackupObservation(api, state, operationID, manifest), nil
	case sdk.ResilienceRestore:
		return api.restoreObjects(ctx, state, operationID, engine, scope)
	case sdk.ResilienceIntegrityCheck:
		return api.integrityObjects(ctx, state, operationID, engine, scope)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Cloud Storage recovery does not implement this action")
	}
}

func (api *NativeAPI) pollObjects(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupObjectsForState(ctx, state, operationID)
	}
	engine, err := api.objectEngine()
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	scope, err := gcpObjectScope(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	manifestBucket, manifestKey, err := parseGCSReference(state.Backup)
	if state.Action == sdk.ResilienceBackup {
		manifestBucket = api.config.ArchiveBucket
		manifestKey = api.objectPrefixForOperation(state) + "/manifest.json"
	}
	if err != nil && state.Action != sdk.ResilienceBackup {
		return provider.NativeOperationObservation{}, err
	}
	manifest, err := engine.ReadManifest(ctx, scope, manifestBucket, manifestKey)
	if err != nil {
		if isGCSNotFound(err) {
			return pending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return objectBackupObservation(api, state, operationID, manifest), nil
	case sdk.ResilienceRestore:
		if state.Destination == sdk.RecoverySameRegion {
			targetPrefix, verifyErr := engine.VerifyRestoreInPlace(ctx, scope, manifest)
			if errors.Is(verifyErr, cloudrecovery.ErrObjectNotReady) {
				return pending(state, operationID), nil
			}
			if verifyErr != nil {
				return provider.NativeOperationObservation{}, verifyErr
			}
			return objectRestoreObservation(api, state, operationID, manifest, scope.SourceBucket, targetPrefix), nil
		}
		targetPrefix, verifyErr := engine.VerifyRestore(ctx, scope, manifest)
		if errors.Is(verifyErr, cloudrecovery.ErrObjectNotReady) {
			return pending(state, operationID), nil
		}
		if verifyErr != nil {
			return provider.NativeOperationObservation{}, verifyErr
		}
		return objectRestoreObservation(api, state, operationID, manifest, api.config.RestoreBucket, targetPrefix), nil
	case sdk.ResilienceIntegrityCheck:
		if err := engine.Integrity(ctx, scope, manifest); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return objectIntegrityObservation(api, state, operationID, manifest), nil
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Cloud Storage recovery does not implement this action")
	}
}

func (api *NativeAPI) restoreObjects(ctx context.Context, state cloudrecovery.OperationState, operationID string, engine *cloudrecovery.ObjectArchiveEngine, scope cloudrecovery.ObjectArchiveScope) (provider.NativeOperationObservation, error) {
	bucket, key, err := parseGCSReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	manifest, err := engine.ReadManifest(ctx, scope, bucket, key)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if state.Destination == sdk.RecoverySameRegion {
		if strings.TrimSpace(state.ApprovalReference) == "" {
			return provider.NativeOperationObservation{}, capabilityError(state, "GCP Cloud Storage in-place restore requires an operator approval reference because restore overwrites the source prefix")
		}
		targetPrefix, err := engine.RestoreInPlace(ctx, scope, manifest)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return objectRestoreObservation(api, state, operationID, manifest, scope.SourceBucket, targetPrefix), nil
	}
	targetPrefix, err := engine.Restore(ctx, scope, manifest)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return objectRestoreObservation(api, state, operationID, manifest, api.config.RestoreBucket, targetPrefix), nil
}

func (api *NativeAPI) integrityObjects(ctx context.Context, state cloudrecovery.OperationState, operationID string, engine *cloudrecovery.ObjectArchiveEngine, scope cloudrecovery.ObjectArchiveScope) (provider.NativeOperationObservation, error) {
	bucket, key, err := parseGCSReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	manifest, err := engine.ReadManifest(ctx, scope, bucket, key)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if err := engine.Integrity(ctx, scope, manifest); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return objectIntegrityObservation(api, state, operationID, manifest), nil
}

func gcpObjectScope(state cloudrecovery.OperationState) (cloudrecovery.ObjectArchiveScope, error) {
	bucket, prefix, err := parseGCSReference(state.Resource)
	if err != nil {
		return cloudrecovery.ObjectArchiveScope{}, err
	}
	return cloudrecovery.ObjectArchiveScope{DataClass: state.DataClass, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker, IdempotencyKey: state.IdempotencyKey, SourceBucket: bucket, SourcePrefix: prefix}, nil
}

func objectBackupObservation(api *NativeAPI, state cloudrecovery.OperationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest) provider.NativeOperationObservation {
	backupID := "gcp-storage://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json"
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: fmt.Sprintf("archived %d objects into an ownership-scoped sealed manifest", len(manifest.Entries))}
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), []string{backupID}, []string{"gcp.storage.manifest", "gcp.storage.encryption", "gcp.storage.ownership"}, []sdk.ResilienceProofEvidence{evidence})
}

func objectRestoreObservation(api *NativeAPI, state cloudrecovery.OperationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest, bucket, targetPrefix string) provider.NativeOperationObservation {
	restoreID := "gcp-storage://" + bucket + "/" + targetPrefix
	reason := fmt.Sprintf("restored %d ownership-scoped objects into %s", len(manifest.Entries), targetPrefix)
	if state.Destination == sdk.RecoverySameRegion {
		reason = fmt.Sprintf("restored %d ownership-scoped objects in place onto %s", len(manifest.Entries), targetPrefix)
	}
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: "gcp-storage://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json", RestoreID: restoreID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: reason}
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), []string{restoreID}, []string{"gcp.storage.restore-manifest", "gcp.storage.restore-ownership"}, []sdk.ResilienceProofEvidence{evidence})
}

func objectIntegrityObservation(api *NativeAPI, state cloudrecovery.OperationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest) provider.NativeOperationObservation {
	backupID := "gcp-storage://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json"
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: fmt.Sprintf("verified %d archived object sizes, validators, ownership metadata, encryption, and retention boundary", len(manifest.Entries))}
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), []string{"gcp-storage://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix}, []string{"gcp.storage.integrity-manifest", "gcp.storage.integrity-ownership"}, []sdk.ResilienceProofEvidence{evidence})
}

func (api *NativeAPI) verifyArchiveObject(ctx context.Context, key string, state cloudrecovery.OperationState) error {
	metadata, err := api.storage.Head(ctx, api.config.ArchiveBucket, key)
	if err != nil {
		return err
	}
	if !metadataMatches(metadata.Metadata, state) || !api.encrypted(metadata) || !api.protected(metadata) {
		return errors.New("GCP Cloud Storage archive object failed ownership, encryption, or retention verification")
	}
	return nil
}

func (api *NativeAPI) protected(metadata GCSObjectMetadata) bool {
	return !api.config.RequireLockedRetention || strings.EqualFold(metadata.RetentionMode, "locked")
}

func parseGCSReference(reference string) (bucket, key string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || (parsed.Scheme != "gs" && parsed.Scheme != "gcs" && parsed.Scheme != "gcp-storage") || strings.TrimSpace(parsed.Host) == "" {
		return "", "", fmt.Errorf("GCP Cloud Storage resource reference %q must use gs://bucket/prefix", reference)
	}
	key, err = url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || strings.TrimSpace(key) == "" {
		return "", "", fmt.Errorf("GCP Cloud Storage resource reference %q must include a non-empty object prefix", reference)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(key, "\r\n\x00") {
		return "", "", fmt.Errorf("GCP Cloud Storage resource reference %q must not contain query, fragment, or control data", reference)
	}
	return parsed.Host, key, nil
}

func isGCSNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "notexist") || strings.Contains(message, "not found") || strings.Contains(message, "404")
}
