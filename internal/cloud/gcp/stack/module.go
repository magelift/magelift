package stack

import (
	"fmt"

	gcpbootstrap "github.com/magelift/magelift/internal/cloud/gcp/bootstrap"
	"github.com/magelift/magelift/internal/cloud/gcp/naming"
	gcptarget "github.com/magelift/magelift/internal/cloud/gcp/target"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Module struct {
	RuntimeID sdk.RuntimeID
	platform.LifecycleFactories
}

type Planned struct {
	Spec Spec
}

func (p Planned) StackName() string {
	return platform.FormatStackName(p.Spec.Identity.Project, p.Spec.Identity.Environment, p.Provider(), p.Runtime())
}
func (p Planned) Provider() sdk.ProviderID { return sdk.ProviderID("gcp") }
func (p Planned) Runtime() sdk.RuntimeID {
	if p.Spec.Identity.Runtime == "" {
		return gcptarget.RuntimeAutopilotID
	}
	return p.Spec.Identity.Runtime
}
func (p Planned) Project() string     { return p.Spec.Identity.Project }
func (p Planned) Environment() string { return p.Spec.Identity.Environment }
func (p Planned) Region() string      { return p.Spec.Identity.Region }
func (p Planned) CertificationTier() platform.CertificationTier {
	if gcpAutopilotPreviewMagentoCertified(p.Spec) {
		return platform.TierCertified
	}
	return platform.TierExperimental
}
func (p Planned) EnvironmentClass() string { return p.Spec.Identity.EnvironmentClass }
func (p Planned) Protected() bool          { return p.Spec.Lifecycle.Protection }
func (p Planned) ImageDigest() string      { return p.Spec.Artifact.ImageDigest }
func (p Planned) TargetDescriptor() sdk.TargetDescriptor {
	return descriptorForRuntime(p.Runtime())
}

// OpaquePlanSpec exposes the provider-owned plan spec for subprocess
// execution. Only the proof provider implements providerhost.OpaqueSpecProvider.
func (p Planned) OpaquePlanSpec() any { return p.Spec }

func (p Planned) WithImageDigest(digest string) (platform.PlannedStack, error) {
	next := p
	next.Spec.Artifact.ImageDigest = digest
	if err := next.Spec.Validate(); err != nil {
		return nil, err
	}
	return next, nil
}

func (p Planned) WithLiveQueueReplicas(replicas int) (platform.PlannedStack, error) {
	next := p
	next.Spec.Catalog.LiveQueueReplicas = replicas
	if err := next.Spec.Validate(); err != nil {
		return nil, err
	}
	return next, nil
}

func (m Module) Descriptor() sdk.TargetDescriptor {
	return descriptorForRuntime(m.runtime())
}

func (m Module) CertificationTier() platform.CertificationTier {
	if m.runtime() == gcptarget.RuntimeAutopilotID {
		return platform.TierCertified
	}
	return platform.TierExperimental
}

// PlanAdmission resolves current GCP project, region, managed-service, and
// GKE catalog availability before any Pulumi resource mutation.
func (m Module) PlanAdmission() platform.PlanAdmission { return RegionAdmission{} }

func (m Module) Plan(cfg config.Config, environment string, opts platform.PlanOptions) (platform.PlannedStack, error) {
	// The zero-value Module remains the original Autopilot target for callers
	// that construct the first-party adapter directly.
	spec, err := PlanFromConfigWithOptions(cfg, environment, PlanOptions{
		AllowExpiredPreview: opts.AllowExpiredPreview,
		RuntimeID:           m.runtime(),
	})
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
	return append(keys, "mediaURL", "mediaBucket", platform.OutputSearchEndpoint, platform.OutputQueueHost, platform.OutputQueueReplicas, platform.OutputDatabaseConnectionName, "queueMode", platform.OutputDatabaseSecretName, platform.OutputQueuePasswordSecretName, "securityPolicyName")
}

func (m Module) runtime() sdk.RuntimeID {
	if m.RuntimeID == "" {
		return gcptarget.RuntimeAutopilotID
	}
	return m.RuntimeID
}

func descriptorForRuntime(runtime sdk.RuntimeID) sdk.TargetDescriptor {
	if runtime == gcptarget.RuntimeStandardID {
		return sdk.TargetDescriptor{ID: gcptarget.TargetStandardID, Provider: gcptarget.ProviderID, Runtime: runtime}
	}
	return sdk.TargetDescriptor{ID: gcptarget.TargetAutopilotID, Provider: gcptarget.ProviderID, Runtime: gcptarget.RuntimeAutopilotID}
}

// GCPSpec exposes the concrete Spec for GCP-only deploy/lock factories.
func (p Planned) GCPSpec() Spec { return p.Spec }

func (p Planned) StateBackendURL() string {
	url, err := gcpbootstrap.StateBackendURL(gcpbootstrap.Spec{
		Project: p.Spec.Identity.Project, Environment: p.Spec.Identity.Environment,
		GCPProject: p.Spec.Identity.GCPProject, Region: p.Spec.Identity.Region,
	})
	if err != nil {
		return ""
	}
	return url
}

// AsGCPPlanned extracts the GCP Planned value when present.
func AsGCPPlanned(planned platform.PlannedStack) (Planned, bool) {
	value, ok := planned.(Planned)
	return value, ok
}

// ClusterHint returns the deterministic GKE cluster name matching runtime.New.
func ClusterHint(spec Spec) string {
	return naming.ClusterNameForRuntime(spec.Identity.Project, spec.Identity.Environment, string(spec.Identity.Runtime))
}
