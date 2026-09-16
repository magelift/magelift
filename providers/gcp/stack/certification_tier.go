package stack

import (
	"github.com/magelift/magelift/internal/platform"
	gcptarget "github.com/magelift/magelift/providers/gcp/target"
	"github.com/magelift/magelift/sdk"
)

// gcpAutopilotPreviewMagentoCertified is the evidenced Magento cell:
// GKE Autopilot Magento 2.4.9 preview. HA, Standard, and other releases stay experimental.
func gcpAutopilotPreviewMagentoCertified(spec Spec) bool {
	runtime := spec.Identity.Runtime
	if runtime == "" {
		runtime = gcptarget.RuntimeAutopilotID
	}
	if runtime != gcptarget.RuntimeAutopilotID {
		return false
	}
	if spec.Identity.Preset != sdk.PresetPreview {
		return false
	}
	if spec.Application.Version != "2.4.9" {
		return false
	}
	return true
}

// CertificationTier reports the evidenced tier for a planned spec.
func (s Spec) CertificationTier() platform.CertificationTier {
	if gcpAutopilotPreviewMagentoCertified(s) {
		return platform.TierCertified
	}
	return platform.TierExperimental
}

// TierForRuntime reports the module-level tier for a runtime: Autopilot is
// the certified target, every other runtime stays experimental.
func TierForRuntime(runtime sdk.RuntimeID) sdk.ExtensionCertificationTier {
	if runtime == "" || runtime == gcptarget.RuntimeAutopilotID {
		return sdk.ExtensionTierCertified
	}
	return sdk.ExtensionTierExperimental
}

// SDKTier maps a planned spec tier to the wire tier.
func (s Spec) SDKTier() sdk.ExtensionCertificationTier {
	if s.CertificationTier() == platform.TierCertified {
		return sdk.ExtensionTierCertified
	}
	return sdk.ExtensionTierExperimental
}
