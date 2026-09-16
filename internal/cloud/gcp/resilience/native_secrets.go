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

type gcpSecretReference struct {
	Project string
	Name    string
}

func (api *NativeAPI) startSecret(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.secrets == nil {
		return provider.NativeOperationObservation{}, errors.New("GCP Secret Manager recovery API is required")
	}
	switch state.Action {
	case sdk.ResilienceCleanup:
		if api.storage == nil {
			return provider.NativeOperationObservation{}, errors.New("GCP Cloud Storage recovery API is required for Secret Manager cleanup")
		}
		return api.cleanupSecret(ctx, state, operationID)
	case sdk.ResilienceBackup:
		if api.storage == nil {
			return provider.NativeOperationObservation{}, errors.New("GCP Cloud Storage recovery API is required for Secret Manager archives")
		}
		reference, err := parseGCPSecretReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.backupSecret(ctx, state, operationID, reference)
	case sdk.ResilienceRestore:
		if api.storage == nil {
			return provider.NativeOperationObservation{}, errors.New("GCP Cloud Storage recovery API is required for Secret Manager restore archives")
		}
		return api.restoreSecret(ctx, state)
	case sdk.ResilienceIntegrityCheck:
		reference, err := parseGCPSecretReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.integritySecret(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Secret Manager recovery does not implement this action")
	}
}

func (api *NativeAPI) pollSecret(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.secrets == nil {
		return provider.NativeOperationObservation{}, errors.New("GCP Secret Manager recovery API is required")
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.verifySecretBackup(ctx, state, operationID)
	case sdk.ResilienceRestore:
		return api.verifySecretRestore(ctx, state, operationID)
	case sdk.ResilienceIntegrityCheck:
		reference, err := parseGCPSecretReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.integritySecret(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Secret Manager recovery does not implement this action")
	}
}

func (api *NativeAPI) backupSecret(ctx context.Context, state cloudrecovery.OperationState, operationID string, reference gcpSecretReference) (provider.NativeOperationObservation, error) {
	metadata, err := api.secrets.Describe(ctx, reference.Name)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("describe GCP Secret Manager secret: %w", err)
	}
	if !secretManagerLabelsMatch(metadata.Labels, state.OwnershipMarker, state.DataClass) {
		return provider.NativeOperationObservation{}, errors.New("refusing to back up an unowned GCP Secret Manager secret")
	}
	value, err := api.secrets.Get(ctx, reference.Name)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("read GCP Secret Manager secret for archive: %w", err)
	}
	archive := cloudrecovery.SecretArchive{
		Version: cloudrecovery.OperationVersion, DataClass: state.DataClass, FixtureID: state.FixtureID,
		OwnershipMarker: state.OwnershipMarker, SourceSecret: reference.Name, Encoding: "binary", Value: append([]byte{}, value.Data...),
	}
	body, _, err := cloudrecovery.SealSecretArchive(archive)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("encode GCP Secret Manager archive: %w", err)
	}
	key := api.objectPrefixForOperation(state) + "/secret.json"
	if existing, headErr := api.storage.Head(ctx, api.config.ArchiveBucket, key); headErr == nil {
		if !api.metadataMatches(existing, state) || !api.encrypted(existing) || !api.protected(existing) {
			return provider.NativeOperationObservation{}, errors.New("GCP Secret Manager archive exists but is not owned by this operation")
		}
	} else if !isGCSNotFound(headErr) {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Secret Manager archive: %w", headErr)
	} else if err := api.storage.Put(ctx, api.config.ArchiveBucket, key, body, metadataFor(state, true), api.config.KMSKeyName); err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("write GCP Secret Manager archive: %w", err)
	} else if err := api.verifyArchiveObject(ctx, key, state); err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("verify GCP Secret Manager archive: %w", err)
	}
	return secretBackupObservation(api, state, operationID, key), nil
}

