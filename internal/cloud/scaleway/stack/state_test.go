package stack

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/s3state"
)

func TestStateTargetPreservesBucketRegionAndEncryption(t *testing.T) {
	p := Planned{Spec: Spec{Dependencies: Dependencies{StateBucket: "shop-state", StateRegion: "test-region", StateEndpoint: "http://localhost:4566"}}}
	target, err := stateTarget(p)
	if err != nil {
		t.Fatal(err)
	}
	if target.Bucket != "shop-state" || target.Region != "test-region" || target.Endpoint != "http://localhost:4566" || target.Encryption.Mode != s3state.EncryptionAES256 || target.Encryption.KMSKeyARN != "" {
		t.Fatalf("state target = %#v", target)
	}
}
func TestStateLockRequiresConfiguredBucket(t *testing.T) {
	_, err := (Module{}).State().Lock(t.Context(), Planned{}, "operator")
	if err == nil || !strings.Contains(err.Error(), "target.scaleway.stateBucket") {
		t.Fatalf("missing state bucket: %v", err)
	}
}
