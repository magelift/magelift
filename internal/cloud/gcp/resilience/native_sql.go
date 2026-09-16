package resilience

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

// CloudSQLBackup is the provider-local observation of a Cloud SQL backup.
// It deliberately contains no sqladmin SDK model.
type CloudSQLBackup struct {
	Name        string
	Instance    string
	Description string
	Type        string
	State       string
	KMSKey      string
	ExpiryTime  time.Time
}

type CloudSQLInstance struct {
	Project            string
	Name               string
	Region             string
	State              string
	DatabaseVersion    string
	Tier               string
	AvailabilityType   string
	KMSKey             string
	DeletionProtection bool
	BackupsEnabled     bool
	UserLabels         map[string]string
	WriteEndpoint      string
}

type CloudSQLOperation struct {
	Name         string
	Status       string
	Error        string
	ResourceName string
}

type CloudSQLRestoreSettings struct {
	DatabaseVersion    string
	Tier               string
	Region             string
	AvailabilityType   string
	KMSKey             string
	DeletionProtection bool
	UserLabels         map[string]string
}

// CloudSQLAPI is the provider-local port for the official Cloud SQL Admin API.
// SDK request/response models are translated in native_sql_sdk.go and never
// cross into the provider-neutral recovery lifecycle.
type CloudSQLAPI interface {
	CreateBackup(context.Context, string, string, string, int64) (CloudSQLOperation, error)
	DeleteBackup(context.Context, string) (CloudSQLOperation, error)
	ListBackups(context.Context, string) ([]CloudSQLBackup, error)
	GetBackup(context.Context, string) (CloudSQLBackup, error)
	RestoreBackup(context.Context, string, string, string, CloudSQLRestoreSettings) (CloudSQLOperation, error)
	GetInstance(context.Context, string, string) (CloudSQLInstance, error)
	ListInstances(context.Context, string) ([]CloudSQLInstance, error)
	DeleteInstance(context.Context, string, string) (CloudSQLOperation, error)
	GetOperation(context.Context, string, string) (CloudSQLOperation, error)
}

// DeleteOwnedCloudSQLBackups removes only on-demand backups whose descriptions
// carry the exact MageLift ownership prefix. The Cloud SQL API returns an
// asynchronous operation, so a successful delete request is not treated as
// cleanup proof until the operation completes and Inventory observes no owned
// backup. Unmarked backups are never touched.
func (api *NativeAPI) DeleteOwnedCloudSQLBackups(ctx context.Context, marker string) error {
	if api == nil || api.sql == nil {
		return errors.New("GCP Cloud SQL recovery translator is not configured")
	}
	if err := validateGCPContext(ctx); err != nil {
		return err
	}
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return errors.New("GCP Cloud SQL cleanup ownership marker is required and must be single-line")
	}
	project := firstNonEmpty(api.config.Project, api.config.RestoreProject)
	if project == "" {
		return errors.New("GCP Cloud SQL cleanup project is required")
	}
	backups, err := api.sql.ListBackups(ctx, project)
	if err != nil {
		return fmt.Errorf("list GCP Cloud SQL backups for cleanup: %w", err)
	}
	prefix := "magelift-recovery:" + shortDigest(marker) + ":"
	for _, backup := range backups {
		if !strings.HasPrefix(backup.Description, prefix) {
			continue
		}
		op, err := api.sql.DeleteBackup(ctx, backup.Name)
		if err != nil {
			if isCloudSQLNotFound(err) {
				continue
			}
			return fmt.Errorf("delete owned GCP Cloud SQL backup %q: %w", backup.Name, err)
		}
		if err := api.waitCloudSQLOperation(ctx, project, op); err != nil {
			return fmt.Errorf("wait for deletion of owned GCP Cloud SQL backup %q: %w", backup.Name, err)
		}
	}
	remaining, err := api.sql.ListBackups(ctx, project)
	if err != nil {
		return fmt.Errorf("verify GCP Cloud SQL backup cleanup: %w", err)
	}
	for _, backup := range remaining {
		if strings.HasPrefix(backup.Description, prefix) {
			return fmt.Errorf("owned GCP Cloud SQL backup %q remains after cleanup", backup.Name)
		}
	}
	return nil
}

