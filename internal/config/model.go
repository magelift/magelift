package config

type Config struct {
	SchemaVersion      int                 `yaml:"schemaVersion" json:"schemaVersion" config:"MageLift configuration schema version" schema:"const=1"`
	Project            Project             `yaml:"project" json:"project"`
	Application        Application         `yaml:"application" json:"application"`
	Build              Build               `yaml:"build" json:"build"`
	Local              LocalRuntime        `yaml:"local,omitempty" json:"local,omitempty" config:"Local runtime service and email choices"`
	Target             Target              `yaml:"target" json:"target"`
	Edge               EdgeConfig          `yaml:"edge,omitempty" json:"edge,omitempty" config:"Optional edge delivery provider"`
	Observability      ObservabilityConfig `yaml:"observability,omitempty" json:"observability,omitempty" config:"Optional observability provider"`
	Email              EmailConfig         `yaml:"email,omitempty" json:"email,omitempty" config:"Cloud transactional email"`
	Resilience         ResilienceConfig    `yaml:"resilience,omitempty" json:"resilience,omitempty" config:"Availability, backup, and disaster-recovery policy"`
	Defaults           Defaults            `yaml:"defaults" json:"defaults"`
	Compatibility      Compatibility       `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`
	PreviewIdentity    *PreviewIdentity    `yaml:"previewIdentity,omitempty" json:"previewIdentity,omitempty" config:"Derived pull-request preview identity" schema:"nullable"`
	Extensions         map[string]any      `yaml:"extensions,omitempty" json:"extensions,omitempty"`
	Account            string              `yaml:"account,omitempty" json:"account,omitempty"`
	Class              string              `yaml:"class,omitempty" json:"class,omitempty"`
	Preset             string              `yaml:"preset,omitempty" json:"preset,omitempty"`
	Domain             string              `yaml:"domain,omitempty" json:"domain,omitempty"`
	Protection         bool                `yaml:"protection,omitempty" json:"protection,omitempty"`
	ExpiresAt          string              `yaml:"expiresAt,omitempty" json:"expiresAt,omitempty" config:"Preview expiration as RFC3339" schema:"nullable"`
	MonthlyBudgetCents int64               `yaml:"monthlyBudgetCents,omitempty" json:"monthlyBudgetCents,omitempty" config:"Maximum monthly AWS budget in cents" schema:"nullable"`
	Branches           []string            `yaml:"branches,omitempty" json:"branches,omitempty"`
	SeedDump           string              `yaml:"seedDump,omitempty" json:"seedDump,omitempty" config:"Local MySQL dump path seeded after first deploy" schema:"nullable"`
}

type Project struct {
	Name string `yaml:"name" json:"name" config:"Project name" schema:"minLength=1"`
}

type Application struct {
	Edition    string         `yaml:"edition" json:"edition" config:"Magento edition" schema:"enum=open-source|commerce"`
	Version    string         `yaml:"version" json:"version" config:"Exact Magento release"`
	Mode       string         `yaml:"mode" json:"mode" config:"Application mode" schema:"enum=integrated|headless"`
	WebRuntime string         `yaml:"webRuntime,omitempty" json:"webRuntime,omitempty" config:"HTTP application runtime" schema:"enum=nginx-fpm|frankenphp-classic|php-apache"`
	Magento    MagentoRuntime `yaml:"magento,omitempty" json:"magento,omitempty" config:"Magento runtime overlays"`
	Cron       []CronEntry    `yaml:"cron,omitempty" json:"cron,omitempty" config:"Portable Magento cron schedule entries"`
}

// MagentoRuntime is Magento-shaped YAML for env.php / CONFIG__* / MAGENTO_DC_* overlays.
type MagentoRuntime struct {
	FrontName        string            `yaml:"frontName,omitempty" json:"frontName,omitempty" config:"Magento admin frontName"`
	CookieDomain     string            `yaml:"cookieDomain,omitempty" json:"cookieDomain,omitempty" config:"Magento cookie domain"`
	UnsecureBaseURL  string            `yaml:"unsecureBaseUrl,omitempty" json:"unsecureBaseUrl,omitempty" config:"Unsecure Magento base URL"`
	SecureBaseURL    string            `yaml:"secureBaseUrl,omitempty" json:"secureBaseUrl,omitempty" config:"Secure Magento base URL"`
	CORSOrigins      []string          `yaml:"corsOrigins,omitempty" json:"corsOrigins,omitempty" config:"Allowed CORS origins for the Magento API"`
	StorefrontOrigin string            `yaml:"storefrontOrigin,omitempty" json:"storefrontOrigin,omitempty" config:"Headless storefront origin allowed by CORS"`
	Consumers        MagentoConsumers  `yaml:"consumers,omitempty" json:"consumers,omitempty" config:"Magento message consumer runners"`
	QueueTransport   string            `yaml:"queueTransport,omitempty" json:"queueTransport,omitempty" config:"Optional Magento-module queue transport" schema:"nullable,enum=sqs|pubsub"`
	QueueModule      string            `yaml:"queueModule,omitempty" json:"queueModule,omitempty" config:"Locked Composer Magento package providing SQS or Pub/Sub transport" schema:"nullable"`
	Variables        map[string]string `yaml:"variables,omitempty" json:"variables,omitempty" config:"CONFIG__* and MAGENTO_DC_* overlays"`
}

type MagentoConsumers struct {
	Mode  string   `yaml:"mode,omitempty" json:"mode,omitempty" config:"Consumer runner" schema:"nullable,enum=cron|processes|both"`
	Names []string `yaml:"names,omitempty" json:"names,omitempty" config:"Named Magento consumers"`
}

// CronEntry is a Magento cron:run-style schedule mapped from PaaS crons (IMPORT-03).
// Free-form shell crons remain unmapped by the importer.
type CronEntry struct {
	Schedule string `yaml:"schedule" json:"schedule" config:"Cron schedule expression" schema:"minLength=1"`
	Command  string `yaml:"command" json:"command" config:"Command to run (bin/magento cron:run style)" schema:"minLength=1"`
}

type Build struct {
	PHP            string                `yaml:"php" json:"php" config:"Exact PHP branch or patch version"`
	Extensions     []string              `yaml:"extensions,omitempty" json:"extensions,omitempty" config:"Required PHP extensions"`
	Composer       Composer              `yaml:"composer,omitempty" json:"composer,omitempty" config:"Composer settings"`
	StaticContent  StaticContentSettings `yaml:"staticContent,omitempty" json:"staticContent,omitempty" config:"Static content deployment settings"`
	QualityPatches []string              `yaml:"qualityPatches,omitempty" json:"qualityPatches,omitempty" config:"Quality Patch IDs applied at build"`
	Hooks          map[string]BuildHook  `yaml:"hooks,omitempty" json:"hooks,omitempty" config:"Build lifecycle hooks"`
}

// LocalRuntime contains workstation-only choices. Provider-managed services
// remain under Target; a local choice never changes the cloud topology.
type LocalRuntime struct {
	Database    LocalService       `yaml:"database,omitempty" json:"database,omitempty" config:"Local database family and version"`
	Cache       LocalService       `yaml:"cache,omitempty" json:"cache,omitempty" config:"Local cache family and version"`
	Search      LocalService       `yaml:"search,omitempty" json:"search,omitempty" config:"Local search family and version"`
	Queue       LocalService       `yaml:"queue,omitempty" json:"queue,omitempty" config:"Local queue family and version"`
	WebServer   LocalService       `yaml:"webServer,omitempty" json:"webServer,omitempty" config:"Local web server family and version"`
	WebCache    LocalService       `yaml:"webCache,omitempty" json:"webCache,omitempty" config:"Local web cache family and version"`
	PHPSettings map[string]string  `yaml:"phpSettings,omitempty" json:"phpSettings,omitempty" config:"Local PHP ini settings" schema:"nullable"`
	Email       LocalEmailSettings `yaml:"email,omitempty" json:"email,omitempty" config:"Local email delivery mode"`
}

type LocalService struct {
	Family  string `yaml:"family,omitempty" json:"family,omitempty" config:"Service family" schema:"nullable"`
	Version string `yaml:"version,omitempty" json:"version,omitempty" config:"Service version" schema:"nullable"`
}

type LocalEmailSettings struct {
	Mode          string `yaml:"mode,omitempty" json:"mode,omitempty" config:"Email mode" schema:"nullable,enum=disabled|smtp|ses|mailpit"`
	Host          string `yaml:"host,omitempty" json:"host,omitempty" config:"SMTP or local email host" schema:"nullable"`
	Port          int    `yaml:"port,omitempty" json:"port,omitempty" config:"SMTP or local email port" schema:"nullable,minimum=1,maximum=65535"`
	Username      string `yaml:"username,omitempty" json:"username,omitempty" config:"SMTP username" schema:"nullable"`
	From          string `yaml:"from,omitempty" json:"from,omitempty" config:"Default sender address" schema:"nullable"`
	CredentialEnv string `yaml:"credentialEnv,omitempty" json:"credentialEnv,omitempty" config:"Environment variable containing the email credential" schema:"nullable"`
}

// EmailConfig is Magento SMTP on a cloud environment. Credentials are secret
// references, never plaintext YAML. Validation success is not certified delivery.
type EmailConfig struct {
	Mode       string        `yaml:"mode,omitempty" json:"mode,omitempty" config:"Cloud email mode" schema:"nullable,enum=disabled|smtp|ses|tem|ovh"`
	Host       string        `yaml:"host,omitempty" json:"host,omitempty" config:"SMTP host" schema:"nullable"`
	Port       int           `yaml:"port,omitempty" json:"port,omitempty" config:"SMTP port" schema:"nullable,minimum=1,maximum=65535"`
	Username   string        `yaml:"username,omitempty" json:"username,omitempty" config:"SMTP username" schema:"nullable"`
	From       string        `yaml:"from,omitempty" json:"from,omitempty" config:"Default sender address" schema:"nullable"`
	Credential string        `yaml:"credential,omitempty" json:"credential,omitempty" config:"SES credential secret reference" schema:"nullable,pattern=^(aws-secrets-manager|ssm|gcp-secret-manager)://\\S+$"`
	Managed    *EmailManaged `yaml:"managed,omitempty" json:"managed,omitempty" config:"Managed email provisioning inputs"`
}

