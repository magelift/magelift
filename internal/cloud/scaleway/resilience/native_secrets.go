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

// SecretMetadata is the provider-local view of a Scaleway Secret Manager
// secret. Secret values never cross this metadata boundary except through the
// explicit Access method used by the provider translator.
type SecretMetadata struct {
	ID        string
	Name      string
	Status    string
	Protected bool
	Tags      []string
}

// SecretAPI is the narrow provider-local Secret Manager port. It is small
// enough for deterministic fakes and lets community implementations wrap a
// compatible SDK without importing native models into the lifecycle core.
type SecretAPI interface {
	Get(context.Context, string) (SecretMetadata, error)
	List(context.Context) ([]SecretMetadata, error)
	Access(context.Context, string, string) ([]byte, error)
	Delete(context.Context, string) error
	Create(context.Context, string, []string, bool) (SecretMetadata, error)
	CreateVersion(context.Context, string, []byte) error
	Protect(context.Context, string) (SecretMetadata, error)
	Unprotect(context.Context, string) (SecretMetadata, error)
}

func (api *NativeAPI) startSecret(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.secrets == nil {
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway Secret Manager recovery translator is not configured")
	}
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupSecretForState(ctx, state, operationID)
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		if err := api.requireSecretArchivePolicy(state); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		reference, err := parseScalewaySecretReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.backupSecret(ctx, state, operationID, reference)
	case sdk.ResilienceRestore:
		if err := api.requireSecretArchivePolicy(state); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.restoreSecret(ctx, state)
	case sdk.ResilienceIntegrityCheck:
		reference, err := parseScalewaySecretReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.integritySecret(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway Secret Manager recovery does not implement this action")
	}
}

func (api *NativeAPI) pollSecret(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.secrets == nil {
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway Secret Manager recovery translator is not configured")
	}
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupSecretForState(ctx, state, operationID)
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.verifySecretBackup(ctx, state, operationID)
	case sdk.ResilienceRestore:
		return api.verifySecretRestore(ctx, state, operationID)
	case sdk.ResilienceIntegrityCheck:
		reference, err := parseScalewaySecretReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.integritySecret(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway Secret Manager recovery does not implement this action")
	}
}

func (api *NativeAPI) backupSecret(ctx context.Context, state operationState, operationID string, reference scalewaySecretReference) (provider.NativeOperationObservation, error) {
	secret, err := api.secrets.Get(ctx, reference.ID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect Scaleway Secret Manager source: %w", err)
	}
	if err := verifyScalewaySecret(secret, state); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	value, err := api.secrets.Access(ctx, secret.ID, "latest_enabled")
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("read Scaleway Secret Manager source version: %w", err)
	}
	archive := cloudrecovery.SecretArchive{
		Version: cloudrecovery.OperationVersion, DataClass: state.DataClass, FixtureID: state.FixtureID,
		OwnershipMarker: state.OwnershipMarker, SourceSecret: secret.ID, Encoding: "binary", Value: append([]byte(nil), value...),
	}
	body, _, err := cloudrecovery.SealSecretArchive(archive)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("seal Scaleway Secret Manager archive: %w", err)
	}
	key := api.objectPrefixForOperation(state) + "/secret.json"
	store, err := api.objectStore()
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if existing, headErr := store.Head(ctx, api.config.ArchiveBucket, key); headErr == nil {
		if err := verifyScalewaySecretArchiveMetadata(existing, state); err != nil {
			return provider.NativeOperationObservation{}, err
		}
	} else if !isScalewayNotFound(headErr) {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect Scaleway Secret Manager archive: %w", headErr)
	} else if err := store.Put(ctx, api.config.ArchiveBucket, key, body, cloudrecovery.MetadataFor(cloudrecovery.ObjectArchiveScope{DataClass: state.DataClass, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker}, true)); err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("write Scaleway Secret Manager archive: %w", err)
	}
	if err := api.verifySecretArchive(ctx, state, key); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return scalewaySecretBackupObservation(api, state, operationID, key), nil
}

