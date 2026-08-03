// Package v1 defines the stable extension boundary for MageLift schema v1.
package v1

import "context"

type ProviderID string
type RuntimeID string
type TargetID string
type CapabilityProviderID string
type CapabilityID string
type ComponentID string
type ResourceID string
type HookID string
type TransformID string

type Application struct {
	Edition string
	Version string
	Mode    string
}

type BuildArtifact struct {
	ImageDigest    string
	ManifestDigest string
}

type TargetRequest struct {
	Application      Application
	Artifact         BuildArtifact
	EnvironmentClass string
	Topology         DesiredTopology
}

type TargetDescriptor struct {
	ID       TargetID
	Provider ProviderID
	Runtime  RuntimeID
}

// Target owns a provider/runtime implementation. Validation must not mutate
// infrastructure or contact the provider.
type Target interface {
	Descriptor() TargetDescriptor
	Validate(context.Context, TargetRequest) error
}

type CapabilityKind string

const (
	CapabilityDatabase      CapabilityKind = "database"
	CapabilityCache         CapabilityKind = "cache"
	CapabilitySearch        CapabilityKind = "search"
	CapabilityQueue         CapabilityKind = "queue"
	CapabilityObjectStorage CapabilityKind = "object-storage"
	CapabilityEdge          CapabilityKind = "edge"
	CapabilityObservability CapabilityKind = "observability"
)

const (
	CapabilityDatabaseMySQL     CapabilityID = "database.mysql"
	CapabilityCacheValkey       CapabilityID = "cache.valkey"
	CapabilitySearchFullText    CapabilityID = "search.fulltext"
	CapabilityQueueDatabase     CapabilityID = "queue.database"
	CapabilityQueueRabbitMQ     CapabilityID = "queue.rabbitmq"
	CapabilityObjectStorageBlob CapabilityID = "object-storage.blob"
	CapabilityEdgeCDN           CapabilityID = "edge.cdn"
	CapabilityObservabilityLogs CapabilityID = "observability.logs"

	// Deprecated AWS-product aliases — prefer the Magento-shaped IDs above.
	CapabilitySearchOpenSearch        = CapabilitySearchFullText
	CapabilityObjectStorageS3         = CapabilityObjectStorageBlob
	CapabilityEdgeCloudFront          = CapabilityEdgeCDN
	CapabilityObservabilityCloudWatch = CapabilityObservabilityLogs
)

type CapabilityRequest struct {
	Application Application
	Class       string
}

type CapabilityDescriptor struct {
	ID         CapabilityProviderID
	Capability CapabilityID
	Kind       CapabilityKind
	Provider   ProviderID
}

type CapabilityProvider interface {
	Descriptor() CapabilityDescriptor
	Validate(context.Context, CapabilityRequest) error
}

// ResourceOptions is implemented by typed options in a target package. The
// marker prevents transforms from falling back to map[string]any.
type ResourceOptions interface {
	MageLiftResourceOptions()
}

type TransformDescriptor struct {
	ID         TransformID
	Provider   ProviderID
	Runtime    RuntimeID
	Components []ComponentID
}

type TransformInput[T ResourceOptions] struct {
	Component ComponentID
	Resource  ResourceID
	Options   T
}

type Transform[T ResourceOptions] interface {
	Descriptor() TransformDescriptor
	Apply(context.Context, TransformInput[T]) (T, error)
}

type ExistingResourceKind string

const (
	ExistingNetwork     ExistingResourceKind = "network"
	ExistingDatabase    ExistingResourceKind = "database"
	ExistingCache       ExistingResourceKind = "cache"
	ExistingSearch      ExistingResourceKind = "search"
	ExistingQueue       ExistingResourceKind = "queue"
	ExistingObjectStore ExistingResourceKind = "object-storage"
	ExistingDNSZone     ExistingResourceKind = "dns-zone"
	ExistingCertificate ExistingResourceKind = "certificate"
)

type ExistingResourceRef struct {
	ID         ResourceID
	Provider   ProviderID
	Kind       ExistingResourceKind
	ExternalID string
}

type LifecyclePhase string

const (
	PhaseValidate   LifecyclePhase = "validate"
	PhaseBuild      LifecyclePhase = "build"
	PhasePackage    LifecyclePhase = "package"
	PhaseDeploy     LifecyclePhase = "deploy"
	PhasePostDeploy LifecyclePhase = "post-deploy"
)

type HookRelationship string

const (
	HookBefore  HookRelationship = "before"
	HookAfter   HookRelationship = "after"
	HookReplace HookRelationship = "replace"
	HookDisable HookRelationship = "disable"
)

type LifecycleHookDescriptor struct {
	ID           HookID
	Phase        LifecyclePhase
	Relationship HookRelationship
	RelativeTo   HookID
	DependsOn    []HookID
	Timeout      int
	MaxAttempts  int
	Idempotent   bool
}

type LifecycleHookRequest struct {
	Application Application
	Artifact    BuildArtifact
}

type LifecycleHook interface {
	Descriptor() LifecycleHookDescriptor
	Validate(context.Context, LifecycleHookRequest) error
	// Run is invoked only after Validate succeeds and always receives a bounded
	// context owned by the deployment orchestrator.
	Run(context.Context, LifecycleHookRequest) error
}
