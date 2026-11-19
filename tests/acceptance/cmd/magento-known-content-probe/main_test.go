package main

import (
	"strings"
	"testing"
)

func TestRunRejectsMissingURLAndExpect(t *testing.T) {
	err := run([]string{"--expect=Fusion Backpack"})
	if err == nil || !strings.Contains(err.Error(), "absolute http") {
		t.Fatalf("error = %v", err)
	}
	err = run([]string{"--url=https://example.invalid/", "--expect="})
	if err == nil || !strings.Contains(err.Error(), "single-line") {
		t.Fatalf("error = %v", err)
	}
	if err := run([]string{"https://example.invalid/"}); err == nil || !strings.Contains(err.Error(), "unexpected positional") {
		t.Fatalf("error = %v", err)
	}
}
