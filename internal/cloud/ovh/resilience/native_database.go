package resilience

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

const (
	ovhDatabaseNamePrefix    = "magelift-recovery-"
	ovhDatabaseRestorePrefix = "magelift-restore-"
)

// DatabaseInstance is the provider-local view of one OVHcloud Public Cloud
// Database service. Description is the only ownership marker exposed by the
// documented service model, so restore targets use a deterministic managed
// description and source ownership is explicitly configured by ID.
type DatabaseInstance struct {
	ID                  string
	Description         string
	Engine              string
	Version             string
	Region              string
	Status              string
	Plan                string
	Flavor              string
	NodeCount           int
	Encrypted           bool
	BackupRetentionDays int
	PITRAvailableFrom   time.Time
	DeletionProtection  bool
}

// DatabaseBackup is the normalized provider-managed backup view. OVHcloud
// lists backup IDs and exposes backup details; it does not expose a
// user-created snapshot operation for this service boundary.
type DatabaseBackup struct {
	ID            string
	InstanceID    string
	CreatedAt     time.Time
	Status        string
	Encrypted     bool
	Regions       []string
	RetentionDays int
	PointInTime   *time.Time
}

// DatabaseRestoreRequest contains only the documented fork inputs needed by
// the OVHcloud API. Provider SDK request models remain in native_database_sdk.go.
type DatabaseRestoreRequest struct {
	InstanceID     string
	Engine         string
	Description    string
	Region         string
	Plan           string
	Flavor         string
	Version        string
	DiskGB         int
	NodeCount      int
	BackupID       string
	PointInTime    *time.Time
	IPRestrictions []string
}

// DatabaseAPI is the narrow OVHcloud service port used by the recovery
// translator. It is intentionally injectable for deterministic fake-client
// tests and community-maintained API implementations.
type DatabaseAPI interface {
	GetInstance(context.Context, string, string) (DatabaseInstance, error)
	ListInstances(context.Context, string) ([]DatabaseInstance, error)
	DeleteInstance(context.Context, string, string) error
	ListBackups(context.Context, string, string) ([]DatabaseBackup, error)
	GetBackup(context.Context, string, string, string) (DatabaseBackup, error)
	CreateInstanceFromBackup(context.Context, DatabaseRestoreRequest) (DatabaseInstance, error)
}

type RecoveryVerification struct {
	ManifestVerified         bool
	CountsVerified           bool
	ApplicationReadsVerified bool
	PermissionsVerified      bool
	SecretReferencesVerified bool
	ServiceHealthVerified    bool
}

type RecoveryVerifier interface {
	Verify(context.Context, RecoveryVerificationRequest) (RecoveryVerification, error)
}

type RecoveryVerificationRequest struct {
	DataClass       string
	Resource        string
	FixtureID       string
	OwnershipMarker string
}

type ovhDatabaseReference struct {
	Engine     string
	InstanceID string
}

type ovhDatabaseBackupReference struct {
	InstanceID  string
	BackupID    string
	PointInTime *time.Time
}

func (api *NativeAPI) startDatabase(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api.database == nil {
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud Public Cloud Database API is not configured")
	}
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupDatabaseForState(ctx, state, operationID)
	}
	reference, err := parseOVHDatabaseReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	reference = api.normalizeDatabaseReference(reference)
	switch state.Action {
	case sdk.ResilienceBackup:
		if state.DataClass != "database" {
			return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud cache recovery is reconstructible from the source of truth and does not support durable backup")
		}
		if api.config.DatabaseUsePITR {
			return api.backupDatabasePITR(ctx, state, operationID, reference)
		}
		return api.backupDatabase(ctx, state, operationID, reference)
	case sdk.ResilienceRestore:
		return api.restoreDatabase(ctx, state, reference)
	case sdk.ResilienceIntegrityCheck:
		return api.integrityDatabase(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud Public Cloud Database recovery does not implement this action")
	}
}

