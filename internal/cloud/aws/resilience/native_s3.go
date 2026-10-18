package resilience

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

type s3ObjectStore struct {
	api    S3API
	config NativeAPIConfig
}

func (s s3ObjectStore) List(ctx context.Context, bucket, prefix string) ([]cloudrecovery.ObjectInfo, error) {
	objects := make([]cloudrecovery.ObjectInfo, 0)
	var token *string
	for {
		output, err := s.api.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix), ContinuationToken: token})
		if err != nil {
			return nil, err
		}
		if output == nil {
			return nil, errors.New("AWS S3 object listing returned an empty response")
		}
		for _, object := range output.Contents {
			objects = append(objects, cloudrecovery.ObjectInfo{Key: aws.ToString(object.Key), Size: aws.ToInt64(object.Size), Validator: aws.ToString(object.ETag)})
		}
		if !aws.ToBool(output.IsTruncated) {
			return objects, nil
		}
		if aws.ToString(output.NextContinuationToken) == "" {
			return nil, errors.New("AWS S3 object listing was truncated without a continuation token")
		}
		token = output.NextContinuationToken
	}
}

func (s s3ObjectStore) Head(ctx context.Context, bucket, key string) (cloudrecovery.ObjectMetadata, error) {
	output, err := s.api.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return cloudrecovery.ObjectMetadata{}, err
	}
	if output == nil {
		return cloudrecovery.ObjectMetadata{}, errors.New("AWS S3 object inspection returned an empty response")
	}
	return cloudrecovery.ObjectMetadata{Size: aws.ToInt64(output.ContentLength), Validator: aws.ToString(output.ETag), Metadata: cloneAWSMetadata(output.Metadata), Encryption: string(output.ServerSideEncryption), EncryptionKey: aws.ToString(output.SSEKMSKeyId), Protected: s.protected(output)}, nil
}

func (s s3ObjectStore) Read(ctx context.Context, bucket, key string) ([]byte, error) {
	return readObject(ctx, s.api, bucket, key)
}

func (s s3ObjectStore) Put(ctx context.Context, bucket, key string, body []byte, metadata map[string]string) error {
	input := &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body), Metadata: cloneAWSMetadata(metadata)}
	applyS3Encryption(input, s.config)
	applyS3ObjectLock(input, s.config)
	_, err := s.api.PutObject(ctx, input)
	return err
}

func (s s3ObjectStore) Copy(ctx context.Context, sourceBucket, sourceKey, targetBucket, targetKey string, metadata map[string]string) error {
	input := &s3.CopyObjectInput{Bucket: aws.String(targetBucket), Key: aws.String(targetKey), CopySource: aws.String(url.PathEscape(sourceBucket + "/" + sourceKey)), MetadataDirective: s3types.MetadataDirectiveReplace, Metadata: cloneAWSMetadata(metadata)}
	applyS3CopyEncryption(input, s.config)
	applyS3CopyObjectLock(input, s.config)
	_, err := s.api.CopyObject(ctx, input)
	return err
}

func cloneAWSMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	clone := make(map[string]string, len(metadata))
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

// Delete removes one object selected by a provider-local cleanup path. The
// core archive engine never deletes source data; cleanup remains an explicit
// normalized operation with exact ownership checks in native_s3_cleanup.go.
func (s s3ObjectStore) Delete(ctx context.Context, bucket, key string) error {
	_, err := s.api.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	return err
}

func applyS3ObjectLock(input *s3.PutObjectInput, config NativeAPIConfig) {
	if !config.RequireObjectLock {
		return
	}
	input.ObjectLockMode = s3types.ObjectLockModeCompliance
	retainUntil := time.Now().Add(time.Duration(config.RetentionDays) * 24 * time.Hour)
	if config.Now != nil {
		retainUntil = config.Now().Add(time.Duration(config.RetentionDays) * 24 * time.Hour)
	}
	input.ObjectLockRetainUntilDate = &retainUntil
}

func applyS3CopyObjectLock(input *s3.CopyObjectInput, config NativeAPIConfig) {
	if !config.RequireObjectLock {
		return
	}
	input.ObjectLockMode = s3types.ObjectLockModeCompliance
	retainUntil := time.Now().Add(time.Duration(config.RetentionDays) * 24 * time.Hour)
	if config.Now != nil {
		retainUntil = config.Now().Add(time.Duration(config.RetentionDays) * 24 * time.Hour)
	}
	input.ObjectLockRetainUntilDate = &retainUntil
}

func (s s3ObjectStore) protected(output *s3.HeadObjectOutput) bool {
	if !s.config.RequireObjectLock {
		return true
	}
	return output.ObjectLockMode == s3types.ObjectLockModeCompliance && output.ObjectLockRetainUntilDate != nil && !output.ObjectLockRetainUntilDate.Before(s.now())
}

func (s s3ObjectStore) now() time.Time {
	if s.config.Now != nil {
		return s.config.Now()
	}
	return time.Now()
}