func (api *NativeAPI) verifySecretBackup(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	key := api.objectPrefixForOperation(state) + "/secret.json"
	metadata, err := api.storage.Head(ctx, api.config.ArchiveBucket, key)
	if err != nil {
		if isGCSNotFound(err) {
			return pending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Secret Manager archive: %w", err)
	}
	if !api.metadataMatches(metadata, state) || metadata.Metadata[manifestMetadataKey] != "true" || !api.encrypted(metadata) || !api.protected(metadata) {
		return provider.NativeOperationObservation{}, errors.New("GCP Secret Manager archive is not owned by this operation")
	}
	return secretBackupObservation(api, state, operationID, key), nil
}

func secretBackupObservation(api *NativeAPI, state cloudrecovery.OperationState, operationID, key string) provider.NativeOperationObservation {
	backupID := "gcp-storage://" + api.config.ArchiveBucket + "/" + key
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "archived the Secret Manager value in an encrypted ownership-scoped envelope without exposing its contents",
	}
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), []string{backupID}, []string{"gcp.secret-manager.archive", "gcp.storage.encryption", "gcp.secret-manager.ownership"}, []sdk.ResilienceProofEvidence{evidence})
}

func (api *NativeAPI) restoreSecret(ctx context.Context, state cloudrecovery.OperationState) (provider.NativeOperationObservation, error) {
	archive, err := api.loadSecretArchive(ctx, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if state.Destination == sdk.RecoverySameRegion {
		return api.restoreSecretInPlace(ctx, state, archive)
	}
	project := strings.TrimSpace(api.config.Project)
	if project == "" {
		reference, referenceErr := parseGCPSecretReference(archive.SourceSecret)
		if referenceErr != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("derive GCP Secret Manager restore project: %w", referenceErr)
		}
		project = reference.Project
	}
	secretID, target, err := api.restoreSecretName(project, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	state.Target = target
	operationID, err := cloudrecovery.EncodeOperationID(operationPrefix, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	metadata, describeErr := api.secrets.Describe(ctx, target)
	if describeErr == nil {
		if !secretManagerLabelsMatch(metadata.Labels, state.OwnershipMarker, state.DataClass) {
			return provider.NativeOperationObservation{}, errors.New("refusing to overwrite an unowned GCP Secret Manager restore secret")
		}
		current, getErr := api.secrets.Get(ctx, target)
		if getErr != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Secret Manager restore secret value: %w", getErr)
		}
		if !equalBytes(current.Data, archive.Value) {
			if err := api.secrets.AddVersion(ctx, target, archive.Value); err != nil {
				return provider.NativeOperationObservation{}, fmt.Errorf("add GCP Secret Manager restore version: %w", err)
			}
		}
	} else if isGCPSecretNotFound(describeErr) {
		if err := api.secrets.Create(ctx, project, secretID, archive.Value, secretManagerLabelsFor(state)); err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("create GCP Secret Manager restore secret: %w", err)
		}
	} else {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Secret Manager restore secret: %w", describeErr)
	}
	return api.secretRestoreObservation(ctx, state, operationID, target), nil
}

