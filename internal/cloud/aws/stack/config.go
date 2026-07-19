package stack

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	awstarget "github.com/acourtiol/magelift/internal/cloud/aws/target"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/topology"
	sdk "github.com/acourtiol/magelift/sdk/v1"
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
	var existingNetwork *sdk.ExistingResourceRef
	existingPublicSubnets := append([]string(nil), aws.Existing.PublicSubnetIDs...)
	existingPrivateSubnets := append([]string(nil), aws.Existing.PrivateSubnetIDs...)
	existingDataSubnets := append([]string(nil), aws.Existing.DataSubnetIDs...)
	if aws.Existing.Network != nil {
		ref := sdk.ExistingResourceRef{ID: sdk.ResourceID(cfg.Project.Name + "-" + environment + "-network"), Provider: sdk.ProviderID(aws.Existing.Network.Provider), Kind: sdk.ExistingResourceKind(aws.Existing.Network.Kind), ExternalID: aws.Existing.Network.ExternalID}
		existingNetwork = &ref
	}
	status := "compatible"
	if cfg.Compatibility.AllowUnsupported {
		status = "unsupported-allowed"
	}
	if err := validateAWSServiceCompatibility(cfg); err != nil && !cfg.Compatibility.AllowUnsupported {
		return Spec{}, err
	}
	spec := Spec{
		Identity: Identity{
			Project: cfg.Project.Name, Environment: environment, AccountID: cfg.Account, Region: cfg.Defaults.Region,
			EnvironmentClass: class, Preset: preset,
			Tags: map[string]string{"magelift:managed-by": "magelift", "magelift:project": cfg.Project.Name, "magelift:environment": environment},
		},
		Application:  Application{Edition: cfg.Application.Edition, Version: cfg.Application.Version, Mode: cfg.Application.Mode, WebRuntime: cfg.Application.WebRuntime},
		Artifact:     Artifact{ImageDigest: aws.ImageDigest, CompatibilityStatus: status},
		Lifecycle:    Lifecycle{ExpiresAt: expiresAt, MonthlyBudgetCents: cfg.MonthlyBudgetCents, Protection: cfg.Protection},
		Existing:     ExistingResources{Network: existingNetwork, PublicSubnetIDs: existingPublicSubnets, PrivateSubnetIDs: existingPrivateSubnets, DataSubnetIDs: existingDataSubnets, HostedZone: &hostedZone, Certificate: &cloudFrontCertificate, ALBCertificate: &albCertificate, SNSTopicARN: aws.SNSTopicARN},
		Dependencies: Dependencies{KMSKeyARN: aws.KMSKeyARN, CacheSecretARN: aws.CacheSecretARN, SessionSecretARN: aws.SessionSecretARN, QueueSecretARN: aws.QueueSecretARN, EncryptionKeyARN: aws.EncryptionKeySecretARN, DatabaseName: aws.DatabaseName, MasterUsername: aws.MasterUsername},
		Policy:       NetworkPolicy{VPCCIDR: cidr, AvailabilityZones: append([]string(nil), aws.AvailabilityZones...), MediaDomain: aws.MediaDomain, ApplicationDomain: cfg.Domain, NatMode: resolveNatMode(aws.NatMode)},
		Catalog:      catalogFromConfig(aws.Catalog, preset),
	}
	validate := spec.Validate
	if options.AllowExpiredPreview && preset == sdk.PresetPreview && !expiresAt.IsZero() && expiresAt.Before(time.Now().UTC()) {
		validate = spec.ValidateAllowExpiredPreview
	}
	if err := validate(); err != nil {
		return Spec{}, fmt.Errorf("AWS deployment plan is invalid: %w", err)
	}
	return spec, nil
}

