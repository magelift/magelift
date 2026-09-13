package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCertificationPlanCommandIsSideEffectFree(t *testing.T) {
	var out bytes.Buffer
	o := testOptions(&out, nil)
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--output", "json", "certification", "plan", "--target", "gcp/gke-standard", "--release", "2.4.9"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{`"cellCount"`, `"warmGroups"`, `"statusCounts"`, `"not-calculated"`, `"teardownScope"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("plan output missing %q: %s", want, text)
		}
	}
}

func TestCertificationDocsCommandWritesAllGeneratedDocuments(t *testing.T) {
	var out bytes.Buffer
	o := testOptions(&out, nil)
	outputDir := t.TempDir()
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--output", "json", "certification", "docs", "--output-dir", outputDir})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"capability-matrix.md", "architecture-matrix.md", "resilience.md",
		"observability.md", "edge.md", "certification.md", "recovery-runbook.md",
	} {
		data, err := os.ReadFile(filepath.Join(outputDir, name))
		if err != nil {
			t.Fatalf("read generated document %q: %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("generated document %q is empty", name)
		}
	}
}
