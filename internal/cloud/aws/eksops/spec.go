package eksops

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/cloud/aws/network"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

var (
	stableName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	accountID  = regexp.MustCompile(`^[0-9]{12}$`)
	regionName = regexp.MustCompile(`^[a-z]{2}(?:-gov)?-[a-z]+-[0-9]+$`)
	digest     = regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`)
	kubeMinor  = regexp.MustCompile(`^1\.[0-9]+$`)
	nodeAMI    = regexp.MustCompile(`^ami-[A-Za-z0-9]+$`)
	kmsARN     = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):kms:[a-z0-9-]+:[0-9]{12}:key/[A-Za-z0-9-]+$`)
	secretARN  = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):secretsmanager:[a-z0-9-]+:[0-9]{12}:secret:[A-Za-z0-9/_+=.@!-]+$`)
)

const (
	NatModeGateway = "nat-gateway"
	NatModeFckNat  = "fck-nat"

	DatabaseEngineAuroraMySQL = "aurora-mysql"
	DatabaseEngineRDSMySQL    = "rds-mysql"
	DatabaseEngineRDSMariaDB  = "rds-mariadb"

	DefaultKubernetesVersion = "1.36"
	ComputeModeAuto          = "auto-mode"
	ComputeModeManagedNodes  = "managed-node-groups"
	ComputeModeSelfManaged   = "self-managed"
	ComputeModeFargate       = "fargate"
	SearchModeOpenSearch     = "opensearch"
	SearchModeDisabled       = "disabled"
	QueueModeDatabase        = "database"
	QueueModeRabbitMQ        = "rabbitmq"
)

// Spec is the validated plan for experimental aws/eks.
type Spec struct {
	Identity     Identity
	Application  Application
	Artifact     Artifact
	Lifecycle    Lifecycle
	Policy       NetworkPolicy
	Catalog      CatalogSelection
	Dependencies Dependencies
	// AllowExpiredPreview records that CLI planning already accepted an
	// expired preview for teardown. The Pulumi program reads it to skip
	// only the expiry check; deploy planning never sets it.
	AllowExpiredPreview bool
}

type Identity struct {
	Project          string
	Environment      string
	AccountID        string
	Region           string
	EnvironmentClass string
	Preset           sdk.PresetID
	Tags             map[string]string
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
	VPCCIDR            netip.Prefix
	AvailabilityZones  []string
	NatMode            string
	NatTopology        string
	NatReplacementMode string
	NatInstanceType    string
}

type CatalogSelection struct {
	DatabaseEngine                 string
	KubernetesVersion              string
	ComputeMode                    string
	NodeInstanceType               string
	NodeAMI                        string
	NodeMinSize                    int
	NodeDesiredSize                int
	NodeMaxSize                    int
	FargateNamespaces              []string
	SearchMode                     string
	SearchReplicas                 int
	QueueMode                      string
	QueueReplicas                  int
	LiveQueueReplicas              int
	AuroraMinACU                   float64
	AuroraMaxACU                   float64
	AuroraAutoPause                int
	AuroraAutoPauseOK              bool
	InstanceClass                  string
	InstanceCount                  int
	ValkeyNodeType                 string
	ValkeyReplicaCount             int
	CPURequest                     string
	MemoryRequest                  string
	DesiredWebReplicas             int
	QueueConsumerCount             int
	BackupDays                     int
	DatabaseBackupWindow           string
	DatabaseMaintenanceWindow      string
	DatabaseDeletionProtection     *bool
	DatabaseDeleteAutomatedBackups *bool
	CacheSnapshotRetentionLimit    *int
	CacheSnapshotWindow            string
	AuroraMySQLVersion             string
	MySQLVersion                   string
	MariaDBVersion                 string
	ValkeyVersion                  string
}

type Dependencies struct {
	KMSKeyARN        string
	CacheSecretARN   string
	SessionSecretARN string
	EncryptionKeyARN string
	DatabaseName     string
	MasterUsername   string
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
	if !stableName.MatchString(s.Identity.Project) {
		problems = append(problems, errors.New("project name must be a stable lowercase identifier"))
	}
	if !stableName.MatchString(s.Identity.Environment) {
		problems = append(problems, errors.New("environment name must be a stable lowercase identifier"))
	}
	if !accountID.MatchString(s.Identity.AccountID) {
		problems = append(problems, errors.New("account ID must be a 12-digit AWS account"))
	}
	if !regionName.MatchString(s.Identity.Region) {
		problems = append(problems, fmt.Errorf("invalid AWS region %q", s.Identity.Region))
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
	if !s.Policy.VPCCIDR.IsValid() {
		problems = append(problems, errors.New("VPC CIDR is required"))
	}
	if len(s.Policy.AvailabilityZones) < 2 {
		problems = append(problems, errors.New("at least two availability zones are required"))
	}
	if s.Policy.NatMode != NatModeGateway && s.Policy.NatMode != NatModeFckNat {
		problems = append(problems, errors.New("natMode must be nat-gateway or fck-nat"))
	}
	if s.Policy.NatTopology != "" && s.Policy.NatTopology != network.NatTopologySingleAZ && s.Policy.NatTopology != network.NatTopologyMultiAZ {
		problems = append(problems, errors.New("natTopology must be single-az or multi-az"))
	}
	natTopology := network.ResolveNatTopology(s.Policy.NatTopology, s.Identity.Preset)
	natReplacementMode := network.ResolveNatReplacementMode(s.Policy.NatReplacementMode, s.Identity.Preset, s.Policy.NatMode, natTopology)
	if natReplacementMode != network.NatReplacementNone && natReplacementMode != network.NatReplacementAutoScaling {
		problems = append(problems, errors.New("natReplacementMode must be none or auto-scaling"))
	}
	if s.Policy.NatMode == NatModeGateway && natReplacementMode != network.NatReplacementNone {
		problems = append(problems, errors.New("natReplacementMode is only supported with fck-nat"))
	}
	if s.Policy.NatMode == NatModeFckNat && s.Identity.Preset == sdk.PresetHighAvailability && (natTopology != network.NatTopologyMultiAZ || natReplacementMode != network.NatReplacementAutoScaling) {
		problems = append(problems, errors.New("high-availability fck-nat requires multi-az topology with auto-scaling replacement"))
	}
	if s.Catalog.DatabaseEngine != DatabaseEngineAuroraMySQL && s.Catalog.DatabaseEngine != DatabaseEngineRDSMySQL && s.Catalog.DatabaseEngine != DatabaseEngineRDSMariaDB {
		problems = append(problems, errors.New("databaseEngine must be aurora-mysql, rds-mysql, or rds-mariadb"))
	}
	if (s.Catalog.DatabaseEngine == DatabaseEngineRDSMySQL || s.Catalog.DatabaseEngine == DatabaseEngineRDSMariaDB) && s.Identity.Preset != sdk.PresetPreview {
		problems = append(problems, errors.New("RDS database engines are only supported for the preview preset"))
	}
	if version := strings.TrimSpace(s.Catalog.KubernetesVersion); version != "" {
		if err := validateKubernetesVersion(version); err != nil {
			problems = append(problems, err)
		}
	}
	computeMode := strings.TrimSpace(s.Catalog.ComputeMode)
	if computeMode == "" {
		computeMode = ComputeModeAuto
	}
	if computeMode != ComputeModeAuto && computeMode != ComputeModeManagedNodes && computeMode != ComputeModeSelfManaged && computeMode != ComputeModeFargate {
		problems = append(problems, fmt.Errorf("EKS computeMode %q is unsupported", computeMode))
	}
	if computeMode == ComputeModeSelfManaged && !nodeAMI.MatchString(strings.TrimSpace(s.Catalog.NodeAMI)) {
		problems = append(problems, errors.New("self-managed EKS nodes require a pinned ami-* node AMI"))
	}
	if computeMode != ComputeModeSelfManaged && strings.TrimSpace(s.Catalog.NodeAMI) != "" {
		problems = append(problems, errors.New("nodeAMI is only supported for self-managed EKS nodes"))
	}
	if (computeMode == ComputeModeAuto || computeMode == ComputeModeFargate) && (strings.TrimSpace(s.Catalog.NodeInstanceType) != "" || s.Catalog.NodeMinSize > 0 || s.Catalog.NodeDesiredSize > 0 || s.Catalog.NodeMaxSize > 0) {
		problems = append(problems, errors.New("EKS node capacity settings require managed-node-groups or self-managed mode"))
	}
	if computeMode != ComputeModeFargate && len(s.Catalog.FargateNamespaces) > 0 {
		problems = append(problems, errors.New("Fargate namespaces require fargate compute mode"))
	}
	if s.Catalog.NodeMinSize < 0 || s.Catalog.NodeDesiredSize < 0 || s.Catalog.NodeMaxSize < 0 || (s.Catalog.NodeMaxSize > 0 && s.Catalog.NodeMinSize > s.Catalog.NodeMaxSize) || (s.Catalog.NodeDesiredSize > 0 && s.Catalog.NodeDesiredSize < s.Catalog.NodeMinSize) || (s.Catalog.NodeMaxSize > 0 && s.Catalog.NodeDesiredSize > s.Catalog.NodeMaxSize) {
		problems = append(problems, errors.New("EKS node capacity bounds are invalid"))
	}
	if computeMode == ComputeModeFargate && (s.Catalog.SearchMode == SearchModeOpenSearch || s.Catalog.QueueMode == QueueModeRabbitMQ) {
		problems = append(problems, errors.New("EKS Fargate cannot host the persistent OpenSearch or RabbitMQ workloads; select database queue and disabled search"))
	}
	if mode := strings.TrimSpace(s.Catalog.SearchMode); mode != "" && mode != SearchModeOpenSearch && mode != SearchModeDisabled {
		problems = append(problems, errors.New("EKS searchMode must be opensearch or disabled"))
	}
	if s.Catalog.SearchMode == SearchModeOpenSearch && s.Catalog.SearchReplicas < 1 {
		problems = append(problems, errors.New("EKS OpenSearch requires at least one replica"))
	}
	if s.Catalog.SearchMode == SearchModeDisabled && s.Catalog.SearchReplicas != 0 {
		problems = append(problems, errors.New("EKS disabled search must not set search replicas"))
	}
	if mode := strings.TrimSpace(s.Catalog.QueueMode); mode != "" && mode != QueueModeDatabase && mode != QueueModeRabbitMQ {
		problems = append(problems, errors.New("EKS queueMode must be database or rabbitmq"))
	}
	if s.Catalog.QueueMode == QueueModeRabbitMQ && s.Catalog.QueueReplicas < 1 {
		problems = append(problems, errors.New("EKS RabbitMQ requires at least one replica"))
	}
	if s.Catalog.QueueMode == QueueModeDatabase && s.Catalog.QueueReplicas != 0 {
		problems = append(problems, errors.New("EKS database queue must not set queue replicas"))
	}
	if s.Catalog.QueueMode == QueueModeRabbitMQ {
		if err := platform.RefuseInPlaceRabbitMQTopologyChange(s.Catalog.LiveQueueReplicas, s.Catalog.QueueReplicas); err != nil {
			problems = append(problems, err)
		}
	}
	if strings.TrimSpace(s.Catalog.ValkeyNodeType) == "" {
		problems = append(problems, errors.New("Valkey node type is required"))
	}
	if s.Catalog.DesiredWebReplicas < 1 {
		problems = append(problems, errors.New("desired web replicas must be at least 1"))
	}
	if strings.TrimSpace(s.Catalog.CPURequest) == "" || strings.TrimSpace(s.Catalog.MemoryRequest) == "" {
		problems = append(problems, errors.New("EKS CPU and memory requests are required"))
	}
	if !kmsARN.MatchString(s.Dependencies.KMSKeyARN) {
		problems = append(problems, errors.New("KMS key ARN is required"))
	}
	if s.Dependencies.DatabaseName == "" || s.Dependencies.MasterUsername == "" {
		problems = append(problems, errors.New("database name and master username are required"))
	}
	if strings.TrimSpace(s.Dependencies.CacheSecretARN) == "" {
		problems = append(problems, errors.New("cache secret ARN is required"))
	}
	if !secretARN.MatchString(s.Dependencies.EncryptionKeyARN) {
		problems = append(problems, errors.New("Magento encryption key Secrets Manager ARN is required"))
	}
	if s.Identity.Preset != sdk.PresetPreview && strings.TrimSpace(s.Dependencies.SessionSecretARN) == "" {
		problems = append(problems, errors.New("session secret ARN is required outside preview"))
	}
	if !allowExpiredPreview && !s.Lifecycle.ExpiresAt.IsZero() && time.Now().After(s.Lifecycle.ExpiresAt) {
		problems = append(problems, errors.New("preview environment has expired"))
	}
	return errors.Join(problems...)
}

func validateKubernetesVersion(version string) error {
	if !kubeMinor.MatchString(version) {
		return fmt.Errorf("EKS Kubernetes version %q must use the 1.<minor> format", version)
	}
	minor, err := strconv.Atoi(strings.TrimPrefix(version, "1."))
	if err != nil || minor < 29 {
		return fmt.Errorf("EKS Auto Mode requires Kubernetes 1.29 or newer, got %q", version)
	}
	return nil
}
