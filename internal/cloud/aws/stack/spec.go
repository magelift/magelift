// Package stack defines the validated input contract for an AWS target stack.
package stack

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
	stableName          = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	accountID           = regexp.MustCompile(`^[0-9]{12}$`)
	regionName          = regexp.MustCompile(`^[a-z]{2}(?:-gov)?-[a-z]+-[0-9]+$`)
	digest              = regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`)
	version             = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)
	certificate         = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):acm:us-east-1:[0-9]{12}:certificate/[A-Za-z0-9-]+$`)
	domain              = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
	regionalCertificate = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):acm:[a-z0-9-]+:[0-9]{12}:certificate/[A-Za-z0-9-]+$`)
	auroraVersion       = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:\.mysql_aurora\.[0-9.]+)?$`)
	openSearchVersion   = regexp.MustCompile(`^OpenSearch_[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)
	subnetID            = regexp.MustCompile(`^subnet-[A-Za-z0-9-]+$`)
	vpcID               = regexp.MustCompile(`^vpc-[A-Za-z0-9-]+$`)
	retentionSet        = map[int]bool{1: true, 3: true, 5: true, 7: true, 14: true, 30: true, 60: true, 90: true, 120: true, 150: true, 180: true, 365: true, 400: true, 545: true, 731: true, 1096: true, 1827: true, 2192: true, 2557: true, 2922: true, 3288: true, 3653: true}
)

type Spec struct {
	Identity     Identity
	Application  Application
	Artifact     Artifact
	Lifecycle    Lifecycle
	Existing     ExistingResources
	Dependencies Dependencies
	Policy       NetworkPolicy
	Catalog      CatalogSelection
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
	ImageDigest                 string
	RequiredRuntimeCapabilities []sdk.CapabilityID
	CompatibilityStatus         string
}

type Lifecycle struct {
	ExpiresAt          time.Time
	MonthlyBudgetCents int64
	Protection         bool
}

type ExistingResources struct {
	Network          *sdk.ExistingResourceRef
	PublicSubnetIDs  []string
	PrivateSubnetIDs []string
	DataSubnetIDs    []string
	HostedZone       *sdk.ExistingResourceRef
	Certificate      *sdk.ExistingResourceRef
	ALBCertificate   *sdk.ExistingResourceRef
	SNSTopicARN      string
}

type Dependencies struct {
	KMSKeyARN        string
	CacheSecretARN   string
	SessionSecretARN string
	QueueSecretARN   string
	EncryptionKeyARN string
	DatabaseName     string
	MasterUsername   string
}

type NetworkPolicy struct {
	VPCCIDR           netip.Prefix
	AvailabilityZones []string
	MediaDomain       string
	ApplicationDomain string
}

type CatalogSelection struct {
	Version           string
	Aurora            AuroraPreviewProfile
	Valkey            ValkeyPreviewProfile
	Search            SearchPreviewProfile
	Fargate           FargatePreviewProfile
	Retention         RetentionProfile
	Versions          ServiceVersions
	AuroraProvisioned AuroraProvisionedProfile
	SearchProvisioned SearchProvisionedProfile
	RabbitMQ          RabbitMQProfile
}

type AuroraPreviewProfile struct {
	MinimumACU              float64
	MaximumACU              float64
	AutoPauseSeconds        int
	EngineSupportsAutoPause bool
}

type ValkeyPreviewProfile struct {
	NodeType     string
	ReplicaCount int
}

type SearchPreviewProfile struct {
	MaximumIndexingOCU float64
	MaximumSearchOCU   float64
	AcceptColdStarts   bool
}

type FargatePreviewProfile struct {
	CPU          int
	MemoryMiB    int
	DesiredCount int
}

type AuroraProvisionedProfile struct {
	InstanceClass string
	InstanceCount int
}

type SearchProvisionedProfile struct {
	InstanceType         string
	InstanceCount        int
	DedicatedMasterType  string
	DedicatedMasterCount int
	EBSVolumeType        string
	EBSVolumeSizeGiB     int
}

type RabbitMQProfile struct {
	InstanceType string
}

type RetentionProfile struct {
	LogDays      int
	BackupDays   int
	ArtifactDays int
}

type ServiceVersions struct {
	AuroraMySQL string
	Valkey      string
	OpenSearch  string
	RabbitMQ    string
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
	problems = append(problems, s.Identity.validate(), s.Application.validate(), s.Artifact.validate(s.Identity.Preset), s.Lifecycle.validate(s.Identity.Preset, allowExpiredPreview), s.Existing.validate(), s.Dependencies.validate(s.Identity.Preset), s.Policy.validate(s.Identity.Preset), s.Catalog.validate(s.Identity.Preset))
	if s.Existing.Network != nil {
		for _, group := range []struct {
			name   string
			values []string
		}{
			{name: "public", values: s.Existing.PublicSubnetIDs},
			{name: "private", values: s.Existing.PrivateSubnetIDs},
			{name: "data", values: s.Existing.DataSubnetIDs},
		} {
			if len(group.values) != len(s.Policy.AvailabilityZones) {
				problems = append(problems, fmt.Errorf("existing %s subnet count must match availability zones", group.name))
			}
		}
	}
	return errors.Join(problems...)
}

func (i Identity) validate() error {
	var problems []error
	if !stableName.MatchString(i.Project) || !stableName.MatchString(i.Environment) {
		problems = append(problems, errors.New("stack project and environment must be lowercase stable names"))
	}
	if !accountID.MatchString(i.AccountID) {
		problems = append(problems, errors.New("stack account ID must contain 12 digits"))
	}
	if !regionName.MatchString(i.Region) {
		problems = append(problems, errors.New("stack region is invalid"))
	}
	if i.EnvironmentClass != "preview" && i.EnvironmentClass != "production" && i.EnvironmentClass != "staging" {
		problems = append(problems, fmt.Errorf("unsupported environment class %q", i.EnvironmentClass))
	}
	if i.Preset != sdk.PresetPreview && i.Preset != sdk.PresetStandard && i.Preset != sdk.PresetHighAvailability {
		problems = append(problems, fmt.Errorf("unsupported stack preset %q", i.Preset))
	}
	return errors.Join(problems...)
}

func (a Application) validate() error {
	var problems []error
	if a.Edition != "open-source" && a.Edition != "commerce" {
		problems = append(problems, fmt.Errorf("unsupported Magento edition %q", a.Edition))
	}
	if strings.TrimSpace(a.Version) == "" {
		problems = append(problems, errors.New("Magento version is required"))
	}
	if a.Mode != "integrated" && a.Mode != "headless" {
		problems = append(problems, fmt.Errorf("unsupported application mode %q", a.Mode))
	}
	if a.WebRuntime != "nginx-fpm" && a.WebRuntime != "frankenphp-classic" {
		problems = append(problems, fmt.Errorf("unsupported web runtime %q", a.WebRuntime))
	}
	return errors.Join(problems...)
}

func (a Artifact) validate(preset sdk.PresetID) error {
	var problems []error
	if !digest.MatchString(a.ImageDigest) {
		problems = append(problems, errors.New("artifact image must be a registry digest"))
	}
	if a.CompatibilityStatus != "compatible" && a.CompatibilityStatus != "unsupported-allowed" {
		problems = append(problems, fmt.Errorf("unsupported compatibility status %q", a.CompatibilityStatus))
	}
	problems = append(problems, sdk.ValidateCapabilityRequirements(a.RequiredRuntimeCapabilities, awsCapabilities(preset)))
	return errors.Join(problems...)
}

func awsCapabilities(preset sdk.PresetID) []sdk.CapabilityID {
	capabilities := []sdk.CapabilityID{
		sdk.CapabilityDatabaseMySQL,
		sdk.CapabilityCacheValkey,
		sdk.CapabilitySearchOpenSearch,
		sdk.CapabilityObjectStorageS3,
		sdk.CapabilityEdgeCloudFront,
		sdk.CapabilityObservabilityCloudWatch,
	}
	if preset == sdk.PresetPreview {
		return append(capabilities, sdk.CapabilityQueueDatabase)
	}
	return append(capabilities, sdk.CapabilityQueueRabbitMQ)
}

func (l Lifecycle) validate(preset sdk.PresetID, allowExpiredPreview bool) error {
	var problems []error
	if preset == sdk.PresetPreview && l.ExpiresAt.IsZero() {
		problems = append(problems, errors.New("preview stacks require an expiration time"))
	}
	if preset == sdk.PresetPreview && !allowExpiredPreview && l.ExpiresAt.Before(time.Now().UTC()) {
		problems = append(problems, errors.New("preview stack expiration must be in the future"))
	}
	if l.MonthlyBudgetCents <= 0 {
		problems = append(problems, errors.New("stack monthly budget must be positive and expressed in cents"))
	}
	return errors.Join(problems...)
}

func (e ExistingResources) validate() error {
	var problems []error
	if e.Network != nil {
		if e.Network.Provider != "aws" || e.Network.Kind != sdk.ExistingNetwork {
			problems = append(problems, errors.New("existing network must be an AWS network reference"))
		} else if err := sdk.ValidateExistingResourceRef(*e.Network); err != nil {
			problems = append(problems, err)
		}
		if !vpcID.MatchString(e.Network.ExternalID) {
			problems = append(problems, errors.New("existing network external ID must be a VPC ID"))
		}
		problems = append(problems, validateSubnetIDs("existing public subnet", e.PublicSubnetIDs), validateSubnetIDs("existing private subnet", e.PrivateSubnetIDs), validateSubnetIDs("existing data subnet", e.DataSubnetIDs))
		if len(e.PublicSubnetIDs) == 0 || len(e.PrivateSubnetIDs) == 0 || len(e.DataSubnetIDs) == 0 {
			problems = append(problems, errors.New("existing network requires public, private, and data subnet IDs"))
		}
	} else if len(e.PublicSubnetIDs) != 0 || len(e.PrivateSubnetIDs) != 0 || len(e.DataSubnetIDs) != 0 {
		problems = append(problems, errors.New("existing subnet IDs require an existing network reference"))
	}
	if e.HostedZone == nil || e.HostedZone.Provider != "aws" || e.HostedZone.Kind != sdk.ExistingDNSZone {
		problems = append(problems, errors.New("stack requires an explicit AWS hosted-zone reference"))
	} else if err := sdk.ValidateExistingResourceRef(*e.HostedZone); err != nil {
		problems = append(problems, err)
	}
	if e.Certificate == nil || e.Certificate.Provider != "aws" || e.Certificate.Kind != sdk.ExistingCertificate || !certificate.MatchString(e.Certificate.ExternalID) {
		problems = append(problems, errors.New("stack requires an explicit ACM certificate ARN"))
	} else if err := sdk.ValidateExistingResourceRef(*e.Certificate); err != nil {
		problems = append(problems, err)
	}
	if e.ALBCertificate == nil || e.ALBCertificate.Provider != "aws" || e.ALBCertificate.Kind != sdk.ExistingCertificate || !regionalCertificate.MatchString(e.ALBCertificate.ExternalID) {
		problems = append(problems, errors.New("stack requires an explicit regional ACM certificate ARN for the application load balancer"))
	} else if err := sdk.ValidateExistingResourceRef(*e.ALBCertificate); err != nil {
		problems = append(problems, err)
	}
	if e.SNSTopicARN != "" && !strings.HasPrefix(e.SNSTopicARN, "arn:") {
		problems = append(problems, errors.New("stack notification topic must be an ARN"))
	}
	return errors.Join(problems...)
}

func validateSubnetIDs(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !subnetID.MatchString(value) {
			return fmt.Errorf("%s ID %q is invalid", name, value)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s IDs must be unique", name)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func (d Dependencies) validate(preset sdk.PresetID) error {
	var problems []error
	if !kmsARN.MatchString(d.KMSKeyARN) {
		problems = append(problems, errors.New("stack requires a customer-managed KMS key ARN"))
	}
	if !secretARN.MatchString(d.CacheSecretARN) {
		problems = append(problems, errors.New("stack requires a cache token Secrets Manager ARN"))
	}
	if !secretARN.MatchString(d.EncryptionKeyARN) {
		problems = append(problems, errors.New("stack requires a Magento encryption key Secrets Manager ARN"))
	}
	if preset != sdk.PresetPreview && !secretARN.MatchString(d.SessionSecretARN) {
		problems = append(problems, errors.New("non-preview stacks require a session token Secrets Manager ARN"))
	}
	if preset != sdk.PresetPreview && !secretARN.MatchString(d.QueueSecretARN) {
		problems = append(problems, errors.New("non-preview stacks require a RabbitMQ password Secrets Manager ARN"))
	}
	if !databaseName.MatchString(d.DatabaseName) || !username.MatchString(d.MasterUsername) {
		problems = append(problems, errors.New("stack database name and master username are invalid"))
	}
	return errors.Join(problems...)
}

func (p NetworkPolicy) validate(preset sdk.PresetID) error {
	var problems []error
	if !p.VPCCIDR.IsValid() || !p.VPCCIDR.Addr().Is4() || p.VPCCIDR != p.VPCCIDR.Masked() || p.VPCCIDR.Bits() > 24 {
		problems = append(problems, errors.New("stack VPC CIDR must be a canonical IPv4 prefix of /24 or larger"))
	}
	wantZones := map[sdk.PresetID]int{sdk.PresetPreview: 2, sdk.PresetStandard: 2, sdk.PresetHighAvailability: 3}[preset]
	if preset == sdk.PresetStandard && len(p.AvailabilityZones) == 3 {
		wantZones = 3
	}
	if len(p.AvailabilityZones) != wantZones {
		problems = append(problems, fmt.Errorf("preset %q requires %d availability zones", preset, wantZones))
	}
	seen := map[string]struct{}{}
	for _, zone := range p.AvailabilityZones {
		if strings.TrimSpace(zone) == "" {
			problems = append(problems, errors.New("availability zones cannot be empty"))
		}
		if _, found := seen[zone]; found {
			problems = append(problems, fmt.Errorf("availability zone %q is duplicated", zone))
		}
		seen[zone] = struct{}{}
	}
	if !domain.MatchString(p.ApplicationDomain) || !domain.MatchString(p.MediaDomain) {
		problems = append(problems, errors.New("application and media domains are required"))
	}
	return errors.Join(problems...)
}

func (c CatalogSelection) validate(preset sdk.PresetID) error {
	var problems []error
	if strings.TrimSpace(c.Version) == "" {
		problems = append(problems, errors.New("benchmark catalog version is required"))
	}
	if strings.TrimSpace(c.Valkey.NodeType) == "" || c.Valkey.ReplicaCount < 0 {
		problems = append(problems, errors.New("Valkey capacity is incomplete"))
	}
	if c.Fargate.CPU <= 0 || c.Fargate.MemoryMiB <= 0 || c.Fargate.DesiredCount <= 0 {
		problems = append(problems, errors.New("Fargate capacity is incomplete"))
	}
	minimumTasks := map[sdk.PresetID]int{sdk.PresetPreview: 1, sdk.PresetStandard: 2, sdk.PresetHighAvailability: 3}[preset]
	if c.Fargate.DesiredCount < minimumTasks {
		problems = append(problems, fmt.Errorf("preset %q requires at least %d web tasks", preset, minimumTasks))
	}
	if !retentionSet[c.Retention.LogDays] || c.Retention.BackupDays < 1 || c.Retention.ArtifactDays < 1 {
		problems = append(problems, errors.New("retention profile is invalid"))
	}
	if !auroraVersion.MatchString(c.Versions.AuroraMySQL) || !version.MatchString(c.Versions.Valkey) || !openSearchVersion.MatchString(c.Versions.OpenSearch) || !version.MatchString(c.Versions.RabbitMQ) {
		problems = append(problems, errors.New("all managed service versions must use their explicit AWS version format"))
	}
	if preset == sdk.PresetPreview {
		if c.Aurora.MinimumACU < 0 || c.Aurora.MaximumACU <= 0 || c.Aurora.MaximumACU < c.Aurora.MinimumACU || !halfStep(c.Aurora.MinimumACU) || !halfStep(c.Aurora.MaximumACU) {
			problems = append(problems, errors.New("Aurora preview capacity bounds are invalid"))
		}
		if c.Search.MaximumIndexingOCU <= 0 || c.Search.MaximumSearchOCU <= 0 || !c.Search.AcceptColdStarts {
			problems = append(problems, errors.New("OpenSearch preview capacity must be bounded and accept cold starts explicitly"))
		}
		if c.Valkey.ReplicaCount > 1 {
			problems = append(problems, errors.New("preview Valkey catalog profile cannot exceed one replica"))
		}
	} else {
		minimumInstances := 2
		if preset == sdk.PresetHighAvailability {
			minimumInstances = 3
		}
		if strings.TrimSpace(c.AuroraProvisioned.InstanceClass) == "" || c.AuroraProvisioned.InstanceCount < minimumInstances {
			problems = append(problems, fmt.Errorf("preset %q requires at least %d Aurora instances", preset, minimumInstances))
		}
		if strings.TrimSpace(c.SearchProvisioned.InstanceType) == "" || c.SearchProvisioned.InstanceCount < 2 || strings.TrimSpace(c.SearchProvisioned.EBSVolumeType) == "" || c.SearchProvisioned.EBSVolumeSizeGiB <= 0 {
			problems = append(problems, errors.New("non-preview catalog requires explicit OpenSearch capacity"))
		}
		if strings.TrimSpace(c.RabbitMQ.InstanceType) == "" {
			problems = append(problems, errors.New("non-preview catalog requires an explicit RabbitMQ instance type"))
		}
		if preset == sdk.PresetStandard && c.Valkey.ReplicaCount < 1 {
			problems = append(problems, errors.New("standard Valkey catalog profile requires at least one replica"))
		}
	}
	if preset == sdk.PresetHighAvailability && (c.SearchProvisioned.InstanceCount%3 != 0 || strings.TrimSpace(c.SearchProvisioned.DedicatedMasterType) == "" || c.SearchProvisioned.DedicatedMasterCount != 3 || c.Valkey.ReplicaCount < 2) {
		problems = append(problems, errors.New("high-availability catalog requires at least two Valkey replicas, OpenSearch multiples of three, and three dedicated masters"))
	}
	return errors.Join(problems...)
}

func halfStep(value float64) bool {
	return value*2 == float64(int(value*2))
}

var (
	kmsARN       = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):kms:[a-z0-9-]+:[0-9]{12}:key/[A-Za-z0-9-]+$`)
	secretARN    = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):secretsmanager:[a-z0-9-]+:[0-9]{12}:secret:[A-Za-z0-9/_+=.@-]+$`)
	databaseName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
	username     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,15}$`)
)