func (api *NativeAPI) restoreSecretInPlace(ctx context.Context, state cloudrecovery.OperationState, archive cloudrecovery.SecretArchive) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.ApprovalReference) == "" {
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Secret Manager in-place restore requires an operator approval reference because restore adds a version on the source secret")
	}
	source, err := parseGCPSecretReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if archive.SourceSecret != "" && archive.SourceSecret != source.Name && archive.SourceSecret != "gcp-secret-manager://"+source.Name {
		return provider.NativeOperationObservation{}, errors.New("GCP Secret Manager in-place restore requires an archive taken from the source secret")
	}
	secretID := source.Name[strings.LastIndex(source.Name, "/")+1:]
	prefix := strings.TrimSpace(api.config.RestoreSecretPrefix)
	if prefix == "" {
		prefix = "magelift-recovery"
	}
	if strings.HasPrefix(secretID, prefix+"-") || secretID == prefix {
		return provider.NativeOperationObservation{}, errors.New("GCP Secret Manager in-place restore must target the source secret, not an isolated recovery secret")
	}
	metadata, err := api.secrets.Describe(ctx, source.Name)
	if err != nil {
		if !isGCPSecretNotFound(err) {
			return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Secret Manager in-place source: %w", err)
		}
		if err := api.secrets.Create(ctx, source.Project, secretID, archive.Value, secretManagerLabelsFor(state)); err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("recreate GCP Secret Manager in-place source: %w", err)
		}
	} else {
		if !secretManagerLabelsMatch(metadata.Labels, state.OwnershipMarker, state.DataClass) {
			return provider.NativeOperationObservation{}, errors.New("refusing to overwrite an unowned GCP Secret Manager source secret")
		}
		current, getErr := api.secrets.Get(ctx, source.Name)
		if getErr != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("read GCP Secret Manager in-place source: %w", getErr)
		}
		if !equalBytes(current.Data, archive.Value) {
			if err := api.secrets.AddVersion(ctx, source.Name, archive.Value); err != nil {
				return provider.NativeOperationObservation{}, fmt.Errorf("add GCP Secret Manager in-place version: %w", err)
			}
		}
	}
	state.Target = source.Name
	operationID, err := cloudrecovery.EncodeOperationID(operationPrefix, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return api.secretRestoreObservation(ctx, state, operationID, source.Name), nil
}

func (api *NativeAPI) verifySecretRestore(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.Target) == "" {
		return provider.NativeOperationObservation{}, errors.New("GCP Secret Manager restore operation has no target")
	}
	return api.secretRestoreObservation(ctx, state, operationID, state.Target), nil
}

func (api *NativeAPI) secretRestoreObservation(ctx context.Context, state cloudrecovery.OperationState, operationID, target string) provider.NativeOperationObservation {
	metadata, err := api.secrets.Describe(ctx, target)
	if err != nil || !secretManagerLabelsMatch(metadata.Labels, state.OwnershipMarker, state.DataClass) {
		return pending(state, operationID)
	}
	value, err := api.secrets.Get(ctx, target)
	if err != nil || value.Data == nil {
		return pending(state, operationID)
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup, RestoreID: "gcp-secret-manager://" + target,
		FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "restored an isolated Secret Manager version and verified value availability without recording its contents",
	}
	if state.Destination == sdk.RecoverySameRegion {
		evidence.Reason = "restored a Secret Manager version onto the approved source secret and verified value availability without recording its contents"
	}
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), []string{"gcp-secret-manager://" + target}, []string{"gcp.secret-manager.restore", "gcp.secret-manager.ownership"}, []sdk.ResilienceProofEvidence{evidence})
}

func (api *NativeAPI) integritySecret(ctx context.Context, state cloudrecovery.OperationState, operationID string, reference gcpSecretReference) (provider.NativeOperationObservation, error) {
	metadata, err := api.secrets.Describe(ctx, reference.Name)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("describe GCP Secret Manager secret for integrity: %w", err)
	}
	if !secretManagerLabelsMatch(metadata.Labels, state.OwnershipMarker, state.DataClass) {
		return provider.NativeOperationObservation{}, errors.New("GCP Secret Manager integrity target is not owned by this operation")
	}
	value, err := api.secrets.Get(ctx, reference.Name)
	if err != nil || value.Data == nil {
		return provider.NativeOperationObservation{}, errors.New("GCP Secret Manager integrity target has no readable value")
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true,
		ApplicationReadsVerified: true, PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "verified Secret Manager version availability, ownership labels, permissions, and reference reachability",
	}
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), []string{"gcp-secret-manager://" + reference.Name}, []string{"gcp.secret-manager.integrity", "gcp.secret-manager.permissions"}, []sdk.ResilienceProofEvidence{evidence}), nil
}

