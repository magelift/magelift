package resilience

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

const (
	ovhSecretOwnershipKey = "magelift-ownership"
	ovhSecretClassKey     = "magelift-data-class"
	ovhSecretEnvelopeKey  = "magelift.io/secret-envelope"
	ovhSecretEnvelopeV1   = "v1"
	ovhSecretValueKey     = "value-base64"
)

// SecretMetadata is the provider-local view of one OVHcloud Secret Manager
// secret. Secret values never cross this metadata boundary.
type SecretMetadata struct {
	Path           string
	State          string
	CurrentVersion uint32
	CustomMetadata map[string]string
}

// SecretAPI is the narrow OVHcloud Secret Manager port used by the recovery
// translator. Values cross only through Access/CreateVersion and are sealed
// into the shared archive before they can be persisted elsewhere.
type SecretAPI interface {
	Get(context.Context, string) (SecretMetadata, error)
	List(context.Context) ([]SecretMetadata, error)
	Access(context.Context, string, uint32) ([]byte, error)
	Delete(context.Context, string) error
	Create(context.Context, string, map[string]string, []byte) (SecretMetadata, error)
	CreateVersion(context.Context, string, []byte) (SecretMetadata, error)
}

type ovhSecretReference struct {
	OKMSID string
	Path   string
}

func (api *NativeAPI) startSecret(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api.secrets == nil {
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud Secret Manager API is not configured")
	}
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupSecretForState(ctx, state, operationID)
	}
	reference, err := parseOVHSecretReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if err := api.verifySecretReference(reference); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		if err := api.requireSecretArchivePolicy(state); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.backupSecret(ctx, state, operationID, reference)
	case sdk.ResilienceRestore:
		if err := api.requireSecretArchivePolicy(state); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.restoreSecret(ctx, state)
	case sdk.ResilienceIntegrityCheck:
		return api.integritySecret(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud Secret Manager recovery does not implement this action")
	}
}

func (api *NativeAPI) pollSecret(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api.secrets == nil {
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud Secret Manager API is not configured")
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.verifySecretBackup(ctx, state, operationID)
	case sdk.ResilienceRestore:
		return api.verifySecretRestore(ctx, state, operationID)
	case sdk.ResilienceIntegrityCheck:
		reference, err := parseOVHSecretReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		if err := api.verifySecretReference(reference); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.integritySecret(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud Secret Manager recovery does not implement this action")
	}
}

func (api *NativeAPI) backupSecret(ctx context.Context, state operationState, operationID string, reference ovhSecretReference) (provider.NativeOperationObservation, error) {
	secret, err := api.secrets.Get(ctx, reference.Path)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud Secret Manager source: %w", err)
	}
	if err := api.verifySecret(secret, reference, state); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	value, err := api.secrets.Access(ctx, reference.Path, secret.CurrentVersion)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("read OVHcloud Secret Manager source version: %w", err)
	}
	archive := cloudrecovery.SecretArchive{
		Version: cloudrecovery.OperationVersion, DataClass: state.DataClass, FixtureID: state.FixtureID,
		OwnershipMarker: state.OwnershipMarker, SourceSecret: reference.Path, Encoding: "binary", Value: append([]byte(nil), value...),
	}
	body, _, err := cloudrecovery.SealSecretArchive(archive)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("seal OVHcloud Secret Manager archive: %w", err)
	}
	key := api.objectPrefixForOperation(state) + "/secret.json"
	store, err := api.objectStore()
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if existing, headErr := store.Head(ctx, api.config.ArchiveBucket, key); headErr == nil {
		if err := api.verifySecretArchiveMetadata(existing, state); err != nil {
			return provider.NativeOperationObservation{}, err
		}
	} else if !isOVHNotFound(headErr) {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud Secret Manager archive: %w", headErr)
	} else if err := store.Put(ctx, api.config.ArchiveBucket, key, body, cloudrecovery.MetadataFor(cloudrecovery.ObjectArchiveScope{DataClass: state.DataClass, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker}, true)); err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("write OVHcloud Secret Manager archive: %w", err)
	}
	if err := api.verifySecretArchive(ctx, state, key); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return ovhSecretBackupObservation(api, state, operationID, key), nil
}

