// Package resilience contains the small first-party glue shared by provider
// recovery adapters. It builds descriptors and delegates execution to the
// public SDK's operation-backed adapter; provider packages only declare their
// current native capability boundaries.
package resilience

import (
	"fmt"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// DataClassSpec is provider-owned capability metadata expressed in the
// provider-neutral vocabulary. Native API names belong in the mechanism
// strings, never in the core recovery graph.
type DataClassSpec struct {
	Name                string
	Status              sdk.ResilienceCapabilityStatus
	Actions             []sdk.ResilienceAction
	Destinations        []sdk.RecoveryDestination
	Strategy            string
	BackupMechanism     string
	RestoreMechanism    string
	IntegrityMethod     string
	RetentionPolicy     string
	EncryptionBoundary  string
	ProtectionMechanism string
	Durable             bool
	// ProtectionOptional records a provider boundary where retention and
	// encryption are enforceable but an equivalent deletion-protection control
	// is not exposed by the native service. The descriptor must not turn that
	// absence into an unverified guarantee.
	ProtectionOptional bool
	Reason             string
}

// CapabilityProvider is an optional provider-client declaration for global
// recovery actions. A client may opt into fencing or traffic failover only
// when its native translator can execute and prove those actions. The
// provider-neutral operation client remains usable without this extension.
type CapabilityProvider interface {
	ResilienceCapabilities() []sdk.ResilienceAction
}

// Descriptor builds the same descriptor shape for first-party and community
// adapters. The caller remains responsible for selecting honest capability
// statuses and mechanisms for its provider.
func Descriptor(provider sdk.ProviderID, id, version string, specs []DataClassSpec) sdk.ResilienceAdapterDescriptor {
	return DescriptorWithCapabilities(provider, id, version, specs, []sdk.ResilienceAction{
		sdk.ResilienceBackup,
		sdk.ResilienceRestore,
		sdk.ResilienceIntegrityCheck,
		sdk.ResilienceCleanup,
	})
}

// DescriptorWithCapabilities lets a provider-owned implementation opt into
// runtime fencing or traffic failover only after it supplies the corresponding
// operation translator. The default Descriptor deliberately excludes those
// actions because data backup/restore adapters cannot prove writer fencing or
// route convergence by themselves.
func DescriptorWithCapabilities(provider sdk.ProviderID, id, version string, specs []DataClassSpec, capabilities []sdk.ResilienceAction) sdk.ResilienceAdapterDescriptor {
	dataClasses := make([]sdk.ResilienceDataClassCapability, 0, len(specs))
	for _, spec := range specs {
		dataClasses = append(dataClasses, sdk.ResilienceDataClassCapability{
			Name:                spec.Name,
			Status:              spec.Status,
			Actions:             append([]sdk.ResilienceAction(nil), spec.Actions...),
			Destinations:        append([]sdk.RecoveryDestination(nil), spec.Destinations...),
			Strategy:            spec.Strategy,
			BackupMechanism:     spec.BackupMechanism,
			RestoreMechanism:    spec.RestoreMechanism,
			IntegrityMethod:     spec.IntegrityMethod,
			RetentionPolicy:     spec.RetentionPolicy,
			EncryptionBoundary:  spec.EncryptionBoundary,
			ProtectionMechanism: spec.ProtectionMechanism,
			PollingRequired:     true,
			RetentionRequired:   spec.Durable,
			EncryptionRequired:  spec.Durable,
			ProtectionRequired:  spec.Durable && !spec.ProtectionOptional,
			Reason:              spec.Reason,
		})
	}
	return sdk.ResilienceAdapterDescriptor{
		APIVersion:   sdk.ExtensionAPIVersion,
		ID:           id,
		Provider:     provider,
		Version:      version,
		Capabilities: append([]sdk.ResilienceAction(nil), capabilities...),
		DataClasses:  dataClasses,
	}
}

// ExperimentalReason keeps capability descriptors honest while a provider
// package exposes the semantic port but has not yet shipped its concrete
// operation translator and independent live evidence. An injected first-party
// or community client can still execute the contract; the status must not be
// promoted until that client and evidence exist.
func ExperimentalReason(provider sdk.ProviderID, dataClass string) string {
	return fmt.Sprintf("The %s %s operation translator and independent live evidence are not included yet; use an injected or community client until both are available.", provider, dataClass)
}

// NativeTranslatorReason distinguishes a shipped official-SDK translator from
// a provider package that only exposes an injected lifecycle seam. It keeps
// experimental status honest until independent live certification evidence is
// available.
func NativeTranslatorReason(provider sdk.ProviderID, dataClass string) string {
	return fmt.Sprintf("The %s %s operation translator is implemented against the official SDK; independent live evidence and provider certification are still required.", provider, dataClass)
}

// New returns the shared operation-backed adapter. Provider code supplies a
// client that translates these semantic operations to the current provider
// SDK/API and returns normalized proof evidence.
func New(descriptor sdk.ResilienceAdapterDescriptor, client sdk.ResilienceOperationClient, policy sdk.ResilienceOperationPolicy) (sdk.ResilienceAdapter, error) {
	if strings.TrimSpace(string(descriptor.Provider)) == "" {
		return nil, fmt.Errorf("resilience adapter provider is required")
	}
	if capabilities, ok := client.(CapabilityProvider); ok {
		descriptor.Capabilities = append([]sdk.ResilienceAction(nil), capabilities.ResilienceCapabilities()...)
	}
	return sdk.NewOperationBackedResilienceAdapter(descriptor, client, policy)
}