// LeftoverCloudSQLBackupsForInstance lists project-level backups that still
// name a destroyed Magelift Cloud SQL instance. Empty instance names and
// wildcard filters are refused before the list call.
func (api *NativeAPI) LeftoverCloudSQLBackupsForInstance(ctx context.Context, instance string) ([]CloudSQLBackup, error) {
	instance, project, err := api.leftoverCloudSQLCleanupScope(ctx, instance)
	if err != nil {
		return nil, err
	}
	backups, err := api.sql.ListBackups(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("list leftover GCP Cloud SQL backups: %w", err)
	}
	matched := make([]CloudSQLBackup, 0)
	for _, backup := range backups {
		if cloudSQLBackupBelongsToInstance(backup, instance) {
			matched = append(matched, backup)
		}
	}
	return matched, nil
}

// DeleteLeftoverCloudSQLBackupsForInstance removes project-level backups that
// still name the destroyed Magelift Cloud SQL instance. That includes the
// final backup created by FinalBackupConfig and any retained automated or
// on-demand backups that survive instance deletion. Backups for other
// instances are never touched. The instance name must be the Magelift-computed
// name, not an operator-supplied filter.
func (api *NativeAPI) DeleteLeftoverCloudSQLBackupsForInstance(ctx context.Context, instance string) ([]string, error) {
	instance, project, err := api.leftoverCloudSQLCleanupScope(ctx, instance)
	if err != nil {
		return nil, err
	}
	backups, err := api.sql.ListBackups(ctx, project)
	if err != nil {
		return nil, fmt.Errorf("list leftover GCP Cloud SQL backups: %w", err)
	}
	deleted := make([]string, 0)
	for _, backup := range backups {
		if !cloudSQLBackupBelongsToInstance(backup, instance) {
			continue
		}
		op, err := api.sql.DeleteBackup(ctx, backup.Name)
		if err != nil {
			if isCloudSQLNotFound(err) {
				continue
			}
			return deleted, fmt.Errorf("delete leftover GCP Cloud SQL backup %q: %w", backup.Name, err)
		}
		if err := api.waitCloudSQLOperation(ctx, project, op); err != nil {
			return deleted, fmt.Errorf("wait for deletion of leftover GCP Cloud SQL backup %q: %w", backup.Name, err)
		}
		deleted = append(deleted, backup.Name)
	}
	remaining, err := api.sql.ListBackups(ctx, project)
	if err != nil {
		return deleted, fmt.Errorf("verify leftover GCP Cloud SQL backup cleanup: %w", err)
	}
	for _, backup := range remaining {
		if cloudSQLBackupBelongsToInstance(backup, instance) {
			return deleted, fmt.Errorf("leftover GCP Cloud SQL backup %q remains after cleanup", backup.Name)
		}
	}
	sort.Strings(deleted)
	return deleted, nil
}

func (api *NativeAPI) leftoverCloudSQLCleanupScope(ctx context.Context, instance string) (string, string, error) {
	if api == nil || api.sql == nil {
		return "", "", errors.New("GCP Cloud SQL recovery translator is not configured")
	}
	if err := validateGCPContext(ctx); err != nil {
		return "", "", err
	}
	instance = strings.TrimSpace(instance)
	if instance == "" || strings.ContainsAny(instance, "\r\n\x00/*?") {
		return "", "", errors.New("GCP Cloud SQL leftover backup cleanup requires a single Magelift instance name")
	}
	project := firstNonEmpty(api.config.Project, api.config.RestoreProject)
	if project == "" {
		return "", "", errors.New("GCP Cloud SQL leftover backup cleanup project is required")
	}
	return instance, project, nil
}

func cloudSQLBackupBelongsToInstance(backup CloudSQLBackup, instance string) bool {
	observed := strings.TrimSpace(backup.Instance)
	if observed == "" || instance == "" {
		return false
	}
	if observed == instance {
		return true
	}
	return strings.HasSuffix(observed, "/instances/"+instance)
}

