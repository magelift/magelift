package stack

import (
	"testing"

	"github.com/magelift/magelift/internal/platform"
	gcptarget "github.com/magelift/magelift/providers/gcp/target"
	"github.com/magelift/magelift/sdk"
)

func TestCertificationTierFollowsEvidenceTuple(t *testing.T) {
	t.Parallel()
	preview := Spec{
		Identity:    Identity{Runtime: gcptarget.RuntimeAutopilotID, Preset: sdk.PresetPreview},
		Application: Application{Version: "2.4.9"},
	}
	if got := preview.CertificationTier(); got != platform.TierCertified {
		t.Fatalf("autopilot preview 2.4.9 = %s, want certified", got)
	}
	ha := preview
	ha.Identity.Preset = sdk.PresetHighAvailability
	if got := ha.CertificationTier(); got != platform.TierExperimental {
		t.Fatalf("autopilot HA = %s, want experimental", got)
	}
	std := preview
	std.Identity.Runtime = "gke-standard"
	if got := std.CertificationTier(); got != platform.TierExperimental {
		t.Fatalf("gke-standard = %s, want experimental", got)
	}
}

func TestTierForRuntime(t *testing.T) {
	t.Parallel()
	if got := TierForRuntime(gcptarget.RuntimeAutopilotID); got != sdk.ExtensionTierCertified {
		t.Fatalf("autopilot = %q", got)
	}
	if got := TierForRuntime(""); got != sdk.ExtensionTierCertified {
		t.Fatalf("empty runtime = %q", got)
	}
	if got := TierForRuntime(gcptarget.RuntimeStandardID); got != sdk.ExtensionTierExperimental {
		t.Fatalf("standard = %q", got)
	}
}
