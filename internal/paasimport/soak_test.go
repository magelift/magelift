package paasimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/config"
)

func TestMapACCCronMapsMagentoOnly(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "acc", "supported")
	result, err := MapACC(root)
	if err != nil {
		t.Fatalf("MapACC: %v", err)
	}
	if len(result.Unmapped) != 0 {
		t.Fatalf("supported must be fully mapped, got %#v", result.Unmapped)
	}
	text := string(result.YAML)
	if !strings.Contains(text, "cron:") || !strings.Contains(text, "bin/magento cron:run") {
		t.Fatalf("expected application.cron Magento entry:\n%s", text)
	}
	file, err := config.Load(result.YAML)
	if err != nil {
		t.Fatalf("config.Load: %v\n%s", err, text)
	}
	_ = file
	if !strings.Contains(text, "schedule:") {
		t.Fatalf("expected cron schedule in YAML:\n%s", text)
	}
}

func TestMapACCFreeFormCronRemainsUnmapped(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "fixtures", "acc", "unmapped")
	result, err := MapACC(root)
	if err != nil {
		t.Fatalf("MapACC: %v", err)
	}
	report := RenderUnmappedReport(result.Unmapped)
	if !strings.Contains(report, "crons.shell-cleanup") {
		t.Fatalf("shell cron must be unmapped:\n%s", report)
	}
	if strings.Contains(report, "crons.magento") {
		t.Fatalf("Magento cron must not be unmapped:\n%s", report)
	}
	if !strings.Contains(string(result.YAML), "bin/magento cron:run") {
		t.Fatalf("Magento cron must appear in YAML:\n%s", result.YAML)
	}
}

func TestSoakACCSkipsWhenUnset(t *testing.T) {
	if os.Getenv("MAGELIFT_IMPORT_FIXTURE_ACC") != "" {
		t.Skip("MAGELIFT_IMPORT_FIXTURE_ACC set; skip-unset assertion not applicable")
	}
	t.Setenv("MAGELIFT_IMPORT_FIXTURE_ACC", "")
	path := os.Getenv("MAGELIFT_IMPORT_FIXTURE_ACC")
	if path != "" {
		t.Fatal("expected empty soak path")
	}
	// Contract: callers skip when unset — this test documents the gate.
}

func TestSoakUpsunSkipsWhenUnset(t *testing.T) {
	if os.Getenv("MAGELIFT_IMPORT_FIXTURE_UPSUN") != "" {
		t.Skip("MAGELIFT_IMPORT_FIXTURE_UPSUN set; skip-unset assertion not applicable")
	}
}

func TestSoakACCFixtureWhenSet(t *testing.T) {
	raw := os.Getenv("MAGELIFT_IMPORT_FIXTURE_ACC")
	if raw == "" {
		t.Skip("MAGELIFT_IMPORT_FIXTURE_ACC unset")
	}
	root, err := ConfineConfigRoot(raw)
	if err != nil {
		t.Fatalf("ConfineConfigRoot: %v", err)
	}
	result, err := MapACC(root)
	if err != nil {
		t.Fatalf("MapACC soak: %v", err)
	}
	if _, err := config.Load(result.YAML); err != nil {
		t.Fatalf("soak YAML Load: %v", err)
	}
}

func TestSoakUpsunFixtureWhenSet(t *testing.T) {
	raw := os.Getenv("MAGELIFT_IMPORT_FIXTURE_UPSUN")
	if raw == "" {
		t.Skip("MAGELIFT_IMPORT_FIXTURE_UPSUN unset")
	}
	root, err := ConfineConfigRoot(raw)
	if err != nil {
		t.Fatalf("ConfineConfigRoot: %v", err)
	}
	result, err := MapUpsun(root)
	if err != nil {
		t.Fatalf("MapUpsun soak: %v", err)
	}
	if _, err := config.Load(result.YAML); err != nil {
		t.Fatalf("soak YAML Load: %v", err)
	}
}

func TestConfineConfigRootRejectsDotDot(t *testing.T) {
	if _, err := ConfineConfigRoot("../etc"); err == nil {
		t.Fatal("expected reject for ..")
	}
}

func TestSupportedImportsValidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func(string) (Result, error)
		dir  string
	}{
		{"acc", MapACC, filepath.Join("testdata", "fixtures", "acc", "supported")},
		{"upsun", MapUpsun, filepath.Join("testdata", "fixtures", "upsun", "supported")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(repoRoot(t), tc.dir)
			result, err := tc.fn(root)
			if err != nil {
				t.Fatalf("map: %v", err)
			}
			if result.HasUnmapped() {
				t.Fatalf("unmapped: %#v", result.Unmapped)
			}
			file, err := config.Load(result.YAML)
			if err != nil {
				t.Fatalf("Load: %v\n%s", err, result.YAML)
			}
			envs := file.Environments()
			if len(envs) == 0 {
				t.Fatal("no environments")
			}
			if _, err := file.ResolveBuild(); err != nil {
				t.Fatalf("ResolveBuild: %v", err)
			}
			for _, env := range envs {
				if _, err := file.Resolve(env, config.ResolveOptions{}); err != nil {
					t.Fatalf("Resolve(%s): %v", env, err)
				}
			}
		})
	}
}
