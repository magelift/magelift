package resilience

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/ovh/okms-sdk-go"
)

func TestNewSecretAPIFromClientValidatesOfficialSDKBoundary(t *testing.T) {
	if _, err := NewSecretAPIFromClient(nil, "123e4567-e89b-12d3-a456-426614174000"); err == nil {
		t.Fatal("nil OKMS client was accepted")
	}
	if _, err := NewSecretAPIFromClient(&okms.Client{}, "not-a-uuid"); err == nil {
		t.Fatal("invalid OKMS identity was accepted")
	}
	api, err := NewSecretAPIFromClient(&okms.Client{}, "123e4567-e89b-12d3-a456-426614174000")
	if err != nil {
		t.Fatalf("valid SDK boundary: %v", err)
	}
	if api == nil {
		t.Fatal("valid SDK boundary returned nil")
	}
}

func TestNativeAPIOVHSecretRecoveryIsOwnedIdempotentAndRedacted(t *testing.T) {
	const (
		okmsID = "123e4567-e89b-12d3-a456-426614174000"
		marker = "magelift/test/ovh/configuration-secrets"
	)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	value := []byte(`{"password":"fixture-secret","username":"magento"}`)
	secrets := newFakeOVHSecrets()
	secrets.values["prod/magento"] = fakeOVHSecret{
		metadata: SecretMetadata{
			Path: "prod/magento", State: "active", CurrentVersion: 1,
			CustomMetadata: map[string]string{ovhSecretOwnershipKey: marker, ovhSecretClassKey: "configuration-secrets"},
		},
		value: append([]byte(nil), value...),
	}
	native, err := NewNativeAPIWithDatabaseAndSecrets(newFakeOVHS3(), nil, secrets, NativeAPIConfig{
		ArchiveBucket: "archive", ArchivePrefix: "snapshots", RestoreSecretPrefix: "isolated-secrets",
		SecretOKMSID: okmsID, SecretEndpoint: "https://gra.okms.ovh.net", RequireObjectLock: true, ObjectLockRetention: time.Second, RetentionDays: 7,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceBackup, DataClasses: []string{"configuration-secrets"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture/known-content", OwnershipMarker: marker, IdempotencyKey: "secrets/backup/1",
		ResourceReferences: map[string]string{"configuration-secrets": "ovh-secret://" + okmsID + "/prod/magento"},
	}
	backup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || len(backup.Evidence) != 1 || !backup.Evidence[0].ProtectionVerified {
		t.Fatalf("backup observation = %#v", backup)
	}
	if strings.Contains(fmt.Sprintf("%#v", backup), string(value)) {
		t.Fatalf("backup observation contains secret value: %#v", backup)
	}
	if secrets.createCalls != 0 || secrets.versionCalls != 0 {
		t.Fatalf("backup mutated Secret Manager: creates=%d versions=%d", secrets.createCalls, secrets.versionCalls)
	}

	secondBackup, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("idempotent backup: %v", err)
	}
	if secondBackup.Status != sdk.ResilienceOperationSucceeded || secrets.createCalls != 0 || secrets.versionCalls != 0 {
		t.Fatalf("idempotent backup = %#v, creates=%d versions=%d", secondBackup, secrets.createCalls, secrets.versionCalls)
	}

	restore := request
	restore.Action = sdk.ResilienceRestore
	restore.IdempotencyKey = "secrets/restore/1"
	restore.BackupReferences = map[string]string{"configuration-secrets": backup.Evidence[0].BackupID}
	restored, err := client.Start(context.Background(), restore)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || !strings.HasPrefix(restored.Evidence[0].RestoreID, "ovh-secret://"+okmsID+"/isolated-secrets/") {
		t.Fatalf("restore observation = %#v", restored)
	}
	if secrets.createCalls != 1 || secrets.versionCalls != 0 {
		t.Fatalf("restore mutation = creates=%d versions=%d, want one create and no version", secrets.createCalls, secrets.versionCalls)
	}

	repeated, err := client.Start(context.Background(), restore)
	if err != nil {
		t.Fatalf("idempotent restore: %v", err)
	}
	if repeated.Status != sdk.ResilienceOperationSucceeded || secrets.createCalls != 1 || secrets.versionCalls != 0 {
		t.Fatalf("idempotent restore = %#v, creates=%d versions=%d", repeated, secrets.createCalls, secrets.versionCalls)
	}
	resumed, err := client.Poll(context.Background(), restored.OperationID)
	if err != nil {
		t.Fatalf("resume restore: %v", err)
	}
	if resumed.Status != sdk.ResilienceOperationSucceeded || resumed.OperationID != restored.OperationID {
		t.Fatalf("resumed restore = %#v", resumed)
	}

	integrity := request
	integrity.Action = sdk.ResilienceIntegrityCheck
	integrity.IdempotencyKey = "secrets/integrity/1"
	checked, err := client.Start(context.Background(), integrity)
	if err != nil {
		t.Fatalf("integrity: %v", err)
	}
	if checked.Status != sdk.ResilienceOperationSucceeded || !checked.Evidence[0].ApplicationReadsVerified || !checked.Evidence[0].SecretReferencesVerified {
		t.Fatalf("integrity observation = %#v", checked)
	}

	inventory, err := client.Inventory(context.Background(), marker)
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	var foundRestore bool
	for _, resource := range inventory {
		if strings.HasPrefix(resource.Identity, "ovh-secret://"+okmsID+"/isolated-secrets/") && resource.Owned && resource.Live {
			foundRestore = true
		}
	}
	if !foundRestore {
		t.Fatalf("inventory = %#v, want owned restored secret", inventory)
	}

	now = now.Add(2 * time.Second)
	cleanup := request
	cleanup.Action = sdk.ResilienceCleanup
	cleanup.IdempotencyKey = "secrets/cleanup/1"
	cleaned, err := client.Start(context.Background(), cleanup)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if cleaned.Status != sdk.ResilienceOperationSucceeded || len(cleaned.ResourceRefs) < 2 {
		t.Fatalf("cleanup observation = %#v", cleaned)
	}
	remaining, err := native.Inventory(context.Background(), marker)
	if err != nil {
		t.Fatalf("inventory after cleanup: %v", err)
	}
	for _, resource := range remaining {
		if strings.Contains(resource.Identity, "/isolated-secrets/") {
			t.Fatalf("restore secret remains after cleanup: %#v", remaining)
		}
	}
	if _, err := secrets.Get(context.Background(), "prod/magento"); err != nil {
		t.Fatalf("cleanup deleted source secret: %v", err)
	}
}

func TestNativeAPIOVHSecretRecoveryRequiresArchiveProtection(t *testing.T) {
	const okmsID = "123e4567-e89b-12d3-a456-426614174000"
	marker := "magelift/test/ovh/configuration-secrets"
	secrets := newFakeOVHSecrets()
	secrets.values["prod/magento"] = fakeOVHSecret{metadata: SecretMetadata{
		Path: "prod/magento", State: "active", CurrentVersion: 1,
		CustomMetadata: map[string]string{ovhSecretOwnershipKey: marker, ovhSecretClassKey: "configuration-secrets"},
	}, value: []byte(`{"password":"fixture-secret"}`)}
	native, err := NewNativeAPIWithDatabaseAndSecrets(newFakeOVHS3(), nil, secrets, NativeAPIConfig{ArchiveBucket: "archive", SecretOKMSID: okmsID, SecretEndpoint: "https://gra.okms.ovh.net"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), cloudresilience.NativeOperationRequest{
		Provider: "ovh", Operation: "ovh.secret-reference.backup.configuration-secrets", Request: sdk.ResilienceOperationRequest{
			Action: sdk.ResilienceBackup, DataClasses: []string{"configuration-secrets"}, FixtureID: "fixture", OwnershipMarker: marker, IdempotencyKey: "secrets/backup/unprotected",
			ResourceReferences: map[string]string{"configuration-secrets": "ovh-secret://" + okmsID + "/prod/magento"},
		},
	})
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("error = %v, want unsupported protection capability", err)
	}
	if secrets.createCalls != 0 || secrets.versionCalls != 0 {
		t.Fatalf("unprotected backup mutated Secret Manager: creates=%d versions=%d", secrets.createCalls, secrets.versionCalls)
	}
}

