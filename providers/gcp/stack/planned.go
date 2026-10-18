package stack

import (
	"errors"

	"github.com/magelift/magelift/internal/platform"
	gcptarget "github.com/magelift/magelift/providers/gcp/target"
	"github.com/magelift/magelift/sdk"
)

// SpecPlanned adapts a resolved Spec to platform.PlannedStack for
// server-side consumers that take the interface (the shared kube observe
// helpers). Readers come from the spec. Mutation is unsupported: plans
// change only through re-planning via the Plan operation.
type SpecPlanned struct {
	Spec Spec
}

var _ platform.PlannedStack = SpecPlanned{}

func (p SpecPlanned) StackName() string {
	return platform.FormatStackName(p.Spec.Identity.Project, p.Spec.Identity.Environment, p.Provider(), p.Runtime())
}

func (p SpecPlanned) Provider() sdk.ProviderID { return gcptarget.ProviderID }

func (p SpecPlanned) Runtime() sdk.RuntimeID {
	if p.Spec.Identity.Runtime == "" {
		return gcptarget.RuntimeAutopilotID
	}
	return p.Spec.Identity.Runtime
}

func (p SpecPlanned) Project() string     { return p.Spec.Identity.Project }
func (p SpecPlanned) Environment() string { return p.Spec.Identity.Environment }
func (p SpecPlanned) Region() string      { return p.Spec.Identity.Region }

func (p SpecPlanned) CertificationTier() platform.CertificationTier {
	return p.Spec.CertificationTier()
}

func (p SpecPlanned) EnvironmentClass() string { return p.Spec.Identity.EnvironmentClass }
func (p SpecPlanned) Protected() bool          { return p.Spec.Lifecycle.Protection }
func (p SpecPlanned) ImageDigest() string      { return p.Spec.Artifact.ImageDigest }

// WithImageDigest always fails: image changes re-plan through the server.
func (p SpecPlanned) WithImageDigest(string) (platform.PlannedStack, error) {
	return nil, errors.New("GCP server plans are immutable; re-plan with the new image digest")
}

func (p SpecPlanned) TargetDescriptor() sdk.TargetDescriptor {
	if p.Runtime() == gcptarget.RuntimeStandardID {
		return sdk.TargetDescriptor{ID: gcptarget.TargetStandardID, Provider: gcptarget.ProviderID, Runtime: p.Runtime()}
	}
	return sdk.TargetDescriptor{ID: gcptarget.TargetAutopilotID, Provider: gcptarget.ProviderID, Runtime: gcptarget.RuntimeAutopilotID}
}