func (api *NativeAPI) objectEngine() (*cloudrecovery.ObjectArchiveEngine, error) {
	if api == nil || api.s3 == nil {
		return nil, errors.New("AWS S3 recovery API is required")
	}
	return cloudrecovery.NewObjectArchiveEngine(s3ObjectStore{api: api.s3, config: api.config}, cloudrecovery.ObjectArchiveConfig{
		ArchiveBucket: api.config.ArchiveBucket, ArchivePrefix: api.config.ArchivePrefix, RestoreBucket: api.restoreBucket(), RestorePrefix: api.config.RestorePrefix,
		IsNotFound: isS3NotFound,
		EncryptionVerified: func(metadata cloudrecovery.ObjectMetadata) bool {
			if api.config.RequireKMS {
				return metadata.Encryption == string(s3types.ServerSideEncryptionAwsKms) && metadata.EncryptionKey == api.config.KMSKeyID
			}
			return metadata.Encryption != ""
		},
		ProtectionVerified: func(metadata cloudrecovery.ObjectMetadata) bool {
			return !api.config.RequireObjectLock || metadata.Protected
		},
	})
}

func (api *NativeAPI) startObjectClass(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupObjectClass(ctx, state, operationID)
	}
	if _, _, err := parseS3Reference(state.Resource); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	engine, err := api.objectEngine()
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	scope, err := awsObjectScope(state)
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
		return provider.NativeOperationObservation{}, capabilityError(state, "AWS S3 recovery does not implement this action")
	}
}

func (api *NativeAPI) pollObjectClass(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupObjectClass(ctx, state, operationID)
	}
	engine, err := api.objectEngine()
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	scope, err := awsObjectScope(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	manifestBucket, manifestKey, err := parseS3Reference(state.Backup)
	if state.Action == sdk.ResilienceBackup {
		manifestBucket = api.config.ArchiveBucket
		manifestKey = api.objectPrefixForOperation(state) + "/manifest.json"
	}
	if err != nil && state.Action != sdk.ResilienceBackup {
		return provider.NativeOperationObservation{}, err
	}
	manifest, err := engine.ReadManifest(ctx, scope, manifestBucket, manifestKey)
	if err != nil {
		if isS3NotFound(err) {
			return pendingOperation(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return objectBackupObservation(api, state, operationID, manifest), nil
	case sdk.ResilienceRestore:
		targetPrefix, verifyErr := engine.VerifyRestore(ctx, scope, manifest)
		if errors.Is(verifyErr, cloudrecovery.ErrObjectNotReady) {
			return pendingOperation(state, operationID), nil
		}
		if verifyErr != nil {
			return provider.NativeOperationObservation{}, verifyErr
		}
		return objectRestoreObservation(api, state, operationID, manifest, targetPrefix), nil
	case sdk.ResilienceIntegrityCheck:
		if err := engine.Integrity(ctx, scope, manifest); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return objectIntegrityObservation(api, state, operationID, manifest), nil
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "AWS S3 recovery does not implement this action")
	}
}

func (api *NativeAPI) restoreObjects(ctx context.Context, state operationState, operationID string, engine *cloudrecovery.ObjectArchiveEngine, scope cloudrecovery.ObjectArchiveScope) (provider.NativeOperationObservation, error) {
	bucket, key, err := parseS3Reference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	manifest, err := engine.ReadManifest(ctx, scope, bucket, key)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	targetPrefix, err := engine.Restore(ctx, scope, manifest)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return objectRestoreObservation(api, state, operationID, manifest, targetPrefix), nil
}

func (api *NativeAPI) integrityObjects(ctx context.Context, state operationState, operationID string, engine *cloudrecovery.ObjectArchiveEngine, scope cloudrecovery.ObjectArchiveScope) (provider.NativeOperationObservation, error) {
	bucket, key, err := parseS3Reference(state.Backup)
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

func awsObjectScope(state operationState) (cloudrecovery.ObjectArchiveScope, error) {
	bucket, prefix, err := parseS3Reference(state.Resource)
	if err != nil {
		return cloudrecovery.ObjectArchiveScope{}, err
	}
	return cloudrecovery.ObjectArchiveScope{DataClass: state.DataClass, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker, IdempotencyKey: state.IdempotencyKey, SourceBucket: bucket, SourcePrefix: prefix}, nil
}

func objectBackupObservation(api *NativeAPI, state operationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest) provider.NativeOperationObservation {
	backupID := "aws-s3://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json"
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: fmt.Sprintf("archived %d objects into an ownership-scoped immutable manifest", len(manifest.Entries))}
	return observationWithOperation(state, operationID, []string{backupID}, []string{"aws.s3.manifest", "aws.s3.encryption", "aws.s3.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func objectRestoreObservation(api *NativeAPI, state operationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest, targetPrefix string) provider.NativeOperationObservation {
	restoreID := "aws-s3://" + api.restoreBucket() + "/" + targetPrefix
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: "aws-s3://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json", RestoreID: restoreID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: fmt.Sprintf("restored %d ownership-scoped objects into %s", len(manifest.Entries), targetPrefix)}
	return observationWithOperation(state, operationID, []string{restoreID}, []string{"aws.s3.restore-manifest", "aws.s3.restore-ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func objectIntegrityObservation(api *NativeAPI, state operationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest) provider.NativeOperationObservation {
	backupID := "aws-s3://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json"
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: fmt.Sprintf("verified %d archived object identities, sizes, ownership metadata, encryption boundary, and protection policy", len(manifest.Entries))}
	return observationWithOperation(state, operationID, []string{"aws-s3://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix}, []string{"aws.s3.integrity-manifest", "aws.s3.integrity-ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}
