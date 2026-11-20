package stack

import (
	"github.com/magelift/magelift/internal/platform"
	gcptarget "github.com/magelift/magelift/providers/gcp/target"
	"github.com/magelift/magelift/sdk"
)

// RuntimeTier reports the implementation tier for the planned runtime.
func (s Spec) RuntimeTier() platform.RuntimeTier {
	return platform.RuntimeTier(TierForRuntime(s.Identity.Runtime))
}

// TierForRuntime reports the support tier for the GKE runtime.
func TierForRuntime(runtime sdk.RuntimeID) sdk.ExtensionTier {
	if runtime == "" || runtime == gcptarget.RuntimeAutopilotID {
		return sdk.ExtensionTierBuiltIn
	}
	return sdk.ExtensionTierExperimental
}

// SDKTier maps a planned spec tier to the wire tier.
func (s Spec) SDKTier() sdk.ExtensionTier {
	return TierForRuntime(s.Identity.Runtime)
}
