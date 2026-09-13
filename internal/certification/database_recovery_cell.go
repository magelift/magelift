package certification

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// DatabaseRecoveryFixtureStore is the provider-local data-plane seam for the
// shared database recovery cell. It materializes scrubbed known content and
// removes only that source fixture; SQL drivers, credentials, and cloud SDK
// models stay outside the certification core.
type DatabaseRecoveryFixtureStore interface {
	Prepare(context.Context, string, RecoveryFixtureManifest) error
	Delete(context.Context, string) error
}

// DatabaseRecoveryOverwriter is required only for same-region in-place cells.
// Isolated stores may omit it. Overwrite must change the existing fixture
// payload on the source identity; it must not create a new identity.
type DatabaseRecoveryOverwriter interface {
	Overwrite(ctx context.Context, resource, payload string) error
}

// DatabaseRecoveryCorruptPayload is the synthetic value written onto the
// source after backup and before in-place restore. Integrity must not observe
// this value after restore.
const DatabaseRecoveryCorruptPayload = "magelift-corrupt-before-inplace"

// DatabaseRecoveryProtectionRequirement describes whether the selected
// provider/data-class contract has an enforceable deletion-protection
// equivalent. It is deliberately explicit: an omitted value defaults to
// required, while an adapter must opt into optional only when its capability
// descriptor says the native service has no equivalent control.
type DatabaseRecoveryProtectionRequirement string

const (
	DatabaseRecoveryProtectionRequired DatabaseRecoveryProtectionRequirement = "required"
	DatabaseRecoveryProtectionOptional DatabaseRecoveryProtectionRequirement = "optional"
)

// DatabaseRecoveryCellConfig binds one managed or self-hosted database to the
// normalized recovery client. The provider adapter remains responsible for
// backup/restore implementation and for independent verifier evidence.
type DatabaseRecoveryCellConfig struct {
	Operations        sdk.ResilienceOperationClient
	FixtureStore      DatabaseRecoveryFixtureStore
	OperationPolicy   sdk.ResilienceOperationPolicy
	Fixture           RecoveryFixtureManifest
	DataClass         string
	Destination       sdk.RecoveryDestination
	ResourceReference string
	Marker            string
	ApprovalReference string
	Protection        DatabaseRecoveryProtectionRequirement
}

// DatabaseRecoveryCell is reusable by AWS RDS/Aurora, GCP Cloud SQL,
// Scaleway RDB, OVH Managed Database, self-hosted database adapters, and
// community implementations that can satisfy the two small provider seams.
type DatabaseRecoveryCell struct {
	config DatabaseRecoveryCellConfig
	class  RecoveryFixtureClass
}

type DatabaseRecoveryCellResult struct {
	BackupID                string
	RestoreID               string
	IntegrityEvidence       sdk.ResilienceProofEvidence
	RestoreDurationSeconds  int64
	SourceInventoryVerified bool
	OutputInventoryVerified bool
	CorruptionInjected      bool
}