func TestNativeAPIOVHSecretCleanupReportsObjectLockPendingAndRemovesRestoreSecret(t *testing.T) {
	const (
		okmsID = "123e4567-e89b-12d3-a456-426614174000"
		marker = "magelift/test/ovh/pending"
	)
	now := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	retainUntil := now.Add(time.Hour)
	secrets := newFakeOVHSecrets()
	secrets.values["isolated-secrets/restore"] = fakeOVHSecret{metadata: SecretMetadata{
		Path: "isolated-secrets/restore", State: "active", CurrentVersion: 1,
		CustomMetadata: map[string]string{ovhSecretOwnershipKey: marker, ovhSecretClassKey: "configuration-secrets"},
	}, value: []byte(`{"password":"fixture"}`)}
	storage := newFakeOVHS3()
	archiveKey := "magelift/recovery/" + shortDigest(marker) + "/configuration-secrets/secret.json"
	storage.objects["archive\x00"+archiveKey] = fakeOVHObject{
		body: []byte("sealed"), metadata: map[string]string{
			cloudrecovery.OwnershipMetadataKey: marker,
			cloudrecovery.ClassMetadataKey:     "configuration-secrets",
			cloudrecovery.FixtureMetadataKey:   "fixture",
		}, encryption: s3types.ServerSideEncryptionAes256, lockMode: s3types.ObjectLockModeCompliance,
		retain: &retainUntil,
	}
	native, err := NewNativeAPIWithDatabaseAndSecrets(storage, nil, secrets, NativeAPIConfig{
		ArchiveBucket: "archive", RestoreSecretPrefix: "isolated-secrets", SecretOKMSID: okmsID,
		SecretEndpoint: "https://gra.okms.ovh.net", RequireObjectLock: true, RetentionDays: 1,
		Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewNativeResilienceClient(native)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{
		Action: sdk.ResilienceCleanup, DataClasses: []string{"configuration-secrets"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture", OwnershipMarker: marker, IdempotencyKey: "secrets/cleanup/pending",
		ResourceReferences: map[string]string{"configuration-secrets": "ovh-secret://" + okmsID + "/source"},
	}
	_, err = client.Start(context.Background(), request)
	var pending *OVHCleanupPendingError
	if !errors.As(err, &pending) {
		t.Fatalf("cleanup error = %v, want Object Lock pending", err)
	}
	if _, err := secrets.Get(context.Background(), "isolated-secrets/restore"); err == nil {
		t.Fatal("cleanup left owned restore secret while archive was pending")
	}

	now = now.Add(2 * time.Hour)
	cleaned, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatalf("cleanup after retention: %v", err)
	}
	if cleaned.Status != sdk.ResilienceOperationSucceeded {
		t.Fatalf("cleanup after retention = %#v", cleaned)
	}
}

func TestOVHSecretPayloadRoundTripsArbitraryProviderNeutralBytes(t *testing.T) {
	for _, value := range [][]byte{
		[]byte("fixture:plain-text"),
		[]byte(`{"password":"contains whitespace"}`),
		{0x00, 0x01, 0xfe, 0xff},
	} {
		encoded, err := EncodeSecretPayload(value)
		if err != nil {
			t.Fatalf("encode %q: %v", value, err)
		}
		decoded, err := DecodeSecretPayload(encoded)
		if err != nil {
			t.Fatalf("decode %q: %v", value, err)
		}
		if !bytes.Equal(decoded, value) {
			t.Fatalf("round trip = %x, want %x", decoded, value)
		}
	}
}

type fakeOVHSecret struct {
	metadata SecretMetadata
	value    []byte
}

type fakeOVHSecrets struct {
	values       map[string]fakeOVHSecret
	createCalls  int
	versionCalls int
}

func newFakeOVHSecrets() *fakeOVHSecrets {
	return &fakeOVHSecrets{values: make(map[string]fakeOVHSecret)}
}

func (fake *fakeOVHSecrets) Get(_ context.Context, path string) (SecretMetadata, error) {
	secret, ok := fake.values[path]
	if !ok {
		return SecretMetadata{}, errors.New("not found")
	}
	return cloneOVHSecretMetadata(secret.metadata), nil
}

func (fake *fakeOVHSecrets) List(_ context.Context) ([]SecretMetadata, error) {
	paths := make([]string, 0, len(fake.values))
	for path := range fake.values {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]SecretMetadata, 0, len(paths))
	for _, path := range paths {
		result = append(result, cloneOVHSecretMetadata(fake.values[path].metadata))
	}
	return result, nil
}

func (fake *fakeOVHSecrets) Access(_ context.Context, path string, version uint32) ([]byte, error) {
	secret, ok := fake.values[path]
	if !ok {
		return nil, errors.New("not found")
	}
	if version != 0 && version != secret.metadata.CurrentVersion {
		return nil, errors.New("version not found")
	}
	return append([]byte(nil), secret.value...), nil
}

func (fake *fakeOVHSecrets) Delete(_ context.Context, path string) error {
	if _, ok := fake.values[path]; !ok {
		return errors.New("not found")
	}
	delete(fake.values, path)
	return nil
}

func (fake *fakeOVHSecrets) Create(_ context.Context, path string, metadata map[string]string, value []byte) (SecretMetadata, error) {
	if _, ok := fake.values[path]; ok {
		return SecretMetadata{}, errors.New("already exists")
	}
	fake.createCalls++
	secret := fakeOVHSecret{metadata: SecretMetadata{Path: path, State: "active", CurrentVersion: 1, CustomMetadata: cloneOVHSecretMetadataMap(metadata)}, value: append([]byte(nil), value...)}
	fake.values[path] = secret
	return cloneOVHSecretMetadata(secret.metadata), nil
}

func (fake *fakeOVHSecrets) CreateVersion(_ context.Context, path string, value []byte) (SecretMetadata, error) {
	secret, ok := fake.values[path]
	if !ok {
		return SecretMetadata{}, errors.New("not found")
	}
	fake.versionCalls++
	secret.metadata.CurrentVersion++
	secret.metadata.State = "active"
	secret.value = append([]byte(nil), value...)
	fake.values[path] = secret
	return cloneOVHSecretMetadata(secret.metadata), nil
}

func cloneOVHSecretMetadata(metadata SecretMetadata) SecretMetadata {
	metadata.CustomMetadata = cloneOVHSecretMetadataMap(metadata.CustomMetadata)
	return metadata
}

func cloneOVHSecretMetadataMap(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = value
	}
	return result
}

var _ SecretAPI = (*fakeOVHSecrets)(nil)