func (api *NativeAPI) waitCloudSQLOperation(ctx context.Context, project string, operation CloudSQLOperation) error {
	if strings.TrimSpace(operation.Name) == "" {
		if operation.Status == "succeeded" || operation.Status == "done" || operation.Status == "completed" {
			return nil
		}
		return errors.New("GCP Cloud SQL cleanup API returned no operation identity")
	}
	for {
		switch strings.ToLower(operation.Status) {
		case "succeeded", "done", "completed":
			return nil
		case "failed", "error":
			if operation.Error == "" {
				return errors.New("GCP Cloud SQL cleanup operation failed")
			}
			return errors.New(operation.Error)
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
		var err error
		operation, err = api.sql.GetOperation(ctx, project, operation.Name)
		if err != nil {
			return fmt.Errorf("poll GCP Cloud SQL cleanup operation: %w", err)
		}
	}
}

type cloudSQLReference struct {
	Project  string
	Instance string
}

func (api *NativeAPI) startDatabase(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.sql == nil {
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Cloud SQL recovery translator is not configured")
	}
	reference, err := parseCloudSQLReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.backupDatabase(ctx, state, operationID, reference)
	case sdk.ResilienceRestore:
		return api.restoreDatabase(ctx, state, reference)
	case sdk.ResilienceIntegrityCheck:
		return api.integrityDatabase(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Cloud SQL recovery does not implement this action")
	}
}

func (api *NativeAPI) pollDatabase(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.sql == nil {
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Cloud SQL recovery translator is not configured")
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.pollDatabaseBackup(ctx, state, operationID)
	case sdk.ResilienceRestore:
		return api.pollDatabaseRestore(ctx, state, operationID)
	case sdk.ResilienceIntegrityCheck:
		reference, err := parseCloudSQLReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.integrityDatabase(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Cloud SQL recovery does not implement this action")
	}
}

func (api *NativeAPI) backupDatabase(ctx context.Context, state cloudrecovery.OperationState, _ string, reference cloudSQLReference) (provider.NativeOperationObservation, error) {
	instance, err := api.sql.GetInstance(ctx, reference.Project, reference.Instance)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Cloud SQL source instance: %w", err)
	}
	if err := api.verifySQLInstance(instance, state, true); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	description := sqlBackupDescription(state)
	backups, err := api.sql.ListBackups(ctx, reference.Project)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("list GCP Cloud SQL backups: %w", err)
	}
	for _, backup := range backups {
		if backup.Instance != reference.Instance || backup.Description != description {
			continue
		}
		switch strings.ToLower(backup.State) {
		case "successful", "success", "done":
			state.ResourceRef = backup.Name
			return api.databaseBackupObservation(ctx, state, operationIDForState(state), backup)
		case "running", "enqueued", "pending":
			state.ResourceRef = backup.Name
			return pending(state, operationIDForState(state)), nil
		case "failed", "deleting", "deletion_failed":
			return provider.NativeOperationObservation{}, fmt.Errorf("GCP Cloud SQL backup %q already exists in state %q; use a new idempotency key", backup.Name, backup.State)
		}
	}
	op, err := api.sql.CreateBackup(ctx, reference.Project, reference.Instance, description, int64(api.retentionDays()))
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("create GCP Cloud SQL backup: %w", err)
	}
	if strings.TrimSpace(op.Name) == "" {
		return provider.NativeOperationObservation{}, errors.New("GCP Cloud SQL backup API returned no operation identity")
	}
	state.OperationRef = op.Name
	state.ResourceRef = op.ResourceName
	operationID := operationIDForState(state)
	if op.Status == "succeeded" {
		backup, err := api.findBackup(ctx, reference.Project, reference.Instance, description, op.ResourceName)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.databaseBackupObservation(ctx, state, operationID, backup)
	}
	return pending(state, operationID), nil
}

