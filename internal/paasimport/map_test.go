package paasimport

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/config"
)

func TestMapACCSupportedFixture(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "acc", "supported")
	result, err := MapACC(root)
	if err != nil {
		t.Fatalf("MapACC: %v", err)
	}
	if len(result.Unmapped) != 0 {
		t.Fatalf("supported ACC must have zero unmapped, got %#v", result.Unmapped)
	}
	if strings.Contains(string(result.YAML), "synthetic-crypt-must-not-leak") {
		t.Fatal("crypt plaintext leaked into generated magelift.yaml")
	}
	file, err := config.Load(result.YAML)
	if err != nil {
		t.Fatalf("config.Load: %v\nYAML:\n%s", err, result.YAML)
	}
	envs := file.Environments()
	if len(envs) == 0 {
		t.Fatal("expected at least one environment")
	}
	if _, err := file.ResolveBuild(); err != nil {
		t.Fatalf("ResolveBuild: %v", err)
	}
	for _, env := range envs {
		if _, err := file.Resolve(env, config.ResolveOptions{}); err != nil {
			t.Fatalf("Resolve(%s): %v", env, err)
		}
	}
	text := string(result.YAML)
	for _, want := range []string{
		"name: sample-shop",
		`php: "8.5"`,
		"strategy: compact",
		"encryptionKeySecretArn:",
		"domain: staging.example.com",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated YAML missing %q:\n%s", want, text)
		}
	}
}

func TestMapUpsunSupportedFixture(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "upsun", "supported")
	result, err := MapUpsun(root)
	if err != nil {
		t.Fatalf("MapUpsun: %v", err)
	}
	if len(result.Unmapped) != 0 {
		t.Fatalf("supported Upsun fixture must have zero unmapped keys, got %#v", result.Unmapped)
	}
	if strings.Contains(string(result.YAML), "upsun-crypt-must-not-leak") {
		t.Fatal("crypt plaintext leaked into generated magelift.yaml")
	}
	file, err := config.Load(result.YAML)
	if err != nil {
		t.Fatalf("config.Load: %v\nYAML:\n%s", err, result.YAML)
	}
	if _, err := file.ResolveBuild(); err != nil {
		t.Fatalf("ResolveBuild: %v", err)
	}
	text := string(result.YAML)
	for _, want := range []string{
		"name: upsun-sample-shop",
		`php: "8.5"`,
		"strategy: standard",
		"threads: 4",
		"encryptionKeySecretArn:",
		"searchMode:",
		"queueMode:",
		"domain: preview.example.com",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated YAML missing %q:\n%s", want, text)
		}
	}
}

func TestMapACCUnmappedFixture(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "acc", "unmapped")
	result, err := MapACC(root)
	if err != nil {
		t.Fatalf("MapACC: %v", err)
	}
	if len(result.Unmapped) == 0 {
		t.Fatal("expected unmapped keys for ACC unmapped fixture")
	}
	if _, err := config.Load(result.YAML); err != nil {
		t.Fatalf("best-effort YAML must still Load: %v\n%s", err, result.YAML)
	}
	report := RenderUnmappedReport(result.Unmapped)
	for _, want := range []string{
		"hooks.build",
		"hooks.deploy",
		"crons.shell-cleanup",
		"CUSTOM_FEATURE_FLAG",
		"WEIRD_DEPLOY_HOOK",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("unmapped report missing %q:\n%s", want, report)
		}
	}
	if strings.Contains(string(result.YAML), "synthetic-crypt-must-not-leak") {
		t.Fatal("crypt plaintext leaked")
	}
}

func TestMapUpsunUnmappedFixture(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "upsun", "unmapped")
	result, err := MapUpsun(root)
	if err != nil {
		t.Fatalf("MapUpsun: %v", err)
	}
	if len(result.Unmapped) == 0 {
		t.Fatal("expected unmapped keys")
	}
	report := RenderUnmappedReport(result.Unmapped)
	for _, want := range []string{
		"hooks.build",
		"crons.shell-backup",
		"PLATFORM_CUSTOM_VAR",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("unmapped report missing %q:\n%s", want, report)
		}
	}
}

func TestAllowlistEnvKeys(t *testing.T) {
	for _, key := range []string{"CRYPT_KEY", "SCD_STRATEGY", "SCD_THREADS", "UPDATE_URLS"} {
		if !IsAllowlistedEnv(key) {
			t.Fatalf("%s must be on D-07 allowlist", key)
		}
	}
	if IsAllowlistedEnv("CUSTOM_FEATURE_FLAG") {
		t.Fatal("CUSTOM_FEATURE_FLAG must not be allowlisted")
	}
}

func TestSidecarPath(t *testing.T) {
	if got := SidecarPath("/tmp/magelift.yaml"); got != "/tmp/magelift.unmapped.md" {
		t.Fatalf("SidecarPath magelift.yaml = %q", got)
	}
	if got := SidecarPath("/tmp/review.magelift.yaml"); got != "/tmp/review.magelift.unmapped.md" {
		t.Fatalf("SidecarPath config-out = %q", got)
	}
}

func TestFullyMappedNeverWritesSidecarContent(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "acc", "supported")
	result, err := MapACC(root)
	if err != nil {
		t.Fatalf("MapACC: %v", err)
	}
	if result.HasUnmapped() {
		t.Fatal("fully mapped result must not report unmapped keys")
	}
}

func TestDetectACCRequiresAppYAML(t *testing.T) {
	dir := t.TempDir()
	if err := DetectACC(dir); err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func TestDetectUpsunRequiresAppYAML(t *testing.T) {
	dir := t.TempDir()
	if err := DetectUpsun(dir); err == nil {
		t.Fatal("expected error for empty directory")
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s: %v", root, err)
	}
	return root
}