func NewDatabaseRecoveryCell(config DatabaseRecoveryCellConfig) (*DatabaseRecoveryCell, error) {
	if config.Operations == nil {
		return nil, errors.New("database recovery cell operation client is required")
	}
	if config.FixtureStore == nil {
		return nil, errors.New("database recovery cell fixture store is required")
	}
	for _, part := range []struct {
		value string
		name  string
	}{
		{config.ResourceReference, "resource reference"}, {config.Marker, "ownership marker"},
	} {
		if strings.TrimSpace(part.value) == "" || strings.ContainsAny(part.value, "\r\n\x00") {
			return nil, fmt.Errorf("database recovery cell %s is required and must be single-line", part.name)
		}
	}
	if err := config.Fixture.Validate(); err != nil {
		return nil, fmt.Errorf("database recovery cell fixture: %w", err)
	}
	if strings.TrimSpace(config.DataClass) == "" {
		config.DataClass = "database"
	}
	if strings.ContainsAny(config.DataClass, "\r\n\x00") {
		return nil, errors.New("database recovery cell data class must be single-line")
	}
	if config.Destination == "" {
		config.Destination = sdk.RecoverySameRegionIsolated
	}
	if err := validateDatabaseRecoveryPolicy(config.OperationPolicy); err != nil {
		return nil, err
	}
	if config.Protection == "" {
		config.Protection = DatabaseRecoveryProtectionRequired
	}
	if config.Protection != DatabaseRecoveryProtectionRequired && config.Protection != DatabaseRecoveryProtectionOptional {
		return nil, fmt.Errorf("database recovery cell protection must be %q or %q", DatabaseRecoveryProtectionRequired, DatabaseRecoveryProtectionOptional)
	}
	if config.Destination == sdk.RecoverySameRegion {
		if _, ok := config.FixtureStore.(DatabaseRecoveryOverwriter); !ok {
			return nil, errors.New("database recovery in-place cell requires a store that can overwrite an existing fixture")
		}
	}
	class, ok := databaseFixtureClass(config.Fixture, config.DataClass)
	if !ok {
		return nil, fmt.Errorf("database recovery cell fixture has no %q class", config.DataClass)
	}
	return &DatabaseRecoveryCell{config: config, class: class}, nil
}

func (cell *DatabaseRecoveryCell) Prepare(ctx context.Context) error {
	if cell == nil {
		return errors.New("database recovery cell is required")
	}
	if err := validateDatabaseRecoveryContext(ctx); err != nil {
		return err
	}
	if err := cell.config.FixtureStore.Prepare(ctx, cell.config.ResourceReference, cell.config.Fixture); err != nil {
		return fmt.Errorf("materialize database recovery fixture: %w", err)
	}
	return nil
}

