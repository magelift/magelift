package certification

import (
	"fmt"
	"sort"
	"strings"

	"github.com/magelift/magelift/sdk"
)

// BuildResiliencePlan delegates deterministic recovery graph construction to
// the public SDK after applying first-party policy ceilings and data-class
// availability rules.
func BuildResiliencePlan(intent sdk.ArchitectureIntent) (sdk.ResiliencePlan, error) {
	if err := ValidateResilienceArchitecture(intent); err != nil {
		return sdk.ResiliencePlan{}, fmt.Errorf("validate resilience architecture: %w", err)
	}
	profile, found := resilienceProfileForIntent(intent)
	if !found {
		return sdk.ResiliencePlan{}, fmt.Errorf("no resilience policy profile for provider %q runtime %q", intent.Provider, intent.Runtime)
	}
	capabilities := make(map[string]ResilienceDataClassCapability, len(profile.DataClasses))
	for _, capability := range profile.DataClasses {
		capabilities[capability.Name] = capability
	}
	for _, dataClass := range intent.Resilience.DataClasses {
		if capability := capabilities[dataClass.Name]; capability.Durable && rebuildOnly(dataClass) {
			return sdk.ResiliencePlan{}, fmt.Errorf("durable data class %q cannot use a rebuild-only recovery method", dataClass.Name)
		}
	}
	descriptor := resilienceAdapterDescriptor(intent, profile)
	return sdk.CompileResiliencePlan(descriptor, sdk.ResiliencePlanRequest{
		Architecture:    intent,
		FixtureID:       "architecture-" + string(intent.Runtime),
		OwnershipMarker: intent.OwnershipMarker,
	})
}

func resilienceAdapterDescriptor(intent sdk.ArchitectureIntent, profile ResilienceCapabilityProfile) sdk.ResilienceAdapterDescriptor {
	descriptor := sdk.ResilienceAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion,
		ID:         "core." + string(intent.Provider) + "." + string(intent.Runtime),
		Provider:   intent.Provider,
		Version:    "1.0.0",
		Capabilities: []sdk.ResilienceAction{
			sdk.ResilienceBackup, sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck,
			sdk.ResilienceFence, sdk.ResilienceFailover, sdk.ResilienceCleanup,
		},
		DataClasses: make([]sdk.ResilienceDataClassCapability, 0, len(profile.DataClasses)),
	}
	for _, capability := range profile.DataClasses {
		status := sdk.ResilienceCapabilitySupported
		switch capability.Status {
		case CapabilityCertified:
			status = sdk.ResilienceCapabilityCertified
		case CapabilityExperimental:
			status = sdk.ResilienceCapabilityExperimental
		case CapabilityUnavailable:
			status = sdk.ResilienceCapabilityUnavailable
		case CapabilityUnsupported:
			status = sdk.ResilienceCapabilityUnsupported
		case CapabilityBlocked:
			status = sdk.ResilienceCapabilityBlocked
		}
		dataClass := sdk.ResilienceDataClassCapability{
			Name:                capability.Name,
			Status:              status,
			RetentionPolicy:     "profile-retention-policy",
			EncryptionBoundary:  "profile-encryption-boundary",
			ProtectionMechanism: "profile-protection-policy",
			PollingRequired:     true,
			RetentionRequired:   capability.Durable,
			EncryptionRequired:  capability.Durable,
			ProtectionRequired:  capability.Durable,
			Reason:              capability.Reason,
		}
		if status == sdk.ResilienceCapabilitySupported || status == sdk.ResilienceCapabilityCertified || status == sdk.ResilienceCapabilityExperimental {
			if capability.BackupSupported {
				dataClass.Actions = append(dataClass.Actions, sdk.ResilienceBackup)
			}
			if capability.RestoreSupported {
				dataClass.Actions = append(dataClass.Actions, sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck)
			}
			for _, destination := range capability.Destinations {
				dataClass.Destinations = append(dataClass.Destinations, sdk.RecoveryDestination(destination))
			}
		}
		descriptor.DataClasses = append(descriptor.DataClasses, dataClass)
	}
	return descriptor
}

func resilienceProfileForIntent(intent sdk.ArchitectureIntent) (ResilienceCapabilityProfile, bool) {
	if profile, found := FindResilienceProfile(string(intent.Provider) + "." + string(intent.Runtime)); found {
		return profile, true
	}
	for _, profile := range CurrentResilienceProfiles() {
		if profile.Provider == string(intent.Provider) && profile.Runtime == string(intent.Runtime) {
			return profile, true
		}
	}
	return ResilienceCapabilityProfile{}, false
}

func rebuildOnly(dataClass sdk.DataClassIntent) bool {
	method := strings.ToLower(strings.TrimSpace(dataClass.BackupMethod))
	return method == "rebuild" || method == "recreate" || method == "none"
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
