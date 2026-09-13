package stack

import (
	gcptarget "github.com/magelift/magelift/internal/cloud/gcp/target"
	sdk "github.com/magelift/magelift/sdk/v1"
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