// Run executes backup, optional in-place source corruption, isolated or
// in-place restore, and data-plane integrity verification. The restore
// identity is deliberately passed to the integrity stage; checking the
// original source after a successful isolated restore would be a false
// positive.
func (cell *DatabaseRecoveryCell) Run(ctx context.Context) (DatabaseRecoveryCellResult, error) {
	if cell == nil {
		return DatabaseRecoveryCellResult{}, errors.New("database recovery cell is required")
	}
	if err := validateDatabaseRecoveryContext(ctx); err != nil {
		return DatabaseRecoveryCellResult{}, err
	}
	base := sdk.ResilienceOperationRequest{
		DataClasses: []string{cell.config.DataClass}, Destination: cell.config.Destination,
		FixtureID: cell.config.Fixture.ID, OwnershipMarker: cell.config.Marker,
		ResourceReferences: map[string]string{cell.config.DataClass: cell.config.ResourceReference},
		ApprovalReference:  cell.config.ApprovalReference,
	}

	backupRequest := base
	backupRequest.Action = sdk.ResilienceBackup
	backupRequest.IdempotencyKey = "live/" + cell.config.DataClass + "/backup"
	backup, err := cell.config.Operations.Start(ctx, backupRequest)
	if err != nil {
		return DatabaseRecoveryCellResult{}, fmt.Errorf("start database recovery backup: %w", err)
	}
	backup, err = waitDatabaseRecoveryOperation(ctx, cell.config.Operations, backup, cell.config.OperationPolicy)
	if err != nil {
		return DatabaseRecoveryCellResult{}, fmt.Errorf("wait for database recovery backup: %w", err)
	}
	if len(backup.Evidence) != 1 || backup.Evidence[0].BackupID == "" || !backup.Evidence[0].EncryptionVerified || !cell.protectionVerified(backup.Evidence[0]) {
		return DatabaseRecoveryCellResult{}, fmt.Errorf("database recovery backup evidence is incomplete: %#v", backup.Evidence)
	}

	corruptionInjected := false
	if cell.config.Destination == sdk.RecoverySameRegion {
		overwriter, ok := cell.config.FixtureStore.(DatabaseRecoveryOverwriter)
		if !ok {
			return DatabaseRecoveryCellResult{}, errors.New("database recovery in-place cell requires a store that can overwrite an existing fixture")
		}
		if err := overwriter.Overwrite(ctx, cell.config.ResourceReference, DatabaseRecoveryCorruptPayload); err != nil {
			return DatabaseRecoveryCellResult{}, fmt.Errorf("corrupt database recovery source before in-place restore: %w", err)
		}
		corruptionInjected = true
	}

	restoreStarted := time.Now()
	restoreRequest := base
	restoreRequest.Action = sdk.ResilienceRestore
	restoreRequest.IdempotencyKey = "live/" + cell.config.DataClass + "/restore"
	restoreRequest.BackupReferences = map[string]string{cell.config.DataClass: backup.Evidence[0].BackupID}
	restored, err := cell.config.Operations.Start(ctx, restoreRequest)
	if err != nil {
		return DatabaseRecoveryCellResult{}, fmt.Errorf("start database recovery restore: %w", err)
	}
	restored, err = waitDatabaseRecoveryOperation(ctx, cell.config.Operations, restored, cell.config.OperationPolicy)
	if err != nil {
		return DatabaseRecoveryCellResult{}, fmt.Errorf("wait for database recovery restore: %w", err)
	}
	if len(restored.Evidence) != 1 || restored.Evidence[0].RestoreID == "" || !restored.Evidence[0].ServiceHealthVerified {
		return DatabaseRecoveryCellResult{}, fmt.Errorf("database recovery restore evidence is incomplete: %#v", restored.Evidence)
	}
	restoreID := restored.Evidence[0].RestoreID

	integrityRequest := base
	integrityRequest.Action = sdk.ResilienceIntegrityCheck
	integrityRequest.IdempotencyKey = "live/" + cell.config.DataClass + "/integrity"
	integrityRequest.ResourceReferences = map[string]string{cell.config.DataClass: restoreID}
	integrityRequest.BackupReferences = map[string]string{cell.config.DataClass: backup.Evidence[0].BackupID}
	integrity, err := cell.config.Operations.Start(ctx, integrityRequest)
	if err != nil {
		return DatabaseRecoveryCellResult{}, fmt.Errorf("verify database recovery fixture: %w", err)
	}
	integrity, err = waitDatabaseRecoveryOperation(ctx, cell.config.Operations, integrity, cell.config.OperationPolicy)
	if err != nil {
		return DatabaseRecoveryCellResult{}, fmt.Errorf("wait for database recovery integrity: %w", err)
	}
	if len(integrity.Evidence) != 1 {
		return DatabaseRecoveryCellResult{}, fmt.Errorf("database recovery integrity evidence is incomplete: %#v", integrity.Evidence)
	}
	evidence := integrity.Evidence[0]
	if err := validateDatabaseRecoveryEvidence(evidence, cell.config.DataClass, cell.config.Fixture.ID, cell.class, cell.config.Protection); err != nil {
		return DatabaseRecoveryCellResult{}, err
	}

	inventory, err := cell.config.Operations.Inventory(ctx, cell.config.Marker)
	if err != nil {
		return DatabaseRecoveryCellResult{}, fmt.Errorf("inventory database recovery outputs: %w", err)
	}
	if len(inventory) == 0 {
		return DatabaseRecoveryCellResult{}, errors.New("database recovery inventory was empty before cleanup")
	}
	sourceInventoryVerified := containsRecoveryInventory(inventory, cell.config.ResourceReference)
	outputInventoryVerified := containsRecoveryInventory(inventory, restoreID)
	if !sourceInventoryVerified || !outputInventoryVerified {
		return DatabaseRecoveryCellResult{}, fmt.Errorf("database recovery inventory did not prove the source and restore identities: source=%t output=%t", sourceInventoryVerified, outputInventoryVerified)
	}
	return DatabaseRecoveryCellResult{
		BackupID: backup.Evidence[0].BackupID, RestoreID: restoreID, IntegrityEvidence: evidence,
		RestoreDurationSeconds:  maxInt64(1, int64(time.Since(restoreStarted)/time.Second)),
		SourceInventoryVerified: sourceInventoryVerified,
		OutputInventoryVerified: outputInventoryVerified,
		CorruptionInjected:      corruptionInjected,
	}, nil
}

