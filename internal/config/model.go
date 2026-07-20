package config

type Config struct {
	SchemaVersion      int            `yaml:"schemaVersion" json:"schemaVersion" config:"MageLift configuration schema version" schema:"const=1"`
	Project            Project        `yaml:"project" json:"project"`
	Application        Application    `yaml:"application" json:"application"`
	Build              Build          `yaml:"build" json:"build"`
	Target             Target         `yaml:"target" json:"target"`
	Defaults           Defaults       `yaml:"defaults" json:"defaults"`
	Compatibility      Compatibility  `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`
	Extensions         map[string]any `yaml:"extensions,omitempty" json:"extensions,omitempty"`
	Account            string         `yaml:"account,omitempty" json:"account,omitempty"`
	Class              string         `yaml:"class,omitempty" json:"class,omitempty"`
	Preset             string         `yaml:"preset,omitempty" json:"preset,omitempty"`
	Domain             string         `yaml:"domain,omitempty" json:"domain,omitempty"`
	Protection         bool           `yaml:"protection,omitempty" json:"protection,omitempty"`
	ExpiresAt          string         `yaml:"expiresAt,omitempty" json:"expiresAt,omitempty" config:"Preview expiration as RFC3339" schema:"nullable"`
	MonthlyBudgetCents int64          `yaml:"monthlyBudgetCents,omitempty" json:"monthlyBudgetCents,omitempty" config:"Maximum monthly AWS budget in cents" schema:"nullable"`
	Branches           []string       `yaml:"branches,omitempty" json:"branches,omitempty"`
}

type Project struct {
	Name string `yaml:"name" json:"name" config:"Project name" schema:"minLength=1"`
}

type Application struct {
	Edition    string `yaml:"edition" json:"edition" config:"Magento edition" schema:"enum=open-source|commerce"`
	Version    string `yaml:"version" json:"version" config:"Exact Magento release"`
	Mode       string `yaml:"mode" json:"mode" config:"Application mode" schema:"enum=integrated|headless"`
	WebRuntime string `yaml:"webRuntime,omitempty" json:"webRuntime,omitempty" config:"HTTP application runtime" schema:"enum=nginx-fpm|frankenphp-classic"`
}

type Build struct {
	PHP           string               `yaml:"php" json:"php" config:"Exact PHP branch or patch version"`
	Composer      Composer             `yaml:"composer,omitempty" json:"composer,omitempty" config:"Composer settings"`
	StaticContent map[string]any       `yaml:"staticContent,omitempty" json:"staticContent,omitempty" config:"Static content deployment settings"`
	Hooks         map[string]BuildHook `yaml:"hooks,omitempty" json:"hooks,omitempty" config:"Build lifecycle hooks"`
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
	Credentials string `yaml:"credentials,omitempty" json:"credentials,omitempty" config:"Composer credentials secret reference" schema:"pattern=^(aws-secrets-manager|ssm|gcp-secret-manager)://\\S+$"`
}
type Target struct {
	Provider string          `yaml:"provider" json:"provider" config:"Infrastructure provider" schema:"enum=aws|gcp|ovh|scaleway"`
	Runtime  string          `yaml:"runtime" json:"runtime" config:"Application runtime" schema:"enum=ecs-fargate|eks-autopilot|gke-autopilot|mks|kapsule"`
	AWS      *AWSTarget      `yaml:"aws,omitempty" json:"aws,omitempty" config:"AWS deployment inputs"`
	GCP      *GCPTarget      `yaml:"gcp,omitempty" json:"gcp,omitempty" config:"GCP deployment inputs (experimental)"`
	OVH      *OVHTarget      `yaml:"ovh,omitempty" json:"ovh,omitempty" config:"OVHcloud deployment inputs (experimental)"`
	Scaleway *ScalewayTarget `yaml:"scaleway,omitempty" json:"scaleway,omitempty" config:"Scaleway deployment inputs (experimental)"`
}

