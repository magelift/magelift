package stack

import (
	"testing"

	gcptarget "github.com/magelift/magelift/internal/cloud/gcp/target"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

func TestCertificationTierFollowsEvidenceTuple(t *testing.T) {
	t.Parallel()
	preview := Spec{
		Identity:    Identity{Runtime: gcptarget.RuntimeAutopilotID, Preset: sdk.PresetPreview},
		Application: Application{Version: "2.4.9"},
	}
	if got := (Planned{Spec: preview}).CertificationTier(); got != platform.TierCertified {
		t.Fatalf("autopilot preview 2.4.9 = %s, want certified", got)
	}
	ha := preview
	ha.Identity.Preset = sdk.PresetHighAvailability
	if got := (Planned{Spec: ha}).CertificationTier(); got != platform.TierExperimental {
		t.Fatalf("autopilot HA = %s, want experimental", got)
	}
	std := preview
	std.Identity.Runtime = "gke-standard"
	if got := (Planned{Spec: std}).CertificationTier(); got != platform.TierExperimental {
		t.Fatalf("gke-standard = %s, want experimental", got)
	}
}
