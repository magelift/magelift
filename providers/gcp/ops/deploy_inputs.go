package ops

import (
	"encoding/json"
	"fmt"

	"github.com/magelift/magelift/sdk"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
)

// BuildDeployInputs derives the versioned deploy contract from a resolved
// spec. The protocol server embeds the JSON in the stored plan; the plugin
// decodes and executes it per phase, and the core passes it through
// without interpreting provider behavior.
func BuildDeployInputs(spec gcpstack.Spec) ([]byte, error) {
	deploySpec := sdk.DeployInputs{
		ImageDigest:        spec.Artifact.ImageDigest,
		DatabaseName:       spec.Dependencies.DatabaseName,
		ApplicationMode:    spec.Application.Mode,
		ApplicationVersion: spec.Application.Version,
		WebRuntime:         spec.Application.WebRuntime,
		Magento:            spec.Application.Magento,
		CPURequest:         spec.Catalog.AutopilotCPURequest,
		MemoryRequest:      spec.Catalog.AutopilotMemoryRequest,
		CloudProject:       spec.Identity.GCPProject,
		Region:             spec.Identity.Region,
	}
	data, err := json.Marshal(deploySpec)
	if err != nil {
		return nil, fmt.Errorf("marshal GCP deploy inputs: %w", err)
	}
	return data, nil
}
