package webruntime

import (
	sdk "github.com/magelift/magelift/sdk/v1"
)

const pluginVersion = "0.1.0"

var firstPartyMagentoReleases = []string{"2.4.6", "2.4.7", "2.4.8", "2.4.9"}

type plugin struct {
	descriptor sdk.WebRuntimeDescriptor
}

func (p plugin) Descriptor() sdk.WebRuntimeDescriptor {
	return sdk.WebRuntimeDescriptorsByID([]sdk.WebRuntimeDescriptor{p.descriptor})[0]
}

func (p plugin) Admit(magentoVersion string, allowUnsupported bool) ([]string, error) {
	return sdk.AdmitWebRuntime(p.descriptor.ID, magentoVersion, allowUnsupported)
}

func nginxFPM() sdk.WebRuntime {
	return plugin{descriptor: sdk.WebRuntimeDescriptor{
		APIVersion:      sdk.ExtensionAPIVersion,
		ID:              sdk.WebRuntimeNginxFPM,
		Version:         pluginVersion,
		Source:          "magelift",
		Tier:            sdk.ExtensionTierCertified,
		Adobe:           sdk.AdobeSupported,
		MagentoReleases: append([]string(nil), firstPartyMagentoReleases...),
		LocalCompose:    sdk.WebRuntimeComposeHints{ImageFamily: "php-runtime", HealthPath: sdk.WebRuntimeHealthPath},
		Cloud:           sdk.WebRuntimeCloudHints{Ports: []int{8080}, Placement: sdk.WebRuntimePlacementSidecar},
	}}
}

func frankenPHPClassic() sdk.WebRuntime {
	return plugin{descriptor: sdk.WebRuntimeDescriptor{
		APIVersion:      sdk.ExtensionAPIVersion,
		ID:              sdk.WebRuntimeFrankenPHPClassic,
		Version:         pluginVersion,
		Source:          "magelift",
		Tier:            sdk.ExtensionTierExperimental,
		Adobe:           sdk.AdobeUnsupported,
		MagentoReleases: append([]string(nil), firstPartyMagentoReleases...),
		LocalCompose:    sdk.WebRuntimeComposeHints{ImageFamily: "frankenphp-classic", HealthPath: sdk.WebRuntimeHealthPath},
		Cloud:           sdk.WebRuntimeCloudHints{Ports: []int{8080}, Placement: sdk.WebRuntimePlacementProcess},
	}}
}

func phpApache() sdk.WebRuntime {
	return plugin{descriptor: sdk.WebRuntimeDescriptor{
		APIVersion:      sdk.ExtensionAPIVersion,
		ID:              sdk.WebRuntimePHPApache,
		Version:         pluginVersion,
		Source:          "magelift",
		Tier:            sdk.ExtensionTierExperimental,
		Adobe:           sdk.AdobeUnsupported,
		MagentoReleases: append([]string(nil), firstPartyMagentoReleases...),
		LocalCompose:    sdk.WebRuntimeComposeHints{ImageFamily: "php-apache", HealthPath: sdk.WebRuntimeHealthPath},
		Cloud:           sdk.WebRuntimeCloudHints{Ports: []int{8080}, Placement: sdk.WebRuntimePlacementProcess},
	}}
}
