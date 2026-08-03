package paasimport

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Result is the output of a PaaS → MageLift map (D-05).
type Result struct {
	YAML     []byte
	Unmapped []UnmappedKey
}

// UnmappedKey is a residual PaaS key that could not be mapped.
type UnmappedKey struct {
	Path   string // dotted path, e.g. hooks.build or stage.deploy.FOO
	Source string // relative config file
}

// HasUnmapped reports whether residual keys remain.
func (r Result) HasUnmapped() bool {
	return len(r.Unmapped) > 0
}

// SidecarPath returns the durable unmapped report path adjacent to yamlPath.
// Default basename magelift.yaml → magelift.unmapped.md; other stems keep their basename.
func SidecarPath(yamlPath string) string {
	dir := filepath.Dir(yamlPath)
	base := filepath.Base(yamlPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		stem = "magelift"
	}
	return filepath.Join(dir, stem+".unmapped.md")
}

// RenderUnmappedReport formats residual keys as Markdown for operators/CI.
func RenderUnmappedReport(keys []UnmappedKey) string {
	var b strings.Builder
	b.WriteString("# MageLift import — unmapped keys\n\n")
	b.WriteString("Import wrote a best-effort `magelift.yaml` but refused success because these PaaS keys could not be mapped (D-05).\n\n")
	if len(keys) == 0 {
		b.WriteString("_No unmapped keys._\n")
		return b.String()
	}
	b.WriteString("| Source | Key |\n| --- | --- |\n")
	for _, k := range keys {
		src := k.Source
		if src == "" {
			src = "?"
		}
		fmt.Fprintf(&b, "| `%s` | `%s` |\n", src, k.Path)
	}
	b.WriteString("\n")
	return b.String()
}
