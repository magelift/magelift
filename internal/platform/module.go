package platform

import (
	"context"
	"errors"
	"fmt"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// CertificationTier matches ADR 0007.
type CertificationTier string

const (
	TierCertified    CertificationTier = "certified"
	TierExperimental CertificationTier = "experimental"
)

// PlanOptions controls validation exceptions that are safe only for a specific
// lifecycle operation.
type PlanOptions struct {
	AllowExpiredPreview bool
	Context             context.Context
}

// PlannedStack is an opaque, validated stack plan produced by a StackModule.
// Callers must not depend on cloud-specific Spec types except through typed
// adapters (for example AWS deploy factories).
type PlannedStack interface {
	StackName() string
	Provider() sdk.ProviderID
	Runtime() sdk.RuntimeID
	Project() string
	Environment() string
	Region() string
	CertificationTier() CertificationTier
	EnvironmentClass() string
	Protected() bool
	ImageDigest() string
	WithImageDigest(digest string) (PlannedStack, error)
	TargetDescriptor() sdk.TargetDescriptor
}

// StackModule is the port every cloud adapter registers. Plan must not contact
// the cloud or register Pulumi resources.
type StackModule interface {
	Descriptor() sdk.TargetDescriptor
	CertificationTier() CertificationTier
	Plan(cfg config.Config, environment string, opts PlanOptions) (PlannedStack, error)
	Program(planned PlannedStack) (pulumi.RunFunc, error)
	OutputKeys() []string
}

// ModuleRegistry selects stack modules by provider/runtime.
type ModuleRegistry struct {
	byPlatform     map[targetKey]StackModule
	byID           map[sdk.TargetID]StackModule
	extensionsByID map[string]sdk.ExtensionDescriptor
}

type targetKey struct {
	provider sdk.ProviderID
	runtime  sdk.RuntimeID
}

// NewModuleRegistry returns an empty registry.
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		byPlatform:     make(map[targetKey]StackModule),
		byID:           make(map[sdk.TargetID]StackModule),
		extensionsByID: make(map[string]sdk.ExtensionDescriptor),
	}
}

// RegisterModule adds a StackModule. Duplicate target IDs or platforms fail.
func (r *ModuleRegistry) RegisterModule(module StackModule) error {
	return r.registerModule(module, nil)
}

// RegisterPublicModule adapts a public, compile-time extension module into
// the internal stack lifecycle and preserves its declared provenance.
func (r *ModuleRegistry) RegisterPublicModule(module sdk.Module) error {
	adapter, err := newPublicModuleAdapter(module)
	if err != nil {
		return err
	}
	manifest := adapter.descriptor
	if err := r.registerModule(adapter, &manifest); err != nil {
		return fmt.Errorf("register public extension %q: %w", manifest.ID, err)
	}
	return nil
}

