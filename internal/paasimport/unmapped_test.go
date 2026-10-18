package paasimport

import (
	"strings"
	"testing"
)

func TestRenderUnmappedReportGuidesEachFamily(t *testing.T) {
	keys := []UnmappedKey{
		{Source: ".magento.app.yaml", Path: "hooks.deploy"},
		{Source: ".magento.app.yaml", Path: "crons.shell-cleanup"},
		{Source: ".magento.app.yaml", Path: "relationships.database"},
		{Source: ".magento.app.yaml", Path: "runtime.extensions"},
		{Source: ".magento.app.yaml", Path: "dependencies.php.foo/bar"},
		{Source: ".magento.env.yaml", Path: "stage.build.CUSTOM_FLAG"},
		{Source: ".magento.app.yaml", Path: "mounts.var"},
	}
	report := RenderUnmappedReport(keys)
	for _, want := range []string{
		"### hooks",
		"### crons",
		"### relationships",
		"### runtime",
		"### dependencies",
		"### stage",
		"### other",
		"Before (ACC shape)",
		"After (MageLift shape",
		"executable: magento",
		"docs/migrating-from-paas.md",
		"magelift config validate",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
	if got := strings.Count(report, "### hooks"); got != 1 {
		t.Fatalf("hooks family rendered %d times, want once", got)
	}
}

func TestRenderUnmappedReportEchoesNoValues(t *testing.T) {
	// Values from the ACC unmapped fixture: names may appear, values must not.
	report := RenderUnmappedReport([]UnmappedKey{
		{Source: ".magento.env.yaml", Path: "stage.build.CUSTOM_FEATURE_FLAG"},
		{Source: ".magento.env.yaml", Path: "stage.build.WEIRD_DEPLOY_HOOK"},
	})
	for _, leaked := range []string{"must-appear-in-unmapped-report", "also-unmapped"} {
		if strings.Contains(report, leaked) {
			t.Fatalf("report echoes PaaS value %q:\n%s", leaked, report)
		}
	}
	for _, name := range []string{"CUSTOM_FEATURE_FLAG", "WEIRD_DEPLOY_HOOK"} {
		if !strings.Contains(report, name) {
			t.Fatalf("report missing key name %q:\n%s", name, report)
		}
	}
}