// EmailManaged carries managed-provisioning inputs. When present, MageLift
// creates the sender identity, credentials, and DNS records for the mode's
// provider; explicit host/port/username/credential values are rejected
// alongside it. Absent means operator-supplied endpoints (BYO).
type EmailManaged struct {
	Domain       string `yaml:"domain,omitempty" json:"domain,omitempty" config:"Verified sender domain" schema:"nullable"`
	HostedZoneID string `yaml:"hostedZoneId,omitempty" json:"hostedZoneId,omitempty" config:"Route 53 hosted zone ID for DKIM records (SES only)" schema:"nullable"`
	Account      string `yaml:"account,omitempty" json:"account,omitempty" config:"Mailbox account name (OVH only)" schema:"nullable"`
}

// StaticContentSettings configures Magento setup:static-content:deploy.
// Locales and themes form a cartesian product; strategy/threads apply to each pair (-s/-j).
type StaticContentSettings struct {
	Locales  []string `yaml:"locales,omitempty" json:"locales,omitempty" config:"Locales passed to setup:static-content:deploy --language"`
	Themes   []string `yaml:"themes,omitempty" json:"themes,omitempty" config:"Themes passed to setup:static-content:deploy --theme"`
	Strategy string   `yaml:"strategy,omitempty" json:"strategy,omitempty" config:"Static content deploy strategy (-s)" schema:"enum=quick|standard|compact,nullable"`
	Threads  int      `yaml:"threads,omitempty" json:"threads,omitempty" config:"Static content deploy thread count (-j)" schema:"minimum=1,nullable"`
}

// BuildHook is keyed by its stable ID in Build.Hooks. Commands are argument
// vectors, never shell strings, so the build runner can enforce its trust
// boundary before executing them.
type BuildHook struct {
	Phase        string            `yaml:"phase" json:"phase" config:"Preparation lifecycle phase" schema:"enum=validate|build|package"`
	Relationship string            `yaml:"relationship" json:"relationship" config:"Relationship to the target step" schema:"enum=before|after|replace|disable"`
	Target       string            `yaml:"target" json:"target" config:"Stable target step ID"`
	Command      *BuildHookCommand `yaml:"command,omitempty" json:"command,omitempty" config:"Validated command vector" schema:"nullable"`
	Dependencies []string          `yaml:"dependencies,omitempty" json:"dependencies,omitempty" config:"Additional stable step dependencies"`
	Timeout      int               `yaml:"timeoutSeconds,omitempty" json:"timeoutSeconds,omitempty" config:"Step timeout in seconds"`
	Retries      BuildHookRetries  `yaml:"retries,omitempty" json:"retries,omitempty" config:"Retry policy"`
	Failure      string            `yaml:"failure,omitempty" json:"failure,omitempty" config:"Failure action" schema:"enum=abort|continue"`
}

type BuildHookCommand struct {
	Executable string   `yaml:"executable" json:"executable" config:"Allowed executable" schema:"enum=composer|magento"`
	Arguments  []string `yaml:"arguments,omitempty" json:"arguments,omitempty" config:"Argument vector"`
}

type BuildHookRetries struct {
	MaxAttempts  int  `yaml:"maxAttempts,omitempty" json:"maxAttempts,omitempty" config:"Maximum attempts"`
	DelaySeconds int  `yaml:"delaySeconds,omitempty" json:"delaySeconds,omitempty" config:"Retry delay in seconds"`
	Idempotent   bool `yaml:"idempotent,omitempty" json:"idempotent,omitempty" config:"Whether retrying is safe"`
}

type Composer struct {
	Version     string `yaml:"version,omitempty" json:"version,omitempty" config:"Required Composer 2 major, minor, or patch version" schema:"nullable,pattern=^2\\.\\d+(?:\\.\\d+)?$"`
	Credentials string `yaml:"credentials,omitempty" json:"credentials,omitempty" config:"Composer credentials secret reference" schema:"pattern=^(aws-secrets-manager|ssm|gcp-secret-manager)://\\S+$"`
}
type Target struct {
	Provider string          `yaml:"provider" json:"provider" config:"Infrastructure provider" schema:"enum=aws|gcp|ovh|scaleway"`
	Runtime  string          `yaml:"runtime" json:"runtime" config:"Application runtime" schema:"enum=ecs-fargate|eks|gke-autopilot|gke-standard|mks|kapsule"`
	AWS      *AWSTarget      `yaml:"aws,omitempty" json:"aws,omitempty" config:"AWS deployment inputs"`
	GCP      *GCPTarget      `yaml:"gcp,omitempty" json:"gcp,omitempty" config:"GCP deployment inputs"`
	OVH      *OVHTarget      `yaml:"ovh,omitempty" json:"ovh,omitempty" config:"OVHcloud deployment inputs (experimental)"`
	Scaleway *ScalewayTarget `yaml:"scaleway,omitempty" json:"scaleway,omitempty" config:"Scaleway deployment inputs (experimental)"`
}

// EdgeConfig describes an optional edge provider without carrying provider
// credentials. Fastly remains experimental until a real-account acceptance
// cell proves create, route, purge, and teardown behavior.
type EdgeConfig struct {
	ExternalProvider  string            `yaml:"externalProvider,omitempty" json:"externalProvider,omitempty" config:"External edge delivery provider" schema:"nullable"`
	Mode              string            `yaml:"mode,omitempty" json:"mode,omitempty" config:"Edge composition mode" schema:"enum=none|native|external|both,nullable"`
	NativeProvider    string            `yaml:"nativeProvider,omitempty" json:"nativeProvider,omitempty" config:"Provider-native edge implementation" schema:"nullable"`
	ServiceID         string            `yaml:"serviceId,omitempty" json:"serviceId,omitempty" config:"Fastly service identifier" schema:"nullable"`
	TokenSecret       string            `yaml:"tokenSecret,omitempty" json:"tokenSecret,omitempty" config:"Fastly API token secret reference" schema:"nullable,pattern=^(aws-secrets-manager|ssm|gcp-secret-manager)://\\S+$"`
	Domains           []string          `yaml:"domains,omitempty" json:"domains,omitempty" config:"Edge hostnames" schema:"nullable"`
	TLS               bool              `yaml:"tls,omitempty" json:"tls,omitempty" config:"Require edge TLS"`
	TLSMode           string            `yaml:"tlsMode,omitempty" json:"tlsMode,omitempty" config:"TLS certificate ownership and renewal mode" schema:"nullable"`
	DNSMode           string            `yaml:"dnsMode,omitempty" json:"dnsMode,omitempty" config:"DNS ownership and convergence mode" schema:"nullable"`
	VCLRef            string            `yaml:"vclRef,omitempty" json:"vclRef,omitempty" config:"Reference to reviewed Fastly VCL or policy artifact" schema:"nullable"`
	PurgeOnDeploy     bool              `yaml:"purgeOnDeploy,omitempty" json:"purgeOnDeploy,omitempty" config:"Purge Fastly content after deploy"`
	OriginHealthRef   string            `yaml:"originHealthRef,omitempty" json:"originHealthRef,omitempty" config:"Origin health gate reference" schema:"nullable"`
	CachePolicyRef    string            `yaml:"cachePolicyRef,omitempty" json:"cachePolicyRef,omitempty" config:"Reviewed cache-key and bypass policy reference" schema:"nullable"`
	PurgePolicyRef    string            `yaml:"purgePolicyRef,omitempty" json:"purgePolicyRef,omitempty" config:"Purge verification policy reference" schema:"nullable"`
	WAFPolicyRef      string            `yaml:"wafPolicyRef,omitempty" json:"wafPolicyRef,omitempty" config:"WAF or security-header policy reference" schema:"nullable"`
	FailoverPolicyRef string            `yaml:"failoverPolicyRef,omitempty" json:"failoverPolicyRef,omitempty" config:"Origin failover and failback policy reference" schema:"nullable"`
	OwnershipMarker   string            `yaml:"ownershipMarker,omitempty" json:"ownershipMarker,omitempty" config:"Edge resource ownership marker" schema:"nullable"`
	Health            *EdgeHealthConfig `yaml:"health,omitempty" json:"health,omitempty" config:"External edge health verification policy" schema:"nullable"`
}

// EdgeHealthConfig is provider-neutral edge safety policy. Providers translate
// these values into their own health-check and route-verification APIs; the
// portable configuration never carries a raw SDK request.
type EdgeHealthConfig struct {
	OriginURL           string `yaml:"originUrl,omitempty" json:"originUrl,omitempty" config:"Origin health URL checked before edge mutation" schema:"nullable"`
	OriginHost          string `yaml:"originHost,omitempty" json:"originHost,omitempty" config:"TLS SNI and HTTP Host used for the origin health URL" schema:"nullable"`
	ExpectedCNAME       string `yaml:"expectedCname,omitempty" json:"expectedCname,omitempty" config:"Expected edge DNS CNAME" schema:"nullable"`
	RoutePath           string `yaml:"routePath,omitempty" json:"routePath,omitempty" config:"Path used for post-mutation edge verification" schema:"nullable"`
	ExpectedStatus      int    `yaml:"expectedStatus,omitempty" json:"expectedStatus,omitempty" config:"Expected post-mutation HTTP status" schema:"nullable"`
	RouteTimeoutSeconds int    `yaml:"routeTimeoutSeconds,omitempty" json:"routeTimeoutSeconds,omitempty" config:"Maximum edge route convergence time" schema:"nullable"`
	RoutePollSeconds    int    `yaml:"routePollSeconds,omitempty" json:"routePollSeconds,omitempty" config:"Edge route convergence polling interval" schema:"nullable"`
}

