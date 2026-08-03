package stack

import (
	"fmt"

	"github.com/magelift/magelift/internal/cloud/gcp/naming"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Module struct{}

type Planned struct {
	Spec Spec
}

func (p Planned) StackName() string {
	return platform.FormatStackName(p.Spec.Identity.Project, p.Spec.Identity.Environment, p.Provider(), p.Runtime())
}
func (p Planned) Provider() sdk.ProviderID { return sdk.ProviderID("gcp") }
func (p Planned) Runtime() sdk.RuntimeID   { return sdk.RuntimeID("gke-autopilot") }
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
	return sdk.TargetDescriptor{ID: "gcp.gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot"}
}

func (p Planned) WithImageDigest(digest string) (platform.PlannedStack, error) {
	next := p
	next.Spec.Artifact.ImageDigest = digest
	if err := next.Spec.Validate(); err != nil {
		return nil, err
	}
	return next, nil
}

func (Module) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "gcp.gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot"}
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
	gcpPlanned, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("GCP stack module received unexpected planned type %T", planned)
	}
	return Program(gcpPlanned.Spec), nil
}

func (Module) OutputKeys() []string {
	keys := append([]string(nil), platform.RequiredOutputKeys()...)
	return append(keys, "mediaURL", "mediaBucket", "searchEndpoint", "queueMode", "queueHost", "securityPolicyName")
}

// GCPSpec exposes the concrete Spec for GCP-only deploy/lock factories.
func (p Planned) GCPSpec() Spec { return p.Spec }

// AsGCPPlanned extracts the GCP Planned value when present.
func AsGCPPlanned(planned platform.PlannedStack) (Planned, bool) {
	value, ok := planned.(Planned)
	return value, ok
}

// ClusterHint returns the deterministic Autopilot cluster name matching runtime.New.
func ClusterHint(spec Spec) string {
	return naming.ClusterName(spec.Identity.Project, spec.Identity.Environment)
}
