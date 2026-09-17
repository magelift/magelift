package providerhost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGenerateFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	build := filepath.Join(dist, "magelift-provider-gcp_linux_amd64_v1")
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := "magelift-provider-gcp_1.2.3_linux_amd64"
	bundle := binary + ".sigstore.json"
	if err := os.WriteFile(filepath.Join(build, binary), []byte("fake-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(build, bundle), []byte("fake-bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("c", 64)
	checksums := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  magelift_1.2.3_linux_amd64.tar.gz\n" +
		digest + "  " + binary + "\n" +
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  " + binary + ".sbom.json\n"
	if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), []byte(checksums), 0o644); err != nil {
		t.Fatal(err)
	}
	artifacts := []map[string]string{
		{"name": binary, "path": filepath.Join("dist", "magelift-provider-gcp_linux_amd64_v1", binary), "type": "Binary"},
		{"name": bundle, "path": filepath.Join("dist", "magelift-provider-gcp_linux_amd64_v1", bundle), "type": "Signature"},
	}
	encoded, err := json.Marshal(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "artifacts.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	return dist
}

func TestGenerateLockfilesEmitsLoadableSchema(t *testing.T) {
	locks, err := GenerateLockfiles(writeGenerateFixture(t), "v1.2.3")
	if err != nil {
		t.Fatalf("GenerateLockfiles: %v", err)
	}
	lock, ok := locks["linux_amd64"]
	if !ok {
		t.Fatalf("platforms = %v, want linux_amd64", locks)
	}
	if lock.SchemaVersion != SchemaVersion || lock.SDKAPIVersion != SDKAPIVersion {
		t.Fatalf("lock = %+v, want schema %d api %q", lock, SchemaVersion, SDKAPIVersion)
	}
	artifact, err := lock.Artifact("gcp")
	if err != nil {
		t.Fatalf("Artifact: %v", err)
	}
	if artifact.Protocol != ProtocolV1Marker || artifact.Version != "v1.2.3" {
		t.Fatalf("artifact = %+v", artifact)
	}
	if !strings.HasSuffix(artifact.Cosign.Identity, "@refs/tags/v1.2.3") {
		t.Fatalf("identity = %q", artifact.Cosign.Identity)
	}
	// The emitted lockfile parses under the loader's own rules.
	encoded, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseLock(strings.NewReader(string(encoded))); err != nil {
		t.Fatalf("generated lockfile does not parse: %v", err)
	}
}

func TestGenerateLockfilesRefusesMissingBundle(t *testing.T) {
	dist := writeGenerateFixture(t)
	artifacts := []map[string]string{
		{"name": "magelift-provider-gcp_1.2.3_linux_amd64", "path": "dist/x", "type": "Binary"},
	}
	encoded, err := json.Marshal(artifacts)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "artifacts.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateLockfiles(dist, "v1.2.3"); err == nil || !strings.Contains(err.Error(), "missing bundle") {
		t.Fatalf("err = %v, want missing bundle", err)
	}
}

func TestGenerateLockfilesRefusesEmptyDist(t *testing.T) {
	dist := t.TempDir()
	if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  magelift_1.2.3_linux_amd64.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "artifacts.json"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := GenerateLockfiles(dist, "v1.2.3"); err == nil || !strings.Contains(err.Error(), "no provider artifacts") {
		t.Fatalf("err = %v, want no-provider refusal", err)
	}
}

func TestGenerateLockfilesRequiresTag(t *testing.T) {
	if _, err := GenerateLockfiles(writeGenerateFixture(t), " "); err == nil {
		t.Fatal("empty tag was accepted")
	}
}