// ObservabilityConfig selects telemetry without placing provider credentials
// or provider-specific settings in the portable configuration. Extensions own
// those settings under their namespaced configuration.
type ObservabilityConfig struct {
	NativeProvider       string                   `yaml:"nativeProvider,omitempty" json:"nativeProvider,omitempty" config:"Provider-native observability destination" schema:"nullable"`
	NativeReference      string                   `yaml:"nativeReference,omitempty" json:"nativeReference,omitempty" config:"Opaque provider-owned native destination reference" schema:"nullable"`
	ExternalProvider     string                   `yaml:"externalProvider,omitempty" json:"externalProvider,omitempty" config:"External observability destination" schema:"nullable"`
	CredentialReferences []string                 `yaml:"credentialReferences,omitempty" json:"credentialReferences,omitempty" config:"Secret references resolved by the observability extension" schema:"nullable"`
	Endpoint             string                   `yaml:"endpoint,omitempty" json:"endpoint,omitempty" config:"Optional telemetry endpoint" schema:"nullable"`
	ServiceName          string                   `yaml:"serviceName,omitempty" json:"serviceName,omitempty" config:"Telemetry service name" schema:"nullable"`
	Environment          string                   `yaml:"environment,omitempty" json:"environment,omitempty" config:"Telemetry environment name" schema:"nullable"`
	Logs                 bool                     `yaml:"logs,omitempty" json:"logs,omitempty" config:"Export logs"`
	Metrics              bool                     `yaml:"metrics,omitempty" json:"metrics,omitempty" config:"Export metrics"`
	Traces               bool                     `yaml:"traces,omitempty" json:"traces,omitempty" config:"Export traces"`
	Signals              []string                 `yaml:"signals,omitempty" json:"signals,omitempty" config:"Portable telemetry signal names" schema:"nullable"`
	RetentionDays        int                      `yaml:"retentionDays,omitempty" json:"retentionDays,omitempty" config:"Telemetry retention days" schema:"nullable"`
	SamplingRatio        float64                  `yaml:"samplingRatio,omitempty" json:"samplingRatio,omitempty" config:"Trace sampling ratio" schema:"nullable"`
	RedactionPolicyRef   string                   `yaml:"redactionPolicyRef,omitempty" json:"redactionPolicyRef,omitempty" config:"Telemetry redaction policy reference" schema:"nullable"`
	AlertReferences      []string                 `yaml:"alertReferences,omitempty" json:"alertReferences,omitempty" config:"Alert and dashboard references" schema:"nullable"`
	Labels               map[string]string        `yaml:"labels,omitempty" json:"labels,omitempty" config:"Additional resource attributes" schema:"nullable"`
	DataResidency        string                   `yaml:"dataResidency,omitempty" json:"dataResidency,omitempty" config:"Declared telemetry data residency" schema:"nullable"`
	Alerts               []ObservabilityAlert     `yaml:"alerts,omitempty" json:"alerts,omitempty" config:"Actionable alert policies" schema:"nullable"`
	Dashboards           []ObservabilityDashboard `yaml:"dashboards,omitempty" json:"dashboards,omitempty" config:"Operational dashboards" schema:"nullable"`
	SLOs                 []ObservabilitySLO       `yaml:"slos,omitempty" json:"slos,omitempty" config:"Service-level objectives" schema:"nullable"`
}

type ObservabilityAlert struct {
	ID                   string  `yaml:"id" json:"id" config:"Stable alert identifier"`
	Signal               string  `yaml:"signal" json:"signal" config:"Portable signal name"`
	Severity             string  `yaml:"severity" json:"severity" config:"Alert severity" schema:"enum=info|warning|critical"`
	Operator             string  `yaml:"operator" json:"operator" config:"Threshold operator" schema:"enum=gt|gte|lt|lte|eq"`
	Threshold            float64 `yaml:"threshold" json:"threshold" config:"Threshold value"`
	WindowSeconds        int64   `yaml:"windowSeconds" json:"windowSeconds" config:"Evaluation window in seconds"`
	Owner                string  `yaml:"owner" json:"owner" config:"Alert owner"`
	RunbookURL           string  `yaml:"runbookUrl" json:"runbookUrl" config:"HTTPS runbook URL"`
	DeduplicationKey     string  `yaml:"deduplicationKey" json:"deduplicationKey" config:"Alert deduplication key"`
	MaintenancePolicyRef string  `yaml:"maintenancePolicyRef,omitempty" json:"maintenancePolicyRef,omitempty" config:"Maintenance suppression policy reference" schema:"nullable"`
}

type ObservabilityDashboard struct {
	ID      string   `yaml:"id" json:"id" config:"Stable dashboard identifier"`
	Signals []string `yaml:"signals" json:"signals" config:"Signals shown by the dashboard"`
	Owner   string   `yaml:"owner" json:"owner" config:"Dashboard owner"`
}

type ObservabilitySLO struct {
	ID                string  `yaml:"id" json:"id" config:"Stable SLO identifier"`
	Signal            string  `yaml:"signal" json:"signal" config:"Portable signal name"`
	Target            float64 `yaml:"target" json:"target" config:"Objective ratio between zero and one"`
	WindowSeconds     int64   `yaml:"windowSeconds" json:"windowSeconds" config:"Evaluation window in seconds"`
	Owner             string  `yaml:"owner" json:"owner" config:"SLO owner"`
	RunbookURL        string  `yaml:"runbookUrl" json:"runbookUrl" config:"HTTPS runbook URL"`
	ErrorBudgetPolicy string  `yaml:"errorBudgetPolicy" json:"errorBudgetPolicy" config:"Error budget policy reference"`
}

// ResilienceConfig is provider-neutral. Provider adapters implement the
// backup, restore, failover, and fencing operations named by this policy.
type ResilienceConfig struct {
	ProfileID            string                `yaml:"profileId,omitempty" json:"profileId,omitempty" config:"Named resilience profile" schema:"nullable"`
	AvailabilityTarget   string                `yaml:"availabilityTarget,omitempty" json:"availabilityTarget,omitempty" config:"Availability target" schema:"nullable"`
	RPOSeconds           int64                 `yaml:"rpoSeconds,omitempty" json:"rpoSeconds,omitempty" config:"Maximum recovery point objective in seconds" schema:"nullable"`
	RTOSeconds           int64                 `yaml:"rtoSeconds,omitempty" json:"rtoSeconds,omitempty" config:"Maximum recovery time objective in seconds" schema:"nullable"`
	RetentionDays        int                   `yaml:"retentionDays,omitempty" json:"retentionDays,omitempty" config:"Backup retention days" schema:"nullable"`
	RecoveryScope        string                `yaml:"recoveryScope,omitempty" json:"recoveryScope,omitempty" config:"Recovery scope" schema:"nullable"`
	FailoverOwner        string                `yaml:"failoverOwner,omitempty" json:"failoverOwner,omitempty" config:"Failover operator" schema:"nullable"`
	FencingPolicy        string                `yaml:"fencingPolicy,omitempty" json:"fencingPolicy,omitempty" config:"Single-writer and split-brain fencing policy" schema:"nullable"`
	DataRegion           string                `yaml:"dataRegion,omitempty" json:"dataRegion,omitempty" config:"Optional region residency for Magento database, media, and backups" schema:"nullable"`
	RecoveryDestinations []string              `yaml:"recoveryDestinations,omitempty" json:"recoveryDestinations,omitempty" config:"Allowed recovery destinations" schema:"nullable"`
	Projection           *ResilienceProjection `yaml:"projection,omitempty" json:"projection,omitempty" config:"Search and cache rebuild workload target" schema:"nullable"`
	DataClasses          []ResilienceDataClass `yaml:"dataClasses,omitempty" json:"dataClasses,omitempty" config:"Independent data-class recovery policies" schema:"nullable"`
}

// ResilienceProjection identifies the application workload that receives
// provider-neutral search rebuild and cache reconstruction commands. It is a
// semantic target, not a provider SDK request: ECS resolves a service to a
// running task (or uses an explicitly pinned task), while Kubernetes resolves
// a ready pod from namespace/workload labels. Credentials and verifier logic
// remain injected by the provider adapter.
type ResilienceProjection struct {
	Runtime   string `yaml:"runtime" json:"runtime" config:"Projection runtime transport" schema:"enum=ecs|kubernetes"`
	Cluster   string `yaml:"cluster,omitempty" json:"cluster,omitempty" config:"ECS cluster identity" schema:"nullable"`
	Service   string `yaml:"service,omitempty" json:"service,omitempty" config:"ECS service used to select a running task" schema:"nullable"`
	Task      string `yaml:"task,omitempty" json:"task,omitempty" config:"Optional pinned ECS task identity" schema:"nullable"`
	Namespace string `yaml:"namespace,omitempty" json:"namespace,omitempty" config:"Kubernetes workload namespace" schema:"nullable"`
	Workload  string `yaml:"workload,omitempty" json:"workload,omitempty" config:"Kubernetes workload app label" schema:"nullable"`
	Container string `yaml:"container,omitempty" json:"container,omitempty" config:"Container receiving projection commands" schema:"nullable"`
}