func validateAWSServiceCompatibility(cfg config.Config) error {
	versionLine := strings.SplitN(cfg.Application.Version, "-", 2)[0]
	policy, ok := map[string]struct {
		openSearchPrefix string
		valkeyPrefixes   []string
	}{
		"2.4.9": {openSearchPrefix: "OpenSearch_3", valkeyPrefixes: []string{"8", "9"}},
		"2.4.8": {openSearchPrefix: "OpenSearch_3", valkeyPrefixes: []string{"8"}},
		"2.4.7": {openSearchPrefix: "OpenSearch_", valkeyPrefixes: []string{"8"}},
		"2.4.6": {openSearchPrefix: "OpenSearch_", valkeyPrefixes: []string{"8"}},
	}[versionLine]
	if !ok {
		return fmt.Errorf("AWS service compatibility is not cataloged for Magento %s", versionLine)
	}
	versions := cfg.Target.AWS.Catalog.Versions
	engine := resolveDatabaseEngine(cfg.Target.AWS.Catalog.DatabaseEngine)
	presetName := cfg.Preset
	if strings.TrimSpace(presetName) == "" {
		presetName = cfg.Defaults.Preset
	}
	searchMode := resolveSearchMode(cfg.Target.AWS.Catalog.SearchMode, sdk.PresetID(presetName))
	if searchMode != SearchModeDisabled {
		searchCompatible := strings.HasPrefix(versions.OpenSearch, policy.openSearchPrefix)
		if versionLine == "2.4.7" || versionLine == "2.4.6" {
			searchCompatible = strings.HasPrefix(versions.OpenSearch, "OpenSearch_2") || strings.HasPrefix(versions.OpenSearch, "OpenSearch_3")
		}
		if !searchCompatible {
			return fmt.Errorf("Magento %s requires an Adobe-listed AWS OpenSearch 2 or 3 version", versionLine)
		}
	}
	if !hasPrefix(versions.Valkey, policy.valkeyPrefixes) {
		return fmt.Errorf("Magento %s requires an Adobe-listed AWS Valkey version", versionLine)
	}
	if !strings.HasPrefix(versions.RabbitMQ, "3.13") && !strings.HasPrefix(versions.RabbitMQ, "4.2") {
		return fmt.Errorf("Magento %s requires AWS MQ RabbitMQ 3.13 or 4.2 for the v1 target", versionLine)
	}
	// Preview uses Magento database queues, so the RabbitMQ instance type may be unset.
	if instanceType := strings.TrimSpace(cfg.Target.AWS.Catalog.RabbitMQ.InstanceType); strings.HasPrefix(versions.RabbitMQ, "4.2") && instanceType != "" && !strings.HasPrefix(instanceType, "mq.m7g.") {
		return errors.New("AWS MQ RabbitMQ 4.2 requires an mq.m7g instance type")
	}
	if engine == DatabaseEngineRDSMySQL {
		if strings.TrimSpace(versions.MySQL) == "" || (!strings.HasPrefix(versions.MySQL, "8.0.") && !strings.HasPrefix(versions.MySQL, "8.4.")) {
			return fmt.Errorf("Magento %s requires an AWS RDS MySQL 8.0 or 8.4 engine", versionLine)
		}
		return nil
	}
	if !strings.Contains(versions.AuroraMySQL, ".3.12") && !strings.Contains(versions.AuroraMySQL, ".3.11") {
		return fmt.Errorf("Magento %s requires an AWS Aurora MySQL 3.11 or 3.12 engine", versionLine)
	}
	return nil
}

func hasPrefix(value string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
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
		Version:           input.Version,
		DatabaseEngine:    resolveDatabaseEngine(input.DatabaseEngine),
		SearchMode:        resolveSearchMode(input.SearchMode, preset),
		Aurora:            AuroraPreviewProfile{MinimumACU: input.Aurora.MinimumACU, MaximumACU: input.Aurora.MaximumACU, AutoPauseSeconds: input.Aurora.AutoPauseSeconds, EngineSupportsAutoPause: input.Aurora.EngineSupportsAutoPause},
		Valkey:            ValkeyPreviewProfile{NodeType: input.Valkey.NodeType, ReplicaCount: input.Valkey.ReplicaCount},
		Search:            SearchPreviewProfile{MaximumIndexingOCU: input.Search.MaximumIndexingOCU, MaximumSearchOCU: input.Search.MaximumSearchOCU, AcceptColdStarts: input.Search.AcceptColdStarts},
		Fargate:           FargatePreviewProfile{CPU: input.Fargate.CPU, MemoryMiB: input.Fargate.MemoryMiB, DesiredCount: input.Fargate.DesiredCount},
		Retention:         RetentionProfile{LogDays: input.Retention.LogDays, BackupDays: input.Retention.BackupDays, ArtifactDays: input.Retention.ArtifactDays},
		Versions:          ServiceVersions{AuroraMySQL: input.Versions.AuroraMySQL, MySQL: input.Versions.MySQL, Valkey: input.Versions.Valkey, OpenSearch: input.Versions.OpenSearch, RabbitMQ: input.Versions.RabbitMQ},
		AuroraProvisioned: AuroraProvisionedProfile{InstanceClass: input.Aurora.InstanceClass, InstanceCount: input.Aurora.InstanceCount},
		SearchProvisioned: SearchProvisionedProfile{InstanceType: input.Search.InstanceType, InstanceCount: input.Search.InstanceCount, DedicatedMasterType: input.Search.DedicatedMasterType, DedicatedMasterCount: input.Search.DedicatedMasterCount, EBSVolumeType: input.Search.EBSVolumeType, EBSVolumeSizeGiB: input.Search.EBSVolumeSizeGiB},
		RabbitMQ:          RabbitMQProfile{InstanceType: input.RabbitMQ.InstanceType},
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
