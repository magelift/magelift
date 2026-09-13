package resilience

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	scalewayDatabaseOwnershipTag  = "magelift.io/ownership"
	scalewayDatabaseClassTag      = "magelift.io/data-class"
	scalewayRecoveryOutputTag     = "magelift.io/recovery-output=true"
	scalewayDatabaseNamePrefix    = "magelift-recovery-"
	scalewayDatabaseRestorePrefix = "magelift-restore-"
	databaseMutatingPostTime      = 2 * time.Minute
	databaseRestoreReconcileTime  = 90 * time.Second
	databaseRestoreReconcileEvery = 5 * time.Second
)

var errScalewayDatabaseInstanceNotFound = errors.New("Scaleway Managed Database restore target not found")
var errScalewayDatabaseSnapshotNotFound = errors.New("Scaleway Managed Database snapshot not found")

// DatabaseInstance is the provider-local view of a Scaleway Managed Database
// instance. RDB SDK request and response models stop at native_database_sdk.go.
type DatabaseInstance struct {
	ID                string
	Name              string
	Region            string
	Status            string
	NodeType          string
	VolumeType        string
	IsHA              bool
	EncryptionEnabled bool
	Tags              []string
}

// DatabaseSnapshot is the provider-local view of a Scaleway RDB block
// snapshot. Scaleway does not expose snapshot tags, so ownership is bound to
// its deterministic name and the ownership-tagged source instance.
type DatabaseSnapshot struct {
	ID         string
	InstanceID string
	Name       string
	Region     string
	Status     string
	NodeType   string
	VolumeType string
	CreatedAt  time.Time
	ExpiresAt  time.Time
}

// DatabaseAPI is the narrow provider-local port used by the recovery
// translator. Community implementations can wrap another Scaleway SDK
// version without leaking its models into the core lifecycle.
type DatabaseAPI interface {
	GetInstance(context.Context, string) (DatabaseInstance, error)
	ListInstances(context.Context) ([]DatabaseInstance, error)
	DeleteInstance(context.Context, string) error
	CreateSnapshot(context.Context, string, string, time.Time) (DatabaseSnapshot, error)
	GetSnapshot(context.Context, string) (DatabaseSnapshot, error)
	ListSnapshots(context.Context) ([]DatabaseSnapshot, error)
	DeleteSnapshot(context.Context, string) error
	CreateInstanceFromSnapshot(context.Context, string, string, string, bool) (DatabaseInstance, error)
	UpdateInstanceTags(context.Context, string, []string) (DatabaseInstance, error)
}

// RecoveryVerification is the application-owned half of a database
// integrity proof. The RDB control plane can prove instance health and policy;
// only the workload can prove known fixture reads and permissions.
type RecoveryVerification struct {
	ManifestVerified         bool
	CountsVerified           bool
	ApplicationReadsVerified bool
	PermissionsVerified      bool
	SecretReferencesVerified bool
	ServiceHealthVerified    bool
}

// RecoveryVerifier is required for database integrity checks. It receives
// opaque references and fixture identities, never restored records or secret
// values.
type RecoveryVerifier interface {
	Verify(context.Context, RecoveryVerificationRequest) (RecoveryVerification, error)
}

type RecoveryVerificationRequest struct {
	DataClass       string
	Resource        string
	FixtureID       string
	OwnershipMarker string
}

type scalewayDatabaseReference struct {
	InstanceID string
}

func (api *NativeAPI) startDatabase(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.database == nil {
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway Managed Database recovery translator is not configured")
	}
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupDatabaseForState(ctx, state, operationID)
	}
	reference, err := parseScalewayDatabaseReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.backupDatabase(ctx, state, reference)
	case sdk.ResilienceRestore:
		return api.restoreDatabase(ctx, state, reference)
	case sdk.ResilienceIntegrityCheck:
		return api.integrityDatabase(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway Managed Database recovery does not implement this action")
	}
}

func (api *NativeAPI) pollDatabase(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if api == nil || api.database == nil {
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway Managed Database recovery translator is not configured")
	}
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupDatabaseForState(ctx, state, operationID)
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return api.pollDatabaseBackup(ctx, state, operationID)
	case sdk.ResilienceRestore:
		return api.pollDatabaseRestore(ctx, state, operationID)
	case sdk.ResilienceIntegrityCheck:
		reference, err := parseScalewayDatabaseReference(state.Resource)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.integrityDatabase(ctx, state, operationID, reference)
	default:
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway Managed Database recovery does not implement this action")
	}
}