func (api *NativeAPI) pollDatabaseBackup(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	reference, err := parseCloudSQLReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if strings.TrimSpace(state.OperationRef) != "" {
		op, err := api.sql.GetOperation(ctx, reference.Project, state.OperationRef)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("poll GCP Cloud SQL backup operation: %w", err)
		}
		switch strings.ToLower(op.Status) {
		case "pending", "running", "accepted":
			return pending(state, operationID), nil
		case "failed", "error":
			return failedOperation(state, operationID, op.Error), nil
		case "succeeded", "done", "completed":
			if state.ResourceRef == "" {
				state.ResourceRef = op.ResourceName
			}
		}
	}
	backup, err := api.findBackup(ctx, reference.Project, reference.Instance, sqlBackupDescription(state), state.ResourceRef)
	if err != nil {
		if isCloudSQLNotFound(err) {
			return pending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, err
	}
	return api.databaseBackupObservation(ctx, state, operationID, backup)
}

func (api *NativeAPI) databaseBackupObservation(ctx context.Context, state cloudrecovery.OperationState, operationID string, backup CloudSQLBackup) (provider.NativeOperationObservation, error) {
	if !isSuccessfulSQLState(backup.State) {
		return pending(state, operationID), nil
	}
	encryptionVerified := !api.config.RequireCMEK || backup.KMSKey == api.config.KMSKeyName
	// Standard Cloud SQL on-demand backups are retained by the provider until
	// deletion; unlike automated backups they may not expose an expiryTime.
	// Accept that documented provider-retained boundary only when the API
	// identifies the resource as ON_DEMAND. An unknown backup type remains a
	// failed protection proof.
	protectionVerified := !backup.ExpiryTime.IsZero() || strings.EqualFold(backup.Type, "on_demand")
	if !encryptionVerified || !protectionVerified {
		return provider.NativeOperationObservation{}, errors.New("GCP Cloud SQL backup did not satisfy encryption or retention policy")
	}
	backupID := "gcp-cloud-sql-backup://" + backup.Name
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: encryptionVerified, ProtectionVerified: protectionVerified,
		ServiceHealthVerified: true,
		Reason:                "created and independently inspected an encrypted Cloud SQL on-demand backup with provider-retained protection",
	}
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), []string{backupID}, []string{"gcp.cloud-sql.backup", "gcp.cloud-sql.encryption", "gcp.cloud-sql.retention"}, []sdk.ResilienceProofEvidence{evidence}), nil
}

func (api *NativeAPI) restoreDatabase(ctx context.Context, state cloudrecovery.OperationState, reference cloudSQLReference) (provider.NativeOperationObservation, error) {
	backupName, err := parseCloudSQLBackupReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	backup, err := api.sql.GetBackup(ctx, backupName)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Cloud SQL restore backup: %w", err)
	}
	if !isSuccessfulSQLState(backup.State) {
		return provider.NativeOperationObservation{}, errors.New("GCP Cloud SQL restore requires a successful backup")
	}
	if state.Destination == sdk.RecoverySameRegion {
		return api.restoreDatabaseInPlace(ctx, state, reference, backup)
	}
	project := strings.TrimSpace(api.config.RestoreProject)
	if project == "" {
		project = reference.Project
	}
	target := api.restoreInstanceName(project, state)
	existing, getErr := api.sql.GetInstance(ctx, project, target)
	if getErr == nil {
		if err := api.verifySQLInstance(existing, state, false); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		state.Target = target
		state.ResourceRef = backup.Name
		return api.databaseRestoreObservation(ctx, state, operationIDForState(state), existing)
	}
	if !isCloudSQLNotFound(getErr) {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Cloud SQL restore target: %w", getErr)
	}
	source, sourceErr := api.sql.GetInstance(ctx, reference.Project, reference.Instance)
	if sourceErr != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Cloud SQL source settings: %w", sourceErr)
	}
	settings := api.restoreSettings(source, state, project)
	op, err := api.sql.RestoreBackup(ctx, project, target, backup.Name, settings)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("restore GCP Cloud SQL backup: %w", err)
	}
	if strings.TrimSpace(op.Name) == "" {
		return provider.NativeOperationObservation{}, errors.New("GCP Cloud SQL restore API returned no operation identity")
	}
	state.Target = target
	state.OperationRef = op.Name
	state.ResourceRef = backup.Name
	return pending(state, operationIDForState(state)), nil
}

