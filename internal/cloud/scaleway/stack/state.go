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
	if region := strings.TrimSpace(spec.Identity.Region); region != "" {
		return region
	}
	return "fr-par"
}

func stateTarget(planned platform.PlannedStack) (s3state.Target, error) {
	p, ok := planned.(Planned)
	if !ok {
		return s3state.Target{}, fmt.Errorf("Scaleway state received unexpected planned type %T", planned)
	}
	bucket := strings.TrimSpace(p.Spec.Dependencies.StateBucket)
	if bucket == "" {
		return s3state.Target{}, fmt.Errorf("Scaleway DIY state requires target.scaleway.stateBucket")
	}
	return s3state.Target{Bucket: bucket, Region: resolveStateRegion(p.Spec), Endpoint: p.Spec.Dependencies.StateEndpoint, Encryption: s3state.ObjectEncryption{Mode: s3state.EncryptionAES256}}, nil
}