func (cell *DatabaseRecoveryCell) Cleanup(ctx context.Context) error {
	if cell == nil {
		return errors.New("database recovery cell is required")
	}
	if err := validateDatabaseRecoveryContext(ctx); err != nil {
		return err
	}
	var cleanupErrors []error
	cleanup := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceCleanup, DataClasses: []string{cell.config.DataClass}, Destination: cell.config.Destination,
		FixtureID: cell.config.Fixture.ID, OwnershipMarker: cell.config.Marker, IdempotencyKey: "live/" + cell.config.DataClass + "/cleanup",
		ResourceReferences: map[string]string{cell.config.DataClass: cell.config.ResourceReference},
		ApprovalReference:  cell.config.ApprovalReference,
	}
	observation, err := cell.config.Operations.Start(ctx, cleanup)
	if err == nil {
		_, err = waitDatabaseRecoveryOperation(ctx, cell.config.Operations, observation, cell.config.OperationPolicy)
	}
	if err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("clean database recovery outputs: %w", err))
	}
	if err := cell.config.FixtureStore.Delete(ctx, cell.config.ResourceReference); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("delete database recovery source fixture: %w", err))
	}
	return errors.Join(cleanupErrors...)
}

func waitDatabaseRecoveryOperation(ctx context.Context, client sdk.ResilienceOperationClient, observation sdk.ResilienceOperationObservation, policy sdk.ResilienceOperationPolicy) (sdk.ResilienceOperationObservation, error) {
	if observation.Status == sdk.ResilienceOperationSucceeded {
		return observation, nil
	}
	return sdk.WaitForResilienceOperation(ctx, client, observation.OperationID, policy)
}

func validateDatabaseRecoveryPolicy(policy sdk.ResilienceOperationPolicy) error {
	if policy.Timeout <= 0 || policy.PollInterval <= 0 || policy.MaxAttempts <= 0 {
		return errors.New("database recovery cell requires a positive operation timeout, poll interval, and attempt limit")
	}
	return nil
}

func validateDatabaseRecoveryContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("database recovery cell context is required")
	}
	return ctx.Err()
}

func databaseFixtureClass(manifest RecoveryFixtureManifest, dataClass string) (RecoveryFixtureClass, bool) {
	for _, class := range manifest.Classes {
		if class.Name == dataClass {
			return class, true
		}
	}
	return RecoveryFixtureClass{}, false
}

func validateDatabaseRecoveryEvidence(evidence sdk.ResilienceProofEvidence, dataClass, fixtureID string, class RecoveryFixtureClass, protection DatabaseRecoveryProtectionRequirement) error {
	if evidence.DataClass != dataClass || evidence.FixtureID != fixtureID {
		return fmt.Errorf("database recovery integrity evidence identity mismatch: %#v", evidence)
	}
	if !evidence.EncryptionVerified || !evidence.ManifestVerified || !evidence.CountsVerified || !evidence.ApplicationReadsVerified || !evidence.PermissionsVerified || !evidence.ServiceHealthVerified || (protection == DatabaseRecoveryProtectionRequired && !evidence.ProtectionVerified) {
		return fmt.Errorf("database recovery integrity evidence must prove encryption, manifest, counts, application reads, permissions, service health, and the configured protection policy: %#v", evidence)
	}
	if len(class.SecretReferences) > 0 && !evidence.SecretReferencesVerified {
		return errors.New("database recovery integrity evidence must prove secret references")
	}
	return nil
}

func (cell *DatabaseRecoveryCell) protectionVerified(evidence sdk.ResilienceProofEvidence) bool {
	return cell.config.Protection == DatabaseRecoveryProtectionOptional || evidence.ProtectionVerified
}

func containsRecoveryInventory(resources []sdk.ResilienceInventoryResource, identity string) bool {
	for _, resource := range resources {
		if resource.Identity == identity && resource.Owned && resource.Live {
			return true
		}
	}
	return false
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