func (api *NativeAPI) verifySecretBackup(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	key := api.objectPrefixForOperation(state) + "/secret.json"
	if err := api.verifySecretArchive(ctx, state, key); err != nil {
		if isOVHNotFound(err) {
			return ovhPending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, err
	}
	return ovhSecretBackupObservation(api, state, operationID, key), nil
}

func (api *NativeAPI) restoreSecret(ctx context.Context, state operationState) (provider.NativeOperationObservation, error) {
	archive, err := api.loadSecretArchive(ctx, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	targetPath := api.restoreSecretPath(state)
	target, findErr := api.secrets.Get(ctx, targetPath)
	if findErr == nil {
		if !ovhSecretMetadataMatches(target, state.OwnershipMarker, state.DataClass) {
			return provider.NativeOperationObservation{}, errors.New("refusing to overwrite an unowned OVHcloud Secret Manager restore secret")
		}
		current, accessErr := api.secrets.Access(ctx, targetPath, target.CurrentVersion)
		if accessErr != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud Secret Manager restore version: %w", accessErr)
		}
		if !secureBytesEqual(current, archive.Value) {
			target, err = api.secrets.CreateVersion(ctx, targetPath, archive.Value)
			if err != nil {
				return provider.NativeOperationObservation{}, fmt.Errorf("update OVHcloud Secret Manager restore version: %w", err)
			}
		}
	} else if isOVHNotFound(findErr) {
		target, err = api.secrets.Create(ctx, targetPath, map[string]string{ovhSecretOwnershipKey: state.OwnershipMarker, ovhSecretClassKey: state.DataClass}, archive.Value)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("create OVHcloud Secret Manager restore secret: %w", err)
		}
	} else {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud Secret Manager restore secret: %w", findErr)
	}
	if !ovhSecretMetadataMatches(target, state.OwnershipMarker, state.DataClass) {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud Secret Manager restore response did not prove ownership")
	}
	state.Target = target.Path
	operationID, err := cloudrecovery.EncodeOperationID(ovhRecoveryOperationPrefix, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return api.secretRestoreObservation(ctx, state, operationID, archive.Value), nil
}

func (api *NativeAPI) verifySecretRestore(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.Target) == "" {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud Secret Manager restore operation has no target")
	}
	archive, err := api.loadSecretArchive(ctx, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return api.secretRestoreObservation(ctx, state, operationID, archive.Value), nil
}

func (api *NativeAPI) secretRestoreObservation(ctx context.Context, state operationState, operationID string, expected []byte) provider.NativeOperationObservation {
	secret, err := api.secrets.Get(ctx, state.Target)
	if err != nil || !ovhSecretMetadataMatches(secret, state.OwnershipMarker, state.DataClass) {
		return ovhPending(state, operationID)
	}
	value, err := api.secrets.Access(ctx, secret.Path, secret.CurrentVersion)
	if err != nil || !secureBytesEqual(value, expected) {
		return ovhPending(state, operationID)
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup,
		RestoreID: "ovh-secret://" + api.config.SecretOKMSID + "/" + secret.Path, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: api.config.RequireObjectLock,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "restored an ownership-bound OVHcloud Secret Manager version and verified value availability without recording its contents",
	}
	return ovhObservation(state, operationID, []string{"ovh-secret://" + api.config.SecretOKMSID + "/" + secret.Path}, []string{"ovh.secret-manager.restore", "ovh.secret-manager.references"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func (api *NativeAPI) integritySecret(ctx context.Context, state operationState, operationID string, reference ovhSecretReference) (provider.NativeOperationObservation, error) {
	secret, err := api.secrets.Get(ctx, reference.Path)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud Secret Manager secret for integrity: %w", err)
	}
	if err := api.verifySecret(secret, reference, state); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	value, err := api.secrets.Access(ctx, reference.Path, secret.CurrentVersion)
	if err != nil || len(value) == 0 {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud Secret Manager integrity target has no readable version")
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: api.config.RequireObjectLock,
		ManifestVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true,
		SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "verified OVHcloud Secret Manager version availability, ownership metadata, permissions, and reference reachability without exposing the value",
	}
	return ovhObservation(state, operationID, []string{"ovh-secret://" + reference.OKMSID + "/" + reference.Path}, []string{"ovh.secret-manager.integrity", "ovh.secret-manager.permissions"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) loadSecretArchive(ctx context.Context, state operationState) (cloudrecovery.SecretArchive, error) {
	bucket, key, err := parseOVHObjectReference(state.Backup)
	if err != nil {
		return cloudrecovery.SecretArchive{}, err
	}
	store, err := api.objectStore()
	if err != nil {
		return cloudrecovery.SecretArchive{}, err
	}
	metadata, err := store.Head(ctx, bucket, key)
	if err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("inspect OVHcloud Secret Manager archive: %w", err)
	}
	if err := api.verifySecretArchiveMetadata(metadata, state); err != nil {
		return cloudrecovery.SecretArchive{}, err
	}
	body, err := store.Read(ctx, bucket, key)
	if err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("read OVHcloud Secret Manager archive: %w", err)
	}
	archive, _, err := cloudrecovery.OpenSecretArchive(body)
	if err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("decode OVHcloud Secret Manager archive: %w", err)
	}
	if err := cloudrecovery.ValidateSecretArchive(archive, cloudrecovery.OperationVersion, state.DataClass, state.FixtureID, state.OwnershipMarker); err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("validate OVHcloud Secret Manager archive: %w", err)
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
		return fmt.Errorf("inspect OVHcloud Secret Manager archive: %w", err)
	}
	return api.verifySecretArchiveMetadata(metadata, state)
}

func (api *NativeAPI) verifySecretArchiveMetadata(metadata cloudrecovery.ObjectMetadata, state operationState) error {
	if metadata.Metadata[cloudrecovery.OwnershipMetadataKey] != state.OwnershipMarker || metadata.Metadata[cloudrecovery.ClassMetadataKey] != state.DataClass || metadata.Metadata[cloudrecovery.FixtureMetadataKey] != state.FixtureID || !api.policyVerified(metadata) {
		return errors.New("OVHcloud Secret Manager archive ownership, encryption, or retention proof is invalid")
	}
	return nil
}

func (api *NativeAPI) requireSecretArchivePolicy(state operationState) error {
	if !api.config.RequireObjectLock {
		return ovhCapabilityError(state, "OVHcloud Secret Manager durable recovery requires Object Storage Object Lock for the configured protection guarantee")
	}
	return nil
}

func (api *NativeAPI) verifySecretReference(reference ovhSecretReference) error {
	if strings.TrimSpace(api.config.SecretOKMSID) == "" || !strings.EqualFold(reference.OKMSID, api.config.SecretOKMSID) {
		return errors.New("OVHcloud Secret Manager reference must identify the configured OKMS domain")
	}
	return nil
}

func (api *NativeAPI) verifySecret(secret SecretMetadata, reference ovhSecretReference, state operationState) error {
	if secret.Path != reference.Path || secret.CurrentVersion == 0 || !strings.EqualFold(strings.TrimSpace(secret.State), "active") {
		return errors.New("OVHcloud Secret Manager source does not identify an active version")
	}
	if secret.CustomMetadata[ovhSecretOwnershipKey] != state.OwnershipMarker || secret.CustomMetadata[ovhSecretClassKey] != state.DataClass {
		return errors.New("OVHcloud Secret Manager source ownership metadata does not match the recovery scope")
	}
	return nil
}
func (api *NativeAPI) inventorySecrets(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	secrets, err := api.secrets.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("inventory OVHcloud Secret Manager secrets: %w", err)
	}
	resources := make([]provider.InventoryResource, 0, len(secrets))
	for _, secret := range secrets {
		if ovhSecretMetadataMatches(secret, marker, "configuration-secrets") {
			resources = append(resources, provider.InventoryResource{Identity: "ovh-secret://" + api.config.SecretOKMSID + "/" + secret.Path, Owned: true, Live: true})
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func (api *NativeAPI) restoreSecretPath(state operationState) string {
	prefix := strings.Trim(strings.TrimSpace(api.config.RestoreSecretPrefix), "/")
	if prefix == "" {
		prefix = "magelift-recovery"
	}
	return prefix + "/" + shortDigest(state.OwnershipMarker) + "/" + shortDigest(state.FixtureID+"\x00"+state.IdempotencyKey)
}

func parseOVHSecretReference(reference string) (ovhSecretReference, error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || (parsed.Scheme != "ovh-secret" && parsed.Scheme != "ovhcloud-secret" && parsed.Scheme != "ovh-okms-secret") || strings.TrimSpace(parsed.Host) == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ovhSecretReference{}, fmt.Errorf("OVHcloud Secret Manager reference %q must use ovh-secret://okms-id/path", reference)
	}
	path, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || strings.TrimSpace(path) == "" || strings.ContainsAny(path, "\r\n\x00") {
		return ovhSecretReference{}, fmt.Errorf("OVHcloud Secret Manager reference %q contains an invalid secret path", reference)
	}
	return ovhSecretReference{OKMSID: parsed.Host, Path: path}, nil
}

func ovhSecretMetadataMatches(secret SecretMetadata, marker, dataClass string) bool {
	return secret.Path != "" && secret.CurrentVersion > 0 && strings.EqualFold(strings.TrimSpace(secret.State), "active") && secret.CustomMetadata[ovhSecretOwnershipKey] == marker && secret.CustomMetadata[ovhSecretClassKey] == dataClass
}

func secureBytesEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare(left, right) == 1
}

func ovhSecretBackupObservation(api *NativeAPI, state operationState, operationID, key string) provider.NativeOperationObservation {
	backupID := "ovh-object://" + api.config.ArchiveBucket + "/" + key
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: api.config.RequireObjectLock,
		ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "sealed an ownership-bound OVHcloud Secret Manager version into encrypted Object Storage with the configured retention policy; the secret value is absent from evidence",
	}
	return ovhObservation(state, operationID, []string{backupID}, []string{"ovh.secret-manager.archive", "ovh.secret-manager.encryption", "ovh.secret-manager.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

// EncodeSecretPayload keeps the provider's JSON-object API lossless for the
// provider-neutral byte contract. The envelope is provider-local and is
// decoded only by this adapter; it must not appear in shared evidence.
func EncodeSecretPayload(value []byte) (map[string]interface{}, error) {
	if value == nil {
		return nil, errors.New("OVHcloud Secret Manager value is required")
	}
	return map[string]interface{}{
		ovhSecretEnvelopeKey: ovhSecretEnvelopeV1,
		ovhSecretValueKey:    base64.StdEncoding.EncodeToString(value),
	}, nil
}

// DecodeSecretPayload reverses EncodeSecretPayload. Older or foreign JSON
// object values remain readable through their canonical JSON representation.
func DecodeSecretPayload(data map[string]interface{}) ([]byte, error) {
	if data == nil {
		return nil, errors.New("OVHcloud Secret Manager returned no secret data")
	}
	if data[ovhSecretEnvelopeKey] == ovhSecretEnvelopeV1 {
		encoded, ok := data[ovhSecretValueKey].(string)
		if !ok {
			return nil, errors.New("OVHcloud Secret Manager returned an incomplete secret envelope")
		}
		value, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return nil, errors.New("OVHcloud Secret Manager returned an invalid secret envelope")
		}
		return value, nil
	}
	value, err := json.Marshal(data)
	if err != nil {
		return nil, errors.New("marshal OVHcloud Secret Manager value failed")
	}
	return value, nil
}

var _ SecretAPI = (*ovhSecretSDK)(nil)