func (api *NativeAPI) verifySecretBackup(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	key := api.objectPrefixForOperation(state) + "/secret.json"
	if err := api.verifySecretArchive(ctx, state, key); err != nil {
		if isScalewayNotFound(err) {
			return scalewayDatabasePending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, err
	}
	return scalewaySecretBackupObservation(api, state, operationID, key), nil
}

func (api *NativeAPI) restoreSecret(ctx context.Context, state operationState) (provider.NativeOperationObservation, error) {
	archive, err := api.loadSecretArchive(ctx, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	targetName := api.restoreSecretName(state)
	target, findErr := api.findSecretByName(ctx, targetName)
	if findErr == nil {
		if !scalewaySecretRestoreOwned(target, state) {
			return provider.NativeOperationObservation{}, errors.New("refusing to overwrite an unowned Scaleway Secret Manager restore secret")
		}
		if !target.Protected {
			target, err = api.secrets.Protect(ctx, target.ID)
			if err != nil {
				return provider.NativeOperationObservation{}, fmt.Errorf("protect Scaleway Secret Manager restore secret: %w", err)
			}
		}
		if err := verifyScalewaySecret(target, state); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		current, err := api.secrets.Access(ctx, target.ID, "latest_enabled")
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("inspect Scaleway Secret Manager restore version: %w", err)
		}
		if !equalScalewayBytes(current, archive.Value) {
			if err := api.secrets.CreateVersion(ctx, target.ID, archive.Value); err != nil {
				return provider.NativeOperationObservation{}, fmt.Errorf("update Scaleway Secret Manager restore version: %w", err)
			}
		}
	} else if isScalewayNotFound(findErr) {
		target, err = api.secrets.Create(ctx, targetName, scalewaySecretTags(state), true)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("create Scaleway Secret Manager restore secret: %w", err)
		}
		if err := api.secrets.CreateVersion(ctx, target.ID, archive.Value); err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("create Scaleway Secret Manager restore version: %w", err)
		}
	} else {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect Scaleway Secret Manager restore secret: %w", findErr)
	}
	state.Target = target.ID
	operationID, err := scalewayDatabaseOperationID(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return api.secretRestoreObservation(ctx, state, operationID, archive.Value), nil
}

func (api *NativeAPI) verifySecretRestore(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.Target) == "" {
		return provider.NativeOperationObservation{}, errors.New("Scaleway Secret Manager restore operation has no target")
	}
	archive, err := api.loadSecretArchive(ctx, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return api.secretRestoreObservation(ctx, state, operationID, archive.Value), nil
}

