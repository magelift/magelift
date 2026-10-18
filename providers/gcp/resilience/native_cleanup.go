package resilience

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

// cleanupObjectsForState removes only the archive and isolated-restore
// objects owned by the requested marker. Source objects remain outside this
// provider operation; the certification cell owns their exact fixture keys.
func (api *NativeAPI) cleanupObjectsForState(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.OwnershipMarker) == "" || strings.ContainsAny(state.OwnershipMarker, "\r\n\x00") {
		return provider.NativeOperationObservation{}, errors.New("GCP Cloud Storage cleanup ownership marker is required and must be single-line")
	}
	if api == nil || api.storage == nil {
		return provider.NativeOperationObservation{}, errors.New("GCP Cloud Storage recovery API is required for cleanup")
	}
	refs := make([]string, 0)
	archivePrefix := api.archivePrefixForMarker(state.OwnershipMarker)
	if err := api.cleanupGCSObjectPrefix(ctx, api.config.ArchiveBucket, archivePrefix, state.OwnershipMarker, true, &refs); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	restorePrefix := strings.Trim(strings.TrimSpace(api.config.RestorePrefix), "/") + "/"
	if err := api.cleanupGCSObjectPrefix(ctx, api.config.RestoreBucket, restorePrefix, state.OwnershipMarker, false, &refs); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	sort.Strings(refs)
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), refs, []string{"gcp.storage.ownership-cleanup"}, nil), nil
}

func (api *NativeAPI) cleanupGCSObjectPrefix(ctx context.Context, bucket, prefix, marker string, strictOwnership bool, refs *[]string) error {
	objects, err := api.storage.List(ctx, bucket, prefix)
	if err != nil {
		return fmt.Errorf("list GCP Cloud Storage recovery objects for cleanup: %w", err)
	}
	for _, object := range objects {
		if strings.TrimSpace(object.Key) == "" {
			continue
		}
		identity := "gcp-storage://" + bucket + "/" + object.Key
		metadata, err := api.storage.Head(ctx, bucket, object.Key)
		if err != nil {
			if isGCSNotFound(err) {
				continue
			}
			return fmt.Errorf("inspect GCP Cloud Storage recovery object %q for cleanup: %w", identity, err)
		}
		owned := metadata.Metadata[ownershipMetadataKey] == marker
		if !owned {
			if strictOwnership {
				return fmt.Errorf("refusing to clean GCP Cloud Storage object %q because ownership metadata drifted", identity)
			}
			continue
		}
		if strings.TrimSpace(metadata.Metadata[classMetadataKey]) == "" || strings.TrimSpace(metadata.Metadata[fixtureMetadataKey]) == "" {
			return fmt.Errorf("refusing to clean GCP Cloud Storage object %q because ownership metadata is incomplete", identity)
		}
		if !api.encrypted(metadata) {
			return fmt.Errorf("refusing to clean GCP Cloud Storage object %q because encryption metadata drifted", identity)
		}
		if api.config.RequireLockedRetention && strings.EqualFold(metadata.RetentionMode, "locked") {
			return fmt.Errorf("GCP Cloud Storage recovery object %q remains protected by locked retention", identity)
		}
		if err := api.storage.Delete(ctx, bucket, object.Key); err != nil && !isGCSNotFound(err) {
			return fmt.Errorf("delete owned GCP Cloud Storage object %q: %w", identity, err)
		}
		*refs = append(*refs, identity)
	}

	remaining, err := api.storage.List(ctx, bucket, prefix)
	if err != nil {
		return fmt.Errorf("verify GCP Cloud Storage recovery cleanup: %w", err)
	}
	for _, object := range remaining {
		if strings.TrimSpace(object.Key) == "" {
			continue
		}
		identity := "gcp-storage://" + bucket + "/" + object.Key
		metadata, err := api.storage.Head(ctx, bucket, object.Key)
		if err != nil {
			if isGCSNotFound(err) {
				continue
			}
			return fmt.Errorf("inspect remaining GCP Cloud Storage object %q: %w", identity, err)
		}
		if metadata.Metadata[ownershipMetadataKey] != marker {
			if strictOwnership {
				return fmt.Errorf("owned GCP Cloud Storage object %q remains with ownership drift", identity)
			}
			continue
		}
		return fmt.Errorf("owned GCP Cloud Storage object %q remains after cleanup", identity)
	}
	return nil
}