// OVHTarget holds provider-specific OVHcloud inputs. Topology stays out of portable YAML.
type OVHTarget struct {
	ServiceName         string            `yaml:"serviceName" json:"serviceName" config:"OVH Public Cloud project service name" schema:"minLength=1"`
	Region              string            `yaml:"region,omitempty" json:"region,omitempty" config:"OVH Public Cloud region (e.g. GRA9)" schema:"nullable"`
	NetworkCIDR         string            `yaml:"networkCidr,omitempty" json:"networkCidr,omitempty" config:"Private network IPv4 CIDR" schema:"nullable"`
	Zones               []string          `yaml:"zones,omitempty" json:"zones,omitempty" config:"OVH regions used as network availability zones" schema:"nullable"`
	ImageDigest         string            `yaml:"imageDigest,omitempty" json:"imageDigest,omitempty" config:"Signed immutable OCI image digest" schema:"nullable"`
	DatabaseName        string            `yaml:"databaseName,omitempty" json:"databaseName,omitempty" config:"Magento database name" schema:"nullable"`
	MasterUsername      string            `yaml:"masterUsername,omitempty" json:"masterUsername,omitempty" config:"Managed MySQL master username" schema:"nullable"`
	EncryptionKeySecret string            `yaml:"encryptionKeySecret,omitempty" json:"encryptionKeySecret,omitempty" config:"Secret reference for Magento encryption key" schema:"nullable"`
	DatabaseFlavor      string            `yaml:"databaseFlavor,omitempty" json:"databaseFlavor,omitempty" config:"OVH managed MySQL flavor" schema:"nullable"`
	DatabasePlan        string            `yaml:"databasePlan,omitempty" json:"databasePlan,omitempty" config:"OVH managed MySQL plan" schema:"nullable"`
	ValkeyFlavor        string            `yaml:"valkeyFlavor,omitempty" json:"valkeyFlavor,omitempty" config:"OVH managed Valkey flavor" schema:"nullable"`
	ValkeyPlan          string            `yaml:"valkeyPlan,omitempty" json:"valkeyPlan,omitempty" config:"OVH managed Valkey plan" schema:"nullable"`
	NodeFlavor          string            `yaml:"nodeFlavor,omitempty" json:"nodeFlavor,omitempty" config:"MKS node pool flavor" schema:"nullable"`
	NodeCount           int               `yaml:"nodeCount,omitempty" json:"nodeCount,omitempty" config:"MKS node pool size" schema:"nullable"`
	CPURequest          string            `yaml:"cpuRequest,omitempty" json:"cpuRequest,omitempty" config:"Kubernetes CPU request" schema:"nullable"`
	MemoryRequest       string            `yaml:"memoryRequest,omitempty" json:"memoryRequest,omitempty" config:"Kubernetes memory request" schema:"nullable"`
	DesiredWebReplicas  int               `yaml:"desiredWebReplicas,omitempty" json:"desiredWebReplicas,omitempty" config:"Desired web Deployment replicas" schema:"nullable"`
	QueueConsumerCount  int               `yaml:"queueConsumerCount,omitempty" json:"queueConsumerCount,omitempty" config:"Queue consumer Deployment replicas" schema:"nullable"`
	Labels              map[string]string `yaml:"labels,omitempty" json:"labels,omitempty" config:"Resource labels" schema:"nullable"`
}