func (api *NativeAPI) pollDatabase(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api.database == nil {
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud Public Cloud Database API is not configured")
	}
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupDatabaseForState(ctx, state, operationID)
	}
	reference, err := parseOVHDatabaseReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	reference = api.normalizeDatabaseReference(reference)
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.pollDatabaseBackup(ctx, state, operationID, reference)
	case sdk.ResilienceRestore:
		return api.pollDatabaseRestore(ctx, state, operationID)
	case sdk.ResilienceIntegrityCheck:
		return api.integrityDatabase(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud Public Cloud Database recovery does not implement this action")
	}
}

func (api *NativeAPI) backupDatabase(ctx context.Context, state operationState, operationID string, reference ovhDatabaseReference) (provider.NativeOperationObservation, error) {
	instance, err := api.database.GetInstance(ctx, reference.Engine, reference.InstanceID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud database source: %w", err)
	}
	if err := api.verifyDatabaseSource(instance, state, reference); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	backups, err := api.database.ListBackups(ctx, reference.Engine, instance.ID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("list OVHcloud database backups: %w", err)
	}
	if ovhDatabaseBackupsPending(backups) || api.ovhDatabaseBackupBoundaryPending(backups) {
		return ovhPending(state, operationID), nil
	}
	backup, err := api.selectDatabaseBackup(backups, instance, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	state.ResourceRef = backup.ID
	return ovhDatabaseBackupObservation(state, operationID, instance, backup), nil
}

func (api *NativeAPI) backupDatabasePITR(ctx context.Context, state operationState, operationID string, reference ovhDatabaseReference) (provider.NativeOperationObservation, error) {
	instance, err := api.database.GetInstance(ctx, reference.Engine, reference.InstanceID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud database source for PITR: %w", err)
	}
	if err := api.verifyDatabaseSource(instance, state, reference); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if !instance.Encrypted {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud database PITR encryption was not verified")
	}
	point := api.now().UTC().Add(-30 * time.Second).Truncate(time.Second)
	if boundary := api.config.DatabaseBackupNotBefore; !boundary.IsZero() && point.Before(boundary) {
		return ovhPending(state, operationID), nil
	}
	if settle := api.config.DatabasePITRSettleDuration; settle > 0 && !api.config.DatabaseBackupNotBefore.IsZero() && point.Before(api.config.DatabaseBackupNotBefore.Add(settle)) {
		return ovhPending(state, operationID), nil
	}
	if !instance.PITRAvailableFrom.IsZero() && point.Before(instance.PITRAvailableFrom) {
		return ovhPending(state, operationID), nil
	}
	if retention := instance.BackupRetentionDays; retention > 0 && point.Before(api.now().UTC().Add(-time.Duration(retention)*24*time.Hour)) {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud database PITR point is older than the configured retention window")
	}
	pitr := point
	backup := DatabaseBackup{
		ID: instance.ID, InstanceID: instance.ID, CreatedAt: point, Status: "READY", Encrypted: instance.Encrypted,
		Regions: []string{instance.Region}, RetentionDays: instance.BackupRetentionDays, PointInTime: &pitr,
	}
	return ovhDatabaseBackupObservation(state, operationID, instance, backup), nil
}

func (api *NativeAPI) pollDatabaseBackup(ctx context.Context, state operationState, operationID string, reference ovhDatabaseReference) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.ResourceRef) == "" {
		if api.config.DatabaseUsePITR {
			return api.backupDatabasePITR(ctx, state, operationID, reference)
		}
		return api.backupDatabase(ctx, state, operationID, reference)
	}
	instance, err := api.database.GetInstance(ctx, reference.Engine, reference.InstanceID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud database source for backup: %w", err)
	}
	if err := api.verifyDatabaseSource(instance, state, reference); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	backup, err := api.database.GetBackup(ctx, reference.Engine, instance.ID, state.ResourceRef)
	if err != nil {
		if isOVHNotFound(err) {
			return ovhPending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, fmt.Errorf("poll OVHcloud database backup: %w", err)
	}
	if !isOVHDatabaseReady(backup.Status) {
		if isOVHDatabaseError(backup.Status) {
			return ovhDatabaseFailed(state, operationID, "OVHcloud database backup entered a terminal state"), nil
		}
		return ovhPending(state, operationID), nil
	}
	if err := api.validateDatabaseBackup(backup, instance, state); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return ovhDatabaseBackupObservation(state, operationID, instance, backup), nil
}