type ResilienceDataClass struct {
	Name               string   `yaml:"name" json:"name" config:"Data class name"`
	SourceOfTruth      string   `yaml:"sourceOfTruth" json:"sourceOfTruth" config:"Authoritative source"`
	FailureDomains     []string `yaml:"failureDomains,omitempty" json:"failureDomains,omitempty" config:"Failure domains" schema:"nullable"`
	BackupMethod       string   `yaml:"backupMethod" json:"backupMethod" config:"Backup method"`
	RestoreMethod      string   `yaml:"restoreMethod" json:"restoreMethod" config:"Restore method"`
	IntegrityMethod    string   `yaml:"integrityMethod" json:"integrityMethod" config:"Restore integrity check"`
	LossSemantics      string   `yaml:"lossSemantics" json:"lossSemantics" config:"Data loss or rebuild semantics"`
	RetentionDays      int      `yaml:"retentionDays" json:"retentionDays" config:"Data-class retention days"`
	Encrypted          bool     `yaml:"encrypted,omitempty" json:"encrypted,omitempty" config:"Encryption required"`
	Immutable          bool     `yaml:"immutable,omitempty" json:"immutable,omitempty" config:"Immutable backup required"`
	DeletionProtection bool     `yaml:"deletionProtection,omitempty" json:"deletionProtection,omitempty" config:"Deletion protection required"`
	OwnershipMarker    string   `yaml:"ownershipMarker" json:"ownershipMarker" config:"Data-class ownership marker"`
}

// OVHTarget holds provider-specific OVHcloud semantic inputs. The adapter owns
// the SDK topology; supported topology choices remain explicit and typed here.
type OVHTarget struct {
	ServiceName                    string            `yaml:"serviceName" json:"serviceName" config:"OVH Public Cloud project service name" schema:"minLength=1"`
	APIEndpoint                    string            `yaml:"apiEndpoint,omitempty" json:"apiEndpoint,omitempty" config:"OVHcloud API endpoint alias (ovh-eu, ovh-ca, or ovh-us)" schema:"nullable,enum=ovh-eu|ovh-ca|ovh-us"`
	Region                         string            `yaml:"region,omitempty" json:"region,omitempty" config:"OVH Public Cloud region (for example EU-WEST-PAR or GRA11)" schema:"nullable"`
	NetworkCIDR                    string            `yaml:"networkCidr,omitempty" json:"networkCidr,omitempty" config:"Private network IPv4 CIDR" schema:"nullable"`
	Zones                          []string          `yaml:"zones,omitempty" json:"zones,omitempty" config:"OVH MKS availability zones within the selected region" schema:"nullable"`
	AttachFloatingIPs              bool              `yaml:"attachFloatingIps,omitempty" json:"attachFloatingIps,omitempty" config:"Attach OVH floating IPs to MKS nodes"`
	PrivateNetworkRoutingAsDefault bool              `yaml:"privateNetworkRoutingAsDefault,omitempty" json:"privateNetworkRoutingAsDefault,omitempty" config:"Route MKS egress through OVH's documented private-subnet DHCP gateway; custom gateway routing is not inferred"`
	ImageDigest                    string            `yaml:"imageDigest,omitempty" json:"imageDigest,omitempty" config:"Signed immutable OCI image digest" schema:"nullable"`
	DatabaseName                   string            `yaml:"databaseName,omitempty" json:"databaseName,omitempty" config:"Magento database name" schema:"nullable"`
	MasterUsername                 string            `yaml:"masterUsername,omitempty" json:"masterUsername,omitempty" config:"Managed MySQL master username" schema:"nullable"`
	EncryptionKeySecret            string            `yaml:"encryptionKeySecret,omitempty" json:"encryptionKeySecret,omitempty" config:"Secret reference for Magento encryption key" schema:"nullable"`
	DatabaseFlavor                 string            `yaml:"databaseFlavor,omitempty" json:"databaseFlavor,omitempty" config:"OVH managed MySQL flavor" schema:"nullable"`
	DatabasePlan                   string            `yaml:"databasePlan,omitempty" json:"databasePlan,omitempty" config:"OVH managed MySQL plan (discovery/essential, business/production, or enterprise/advanced)" schema:"nullable,enum=discovery|essential|business|production|enterprise|advanced"`
	DatabaseVersion                string            `yaml:"databaseVersion,omitempty" json:"databaseVersion,omitempty" config:"OVH managed MySQL engine version (8.0 or 8.4)" schema:"nullable,enum=8.0|8.4"`
	DatabaseNodeCount              int               `yaml:"databaseNodeCount,omitempty" json:"databaseNodeCount,omitempty" config:"OVH managed MySQL node count" schema:"nullable,minimum=1"`
	DatabaseBackupTime             string            `yaml:"databaseBackupTime,omitempty" json:"databaseBackupTime,omitempty" config:"OVH managed MySQL daily backup start time (HH:MM)" schema:"nullable"`
	DatabaseBackupRegions          []string          `yaml:"databaseBackupRegions,omitempty" json:"databaseBackupRegions,omitempty" config:"OVH managed MySQL backup regions" schema:"nullable"`
	DatabaseDeletionProtection     *bool             `yaml:"databaseDeletionProtection,omitempty" json:"databaseDeletionProtection,omitempty" config:"Enable OVH managed MySQL deletion protection" schema:"nullable"`
	ValkeyFlavor                   string            `yaml:"valkeyFlavor,omitempty" json:"valkeyFlavor,omitempty" config:"OVH managed Valkey flavor" schema:"nullable"`
	ValkeyPlan                     string            `yaml:"valkeyPlan,omitempty" json:"valkeyPlan,omitempty" config:"OVH managed Valkey plan (discovery/essential or business/production)" schema:"nullable,enum=discovery|essential|business|production"`
	ValkeyVersion                  string            `yaml:"valkeyVersion,omitempty" json:"valkeyVersion,omitempty" config:"OVH managed Valkey engine version (7.2, 8.0, 8.1, 9.0, or 9.1; default 8.1)" schema:"nullable,enum=7.2|8.0|8.1|9.0|9.1"`
	ValkeyNodeCount                int               `yaml:"valkeyNodeCount,omitempty" json:"valkeyNodeCount,omitempty" config:"OVH managed Valkey node count" schema:"nullable,minimum=1"`
	ValkeyBackupTime               string            `yaml:"valkeyBackupTime,omitempty" json:"valkeyBackupTime,omitempty" config:"OVH managed Valkey daily backup start time (HH:MM)" schema:"nullable"`
	ValkeyBackupRegions            []string          `yaml:"valkeyBackupRegions,omitempty" json:"valkeyBackupRegions,omitempty" config:"OVH managed Valkey backup regions" schema:"nullable"`
	ValkeyDeletionProtection       *bool             `yaml:"valkeyDeletionProtection,omitempty" json:"valkeyDeletionProtection,omitempty" config:"Enable OVH managed Valkey deletion protection" schema:"nullable"`
	MKSPlan                        string            `yaml:"mksPlan,omitempty" json:"mksPlan,omitempty" config:"OVH Managed Kubernetes plan" schema:"nullable,enum=free|standard"`
	NodeFlavor                     string            `yaml:"nodeFlavor,omitempty" json:"nodeFlavor,omitempty" config:"MKS node pool flavor" schema:"nullable"`
	NodeCount                      int               `yaml:"nodeCount,omitempty" json:"nodeCount,omitempty" config:"MKS node pool size" schema:"nullable"`
	CPURequest                     string            `yaml:"cpuRequest,omitempty" json:"cpuRequest,omitempty" config:"Kubernetes CPU request" schema:"nullable"`
	MemoryRequest                  string            `yaml:"memoryRequest,omitempty" json:"memoryRequest,omitempty" config:"Kubernetes memory request" schema:"nullable"`
	DesiredWebReplicas             int               `yaml:"desiredWebReplicas,omitempty" json:"desiredWebReplicas,omitempty" config:"Desired web Deployment replicas" schema:"nullable"`
	QueueConsumerCount             int               `yaml:"queueConsumerCount,omitempty" json:"queueConsumerCount,omitempty" config:"Queue consumer Deployment replicas" schema:"nullable"`
	StateBucket                    string            `yaml:"stateBucket,omitempty" json:"stateBucket,omitempty" config:"S3-compatible DIY state bucket name" schema:"nullable"`
	StateEndpoint                  string            `yaml:"stateEndpoint,omitempty" json:"stateEndpoint,omitempty" config:"Loopback S3-compatible endpoint override (Floci)" schema:"nullable"`
	StateRegion                    string            `yaml:"stateRegion,omitempty" json:"stateRegion,omitempty" config:"S3-compatible region code for DIY state" schema:"nullable"`
	Labels                         map[string]string `yaml:"labels,omitempty" json:"labels,omitempty" config:"Resource labels" schema:"nullable"`
}

