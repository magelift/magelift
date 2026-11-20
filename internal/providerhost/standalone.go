package providerhost

import (
	"slices"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

const (
	StandaloneProviderAWS   sdk.ProviderID = "aws"
	StandaloneProviderGCP   sdk.ProviderID = "gcp"
	StandaloneProviderAzure sdk.ProviderID = "azure"
)

// StandaloneRuntime describes a first-party provider runtime served by a
// verified out-of-process plugin. OptIn marks runtimes registered only by the
// experimental provider set; Tier remains the runtime's advertised default.
type StandaloneRuntime struct {
	Provider   sdk.ProviderID
	Runtime    sdk.RuntimeID
	Tier       platform.RuntimeTier
	OptIn      bool
	Edge       bool
	Resilience bool
	OutputKeys []string
}

var standaloneRuntimes = []StandaloneRuntime{
	{Provider: StandaloneProviderAWS, Runtime: "ecs-fargate", Tier: platform.TierBuiltIn, Edge: true, OutputKeys: awsOutputKeys()},
	{Provider: StandaloneProviderAWS, Runtime: "eks", Tier: platform.TierExperimental, OptIn: true, OutputKeys: awsOutputKeys()},
	{Provider: StandaloneProviderGCP, Runtime: "gke-autopilot", Tier: platform.TierBuiltIn, Edge: true, Resilience: true, OutputKeys: gcpOutputKeys()},
	{Provider: StandaloneProviderGCP, Runtime: "gke-standard", Tier: platform.TierExperimental, Edge: true, Resilience: true, OutputKeys: gcpOutputKeys()},
	{Provider: StandaloneProviderAzure, Runtime: "aks", Tier: platform.TierExperimental, OutputKeys: azureOutputKeys()},
}

// StandaloneRuntimes returns independent copies of the first-party runtime
// descriptors in stable provider/runtime order.
func StandaloneRuntimes() []StandaloneRuntime {
	runtimes := make([]StandaloneRuntime, len(standaloneRuntimes))
	for i, runtime := range standaloneRuntimes {
		runtime.OutputKeys = slices.Clone(runtime.OutputKeys)
		runtimes[i] = runtime
	}
	return runtimes
}

// StandaloneProviderIDs returns the first-party provider IDs in descriptor
// order, with each provider listed once.
func StandaloneProviderIDs() []string {
	providers := make([]string, 0, len(standaloneRuntimes))
	for _, runtime := range standaloneRuntimes {
		provider := string(runtime.Provider)
		if !slices.Contains(providers, provider) {
			providers = append(providers, provider)
		}
	}
	return providers
}

// IsStandaloneProvider reports whether a provider is an auto-acquirable
// first-party standalone plugin. Community extensions are never discovered or
// downloaded automatically.
func IsStandaloneProvider(provider string) bool {
	provider = strings.TrimSpace(provider)
	for _, descriptor := range standaloneRuntimes {
		if string(descriptor.Provider) == provider {
			return true
		}
	}
	return false
}

func standaloneRuntime(provider sdk.ProviderID, runtime sdk.RuntimeID) (StandaloneRuntime, bool) {
	for _, descriptor := range standaloneRuntimes {
		if descriptor.Provider == provider && descriptor.Runtime == runtime {
			descriptor.OutputKeys = slices.Clone(descriptor.OutputKeys)
			return descriptor, true
		}
	}
	return StandaloneRuntime{}, false
}