func ovhDatabaseBackupsPending(backups []DatabaseBackup) bool {
	if len(backups) == 0 {
		return true
	}
	for _, backup := range backups {
		if !isOVHDatabaseReady(backup.Status) && !isOVHDatabaseError(backup.Status) {
			return true
		}
	}
	return false
}

func (api *NativeAPI) ovhDatabaseBackupBoundaryPending(backups []DatabaseBackup) bool {
	boundary := api.config.DatabaseBackupNotBefore
	if boundary.IsZero() || len(backups) == 0 {
		return false
	}
	for _, backup := range backups {
		if backup.CreatedAt.IsZero() || !backup.CreatedAt.Before(boundary) {
			return false
		}
	}
	return true
}

func (api *NativeAPI) selectDatabaseBackup(backups []DatabaseBackup, instance DatabaseInstance, state operationState) (DatabaseBackup, error) {
	valid := make([]DatabaseBackup, 0, len(backups))
	for _, backup := range backups {
		if err := api.validateDatabaseBackup(backup, instance, state); err == nil {
			valid = append(valid, backup)
		}
	}
	if len(valid) == 0 {
		return DatabaseBackup{}, ovhCapabilityError(state, "OVHcloud has no ready encrypted database backup within the configured age and ownership boundary")
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].CreatedAt.After(valid[j].CreatedAt) })
	return valid[0], nil
}

func (api *NativeAPI) validateDatabaseBackup(backup DatabaseBackup, instance DatabaseInstance, state operationState) error {
	if strings.TrimSpace(backup.ID) == "" || backup.InstanceID != instance.ID {
		return errors.New("OVHcloud database backup is not bound to the owned source service")
	}
	if !isOVHDatabaseReady(backup.Status) {
		return errors.New("OVHcloud database backup is not ready")
	}
	if !backup.Encrypted {
		return errors.New("OVHcloud database backup encryption was not verified")
	}
	if backup.CreatedAt.IsZero() {
		return errors.New("OVHcloud database backup has no creation timestamp")
	}
	if maxAge := api.config.MaxDatabaseBackupAge; maxAge > 0 && api.now().Sub(backup.CreatedAt) > maxAge {
		return errors.New("OVHcloud database backup is older than the configured maximum age")
	}
	if notBefore := api.config.DatabaseBackupNotBefore; !notBefore.IsZero() && backup.CreatedAt.Before(notBefore) {
		return errors.New("OVHcloud database backup was created before the recovery fixture boundary")
	}
	if backup.RetentionDays <= 0 {
		backup.RetentionDays = instance.BackupRetentionDays
	}
	if backup.RetentionDays <= 0 && state.DataClass == "database" {
		return errors.New("OVHcloud database backup retention was not exposed by the owning service")
	}
	return nil
}