func (r *ModuleRegistry) registerModule(module StackModule, manifest *sdk.ExtensionDescriptor) error {
	if r == nil {
		return errors.New("module registry is required")
	}
	if module == nil {
		return errors.New("stack module is required")
	}
	r.ensureMaps()
	descriptor := module.Descriptor()
	if err := sdk.ValidateTargetDescriptor(descriptor); err != nil {
		return fmt.Errorf("register stack module: %w", err)
	}
	if resilience := ModuleResilience(module); resilience != nil {
		resilienceDescriptor := resilience.ResilienceDescriptor()
		if err := sdk.ValidateResilienceAdapterDescriptor(resilienceDescriptor); err != nil {
			return fmt.Errorf("register stack module resilience adapter: %w", err)
		}
		if resilienceDescriptor.Provider != descriptor.Provider {
			return fmt.Errorf("stack module %q resilience adapter provider %q does not match target provider %q", descriptor.ID, resilienceDescriptor.Provider, descriptor.Provider)
		}
	}
	if edge := ModuleEdge(module); edge != nil {
		edgeDescriptor := edge.EdgeDescriptor()
		if err := sdk.ValidateEdgeAdapterDescriptor(edgeDescriptor); err != nil {
			return fmt.Errorf("register stack module edge adapter: %w", err)
		}
		if edgeDescriptor.Provider != descriptor.Provider {
			return fmt.Errorf("stack module %q edge adapter provider %q does not match target provider %q", descriptor.ID, edgeDescriptor.Provider, descriptor.Provider)
		}
	}
	if observability := ModuleObservability(module); observability != nil {
		observabilityDescriptor := observability.ObservabilityDescriptor()
		if err := sdk.ValidateObservabilityAdapterDescriptor(observabilityDescriptor); err != nil {
			return fmt.Errorf("register stack module observability adapter: %w", err)
		}
		if observabilityDescriptor.Provider != descriptor.Provider {
			return fmt.Errorf("stack module %q observability adapter provider %q does not match target provider %q", descriptor.ID, observabilityDescriptor.Provider, descriptor.Provider)
		}
	}
	if certification := ModuleCertification(module); certification != nil {
		certificationDescriptor := certification.CertificationDescriptor()
		if err := sdk.ValidateCertificationAdapterDescriptor(certificationDescriptor); err != nil {
			return fmt.Errorf("register stack module certification adapter: %w", err)
		}
		if certificationDescriptor.Provider != descriptor.Provider || certificationDescriptor.Runtime != descriptor.Runtime {
			return fmt.Errorf("stack module %q certification adapter target %q/%q does not match %q/%q", descriptor.ID, certificationDescriptor.Provider, certificationDescriptor.Runtime, descriptor.Provider, descriptor.Runtime)
		}
	}
	if admission := ModuleCertificationAdmission(module); admission != nil {
		admissionDescriptor := admission.CertificationAdmissionDescriptor()
		if err := sdk.ValidateCertificationAdmissionAdapterDescriptor(admissionDescriptor); err != nil {
			return fmt.Errorf("register stack module certification admission adapter: %w", err)
		}
		if admissionDescriptor.Provider != descriptor.Provider || admissionDescriptor.Runtime != descriptor.Runtime {
			return fmt.Errorf("stack module %q certification admission adapter target %q/%q does not match %q/%q", descriptor.ID, admissionDescriptor.Provider, admissionDescriptor.Runtime, descriptor.Provider, descriptor.Runtime)
		}
	}
	if tier := module.CertificationTier(); tier != TierCertified && tier != TierExperimental {
		return fmt.Errorf("stack module %q has invalid certification tier %q", descriptor.ID, tier)
	}
	keys := RequiredOutputKeys()
	provided := make(map[string]struct{}, len(module.OutputKeys()))
	for _, key := range module.OutputKeys() {
		provided[key] = struct{}{}
	}
	for _, key := range keys {
		if _, ok := provided[key]; !ok {
			return fmt.Errorf("stack module %q omits required output key %q", descriptor.ID, key)
		}
	}
	platform := targetKey{provider: descriptor.Provider, runtime: descriptor.Runtime}
	if _, exists := r.byID[descriptor.ID]; exists {
		return fmt.Errorf("stack module ID %q is already registered", descriptor.ID)
	}
	if _, exists := r.byPlatform[platform]; exists {
		return fmt.Errorf("stack module for provider %q and runtime %q is already registered", descriptor.Provider, descriptor.Runtime)
	}
	registeredManifest := sdk.ExtensionDescriptor{
		APIVersion: sdk.ExtensionAPIVersion,
		ID:         "magelift." + string(descriptor.ID),
		Version:    "builtin",
		Source:     "magelift",
		Build:      "first-party",
		Tier:       extensionTier(module.CertificationTier()),
		Targets:    []sdk.TargetDescriptor{descriptor},
		OutputKeys: append([]string(nil), module.OutputKeys()...),
	}
	if manifest != nil {
		registeredManifest = *manifest
		if len(registeredManifest.Targets) != 1 || registeredManifest.Targets[0] != descriptor {
			return fmt.Errorf("extension manifest target does not match stack module %q", descriptor.ID)
		}
		if len(registeredManifest.OutputKeys) == 0 {
			registeredManifest.OutputKeys = append([]string(nil), module.OutputKeys()...)
		}
	}
	if err := sdk.ValidateExtensionDescriptor(registeredManifest); err != nil {
		return fmt.Errorf("register stack module metadata: %w", err)
	}
	if _, exists := r.extensionsByID[registeredManifest.ID]; exists {
		return fmt.Errorf("extension ID %q is already registered", registeredManifest.ID)
	}
	r.byID[descriptor.ID] = module
	r.byPlatform[platform] = module
	r.extensionsByID[registeredManifest.ID] = registeredManifest
	return nil
}

