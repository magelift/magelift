package resilience

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

// cleanupObjectClass removes only marker-owned archive and restore objects.
// Source objects remain outside this provider operation; the certification
// cell owns their exact fixture keys. This is deliberately provider-local
// because S3-compatible APIs differ in retention and deletion semantics.
func (api *NativeAPI) cleanupObjectClass(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.OwnershipMarker) == "" || strings.ContainsAny(state.OwnershipMarker, "\r\n\x00") {
		return provider.NativeOperationObservation{}, errors.New("AWS S3 cleanup ownership marker is required and must be single-line")
	}
	if api == nil || api.s3 == nil {
		return provider.NativeOperationObservation{}, errors.New("AWS S3 recovery API is required for cleanup")
	}
	store := s3ObjectStore{api: api.s3, config: api.config}
	refs := make([]string, 0)
	archivePrefix := api.archivePrefixForMarker(state.OwnershipMarker)
	if err := api.cleanupS3ObjectPrefix(ctx, store, api.archiveBucket(), archivePrefix, state.OwnershipMarker, true, &refs); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	restorePrefix := strings.Trim(strings.TrimSpace(api.config.RestorePrefix), "/") + "/"
	if err := api.cleanupS3ObjectPrefix(ctx, store, api.restoreBucket(), restorePrefix, state.OwnershipMarker, false, &refs); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	sort.Strings(refs)
	return observationWithOperation(state, operationID, refs, []string{"aws.s3.ownership-cleanup"}, nil, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) cleanupS3ObjectPrefix(ctx context.Context, store s3ObjectStore, bucket, prefix, marker string, strictOwnership bool, refs *[]string) error {
	objects, err := store.List(ctx, bucket, prefix)
	if err != nil {
		return fmt.Errorf("list AWS S3 recovery objects for cleanup: %w", err)
	}
	for _, object := range objects {
		identity := "aws-s3://" + bucket + "/" + object.Key
		metadata, err := store.Head(ctx, bucket, object.Key)
		if err != nil {
			if isS3NotFound(err) {
				continue
			}
			return fmt.Errorf("inspect AWS S3 recovery object %q for cleanup: %w", identity, err)
		}
		owned := metadata.Metadata[cloudrecovery.OwnershipMetadataKey] == marker
		if !owned {
			if strictOwnership {
				return fmt.Errorf("refusing to clean AWS S3 recovery object %q because ownership metadata drifted", identity)
			}
			continue
		}
		if strings.TrimSpace(metadata.Metadata[cloudrecovery.ClassMetadataKey]) == "" || strings.TrimSpace(metadata.Metadata[cloudrecovery.FixtureMetadataKey]) == "" {
			return fmt.Errorf("refusing to clean AWS S3 recovery object %q because ownership metadata is incomplete", identity)
		}
		if !api.archiveEncryptionVerifiedMetadata(metadata) {
			return fmt.Errorf("refusing to clean AWS S3 recovery object %q because encryption metadata drifted", identity)
		}
		if api.config.RequireObjectLock && metadata.Protected {
			return fmt.Errorf("AWS S3 recovery object %q remains protected by object lock", identity)
		}
		if err := store.Delete(ctx, bucket, object.Key); err != nil && !isS3NotFound(err) {
			return fmt.Errorf("delete owned AWS S3 recovery object %q: %w", identity, err)
		}
		*refs = append(*refs, identity)
	}

	remaining, err := store.List(ctx, bucket, prefix)
	if err != nil {
		return fmt.Errorf("verify AWS S3 recovery cleanup: %w", err)
	}
	for _, object := range remaining {
		identity := "aws-s3://" + bucket + "/" + object.Key
		metadata, headErr := store.Head(ctx, bucket, object.Key)
		if headErr != nil {
			if isS3NotFound(headErr) {
				continue
			}
			return fmt.Errorf("inspect remaining AWS S3 recovery object %q: %w", identity, headErr)
		}
		if metadata.Metadata[cloudrecovery.OwnershipMetadataKey] != marker {
			if strictOwnership {
				return fmt.Errorf("owned AWS S3 recovery object %q remains with ownership drift", identity)
			}
			continue
		}
		return fmt.Errorf("owned AWS S3 recovery object %q remains after cleanup", identity)
	}
	return nil
}

func (api *NativeAPI) archiveEncryptionVerifiedMetadata(metadata cloudrecovery.ObjectMetadata) bool {
	if api.config.RequireKMS {
		return metadata.Encryption == string(s3types.ServerSideEncryptionAwsKms) && metadata.EncryptionKey == api.config.KMSKeyID
	}
	return metadata.Encryption != ""
}
