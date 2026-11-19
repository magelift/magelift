package stack

import (
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/s3state"
)

func resolveStateRegion(spec Spec) string {
	if region := strings.TrimSpace(spec.Dependencies.StateRegion); region != "" {
		return region
	}
	// OVH Public Cloud regions (GRA9) are not AWS region codes; default to a
	// Floci/S3-compatible region and let operators override via stateRegion.
	return "us-east-1"
}

func stateTarget(planned platform.PlannedStack) (s3state.Target, error) {
	p, ok := planned.(Planned)
	if !ok {
		return s3state.Target{}, fmt.Errorf("OVH state received unexpected planned type %T", planned)
	}
	bucket := strings.TrimSpace(p.Spec.Dependencies.StateBucket)
	if bucket == "" {
		return s3state.Target{}, fmt.Errorf("OVH DIY state requires target.ovh.stateBucket")
	}
	return s3state.Target{Bucket: bucket, Region: resolveStateRegion(p.Spec), Endpoint: p.Spec.Dependencies.StateEndpoint, Encryption: s3state.ObjectEncryption{Mode: s3state.EncryptionAES256}}, nil
}