// ScalewayTarget holds provider-specific Scaleway inputs. Topology stays out of portable YAML.
type ScalewayTarget struct {
	ProjectID           string            `yaml:"projectId" json:"projectId" config:"Scaleway project ID" schema:"minLength=1"`
	Region              string            `yaml:"region,omitempty" json:"region,omitempty" config:"Scaleway region (e.g. fr-par)" schema:"nullable"`
	Zone                string            `yaml:"zone,omitempty" json:"zone,omitempty" config:"Scaleway availability zone (e.g. fr-par-1)" schema:"nullable"`
	NetworkCIDR         string            `yaml:"networkCidr,omitempty" json:"networkCidr,omitempty" config:"Private network IPv4 CIDR" schema:"nullable"`
	Zones               []string          `yaml:"zones,omitempty" json:"zones,omitempty" config:"Scaleway zones" schema:"nullable"`
	ImageDigest         string            `yaml:"imageDigest,omitempty" json:"imageDigest,omitempty" config:"Signed immutable OCI image digest" schema:"nullable"`
	DatabaseName        string            `yaml:"databaseName,omitempty" json:"databaseName,omitempty" config:"Magento database name" schema:"nullable"`
	MasterUsername      string            `yaml:"masterUsername,omitempty" json:"masterUsername,omitempty" config:"Managed MySQL master username" schema:"nullable"`
	EncryptionKeySecret string            `yaml:"encryptionKeySecret,omitempty" json:"encryptionKeySecret,omitempty" config:"Secret reference for Magento encryption key" schema:"nullable"`
	DatabaseNodeType    string            `yaml:"databaseNodeType,omitempty" json:"databaseNodeType,omitempty" config:"Scaleway RDB node type" schema:"nullable"`
	RedisNodeType       string            `yaml:"redisNodeType,omitempty" json:"redisNodeType,omitempty" config:"Scaleway Redis node type" schema:"nullable"`
	CacheMode           string            `yaml:"cacheMode,omitempty" json:"cacheMode,omitempty" config:"Cache engine escape hatch" schema:"nullable,enum=redis"`
	CPURequest          string            `yaml:"cpuRequest,omitempty" json:"cpuRequest,omitempty" config:"Kubernetes CPU request" schema:"nullable"`
	MemoryRequest       string            `yaml:"memoryRequest,omitempty" json:"memoryRequest,omitempty" config:"Kubernetes memory request" schema:"nullable"`
	DesiredWebReplicas  int               `yaml:"desiredWebReplicas,omitempty" json:"desiredWebReplicas,omitempty" config:"Desired web Deployment replicas" schema:"nullable"`
	QueueConsumerCount  int               `yaml:"queueConsumerCount,omitempty" json:"queueConsumerCount,omitempty" config:"Queue consumer Deployment replicas" schema:"nullable"`
	KapsuleVersion      string            `yaml:"kapsuleVersion,omitempty" json:"kapsuleVersion,omitempty" config:"Kapsule Kubernetes version" schema:"nullable"`
	NodeType            string            `yaml:"nodeType,omitempty" json:"nodeType,omitempty" config:"Kapsule pool node type" schema:"nullable"`
	NodeCount           int               `yaml:"nodeCount,omitempty" json:"nodeCount,omitempty" config:"Kapsule pool size" schema:"nullable"`
	Labels              map[string]string `yaml:"labels,omitempty" json:"labels,omitempty" config:"Resource labels" schema:"nullable"`
}

// GCPTarget holds provider-specific GCP inputs. Topology stays out of portable YAML.
type GCPTarget struct {
	Project                string            `yaml:"project" json:"project" config:"GCP project ID" schema:"minLength=1"`
	Region                 string            `yaml:"region,omitempty" json:"region,omitempty" config:"GCP region" schema:"nullable"`
	NetworkCIDR            string            `yaml:"networkCidr,omitempty" json:"networkCidr,omitempty" config:"VPC IPv4 CIDR" schema:"nullable"`
	Zones                  []string          `yaml:"zones,omitempty" json:"zones,omitempty" config:"GCP zones" schema:"nullable"`
	ImageDigest            string            `yaml:"imageDigest,omitempty" json:"imageDigest,omitempty" config:"Signed immutable OCI image digest" schema:"nullable"`
	DatabaseName           string            `yaml:"databaseName,omitempty" json:"databaseName,omitempty" config:"Magento database name" schema:"nullable"`
	MasterUsername         string            `yaml:"masterUsername,omitempty" json:"masterUsername,omitempty" config:"Cloud SQL master username" schema:"nullable"`
	EncryptionKeySecret    string            `yaml:"encryptionKeySecret,omitempty" json:"encryptionKeySecret,omitempty" config:"Secret Manager secret ID for Magento encryption key" schema:"nullable"`
	CloudSQLTier           string            `yaml:"cloudSqlTier,omitempty" json:"cloudSqlTier,omitempty" config:"Cloud SQL machine tier" schema:"nullable"`
	MemorystoreNodeType    string            `yaml:"memorystoreNodeType,omitempty" json:"memorystoreNodeType,omitempty" config:"Memorystore for Valkey node type" schema:"nullable"`
	AutopilotCPURequest    string            `yaml:"autopilotCpuRequest,omitempty" json:"autopilotCpuRequest,omitempty" config:"GKE Autopilot CPU request" schema:"nullable"`
	AutopilotMemoryRequest string            `yaml:"autopilotMemoryRequest,omitempty" json:"autopilotMemoryRequest,omitempty" config:"GKE Autopilot memory request" schema:"nullable"`
	DesiredWebReplicas     int               `yaml:"desiredWebReplicas,omitempty" json:"desiredWebReplicas,omitempty" config:"Desired web Deployment replicas" schema:"nullable"`
	QueueConsumerCount     int               `yaml:"queueConsumerCount,omitempty" json:"queueConsumerCount,omitempty" config:"Queue consumer Deployment replicas" schema:"nullable"`
	Labels                 map[string]string `yaml:"labels,omitempty" json:"labels,omitempty" config:"Resource labels" schema:"nullable"`
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
	Existing                 AWSExistingResources `yaml:"existing,omitempty" json:"existing,omitempty" config:"Existing AWS resources" schema:"nullable"`
	Catalog                  AWSCatalog           `yaml:"catalog,omitempty" json:"catalog,omitempty" config:"Benchmark-selected service catalog"`
}

