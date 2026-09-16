// Package schema owns the GCP provider target schema: parsing plus
// validation of the raw `target.gcp` document. It moved here from
// internal/config (struct) and the config validator (rules) for the
// autonomous provider; the core validates envelope presence only. Only the
// yaml/json struct tags are live in this package.
package schema

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

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

// Validate checks a parsed GCP target block plus runtime. A nil target fails
// closed: providers never plan an absent target.
func Validate(target *GCPTarget, runtime string) []string {
	if target == nil {
		return []string{"target.gcp is required"}
	}
	var problems []string
	if strings.TrimSpace(target.Project) == "" {
		problems = append(problems, "target.gcp.project is required when provider is gcp")
	}
	if runtime != "gke-autopilot" && runtime != "gke-standard" {
		problems = append(problems, "target.runtime must be gke-autopilot or gke-standard when provider is gcp")
	}

	if value := strings.TrimSpace(target.CloudSQLAvailability); value != "" && value != "ZONAL" && value != "REGIONAL" {
		problems = append(problems, "target.gcp.cloudSqlAvailability must be ZONAL or REGIONAL")
	}
	if target.CloudSQLBackupRetentionCount != nil && *target.CloudSQLBackupRetentionCount < 1 {
		problems = append(problems, "target.gcp.cloudSqlBackupRetentionCount must be at least 1")
	}
	if target.CloudSQLTransactionLogRetention != nil && (*target.CloudSQLTransactionLogRetention < 1 || *target.CloudSQLTransactionLogRetention > 7) {
		problems = append(problems, "target.gcp.cloudSqlTransactionLogRetentionDays must be between 1 and 7")
	}
	if target.CloudSQLBackupRetentionCount != nil && target.CloudSQLTransactionLogRetention != nil && *target.CloudSQLTransactionLogRetention > *target.CloudSQLBackupRetentionCount {
		problems = append(problems, "target.gcp.cloudSqlTransactionLogRetentionDays cannot exceed cloudSqlBackupRetentionCount")
	}
	if target.CloudSQLBackupStartTime != "" && !validClockTime(target.CloudSQLBackupStartTime) {
		problems = append(problems, "target.gcp.cloudSqlBackupStartTime must use HH:MM")
	}
	for name, value := range map[string]string{
		"cloudSqlBackupStartTime": target.CloudSQLBackupStartTime,
		"cloudSqlBackupLocation":  target.CloudSQLBackupLocation,
	} {
		if strings.ContainsAny(value, "\r\n\x00") {
			problems = append(problems, fmt.Sprintf("target.gcp.%s must not contain control characters", name))
		}
	}
	if strings.TrimSpace(target.CloudSQLAvailability) == "REGIONAL" {
		if target.CloudSQLBackupEnabled != nil && !*target.CloudSQLBackupEnabled {
			problems = append(problems, "target.gcp.cloudSqlBackupEnabled cannot be false for REGIONAL Cloud SQL")
		}
		if target.CloudSQLBinaryLogEnabled != nil && !*target.CloudSQLBinaryLogEnabled {
			problems = append(problems, "target.gcp.cloudSqlBinaryLogEnabled cannot be false for REGIONAL MySQL Cloud SQL")
		}
	}
	if target.CloudSQLBackupEnabled != nil && !*target.CloudSQLBackupEnabled && (target.CloudSQLBackupRetentionCount != nil || target.CloudSQLTransactionLogRetention != nil || target.CloudSQLBackupStartTime != "" || target.CloudSQLBackupLocation != "") {
		problems = append(problems, "target.gcp Cloud SQL backup retention, schedule, and location settings require cloudSqlBackupEnabled")
	}
	if target.CloudSQLBinaryLogEnabled != nil && !*target.CloudSQLBinaryLogEnabled && target.CloudSQLTransactionLogRetention != nil {
		problems = append(problems, "target.gcp.cloudSqlTransactionLogRetentionDays requires cloudSqlBinaryLogEnabled")
	}
	if target.MemorystorePSCConnectionLimit != nil && *target.MemorystorePSCConnectionLimit < 1 {
		problems = append(problems, "target.gcp.memorystorePscConnectionLimit must be at least 1")
	}
	if value := strings.TrimSpace(target.OpenSearchMode); value != "" && value != "opensearch" && value != "disabled" {
		problems = append(problems, "target.gcp.openSearchMode must be opensearch or disabled")
	}
	if value := strings.TrimSpace(target.QueueMode); value != "" && value != "database" && value != "rabbitmq" {
		problems = append(problems, "target.gcp.queueMode must be database or rabbitmq")
	}
	if value := strings.TrimSpace(target.ReleaseChannel); value != "" && value != "RAPID" && value != "REGULAR" && value != "STABLE" {
		problems = append(problems, "target.gcp.releaseChannel must be RAPID, REGULAR, or STABLE")
	}
	for name, value := range map[string]string{
		"kubernetesVersion": target.KubernetesVersion,
		"clusterIpv4Cidr":   target.ClusterIPv4CIDR,
		"servicesIpv4Cidr":  target.ServicesIPv4CIDR,
	} {
		if strings.ContainsAny(value, "\r\n\x00") {
			problems = append(problems, fmt.Sprintf("target.gcp.%s must not contain control characters", name))
		}
	}
	if value := strings.TrimSpace(target.KubernetesVersion); value != "" && !gcpKubernetesMinor.MatchString(value) {
		problems = append(problems, "target.gcp.kubernetesVersion must be a Kubernetes minor such as 1.32")
	}
	if (target.MemorystoreShardCount != nil && *target.MemorystoreShardCount < 1) || (target.MemorystoreReplicas != nil && (*target.MemorystoreReplicas < 0 || *target.MemorystoreReplicas > 5)) || target.OpenSearchReplicas < 0 || target.QueueReplicas < 0 || target.DesiredWebReplicas < 0 || target.QueueConsumerCount < 0 {
		problems = append(problems, "target.gcp replica and count settings cannot be negative")
	}
	if value := strings.TrimSpace(target.MemorystoreMode); value != "" && value != "CLUSTER" && value != "CLUSTER_DISABLED" {
		problems = append(problems, "target.gcp.memorystoreMode must be CLUSTER or CLUSTER_DISABLED")
	}
	if value := strings.TrimSpace(target.MemorystoreZoneDistributionMode); value != "" && value != "MULTI_ZONE" && value != "SINGLE_ZONE" {
		problems = append(problems, "target.gcp.memorystoreZoneDistributionMode must be MULTI_ZONE or SINGLE_ZONE")
	}
	if target.MemorystoreMode == "CLUSTER_DISABLED" && target.MemorystoreShardCount != nil && *target.MemorystoreShardCount > 1 {
		problems = append(problems, "target.gcp.memorystoreShardCount must be 1 when memorystoreMode is CLUSTER_DISABLED")
	}
	if target.MemorystoreZoneDistributionMode == "SINGLE_ZONE" && strings.TrimSpace(target.MemorystoreZone) == "" {
		problems = append(problems, "target.gcp.memorystoreZone is required for SINGLE_ZONE distribution")
	}
	if target.StandardNodeCount < 0 || target.StandardNodeMinCount < 0 || target.StandardNodeMaxCount < 0 || target.StandardNodeDiskSizeGiB < 0 {
		problems = append(problems, "target.gcp Standard node settings cannot be negative")
	}
	if target.StandardNodeMaxCount > 0 && target.StandardNodeMinCount > target.StandardNodeMaxCount {
		problems = append(problems, "target.gcp standard node minimum cannot exceed maximum")
	}
	if target.StandardNodeCount > 0 && ((target.StandardNodeMinCount > 0 && target.StandardNodeCount < target.StandardNodeMinCount) || (target.StandardNodeMaxCount > 0 && target.StandardNodeCount > target.StandardNodeMaxCount)) {
		problems = append(problems, "target.gcp standard node count must be between minimum and maximum")
	}
	if runtime != "gke-standard" && (target.StandardNodeType != "" || target.StandardNodeCount > 0 || target.StandardNodeMinCount > 0 || target.StandardNodeMaxCount > 0 || target.StandardNodeDiskType != "" || target.StandardNodeDiskSizeGiB > 0 || target.StandardNodeImageType != "" || target.StandardNodeSpot) {
		problems = append(problems, "target.gcp Standard node settings require runtime gke-standard")
	}
	sort.Strings(problems)
	return problems
}

// Target is the provider-owned GCP target schema (moved from config).

var gcpKubernetesMinor = regexp.MustCompile(`^1\.[0-9]+$`)

func validClockTime(value string) bool {
	parsed, err := time.Parse("15:04", value)
	return err == nil && parsed.Format("15:04") == value
}