func (api *NativeAPI) backupDatabase(ctx context.Context, state operationState, reference scalewayDatabaseReference) (provider.NativeOperationObservation, error) {
	instance, err := api.database.GetInstance(ctx, reference.InstanceID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect Scaleway Managed Database source instance: %w", err)
	}
	if err := api.verifyDatabaseInstance(instance, state, true); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	snapshotName := scalewayDatabaseSnapshotName(state)
	snapshots, err := api.database.ListSnapshots(ctx)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("list Scaleway Managed Database snapshots: %w", err)
	}
	for _, snapshot := range snapshots {
		if snapshot.InstanceID != instance.ID || snapshot.Name != snapshotName {
			continue
		}
		state.ResourceRef = snapshot.ID
		operationID, err := scalewayDatabaseOperationID(state)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		switch normalizeScalewayStatus(snapshot.Status) {
		case "ready":
			return api.databaseBackupObservation(state, operationID, instance, snapshot)
		case "creating", "restoring", "locked":
			return scalewayDatabasePending(state, operationID), nil
		case "error", "deleting":
			return scalewayDatabaseFailed(state, operationID, "Scaleway Managed Database snapshot exists in terminal state "+snapshot.Status), nil
		default:
			return scalewayDatabasePending(state, operationID), nil
		}
	}

	expiresAt := api.now().Add(time.Duration(api.retentionDays()) * 24 * time.Hour)
	createCtx, cancel := context.WithTimeout(ctx, databaseMutatingPostTime)
	snapshot, err := api.database.CreateSnapshot(createCtx, instance.ID, snapshotName, expiresAt)
	cancel()
	if err != nil {
		createErr := err
		// CreateSnapshot is a mutating POST. Scaleway can accept and materialize
		// the snapshot while the HTTP call stays open; a canceled or timed-out
		// client must recover the deterministic name instead of leaking it.
		snapshot, err = api.reconcileDatabaseSnapshot(ctx, instance.ID, snapshotName)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("create Scaleway Managed Database snapshot: %w (reconcile snapshot: %v)", createErr, err)
		}
	}
	if strings.TrimSpace(snapshot.ID) == "" {
		return provider.NativeOperationObservation{}, errors.New("Scaleway Managed Database snapshot API returned no snapshot identity")
	}
	state.ResourceRef = snapshot.ID
	operationID, err := scalewayDatabaseOperationID(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if normalizeScalewayStatus(snapshot.Status) == "ready" {
		return api.databaseBackupObservation(state, operationID, instance, snapshot)
	}
	return scalewayDatabasePending(state, operationID), nil
}

func (api *NativeAPI) pollDatabaseBackup(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.ResourceRef) == "" {
		return provider.NativeOperationObservation{}, errors.New("Scaleway Managed Database backup operation has no snapshot identity")
	}
	snapshot, err := api.database.GetSnapshot(ctx, state.ResourceRef)
	if err != nil {
		if isScalewayNotFound(err) {
			return scalewayDatabasePending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, fmt.Errorf("poll Scaleway Managed Database snapshot: %w", err)
	}
	reference, err := parseScalewayDatabaseReference(state.Resource)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	instance, err := api.database.GetInstance(ctx, reference.InstanceID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect Scaleway Managed Database source for snapshot: %w", err)
	}
	switch normalizeScalewayStatus(snapshot.Status) {
	case "ready":
		observation, err := api.databaseBackupObservation(state, operationID, instance, snapshot)
		if err != nil {
			return scalewayDatabaseFailed(state, operationID, err.Error()), nil
		}
		return observation, nil
	case "error", "deleting":
		return scalewayDatabaseFailed(state, operationID, "Scaleway Managed Database snapshot entered terminal state "+snapshot.Status), nil
	default:
		return scalewayDatabasePending(state, operationID), nil
	}
}