type AWSExistingResources struct {
	Network          *AWSExistingResource `yaml:"network,omitempty" json:"network,omitempty" config:"Existing VPC reference" schema:"nullable"`
	PublicSubnetIDs  []string             `yaml:"publicSubnetIds,omitempty" json:"publicSubnetIds,omitempty" config:"Existing public subnet IDs" schema:"nullable"`
	PrivateSubnetIDs []string             `yaml:"privateSubnetIds,omitempty" json:"privateSubnetIds,omitempty" config:"Existing private subnet IDs" schema:"nullable"`
	DataSubnetIDs    []string             `yaml:"dataSubnetIds,omitempty" json:"dataSubnetIds,omitempty" config:"Existing data subnet IDs" schema:"nullable"`
}

type AWSExistingResource struct {
	Provider   string `yaml:"provider" json:"provider" config:"Resource provider" schema:"const=aws"`
	Kind       string `yaml:"kind" json:"kind" config:"Resource kind"`
	ExternalID string `yaml:"externalId" json:"externalId" config:"Provider resource identifier"`
}

type AWSCatalog struct {
	Version        string              `yaml:"version,omitempty" json:"version,omitempty" config:"Benchmark catalog version" schema:"nullable"`
	DatabaseEngine string              `yaml:"databaseEngine,omitempty" json:"databaseEngine,omitempty" config:"MySQL engine shape" schema:"nullable,enum=aurora-mysql|rds-mysql"`
	SearchMode     string              `yaml:"searchMode,omitempty" json:"searchMode,omitempty" config:"OpenSearch provisioning mode" schema:"nullable,enum=serverless|provisioned|disabled"`
	Aurora         AWSCatalogAurora    `yaml:"aurora,omitempty" json:"aurora,omitempty" config:"Aurora or RDS MySQL capacity"`
	Valkey         AWSCatalogValkey    `yaml:"valkey,omitempty" json:"valkey,omitempty" config:"Valkey capacity"`
	Search         AWSCatalogSearch    `yaml:"search,omitempty" json:"search,omitempty" config:"OpenSearch capacity"`
	RabbitMQ       AWSCatalogRabbitMQ  `yaml:"rabbitMq,omitempty" json:"rabbitMq,omitempty" config:"RabbitMQ capacity"`
	Fargate        AWSCatalogFargate   `yaml:"fargate,omitempty" json:"fargate,omitempty" config:"Fargate capacity (ecs-fargate)"`
	EKS            AWSCatalogEKS       `yaml:"eks,omitempty" json:"eks,omitempty" config:"EKS Autopilot capacity (eks-autopilot)" schema:"nullable"`
	Retention      AWSCatalogRetention `yaml:"retention,omitempty" json:"retention,omitempty" config:"Retention policy"`
	Versions       AWSCatalogVersions  `yaml:"versions,omitempty" json:"versions,omitempty" config:"Managed service versions"`
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
	CPU          int `yaml:"cpu,omitempty" json:"cpu,omitempty" config:"Fargate task CPU"`
	MemoryMiB    int `yaml:"memoryMiB,omitempty" json:"memoryMiB,omitempty" config:"Fargate task memory"`
	DesiredCount int `yaml:"desiredCount,omitempty" json:"desiredCount,omitempty" config:"Fargate desired task count"`
}

