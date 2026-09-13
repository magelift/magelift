package certification

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// SecretRecoveryStore is the provider-local value boundary for one disposable
// secret fixture. It deliberately carries opaque references and bytes only;
// provider SDK models, secret names, and ownership tags stay in the adapter.
// Implementations must never log or return the value except to this cell.
type SecretRecoveryStore interface {
	Put(context.Context, string, []byte) error
	Read(context.Context, string) ([]byte, error)
	Delete(context.Context, string) error
}

// SecretRecoveryOverwriter is required only for same-region in-place cells.
// Isolated stores may omit it. Overwrite must change the latest value of an
// existing secret; it must not create a new identity.
type SecretRecoveryOverwriter interface {
	Overwrite(context.Context, string, []byte) error
}

const secretRecoveryCorruptValue = "magelift-corrupt-before-inplace"

// SecretRecoveryCellConfig binds one disposable secret to the normalized
// recovery operation client. The provider owns source creation details while
// this core runner owns the backup, isolated or approved in-place restore,
// integrity, inventory, and exact source cleanup sequence.
type SecretRecoveryCellConfig struct {
	Operations        sdk.ResilienceOperationClient
	Store             SecretRecoveryStore
	ResourceReference string
	Marker            string
	FixtureID         string
	DataClass         string
	Destination       sdk.RecoveryDestination
	ApprovalReference string
}

// SecretRecoveryCell is reusable by AWS Secrets Manager, GCP Secret Manager,
// Scaleway Secret Manager, OVHcloud OKMS-backed secrets, and community
// providers that can implement the small store boundary.
type SecretRecoveryCell struct {
	config SecretRecoveryCellConfig
	value  []byte
}

type SecretRecoveryCellResult struct {
	BackupID         string
	RestoreID        string
	ValueFingerprint string
}

func NewSecretRecoveryCell(config SecretRecoveryCellConfig) (*SecretRecoveryCell, error) {
	if config.Operations == nil {
		return nil, errors.New("secret recovery cell operation client is required")
	}
	if config.Store == nil {
		return nil, errors.New("secret recovery cell secret store is required")
	}
	for _, part := range []struct {
		value string
		name  string
	}{
		{config.ResourceReference, "resource reference"}, {config.Marker, "ownership marker"}, {config.FixtureID, "fixture ID"},
	} {
		if strings.TrimSpace(part.value) == "" || strings.ContainsAny(part.value, "\r\n\x00") {
			return nil, fmt.Errorf("secret recovery cell %s is required and must be single-line", part.name)
		}
	}
	if strings.TrimSpace(config.DataClass) == "" {
		config.DataClass = "configuration-secrets"
	}
	if strings.ContainsAny(config.DataClass, "\r\n\x00") {
		return nil, errors.New("secret recovery cell data class must be single-line")
	}
	if config.Destination == "" {
		config.Destination = sdk.RecoverySameRegionIsolated
	}
	if config.Destination == sdk.RecoverySameRegion {
		if _, ok := config.Store.(SecretRecoveryOverwriter); !ok {
			return nil, errors.New("secret recovery in-place cell requires a store that can overwrite an existing secret")
		}
	}
	return &SecretRecoveryCell{config: config, value: knownSecretRecoveryValue(config.FixtureID)}, nil
}

// Prepare writes one deterministic synthetic value. The value is generated
// from the fixture identity and is never included in an observation, error, or
// evidence record.
func (cell *SecretRecoveryCell) Prepare(ctx context.Context) error {
	if cell == nil {
		return errors.New("secret recovery cell is required")
	}
	if err := validateSecretCellContext(ctx); err != nil {
		return err
	}
	if err := cell.config.Store.Put(ctx, cell.config.ResourceReference, append([]byte(nil), cell.value...)); err != nil {
		return fmt.Errorf("put secret recovery fixture: %w", err)
	}
	return nil
}

