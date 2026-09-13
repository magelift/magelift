package resilience

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestNativeAPISecretRecoveryUsesProtectedSealedArchive(t *testing.T) {
	secrets := newFakeScalewaySecrets()
	secrets.secrets["source-secret"] = SecretMetadata{
		ID: "source-secret", Name: "app", Status: "ready", Protected: true,
		Tags: scalewaySecretTags(operationState{OwnershipMarker: "owner", DataClass: "configuration-secrets"}),
	}
	secrets.values["source-secret"] = []byte("top-secret-value")
	native, err := NewNativeAPIWithServices(newFakeScalewayS3(), nil, secrets, NativeAPIConfig{ArchiveBucket: "archive", RequireObjectLock: true, RetentionDays: 7})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"configuration-secrets"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/secret", OwnershipMarker: "owner", IdempotencyKey: "secret/backup/1",
		ResourceReferences: map[string]string{"configuration-secrets": "scaleway-secret://source-secret"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || !backup.Evidence[0].EncryptionVerified || !backup.Evidence[0].ProtectionVerified || strings.Contains(backup.Evidence[0].Reason, "top-secret-value") {
		t.Fatalf("backup = %#v", backup)
	}
	archiveID := backup.Evidence[0].BackupID
	second, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("idempotent backup: %v", err)
	}
	if second.Status != sdk.ResilienceOperationSucceeded || secrets.createCount != 0 {
		t.Fatalf("idempotent backup = %#v creates=%d", second, secrets.createCount)
	}

	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "secret/restore/1"
	restore.BackupReferences = map[string]string{"configuration-secrets": archiveID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || len(restored.Evidence) != 1 || !restored.Evidence[0].SecretReferencesVerified {
		t.Fatalf("restore = %#v", restored)
	}
	if !bytesEqual(secrets.values[restored.ResourceRefs[0][len("scaleway-secret://"):]], []byte("top-secret-value")) {
		t.Fatalf("restore did not reproduce secret value")
	}

	integrity := request
	integrity.Action = sdk.ResilienceIntegrityCheck
	integrity.IdempotencyKey = "secret/integrity/1"
	checked, err := client.Start(context.Background(), integrity)
	if err != nil {
		t.Fatalf("integrity: %v", err)
	}
	if checked.Status != sdk.ResilienceOperationSucceeded || !checked.Evidence[0].SecretReferencesVerified || !checked.Evidence[0].PermissionsVerified {
		t.Fatalf("integrity = %#v", checked)
	}

	inventory, err := client.Inventory(context.Background(), "owner")
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	foundSecret := false
	for _, resource := range inventory {
		if resource.Identity == "scaleway-secret://source-secret" && resource.Owned && resource.Live {
			foundSecret = true
		}
	}
	if !foundSecret {
		t.Fatalf("inventory = %#v", inventory)
	}
}

func TestNativeAPISecretCleanupRemovesArchiveAndRestoreOutputs(t *testing.T) {
	storage := newFakeScalewayS3()
	secrets := newFakeScalewaySecrets()
	owner := "owner"
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	secrets.secrets["source-secret"] = SecretMetadata{
		ID: "source-secret", Name: "app", Status: "ready", Protected: true,
		Tags: scalewaySecretTags(operationState{OwnershipMarker: owner, DataClass: "configuration-secrets"}),
	}
	secrets.values["source-secret"] = []byte("top-secret-value")
	native, err := NewNativeAPIWithServices(storage, nil, secrets, NativeAPIConfig{ArchiveBucket: "archive", RestoreBucket: "restore", ArchivePrefix: "snapshots", RestorePrefix: "isolated", RequireObjectLock: true, ObjectLockRetention: time.Hour, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := cloudNativeRequestForSecret(sdk.ResilienceBackup, "secret/backup/cleanup").Request
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	request.Action = sdk.ResilienceRestore
	request.IdempotencyKey = "secret/restore/cleanup"
	request.BackupReferences = map[string]string{"configuration-secrets": backup.Evidence[0].BackupID}
	if _, err := client.Start(context.Background(), request); err != nil {
		t.Fatalf("restore: %v", err)
	}
	now = now.Add(2 * time.Hour)
	request.Action = sdk.ResilienceCleanup
	request.IdempotencyKey = "secret/cleanup"
	cleaned, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if cleaned.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("cleanup = %#v", cleaned)
	}
	if len(storage.objects) != 0 {
		t.Fatalf("cleanup left recovery objects: %#v", storage.objects)
	}
	if _, ok := secrets.secrets["source-secret"]; !ok {
		t.Fatal("cleanup deleted the source secret")
	}
}

func TestNativeAPISecretRecoveryRequiresProtectionBeforeMutation(t *testing.T) {
	secrets := newFakeScalewaySecrets()
	secrets.secrets["source-secret"] = SecretMetadata{ID: "source-secret", Name: "app", Status: "ready", Protected: false, Tags: scalewaySecretTags(operationState{OwnershipMarker: "owner", DataClass: "configuration-secrets"})}
	secrets.values["source-secret"] = []byte("value")
	native, err := NewNativeAPIWithServices(newFakeScalewayS3(), nil, secrets, NativeAPIConfig{ArchiveBucket: "archive", RequireObjectLock: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), cloudNativeRequestForSecret(sdk.ResilienceBackup, "secret/backup/1"))
	if err == nil || !strings.Contains(err.Error(), "not protected") || secrets.createCount != 0 {
		t.Fatalf("unprotected backup error = %v creates=%d", err, secrets.createCount)
	}

	native, err = NewNativeAPIWithServices(newFakeScalewayS3(), nil, secrets, NativeAPIConfig{ArchiveBucket: "archive"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), cloudNativeRequestForSecret(sdk.ResilienceBackup, "secret/backup/2"))
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("missing object lock error = %v, want unsupported capability", err)
	}
}

func cloudNativeRequestForSecret(action sdk.ResilienceAction, idempotency string) cloudresilience.NativeOperationRequest {
	operation := "scaleway.secret-manager." + string(action) + ".configuration-secrets"
	return cloudresilience.NativeOperationRequest{Provider: "scaleway", Operation: operation, Request: sdk.ResilienceOperationRequest{
		Action: action, DataClasses: []string{"configuration-secrets"}, FixtureID: "fixture/secret", OwnershipMarker: "owner", IdempotencyKey: idempotency,
		ResourceReferences: map[string]string{"configuration-secrets": "scaleway-secret://source-secret"},
	}}
}

type fakeScalewaySecrets struct {
	secrets     map[string]SecretMetadata
	values      map[string][]byte
	createCount int
}

func newFakeScalewaySecrets() *fakeScalewaySecrets {
	return &fakeScalewaySecrets{secrets: make(map[string]SecretMetadata), values: make(map[string][]byte)}
}

func (fake *fakeScalewaySecrets) Get(_ context.Context, id string) (SecretMetadata, error) {
	secret, ok := fake.secrets[id]
	if !ok {
		return SecretMetadata{}, errors.New("not found")
	}
	secret.Tags = append([]string(nil), secret.Tags...)
	return secret, nil
}

func (fake *fakeScalewaySecrets) List(context.Context) ([]SecretMetadata, error) {
	result := make([]SecretMetadata, 0, len(fake.secrets))
	for _, secret := range fake.secrets {
		secret.Tags = append([]string(nil), secret.Tags...)
		result = append(result, secret)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func (fake *fakeScalewaySecrets) Access(_ context.Context, id, _ string) ([]byte, error) {
	if _, ok := fake.secrets[id]; !ok {
		return nil, errors.New("not found")
	}
	value, ok := fake.values[id]
	if !ok {
		return nil, errors.New("version not found")
	}
	return append([]byte(nil), value...), nil
}

func (fake *fakeScalewaySecrets) Delete(_ context.Context, id string) error {
	if _, ok := fake.secrets[id]; !ok {
		return errors.New("not found")
	}
	delete(fake.secrets, id)
	delete(fake.values, id)
	return nil
}

func (fake *fakeScalewaySecrets) Create(_ context.Context, name string, tags []string, protected bool) (SecretMetadata, error) {
	fake.createCount++
	secret := SecretMetadata{ID: "restored-secret", Name: name, Status: "ready", Protected: protected, Tags: append([]string(nil), tags...)}
	fake.secrets[secret.ID] = secret
	return secret, nil
}

func (fake *fakeScalewaySecrets) CreateVersion(_ context.Context, id string, data []byte) error {
	if _, ok := fake.secrets[id]; !ok {
		return errors.New("not found")
	}
	fake.values[id] = append([]byte(nil), data...)
	return nil
}

func (fake *fakeScalewaySecrets) Protect(_ context.Context, id string) (SecretMetadata, error) {
	secret, err := fake.Get(context.Background(), id)
	if err != nil {
		return SecretMetadata{}, err
	}
	secret.Protected = true
	fake.secrets[id] = secret
	return secret, nil
}

func (fake *fakeScalewaySecrets) Unprotect(_ context.Context, id string) (SecretMetadata, error) {
	secret, err := fake.Get(context.Background(), id)
	if err != nil {
		return SecretMetadata{}, err
	}
	secret.Protected = false
	fake.secrets[id] = secret
	return secret, nil
}

func bytesEqual(left, right []byte) bool {
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
