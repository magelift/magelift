package certification

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestBuildAndDecodeRecoveryFixtureManifestHashesKnownContent(t *testing.T) {
	manifest, err := BuildRecoveryFixtureManifest("recovery-known-content", "testdata/fixtures/recovery", []RecoveryFixtureMaterial{
		{Name: "database", ExpectedRecords: 2, Manifest: []byte("schema-v1"), Content: []byte("product=known\n"), Permissions: []byte("app=read-write"), SecretReferences: []string{"aws-secrets-manager://test/database"}, MaxRestoreDurationSeconds: 30},
		{Name: "media", ExpectedObjects: 1, Manifest: []byte("media-manifest-v1"), Content: []byte("fixture-media"), Permissions: []byte("app=read"), MaxRestoreDurationSeconds: 30},
	})
	if err != nil {
		t.Fatalf("BuildRecoveryFixtureManifest() error = %v", err)
	}
	if manifest.Classes[0].ManifestDigest == "" || string(manifest.Classes[0].ManifestDigest) == "schema-v1" {
		t.Fatalf("manifest did not hash known content: %#v", manifest.Classes[0])
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	decoded, err := DecodeRecoveryFixtureManifest(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("DecodeRecoveryFixtureManifest() error = %v", err)
	}
	if decoded.ID != manifest.ID || decoded.Classes[1].ContentDigest != manifest.Classes[1].ContentDigest {
		t.Fatalf("decoded manifest = %#v, want %#v", decoded, manifest)
	}
}

func TestBuildRecoveryFixtureManifestFromFSIsDeterministicAndHashesFileIdentity(t *testing.T) {
	filesystem := fstest.MapFS{
		"root/database.sql":       &fstest.MapFile{Data: []byte("create table products;\n"), Mode: 0o640},
		"root/media/catalog.json": &fstest.MapFile{Data: []byte(`{"sku":"known"}\n`), Mode: 0o644},
	}
	files := []RecoveryFixtureFile{
		{Name: "media", Path: "media/catalog.json", ExpectedObjects: 1, MaxRestoreDurationSeconds: 30},
		{Name: "database", Path: "database.sql", ExpectedRecords: 1, SecretReferences: []string{"aws-secrets-manager://magelift/test/db"}, MaxRestoreDurationSeconds: 30},
	}
	manifest, err := BuildRecoveryFixtureManifestFromFS(filesystem, "fixture", "testdata/fixtures/recovery", "root", files)
	if err != nil {
		t.Fatalf("BuildRecoveryFixtureManifestFromFS() error = %v", err)
	}
	if got, want := []string{manifest.Classes[0].Name, manifest.Classes[1].Name}, []string{"database", "media"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("classes = %v, want %v", got, want)
	}
	if got, want := manifest.Classes[0].ManifestDigest, digestBytes([]byte("database.sql\x00-rw-r-----")); got != want {
		t.Fatalf("database manifest digest = %q, want %q", got, want)
	}
	if got, want := manifest.Classes[1].ContentDigest, digestBytes([]byte(`{"sku":"known"}\n`)); got != want {
		t.Fatalf("media content digest = %q, want %q", got, want)
	}
	reversed := append([]RecoveryFixtureFile(nil), files...)
	reversed[0], reversed[1] = reversed[1], reversed[0]
	reordered, err := BuildRecoveryFixtureManifestFromFS(filesystem, "fixture", "testdata/fixtures/recovery", "root", reversed)
	if err != nil {
		t.Fatalf("BuildRecoveryFixtureManifestFromFS(reordered) error = %v", err)
	}
	if !reflect.DeepEqual(manifest, reordered) {
		t.Fatalf("reordered manifest = %#v, want %#v", reordered, manifest)
	}
}

func TestBuildRecoveryFixtureManifestFromFSRejectsTraversal(t *testing.T) {
	filesystem := fstest.MapFS{"root/database.sql": &fstest.MapFile{Data: []byte("fixture"), Mode: 0o640}}
	_, err := BuildRecoveryFixtureManifestFromFS(filesystem, "fixture", "test", "root", []RecoveryFixtureFile{{Name: "database", Path: "../database.sql"}})
	if err == nil || !strings.Contains(err.Error(), "stay below the root") {
		t.Fatalf("path traversal accepted: %v", err)
	}
}

func TestRepositoryRecoveryFixtureIsScrubbedAndManifestable(t *testing.T) {
	filesystem := os.DirFS("../..")
	manifest, err := BuildRecoveryFixtureManifestFromFS(filesystem, "fixture/known-content", "testdata/fixtures/recovery", "testdata/fixtures/recovery", []RecoveryFixtureFile{
		{Name: "configuration-secrets", Path: "configuration.env.template", SecretReferences: []string{"aws-secrets-manager://magelift/recovery/database-password"}},
		{Name: "database", Path: "database.sql", ExpectedRecords: 2},
		{Name: "media", Path: "media/catalog.json", ExpectedObjects: 1},
		{Name: "queue", Path: "queue.jsonl", ExpectedRecords: 2},
	})
	if err != nil {
		t.Fatalf("build repository recovery fixture manifest: %v", err)
	}
	if err := ValidateSecretSafeValue(manifest); err != nil {
		t.Fatalf("repository recovery fixture contains secret-like material: %v", err)
	}
	if got, want := len(manifest.Classes), 4; got != want {
		t.Fatalf("repository recovery fixture classes = %d, want %d", got, want)
	}
}

func TestDecodeRecoveryFixtureManifestRejectsTrailingData(t *testing.T) {
	manifest := RecoveryFixtureManifest{ID: "fixture", Source: "test", Classes: []RecoveryFixtureClass{{Name: "database", ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest}}}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	_, err = DecodeRecoveryFixtureManifest(bytes.NewBufferString(string(encoded) + "{}"))
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("error = %v, want trailing-data error", err)
	}
}

const recoveryFixtureDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestRecoveryFixtureVerifiesKnownContentAcrossAllDataClasses(t *testing.T) {
	manifest := RecoveryFixtureManifest{ID: "fixture-2026-08", Source: "test-known-content", Classes: []RecoveryFixtureClass{
		{Name: "database", ExpectedRecords: 3, ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest, SecretReferences: []string{"aws-secrets-manager://magelift/test/db"}, MaxRestoreDurationSeconds: 60},
		{Name: "media", ExpectedObjects: 2, ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest, MaxRestoreDurationSeconds: 60},
	}}
	observations := []RecoveryFixtureObservation{
		{Name: "database", Records: 3, ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest, SecretReferences: []string{"aws-secrets-manager://magelift/test/db"}, PermissionsOK: true, ApplicationReadsOK: true, SecretReferencesResolved: true, ServiceHealthy: true, RestoreDurationSeconds: 12},
		{Name: "media", Objects: 2, ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest, PermissionsOK: true, ApplicationReadsOK: true, SecretReferencesResolved: true, ServiceHealthy: true, RestoreDurationSeconds: 10},
	}
	if err := VerifyRecoveryFixture(manifest, observations); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryFixtureRejectsUnverifiedApplicationReadsAndDuration(t *testing.T) {
	manifest := RecoveryFixtureManifest{ID: "fixture", Source: "test", Classes: []RecoveryFixtureClass{{Name: "database", ExpectedRecords: 1, ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest, MaxRestoreDurationSeconds: 5}}}
	err := VerifyRecoveryFixture(manifest, []RecoveryFixtureObservation{{Name: "database", Records: 1, ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest, PermissionsOK: true, SecretReferencesResolved: true, ServiceHealthy: true, RestoreDurationSeconds: 6}})
	if err == nil || !strings.Contains(err.Error(), "application read") || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("unverified or slow restore accepted: %v", err)
	}
}

func TestRecoveryFixtureRejectsInlineSecretAndPartialRestore(t *testing.T) {
	manifest := RecoveryFixtureManifest{ID: "fixture", Source: "test", Classes: []RecoveryFixtureClass{{Name: "database", ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest, SecretReferences: []string{"plaintext-secret"}}}}
	if err := manifest.Validate(); err == nil || !strings.Contains(err.Error(), "secret reference") {
		t.Fatalf("inline secret accepted: %v", err)
	}
	manifest.Classes[0].SecretReferences = []string{"gcp-secret-manager://magelift/test/db"}
	err := VerifyRecoveryFixture(manifest, nil)
	if err == nil || !strings.Contains(err.Error(), "missing recovered data class") {
		t.Fatalf("partial restore accepted: %v", err)
	}
}

func TestRecoveryFixtureRejectsMismatchedSecretReferences(t *testing.T) {
	manifest := RecoveryFixtureManifest{ID: "fixture", Source: "test", Classes: []RecoveryFixtureClass{{Name: "database", ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest, SecretReferences: []string{"aws-secrets-manager://magelift/test/db"}}}}
	err := VerifyRecoveryFixture(manifest, []RecoveryFixtureObservation{{Name: "database", ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest, SecretReferences: []string{"aws-secrets-manager://magelift/test/other-db"}, PermissionsOK: true, ApplicationReadsOK: true, SecretReferencesResolved: true, ServiceHealthy: true, RestoreDurationSeconds: 1}})
	if err == nil || !strings.Contains(err.Error(), "secret references do not match") {
		t.Fatalf("mismatched secret references accepted: %v", err)
	}
}

func TestRecoveryFixtureRejectsInlineSecretInObservation(t *testing.T) {
	manifest := RecoveryFixtureManifest{ID: "fixture", Source: "test", Classes: []RecoveryFixtureClass{{Name: "database", ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest, SecretReferences: []string{"aws-secrets-manager://magelift/test/db"}}}}
	observation := RecoveryFixtureObservation{
		Name: "database", ManifestDigest: recoveryFixtureDigest, ContentDigest: recoveryFixtureDigest, PermissionDigest: recoveryFixtureDigest,
		SecretReferences: []string{"password=plaintext-value"}, PermissionsOK: true, ApplicationReadsOK: true, SecretReferencesResolved: true, ServiceHealthy: true, RestoreDurationSeconds: 1,
	}
	err := VerifyRecoveryFixture(manifest, []RecoveryFixtureObservation{observation})
	if err == nil || !strings.Contains(err.Error(), "secret reference") {
		t.Fatalf("inline observation secret accepted: %v", err)
	}
}