// RegisterExtension records a public extension manifest for an explicitly
// assembled custom binary. It does not discover or execute files.
func (r *ModuleRegistry) RegisterExtension(extension sdk.Extension) error {
	if r == nil {
		return errors.New("module registry is required")
	}
	if extension == nil {
		return errors.New("extension is required")
	}
	r.ensureMaps()
	descriptor := extension.Descriptor()
	if err := sdk.ValidateExtensionDescriptor(descriptor); err != nil {
		return fmt.Errorf("register extension: %w", err)
	}
	if _, exists := r.extensionsByID[descriptor.ID]; exists {
		return fmt.Errorf("extension ID %q is already registered", descriptor.ID)
	}
	r.extensionsByID[descriptor.ID] = descriptor
	return nil
}

// Extensions returns a stable copy of the explicit extension manifests.
func (r *ModuleRegistry) Extensions() []sdk.ExtensionDescriptor {
	if r == nil {
		return nil
	}
	result := make([]sdk.ExtensionDescriptor, 0, len(r.extensionsByID))
	for _, descriptor := range r.extensionsByID {
		descriptor.Targets = append([]sdk.TargetDescriptor(nil), descriptor.Targets...)
		descriptor.Capabilities = append([]sdk.CapabilityDescriptor(nil), descriptor.Capabilities...)
		descriptor.OutputKeys = append([]string(nil), descriptor.OutputKeys...)
		result = append(result, descriptor)
	}
	return sdk.ExtensionDescriptorsByID(result)
}

func extensionTier(tier CertificationTier) sdk.ExtensionCertificationTier {
	if tier == TierCertified {
		return sdk.ExtensionTierCertified
	}
	return sdk.ExtensionTierExperimental
}

func (r *ModuleRegistry) ensureMaps() {
	if r.byPlatform == nil {
		r.byPlatform = make(map[targetKey]StackModule)
	}
	if r.byID == nil {
		r.byID = make(map[sdk.TargetID]StackModule)
	}
	if r.extensionsByID == nil {
		r.extensionsByID = make(map[string]sdk.ExtensionDescriptor)
	}
}

// Module returns the StackModule for a provider/runtime pair.
func (r *ModuleRegistry) Module(provider sdk.ProviderID, runtime sdk.RuntimeID) (StackModule, bool) {
	if r == nil {
		return nil, false
	}
	module, found := r.byPlatform[targetKey{provider: provider, runtime: runtime}]
	return module, found
}

// Plan selects the module for cfg.Target and plans the stack.
func (r *ModuleRegistry) Plan(cfg config.Config, environment string, opts PlanOptions) (StackModule, PlannedStack, error) {
	if r == nil {
		return nil, nil, errors.New("module registry is required")
	}
	module, found := r.Module(sdk.ProviderID(cfg.Target.Provider), sdk.RuntimeID(cfg.Target.Runtime))
	if !found {
		return nil, nil, fmt.Errorf("no stack module registered for target %q/%q", cfg.Target.Provider, cfg.Target.Runtime)
	}
	planned, err := module.Plan(cfg, environment, opts)
	if err != nil {
		return nil, nil, err
	}
	return module, planned, nil
}
