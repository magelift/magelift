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
	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// OVHCleanupPendingError means Object Lock still protects an owned recovery
// object. Callers must retain the marker and retry after the configured
// retention boundary instead of treating a protected object as deleted.
type OVHCleanupPendingError struct {
	Resources []string
}

func (err *OVHCleanupPendingError) Error() string {
	if err == nil || len(err.Resources) == 0 {
		return "OVHcloud recovery cleanup is pending provider retention"
	}
	return fmt.Sprintf("OVHcloud recovery cleanup is pending provider retention for %s", strings.Join(err.Resources, ", "))
}

// cleanupObjectsForState removes only marker-owned archive and isolated-restore
// objects. Source objects remain outside this provider operation; the
// certification cell owns their exact fixture keys. This is provider-local so
// S3-compatible retention and deletion behavior does not leak into the core.
func (api *NativeAPI) cleanupObjectsForState(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.OwnershipMarker) == "" || strings.ContainsAny(state.OwnershipMarker, "\r\n\x00") {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud Object Storage cleanup ownership marker is required and must be single-line")
	}
	if api == nil || api.objects == nil {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud Object Storage recovery API is required for cleanup")
	}
	store, err := api.objectStore()
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	refs, pending, err := api.cleanupOwnedObjectOutputs(ctx, store, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if pending != nil {
		return provider.NativeOperationObservation{}, pending
	}
	sort.Strings(refs)
	return ovhObservation(state, operationID, refs, []string{"ovh.object-storage.ownership-cleanup"}, nil, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) cleanupOwnedObjectOutputs(ctx context.Context, store cloudrecovery.ObjectStore, marker string) ([]string, *OVHCleanupPendingError, error) {
	refs := make([]string, 0)
	var pending *OVHCleanupPendingError
	archivePrefix := api.archivePrefixForMarker(marker)
	if err := api.cleanupOVHObjectPrefix(ctx, store, api.config.ArchiveBucket, archivePrefix, marker, true, &refs); err != nil {
		var retention *OVHCleanupPendingError
		if !errors.As(err, &retention) {
			return refs, nil, err
		}
		pending = mergeOVHCleanupPending(pending, retention)
	}
	restorePrefix := strings.Trim(strings.TrimSpace(api.config.RestorePrefix), "/") + "/"
	if err := api.cleanupOVHObjectPrefix(ctx, store, api.config.RestoreBucket, restorePrefix, marker, false, &refs); err != nil {
		var retention *OVHCleanupPendingError
		if !errors.As(err, &retention) {
			return refs, nil, err
		}
		pending = mergeOVHCleanupPending(pending, retention)
	}
	sort.Strings(refs)
	return refs, pending, nil
}

func mergeOVHCleanupPending(current, next *OVHCleanupPendingError) *OVHCleanupPendingError {
	if next == nil {
		return current
	}
	if current == nil {
		current = &OVHCleanupPendingError{}
	}
	current.Resources = append(current.Resources, next.Resources...)
	sort.Strings(current.Resources)
	return current
}

func (api *NativeAPI) cleanupSecretForState(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.secrets == nil {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud Secret Manager recovery API is required for cleanup")
	}
	if strings.TrimSpace(state.OwnershipMarker) == "" || strings.ContainsAny(state.OwnershipMarker, "\r\n\x00") {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud Secret Manager cleanup ownership marker is required and must be single-line")
	}
	store, err := api.objectStore()
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	objectRefs, pending, err := api.cleanupOwnedObjectOutputs(ctx, store, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	secretRefs, err := api.cleanupOwnedSecretOutputs(ctx, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if pending != nil {
		return provider.NativeOperationObservation{}, pending
	}
	refs := append(objectRefs, secretRefs...)
	sort.Strings(refs)
	return ovhObservation(state, operationID, refs, []string{"ovh.secret-manager.ownership-cleanup", "ovh.object-storage.ownership-cleanup"}, nil, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) cleanupDatabaseForState(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.database == nil {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud Public Cloud Database recovery API is required for cleanup")
	}
	if strings.TrimSpace(state.OwnershipMarker) == "" || strings.ContainsAny(state.OwnershipMarker, "\r\n\x00") {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud Public Cloud Database cleanup ownership marker is required and must be single-line")
	}
	if strings.TrimSpace(api.config.DatabaseSourceID) == "" {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud Public Cloud Database cleanup requires the configured source service identity")
	}
	refs, err := api.cleanupOwnedDatabaseOutputs(ctx, state.OwnershipMarker)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return ovhObservation(state, operationID, refs, []string{"ovh.database.ownership-cleanup"}, nil, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) cleanupOwnedDatabaseOutputs(ctx context.Context, marker string) ([]string, error) {
	engine := api.normalizeDatabaseReference(ovhDatabaseReference{Engine: api.config.DatabaseEngine}).Engine
	instances, err := api.database.ListInstances(ctx, engine)
	if err != nil {
		return nil, fmt.Errorf("list OVHcloud Public Cloud Database restore services for cleanup: %w", err)
	}
	refs := make([]string, 0)
	seenDescriptions := make(map[string]string)
	for _, instance := range instances {
		if instance.ID == api.config.DatabaseSourceID || !isOVHDatabaseRestoreDescriptionOwned(instance.Description, marker) {
			continue
		}
		if strings.TrimSpace(instance.ID) == "" {
			return refs, errors.New("refusing to clean an OVHcloud database restore service without an identity")
		}
		if previousID, duplicate := seenDescriptions[instance.Description]; duplicate {
			return refs, fmt.Errorf("refusing to clean OVHcloud database restore description %q because services %q and %q collide", instance.Description, previousID, instance.ID)
		}
		seenDescriptions[instance.Description] = instance.ID
	}
	for _, instance := range instances {
		if instance.ID == api.config.DatabaseSourceID || !isOVHDatabaseRestoreDescriptionOwned(instance.Description, marker) {
			continue
		}
		if err := api.database.DeleteInstance(ctx, engine, instance.ID); err != nil && !isOVHNotFound(err) {
			return refs, fmt.Errorf("delete owned OVHcloud Public Cloud Database restore service %q: %w", instance.ID, err)
		}
		if err := waitForOVHDatabaseGone(ctx, api.database, engine, instance.ID, api.config.DatabaseDeletePollInterval); err != nil {
			return refs, fmt.Errorf("verify deletion of OVHcloud Public Cloud Database restore service %q: %w", instance.ID, err)
		}
		refs = append(refs, "ovh-database://"+engine+"/"+instance.ID)
	}
	remaining, err := api.database.ListInstances(ctx, engine)
	if err != nil {
		return refs, fmt.Errorf("verify OVHcloud Public Cloud Database cleanup: %w", err)
	}
	for _, instance := range remaining {
		if instance.ID != api.config.DatabaseSourceID && isOVHDatabaseRestoreDescriptionOwned(instance.Description, marker) {
			return refs, fmt.Errorf("owned OVHcloud Public Cloud Database restore service %q remains after cleanup", instance.ID)
		}
	}
	sort.Strings(refs)
	return refs, nil
}

func waitForOVHDatabaseGone(ctx context.Context, api DatabaseAPI, engine, id string, pollInterval time.Duration) error {
	for {
		_, err := api.GetInstance(ctx, engine, id)
		if err != nil {
			if isOVHNotFound(err) {
				return nil
			}
			return err
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (api *NativeAPI) cleanupOwnedSecretOutputs(ctx context.Context, marker string) ([]string, error) {
	secrets, err := api.secrets.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list OVHcloud Secret Manager restore secrets for cleanup: %w", err)
	}
	prefix := strings.Trim(strings.TrimSpace(api.config.RestoreSecretPrefix), "/")
	if prefix == "" {
		prefix = "magelift-recovery"
	}
	prefix += "/"
	refs := make([]string, 0)
	for _, secret := range secrets {
		if !strings.HasPrefix(secret.Path, prefix) {
			continue
		}
		ownership := secret.CustomMetadata[ovhSecretOwnershipKey]
		if ownership != marker {
			continue
		}
		if secret.CustomMetadata[ovhSecretClassKey] != "configuration-secrets" {
			return refs, fmt.Errorf("refusing to clean OVHcloud Secret Manager restore secret %q because ownership metadata drifted", secret.Path)
		}
		if err := api.secrets.Delete(ctx, secret.Path); err != nil && !isOVHNotFound(err) {
			return refs, fmt.Errorf("delete owned OVHcloud Secret Manager restore secret %q: %w", secret.Path, err)
		}
		if err := waitForOVHSecretGone(ctx, api.secrets, secret.Path); err != nil {
			return refs, fmt.Errorf("verify OVHcloud Secret Manager restore secret %q cleanup: %w", secret.Path, err)
		}
		refs = append(refs, "ovh-secret://"+api.config.SecretOKMSID+"/"+secret.Path)
	}
	remaining, err := api.secrets.List(ctx)
	if err != nil {
		return refs, fmt.Errorf("verify OVHcloud Secret Manager restore cleanup: %w", err)
	}
	for _, secret := range remaining {
		if strings.HasPrefix(secret.Path, prefix) && secret.CustomMetadata[ovhSecretOwnershipKey] == marker && secret.CustomMetadata[ovhSecretClassKey] == "configuration-secrets" {
			return refs, fmt.Errorf("owned OVHcloud Secret Manager restore secret %q remains after cleanup", secret.Path)
		}
	}
	sort.Strings(refs)
	return refs, nil
}

func waitForOVHSecretGone(ctx context.Context, api SecretAPI, path string) error {
	for {
		secrets, err := api.List(ctx)
		if err != nil {
			return err
		}
		found := false
		for _, secret := range secrets {
			if secret.Path == path {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (api *NativeAPI) cleanupOVHObjectPrefix(ctx context.Context, store cloudrecovery.ObjectStore, bucket, prefix, marker string, strictOwnership bool, refs *[]string) error {
	objects, err := store.List(ctx, bucket, prefix)
	if err != nil {
		return fmt.Errorf("list OVHcloud recovery objects for cleanup: %w", err)
	}
	for _, object := range objects {
		identity := "ovh-object://" + bucket + "/" + object.Key
		metadata, err := store.Head(ctx, bucket, object.Key)
		if err != nil {
			if isOVHNotFound(err) {
				continue
			}
			return fmt.Errorf("inspect OVHcloud recovery object %q for cleanup: %w", identity, err)
		}
		owned := metadata.Metadata[cloudrecovery.OwnershipMetadataKey] == marker
		if !owned {
			if strictOwnership {
				return fmt.Errorf("refusing to clean OVHcloud recovery object %q because ownership metadata drifted", identity)
			}
			continue
		}
		if strings.TrimSpace(metadata.Metadata[cloudrecovery.ClassMetadataKey]) == "" || strings.TrimSpace(metadata.Metadata[cloudrecovery.FixtureMetadataKey]) == "" {
			return fmt.Errorf("refusing to clean OVHcloud recovery object %q because ownership metadata is incomplete", identity)
		}
		if !api.encryptionVerified(metadata) {
			return fmt.Errorf("refusing to clean OVHcloud recovery object %q because encryption metadata drifted", identity)
		}
		if api.config.RequireObjectLock && metadata.Protected {
			return &OVHCleanupPendingError{Resources: []string{identity}}
		}
		if _, err := api.objects.DeleteObject(ctx, deleteOVHObjectInput(bucket, object.Key)); err != nil && !isOVHNotFound(err) {
			return fmt.Errorf("delete owned OVHcloud recovery object %q: %w", identity, err)
		}
		*refs = append(*refs, identity)
	}

	remaining, err := store.List(ctx, bucket, prefix)
	if err != nil {
		return fmt.Errorf("verify OVHcloud recovery cleanup: %w", err)
	}
	for _, object := range remaining {
		identity := "ovh-object://" + bucket + "/" + object.Key
		metadata, err := store.Head(ctx, bucket, object.Key)
		if err != nil {
			if isOVHNotFound(err) {
				continue
			}
			return fmt.Errorf("inspect remaining OVHcloud recovery object %q: %w", identity, err)
		}
		if metadata.Metadata[cloudrecovery.OwnershipMetadataKey] != marker {
			if strictOwnership {
				return fmt.Errorf("owned OVHcloud recovery object %q remains with ownership drift", identity)
			}
			continue
		}
		return fmt.Errorf("owned OVHcloud recovery object %q remains after cleanup", identity)
	}
	return nil
}

func deleteOVHObjectInput(bucket, key string) *s3.DeleteObjectInput {
	return &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}
}