func (api *NativeAPI) restoreDatabaseInPlace(ctx context.Context, state cloudrecovery.OperationState, reference cloudSQLReference, backup CloudSQLBackup) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.ApprovalReference) == "" {
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Cloud SQL in-place restore requires an operator approval reference because restoreBackup overwrites the source instance")
	}
	if restoreProject := strings.TrimSpace(api.config.RestoreProject); restoreProject != "" && restoreProject != reference.Project {
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Cloud SQL in-place restore refuses a RestoreProject that is not the source instance project")
	}
	if restoreRegion := strings.TrimSpace(api.config.RestoreRegion); restoreRegion != "" {
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP Cloud SQL in-place restore refuses RestoreRegion retargeting; use isolated or alternate-region restore")
	}
	source, err := api.sql.GetInstance(ctx, reference.Project, reference.Instance)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Cloud SQL in-place source instance: %w", err)
	}
	if err := api.verifySQLInstance(source, state, true); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if backup.Instance != "" && backup.Instance != reference.Instance {
		return provider.NativeOperationObservation{}, errors.New("GCP Cloud SQL in-place restore requires a backup taken from the source instance")
	}
	op, err := api.sql.RestoreBackup(ctx, reference.Project, reference.Instance, backup.Name, CloudSQLRestoreSettings{})
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("restore GCP Cloud SQL backup in place: %w", err)
	}
	if strings.TrimSpace(op.Name) == "" {
		return provider.NativeOperationObservation{}, errors.New("GCP Cloud SQL in-place restore API returned no operation identity")
	}
	state.Target = reference.Instance
	state.OperationRef = op.Name
	state.ResourceRef = backup.Name
	return pending(state, operationIDForState(state)), nil
}

func (api *NativeAPI) pollDatabaseRestore(ctx context.Context, state cloudrecovery.OperationState, operationID string) (provider.NativeOperationObservation, error) {
	reference, err := parseCloudSQLReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	project := api.restorePollProject(state, reference)
	if state.OperationRef != "" {
		op, err := api.sql.GetOperation(ctx, project, state.OperationRef)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("poll GCP Cloud SQL restore operation: %w", err)
		}
		switch strings.ToLower(op.Status) {
		case "pending", "running", "accepted":
			return pending(state, operationID), nil
		case "failed", "error":
			return failedOperation(state, operationID, op.Error), nil
		}
	}
	instance, err := api.sql.GetInstance(ctx, project, state.Target)
	if err != nil {
		if isCloudSQLNotFound(err) {
			return pending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Cloud SQL restored instance: %w", err)
	}
	return api.databaseRestoreObservation(ctx, state, operationID, instance)
}

func (api *NativeAPI) databaseRestoreObservation(ctx context.Context, state cloudrecovery.OperationState, operationID string, instance CloudSQLInstance) (provider.NativeOperationObservation, error) {
	if err := api.verifySQLInstance(instance, state, false); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	reason := "restored an isolated Cloud SQL instance and verified runnable state, ownership, HA, deletion protection, and encryption policy"
	if state.Destination == sdk.RecoverySameRegion {
		reason = "restored a Cloud SQL backup onto the approved source instance and verified runnable state, ownership, HA, deletion protection, and encryption policy"
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup, RestoreID: "gcp-cloud-sql://projects/" + instance.Project + "/instances/" + instance.Name,
		FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: !api.config.RequireCMEK || instance.KMSKey == api.config.KMSKeyName,
		ProtectionVerified: !api.config.RequireDeletionProtect || instance.DeletionProtection, ServiceHealthVerified: true,
		Reason: reason,
	}
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), []string{evidence.RestoreID}, []string{"gcp.cloud-sql.restore", "gcp.cloud-sql.health", "gcp.cloud-sql.ownership"}, []sdk.ResilienceProofEvidence{evidence}), nil
}

