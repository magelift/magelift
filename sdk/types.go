// Package v1 defines the stable extension boundary for MageLift schema v1.
package sdk

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

// ExternalLifecycle describes who owns an edge or observability capability.
type ExternalLifecycle string

const (
	ExternalLifecycleManaged     ExternalLifecycle = "managed"
	ExternalLifecycleExtension   ExternalLifecycle = "extension"
	ExternalLifecycleObserveOnly ExternalLifecycle = "observe-only"
)

// ExternalCertificationStatus is deliberately independent of the target's
// certification tier. A cloud target can be certified while an external edge
// or telemetry adapter remains experimental.
type ExternalCertificationStatus string

const (
	ExternalCertified    ExternalCertificationStatus = "certified"
	ExternalExperimental ExternalCertificationStatus = "experimental"
	ExternalUnavailable  ExternalCertificationStatus = "unavailable"
	ExternalBlocked      ExternalCertificationStatus = "blocked"
)

type Application struct {
	Edition string
	Version string
	Mode    string
}

// EdgeHealthIntent is the provider-neutral safety policy for an edge route.
// Provider adapters translate it to their health-check and convergence APIs.
type EdgeHealthIntent struct {
	OriginURL           string
	OriginHost          string
	ExpectedRouteTarget string
	RoutePath           string
	ExpectedStatus      int
	RouteTimeoutSeconds int
	RoutePollSeconds    int
}

// EdgeIntent is the provider-neutral edge request passed to an extension.
// Provider-specific policy remains in the extension-owned configuration.
type EdgeIntent struct {
	ExternalProvider  string
	Lifecycle         ExternalLifecycle
	Certification     ExternalCertificationStatus
	Mode              string
	NativeProvider    string
	CredentialRefs    []string
	ServiceReference  string
	Domains           []string
	TLS               bool
	TLSMode           string
	DNSMode           string
	PurgeOnDeploy     bool
	PolicyReference   string
	OriginHealthRef   string
	CachePolicyRef    string
	PurgePolicyRef    string
	WAFPolicyRef      string
	FailoverPolicyRef string
	OwnershipMarker   string
	Health            EdgeHealthIntent
}

// ObservabilityIntent selects telemetry signals without coupling the public
// SDK to a vendor API or credential format.
type ObservabilityIntent struct {
	Lifecycle     ExternalLifecycle
	Certification ExternalCertificationStatus
	// OwnershipMarker scopes native and external telemetry permissions to the
	// selected architecture. It is an opaque, non-secret marker; adapters use
	// it when naming resources and constraining vendor permissions.
	OwnershipMarker string
	NativeProvider  string
	// NativeReference is an opaque provider-owned identity for an existing
	// native destination, such as an OVH Logs Data Platform stream. It is not
	// a provider SDK type and must never contain a secret.
	NativeReference    string
	ExternalProvider   string
	CredentialRefs     []string
	Endpoint           string
	ServiceName        string
	Environment        string
	Logs               bool
	Metrics            bool
	Traces             bool
	Signals            []string
	RetentionDays      int
	SamplingRatio      float64
	RedactionPolicyRef string
	AlertRefs          []string
	Labels             map[string]string
	DataResidency      string
	Alerts             []AlertIntent
	Dashboards         []DashboardIntent
	SLOs               []SLOIntent
}

// AlertIntent is the portable minimum for an actionable alert. Provider
// adapters translate it into CloudWatch alarms, Google Cloud alert policies,
// Cockpit rules, Logs Data Platform alerts, or a community implementation.
type AlertIntent struct {
	ID                   string
	Signal               string
	Severity             string
	Operator             string
	Threshold            float64
	WindowSeconds        int64
	Owner                string
	RunbookURL           string
	DeduplicationKey     string
	MaintenancePolicyRef string
}

// DashboardIntent is the portable minimum for an operational dashboard.
// Provider adapters translate it into native dashboards without leaking their
// panel or query schemas into the core contract.
type DashboardIntent struct {
	ID      string
	Signals []string
	Owner   string
}

// SLOIntent keeps the objective and its runbook in the portable contract;
// vendor-specific policy objects remain adapter-owned.
type SLOIntent struct {
	ID                string
	Signal            string
	Target            float64
	WindowSeconds     int64
	Owner             string
	RunbookURL        string
	ErrorBudgetPolicy string
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
	Edge             EdgeIntent
	Observability    ObservabilityIntent
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
	CapabilityDatabaseMySQL                      CapabilityID = "database.mysql"
	CapabilityDatabaseMariaDB                    CapabilityID = "database.mariadb"
	CapabilityCacheValkey                        CapabilityID = "cache.valkey"
	CapabilitySearchFullText                     CapabilityID = "search.fulltext"
	CapabilityQueueDatabase                      CapabilityID = "queue.database"
	CapabilityQueueRabbitMQ                      CapabilityID = "queue.rabbitmq"
	CapabilityObjectStorageBlob                  CapabilityID = "object-storage.blob"
	CapabilityEdgeCDN                            CapabilityID = "edge.cdn"
	CapabilityObservabilityLogs                  CapabilityID = "observability.logs"
	CapabilityEdgeFastly                         CapabilityID = "edge.fastly"
	CapabilityObservabilityCloudWatchProvider    CapabilityID = "observability.cloudwatch"
	CapabilityObservabilityGoogleCloudOperations CapabilityID = "observability.google-cloud-operations"
	CapabilityObservabilityNewRelicProvider      CapabilityID = "observability.newrelic"
	CapabilityObservabilityDatadogProvider       CapabilityID = "observability.datadog"

	// Deprecated AWS-product aliases; prefer the Magento-shaped IDs above.
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