// ScalewayTarget holds provider-specific Scaleway semantic inputs. The adapter
// owns the SDK topology; supported topology choices remain explicit and typed
// here.
type ScalewayTarget struct {
	ProjectID                string            `yaml:"projectId" json:"projectId" config:"Scaleway project ID" schema:"minLength=1"`
	Region                   string            `yaml:"region,omitempty" json:"region,omitempty" config:"Scaleway region (e.g. fr-par)" schema:"nullable"`
	Zone                     string            `yaml:"zone,omitempty" json:"zone,omitempty" config:"Scaleway availability zone (e.g. fr-par-1)" schema:"nullable"`
	NetworkCIDR              string            `yaml:"networkCidr,omitempty" json:"networkCidr,omitempty" config:"Private network IPv4 CIDR" schema:"nullable"`
	Zones                    []string          `yaml:"zones,omitempty" json:"zones,omitempty" config:"Scaleway zones" schema:"nullable"`
	ImageDigest              string            `yaml:"imageDigest,omitempty" json:"imageDigest,omitempty" config:"Signed immutable OCI image digest" schema:"nullable"`
	DatabaseName             string            `yaml:"databaseName,omitempty" json:"databaseName,omitempty" config:"Magento database name" schema:"nullable"`
	MasterUsername           string            `yaml:"masterUsername,omitempty" json:"masterUsername,omitempty" config:"Managed MySQL master username" schema:"nullable"`
	EncryptionKeySecret      string            `yaml:"encryptionKeySecret,omitempty" json:"encryptionKeySecret,omitempty" config:"Secret reference for Magento encryption key" schema:"nullable"`
	DatabaseNodeType         string            `yaml:"databaseNodeType,omitempty" json:"databaseNodeType,omitempty" config:"Scaleway RDB node type" schema:"nullable"`
	DatabaseHighAvailability bool              `yaml:"databaseHighAvailability,omitempty" json:"databaseHighAvailability,omitempty" config:"Enable Scaleway RDB high availability"`
	DatabaseBackupEnabled    *bool             `yaml:"databaseBackupEnabled,omitempty" json:"databaseBackupEnabled,omitempty" config:"Enable Scaleway RDB automated backups" schema:"nullable"`
	DatabaseBackupFrequency  *int              `yaml:"databaseBackupFrequencyHours,omitempty" json:"databaseBackupFrequencyHours,omitempty" config:"Scaleway RDB automated backup frequency in hours" schema:"nullable,minimum=1"`
	DatabaseBackupRetention  *int              `yaml:"databaseBackupRetentionDays,omitempty" json:"databaseBackupRetentionDays,omitempty" config:"Scaleway RDB automated backup retention in days" schema:"nullable,minimum=1"`
	DatabaseBackupSameRegion *bool             `yaml:"databaseBackupSameRegion,omitempty" json:"databaseBackupSameRegion,omitempty" config:"Store Scaleway RDB logical backups in the instance region" schema:"nullable"`
	DatabaseEncryptionAtRest *bool             `yaml:"databaseEncryptionAtRest,omitempty" json:"databaseEncryptionAtRest,omitempty" config:"Enable Scaleway RDB encryption at rest" schema:"nullable"`
	RedisNodeType            string            `yaml:"redisNodeType,omitempty" json:"redisNodeType,omitempty" config:"Scaleway Redis node type" schema:"nullable"`
	RedisVersion             string            `yaml:"redisVersion,omitempty" json:"redisVersion,omitempty" config:"Scaleway Redis engine version" schema:"nullable"`
	RedisClusterSize         int               `yaml:"redisClusterSize,omitempty" json:"redisClusterSize,omitempty" config:"Scaleway Redis node count (1 standalone, 2 HA, 3-6 cluster mode)" schema:"nullable,minimum=1,maximum=6"`
	CacheMode                string            `yaml:"cacheMode,omitempty" json:"cacheMode,omitempty" config:"Cache engine escape hatch" schema:"nullable,enum=redis"`
	CPURequest               string            `yaml:"cpuRequest,omitempty" json:"cpuRequest,omitempty" config:"Kubernetes CPU request" schema:"nullable"`
	MemoryRequest            string            `yaml:"memoryRequest,omitempty" json:"memoryRequest,omitempty" config:"Kubernetes memory request" schema:"nullable"`
	DesiredWebReplicas       int               `yaml:"desiredWebReplicas,omitempty" json:"desiredWebReplicas,omitempty" config:"Desired web Deployment replicas" schema:"nullable"`
	QueueConsumerCount       int               `yaml:"queueConsumerCount,omitempty" json:"queueConsumerCount,omitempty" config:"Queue consumer Deployment replicas" schema:"nullable"`
	KapsuleVersion           string            `yaml:"kapsuleVersion,omitempty" json:"kapsuleVersion,omitempty" config:"Kapsule Kubernetes version" schema:"nullable"`
	NodeType                 string            `yaml:"nodeType,omitempty" json:"nodeType,omitempty" config:"Kapsule pool node type" schema:"nullable"`
	NodeCount                int               `yaml:"nodeCount,omitempty" json:"nodeCount,omitempty" config:"Kapsule pool size" schema:"nullable"`
	StateBucket              string            `yaml:"stateBucket,omitempty" json:"stateBucket,omitempty" config:"S3-compatible DIY state bucket name" schema:"nullable"`
	StateEndpoint            string            `yaml:"stateEndpoint,omitempty" json:"stateEndpoint,omitempty" config:"Loopback S3-compatible endpoint override (Floci)" schema:"nullable"`
	StateRegion              string            `yaml:"stateRegion,omitempty" json:"stateRegion,omitempty" config:"S3-compatible region code for DIY state" schema:"nullable"`
	Labels                   map[string]string `yaml:"labels,omitempty" json:"labels,omitempty" config:"Resource labels" schema:"nullable"`
}

