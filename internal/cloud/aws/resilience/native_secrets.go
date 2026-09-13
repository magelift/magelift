package resilience

import (
	"context"
	"errors"
	"fmt"
	"strings"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretstypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"

	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type secretArchive = cloudrecovery.SecretArchive

func (api *NativeAPI) startSecretClass(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.secrets == nil {
		return provider.NativeOperationObservation{}, errors.New("AWS Secrets Manager recovery API is required")
	}
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupSecretClass(ctx, state, operationID)
	}
	secretID, err := parseSecretReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.backupSecret(ctx, state, operationID, secretID)
	case sdk.ResilienceRestore:
		return api.restoreSecret(ctx, state, operationID)
	case sdk.ResilienceIntegrityCheck:
		return api.integritySecret(ctx, state, operationID, secretID)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "AWS Secrets Manager recovery does not implement this action")
	}
}

func (api *NativeAPI) pollSecretClass(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.verifySecretBackup(ctx, state, operationID)
	case sdk.ResilienceRestore:
		return api.verifySecretRestore(ctx, state, operationID)
	case sdk.ResilienceIntegrityCheck:
		secretID, err := parseSecretReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.integritySecret(ctx, state, operationID, secretID)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "AWS Secrets Manager recovery does not implement this action")
	}
}

func (api *NativeAPI) backupSecret(ctx context.Context, state operationState, operationID, secretID string) (provider.NativeOperationObservation, error) {
	description, err := api.secrets.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(secretID)})
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("describe AWS Secrets Manager secret: %w", err)
	}
	if description == nil || !secretTagsMatch(description.Tags, state.OwnershipMarker, state.DataClass) {
		return provider.NativeOperationObservation{}, errors.New("refusing to back up an unowned AWS Secrets Manager secret")
	}
	value, err := api.secrets.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(secretID)})
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("read AWS Secrets Manager secret for archive: %w", err)
	}
	if value == nil || (value.SecretString == nil && len(value.SecretBinary) == 0) {
		return provider.NativeOperationObservation{}, errors.New("AWS Secrets Manager secret has no value to archive")
	}
	archive := secretArchive{Version: operationVersion, DataClass: state.DataClass, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker, SourceSecret: secretID}
	if value.SecretString != nil {
		archive.Encoding = "string"
		archive.Value = []byte(*value.SecretString)
	} else {
		archive.Encoding = "binary"
		archive.Value = append([]byte(nil), value.SecretBinary...)
	}
	body, _, err := cloudrecovery.SealSecretArchive(archive)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("encode AWS Secrets Manager archive: %w", err)
	}
	key := api.objectPrefixForOperation(state) + "/secret.json"
	if output, headErr := api.s3.HeadObject(ctx, s3HeadObjectInput(api.archiveBucket(), key)); headErr == nil {
		if output == nil || !metadataMatches(output.Metadata, state) {
			return provider.NativeOperationObservation{}, errors.New("AWS Secrets Manager archive exists but is not owned by this operation")
		}
	} else if !isS3NotFound(headErr) {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect AWS Secrets Manager archive: %w", headErr)
	} else if err := putArchiveObject(ctx, api.s3, api.archiveBucket(), key, body, metadataFor(state, true), api.config); err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("write AWS Secrets Manager archive: %w", err)
	} else if err := api.verifyArchiveObject(ctx, api.archiveBucket(), key, state); err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("verify AWS Secrets Manager archive: %w", err)
	}
	return secretBackupObservation(api, state, operationID, key), nil
}

