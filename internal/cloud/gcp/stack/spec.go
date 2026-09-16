package stack

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/cloud/gcp/database"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

var (
	stableName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	digest     = regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`)
	gcpRegion  = regexp.MustCompile(`^[a-z]+-[a-z]+[0-9]+$`)
)

type Spec struct {
	Identity      Identity
	Application   Application
	Artifact      Artifact
	Lifecycle     Lifecycle
	Policy        NetworkPolicy
	Catalog       CatalogSelection
	Dependencies  Dependencies
	Edge          sdk.EdgeIntent
	Observability sdk.ObservabilityIntent
	// AllowExpiredPreview records that CLI planning already accepted an
	// expired preview for teardown. The Pulumi program reads it to skip
	// only the expiry check; deploy planning never sets it.
	AllowExpiredPreview bool
}

type Identity struct {
	Project          string // Magento project name
	GCPProject       string
	Environment      string
	Region           string
	Runtime          sdk.RuntimeID
	EnvironmentClass string
	Preset           sdk.PresetID
	Labels           map[string]string
}

type Application struct {
	Edition    string
	Version    string
	Mode       string
	WebRuntime string
	Magento    platform.MagentoOverlays
}

type Artifact struct {
	ImageDigest string
}

type Lifecycle struct {
	ExpiresAt  time.Time
	Protection bool
}

type NetworkPolicy struct {
	NetworkCIDR       string
	Zones             []string
	ApplicationDomain string
}

type CatalogSelection struct {
	CloudSQLTier                    string
	CloudSQLAvailability            string // ZONAL | REGIONAL
	CloudSQLDatabaseVersion         string // MYSQL_8_0 | MYSQL_8_4; derived from Magento release
	CloudSQLBackupEnabled           bool
	CloudSQLBinaryLogEnabled        bool
	CloudSQLBackupRetentionCount    int
	CloudSQLTransactionLogRetention int
	CloudSQLBackupStartTime         string
	CloudSQLBackupLocation          string
	CloudSQLDeletionProtection      bool
	MemorystoreNodeType             string
	MemorystoreShardCount           int
	MemorystoreEngineVersion        string // VALKEY_8_0 | VALKEY_9_0 | VALKEY_9_1; derived from Magento release
	MemorystoreReplicas             int
	MemorystoreMode                 string // CLUSTER | CLUSTER_DISABLED
	MemorystoreZoneDistributionMode string // MULTI_ZONE | SINGLE_ZONE
	MemorystoreZone                 string
	MemorystoreDeletionProtection   bool
	MemorystorePSCConnectionLimit   int
	KubernetesVersion               string
	ReleaseChannel                  string
	ClusterIPv4CIDR                 string
	ServicesIPv4CIDR                string
	StandardNodeType                string
	StandardNodeCount               int
	StandardNodeMinCount            int
	StandardNodeMaxCount            int
	StandardNodeDiskType            string
	StandardNodeDiskSizeGiB         int
	StandardNodeImageType           string
	StandardNodeSpot                bool
	OpenSearchImage                 string
	RabbitMQImage                   string
	AutopilotCPURequest             string
	AutopilotMemoryRequest          string
	DesiredWebReplicas              int
	QueueConsumerCount              int
	SearchMode                      string // opensearch | disabled
	SearchReplicas                  int
	QueueMode                       string // database | rabbitmq
	QueueReplicas                   int
	LiveQueueReplicas               int
	EnableCloudArmor                bool
}

type Dependencies struct {
	DatabaseName        string
	MasterUsername      string
	EncryptionKeySecret string
}

// CloudSQLDatabaseVersion returns the release-aware Cloud SQL version. A
// manually constructed Spec may omit the catalog field, so derive the same
// value used by PlanFromConfig instead of falling back to an old default.
func (s Spec) CloudSQLDatabaseVersion() (string, error) {
	expected, err := database.DatabaseVersionForMagento(s.Application.Version)
	if err != nil {
		return "", err
	}
	configured := strings.TrimSpace(s.Catalog.CloudSQLDatabaseVersion)
	if configured != "" && configured != expected {
		return "", fmt.Errorf("Cloud SQL database version %q does not match Magento %s; want %q", configured, s.Application.Version, expected)
	}
	return expected, nil
}

// MemorystoreEngineVersion returns the provider API value for the selected
// Magento release. GCP's current Valkey 9.0 GA service is the default
// compatible target for Adobe Commerce 2.4.9. An explicit VALKEY_9_1 request
// is also accepted as a Preview profile; certification must keep that
// lifecycle status separate from the GA 9.0 record. Older release rows remain
// mapped to VALKEY_8_0 only for older compatibility rows whose Adobe major is
// still 8. The current 2.4.9 profile must use Valkey 9; a 9.1 selection is
// explicitly preview and therefore cannot inherit the GA certification status.
func (s Spec) MemorystoreEngineVersion() (string, error) {
	return memorystoreAPIEngineVersionForMagento(s.Application.Version, s.Catalog.MemorystoreEngineVersion)
}

func memorystoreAPIEngineVersionForMagento(version, configured string) (string, error) {
	requirement, err := config.ServiceRequirementForRelease(version, config.CompatibilityCache, "valkey")
	if err != nil {
		return "", err
	}
	if len(requirement.Versions) == 0 {
		return "", fmt.Errorf("Adobe Valkey requirement for Magento %s has no service major", version)
	}
	// This is the provider adapter's API translation. The core requirement is
	// Valkey 9 for current Adobe releases; GCP's GA enum is VALKEY_9_0 and its
	// Preview enum is VALKEY_9_1. Older rows remain available only where their
	// Adobe/provider compatibility record explicitly maps them to VALKEY_8_0.
	expected := ""
	if strings.HasPrefix(requirement.Versions[0], "9") {
		expected = "VALKEY_9_0"
	} else if strings.HasPrefix(requirement.Versions[0], "8") {
		expected = "VALKEY_8_0"
	} else {
		return "", fmt.Errorf("GCP Memorystore has no API mapping for Adobe Valkey %q", requirement.Versions[0])
	}
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return expected, nil
	}
	if configured == expected || (expected == "VALKEY_9_0" && configured == "VALKEY_9_1") {
		return configured, nil
	}
	return "", fmt.Errorf("Memorystore engine version %q does not match Magento %s; want %q or the documented Preview value VALKEY_9_1", configured, version, expected)
}

func (s Spec) Validate() error {
	return s.validate(false)
}

// ValidateAllowExpiredPreview is restricted to teardown planning. It keeps
// every structural and security check while allowing an already expired
// preview to be converted into a destroy request.
func (s Spec) ValidateAllowExpiredPreview() error {
	return s.validate(true)
}

func (s Spec) validate(allowExpiredPreview bool) error {
	var problems []error
	databaseVersion, err := s.CloudSQLDatabaseVersion()
	if err != nil {
		problems = append(problems, fmt.Errorf("Cloud SQL database version: %w", err))
	} else if err := database.ValidateTier(databaseVersion, s.Catalog.CloudSQLTier); err != nil {
		problems = append(problems, err)
	}
	if _, err := s.MemorystoreEngineVersion(); err != nil {
		problems = append(problems, err)
	}
	if s.Catalog.MemorystoreShardCount < 0 {
		problems = append(problems, errors.New("Memorystore shard count cannot be negative"))
	}
	memorystoreMode := strings.TrimSpace(s.Catalog.MemorystoreMode)
	if memorystoreMode == "" {
		memorystoreMode = "CLUSTER_DISABLED"
	}
	if memorystoreMode != "CLUSTER" && memorystoreMode != "CLUSTER_DISABLED" {
		problems = append(problems, fmt.Errorf("invalid Memorystore mode %q", memorystoreMode))
	}
	if s.Catalog.MemorystoreReplicas < 0 || s.Catalog.MemorystoreReplicas > 5 {
		problems = append(problems, errors.New("Memorystore replica count must be between 0 and 5"))
	}
	if memorystoreMode == "CLUSTER_DISABLED" && s.Catalog.MemorystoreShardCount > 1 {
		problems = append(problems, errors.New("Cluster Mode Disabled Memorystore instances support only one shard"))
	}
	zoneDistributionMode := strings.TrimSpace(s.Catalog.MemorystoreZoneDistributionMode)
	if zoneDistributionMode != "" && zoneDistributionMode != "MULTI_ZONE" && zoneDistributionMode != "SINGLE_ZONE" {
		problems = append(problems, fmt.Errorf("invalid Memorystore zone distribution mode %q", zoneDistributionMode))
	}
	if zoneDistributionMode == "SINGLE_ZONE" && strings.TrimSpace(s.Catalog.MemorystoreZone) == "" {
		problems = append(problems, errors.New("single-zone Memorystore placement requires a zone"))
	}
	runtimeID := s.Identity.Runtime
	if runtimeID == "" {
		runtimeID = "gke-autopilot"
	}
	if runtimeID != "gke-autopilot" && runtimeID != "gke-standard" {
		problems = append(problems, fmt.Errorf("invalid GCP runtime %q", runtimeID))
	}
	if !stableName.MatchString(s.Identity.Project) {
		problems = append(problems, errors.New("project name must be a stable lowercase identifier"))
	}
	if strings.TrimSpace(s.Identity.GCPProject) == "" {
		problems = append(problems, errors.New("GCP project ID is required"))
	}
	if !stableName.MatchString(s.Identity.Environment) {
		problems = append(problems, errors.New("environment name must be a stable lowercase identifier"))
	}
	if !gcpRegion.MatchString(s.Identity.Region) {
		problems = append(problems, fmt.Errorf("invalid GCP region %q", s.Identity.Region))
	}
	if s.Identity.EnvironmentClass == "" {
		problems = append(problems, errors.New("environment class is required"))
	}
	if s.Identity.Preset != sdk.PresetPreview && s.Identity.Preset != sdk.PresetStandard && s.Identity.Preset != sdk.PresetHighAvailability {
		problems = append(problems, fmt.Errorf("invalid preset %q", s.Identity.Preset))
	}
	if !digest.MatchString(s.Artifact.ImageDigest) {
		problems = append(problems, errors.New("artifact image digest must be repository@sha256:..."))
	}
	if _, err := netip.ParsePrefix(s.Policy.NetworkCIDR); err != nil {
		problems = append(problems, fmt.Errorf("network CIDR: %w", err))
	}
	if len(s.Policy.Zones) == 0 {
		problems = append(problems, errors.New("at least one zone is required"))
	}
	if s.Dependencies.DatabaseName == "" || s.Dependencies.MasterUsername == "" {
		problems = append(problems, errors.New("database name and master username are required"))
	}
	if strings.TrimSpace(s.Dependencies.EncryptionKeySecret) == "" {
		problems = append(problems, errors.New("Magento encryption key Secret Manager secret ID is required"))
	}
	if err := sdk.ValidateObservabilityIntent(s.Observability); err != nil {
		problems = append(problems, fmt.Errorf("observability intent: %w", err))
	}
	if err := sdk.ValidateEdgeIntent(s.Edge); err != nil {
		problems = append(problems, fmt.Errorf("edge intent: %w", err))
	}
	if edgeMode := strings.TrimSpace(s.Edge.Mode); edgeMode == "native" || edgeMode == "both" {
		if strings.TrimSpace(s.Policy.ApplicationDomain) == "" {
			problems = append(problems, errors.New("GCP native edge requires an application domain"))
		}
	}
	if s.Catalog.DesiredWebReplicas < 1 {
		problems = append(problems, errors.New("desired web replicas must be at least 1"))
	}
	if s.Catalog.SearchMode != "opensearch" && s.Catalog.SearchMode != "disabled" {
		problems = append(problems, fmt.Errorf("invalid GCP search mode %q", s.Catalog.SearchMode))
	}
	if s.Catalog.QueueMode != "database" && s.Catalog.QueueMode != "rabbitmq" {
		problems = append(problems, fmt.Errorf("invalid GCP queue mode %q", s.Catalog.QueueMode))
	}
	if s.Catalog.QueueMode == "rabbitmq" {
		if err := platform.RefuseInPlaceRabbitMQTopologyChange(s.Catalog.LiveQueueReplicas, s.Catalog.QueueReplicas); err != nil {
			problems = append(problems, err)
		}
	}
	if !allowExpiredPreview && !s.Lifecycle.ExpiresAt.IsZero() && time.Now().After(s.Lifecycle.ExpiresAt) {
		problems = append(problems, errors.New("preview environment has expired"))
	}
	return errors.Join(problems...)
}