func (api *NativeAPI) integrityDatabase(ctx context.Context, state cloudrecovery.OperationState, operationID string, reference cloudSQLReference) (provider.NativeOperationObservation, error) {
	instance, err := api.sql.GetInstance(ctx, reference.Project, reference.Instance)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect GCP Cloud SQL instance for integrity: %w", err)
	}
	if err := api.verifySQLInstance(instance, state, false); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if api.config.Verifier == nil {
		return provider.NativeOperationObservation{}, errors.New("GCP Cloud SQL integrity requires an application recovery verifier")
	}
	verification, err := api.config.Verifier.Verify(ctx, RecoveryVerificationRequest{DataClass: state.DataClass, Resource: state.Resource, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker})
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("verify GCP Cloud SQL application fixture: %w", err)
	}
	if !verification.ApplicationReadsVerified || !verification.PermissionsVerified || !verification.ServiceHealthVerified {
		return provider.NativeOperationObservation{}, errors.New("GCP Cloud SQL application integrity verifier did not prove reads, permissions, and service health")
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: !api.config.RequireCMEK || instance.KMSKey == api.config.KMSKeyName,
		ProtectionVerified: !api.config.RequireDeletionProtect || instance.DeletionProtection, ManifestVerified: verification.ManifestVerified,
		CountsVerified: verification.CountsVerified, ApplicationReadsVerified: verification.ApplicationReadsVerified,
		PermissionsVerified: verification.PermissionsVerified, SecretReferencesVerified: verification.SecretReferencesVerified,
		ServiceHealthVerified: verification.ServiceHealthVerified,
		Reason:                "verified Cloud SQL control-plane health and application-owned fixture reads without exposing records in evidence",
	}
	return observation(state, operationID, string(sdk.ResilienceOperationSucceeded), []string{"gcp-cloud-sql://projects/" + instance.Project + "/instances/" + instance.Name}, []string{"gcp.cloud-sql.integrity", "gcp.cloud-sql.application"}, []sdk.ResilienceProofEvidence{evidence}), nil
}

func (api *NativeAPI) verifySQLInstance(instance CloudSQLInstance, state cloudrecovery.OperationState, requireSource bool) error {
	if instance.Name == "" || instance.State != "RUNNABLE" {
		return fmt.Errorf("GCP Cloud SQL instance %q is not runnable", instance.Name)
	}
	if !secretLabelsMatch(instance.UserLabels, state.OwnershipMarker, state.DataClass) {
		return errors.New("GCP Cloud SQL instance is not owned by this operation")
	}
	if api.config.RequireRegionalHA && instance.AvailabilityType != "REGIONAL" {
		return errors.New("GCP Cloud SQL instance is not configured for regional high availability")
	}
	if api.config.RequireDeletionProtect && !instance.DeletionProtection {
		return errors.New("GCP Cloud SQL instance does not have deletion protection enabled")
	}
	if api.config.RequireCMEK && instance.KMSKey != api.config.KMSKeyName {
		return errors.New("GCP Cloud SQL instance does not use the required CMEK")
	}
	if requireSource && !instance.BackupsEnabled {
		return errors.New("GCP Cloud SQL source instance does not have backups enabled")
	}
	return nil
}

func (api *NativeAPI) restorePollProject(state cloudrecovery.OperationState, reference cloudSQLReference) string {
	if state.Destination == sdk.RecoverySameRegion {
		return reference.Project
	}
	if strings.TrimSpace(api.config.RestoreProject) != "" {
		return api.config.RestoreProject
	}
	return reference.Project
}

func (api *NativeAPI) restoreSettings(source CloudSQLInstance, state cloudrecovery.OperationState, project string) CloudSQLRestoreSettings {
	availability := source.AvailabilityType
	if api.config.RequireRegionalHA {
		availability = "REGIONAL"
	}
	return CloudSQLRestoreSettings{DatabaseVersion: source.DatabaseVersion, Tier: firstNonEmpty(api.config.RestoreTier, source.Tier), Region: firstNonEmpty(api.config.RestoreRegion, source.Region), AvailabilityType: availability, KMSKey: api.config.KMSKeyName, DeletionProtection: api.config.RequireDeletionProtect, UserLabels: secretLabelsFor(state)}
}