func (api *NativeAPI) databaseBackupObservation(state operationState, operationID string, instance DatabaseInstance, snapshot DatabaseSnapshot) (provider.NativeOperationObservation, error) {
	if err := api.verifyDatabaseInstance(instance, state, true); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if snapshot.InstanceID != instance.ID || !scalewayDatabaseSnapshotOwned(snapshot, state.OwnershipMarker) {
		return provider.NativeOperationObservation{}, errors.New("Scaleway Managed Database snapshot is not bound to the owned source instance")
	}
	if !isScalewayBlockVolume(snapshot.VolumeType) {
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway RDB snapshots without block-storage volume metadata cannot prove encrypted recovery")
	}
	if err := api.databaseSnapshotRetentionError(snapshot); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	backupID := "scaleway-rdb-snapshot://" + snapshot.ID
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: false,
		PermissionsVerified: true, ServiceHealthVerified: true,
		Reason: "verified an ownership-bound encrypted Scaleway RDB block snapshot with an explicit expiry; native RDB has no equivalent deletion-protection control",
	}
	return scalewayObservation(state, operationID, []string{backupID}, []string{"scaleway.rdb.snapshot", "scaleway.rdb.encryption", "scaleway.rdb.retention"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) databaseSnapshotRetentionError(snapshot DatabaseSnapshot) error {
	if snapshot.ExpiresAt.IsZero() {
		return errors.New("Scaleway Managed Database snapshot does not have an expiry")
	}
	if !snapshot.ExpiresAt.After(api.now()) {
		return errors.New("Scaleway Managed Database snapshot has expired")
	}
	if snapshot.CreatedAt.IsZero() {
		return nil
	}
	minimum := snapshot.CreatedAt.Add(time.Duration(api.retentionDays()) * 24 * time.Hour).Add(-2 * time.Minute)
	if snapshot.ExpiresAt.Before(minimum) {
		return errors.New("Scaleway Managed Database snapshot does not satisfy the configured retention expiry")
	}
	return nil
}

func (api *NativeAPI) restoreDatabase(ctx context.Context, state operationState, reference scalewayDatabaseReference) (provider.NativeOperationObservation, error) {
	if state.Destination == sdk.RecoveryAlternateRegion || state.Destination == sdk.RecoveryAlternateProvider {
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway Managed Database relational backups are documented as same-region; alternate-region recovery requires an independent encrypted export strategy")
	}
	snapshotID, err := parseScalewayDatabaseSnapshotReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	snapshot, err := api.database.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect Scaleway Managed Database restore snapshot: %w", err)
	}
	if normalizeScalewayStatus(snapshot.Status) != "ready" || !scalewayDatabaseSnapshotOwned(snapshot, state.OwnershipMarker) {
		return provider.NativeOperationObservation{}, errors.New("Scaleway Managed Database restore requires an owned ready snapshot")
	}
	if err := api.databaseSnapshotRetentionError(snapshot); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if !isScalewayBlockVolume(snapshot.VolumeType) {
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway RDB logical backups do not provide the encrypted block-snapshot proof required for this restore")
	}
	source, err := api.database.GetInstance(ctx, snapshot.InstanceID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect Scaleway Managed Database restore source: %w", err)
	}
	if err := api.verifyDatabaseInstance(source, state, false); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if reference.InstanceID != "" && reference.InstanceID != source.ID {
		return provider.NativeOperationObservation{}, errors.New("Scaleway Managed Database restore backup does not belong to the requested source instance")
	}
	targetName := api.restoreDatabaseName(state)
	target, getErr := api.findDatabaseInstanceByName(ctx, targetName)
	if getErr == nil {
		if !scalewayDatabaseRestoreOwned(target, state) {
			return provider.NativeOperationObservation{}, errors.New("refusing to overwrite an unowned Scaleway Managed Database restore target")
		}
		if err := api.verifyDatabaseInstance(target, state, false); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		state.Target = target.ID
		state.ResourceRef = snapshot.ID
		operationID, err := scalewayDatabaseOperationID(state)
		if err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return api.databaseRestoreObservation(state, operationID, target), nil
	}
	if !isScalewayNotFound(getErr) {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect Scaleway Managed Database restore target: %w", getErr)
	}
	isHA := source.IsHA || api.config.RequireDatabaseHA
	nodeType := strings.TrimSpace(api.config.RestoreDatabaseNodeType)
	if nodeType == "" {
		nodeType = snapshot.NodeType
	}
	createCtx, cancel := context.WithTimeout(ctx, databaseMutatingPostTime)
	target, err = api.database.CreateInstanceFromSnapshot(createCtx, snapshot.ID, targetName, nodeType, isHA)
	cancel()
	reconciled := false
	if err != nil {
		createErr := err
		// The restore endpoint is a mutating POST. If the request context is
		// canceled after Scaleway accepts it, the SDK may return an error while
		// the new instance continues provisioning. Reconcile by the collision-
		// checked deterministic name before returning, so the cleanup path can
		// claim and delete an ambiguous target instead of leaking it.
		target, err = api.reconcileDatabaseRestoreTarget(ctx, targetName, state)
		if err != nil {
			return provider.NativeOperationObservation{}, fmt.Errorf("restore Scaleway Managed Database snapshot: %w (reconcile target: %v)", createErr, err)
		}
		reconciled = true
	}
	if strings.TrimSpace(target.ID) == "" {
		return provider.NativeOperationObservation{}, errors.New("Scaleway Managed Database restore API returned no instance identity")
	}
	if !reconciled && isScalewayReady(target.Status) {
		tagged, tagErr := api.ensureDatabaseRestoreTargetOwned(ctx, target, state)
		if tagErr == nil {
			target = tagged
		} else if !isScalewayTransient(tagErr) {
			// Tagging is a second mutating call. Treat a canceled/ambiguous tag
			// update the same way as an ambiguous restore create, except when
			// the instance is still provisioning: we already have its ID.
			target, err = api.reconcileDatabaseRestoreTarget(ctx, targetName, state)
			if err != nil {
				return provider.NativeOperationObservation{}, fmt.Errorf("tag Scaleway Managed Database restore target: %w (reconcile target: %v)", tagErr, err)
			}
		}
	}
	state.Target = target.ID
	state.ResourceRef = snapshot.ID
	operationID, err := scalewayDatabaseOperationID(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return scalewayDatabasePending(state, operationID), nil
}

func (api *NativeAPI) findDatabaseSnapshot(ctx context.Context, instanceID, name string) (DatabaseSnapshot, error) {
	snapshots, err := api.database.ListSnapshots(ctx)
	if err != nil {
		return DatabaseSnapshot{}, fmt.Errorf("list Scaleway Managed Database snapshots: %w", err)
	}
	for _, snapshot := range snapshots {
		if snapshot.InstanceID == instanceID && snapshot.Name == name {
			return snapshot, nil
		}
	}
	return DatabaseSnapshot{}, errScalewayDatabaseSnapshotNotFound
}

func (api *NativeAPI) reconcileDatabaseSnapshot(parent context.Context, instanceID, name string) (DatabaseSnapshot, error) {
	if parent == nil {
		return DatabaseSnapshot{}, errors.New("Scaleway Managed Database snapshot reconciliation context is required")
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), databaseRestoreReconcileTime)
	defer cancel()
	ticker := time.NewTicker(databaseRestoreReconcileEvery)
	defer ticker.Stop()
	var lastErr error
	for {
		snapshot, err := api.findDatabaseSnapshot(ctx, instanceID, name)
		if err == nil {
			return snapshot, nil
		}
		if !errors.Is(err, errScalewayDatabaseSnapshotNotFound) && !isScalewayNotFound(err) {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			return DatabaseSnapshot{}, lastErr
		case <-ticker.C:
		}
	}
}