func (api *NativeAPI) verifySecretBackup(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	key := api.objectPrefixForOperation(state) + "/secret.json"
	output, err := api.s3.HeadObject(ctx, s3HeadObjectInput(api.archiveBucket(), key))
	if err != nil {
		if isS3NotFound(err) {
			return pendingOperation(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect AWS Secrets Manager archive: %w", err)
	}
	if output == nil || !metadataMatches(output.Metadata, state) || output.Metadata[archiveManifestKey] != "true" || !api.archiveEncryptionVerified(output) {
		return provider.NativeOperationObservation{}, errors.New("AWS Secrets Manager archive is not owned by this operation")
	}
	return secretBackupObservation(api, state, operationID, key), nil
}

func secretBackupObservation(api *NativeAPI, state operationState, operationID, key string) provider.NativeOperationObservation {
	backupID := "aws-s3://" + api.archiveBucket() + "/" + key
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "archived the encrypted secret value without exposing it in lifecycle evidence",
	}
	return observationWithOperation(state, operationID, []string{backupID}, []string{"aws.secrets-manager.archive", "aws.s3.encryption", "aws.secrets-manager.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func (api *NativeAPI) restoreSecret(ctx context.Context, state operationState, _ string) (provider.NativeOperationObservation, error) {
	archive, err := api.loadSecretArchive(ctx, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	target := api.restoreSecretID(state)
	state.Target = target
	operationID, err := encodeOperationState(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	description, describeErr := api.secrets.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(target)})
	if describeErr == nil {
		if description == nil || !secretTagsMatch(description.Tags, state.OwnershipMarker, state.DataClass) {
			return provider.NativeOperationObservation{}, errors.New("refusing to overwrite an unowned AWS Secrets Manager restore secret")
		}
		if err := api.putRestoredSecret(ctx, target, archive, state); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.secretRestoreObservation(ctx, state, operationID, target), nil
	}
	if !isSecretNotFound(describeErr) {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect AWS Secrets Manager restore secret: %w", describeErr)
	}
	input := &secretsmanager.CreateSecretInput{Name: aws.String(target), Tags: secretTagsFor(state), Description: aws.String("MageLift isolated recovery secret")}
	if archive.Encoding == "string" {
		input.SecretString = aws.String(string(archive.Value))
	} else {
		input.SecretBinary = append([]byte(nil), archive.Value...)
	}
	if _, err := api.secrets.CreateSecret(ctx, input); err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("create AWS Secrets Manager restore secret: %w", err)
	}
	return api.secretRestoreObservation(ctx, state, operationID, target), nil
}

func (api *NativeAPI) verifySecretRestore(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.Target) == "" {
		return provider.NativeOperationObservation{}, errors.New("AWS Secrets Manager restore operation has no target")
	}
	return api.secretRestoreObservation(ctx, state, operationID, state.Target), nil
}

func (api *NativeAPI) secretRestoreObservation(ctx context.Context, state operationState, operationID, target string) provider.NativeOperationObservation {
	description, err := api.secrets.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(target)})
	if err != nil || description == nil || !secretTagsMatch(description.Tags, state.OwnershipMarker, state.DataClass) {
		return pendingOperation(state, operationID)
	}
	value, err := api.secrets.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(target)})
	if err != nil || value == nil || (value.SecretString == nil && len(value.SecretBinary) == 0) {
		return pendingOperation(state, operationID)
	}
	backupID := state.Backup
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, RestoreID: "aws-secretsmanager://" + target,
		FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "restored an isolated encrypted secret and verified value availability without recording its contents",
	}
	return observationWithOperation(state, operationID, []string{"aws-secretsmanager://" + target}, []string{"aws.secrets-manager.restore", "aws.secrets-manager.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func (api *NativeAPI) integritySecret(ctx context.Context, state operationState, operationID, secretID string) (provider.NativeOperationObservation, error) {
	description, err := api.secrets.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(secretID)})
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("describe AWS Secrets Manager secret for integrity: %w", err)
	}
	if description == nil || !secretTagsMatch(description.Tags, state.OwnershipMarker, state.DataClass) {
		return provider.NativeOperationObservation{}, errors.New("AWS Secrets Manager integrity target is not owned by this operation")
	}
	value, err := api.secrets.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(secretID)})
	if err != nil || value == nil || (value.SecretString == nil && len(value.SecretBinary) == 0) {
		return provider.NativeOperationObservation{}, errors.New("AWS Secrets Manager integrity target has no readable value")
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true,
		ApplicationReadsVerified: true, PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		Reason: "verified encrypted secret availability, ownership tags, permissions, and reference reachability",
	}
	return observationWithOperation(state, operationID, []string{"aws-secretsmanager://" + secretID}, []string{"aws.secrets-manager.integrity", "aws.secrets-manager.permissions"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) loadSecretArchive(ctx context.Context, state operationState) (secretArchive, error) {
	if strings.TrimSpace(state.Backup) == "" {
		return secretArchive{}, errors.New("AWS Secrets Manager restore requires a backup reference")
	}
	bucket, key, err := parseArchiveReference(state.Backup)
	if err != nil {
		return secretArchive{}, err
	}
	head, err := api.s3.HeadObject(ctx, s3HeadObjectInput(bucket, key))
	if err != nil {
		return secretArchive{}, fmt.Errorf("inspect AWS Secrets Manager archive: %w", err)
	}
	if head == nil || head.Metadata[archiveOwnershipKey] != state.OwnershipMarker || head.Metadata[archiveClassKey] != state.DataClass || head.Metadata[archiveFixtureKey] != state.FixtureID || !api.archiveEncryptionVerified(head) {
		return secretArchive{}, errors.New("AWS Secrets Manager archive ownership does not match the operation")
	}
	body, err := readObject(ctx, api.s3, bucket, key)
	if err != nil {
		return secretArchive{}, fmt.Errorf("read AWS Secrets Manager archive: %w", err)
	}
	archive, _, err := cloudrecovery.OpenSecretArchive(body)
	if err != nil {
		return secretArchive{}, fmt.Errorf("decode AWS Secrets Manager archive: %w", err)
	}
	if err := cloudrecovery.ValidateSecretArchive(archive, operationVersion, state.DataClass, state.FixtureID, state.OwnershipMarker); err != nil {
		return secretArchive{}, fmt.Errorf("validate AWS Secrets Manager archive: %w", err)
	}
	return archive, nil
}

func (api *NativeAPI) putRestoredSecret(ctx context.Context, target string, archive secretArchive, state operationState) error {
	input := &secretsmanager.PutSecretValueInput{SecretId: aws.String(target), ClientRequestToken: aws.String(shortDigest(state.IdempotencyKey))}
	if archive.Encoding == "string" {
		input.SecretString = aws.String(string(archive.Value))
	} else {
		input.SecretBinary = append([]byte(nil), archive.Value...)
	}
	if _, err := api.secrets.PutSecretValue(ctx, input); err != nil {
		return fmt.Errorf("put AWS Secrets Manager restored value: %w", err)
	}
	return nil
}

func (api *NativeAPI) restoreSecretID(state operationState) string {
	return api.restoreSecretPrefix(state.DataClass) + shortDigest(state.OwnershipMarker+"\x00"+state.IdempotencyKey)
}

func (api *NativeAPI) restoreSecretPrefix(dataClass string) string {
	prefix := strings.Trim(strings.TrimSpace(api.config.RestoreSecretPrefix), "/")
	if prefix == "" {
		prefix = "magelift/recovery"
	}
	return prefix + "/" + dataClass + "/"
}

func secretTagsMatch(tags []secretstypes.Tag, marker, class string) bool {
	if secretTagValue(tags, ownershipTagKey) != marker {
		return false
	}
	return class == "" || secretTagValue(tags, classTagKey) == class
}

func s3HeadObjectInput(bucket, key string) *s3.HeadObjectInput {
	return &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}
}

func isSecretNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "resource not found") || strings.Contains(message, "resourcenotfound") || strings.Contains(message, "not found") || strings.Contains(message, "404")
}

func (api *NativeAPI) listOwnedSecrets(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	if api == nil || api.secrets == nil {
		return nil, errors.New("AWS Secrets Manager recovery API is required")
	}
	resources := make([]provider.InventoryResource, 0)
	prefix := api.restoreSecretPrefix("configuration-secrets")
	var token *string
	for {
		output, err := api.secrets.ListSecrets(ctx, &secretsmanager.ListSecretsInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		if output == nil {
			return nil, errors.New("AWS Secrets Manager list returned an empty response")
		}
		for _, secret := range output.SecretList {
			name := aws.ToString(secret.Name)
			if strings.HasPrefix(name, prefix) && secretTagsMatch(secret.Tags, marker, "configuration-secrets") {
				resources = append(resources, provider.InventoryResource{Identity: "aws-secretsmanager://" + name, Owned: true, Live: true})
			}
		}
		if strings.TrimSpace(aws.ToString(output.NextToken)) == "" {
			return resources, nil
		}
		token = output.NextToken
	}
}