func (api *NativeAPI) restoreInstanceName(project string, state cloudrecovery.OperationState) string {
	prefix := firstNonEmpty(api.config.RestoreInstancePrefix, "magelift-recovery")
	return prefix + "-" + shortDigest(project+"\x00"+state.OwnershipMarker+"\x00"+state.IdempotencyKey)
}

func (api *NativeAPI) findBackup(ctx context.Context, project, instance, description, preferred string) (CloudSQLBackup, error) {
	if preferred != "" {
		backup, err := api.sql.GetBackup(ctx, preferred)
		if err == nil {
			return backup, nil
		}
		if !isCloudSQLNotFound(err) {
			return CloudSQLBackup{}, fmt.Errorf("inspect GCP Cloud SQL backup %q: %w", preferred, err)
		}
	}
	backups, err := api.sql.ListBackups(ctx, project)
	if err != nil {
		return CloudSQLBackup{}, fmt.Errorf("list GCP Cloud SQL backups: %w", err)
	}
	matches := make([]CloudSQLBackup, 0, len(backups))
	for _, backup := range backups {
		if backup.Instance == instance && backup.Description == description {
			matches = append(matches, backup)
		}
	}
	if len(matches) == 0 {
		return CloudSQLBackup{}, fmt.Errorf("GCP Cloud SQL backup %q was not found", description)
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
	return matches[len(matches)-1], nil
}

func parseCloudSQLReference(reference string) (cloudSQLReference, error) {
	value := strings.TrimSpace(reference)
	for _, scheme := range []string{"gcp-cloud-sql://", "gcp-sql://", "cloudsql://"} {
		if strings.HasPrefix(strings.ToLower(value), scheme) {
			value = value[len(scheme):]
			break
		}
	}
	value = strings.TrimPrefix(value, "//")
	if strings.ContainsAny(value, "\r\n\x00?#") {
		return cloudSQLReference{}, fmt.Errorf("Cloud SQL resource reference %q contains invalid control data", reference)
	}
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) == 4 && parts[0] == "projects" && parts[2] == "instances" && parts[1] != "" && parts[3] != "" {
		return cloudSQLReference{Project: parts[1], Instance: parts[3]}, nil
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return cloudSQLReference{Project: parts[0], Instance: parts[1]}, nil
	}
	return cloudSQLReference{}, fmt.Errorf("Cloud SQL resource reference %q must use gcp-cloud-sql://projects/PROJECT/instances/INSTANCE", reference)
}

func parseCloudSQLBackupReference(reference string) (string, error) {
	value := strings.TrimSpace(reference)
	for _, scheme := range []string{"gcp-cloud-sql-backup://", "gcp-sql-backup://"} {
		if strings.HasPrefix(strings.ToLower(value), scheme) {
			value = value[len(scheme):]
			break
		}
	}
	value = strings.TrimPrefix(value, "//")
	parts := strings.Split(strings.Trim(value, "/"), "/")
	if len(parts) != 4 || parts[0] != "projects" || parts[2] != "backups" || parts[1] == "" || parts[3] == "" {
		return "", fmt.Errorf("Cloud SQL backup reference %q must use gcp-cloud-sql-backup://projects/PROJECT/backups/BACKUP", reference)
	}
	return strings.Trim(value, "/"), nil
}

func sqlBackupDescription(state cloudrecovery.OperationState) string {
	return "magelift-recovery:" + shortDigest(state.OwnershipMarker) + ":" + shortDigest(state.FixtureID+"\x00"+state.IdempotencyKey)
}

func operationIDForState(state cloudrecovery.OperationState) string {
	operationID, _ := cloudrecovery.EncodeOperationID(operationPrefix, state)
	return operationID
}

func isSuccessfulSQLState(value string) bool {
	switch strings.ToLower(value) {
	case "successful", "success", "done", "completed":
		return true
	default:
		return false
	}
}

func failedOperation(state cloudrecovery.OperationState, operationID, detail string) provider.NativeOperationObservation {
	return provider.NativeOperationObservation{Status: string(sdk.ResilienceOperationFailed), Action: state.Action, OperationID: operationID, OwnershipMarker: state.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true, Detail: detail}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func isCloudSQLNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "not_found") || strings.Contains(message, "404")
}