func (api *NativeAPI) findDatabaseInstanceByName(ctx context.Context, name string) (DatabaseInstance, error) {
	instances, err := api.database.ListInstances(ctx)
	if err != nil {
		return DatabaseInstance{}, fmt.Errorf("list Scaleway Managed Database restore targets: %w", err)
	}
	for _, instance := range instances {
		if instance.Name == name {
			return instance, nil
		}
	}
	return DatabaseInstance{}, errScalewayDatabaseInstanceNotFound
}

func (api *NativeAPI) ensureDatabaseRestoreTargetOwned(ctx context.Context, target DatabaseInstance, state operationState) (DatabaseInstance, error) {
	if strings.TrimSpace(target.ID) == "" {
		return DatabaseInstance{}, errors.New("Scaleway Managed Database restore target has no identity")
	}
	if len(target.Tags) != 0 && !scalewayDatabaseTagsMatch(target.Tags, state.OwnershipMarker, state.DataClass) {
		return DatabaseInstance{}, errors.New("refusing to claim an unowned Scaleway Managed Database restore target")
	}
	tags := scalewayDatabaseTags(target.Tags, state)
	updated, err := api.database.UpdateInstanceTags(ctx, target.ID, tags)
	if err != nil {
		return DatabaseInstance{}, err
	}
	return updated, nil
}

