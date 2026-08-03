package platform

import (
	"errors"
	"fmt"

	"github.com/magelift/magelift/internal/config"
	sdk "github.com/magelift/magelift/sdk/v1"
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
	byPlatform map[targetKey]StackModule
	byID       map[sdk.TargetID]StackModule
}

type targetKey struct {
	provider sdk.ProviderID
	runtime  sdk.RuntimeID
}

// NewModuleRegistry returns an empty registry.
func NewModuleRegistry() *ModuleRegistry {
	return &ModuleRegistry{
		byPlatform: make(map[targetKey]StackModule),
		byID:       make(map[sdk.TargetID]StackModule),
	}
}

// RegisterModule adds a StackModule. Duplicate target IDs or platforms fail.
func (r *ModuleRegistry) RegisterModule(module StackModule) error {
	if r == nil {
		return errors.New("module registry is required")
	}
	if module == nil {
		return errors.New("stack module is required")
	}
	descriptor := module.Descriptor()
	if err := sdk.ValidateTargetDescriptor(descriptor); err != nil {
		return fmt.Errorf("register stack module: %w", err)
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
	r.byID[descriptor.ID] = module
	r.byPlatform[platform] = module
	return nil
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
