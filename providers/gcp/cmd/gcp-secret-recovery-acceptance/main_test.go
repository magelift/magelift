package main

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestParseRecoveryDestination(t *testing.T) {
	destination, approval, err := parseRecoveryDestination("in-place")
	if err != nil || destination != sdk.RecoverySameRegion || approval != "gcp-secret-recovery-acceptance-in-place" {
		t.Fatalf("in-place destination = %q approval=%q err=%v", destination, approval, err)
	}
	destination, approval, err = parseRecoveryDestination("isolated")
	if err != nil || destination != sdk.RecoverySameRegionIsolated || approval != "" {
		t.Fatalf("isolated destination = %q approval=%q err=%v", destination, approval, err)
	}
	if _, _, err := parseRecoveryDestination("somewhere"); err == nil || !strings.Contains(err.Error(), "destination must be isolated or in-place") {
		t.Fatalf("unknown destination error = %v", err)
	}
}
