package certification

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateDocumentationRendersCatalogAndExplicitGaps(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	bundle, err := GenerateDocumentation(CurrentCapabilityCatalog(), nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.GeneratedAt != now.Format(time.RFC3339) {
		t.Fatalf("generatedAt = %q", bundle.GeneratedAt)
	}
	wantFiles := []string{
		"capability-matrix.md", "architecture-matrix.md", "resilience.md",
		"observability.md", "edge.md", "certification.md", "recovery-runbook.md",
	}
	for _, name := range wantFiles {
		if strings.TrimSpace(bundle.Files[name]) == "" {
			t.Fatalf("generated document %q is empty", name)
		}
	}
	capabilities := bundle.Files["capability-matrix.md"]
	for _, value := range []string{
		"gcp.memorystore.valkey-9.0",
		"gcp.memorystore.valkey-9.1",
		"https://docs.cloud.google.com/memorystore/docs/valkey/supported-versions",
		"Preview",
	} {
		if !strings.Contains(capabilities, value) {
			t.Fatalf("capability document does not contain %q", value)
		}
	}
	if !strings.Contains(bundle.Files["observability.md"], "no native or New Relic delivery claim") {
		t.Fatal("observability document published a claim without evidence")
	}
	if !strings.Contains(bundle.Files["recovery-runbook.md"], "stale-origin fencing") {
		t.Fatal("recovery runbook omitted stale-origin fencing")
	}
}

func TestGenerateDocumentationRequiresSealedEvidence(t *testing.T) {
	record := testCellRecord(StatusFail)
	record.Reason = "live credentials were not admitted"
	if err := record.Seal(); err != nil {
		t.Fatal(err)
	}
	record.RecordDigest = ""
	_, err := GenerateDocumentation(CurrentCapabilityCatalog(), []Record{record}, time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "not sealed") {
		t.Fatalf("unsealed evidence was accepted: %v", err)
	}
}

func TestGenerateDocumentationIncludesOnlyValidatedEvidenceClaims(t *testing.T) {
	record := testCellRecord(StatusSkip)
	record.Reason = "provider admission was blocked"
	if err := record.Seal(); err != nil {
		t.Fatal(err)
	}
	bundle, err := GenerateDocumentation(CurrentCapabilityCatalog(), []Record{record}, time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	certification := bundle.Files["certification.md"]
	if !strings.Contains(certification, "`SKIP`") || !strings.Contains(certification, record.RecordDigest) {
		t.Fatalf("certification document omitted sealed record: %s", certification)
	}
}
