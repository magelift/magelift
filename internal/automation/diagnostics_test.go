package automation

import (
	"fmt"
	"strings"
	"testing"
)

func TestDiagnosticsBoundAndRedactChunkedOutput(t *testing.T) {
	var d Diagnostics
	for i := range 60 {
		fmt.Fprintf(&d, "resource %d\n", i)
	}
	_, _ = d.Write([]byte("Authorization: Bea"))
	_, _ = d.Write([]byte("rer private-credential-abcdefghijkl\n"))
	_, _ = d.Write([]byte(strings.Repeat("x", 8000)))
	lines := d.Drain()
	joined := strings.Join(lines, "\n")
	if len(lines) > 50 || strings.Contains(joined, "private-credential") || !strings.Contains(joined, "resource 59") || !strings.Contains(joined, "[long diagnostic line omitted]") {
		t.Fatalf("wrong progress tail: %s", joined)
	}
	if len(d.Lines()) != 0 {
		t.Fatal("drained progress repeated in the next phase")
	}
}