// Run executes backup, isolated or approved in-place restore, value
// comparison, integrity proof, and ownership inventory. It intentionally does
// not clean up so callers can inspect the provider inventory before Cleanup
// runs.
func (cell *SecretRecoveryCell) Run(ctx context.Context) (SecretRecoveryCellResult, error) {
	if cell == nil {
		return SecretRecoveryCellResult{}, errors.New("secret recovery cell is required")
	}
	if err := validateSecretCellContext(ctx); err != nil {
		return SecretRecoveryCellResult{}, err
	}
	base := sdk.ResilienceOperationRequest{
		DataClasses:        []string{cell.config.DataClass},
		Destination:        cell.config.Destination,
		FixtureID:          cell.config.FixtureID,
		OwnershipMarker:    cell.config.Marker,
		ResourceReferences: map[string]string{cell.config.DataClass: cell.config.ResourceReference},
		ApprovalReference:  cell.config.ApprovalReference,
	}

	backupRequest := base
	backupRequest.Action = sdk.ResilienceBackup
	backupRequest.IdempotencyKey = "live/" + cell.config.DataClass + "/backup"
	backup, err := cell.config.Operations.Start(ctx, backupRequest)
	if err != nil {
		return SecretRecoveryCellResult{}, fmt.Errorf("start secret recovery backup: %w", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || len(backup.Evidence) != 1 || backup.Evidence[0].BackupID == "" || !backup.Evidence[0].EncryptionVerified || !backup.Evidence[0].ProtectionVerified || !backup.Evidence[0].ManifestVerified || !backup.Evidence[0].SecretReferencesVerified {
		return SecretRecoveryCellResult{}, fmt.Errorf("secret recovery backup evidence is incomplete: %#v", backup.Evidence)
	}

	if cell.config.Destination == sdk.RecoverySameRegion {
		overwriter, ok := cell.config.Store.(SecretRecoveryOverwriter)
		if !ok {
			return SecretRecoveryCellResult{}, errors.New("secret recovery in-place cell requires a store that can overwrite an existing secret")
		}
		if err := overwriter.Overwrite(ctx, cell.config.ResourceReference, []byte(secretRecoveryCorruptValue)); err != nil {
			return SecretRecoveryCellResult{}, fmt.Errorf("corrupt secret recovery source before in-place restore: %w", err)
		}
	}

	restoreRequest := base
	restoreRequest.Action = sdk.ResilienceRestore
	restoreRequest.IdempotencyKey = "live/" + cell.config.DataClass + "/restore"
	restoreRequest.BackupReferences = map[string]string{cell.config.DataClass: backup.Evidence[0].BackupID}
	restored, err := cell.config.Operations.Start(ctx, restoreRequest)
	if err != nil {
		return SecretRecoveryCellResult{}, fmt.Errorf("restore secret recovery backup: %w", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || len(restored.Evidence) != 1 || restored.Evidence[0].RestoreID == "" || !restored.Evidence[0].EncryptionVerified || !restored.Evidence[0].ProtectionVerified || !restored.Evidence[0].SecretReferencesVerified || !restored.Evidence[0].ServiceHealthVerified {
		return SecretRecoveryCellResult{}, fmt.Errorf("secret recovery restore evidence is incomplete: %#v", restored.Evidence)
	}

	recovered, err := cell.config.Store.Read(ctx, restored.Evidence[0].RestoreID)
	if err != nil {
		return SecretRecoveryCellResult{}, fmt.Errorf("read secret recovery restore: %w", err)
	}
	if cell.config.Destination == sdk.RecoverySameRegion {
		if restored.Evidence[0].RestoreID != cell.config.ResourceReference {
			return SecretRecoveryCellResult{}, fmt.Errorf("secret recovery in-place restore identity %q does not match source %q", restored.Evidence[0].RestoreID, cell.config.ResourceReference)
		}
		if bytes.Equal(recovered, []byte(secretRecoveryCorruptValue)) {
			return SecretRecoveryCellResult{}, errors.New("secret recovery in-place restore left the corrupted source value")
		}
		if !bytes.Equal(recovered, cell.value) {
			return SecretRecoveryCellResult{}, errors.New("secret recovery in-place restore value does not match the known fixture")
		}
	} else {
		source, err := cell.config.Store.Read(ctx, cell.config.ResourceReference)
		if err != nil {
			return SecretRecoveryCellResult{}, fmt.Errorf("read secret recovery source: %w", err)
		}
		if !bytes.Equal(source, cell.value) || !bytes.Equal(recovered, source) {
			return SecretRecoveryCellResult{}, errors.New("secret recovery isolated restore value does not match the known fixture")
		}
	}

	integrityRequest := base
	integrityRequest.Action = sdk.ResilienceIntegrityCheck
	integrityRequest.IdempotencyKey = "live/" + cell.config.DataClass + "/integrity"
	integrityRequest.BackupReferences = map[string]string{cell.config.DataClass: backup.Evidence[0].BackupID}
	integrity, err := cell.config.Operations.Start(ctx, integrityRequest)
	if err != nil {
		return SecretRecoveryCellResult{}, fmt.Errorf("verify secret recovery archive integrity: %w", err)
	}
	if integrity.Status != sdk.ResilienceOperationSucceeded || len(integrity.Evidence) != 1 || !integrity.Evidence[0].ManifestVerified || !integrity.Evidence[0].PermissionsVerified || !integrity.Evidence[0].SecretReferencesVerified || !integrity.Evidence[0].ServiceHealthVerified {
		return SecretRecoveryCellResult{}, fmt.Errorf("secret recovery integrity evidence is incomplete: %#v", integrity.Evidence)
	}

	inventory, err := cell.config.Operations.Inventory(ctx, cell.config.Marker)
	if err != nil {
		return SecretRecoveryCellResult{}, fmt.Errorf("inventory secret recovery outputs: %w", err)
	}
	if len(inventory) == 0 {
		return SecretRecoveryCellResult{}, errors.New("secret recovery inventory was empty before cleanup")
	}
	return SecretRecoveryCellResult{BackupID: backup.Evidence[0].BackupID, RestoreID: restored.Evidence[0].RestoreID, ValueFingerprint: secretRecoveryFingerprint(recovered)}, nil
}

// Cleanup first asks the provider adapter to remove marker-owned archive and
// restore outputs, then removes only the exact synthetic source reference.
func (cell *SecretRecoveryCell) Cleanup(ctx context.Context) error {
	if cell == nil {
		return errors.New("secret recovery cell is required")
	}
	if err := validateSecretCellContext(ctx); err != nil {
		return err
	}
	var cleanupErrors []error
	_, err := cell.config.Operations.Start(ctx, sdk.ResilienceOperationRequest{
		Action:             sdk.ResilienceCleanup,
		DataClasses:        []string{cell.config.DataClass},
		Destination:        cell.config.Destination,
		FixtureID:          cell.config.FixtureID,
		OwnershipMarker:    cell.config.Marker,
		IdempotencyKey:     "live/" + cell.config.DataClass + "/cleanup",
		ResourceReferences: map[string]string{cell.config.DataClass: cell.config.ResourceReference},
		ApprovalReference:  cell.config.ApprovalReference,
	})
	if err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("clean secret recovery outputs: %w", err))
	}
	if err := cell.config.Store.Delete(ctx, cell.config.ResourceReference); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("delete secret recovery source fixture: %w", err))
	}
	return errors.Join(cleanupErrors...)
}

func knownSecretRecoveryValue(fixtureID string) []byte {
	sum := sha256.Sum256([]byte("magelift-secret-recovery\x00" + fixtureID))
	return []byte("fixture:" + hex.EncodeToString(sum[:]))
}

// SecretRecoveryFixtureValue returns the deterministic synthetic value used
// by the shared secret cell. Provider adapters may need it when their API
// generates the source identity (for example a UUID) during creation, before
// the normalized resource reference can be handed to the cell.
func SecretRecoveryFixtureValue(fixtureID string) []byte {
	return knownSecretRecoveryValue(fixtureID)
}

func secretRecoveryFingerprint(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func validateSecretCellContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("secret recovery cell context is required")
	}
	return ctx.Err()
}
