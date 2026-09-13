package v1

import (
	"context"
)

// ModulePlanRequest is the provider-neutral input passed to a deployable
// extension. Configuration contains the resolved MageLift document as JSON
// values so an extension does not need to import internal/config.
type ModulePlanRequest struct {
	Application      Application
	Artifact         BuildArtifact
	Architecture     ArchitectureIntent
	Resilience       ResilienceIntent
	Edge             EdgeIntent
	Observability    ObservabilityIntent
	Project          string
	Environment      string
	Region           string
	EnvironmentClass string
	Protected        bool
	AllowExpired     bool
	Configuration    map[string]any
}

// ModulePlan is the provider-neutral result of a side-effect-free extension
// plan. Opaque is owned by the extension and is passed back unchanged to
// Program after MageLift validates the stable fields.
type ModulePlan struct {
	StackName        string
	Provider         ProviderID
	Runtime          RuntimeID
	Project          string
	Environment      string
	Region           string
	EnvironmentClass string
	Protected        bool
	ImageDigest      string
	Target           TargetDescriptor
	Tier             ExtensionCertificationTier
	Opaque           any
}

// Module is the public deployable extension contract. Tests and Floci suites
// load a module in-process. The published CLI loads a signed HashiCorp
// go-plugin subprocess that speaks this contract. MageLift never discovers or
// executes an unsigned file from an arbitrary path at runtime.
type Module interface {
	Extension
	Plan(context.Context, ModulePlanRequest) (ModulePlan, error)
	// Program stays opaque in the public SDK. The internal adapter validates the
	// provider's concrete Pulumi program at the lifecycle boundary.
	Program(ModulePlan) (any, error)
}

// ResilienceAdapterFactory is an optional planned-lifecycle port. A module
// may use the provider-owned Opaque plan value to construct a provider SDK
// client and return the provider-neutral recovery adapter. Constructing the
// adapter must not mutate provider state; lifecycle calls perform side
// effects through ResilienceAdapter.
//
// Keeping this port optional lets a module expose deployment only, while a
// provider or community extension can add recovery without importing
// internal MageLift packages.
type ResilienceAdapterFactory interface {
	NewResilience(context.Context, ModulePlan) (ResilienceAdapter, error)
}

// EdgeAdapterFactory is an optional planned-lifecycle port for native or
// external edge ownership. The factory returns nil when the selected module
// does not own an edge lifecycle for the planned target.
type EdgeAdapterFactory interface {
	NewEdge(context.Context, ModulePlan) (EdgeAdapter, error)
}

// ObservabilityAdapterFactory is an optional planned-lifecycle port for
// native or external telemetry ownership. The factory returns nil when the
// selected module does not own a telemetry lifecycle for the planned target.
type ObservabilityAdapterFactory interface {
	NewObservability(context.Context, ModulePlan) (ObservabilityAdapter, error)
}

// CollectorDeploymentAdapterFactory is an optional planned-lifecycle port for
// workload collector deployment. The factory may use the provider-owned plan
// value to bind an ECS task-definition or Kubernetes release backend without
// exposing provider SDK types through the module contract.
type CollectorDeploymentAdapterFactory interface {
	NewCollectorDeployment(context.Context, ModulePlan) (CollectorDeploymentAdapter, error)
}

// PlanAdmission validates a provider-specific plan against live, read-only
// provider capability data before MageLift creates any resource. It may
// return a plan with provider-owned opaque details materialized, but it must
// preserve the stable identity fields validated by the core.
type PlanAdmission interface {
	Admit(context.Context, ModulePlan) (ModulePlan, error)
}

// PlanAdmissionFactory is an optional provider lifecycle port. A factory may
// construct a read-only provider client from the planned module identity; it
// must not mutate provider state while constructing or admitting the plan.
type PlanAdmissionFactory interface {
	NewPlanAdmission(context.Context, ModulePlan) (PlanAdmission, error)
}

// WebRuntimeFactory is optional. HTTP frontends are not stack modules and
// must not require CoreOutputKeys. First-party plugins may compile in for
// RC1 DX; community plugins use the same registry after signed go-plugin load.
type WebRuntimeFactory interface {
	NewWebRuntime(context.Context, ModulePlan) (WebRuntime, error)
}