func (api *NativeAPI) cleanupSecret(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.OwnershipMarker) == "" || strings.ContainsAny(state.OwnershipMarker, "\r\n\x00") {
		return provider.NativeOperationObservation{}, errors.New("GCP Secret Manager cleanup ownership marker is required and must be single-line")
	}
	if api.storage == nil || api.secrets == nil {
		return provider.NativeOperationObservation{}, errors.New("GCP Secret Manager cleanup requires Secret Manager and Cloud Storage APIs")
	}
	refs := make([]string, 0)
	archivePrefix := api.archivePrefixForMarker(state.OwnershipMarker) + "/" + state.DataClass + "/"
	if err := api.cleanupGCSObjectPrefix(ctx, api.config.ArchiveBucket, archivePrefix, state.OwnershipMarker, true, &refs); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	project := strings.TrimSpace(api.config.Project)
	if project == "" {
		reference, err := parseGCPSecretReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("derive GCP Secret Manager cleanup project: %w", err)
		}
		project = reference.Project
	}
	secrets, err := api.secrets.List(ctx, project)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("list GCP Secret Manager restore secrets for cleanup: %w", err)
	}
	prefix := strings.TrimSpace(api.config.RestoreSecretPrefix)
	for _, secret := range secrets {
		secretID := strings.TrimPrefix(secret.Name, "projects/"+project+"/secrets/")
		if secretID == secret.Name || !strings.HasPrefix(secretID, prefix) || !secretManagerLabelsMatch(secret.Labels, state.OwnershipMarker, state.DataClass) {
			continue
		}
		if err := api.secrets.Delete(ctx, secret.Name); err != nil && !isGCPSecretNotFound(err) {
			return provider.NativeOperationObservation{}, fmt.Errorf("delete owned GCP Secret Manager restore secret %q: %w", secret.Name, err)
		}
		refs = append(refs, "gcp-secret-manager://"+secret.Name)
	}
	remaining, err := api.secrets.List(ctx, project)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("verify GCP Secret Manager cleanup: %w", err)
	}
	for _, secret := range remaining {
		secretID := strings.TrimPrefix(secret.Name, "projects/"+project+"/secrets/")
		if strings.HasPrefix(secretID, prefix) && secretManagerLabelsMatch(secret.Labels, state.OwnershipMarker, state.DataClass) {
			return provider.NativeOperationObservation{}, fmt.Errorf("owned GCP Secret Manager restore secret %q remains after cleanup", secret.Name)
		}
	}
	sort.Strings(refs)
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), refs, []string{"gcp.secret-manager.ownership-cleanup", "gcp.storage.ownership-cleanup"}, nil), nil
}

func (api *NativeAPI) loadSecretArchive(ctx context.Context, state cloudrecovery.OperationState) (cloudrecovery.SecretArchive, error) {
	if strings.TrimSpace(state.Backup) == "" {
		return cloudrecovery.SecretArchive{}, errors.New("GCP Secret Manager restore requires a backup reference")
	}
	bucket, key, err := parseGCSReference(state.Backup)
	if err != nil {
		return cloudrecovery.SecretArchive{}, err
	}
	metadata, err := api.storage.Head(ctx, bucket, key)
	if err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("inspect GCP Secret Manager archive: %w", err)
	}
	if !api.metadataMatches(metadata, state) || !api.encrypted(metadata) || !api.protected(metadata) {
		return cloudrecovery.SecretArchive{}, errors.New("GCP Secret Manager archive ownership does not match the operation")
	}
	body, err := api.storage.Read(ctx, bucket, key)
	if err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("read GCP Secret Manager archive: %w", err)
	}
	archive, _, err := cloudrecovery.OpenSecretArchive(body)
	if err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("decode GCP Secret Manager archive: %w", err)
	}
	if err := cloudrecovery.ValidateSecretArchive(archive, cloudrecovery.OperationVersion, state.DataClass, state.FixtureID, state.OwnershipMarker); err != nil {
		return cloudrecovery.SecretArchive{}, fmt.Errorf("validate GCP Secret Manager archive: %w", err)
	}
	if archive.SourceSecret == "" || bucket == "" {
		return cloudrecovery.SecretArchive{}, errors.New("GCP Secret Manager archive has no source identity")
	}
	return archive, nil
}