// GCPTarget holds provider-specific GCP semantic inputs. The adapter owns the
// SDK topology; supported topology choices remain explicit and typed here.
type GCPTarget struct {
	Project                         string            `yaml:"project" json:"project" config:"GCP project ID" schema:"minLength=1"`
	Region                          string            `yaml:"region,omitempty" json:"region,omitempty" config:"GCP region" schema:"nullable"`
	NetworkCIDR                     string            `yaml:"networkCidr,omitempty" json:"networkCidr,omitempty" config:"VPC IPv4 CIDR" schema:"nullable"`
	Zones                           []string          `yaml:"zones,omitempty" json:"zones,omitempty" config:"GCP zones" schema:"nullable"`
	ImageDigest                     string            `yaml:"imageDigest,omitempty" json:"imageDigest,omitempty" config:"Signed immutable OCI image digest" schema:"nullable"`
	DatabaseName                    string            `yaml:"databaseName,omitempty" json:"databaseName,omitempty" config:"Magento database name" schema:"nullable"`
	MasterUsername                  string            `yaml:"masterUsername,omitempty" json:"masterUsername,omitempty" config:"Cloud SQL master username" schema:"nullable"`
	EncryptionKeySecret             string            `yaml:"encryptionKeySecret,omitempty" json:"encryptionKeySecret,omitempty" config:"Secret Manager secret ID for Magento encryption key" schema:"nullable"`
	CloudSQLTier                    string            `yaml:"cloudSqlTier,omitempty" json:"cloudSqlTier,omitempty" config:"Cloud SQL machine tier" schema:"nullable"`
	CloudSQLAvailability            string            `yaml:"cloudSqlAvailability,omitempty" json:"cloudSqlAvailability,omitempty" config:"Cloud SQL availability type" schema:"nullable,enum=ZONAL|REGIONAL"`
	CloudSQLBackupEnabled           *bool             `yaml:"cloudSqlBackupEnabled,omitempty" json:"cloudSqlBackupEnabled,omitempty" config:"Enable Cloud SQL automated backups" schema:"nullable"`
	CloudSQLBinaryLogEnabled        *bool             `yaml:"cloudSqlBinaryLogEnabled,omitempty" json:"cloudSqlBinaryLogEnabled,omitempty" config:"Enable Cloud SQL MySQL binary logging for point-in-time recovery" schema:"nullable"`
	CloudSQLBackupRetentionCount    *int              `yaml:"cloudSqlBackupRetentionCount,omitempty" json:"cloudSqlBackupRetentionCount,omitempty" config:"Cloud SQL retained automated backup count" schema:"nullable,minimum=1"`
	CloudSQLTransactionLogRetention *int              `yaml:"cloudSqlTransactionLogRetentionDays,omitempty" json:"cloudSqlTransactionLogRetentionDays,omitempty" config:"Cloud SQL MySQL transaction log retention in days (1-7)" schema:"nullable,minimum=1,maximum=7"`
	CloudSQLBackupStartTime         string            `yaml:"cloudSqlBackupStartTime,omitempty" json:"cloudSqlBackupStartTime,omitempty" config:"Cloud SQL automated backup start time (HH:MM)" schema:"nullable"`
	CloudSQLBackupLocation          string            `yaml:"cloudSqlBackupLocation,omitempty" json:"cloudSqlBackupLocation,omitempty" config:"Cloud SQL automated backup storage location" schema:"nullable"`
	CloudSQLDeletionProtection      *bool             `yaml:"cloudSqlDeletionProtection,omitempty" json:"cloudSqlDeletionProtection,omitempty" config:"Enable Cloud SQL service and IaC deletion protection" schema:"nullable"`
	MemorystoreNodeType             string            `yaml:"memorystoreNodeType,omitempty" json:"memorystoreNodeType,omitempty" config:"Memorystore for Valkey node type" schema:"nullable"`
	MemorystoreShardCount           *int              `yaml:"memorystoreShardCount,omitempty" json:"memorystoreShardCount,omitempty" config:"Memorystore for Valkey shard count" schema:"nullable,minimum=1"`
	MemorystoreReplicas             *int              `yaml:"memorystoreReplicas,omitempty" json:"memorystoreReplicas,omitempty" config:"Memorystore for Valkey replica count per shard (0-5)" schema:"nullable,minimum=0,maximum=5"`
	MemorystoreEngineVersion        string            `yaml:"memorystoreEngineVersion,omitempty" json:"memorystoreEngineVersion,omitempty" config:"Memorystore for Valkey engine version (VALKEY_9_0 GA or VALKEY_9_1 Preview)" schema:"nullable,enum=VALKEY_8_0|VALKEY_9_0|VALKEY_9_1"`
	MemorystoreMode                 string            `yaml:"memorystoreMode,omitempty" json:"memorystoreMode,omitempty" config:"Memorystore for Valkey cluster mode" schema:"nullable,enum=CLUSTER|CLUSTER_DISABLED"`
	MemorystoreZoneDistributionMode string            `yaml:"memorystoreZoneDistributionMode,omitempty" json:"memorystoreZoneDistributionMode,omitempty" config:"Memorystore for Valkey zone distribution mode" schema:"nullable,enum=MULTI_ZONE|SINGLE_ZONE"`
	MemorystoreZone                 string            `yaml:"memorystoreZone,omitempty" json:"memorystoreZone,omitempty" config:"Memorystore for Valkey single-zone placement" schema:"nullable"`
	MemorystoreDeletionProtection   *bool             `yaml:"memorystoreDeletionProtection,omitempty" json:"memorystoreDeletionProtection,omitempty" config:"Enable Memorystore for Valkey deletion protection" schema:"nullable"`
	MemorystorePSCConnectionLimit   *int              `yaml:"memorystorePscConnectionLimit,omitempty" json:"memorystorePscConnectionLimit,omitempty" config:"Maximum automatic Memorystore PSC connections" schema:"nullable,minimum=1"`
	OpenSearchMode                  string            `yaml:"openSearchMode,omitempty" json:"openSearchMode,omitempty" config:"GKE OpenSearch workload mode" schema:"nullable,enum=opensearch|disabled"`
	OpenSearchReplicas              int               `yaml:"openSearchReplicas,omitempty" json:"openSearchReplicas,omitempty" config:"GKE OpenSearch replica count" schema:"nullable"`
	OpenSearchImage                 string            `yaml:"openSearchImage,omitempty" json:"openSearchImage,omitempty" config:"GKE OpenSearch image reference" schema:"nullable"`
	QueueMode                       string            `yaml:"queueMode,omitempty" json:"queueMode,omitempty" config:"GKE queue workload mode" schema:"nullable,enum=database|rabbitmq"`
	QueueReplicas                   int               `yaml:"queueReplicas,omitempty" json:"queueReplicas,omitempty" config:"GKE RabbitMQ replica count" schema:"nullable"`
	RabbitMQImage                   string            `yaml:"rabbitMqImage,omitempty" json:"rabbitMqImage,omitempty" config:"GKE RabbitMQ image reference" schema:"nullable"`
	EnableCloudArmor                *bool             `yaml:"enableCloudArmor,omitempty" json:"enableCloudArmor,omitempty" config:"Create a GCP Cloud Armor security policy" schema:"nullable"`
	KubernetesVersion               string            `yaml:"kubernetesVersion,omitempty" json:"kubernetesVersion,omitempty" config:"GKE Kubernetes version" schema:"nullable"`
	ReleaseChannel                  string            `yaml:"releaseChannel,omitempty" json:"releaseChannel,omitempty" config:"GKE release channel" schema:"nullable,enum=RAPID|REGULAR|STABLE"`
	ClusterIPv4CIDR                 string            `yaml:"clusterIpv4Cidr,omitempty" json:"clusterIpv4Cidr,omitempty" config:"GKE cluster pod IPv4 CIDR" schema:"nullable"`
	ServicesIPv4CIDR                string            `yaml:"servicesIpv4Cidr,omitempty" json:"servicesIpv4Cidr,omitempty" config:"GKE services IPv4 CIDR" schema:"nullable"`
	StandardNodeType                string            `yaml:"standardNodeType,omitempty" json:"standardNodeType,omitempty" config:"GKE Standard node machine type" schema:"nullable"`
	StandardNodeCount               int               `yaml:"standardNodeCount,omitempty" json:"standardNodeCount,omitempty" config:"GKE Standard initial node count per zone" schema:"nullable"`
	StandardNodeMinCount            int               `yaml:"standardNodeMinCount,omitempty" json:"standardNodeMinCount,omitempty" config:"GKE Standard minimum nodes per zone" schema:"nullable"`
	StandardNodeMaxCount            int               `yaml:"standardNodeMaxCount,omitempty" json:"standardNodeMaxCount,omitempty" config:"GKE Standard maximum nodes per zone" schema:"nullable"`
	StandardNodeDiskType            string            `yaml:"standardNodeDiskType,omitempty" json:"standardNodeDiskType,omitempty" config:"GKE Standard node boot disk type" schema:"nullable"`
	StandardNodeDiskSizeGiB         int               `yaml:"standardNodeDiskSizeGiB,omitempty" json:"standardNodeDiskSizeGiB,omitempty" config:"GKE Standard node boot disk size" schema:"nullable"`
	StandardNodeImageType           string            `yaml:"standardNodeImageType,omitempty" json:"standardNodeImageType,omitempty" config:"GKE Standard node image type" schema:"nullable"`
	StandardNodeSpot                bool              `yaml:"standardNodeSpot,omitempty" json:"standardNodeSpot,omitempty" config:"Use Spot VMs for GKE Standard nodes"`
	AutopilotCPURequest             string            `yaml:"autopilotCpuRequest,omitempty" json:"autopilotCpuRequest,omitempty" config:"GKE Autopilot CPU request" schema:"nullable"`
	AutopilotMemoryRequest          string            `yaml:"autopilotMemoryRequest,omitempty" json:"autopilotMemoryRequest,omitempty" config:"GKE Autopilot memory request" schema:"nullable"`
	DesiredWebReplicas              int               `yaml:"desiredWebReplicas,omitempty" json:"desiredWebReplicas,omitempty" config:"Desired web Deployment replicas" schema:"nullable"`
	QueueConsumerCount              int               `yaml:"queueConsumerCount,omitempty" json:"queueConsumerCount,omitempty" config:"Queue consumer Deployment replicas" schema:"nullable"`
	Labels                          map[string]string `yaml:"labels,omitempty" json:"labels,omitempty" config:"Resource labels" schema:"nullable"`
}

type AWSTarget struct {
	KMSKeyARN                string               `yaml:"kmsKeyArn,omitempty" json:"kmsKeyArn,omitempty" config:"Customer-managed KMS key ARN" schema:"nullable"`
	HostedZoneID             string               `yaml:"hostedZoneId,omitempty" json:"hostedZoneId,omitempty" config:"Route 53 hosted zone ID" schema:"nullable"`
	CloudFrontCertificateARN string               `yaml:"cloudFrontCertificateArn,omitempty" json:"cloudFrontCertificateArn,omitempty" config:"us-east-1 ACM certificate ARN" schema:"nullable"`
	ALBCertificateARN        string               `yaml:"albCertificateArn,omitempty" json:"albCertificateArn,omitempty" config:"Regional ACM certificate ARN" schema:"nullable"`
	SNSTopicARN              string               `yaml:"snsTopicArn,omitempty" json:"snsTopicArn,omitempty" config:"SNS notification topic ARN" schema:"nullable"`
	ImageDigest              string               `yaml:"imageDigest,omitempty" json:"imageDigest,omitempty" config:"Signed immutable OCI image digest" schema:"nullable"`
	CacheSecretARN           string               `yaml:"cacheSecretArn,omitempty" json:"cacheSecretArn,omitempty" config:"Valkey auth token secret ARN" schema:"nullable"`
	SessionSecretARN         string               `yaml:"sessionSecretArn,omitempty" json:"sessionSecretArn,omitempty" config:"Session auth token secret ARN" schema:"nullable"`
	QueueSecretARN           string               `yaml:"queueSecretArn,omitempty" json:"queueSecretArn,omitempty" config:"RabbitMQ password secret ARN" schema:"nullable"`
	EncryptionKeySecretARN   string               `yaml:"encryptionKeySecretArn,omitempty" json:"encryptionKeySecretArn,omitempty" config:"Magento encryption key secret ARN" schema:"nullable"`
	DatabaseName             string               `yaml:"databaseName,omitempty" json:"databaseName,omitempty" config:"Magento database name" schema:"nullable"`
	MasterUsername           string               `yaml:"masterUsername,omitempty" json:"masterUsername,omitempty" config:"Database master username" schema:"nullable"`
	VPCCIDR                  string               `yaml:"vpcCidr,omitempty" json:"vpcCidr,omitempty" config:"Canonical VPC IPv4 CIDR" schema:"nullable"`
	AvailabilityZones        []string             `yaml:"availabilityZones,omitempty" json:"availabilityZones,omitempty" config:"AWS availability zones" schema:"nullable"`
	MediaDomain              string               `yaml:"mediaDomain,omitempty" json:"mediaDomain,omitempty" config:"Media delivery domain" schema:"nullable"`
	NatMode                  string               `yaml:"natMode,omitempty" json:"natMode,omitempty" config:"Private subnet egress mode" schema:"nullable,enum=nat-gateway|fck-nat"`
	NatTopology              string               `yaml:"natTopology,omitempty" json:"natTopology,omitempty" config:"AWS NAT failure-domain topology; omitted defaults to single-AZ for preview and multi-AZ for standard/high-availability" schema:"nullable,enum=single-az|multi-az"`
	NatReplacementMode       string               `yaml:"natReplacementMode,omitempty" json:"natReplacementMode,omitempty" config:"AWS fck-nat replacement mode; omitted defaults to automatic replacement for multi-AZ fck-nat" schema:"nullable,enum=none|auto-scaling"`
	NatInstanceType          string               `yaml:"natInstanceType,omitempty" json:"natInstanceType,omitempty" config:"ARM64 fck-nat instance type; omitted defaults to cost-optimized t4g.nano" schema:"nullable"`
	Labels                   map[string]string    `yaml:"labels,omitempty" json:"labels,omitempty" config:"Resource labels" schema:"nullable"`
	Existing                 AWSExistingResources `yaml:"existing,omitempty" json:"existing,omitempty" config:"Existing AWS resources" schema:"nullable"`
	Catalog                  AWSCatalog           `yaml:"catalog,omitempty" json:"catalog,omitempty" config:"Benchmark-selected service catalog"`
}

