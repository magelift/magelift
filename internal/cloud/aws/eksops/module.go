package eksops

import (
	"fmt"

	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Module struct{}

type Planned struct {
	Spec Spec
}

func (p Planned) StackName() string {
	return platform.FormatStackName(p.Spec.Identity.Project, p.Spec.Identity.Environment, p.Provider(), p.Runtime())
}
func (p Planned) Provider() sdk.ProviderID { return sdk.ProviderID("aws") }
func (p Planned) Runtime() sdk.RuntimeID   { return sdk.RuntimeID(RuntimeID) }
func (p Planned) Project() string          { return p.Spec.Identity.Project }
func (p Planned) Environment() string      { return p.Spec.Identity.Environment }
func (p Planned) Region() string           { return p.Spec.Identity.Region }
func (p Planned) CertificationTier() platform.CertificationTier {
	return platform.TierExperimental
}
func (p Planned) EnvironmentClass() string { return p.Spec.Identity.EnvironmentClass }
func (p Planned) Protected() bool          { return p.Spec.Lifecycle.Protection }
func (p Planned) ImageDigest() string      { return p.Spec.Artifact.ImageDigest }
func (p Planned) TargetDescriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: TargetID, Provider: "aws", Runtime: RuntimeID}
}

func (p Planned) WithImageDigest(digest string) (platform.PlannedStack, error) {
	next := p
	next.Spec.Artifact.ImageDigest = digest
	if err := next.Spec.Validate(); err != nil {
		return nil, err
	}
	return next, nil
}

func (p Planned) AWSSpec() Spec { return p.Spec }

func (Module) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: TargetID, Provider: "aws", Runtime: RuntimeID}
}

func (Module) CertificationTier() platform.CertificationTier { return platform.TierExperimental }

func (Module) Plan(cfg config.Config, environment string, opts platform.PlanOptions) (platform.PlannedStack, error) {
	spec, err := PlanFromConfigWithOptions(cfg, environment, PlanOptions{AllowExpiredPreview: opts.AllowExpiredPreview})
	if err != nil {
		return nil, err
	}
	return Planned{Spec: spec}, nil
}

func (Module) Program(planned platform.PlannedStack) (pulumi.RunFunc, error) {
	eksPlanned, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("EKS stack module received unexpected planned type %T", planned)
	}
	return Program(eksPlanned.Spec), nil
}

func (Module) OutputKeys() []string {
	return platform.RequiredOutputKeys()
}

func AsEKSPlanned(planned platform.PlannedStack) (Planned, bool) {
	value, ok := planned.(Planned)
	return value, ok
}