func (api *NativeAPI) reconcileDatabaseRestoreTarget(parent context.Context, targetName string, state operationState) (DatabaseInstance, error) {
	if parent == nil {
		return DatabaseInstance{}, errors.New("Scaleway Managed Database restore reconciliation context is required")
	}
	// Reconciliation is deliberately detached from a canceled request: the
	// preceding POST may already have created a billable instance. It still has
	// its own short deadline and never becomes unbounded background work.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), databaseRestoreReconcileTime)
	defer cancel()
	ticker := time.NewTicker(databaseRestoreReconcileEvery)
	defer ticker.Stop()
	var lastErr error
	for {
		target, err := api.findDatabaseInstanceByName(ctx, targetName)
		if err == nil {
			owned, tagErr := api.ensureDatabaseRestoreTargetOwned(ctx, target, state)
			if tagErr == nil {
				return owned, nil
			}
			lastErr = tagErr
		} else if !errors.Is(err, errScalewayDatabaseInstanceNotFound) && !isScalewayNotFound(err) {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			return DatabaseInstance{}, lastErr
		case <-ticker.C:
		}
	}
}

func (api *NativeAPI) pollDatabaseRestore(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if strings.TrimSpace(state.Target) == "" {
		return provider.NativeOperationObservation{}, errors.New("Scaleway Managed Database restore operation has no target identity")
	}
	target, err := api.database.GetInstance(ctx, state.Target)
	if err != nil {
		if isScalewayNotFound(err) {
			return scalewayDatabasePending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, fmt.Errorf("poll Scaleway Managed Database restore target: %w", err)
	}
	if !isScalewayReady(target.Status) {
		if normalizeScalewayStatus(target.Status) == "error" {
			return scalewayDatabaseFailed(state, operationID, "Scaleway Managed Database restore target entered an error state"), nil
		}
		return scalewayDatabasePending(state, operationID), nil
	}
	if !scalewayDatabaseRestoreOwned(target, state) {
		tagged, tagErr := api.ensureDatabaseRestoreTargetOwned(ctx, target, state)
		if isScalewayTransient(tagErr) {
			return scalewayDatabasePending(state, operationID), nil
		}
		if tagErr != nil {
			return scalewayDatabaseFailed(state, operationID, tagErr.Error()), nil
		}
		target = tagged
	}
	if err := api.verifyDatabaseInstance(target, state, false); err != nil {
		return scalewayDatabaseFailed(state, operationID, err.Error()), nil
	}
	return api.databaseRestoreObservation(state, operationID, target), nil
}

func (api *NativeAPI) databaseRestoreObservation(state operationState, operationID string, target DatabaseInstance) provider.NativeOperationObservation {
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup,
		RestoreID: "scaleway-rdb://" + target.ID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(),
		EncryptionVerified: target.EncryptionEnabled, ProtectionVerified: false,
		PermissionsVerified: true, ServiceHealthVerified: isScalewayReady(target.Status),
		Reason: "restored an isolated ownership-tagged Scaleway RDB instance and verified ready state, encryption, and HA policy; deletion protection is not exposed by native RDB",
	}
	return scalewayObservation(state, operationID, []string{"scaleway-rdb://" + target.ID}, []string{"scaleway.rdb.restore", "scaleway.rdb.health", "scaleway.rdb.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func (api *NativeAPI) integrityDatabase(ctx context.Context, state operationState, operationID string, reference scalewayDatabaseReference) (provider.NativeOperationObservation, error) {
	if api.config.Verifier == nil {
		return provider.NativeOperationObservation{}, errors.New("Scaleway Managed Database integrity requires an application recovery verifier")
	}
	instance, err := api.database.GetInstance(ctx, reference.InstanceID)
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("inspect Scaleway Managed Database instance for integrity: %w", err)
	}
	if err := api.verifyDatabaseInstance(instance, state, false); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	verification, err := api.config.Verifier.Verify(ctx, RecoveryVerificationRequest{DataClass: state.DataClass, Resource: state.Resource, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker})
	if err != nil {
		return provider.NativeOperationObservation{}, fmt.Errorf("verify Scaleway Managed Database application fixture: %w", err)
	}
	if !verification.ApplicationReadsVerified || !verification.PermissionsVerified || !verification.ServiceHealthVerified {
		return provider.NativeOperationObservation{}, errors.New("Scaleway Managed Database integrity verifier did not prove reads, permissions, and service health")
	}
	evidence := sdk.ResilienceProofEvidence{
		DataClass: state.DataClass, Destination: string(state.Destination), BackupID: state.Backup, FixtureID: state.FixtureID,
		RetentionDays: api.retentionDays(), EncryptionVerified: instance.EncryptionEnabled, ProtectionVerified: false,
		ManifestVerified: verification.ManifestVerified, CountsVerified: verification.CountsVerified,
		ApplicationReadsVerified: verification.ApplicationReadsVerified, PermissionsVerified: verification.PermissionsVerified,
		SecretReferencesVerified: verification.SecretReferencesVerified, ServiceHealthVerified: verification.ServiceHealthVerified,
		Reason: "verified ready encrypted Scaleway RDB control-plane state and application-owned fixture reads without exposing records in evidence",
	}
	return scalewayObservation(state, operationID, []string{"scaleway-rdb://" + instance.ID}, []string{"scaleway.rdb.integrity", "scaleway.rdb.application"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded)), nil
}

func (api *NativeAPI) verifyDatabaseInstance(instance DatabaseInstance, state operationState, requireSource bool) error {
	if strings.TrimSpace(instance.ID) == "" || !isScalewayReady(instance.Status) {
		return fmt.Errorf("Scaleway Managed Database instance %q is not ready", instance.Name)
	}
	if !scalewayDatabaseTagsMatch(instance.Tags, state.OwnershipMarker, state.DataClass) {
		return errors.New("Scaleway Managed Database instance is not owned by this operation")
	}
	if api.config.RequireDatabaseHA && !instance.IsHA {
		return errors.New("Scaleway Managed Database instance is not configured for high availability")
	}
	if !instance.EncryptionEnabled {
		return errors.New("Scaleway Managed Database instance does not have encryption at rest enabled")
	}
	if requireSource && !isScalewayBlockVolume(instance.VolumeType) {
		return scalewayCapabilityError(state, "Scaleway Managed Database logical-backup volume does not provide the encrypted block-snapshot recovery path")
	}
	return nil
}

func (api *NativeAPI) inventoryDatabase(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	instances, err := api.database.ListInstances(ctx)
	if err != nil {
		return nil, fmt.Errorf("inventory Scaleway Managed Database instances: %w", err)
	}
	snapshots, err := api.database.ListSnapshots(ctx)
	if err != nil {
		return nil, fmt.Errorf("inventory Scaleway Managed Database snapshots: %w", err)
	}
	resources := make([]provider.InventoryResource, 0, len(instances)+len(snapshots))
	for _, instance := range instances {
		if scalewayDatabaseTagsMatch(instance.Tags, marker, "database") {
			resources = append(resources, provider.InventoryResource{Identity: "scaleway-rdb://" + instance.ID, Owned: true, Live: true})
		}
	}
	prefix := scalewayDatabaseNamePrefix + shortDigest(marker) + "-"
	for _, snapshot := range snapshots {
		if strings.HasPrefix(snapshot.Name, prefix) {
			resources = append(resources, provider.InventoryResource{Identity: "scaleway-rdb-snapshot://" + snapshot.ID, Owned: true, Live: true})
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func (api *NativeAPI) restoreDatabaseName(state operationState) string {
	return scalewayDatabaseRestorePrefix + shortDigest(state.OwnershipMarker+"\x00"+state.FixtureID+"\x00"+state.IdempotencyKey)
}

func scalewayDatabaseSnapshotName(state operationState) string {
	return scalewayDatabaseNamePrefix + shortDigest(state.OwnershipMarker) + "-" + shortDigest(state.FixtureID+"\x00"+state.IdempotencyKey)
}

func scalewayDatabaseSnapshotOwned(snapshot DatabaseSnapshot, marker string) bool {
	return strings.HasPrefix(snapshot.Name, scalewayDatabaseNamePrefix+shortDigest(marker)+"-")
}

func scalewayDatabaseTags(existing []string, state operationState) []string {
	tags := append([]string(nil), existing...)
	for _, managed := range []string{scalewayDatabaseOwnershipTag + "=" + state.OwnershipMarker, scalewayDatabaseClassTag + "=" + state.DataClass, scalewayRecoveryOutputTag} {
		found := false
		for _, tag := range tags {
			if tag == managed {
				found = true
				break
			}
		}
		if !found {
			tags = append(tags, managed)
		}
	}
	sort.Strings(tags)
	return tags
}

func scalewayDatabaseRestoreOwned(instance DatabaseInstance, state operationState) bool {
	return strings.HasPrefix(instance.Name, scalewayDatabaseRestorePrefix) && scalewayDatabaseTagsMatch(instance.Tags, state.OwnershipMarker, "database") && hasScalewayTag(instance.Tags, scalewayRecoveryOutputTag)
}

func hasScalewayTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if tag == wanted {
			return true
		}
	}
	return false
}

func scalewayDatabaseTagsMatch(tags []string, marker, dataClass string) bool {
	wantOwnership := scalewayDatabaseOwnershipTag + "=" + marker
	wantClass := scalewayDatabaseClassTag + "=" + dataClass
	for _, tag := range tags {
		if tag == wantOwnership {
			for _, other := range tags {
				if other == wantClass {
					return true
				}
			}
		}
	}
	return false
}

func parseScalewayDatabaseReference(reference string) (scalewayDatabaseReference, error) {
	value := strings.TrimSpace(reference)
	for _, scheme := range []string{"scaleway-rdb://", "scaleway-database://", "scw-rdb://"} {
		if strings.HasPrefix(strings.ToLower(value), scheme) {
			value = strings.TrimPrefix(value, value[:len(scheme)])
			break
		}
	}
	if strings.ContainsAny(value, "\r\n\x00?#") {
		return scalewayDatabaseReference{}, fmt.Errorf("Scaleway Managed Database reference %q contains invalid control data", reference)
	}
	value = strings.Trim(value, "/")
	if parsed, err := url.Parse(value); err == nil && parsed.Path != "" {
		value = strings.Trim(parsed.Host+parsed.Path, "/")
	}
	parts := strings.Split(value, "/")
	switch {
	case len(parts) == 1 && parts[0] != "":
		return scalewayDatabaseReference{InstanceID: parts[0]}, nil
	case len(parts) == 2 && parts[0] == "instances" && parts[1] != "":
		return scalewayDatabaseReference{InstanceID: parts[1]}, nil
	case len(parts) == 4 && parts[0] == "projects" && parts[1] != "" && parts[2] == "instances" && parts[3] != "":
		return scalewayDatabaseReference{InstanceID: parts[3]}, nil
	default:
		return scalewayDatabaseReference{}, fmt.Errorf("Scaleway Managed Database reference %q must identify an instance", reference)
	}
}

func parseScalewayDatabaseSnapshotReference(reference string) (string, error) {
	value := strings.TrimSpace(reference)
	for _, scheme := range []string{"scaleway-rdb-snapshot://", "scaleway-database-snapshot://", "scw-rdb-snapshot://"} {
		if strings.HasPrefix(strings.ToLower(value), scheme) {
			value = value[len(scheme):]
			break
		}
	}
	value = strings.Trim(value, "/")
	if strings.ContainsAny(value, "\r\n\x00?#") {
		return "", fmt.Errorf("Scaleway Managed Database snapshot reference %q contains invalid control data", reference)
	}
	value = strings.TrimPrefix(value, "snapshots/")
	if value == "" || strings.Contains(value, "/") {
		return "", fmt.Errorf("Scaleway Managed Database snapshot reference %q must identify one snapshot", reference)
	}
	return value, nil
}

func scalewayDatabaseOperationID(state operationState) (string, error) {
	return cloudrecovery.EncodeOperationID(scalewayRecoveryOperationPrefix, state)
}

func scalewayDatabasePending(state operationState, operationID string) provider.NativeOperationObservation {
	return scalewayObservation(state, operationID, nil, []string{"scaleway.rdb.pending"}, nil, string(sdk.ResilienceOperationPending))
}

func scalewayDatabaseFailed(state operationState, operationID, detail string) provider.NativeOperationObservation {
	observation := scalewayObservation(state, operationID, nil, []string{"scaleway.rdb.failed"}, nil, string(sdk.ResilienceOperationFailed))
	observation.Detail = detail
	return observation
}

func normalizeScalewayStatus(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isScalewayReady(value string) bool {
	return normalizeScalewayStatus(value) == "ready"
}

func isScalewayBlockVolume(value string) bool {
	switch normalizeScalewayStatus(value) {
	case "bssd", "sbs", "sbs_5k", "sbs_15k":
		return true
	default:
		return false
	}
}
