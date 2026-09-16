package stack

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/cloud/aws/network"
	awstarget "github.com/magelift/magelift/internal/cloud/aws/target"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/edge/waf"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/topology"
	"github.com/magelift/magelift/sdk"
)

// PlanOptions controls validation exceptions that are safe only for a specific
// lifecycle operation.
type PlanOptions struct {
	AllowExpiredPreview bool
}

// PlanFromConfig converts the resolved public configuration into the internal
// AWS stack contract. It does not contact AWS or register Pulumi resources.
func PlanFromConfig(cfg config.Config, environment string) (Spec, error) {
	return PlanFromConfigWithOptions(cfg, environment, PlanOptions{})
}

// PlanFromConfigWithOptions converts configuration while allowing a caller to
// opt into the narrow validation exception needed to destroy an expired
// preview stack. No other operation should set AllowExpiredPreview.
func PlanFromConfigWithOptions(cfg config.Config, environment string, options PlanOptions) (Spec, error) {
	if strings.TrimSpace(environment) == "" {
		return Spec{}, fmt.Errorf("environment is required")
	}
	if cfg.Target.Provider != string(awstarget.ProviderID) || cfg.Target.Runtime != string(awstarget.RuntimeID) {
		return Spec{}, fmt.Errorf("unsupported target %q/%q; AWS stack requires %q/%q", cfg.Target.Provider, cfg.Target.Runtime, awstarget.ProviderID, awstarget.RuntimeID)
	}
	if cfg.Target.AWS == nil {
		return Spec{}, fmt.Errorf("target.aws is required for AWS deployment")
	}
	if err := platform.ValidateFirstPartyEdge(cfg); err != nil {
		return Spec{}, err
	}
	if err := platform.ValidateFirstPartyObservability(cfg); err != nil {
		return Spec{}, err
	}
	observability, err := platform.ObservabilityIntentFromConfig(cfg, environment)
	if err != nil {
		return Spec{}, err
	}
	edgeIntent, err := platform.EdgeIntentFromConfig(cfg, environment)
	if err != nil {
		return Spec{}, err
	}
	if strings.TrimSpace(cfg.Edge.Mode) == "" && strings.TrimSpace(cfg.Edge.NativeProvider) == "" && strings.TrimSpace(cfg.Edge.ExternalProvider) == "" {
		edgeIntent = defaultAWSEdgeIntent(edgeIntent, cfg.Domain)
	}
	aws := cfg.Target.AWS
	presetName := cfg.Preset
	if strings.TrimSpace(presetName) == "" {
		presetName = cfg.Defaults.Preset
	}
	if strings.TrimSpace(cfg.Account) == "" || strings.TrimSpace(cfg.Defaults.Region) == "" || strings.TrimSpace(presetName) == "" {
		return Spec{}, fmt.Errorf("account, region, and preset must be resolved before deployment")
	}
	preset := sdk.PresetID(presetName)
	class := cfg.Class
	if strings.TrimSpace(class) == "" {
		return Spec{}, fmt.Errorf("environment class must be explicit; it is not inferred from the environment name")
	}
	desiredTopology, err := topology.ForPreset(preset)
	if err != nil {
		return Spec{}, fmt.Errorf("resolve AWS target topology: %w", err)
	}
	if err := (awstarget.Target{}).Validate(context.Background(), sdk.TargetRequest{
		Application:      sdk.Application{Edition: cfg.Application.Edition, Version: cfg.Application.Version, Mode: cfg.Application.Mode},
		EnvironmentClass: class,
		Topology:         desiredTopology,
	}); err != nil {
		return Spec{}, fmt.Errorf("AWS target validation failed: %w", err)
	}
	cidr, err := netip.ParsePrefix(aws.VPCCIDR)
	if err != nil {
		return Spec{}, fmt.Errorf("target.aws.vpcCidr: %w", err)
	}
	expiresAt, err := parseExpiration(cfg.ExpiresAt)
	if err != nil {
		return Spec{}, err
	}
	zone := sdk.ExistingDNSZone
	certificate := sdk.ExistingCertificate
	hostedZone := sdk.ExistingResourceRef{ID: sdk.ResourceID(cfg.Project.Name + "-" + environment + "-hosted-zone"), Provider: sdk.ProviderID("aws"), Kind: zone, ExternalID: aws.HostedZoneID}
	cloudFrontCertificate := sdk.ExistingResourceRef{ID: sdk.ResourceID(cfg.Project.Name + "-" + environment + "-cloudfront-certificate"), Provider: sdk.ProviderID("aws"), Kind: certificate, ExternalID: aws.CloudFrontCertificateARN}
	albCertificate := sdk.ExistingResourceRef{ID: sdk.ResourceID(cfg.Project.Name + "-" + environment + "-alb-certificate"), Provider: sdk.ProviderID("aws"), Kind: certificate, ExternalID: aws.ALBCertificateARN}
	var hostedZoneRef *sdk.ExistingResourceRef
	cloudFrontCertificateRef := &cloudFrontCertificate
	if nativeEdgeEnabled(edgeIntent) {
		hostedZoneRef = &hostedZone
	}
	var existingNetwork *sdk.ExistingResourceRef
	var existingDatabase *sdk.ExistingResourceRef
	existingPublicSubnets := append([]string(nil), aws.Existing.PublicSubnetIDs...)
	existingPrivateSubnets := append([]string(nil), aws.Existing.PrivateSubnetIDs...)
	existingDataSubnets := append([]string(nil), aws.Existing.DataSubnetIDs...)
	existingDatabaseSecretARN := ""
	existingDatabaseEndpoint := ""
	if aws.Existing.Network != nil {
		ref := sdk.ExistingResourceRef{ID: sdk.ResourceID(cfg.Project.Name + "-" + environment + "-network"), Provider: sdk.ProviderID(aws.Existing.Network.Provider), Kind: sdk.ExistingResourceKind(aws.Existing.Network.Kind), ExternalID: aws.Existing.Network.ExternalID}
		existingNetwork = &ref
	}
	if aws.Existing.Database != nil {
		ref := sdk.ExistingResourceRef{ID: sdk.ResourceID(cfg.Project.Name + "-" + environment + "-database"), Provider: sdk.ProviderID(aws.Existing.Database.Provider), Kind: sdk.ExistingResourceKind(aws.Existing.Database.Kind), ExternalID: aws.Existing.Database.ExternalID}
		existingDatabase = &ref
		existingDatabaseSecretARN = aws.Existing.Database.SecretARN
		existingDatabaseEndpoint = aws.Existing.Database.Endpoint
	}
	status := "compatible"
	if cfg.Compatibility.AllowUnsupported {
		status = "unsupported-allowed"
	}
	if err := validateAWSServiceCompatibility(cfg); err != nil && !cfg.Compatibility.AllowUnsupported {
		return Spec{}, err
	}
	tags := map[string]string{"magelift:managed-by": "magelift", "magelift:project": cfg.Project.Name, "magelift:environment": environment}
	for key, value := range aws.Labels {
		tags[key] = value
	}
	natMode := resolveNatMode(aws.NatMode)
	natTopology := aws.NatTopology
	natReplacementMode := aws.NatReplacementMode
	if aws.Existing.Network == nil {
		natTopology = network.ResolveNatTopology(natTopology, preset)
		natReplacementMode = network.ResolveNatReplacementMode(natReplacementMode, preset, natMode, natTopology)
	}
	spec := Spec{
		Identity: Identity{
			Project: cfg.Project.Name, Environment: environment, AccountID: cfg.Account, Region: cfg.Defaults.Region,
			EnvironmentClass: class, Preset: preset,
			Tags: tags,
		},
		Application: Application{Edition: cfg.Application.Edition, Version: cfg.Application.Version, Mode: cfg.Application.Mode, WebRuntime: cfg.Application.WebRuntime, Magento: platform.NewMagentoOverlays(cfg.Application.Magento.FrontName, cfg.Application.Magento.CookieDomain, cfg.Application.Magento.UnsecureBaseURL, cfg.Application.Magento.SecureBaseURL, cfg.Application.Magento.StorefrontOrigin, cfg.Application.Magento.Consumers.Mode, cfg.Application.Magento.CORSOrigins, cfg.Application.Magento.Consumers.Names, cfg.Application.Magento.Variables)},
		Artifact:    Artifact{ImageDigest: aws.ImageDigest, CompatibilityStatus: status},
		Lifecycle:   Lifecycle{ExpiresAt: expiresAt, MonthlyBudgetCents: cfg.MonthlyBudgetCents, Protection: cfg.Protection},
		Existing: ExistingResources{
			Network: existingNetwork, PublicSubnetIDs: existingPublicSubnets, PrivateSubnetIDs: existingPrivateSubnets, DataSubnetIDs: existingDataSubnets,
			Database: existingDatabase, DatabaseSecretARN: existingDatabaseSecretARN, DatabaseEndpoint: existingDatabaseEndpoint,
			HostedZone: hostedZoneRef, Certificate: cloudFrontCertificateRef, ALBCertificate: &albCertificate, SNSTopicARN: aws.SNSTopicARN,
		},
		Dependencies:  Dependencies{KMSKeyARN: aws.KMSKeyARN, CacheSecretARN: aws.CacheSecretARN, SessionSecretARN: aws.SessionSecretARN, QueueSecretARN: aws.QueueSecretARN, EncryptionKeyARN: aws.EncryptionKeySecretARN, DatabaseName: aws.DatabaseName, MasterUsername: aws.MasterUsername},
		Policy:        NetworkPolicy{VPCCIDR: cidr, AvailabilityZones: append([]string(nil), aws.AvailabilityZones...), MediaDomain: aws.MediaDomain, ApplicationDomain: cfg.Domain, NatMode: natMode, NatTopology: natTopology, NatReplacementMode: natReplacementMode, NatInstanceType: aws.NatInstanceType},
		Catalog:       catalogFromConfig(aws.Catalog, preset),
		Edge:          edgeIntent,
		Observability: observability,
	}
	validate := spec.Validate
	if options.AllowExpiredPreview && preset == sdk.PresetPreview && !expiresAt.IsZero() && expiresAt.Before(time.Now().UTC()) {
		validate = spec.ValidateAllowExpiredPreview
		spec.AllowExpiredPreview = true
	}
	if err := validate(); err != nil {
		return Spec{}, fmt.Errorf("AWS deployment plan is invalid: %w", err)
	}
	return spec, nil
}