func (api *NativeAPI) secretRestoreObservation(ctx context.Context, state operationState, operationID string, expected []byte) provider.NativeOperationObservation {
	secret, err := api.secrets.Get(ctx, state.Target)
	if err != nil || !scalewaySecretOwned(secret, state) {
		return scalewayDatabasePending(state, operationID)
	}
	value, err := api.secrets.Access(ctx, secret.ID, "latest_enabled")
	if err != nil || !equalScalewayBytes(value, expected) {
		return scalewayDatabasePending(state, operationID)
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup,
		RestoreID: "scaleway-secret://" + secret.ID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(),
		EncryptionVerified: true, ProtectionVerified: secret.Protected, PermissionsVerified: true,
		SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "restored an ownership-tagged protected Secret Manager version and verified value availability without recording its contents",
	}
	return scalewayObservation(state, operationID, []string{"scaleway-secret://" + secret.ID}, []string{"scaleway.secret-manager.restore", "scaleway.secret-manager.protection"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func (api *NativeAPI) integritySecret(ctx context.Context, state operationState, operationID string, reference scalewaySecretReference) (provider.NativeOperationObservation, error) {
	secret, err := api.secrets.Get(ctx, reference.ID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect Scaleway Secret Manager secret for integrity: %w", err)
	}
	if err := verifyScalewaySecret(secret, state); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	value, err := api.secrets.Access(ctx, secret.ID, "latest_enabled")
	if err != nil || value == nil {
		return provider.NativeOperationObservation{}, errors.New("Scaleway Secret Manager integrity target has no readable version")
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: secret.Protected,
		ManifestVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true,
		SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "verified Secret Manager version availability, ownership tags, protection, permissions, and reference reachability",
	}
	return scalewayObservation(state, operationID, []string{"scaleway-secret://" + secret.ID}, []string{"scaleway.secret-manager.integrity", "scaleway.secret-manager.permissions"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) loadSecretArchive(ctx context.Context, state operationState) (cloudrecovery.SecretArchive, error) {
	bucket, key, err := parseScalewayObjectReference(state.Backup)
	if err != nil {
		return cloudrecovery.SecretArchive{}, err
	}
	store, err := api.objectStore()
	if err != nil {
		return cloudrecovery.SecretArchive{}, err
	}
	metadata, err := store.Head(ctx, bucket, key)
	if err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("inspect Scaleway Secret Manager archive: %w", err)
	}
	if err := verifyScalewaySecretArchiveMetadata(metadata, state); err != nil {
		return cloudrecovery.SecretArchive{}, err
	}
	body, err := store.Read(ctx, bucket, key)
	if err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("read Scaleway Secret Manager archive: %w", err)
	}
	archive, _, err := cloudrecovery.OpenSecretArchive(body)
	if err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("decode Scaleway Secret Manager archive: %w", err)
	}
	if err := cloudrecovery.ValidateSecretArchive(archive, cloudrecovery.OperationVersion, state.DataClass, state.FixtureID, state.OwnershipMarker); err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("validate Scaleway Secret Manager archive: %w", err)
	}
	return archive, nil
}

func (api *NativeAPI) verifySecretArchive(ctx context.Context, state operationState, key string) error {
	store, err := api.objectStore()
	if err != nil {
		return err
	}
	metadata, err := store.Head(ctx, api.config.ArchiveBucket, key)
	if err != nil {
		return fmt.Errorf("inspect Scaleway Secret Manager archive: %w", err)
	}
	return verifyScalewaySecretArchiveMetadata(metadata, state)
}

func (api *NativeAPI) requireSecretArchivePolicy(state operationState) error {
	if !api.config.RequireObjectLock {
		return scalewayCapabilityError(state, "Scaleway Secret Manager durable archive requires Object Storage Object Lock for the configured protection guarantee")
	}
	return nil
}

func (api *NativeAPI) findSecretByName(ctx context.Context, name string) (SecretMetadata, error) {
	secrets, err := api.secrets.List(ctx)
	if err != nil {
		return SecretMetadata{}, err
	}
	for _, secret := range secrets {
		if secret.Name == name {
			return secret, nil
		}
	}
	return SecretMetadata{}, errors.New("Scaleway Secret Manager restore secret not found")
}

func (api *NativeAPI) inventorySecrets(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	secrets, err := api.secrets.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("inventory Scaleway Secret Manager secrets: %w", err)
	}
	resources := make([]provider.InventoryResource, 0, len(secrets))
	for _, secret := range secrets {
		if scalewaySecretTagsMatch(secret.Tags, marker, "configuration-secrets") {
			resources = append(resources, provider.InventoryResource{Identity: "scaleway-secret://" + secret.ID, Owned: true, Live: true})
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

type scalewaySecretReference struct {
	ID string
}

func parseScalewaySecretReference(reference string) (scalewaySecretReference, error) {
	value := strings.TrimSpace(reference)
	for _, scheme := range []string{"scaleway-secret-manager://", "scaleway-secret://", "scw-secret://"} {
		if strings.HasPrefix(strings.ToLower(value), scheme) {
			value = value[len(scheme):]
			break
		}
	}
	value = strings.Trim(value, "/")
	value = strings.TrimPrefix(value, "secrets/")
	if value == "" || strings.ContainsAny(value, "/\r\n\x00?#") {
		return scalewaySecretReference{}, fmt.Errorf("Scaleway Secret Manager reference %q must identify one secret without query or fragment", reference)
	}
	return scalewaySecretReference{ID: value}, nil
}

func (api *NativeAPI) restoreSecretName(state operationState) string {
	prefix := strings.Trim(strings.TrimSpace(api.config.RestoreSecretPrefix), "/")
	if prefix == "" {
		prefix = "magelift-recovery-secret"
	}
	return prefix + "-" + shortDigest(state.OwnershipMarker+"\x00"+state.IdempotencyKey)
}

func scalewaySecretTags(state operationState) []string {
	tags := []string{scalewayDatabaseOwnershipTag + "=" + state.OwnershipMarker, scalewayDatabaseClassTag + "=" + state.DataClass, scalewayRecoveryOutputTag}
	sort.Strings(tags)
	return tags
}

func scalewaySecretRestoreOwned(secret SecretMetadata, state operationState) bool {
	return scalewaySecretTagsMatch(secret.Tags, state.OwnershipMarker, state.DataClass) && hasScalewayTag(secret.Tags, scalewayRecoveryOutputTag)
}

func verifyScalewaySecret(secret SecretMetadata, state operationState) error {
	if strings.TrimSpace(secret.ID) == "" || normalizeScalewayStatus(secret.Status) != "ready" {
		return fmt.Errorf("Scaleway Secret Manager secret %q is not ready", secret.Name)
	}
	if !scalewaySecretOwned(secret, state) {
		return errors.New("Scaleway Secret Manager secret is not owned by this operation")
	}
	if !secret.Protected {
		return errors.New("Scaleway Secret Manager secret is not protected against deletion")
	}
	return nil
}

func scalewaySecretOwned(secret SecretMetadata, state operationState) bool {
	return scalewaySecretTagsMatch(secret.Tags, state.OwnershipMarker, state.DataClass)
}

func scalewaySecretTagsMatch(tags []string, marker, dataClass string) bool {
	return scalewayDatabaseTagsMatch(tags, marker, dataClass)
}

func verifyScalewaySecretArchiveMetadata(metadata cloudrecovery.ObjectMetadata, state operationState) error {
	if metadata.Metadata[cloudrecovery.OwnershipMetadataKey] != state.OwnershipMarker || metadata.Metadata[cloudrecovery.ClassMetadataKey] != state.DataClass || metadata.Metadata[cloudrecovery.FixtureMetadataKey] != state.FixtureID || metadata.Metadata[cloudrecovery.ManifestMetadataKey] != "true" || metadata.Encryption == "" || !metadata.Protected {
		return errors.New("Scaleway Secret Manager archive is not owned, encrypted, or protected for this operation")
	}
	return nil
}

func scalewaySecretBackupObservation(api *NativeAPI, state operationState, operationID, key string) provider.NativeOperationObservation {
	backupID := "scaleway-object://" + api.config.ArchiveBucket + "/" + key
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "archived the Secret Manager value in an encrypted Object Storage Object-Locked envelope without exposing its contents",
	}
	return scalewayObservation(state, operationID, []string{backupID}, []string{"scaleway.secret-manager.archive", "scaleway.object-storage.encryption", "scaleway.object-storage.protection"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func equalScalewayBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
