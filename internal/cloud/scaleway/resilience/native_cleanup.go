package resilience

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

// ScalewayCleanupPendingError means that cleanup reached a provider retention
// boundary. It is deliberately distinct from a successful cleanup: callers
// must retain the marker and retry after the provider allows deletion.
type ScalewayCleanupPendingError struct {
	Resources []string
}

func (err *ScalewayCleanupPendingError) Error() string {
	if err == nil || len(err.Resources) == 0 {
		return "Scaleway recovery cleanup is pending provider retention"
	}
	return fmt.Sprintf("Scaleway recovery cleanup is pending provider retention for %s", strings.Join(err.Resources, ", "))
}

// CleanupOwned removes only deterministic recovery outputs for marker. Source
// instances and source secrets are not selected by ownership tags alone: they
// must also carry the recovery-output tag and a provider-generated output name.
// Object Lock compliance objects remain pending until their retention expires.
func (api *NativeAPI) CleanupOwned(ctx context.Context, marker string) error {
	if api == nil {
		return errors.New("Scaleway recovery cleanup API is not configured")
	}
	if err := validateScalewayContext(ctx); err != nil {
		return err
	}
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return errors.New("Scaleway recovery cleanup ownership marker is required")
	}

	var cleanupErrors []error
	if _, pending, err := api.cleanupOwnedObjects(ctx, marker); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	} else if len(pending) != 0 {
		cleanupErrors = append(cleanupErrors, &ScalewayCleanupPendingError{Resources: pending})
	}
	if api.database != nil {
		if _, err := api.cleanupOwnedDatabase(ctx, marker); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	if api.secrets != nil {
		if _, err := api.cleanupOwnedSecrets(ctx, marker); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

func (api *NativeAPI) cleanupObjectsForState(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	refs, pending, err := api.cleanupOwnedObjects(ctx, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if len(pending) != 0 {
		return provider.NativeOperationObservation{}, &ScalewayCleanupPendingError{Resources: pending}
	}
	return scalewayObservation(state, operationID, refs, []string{"scaleway.object-storage.ownership-cleanup"}, nil, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) cleanupDatabaseForState(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	refs, err := api.cleanupOwnedDatabase(ctx, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return scalewayObservation(state, operationID, refs, []string{"scaleway.rdb.ownership-cleanup"}, nil, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) cleanupSecretForState(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	objectRefs, pending, err := api.cleanupOwnedObjects(ctx, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if len(pending) != 0 {
		return provider.NativeOperationObservation{}, &ScalewayCleanupPendingError{Resources: pending}
	}
	secretRefs, err := api.cleanupOwnedSecrets(ctx, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	refs := append(objectRefs, secretRefs...)
	sort.Strings(refs)
	return scalewayObservation(state, operationID, refs, []string{"scaleway.secret-manager.ownership-cleanup"}, nil, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) cleanupOwnedObjects(ctx context.Context, marker string) (refs, pending []string, err error) {
	store, err := api.objectStore()
	if err != nil {
		return nil, nil, err
	}
	seen := make(map[string]struct{})
	archivePrefix := api.archivePrefixForMarker(marker)
	if err := api.cleanupObjectPrefix(ctx, store, api.config.ArchiveBucket, archivePrefix, marker, true, seen, &refs, &pending); err != nil {
		return refs, pending, err
	}
	restorePrefix := strings.Trim(strings.TrimSpace(api.config.RestorePrefix), "/") + "/"
	if err := api.cleanupObjectPrefix(ctx, store, api.config.RestoreBucket, restorePrefix, marker, false, seen, &refs, &pending); err != nil {
		return refs, pending, err
	}
	sort.Strings(refs)
	sort.Strings(pending)
	return refs, pending, nil
}

func (api *NativeAPI) cleanupObjectPrefix(ctx context.Context, store cloudrecovery.ObjectStore, bucket, prefix, marker string, strictOwnership bool, seen map[string]struct{}, refs, pending *[]string) error {
	objects, err := store.List(ctx, bucket, prefix)
	if err != nil {
		return fmt.Errorf("list Scaleway recovery objects for cleanup: %w", err)
	}
	for _, object := range objects {
		identity := "scaleway-object://" + bucket + "/" + object.Key
		if _, ok := seen[identity]; ok {
			continue
		}
		metadata, err := store.Head(ctx, bucket, object.Key)
		if err != nil {
			if isScalewayNotFound(err) {
				continue
			}
			return fmt.Errorf("inspect Scaleway recovery object %q for cleanup: %w", identity, err)
		}
		owned := metadata.Metadata[cloudrecovery.OwnershipMetadataKey] == marker
		if !owned {
			if strictOwnership {
				return fmt.Errorf("refusing to clean Scaleway recovery object %q because ownership metadata drifted", identity)
			}
			continue
		}
		if metadata.Metadata[cloudrecovery.ClassMetadataKey] == "" || metadata.Metadata[cloudrecovery.FixtureMetadataKey] == "" {
			return fmt.Errorf("refusing to clean Scaleway recovery object %q because ownership metadata is incomplete", identity)
		}
		if !api.encryptionVerified(metadata) {
			return fmt.Errorf("refusing to clean Scaleway recovery object %q because encryption metadata drifted", identity)
		}
		seen[identity] = struct{}{}
		if api.config.RequireObjectLock && metadata.Protected {
			*pending = append(*pending, identity)
			continue
		}
		if _, err := api.objects.DeleteObject(ctx, deleteObjectInput(bucket, object.Key)); err != nil && !isScalewayNotFound(err) {
			return fmt.Errorf("delete owned Scaleway recovery object %q: %w", identity, err)
		}
		*refs = append(*refs, identity)
	}
	return api.verifyObjectPrefixCleanup(ctx, store, bucket, prefix, marker, strictOwnership, seen, pending)
}

func (api *NativeAPI) verifyObjectPrefixCleanup(ctx context.Context, store cloudrecovery.ObjectStore, bucket, prefix, marker string, strictOwnership bool, seen map[string]struct{}, pending *[]string) error {
	objects, err := store.List(ctx, bucket, prefix)
	if err != nil {
		return fmt.Errorf("verify Scaleway recovery object cleanup: %w", err)
	}
	for _, object := range objects {
		identity := "scaleway-object://" + bucket + "/" + object.Key
		metadata, headErr := store.Head(ctx, bucket, object.Key)
		if headErr != nil {
			if isScalewayNotFound(headErr) {
				continue
			}
			return fmt.Errorf("inspect remaining Scaleway recovery object %q: %w", identity, headErr)
		}
		if metadata.Metadata[cloudrecovery.OwnershipMetadataKey] != marker {
			if strictOwnership {
				return fmt.Errorf("owned Scaleway recovery object %q remains with ownership drift", identity)
			}
			continue
		}
		if api.config.RequireObjectLock && metadata.Protected {
			if _, already := seen[identity]; !already {
				*pending = append(*pending, identity)
			}
			continue
		}
		return fmt.Errorf("owned Scaleway recovery object %q remains after cleanup", identity)
	}
	return nil
}

func (api *NativeAPI) cleanupOwnedDatabase(ctx context.Context, marker string) ([]string, error) {
	instances, err := api.database.ListInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Scaleway Managed Database instances for cleanup: %w", err)
	}
	refs := make([]string, 0)
	for _, instance := range instances {
		if !strings.HasPrefix(instance.Name, scalewayDatabaseRestorePrefix) {
			continue
		}
		owned := scalewayDatabaseTagsMatch(instance.Tags, marker, "database")
		if !owned {
			continue
		}
		if !hasScalewayTag(instance.Tags, scalewayRecoveryOutputTag) {
			return refs, fmt.Errorf("refusing to clean Scaleway Managed Database instance %q because output ownership metadata is incomplete", instance.ID)
		}
		if err := api.database.DeleteInstance(ctx, instance.ID); err != nil && !isScalewayNotFound(err) {
			return refs, fmt.Errorf("delete owned Scaleway Managed Database instance %q: %w", instance.ID, err)
		}
		if err := waitForScalewayDatabaseInstanceGone(ctx, api.database, instance.ID); err != nil {
			return refs, fmt.Errorf("verify deletion of Scaleway Managed Database instance %q: %w", instance.ID, err)
		}
		identity := "scaleway-rdb://" + instance.ID
		refs = append(refs, identity)
	}

	snapshots, err := api.database.ListSnapshots(ctx)
	if err != nil {
		return refs, fmt.Errorf("list Scaleway Managed Database snapshots for cleanup: %w", err)
	}
	for _, snapshot := range snapshots {
		if !scalewayDatabaseSnapshotOwned(snapshot, marker) {
			continue
		}
		if err := api.database.DeleteSnapshot(ctx, snapshot.ID); err != nil && !isScalewayNotFound(err) {
			return refs, fmt.Errorf("delete owned Scaleway Managed Database snapshot %q: %w", snapshot.ID, err)
		}
		if err := waitForScalewayDatabaseSnapshotGone(ctx, api.database, snapshot.ID); err != nil {
			return refs, fmt.Errorf("verify deletion of Scaleway Managed Database snapshot %q: %w", snapshot.ID, err)
		}
		refs = append(refs, "scaleway-rdb-snapshot://"+snapshot.ID)
	}
	remainingInstances, err := api.database.ListInstances(ctx)
	if err != nil {
		return refs, fmt.Errorf("verify Scaleway Managed Database instance cleanup: %w", err)
	}
	for _, instance := range remainingInstances {
		if scalewayDatabaseRestoreOwned(instance, operationState{OwnershipMarker: marker, DataClass: "database"}) {
			return refs, fmt.Errorf("owned Scaleway Managed Database instance %q remains after cleanup", instance.ID)
		}
	}
	remainingSnapshots, err := api.database.ListSnapshots(ctx)
	if err != nil {
		return refs, fmt.Errorf("verify Scaleway Managed Database snapshot cleanup: %w", err)
	}
	for _, snapshot := range remainingSnapshots {
		if scalewayDatabaseSnapshotOwned(snapshot, marker) {
			return refs, fmt.Errorf("owned Scaleway Managed Database snapshot %q remains after cleanup", snapshot.ID)
		}
	}
	sort.Strings(refs)
	return refs, nil
}

func (api *NativeAPI) cleanupOwnedSecrets(ctx context.Context, marker string) ([]string, error) {
	secrets, err := api.secrets.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Scaleway Secret Manager secrets for cleanup: %w", err)
	}
	prefix := strings.Trim(strings.TrimSpace(api.config.RestoreSecretPrefix), "/")
	if prefix == "" {
		prefix = "magelift-recovery-secret"
	}
	prefix += "-"
	refs := make([]string, 0)
	for _, secret := range secrets {
		if !strings.HasPrefix(secret.Name, prefix) || !scalewaySecretTagsMatch(secret.Tags, marker, "configuration-secrets") {
			continue
		}
		if !hasScalewayTag(secret.Tags, scalewayRecoveryOutputTag) {
			return refs, fmt.Errorf("refusing to clean Scaleway Secret Manager secret %q because output ownership metadata is incomplete", secret.ID)
		}
		if secret.Protected {
			secret, err = api.secrets.Unprotect(ctx, secret.ID)
			if err != nil {
				return refs, fmt.Errorf("unprotect owned Scaleway Secret Manager secret %q for cleanup: %w", secret.ID, err)
			}
			if secret.Protected {
				return refs, fmt.Errorf("Scaleway Secret Manager secret %q remained protected after unprotect", secret.ID)
			}
		}
		if err := api.secrets.Delete(ctx, secret.ID); err != nil && !isScalewayNotFound(err) {
			return refs, fmt.Errorf("delete owned Scaleway Secret Manager secret %q: %w", secret.ID, err)
		}
		refs = append(refs, "scaleway-secret://"+secret.ID)
	}
	remaining, err := api.secrets.List(ctx)
	if err != nil {
		return refs, fmt.Errorf("verify Scaleway Secret Manager cleanup: %w", err)
	}
	for _, secret := range remaining {
		if strings.HasPrefix(secret.Name, prefix) && scalewaySecretRestoreOwned(secret, operationState{OwnershipMarker: marker, DataClass: "configuration-secrets"}) {
			return refs, fmt.Errorf("owned Scaleway Secret Manager secret %q remains after cleanup", secret.ID)
		}
	}
	sort.Strings(refs)
	return refs, nil
}

func waitForScalewayDatabaseInstanceGone(ctx context.Context, database DatabaseAPI, id string) error {
	for {
		_, err := database.GetInstance(ctx, id)
		if isScalewayNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := waitForScalewayCleanupPoll(ctx); err != nil {
			return err
		}
	}
}

func waitForScalewayDatabaseSnapshotGone(ctx context.Context, database DatabaseAPI, id string) error {
	for {
		_, err := database.GetSnapshot(ctx, id)
		if isScalewayNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := waitForScalewayCleanupPoll(ctx); err != nil {
			return err
		}
	}
}

func waitForScalewayCleanupPoll(ctx context.Context) error {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func deleteObjectInput(bucket, key string) *s3.DeleteObjectInput {
	return &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}
}