// AWSCatalogEKS sizes Magento workloads on EKS Auto Mode (experimental).
type AWSCatalogEKS struct {
	CPURequest         string `yaml:"cpuRequest,omitempty" json:"cpuRequest,omitempty" config:"Web/cron CPU request" schema:"nullable"`
	MemoryRequest      string `yaml:"memoryRequest,omitempty" json:"memoryRequest,omitempty" config:"Web/cron memory request" schema:"nullable"`
	DesiredWebReplicas int    `yaml:"desiredWebReplicas,omitempty" json:"desiredWebReplicas,omitempty" config:"Desired web Deployment replicas" schema:"nullable"`
	QueueConsumerCount int    `yaml:"queueConsumerCount,omitempty" json:"queueConsumerCount,omitempty" config:"Queue consumer Deployment replicas" schema:"nullable"`
}

type AWSCatalogRetention struct {
	LogDays      int `yaml:"logDays,omitempty" json:"logDays,omitempty" config:"CloudWatch log retention days"`
	BackupDays   int `yaml:"backupDays,omitempty" json:"backupDays,omitempty" config:"Database backup retention days"`
	ArtifactDays int `yaml:"artifactDays,omitempty" json:"artifactDays,omitempty" config:"Artifact retention days"`
}

type AWSCatalogVersions struct {
	AuroraMySQL string `yaml:"auroraMysql,omitempty" json:"auroraMysql,omitempty" config:"Aurora MySQL engine version"`
	MySQL       string `yaml:"mysql,omitempty" json:"mysql,omitempty" config:"RDS MySQL engine version"`
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
	Inherits           string         `yaml:"inherits,omitempty" json:"inherits,omitempty" config:"Parent environment"`
	Account            string         `yaml:"account,omitempty" json:"account,omitempty" config:"AWS account ID" schema:"nullable"`
	Class              string         `yaml:"class,omitempty" json:"class,omitempty" config:"Environment class" schema:"nullable"`
	Preset             string         `yaml:"preset,omitempty" json:"preset,omitempty" config:"Infrastructure preset" schema:"nullable,enum=preview|standard|high-availability"`
	Domain             string         `yaml:"domain,omitempty" json:"domain,omitempty" config:"Environment domain" schema:"nullable"`
	Protection         bool           `yaml:"protection,omitempty" json:"protection,omitempty" config:"Protect against destructive commands" schema:"nullable"`
	ExpiresAt          string         `yaml:"expiresAt,omitempty" json:"expiresAt,omitempty" config:"Preview expiration as RFC3339" schema:"nullable"`
	MonthlyBudgetCents int64          `yaml:"monthlyBudgetCents,omitempty" json:"monthlyBudgetCents,omitempty" config:"Maximum monthly AWS budget in cents" schema:"nullable"`
	Branches           []string       `yaml:"branches,omitempty" json:"branches,omitempty" config:"Git branches mapped to this environment" schema:"nullable"`
	Project            *Project       `yaml:"project,omitempty" json:"project,omitempty"`
	Application        *Application   `yaml:"application,omitempty" json:"application,omitempty"`
	Build              *Build         `yaml:"build,omitempty" json:"build,omitempty"`
	Target             *Target        `yaml:"target,omitempty" json:"target,omitempty"`
	Defaults           *Defaults      `yaml:"defaults,omitempty" json:"defaults,omitempty"`
	Compatibility      *Compatibility `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`
	Extensions         map[string]any `yaml:"extensions,omitempty" json:"extensions,omitempty"`
}

type document struct {
	SchemaVersion int                    `yaml:"schemaVersion" config:"MageLift configuration schema version" schema:"const=1"`
	Project       Project                `yaml:"project" config:"Project identity"`
	Application   Application            `yaml:"application" config:"Magento application settings"`
	Build         Build                  `yaml:"build" config:"Application build settings"`
	Target        Target                 `yaml:"target" config:"Deployment target"`
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
}