func (api *NativeAPI) validateDatabasePITRPoint(point time.Time, instance DatabaseInstance, state operationState, reference ovhDatabaseReference) error {
	if point.IsZero() {
		return errors.New("OVHcloud database PITR point is required")
	}
	if reference.Engine != "" && !strings.EqualFold(reference.Engine, instance.Engine) {
		return errors.New("OVHcloud database PITR point is not bound to the requested engine")
	}
	if !instance.Encrypted {
		return errors.New("OVHcloud database PITR encryption was not verified")
	}
	if point.After(api.now().UTC().Add(time.Minute)) {
		return errors.New("OVHcloud database PITR point is in the future")
	}
	if boundary := api.config.DatabaseBackupNotBefore; !boundary.IsZero() && point.Before(boundary) {
		return errors.New("OVHcloud database PITR point was created before the recovery fixture boundary")
	}
	if !instance.PITRAvailableFrom.IsZero() && point.Before(instance.PITRAvailableFrom) {
		return errors.New("OVHcloud database PITR point is outside the available recovery window")
	}
	if settle := api.config.DatabasePITRSettleDuration; settle > 0 && !api.config.DatabaseBackupNotBefore.IsZero() && point.Before(api.config.DatabaseBackupNotBefore.Add(settle)) {
		return errors.New("OVHcloud database PITR point did not reach the fixture settle boundary")
	}
	if retention := instance.BackupRetentionDays; retention <= 0 && state.DataClass == "database" {
		return errors.New("OVHcloud database PITR retention was not exposed by the owning service")
	} else if retention > 0 && point.Before(api.now().UTC().Add(-time.Duration(retention)*24*time.Hour)) {
		return errors.New("OVHcloud database PITR point is older than the configured retention window")
	}
	return nil
}

func (api *NativeAPI) restoreDatabase(ctx context.Context, state operationState, reference ovhDatabaseReference) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.Backup) == "" {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud database restore requires a provider-managed backup reference")
	}
	backupReference, err := parseOVHDatabaseBackupReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if backupReference.InstanceID != reference.InstanceID {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud database backup does not belong to the requested source service")
	}
	reference = api.normalizeDatabaseReference(reference)
	source, err := api.database.GetInstance(ctx, reference.Engine, reference.InstanceID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud database restore source: %w", err)
	}
	if err := api.verifyDatabaseSource(source, state, reference); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	var backup DatabaseBackup
	if backupReference.PointInTime != nil {
		point := backupReference.PointInTime.UTC()
		if err := api.validateDatabasePITRPoint(point, source, state, reference); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		backup = DatabaseBackup{ID: source.ID, InstanceID: source.ID, CreatedAt: point, Status: "READY", Encrypted: source.Encrypted, Regions: []string{source.Region}, RetentionDays: source.BackupRetentionDays, PointInTime: &point}
	} else {
		backup, err = api.database.GetBackup(ctx, reference.Engine, source.ID, backupReference.BackupID)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud database restore backup: %w", err)
		}
		if err := api.validateDatabaseBackup(backup, source, state); err != nil {
			return provider.NativeOperationObservation{}, err
		}
	}
	targetRegion := strings.TrimSpace(api.config.RestoreDatabaseRegion)
	if targetRegion == "" {
		targetRegion = source.Region
	}
	if state.Destination == sdk.RecoveryAlternateRegion && targetRegion == source.Region {
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud alternate-region recovery requires an explicitly different restore region")
	}
	if targetRegion != source.Region && (backup.PointInTime != nil || !containsString(backup.Regions, targetRegion)) {
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud database backup does not list the requested restore region")
	}
	targetName := api.restoreDatabaseName(state)
	target, findErr := api.findDatabaseByDescription(ctx, targetName)
	if findErr == nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("OVHcloud database restore target %q already exists; ownership cannot be proven", target.ID)
	}
	if !isOVHNotFound(findErr) {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud database restore target: %w", findErr)
	}
	nodeCount := source.NodeCount
	if api.config.RequireDatabaseHA && nodeCount < 2 {
		nodeCount = 2
	}
	request := DatabaseRestoreRequest{
		InstanceID: source.ID, Engine: reference.Engine, Description: targetName, Region: targetRegion,
		Plan:    firstNonEmptyOVH(api.config.RestoreDatabasePlan, api.config.DatabasePlan, source.Plan),
		Flavor:  firstNonEmptyOVH(api.config.RestoreDatabaseFlavor, api.config.DatabaseFlavor, source.Flavor),
		Version: firstNonEmptyOVH(api.config.RestoreDatabaseVersion, api.config.DatabaseVersion, source.Version),
		DiskGB:  api.config.DatabaseDiskGB, NodeCount: nodeCount,
		IPRestrictions: append([]string(nil), api.config.DatabaseIPRestrictions...),
	}
	if backup.PointInTime != nil {
		request.PointInTime = cloneTimePointer(backup.PointInTime)
	} else {
		request.BackupID = backup.ID
	}
	target, err = api.database.CreateInstanceFromBackup(ctx, request)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("fork OVHcloud database from backup: %w", err)
	}
	if strings.TrimSpace(target.ID) == "" {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud database restore returned no service identity")
	}
	state.Target = target.ID
	if backup.PointInTime != nil {
		state.ResourceRef = state.Backup
	} else {
		state.ResourceRef = backup.ID
	}
	operationID, err := ovhDatabaseOperationID(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if isOVHDatabaseReady(target.Status) {
		return ovhDatabaseRestoreObservation(state, operationID, target), nil
	}
	return ovhPending(state, operationID), nil
}