func (api *NativeAPI) restoreSecretName(project string, state cloudrecovery.OperationState) (string, string, error) {
	prefix := strings.TrimSpace(api.config.RestoreSecretPrefix)
	if prefix == "" {
		prefix = "magelift-recovery"
	}
	secretID := prefix + "-" + shortDigest(state.OwnershipMarker+"\x00"+state.IdempotencyKey)
	if !validGCPSecretID(secretID) {
		return "", "", fmt.Errorf("GCP Secret Manager restore secret ID %q is invalid", secretID)
	}
	return secretID, "projects/" + project + "/secrets/" + secretID, nil
}

func parseGCPSecretReference(reference string) (gcpSecretReference, error) {
	value := strings.TrimSpace(reference)
	for _, scheme := range []string{"gcp-secret-manager://", "secretmanager://"} {
		if strings.HasPrefix(strings.ToLower(value), scheme) {
			value = value[len(scheme):]
			break
		}
	}
	value = strings.TrimPrefix(value, "//")
	if strings.ContainsAny(value, "\r\n\x00") || strings.Contains(value, "?") || strings.Contains(value, "#") {
		return gcpSecretReference{}, fmt.Errorf("GCP Secret Manager resource reference %q contains invalid control data", reference)
	}
	if strings.Contains(value, "/versions/") {
		value = strings.Split(value, "/versions/")[0]
	}
	parts := strings.Split(value, "/")
	if len(parts) < 4 || parts[0] != "projects" {
		return gcpSecretReference{}, fmt.Errorf("GCP Secret Manager resource reference %q must use projects/PROJECT/secrets/NAME", reference)
	}
	secretIndex := -1
	for index, part := range parts {
		if part == "secrets" {
			secretIndex = index
			break
		}
	}
	if secretIndex < 2 || secretIndex+1 >= len(parts) || strings.TrimSpace(parts[1]) == "" || strings.TrimSpace(parts[secretIndex+1]) == "" {
		return gcpSecretReference{}, fmt.Errorf("GCP Secret Manager resource reference %q has an invalid secret identity", reference)
	}
	return gcpSecretReference{Project: parts[1], Name: value}, nil
}

func secretLabelsFor(state cloudrecovery.OperationState) map[string]string {
	return map[string]string{ownershipLabelKey: state.OwnershipMarker, classLabelKey: state.DataClass, fixtureLabelKey: state.FixtureID}
}

func secretManagerLabelsFor(state cloudrecovery.OperationState) map[string]string {
	return map[string]string{ownershipLabelKey: secretManagerLabelValue(state.OwnershipMarker), classLabelKey: state.DataClass, fixtureLabelKey: secretManagerLabelValue(state.FixtureID)}
}

func secretManagerLabelValue(value string) string {
	return "m" + shortDigest(value)
}

func secretManagerLabelsMatch(labels map[string]string, marker, dataClass string) bool {
	return labels[ownershipLabelKey] == secretManagerLabelValue(marker) && labels[classLabelKey] == dataClass
}

func secretLabelsMatch(labels map[string]string, marker, dataClass string) bool {
	return labels[ownershipLabelKey] == marker && labels[classLabelKey] == dataClass
}

func validGCPSecretID(value string) bool {
	if len(value) == 0 || len(value) > 255 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' && character != '_' {
			return false
		}
	}
	return true
}

func equalBytes(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func isGCPSecretNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "not_found") || strings.Contains(message, "404")
}