type AWSExistingResources struct {
	Network          *AWSExistingResource `yaml:"network,omitempty" json:"network,omitempty" config:"Existing VPC reference" schema:"nullable"`
	PublicSubnetIDs  []string             `yaml:"publicSubnetIds,omitempty" json:"publicSubnetIds,omitempty" config:"Existing public subnet IDs" schema:"nullable"`
	PrivateSubnetIDs []string             `yaml:"privateSubnetIds,omitempty" json:"privateSubnetIds,omitempty" config:"Existing private subnet IDs" schema:"nullable"`
	DataSubnetIDs    []string             `yaml:"dataSubnetIds,omitempty" json:"dataSubnetIds,omitempty" config:"Existing data subnet IDs" schema:"nullable"`
	Database         *AWSExistingDatabase `yaml:"database,omitempty" json:"database,omitempty" config:"Existing RDS MySQL reference" schema:"nullable"`
}

type AWSExistingResource struct {
	Provider   string `yaml:"provider" json:"provider" config:"Resource provider" schema:"const=aws"`
	Kind       string `yaml:"kind" json:"kind" config:"Resource kind"`
	ExternalID string `yaml:"externalId" json:"externalId" config:"Provider resource identifier"`
}

// AWSExistingDatabase adopts an existing AWS RDS MySQL instance by identifier.
// SecretARN and Endpoint are required so MageLift can inject credentials without
// creating or looking up the database at plan time.
type AWSExistingDatabase struct {
	Provider   string `yaml:"provider" json:"provider" config:"Resource provider" schema:"const=aws"`
	Kind       string `yaml:"kind" json:"kind" config:"Resource kind"`
	ExternalID string `yaml:"externalId" json:"externalId" config:"RDS instance identifier or ARN"`
	SecretARN  string `yaml:"secretArn" json:"secretArn" config:"Secrets Manager master-user secret ARN"`
	Endpoint   string `yaml:"endpoint" json:"endpoint" config:"RDS writer endpoint hostname"`
}

type AWSCatalog struct {
	Version        string `yaml:"version,omitempty" json:"version,omitempty" config:"Benchmark catalog version" schema:"nullable"`
	DatabaseEngine string `yaml:"databaseEngine,omitempty" json:"databaseEngine,omitempty" config:"Database engine shape" schema:"nullable,enum=aurora-mysql|rds-mysql|rds-mariadb"`
	SearchMode     string `yaml:"searchMode,omitempty" json:"searchMode,omitempty" config:"OpenSearch provisioning mode" schema:"nullable,enum=serverless|provisioned|disabled"`
	QueueMode      string `yaml:"queueMode,omitempty" json:"queueMode,omitempty" config:"Magento messaging broker mode" schema:"nullable,enum=db|amazon-mq|ecs-rabbitmq|ecs-artemis"`
	// These fields are semantic AWS durability controls. They intentionally do
	// not expose Pulumi or AWS SDK maps; omitted values retain the safe preset
	// defaults used by the AWS stack.
	DatabaseBackupWindow           string              `yaml:"databaseBackupWindow,omitempty" json:"databaseBackupWindow,omitempty" config:"AWS RDS/Aurora automated backup window in UTC" schema:"nullable"`
	DatabaseMaintenanceWindow      string              `yaml:"databaseMaintenanceWindow,omitempty" json:"databaseMaintenanceWindow,omitempty" config:"AWS RDS/Aurora maintenance window in UTC" schema:"nullable"`
	DatabaseDeletionProtection     *bool               `yaml:"databaseDeletionProtection,omitempty" json:"databaseDeletionProtection,omitempty" config:"Enable AWS RDS/Aurora deletion protection" schema:"nullable"`
	DatabaseDeleteAutomatedBackups *bool               `yaml:"databaseDeleteAutomatedBackups,omitempty" json:"databaseDeleteAutomatedBackups,omitempty" config:"Delete AWS RDS/Aurora automated backups when the database is destroyed" schema:"nullable"`
	CacheSnapshotRetentionLimit    *int                `yaml:"cacheSnapshotRetentionLimit,omitempty" json:"cacheSnapshotRetentionLimit,omitempty" config:"AWS ElastiCache Valkey automatic snapshot retention in days; zero disables snapshots" schema:"nullable,minimum=0,maximum=35"`
	CacheSnapshotWindow            string              `yaml:"cacheSnapshotWindow,omitempty" json:"cacheSnapshotWindow,omitempty" config:"AWS ElastiCache Valkey daily snapshot window in UTC" schema:"nullable"`
	Aurora                         AWSCatalogAurora    `yaml:"aurora,omitempty" json:"aurora,omitempty" config:"Aurora or RDS MySQL capacity"`
	Valkey                         AWSCatalogValkey    `yaml:"valkey,omitempty" json:"valkey,omitempty" config:"Valkey capacity"`
	Search                         AWSCatalogSearch    `yaml:"search,omitempty" json:"search,omitempty" config:"OpenSearch capacity"`
	RabbitMQ                       AWSCatalogRabbitMQ  `yaml:"rabbitMq,omitempty" json:"rabbitMq,omitempty" config:"Amazon MQ RabbitMQ capacity (queueMode amazon-mq)"`
	Fargate                        AWSCatalogFargate   `yaml:"fargate,omitempty" json:"fargate,omitempty" config:"Fargate capacity (ecs-fargate)"`
	EKS                            AWSCatalogEKS       `yaml:"eks,omitempty" json:"eks,omitempty" config:"EKS Autopilot capacity (eks)" schema:"nullable"`
	Retention                      AWSCatalogRetention `yaml:"retention,omitempty" json:"retention,omitempty" config:"Retention policy"`
	Versions                       AWSCatalogVersions  `yaml:"versions,omitempty" json:"versions,omitempty" config:"Managed service versions"`
}

type AWSCatalogAurora struct {
	MinimumACU              float64 `yaml:"minimumAcu,omitempty" json:"minimumAcu,omitempty" config:"Minimum Aurora Serverless v2 capacity"`
	MaximumACU              float64 `yaml:"maximumAcu,omitempty" json:"maximumAcu,omitempty" config:"Maximum Aurora Serverless v2 capacity"`
	AutoPauseSeconds        int     `yaml:"autoPauseSeconds,omitempty" json:"autoPauseSeconds,omitempty" config:"Aurora auto-pause duration"`
	EngineSupportsAutoPause bool    `yaml:"engineSupportsAutoPause,omitempty" json:"engineSupportsAutoPause,omitempty" config:"Whether the selected engine supports auto-pause"`
	InstanceClass           string  `yaml:"instanceClass,omitempty" json:"instanceClass,omitempty" config:"Provisioned Aurora or RDS MySQL instance class"`
	InstanceCount           int     `yaml:"instanceCount,omitempty" json:"instanceCount,omitempty" config:"Provisioned Aurora instance count"`
}

type AWSCatalogValkey struct {
	NodeType     string `yaml:"nodeType,omitempty" json:"nodeType,omitempty" config:"Valkey node type"`
	ReplicaCount int    `yaml:"replicaCount,omitempty" json:"replicaCount,omitempty" config:"Valkey replica count"`
}

type AWSCatalogRabbitMQ struct {
	InstanceType string `yaml:"instanceType,omitempty" json:"instanceType,omitempty" config:"RabbitMQ broker instance type"`
}

type AWSCatalogSearch struct {
	MaximumIndexingOCU   float64 `yaml:"maximumIndexingOcu,omitempty" json:"maximumIndexingOcu,omitempty" config:"OpenSearch Serverless indexing limit"`
	MaximumSearchOCU     float64 `yaml:"maximumSearchOcu,omitempty" json:"maximumSearchOcu,omitempty" config:"OpenSearch Serverless search limit"`
	AcceptColdStarts     bool    `yaml:"acceptColdStarts,omitempty" json:"acceptColdStarts,omitempty" config:"Accept OpenSearch Serverless cold starts"`
	InstanceType         string  `yaml:"instanceType,omitempty" json:"instanceType,omitempty" config:"Provisioned OpenSearch instance type"`
	InstanceCount        int     `yaml:"instanceCount,omitempty" json:"instanceCount,omitempty" config:"Provisioned OpenSearch data node count"`
	DedicatedMasterType  string  `yaml:"dedicatedMasterType,omitempty" json:"dedicatedMasterType,omitempty" config:"OpenSearch dedicated master type"`
	DedicatedMasterCount int     `yaml:"dedicatedMasterCount,omitempty" json:"dedicatedMasterCount,omitempty" config:"OpenSearch dedicated master count"`
	EBSVolumeType        string  `yaml:"ebsVolumeType,omitempty" json:"ebsVolumeType,omitempty" config:"OpenSearch EBS volume type"`
	EBSVolumeSizeGiB     int     `yaml:"ebsVolumeSizeGiB,omitempty" json:"ebsVolumeSizeGiB,omitempty" config:"OpenSearch EBS volume size"`
}

type AWSCatalogFargate struct {
	ComputeMode  string `yaml:"computeMode,omitempty" json:"computeMode,omitempty" config:"ECS capacity mode" schema:"nullable,enum=fargate|fargate-spot|ec2-asg|managed-instances"`
	CPU          int    `yaml:"cpu,omitempty" json:"cpu,omitempty" config:"Fargate task CPU"`
	MemoryMiB    int    `yaml:"memoryMiB,omitempty" json:"memoryMiB,omitempty" config:"Fargate task memory"`
	DesiredCount int    `yaml:"desiredCount,omitempty" json:"desiredCount,omitempty" config:"Fargate desired task count"`
	InstanceType string `yaml:"instanceType,omitempty" json:"instanceType,omitempty" config:"EC2 or managed-instance capacity type" schema:"nullable"`
	InstanceAMI  string `yaml:"instanceAmi,omitempty" json:"instanceAmi,omitempty" config:"Pinned AMI for ECS EC2 capacity" schema:"nullable"`
	MinCapacity  int    `yaml:"minCapacity,omitempty" json:"minCapacity,omitempty" config:"Minimum host capacity" schema:"nullable"`
	MaxCapacity  int    `yaml:"maxCapacity,omitempty" json:"maxCapacity,omitempty" config:"Maximum host capacity" schema:"nullable"`
}