func (api *NativeAPI) pollDatabaseRestore(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.Target) == "" {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud database restore operation has no target identity")
	}
	reference, err := parseOVHDatabaseReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	target, err := api.database.GetInstance(ctx, reference.Engine, state.Target)
	if err != nil {
		if isOVHNotFound(err) {
			return ovhPending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, fmt.Errorf("poll OVHcloud database restore target: %w", err)
	}
	if isOVHDatabaseError(target.Status) {
		return ovhDatabaseFailed(state, operationID, "OVHcloud database restore target entered a terminal state"), nil
	}
	if !isOVHDatabaseReady(target.Status) {
		return ovhPending(state, operationID), nil
	}
	if err := api.verifyDatabaseTarget(target, api.restoreDatabaseName(state)); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return ovhDatabaseRestoreObservation(state, operationID, target), nil
}

func (api *NativeAPI) integrityDatabase(ctx context.Context, state operationState, operationID string, reference ovhDatabaseReference) (provider.NativeOperationObservation, error) {
	if api.config.Verifier == nil {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud database integrity requires an application recovery verifier")
	}
	instance, err := api.database.GetInstance(ctx, reference.Engine, reference.InstanceID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect OVHcloud database for integrity: %w", err)
	}
	if err := api.verifyDatabaseIntegrityTarget(instance, state, reference); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	verification, err := api.config.Verifier.Verify(ctx, RecoveryVerificationRequest{DataClass: state.DataClass, Resource: state.Resource, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker})
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("verify OVHcloud database application fixture: %w", err)
	}
	if !verification.PermissionsVerified || !verification.ServiceHealthVerified {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud database integrity verifier did not prove permissions and service health")
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup, FixtureID: state.FixtureID,
		RetentionDays: instance.BackupRetentionDays, EncryptionVerified: instance.Encrypted,
		ManifestVerified: verification.ManifestVerified, CountsVerified: verification.CountsVerified,
		ApplicationReadsVerified: verification.ApplicationReadsVerified, PermissionsVerified: verification.PermissionsVerified,
		SecretReferencesVerified: verification.SecretReferencesVerified, ServiceHealthVerified: verification.ServiceHealthVerified,
		Reason: "verified the ownership-bound OVHcloud database service and application fixture without placing records or credentials in evidence",
	}
	return ovhObservation(state, operationID, []string{"ovh-database://" + reference.Engine + "/" + instance.ID}, []string{"ovh.database.integrity", "ovh.database.application"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) verifyDatabaseIntegrityTarget(instance DatabaseInstance, state operationState, reference ovhDatabaseReference) error {
	if strings.TrimSpace(api.config.DatabaseSourceID) == "" {
		return errors.New("OVHcloud database integrity requires the configured source service identity")
	}
	if instance.ID == api.config.DatabaseSourceID {
		return errors.New("OVHcloud database integrity reference resolved to the source service")
	}
	if reference.InstanceID != instance.ID {
		return errors.New("OVHcloud database integrity reference does not identify the restored service")
	}
	if reference.Engine != "" && instance.Engine != "" && !strings.EqualFold(reference.Engine, instance.Engine) {
		return errors.New("OVHcloud database integrity reference engine does not match the restored service")
	}
	if !isOVHDatabaseRestoreDescriptionOwned(instance.Description, state.OwnershipMarker) {
		return errors.New("OVHcloud database integrity target is not owned by the recovery marker")
	}
	if !isOVHDatabaseReady(instance.Status) {
		return fmt.Errorf("OVHcloud database integrity target %q is not ready", instance.ID)
	}
	if !instance.Encrypted {
		return errors.New("OVHcloud database integrity target encryption was not verified")
	}
	if api.config.RequireDatabaseHA && instance.NodeCount < 2 {
		return errors.New("OVHcloud database integrity target does not satisfy the configured high-availability boundary")
	}
	return nil
}

func (api *NativeAPI) verifyDatabaseSource(instance DatabaseInstance, state operationState, reference ovhDatabaseReference) error {
	if strings.TrimSpace(api.config.DatabaseSourceID) == "" || api.config.DatabaseSourceID != instance.ID {
		return errors.New("OVHcloud database source ownership requires the configured source service identity")
	}
	if reference.InstanceID != instance.ID {
		return errors.New("OVHcloud database reference does not identify the configured source service")
	}
	if reference.Engine != "" && instance.Engine != "" && !strings.EqualFold(reference.Engine, instance.Engine) {
		return errors.New("OVHcloud database reference engine does not match the source service")
	}
	if !isOVHDatabaseReady(instance.Status) {
		return fmt.Errorf("OVHcloud database source %q is not ready", instance.ID)
	}
	if !instance.Encrypted {
		return errors.New("OVHcloud database source encryption was not verified")
	}
	if api.config.RequireDatabaseHA && instance.NodeCount < 2 {
		return errors.New("OVHcloud database source does not satisfy the configured high-availability boundary")
	}
	return nil
}

func (api *NativeAPI) verifyDatabaseTarget(instance DatabaseInstance, description string) error {
	if instance.Description != description {
		return errors.New("OVHcloud database restore target is not owned by this operation")
	}
	if !isOVHDatabaseReady(instance.Status) {
		return fmt.Errorf("OVHcloud database restore target %q is not ready", instance.ID)
	}
	if !instance.Encrypted {
		return errors.New("OVHcloud database restore target encryption was not verified")
	}
	if api.config.RequireDatabaseHA && instance.NodeCount < 2 {
		return errors.New("OVHcloud database restore target does not satisfy the configured high-availability boundary")
	}
	return nil
}

func (api *NativeAPI) findDatabaseByDescription(ctx context.Context, description string) (DatabaseInstance, error) {
	instances, err := api.database.ListInstances(ctx, api.config.DatabaseEngine)
	if err != nil {
		return DatabaseInstance{}, err
	}
	var match DatabaseInstance
	for _, instance := range instances {
		if instance.Description == description {
			if match.ID != "" {
				return DatabaseInstance{}, fmt.Errorf("OVHcloud database restore target description %q is occupied by multiple services", description)
			}
			match = instance
		}
	}
	if match.ID != "" {
		return match, nil
	}
	return DatabaseInstance{}, errors.New("OVHcloud database restore target not found")
}

func (api *NativeAPI) normalizeDatabaseReference(reference ovhDatabaseReference) ovhDatabaseReference {
	reference.Engine = firstNonEmptyOVH(reference.Engine, api.config.DatabaseEngine)
	if strings.EqualFold(reference.Engine, "valkey") {
		reference.Engine = "redis"
	}
	return reference
}

func (api *NativeAPI) inventoryDatabase(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	instances, err := api.database.ListInstances(ctx, api.config.DatabaseEngine)
	if err != nil {
		return nil, fmt.Errorf("inventory OVHcloud Public Cloud Database services: %w", err)
	}
	resources := make([]provider.InventoryResource, 0, len(instances))
	for _, instance := range instances {
		if instance.ID == api.config.DatabaseSourceID || isOVHDatabaseRestoreDescriptionOwned(instance.Description, marker) {
			resources = append(resources, provider.InventoryResource{Identity: "ovh-database://" + instance.Engine + "/" + instance.ID, Owned: true, Live: true})
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func ovhDatabaseBackupObservation(state operationState, operationID string, instance DatabaseInstance, backup DatabaseBackup) provider.NativeOperationObservation {
	retention := backup.RetentionDays
	if retention <= 0 {
		retention = instance.BackupRetentionDays
	}
	backupID := "ovh-database-backup://" + instance.ID + "/" + backup.ID
	reason := "verified an OVHcloud provider-managed encrypted backup selected from the owning service"
	if backup.PointInTime != nil {
		backupID = ovhDatabasePITRReference(instance.ID, *backup.PointInTime)
		reason = "verified an OVHcloud provider-managed encrypted PITR point selected from the owning service"
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID,
		RetentionDays: retention, EncryptionVerified: backup.Encrypted, ProtectionVerified: instance.DeletionProtection,
		PermissionsVerified: true, ServiceHealthVerified: isOVHDatabaseReady(instance.Status),
		Reason: ovhDatabaseProtectionReason(instance.DeletionProtection, reason),
	}
	return ovhObservation(state, operationID, []string{backupID}, []string{"ovh.database.backup", "ovh.database.encryption", "ovh.database.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func ovhDatabaseRestoreObservation(state operationState, operationID string, instance DatabaseInstance) provider.NativeOperationObservation {
	restoreID := "ovh-database://" + instance.Engine + "/" + instance.ID
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup, RestoreID: restoreID, FixtureID: state.FixtureID,
		RetentionDays: instance.BackupRetentionDays, EncryptionVerified: instance.Encrypted, ProtectionVerified: instance.DeletionProtection,
		PermissionsVerified: true, ServiceHealthVerified: isOVHDatabaseReady(instance.Status),
		Reason: ovhDatabaseProtectionReason(instance.DeletionProtection, "verified an isolated ownership-described OVHcloud database service, encrypted restore target, and ready state"),
	}
	return ovhObservation(state, operationID, []string{restoreID}, []string{"ovh.database.restore", "ovh.database.health", "ovh.database.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func ovhDatabaseFailed(state operationState, operationID, reason string) provider.NativeOperationObservation {
	observation := ovhObservation(state, operationID, nil, []string{"ovh.database.failed"}, nil, string(sdk.ResilienceOperationFailed))
	observation.Detail = reason
	return observation
}

func ovhDatabaseOperationID(state operationState) (string, error) {
	return cloudrecovery.EncodeOperationID(ovhRecoveryOperationPrefix, state)
}

func (api *NativeAPI) restoreDatabaseName(state operationState) string {
	return ovhDatabaseRestoreDescriptionPrefix(state.OwnershipMarker) + shortDigest(state.FixtureID+"\x00"+state.IdempotencyKey)
}

func ovhDatabaseRestoreDescriptionPrefix(marker string) string {
	return ovhDatabaseRestorePrefix + shortDigest(marker) + "-"
}

func isOVHDatabaseRestoreDescriptionOwned(description, marker string) bool {
	prefix := ovhDatabaseRestoreDescriptionPrefix(marker)
	if !strings.HasPrefix(description, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(description, prefix)
	if len(suffix) != 24 {
		return false
	}
	for _, character := range suffix {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func parseOVHDatabaseReference(reference string) (ovhDatabaseReference, error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || (parsed.Scheme != "ovh-database" && parsed.Scheme != "ovh-db" && parsed.Scheme != "ovhcloud-database") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ovhDatabaseReference{}, fmt.Errorf("OVHcloud database reference %q must use ovh-database://engine/instance", reference)
	}
	parts := strings.Split(strings.Trim(parsed.Host+parsed.EscapedPath(), "/"), "/")
	for i := range parts {
		parts[i], err = url.PathUnescape(parts[i])
		if err != nil || strings.TrimSpace(parts[i]) == "" || strings.ContainsAny(parts[i], "\r\n\x00") {
			return ovhDatabaseReference{}, fmt.Errorf("OVHcloud database reference %q contains invalid identity data", reference)
		}
	}
	switch len(parts) {
	case 1:
		return ovhDatabaseReference{InstanceID: parts[0]}, nil
	case 2:
		return ovhDatabaseReference{Engine: parts[0], InstanceID: parts[1]}, nil
	default:
		return ovhDatabaseReference{}, fmt.Errorf("OVHcloud database reference %q must identify one instance", reference)
	}
}

func parseOVHDatabaseBackupReference(reference string) (ovhDatabaseBackupReference, error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ovhDatabaseBackupReference{}, fmt.Errorf("OVHcloud database backup reference %q is invalid", reference)
	}
	isPITR := parsed.Scheme == "ovh-database-pitr" || parsed.Scheme == "ovh-db-pitr" || parsed.Scheme == "ovhcloud-database-pitr"
	isBackup := parsed.Scheme == "ovh-database-backup" || parsed.Scheme == "ovh-db-backup" || parsed.Scheme == "ovhcloud-database-backup"
	if !isPITR && !isBackup {
		return ovhDatabaseBackupReference{}, fmt.Errorf("OVHcloud database backup reference %q must use ovh-database-backup://instance/backup or ovh-database-pitr://instance/nanoseconds", reference)
	}
	parts := strings.Split(strings.Trim(parsed.Host+parsed.EscapedPath(), "/"), "/")
	if len(parts) != 2 {
		return ovhDatabaseBackupReference{}, fmt.Errorf("OVHcloud database backup reference %q must identify one instance and restore point", reference)
	}
	for index := range parts {
		parts[index], err = url.PathUnescape(parts[index])
		if err != nil || strings.TrimSpace(parts[index]) == "" || strings.ContainsAny(parts[index], "\r\n\x00") {
			return ovhDatabaseBackupReference{}, fmt.Errorf("OVHcloud database backup reference %q contains invalid identity data", reference)
		}
	}
	if isBackup {
		return ovhDatabaseBackupReference{InstanceID: parts[0], BackupID: parts[1]}, nil
	}
	nanoseconds, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || nanoseconds <= 0 {
		return ovhDatabaseBackupReference{}, fmt.Errorf("OVHcloud database PITR reference %q contains an invalid point in time", reference)
	}
	point := time.Unix(0, nanoseconds).UTC()
	return ovhDatabaseBackupReference{InstanceID: parts[0], PointInTime: &point}, nil
}

func ovhDatabasePITRReference(instanceID string, point time.Time) string {
	return "ovh-database-pitr://" + url.PathEscape(instanceID) + "/" + strconv.FormatInt(point.UTC().UnixNano(), 10)
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := value.UTC()
	return &cloned
}

func ovhDatabaseProtectionReason(protected bool, prefix string) string {
	if protected {
		return prefix + "; deletion protection was verified on the owning service"
	}
	return prefix + "; deletion protection is exposed by the API and was left off so this cell can complete exact cleanup"
}

func isOVHDatabaseReady(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "READY")
}

func isOVHDatabaseError(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ERROR", "ERROR_INCONSISTENT_SPEC", "DELETING", "SHELVED":
		return true
	default:
		return false
	}
}

func firstNonEmptyOVH(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

var _ DatabaseAPI = (*ovhDatabaseSDK)(nil)
