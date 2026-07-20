package eksops

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	sdk "github.com/acourtiol/magelift/sdk/v1"
)

var (
	stableName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	accountID  = regexp.MustCompile(`^[0-9]{12}$`)
	regionName = regexp.MustCompile(`^[a-z]{2}(?:-gov)?-[a-z]+-[0-9]+$`)
	digest     = regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`)
	kmsARN     = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):kms:[a-z0-9-]+:[0-9]{12}:key/[A-Za-z0-9-]+$`)
)

const (
	NatModeGateway = "nat-gateway"
	NatModeFckNat  = "fck-nat"

	DatabaseEngineAuroraMySQL = "aurora-mysql"
	DatabaseEngineRDSMySQL    = "rds-mysql"
)

// Spec is the validated plan for experimental aws/eks-autopilot.
type Spec struct {
	Identity     Identity
	Application  Application
	Artifact     Artifact
	Lifecycle    Lifecycle
	Policy       NetworkPolicy
	Catalog      CatalogSelection
	Dependencies Dependencies
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
}

type Artifact struct {
	ImageDigest string
}

type Lifecycle struct {
	ExpiresAt  time.Time
	Protection bool
}

type NetworkPolicy struct {
	VPCCIDR           netip.Prefix
	AvailabilityZones []string
	NatMode           string
}

type CatalogSelection struct {
	DatabaseEngine     string
	AuroraMinACU       float64
	AuroraMaxACU       float64
	AuroraAutoPause    int
	AuroraAutoPauseOK  bool
	InstanceClass      string
	InstanceCount      int
	ValkeyNodeType     string
	ValkeyReplicaCount int
	CPURequest         string
	MemoryRequest      string
	DesiredWebReplicas int
	QueueConsumerCount int
	BackupDays         int
	AuroraMySQLVersion string
	MySQLVersion       string
	ValkeyVersion      string
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
	if s.Catalog.DatabaseEngine != DatabaseEngineAuroraMySQL && s.Catalog.DatabaseEngine != DatabaseEngineRDSMySQL {
		problems = append(problems, errors.New("databaseEngine must be aurora-mysql or rds-mysql"))
	}
	if s.Catalog.DatabaseEngine == DatabaseEngineRDSMySQL && s.Identity.Preset != sdk.PresetPreview {
		problems = append(problems, errors.New("rds-mysql is only supported for the preview preset"))
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
	if s.Identity.Preset != sdk.PresetPreview && strings.TrimSpace(s.Dependencies.SessionSecretARN) == "" {
		problems = append(problems, errors.New("session secret ARN is required outside preview"))
	}
	if !s.Lifecycle.ExpiresAt.IsZero() && time.Now().After(s.Lifecycle.ExpiresAt) {
		problems = append(problems, errors.New("preview environment has expired"))
	}
	return errors.Join(problems...)
}