// AWSCatalogEKS selects the Kubernetes compute and in-cluster capability shape
// for the EKS architecture family. ComputeMode is deliberately explicit:
// Auto Mode, managed node groups, self-managed nodes, and Fargate have
// different scheduling, storage, failure, and operational boundaries.
type AWSCatalogEKS struct {
	ComputeMode        string   `yaml:"computeMode,omitempty" json:"computeMode,omitempty" config:"EKS compute mode" schema:"nullable,enum=auto-mode|managed-node-groups|self-managed|fargate"`
	KubernetesVersion  string   `yaml:"kubernetesVersion,omitempty" json:"kubernetesVersion,omitempty" config:"EKS Kubernetes minor version" schema:"nullable"`
	CPURequest         string   `yaml:"cpuRequest,omitempty" json:"cpuRequest,omitempty" config:"Web/cron CPU request" schema:"nullable"`
	MemoryRequest      string   `yaml:"memoryRequest,omitempty" json:"memoryRequest,omitempty" config:"Web/cron memory request" schema:"nullable"`
	DesiredWebReplicas int      `yaml:"desiredWebReplicas,omitempty" json:"desiredWebReplicas,omitempty" config:"Desired web Deployment replicas" schema:"nullable"`
	QueueConsumerCount int      `yaml:"queueConsumerCount,omitempty" json:"queueConsumerCount,omitempty" config:"Queue consumer Deployment replicas" schema:"nullable"`
	SearchMode         string   `yaml:"searchMode,omitempty" json:"searchMode,omitempty" config:"EKS OpenSearch workload mode" schema:"nullable,enum=opensearch|disabled"`
	SearchReplicas     int      `yaml:"searchReplicas,omitempty" json:"searchReplicas,omitempty" config:"OpenSearch StatefulSet replicas" schema:"nullable"`
	QueueMode          string   `yaml:"queueMode,omitempty" json:"queueMode,omitempty" config:"EKS queue workload mode" schema:"nullable,enum=database|rabbitmq"`
	QueueReplicas      int      `yaml:"queueReplicas,omitempty" json:"queueReplicas,omitempty" config:"RabbitMQ StatefulSet replicas" schema:"nullable"`
	NodeInstanceType   string   `yaml:"nodeInstanceType,omitempty" json:"nodeInstanceType,omitempty" config:"EKS node instance type" schema:"nullable"`
	NodeAMI            string   `yaml:"nodeAmi,omitempty" json:"nodeAmi,omitempty" config:"Pinned AMI for self-managed EKS nodes" schema:"nullable"`
	NodeMinSize        int      `yaml:"nodeMinSize,omitempty" json:"nodeMinSize,omitempty" config:"Minimum EKS node count" schema:"nullable"`
	NodeDesiredSize    int      `yaml:"nodeDesiredSize,omitempty" json:"nodeDesiredSize,omitempty" config:"Desired EKS node count" schema:"nullable"`
	NodeMaxSize        int      `yaml:"nodeMaxSize,omitempty" json:"nodeMaxSize,omitempty" config:"Maximum EKS node count" schema:"nullable"`
	FargateNamespaces  []string `yaml:"fargateNamespaces,omitempty" json:"fargateNamespaces,omitempty" config:"Namespaces scheduled on EKS Fargate" schema:"nullable"`
}

type AWSCatalogRetention struct {
	LogDays      int `yaml:"logDays,omitempty" json:"logDays,omitempty" config:"CloudWatch log retention days"`
	BackupDays   int `yaml:"backupDays,omitempty" json:"backupDays,omitempty" config:"Database backup retention days"`
	ArtifactDays int `yaml:"artifactDays,omitempty" json:"artifactDays,omitempty" config:"Artifact retention days"`
}

type AWSCatalogVersions struct {
	AuroraMySQL string `yaml:"auroraMysql,omitempty" json:"auroraMysql,omitempty" config:"Aurora MySQL engine version"`
	MySQL       string `yaml:"mysql,omitempty" json:"mysql,omitempty" config:"RDS MySQL engine version"`
	MariaDB     string `yaml:"mariaDb,omitempty" json:"mariaDb,omitempty" config:"RDS MariaDB engine version"`
	Valkey      string `yaml:"valkey,omitempty" json:"valkey,omitempty" config:"Valkey engine version"`
	OpenSearch  string `yaml:"openSearch,omitempty" json:"openSearch,omitempty" config:"OpenSearch engine version"`
	RabbitMQ    string `yaml:"rabbitMq,omitempty" json:"rabbitMq,omitempty" config:"RabbitMQ engine version"`
}
type Defaults struct {
	Region string `yaml:"region,omitempty" json:"region,omitempty" config:"Default AWS region"`
	Preset string `yaml:"preset,omitempty" json:"preset,omitempty" config:"Default infrastructure preset" schema:"enum=preview|standard|high-availability"`
}
type Compatibility struct {
	AllowUnsupported bool `yaml:"allowUnsupported,omitempty" json:"allowUnsupported,omitempty" config:"Allow an unsupported service combination"`
}

type Environment struct {
	Inherits           string               `yaml:"inherits,omitempty" json:"inherits,omitempty" config:"Parent environment"`
	Account            string               `yaml:"account,omitempty" json:"account,omitempty" config:"AWS account ID" schema:"nullable"`
	Class              string               `yaml:"class,omitempty" json:"class,omitempty" config:"Environment class" schema:"nullable"`
	Preset             string               `yaml:"preset,omitempty" json:"preset,omitempty" config:"Infrastructure preset" schema:"nullable,enum=preview|standard|high-availability"`
	Domain             string               `yaml:"domain,omitempty" json:"domain,omitempty" config:"Environment domain" schema:"nullable"`
	Protection         bool                 `yaml:"protection,omitempty" json:"protection,omitempty" config:"Protect against destructive commands" schema:"nullable"`
	ExpiresAt          string               `yaml:"expiresAt,omitempty" json:"expiresAt,omitempty" config:"Preview expiration as RFC3339" schema:"nullable"`
	MonthlyBudgetCents int64                `yaml:"monthlyBudgetCents,omitempty" json:"monthlyBudgetCents,omitempty" config:"Maximum monthly AWS budget in cents" schema:"nullable"`
	Branches           []string             `yaml:"branches,omitempty" json:"branches,omitempty" config:"Git branches mapped to this environment" schema:"nullable"`
	SeedDump           string               `yaml:"seedDump,omitempty" json:"seedDump,omitempty" config:"Local MySQL dump path seeded after first deploy" schema:"nullable"`
	Project            *Project             `yaml:"project,omitempty" json:"project,omitempty"`
	Application        *Application         `yaml:"application,omitempty" json:"application,omitempty"`
	Build              *Build               `yaml:"build,omitempty" json:"build,omitempty"`
	Target             *Target              `yaml:"target,omitempty" json:"target,omitempty"`
	Edge               *EdgeConfig          `yaml:"edge,omitempty" json:"edge,omitempty"`
	Observability      *ObservabilityConfig `yaml:"observability,omitempty" json:"observability,omitempty"`
	Email              *EmailConfig         `yaml:"email,omitempty" json:"email,omitempty"`
	Resilience         *ResilienceConfig    `yaml:"resilience,omitempty" json:"resilience,omitempty"`
	Defaults           *Defaults            `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Compatibility      *Compatibility       `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`
	Extensions         map[string]any       `yaml:"extensions,omitempty" json:"extensions,omitempty"`
}

type document struct {
	SchemaVersion int                    `yaml:"schemaVersion" config:"MageLift configuration schema version" schema:"const=1"`
	Project       Project                `yaml:"project" config:"Project identity"`
	Application   Application            `yaml:"application" config:"Magento application settings"`
	Build         Build                  `yaml:"build" config:"Application build settings"`
	Local         LocalRuntime           `yaml:"local,omitempty" config:"Local runtime service and email choices"`
	Target        Target                 `yaml:"target" config:"Deployment target"`
	Edge          EdgeConfig             `yaml:"edge,omitempty" config:"Optional edge delivery provider"`
	Observability ObservabilityConfig    `yaml:"observability,omitempty" config:"Optional observability provider"`
	Email         EmailConfig            `yaml:"email,omitempty" config:"Cloud transactional email"`
	Resilience    ResilienceConfig       `yaml:"resilience,omitempty" config:"Availability, backup, and disaster-recovery policy"`
	Defaults      Defaults               `yaml:"defaults" config:"Project defaults"`
	Compatibility Compatibility          `yaml:"compatibility,omitempty" config:"Compatibility policy"`
	Environments  map[string]Environment `yaml:"environments,omitempty" config:"Named deployment environments" schema:"required,minProperties=1"`
	Extensions    map[string]any         `yaml:"extensions,omitempty" config:"Namespaced extension settings"`
}

type Provenance struct {
	Source  string `json:"source" yaml:"source"`
	Removed bool   `json:"removed,omitempty" yaml:"removed,omitempty"`
}

type Effective struct {
	Config        Config                  `json:"config" yaml:"config"`
	Provenance    map[string]Provenance   `json:"provenance" yaml:"provenance"`
	Compatibility CompatibilityAssessment `json:"compatibility" yaml:"compatibility"`
	Fingerprint   string                  `json:"fingerprint" yaml:"fingerprint"`
}