func defaultAWSEdgeIntent(intent sdk.EdgeIntent, domain string) sdk.EdgeIntent {
	intent.Mode = "native"
	intent.NativeProvider = "cloudfront-waf"
	intent.Domains = []string{domain}
	intent.TLS = true
	intent.TLSMode = "existing"
	intent.DNSMode = "existing"
	intent.OriginHealthRef = "aws/alb"
	intent.WAFPolicyRef = waf.PolicyRef
	return intent
}

func validateAWSServiceCompatibility(cfg config.Config) error {
	return config.ValidateAWSServiceCompatibility(cfg)
}

func parseExpiration(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("expiresAt must be RFC3339: %w", err)
	}
	return parsed.UTC(), nil
}

func catalogFromConfig(input config.AWSCatalog, preset sdk.PresetID) CatalogSelection {
	return CatalogSelection{
		Version:                        input.Version,
		DatabaseEngine:                 resolveDatabaseEngine(input.DatabaseEngine),
		SearchMode:                     resolveSearchMode(input.SearchMode, preset),
		QueueMode:                      resolveQueueMode(input.QueueMode, preset),
		DatabaseBackupWindow:           input.DatabaseBackupWindow,
		DatabaseMaintenanceWindow:      input.DatabaseMaintenanceWindow,
		DatabaseDeletionProtection:     input.DatabaseDeletionProtection,
		DatabaseDeleteAutomatedBackups: input.DatabaseDeleteAutomatedBackups,
		CacheSnapshotRetentionLimit:    input.CacheSnapshotRetentionLimit,
		CacheSnapshotWindow:            input.CacheSnapshotWindow,
		Aurora:                         AuroraPreviewProfile{MinimumACU: input.Aurora.MinimumACU, MaximumACU: input.Aurora.MaximumACU, AutoPauseSeconds: input.Aurora.AutoPauseSeconds, EngineSupportsAutoPause: input.Aurora.EngineSupportsAutoPause},
		Valkey:                         ValkeyPreviewProfile{NodeType: input.Valkey.NodeType, ReplicaCount: input.Valkey.ReplicaCount},
		Search:                         SearchPreviewProfile{MaximumIndexingOCU: input.Search.MaximumIndexingOCU, MaximumSearchOCU: input.Search.MaximumSearchOCU, AcceptColdStarts: input.Search.AcceptColdStarts},
		Fargate:                        FargatePreviewProfile{ComputeMode: input.Fargate.ComputeMode, CPU: input.Fargate.CPU, MemoryMiB: input.Fargate.MemoryMiB, DesiredCount: input.Fargate.DesiredCount, InstanceType: input.Fargate.InstanceType, InstanceAMI: input.Fargate.InstanceAMI, MinCapacity: input.Fargate.MinCapacity, MaxCapacity: input.Fargate.MaxCapacity},
		Retention:                      RetentionProfile{LogDays: input.Retention.LogDays, BackupDays: input.Retention.BackupDays, ArtifactDays: input.Retention.ArtifactDays},
		Versions:                       ServiceVersions{AuroraMySQL: config.CanonicalAuroraMySQLVersion(input.Versions.AuroraMySQL), MySQL: input.Versions.MySQL, MariaDB: input.Versions.MariaDB, Valkey: input.Versions.Valkey, OpenSearch: input.Versions.OpenSearch, RabbitMQ: input.Versions.RabbitMQ},
		AuroraProvisioned:              AuroraProvisionedProfile{InstanceClass: input.Aurora.InstanceClass, InstanceCount: input.Aurora.InstanceCount},
		SearchProvisioned:              SearchProvisionedProfile{InstanceType: input.Search.InstanceType, InstanceCount: input.Search.InstanceCount, DedicatedMasterType: input.Search.DedicatedMasterType, DedicatedMasterCount: input.Search.DedicatedMasterCount, EBSVolumeType: input.Search.EBSVolumeType, EBSVolumeSizeGiB: input.Search.EBSVolumeSizeGiB},
		RabbitMQ:                       RabbitMQProfile{InstanceType: input.RabbitMQ.InstanceType},
	}
}

func resolveNatMode(value string) string {
	if strings.TrimSpace(value) == "" {
		return NatModeGateway
	}
	return strings.TrimSpace(value)
}

func resolveDatabaseEngine(value string) string {
	if strings.TrimSpace(value) == "" {
		return DatabaseEngineAuroraMySQL
	}
	return strings.TrimSpace(value)
}

func resolveSearchMode(value string, preset sdk.PresetID) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if preset == sdk.PresetPreview {
		return SearchModeServerless
	}
	return SearchModeProvisioned
}

func resolveQueueMode(value string, preset sdk.PresetID) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return defaultQueueMode(preset)
}
