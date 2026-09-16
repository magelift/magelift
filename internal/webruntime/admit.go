package webruntime

import (
	"strings"

	"github.com/magelift/magelift/sdk"
)

var defaultRegistry = mustDefaults()

func mustDefaults() *Registry {
	registry := NewRegistry()
	if err := RegisterDefaults(registry); err != nil {
		panic(err)
	}
	return registry
}

// ListedPlugin is the extensions-list projection: ID, independent version,
// Adobe flag, and claimed Magento releases.
type ListedPlugin struct {
	ID              string                         `json:"id" yaml:"id"`
	Version         string                         `json:"version" yaml:"version"`
	AdobeSupported  bool                           `json:"adobeSupported" yaml:"adobeSupported"`
	MagentoReleases []string                       `json:"magentoReleases" yaml:"magentoReleases"`
	Tier            sdk.ExtensionCertificationTier `json:"tier" yaml:"tier"`
}

// List returns the first-party web-runtime plugins. frankenphp-worker is omitted.
func List() []ListedPlugin {
	descriptors := defaultRegistry.List()
	result := make([]ListedPlugin, 0, len(descriptors))
	for _, descriptor := range descriptors {
		result = append(result, ListedPlugin{
			ID:              string(descriptor.ID),
			Version:         descriptor.Version,
			AdobeSupported:  descriptor.Adobe == sdk.AdobeSupported,
			MagentoReleases: append([]string(nil), descriptor.MagentoReleases...),
			Tier:            descriptor.Tier,
		})
	}
	return result
}

// Admit looks up application.webRuntime. Empty IDs resolve to nginx-fpm.
// Unknown IDs fail closed and name the plugin. Adobe-unsupported plugins
// require compatibility.allowUnsupported and return a hatch warning.
func Admit(id, magentoVersion string, allowUnsupported bool) (warning string, err error) {
	warnings, err := defaultRegistry.Admit(id, magentoVersion, allowUnsupported)
	if err != nil {
		return "", err
	}
	return strings.Join(warnings, "; "), nil
}
