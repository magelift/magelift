package eksops

import (
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/cloud/aws/network"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	RuntimeID = "eks"
	TargetID  = "aws.eks"
)

type PlanOptions struct {
	AllowExpiredPreview bool
}

func PlanFromConfig(cfg config.Config, environment string) (Spec, error) {
	return PlanFromConfigWithOptions(cfg, environment, PlanOptions{})
}

func PlanFromConfigWithOptions(cfg config.Config, environment string, options PlanOptions) (Spec, error) {
	if strings.TrimSpace(environment) == "" {
		return Spec{}, fmt.Errorf("environment is required")
	}
	if cfg.Target.Provider != "aws" || cfg.Target.Runtime != RuntimeID {
		return Spec{}, fmt.Errorf("unsupported target %q/%q; EKS stack requires aws/%s", cfg.Target.Provider, cfg.Target.Runtime, RuntimeID)
	}
	if cfg.Target.AWS == nil {
		return Spec{}, fmt.Errorf("target.aws is required for AWS EKS deployment")
	}
	if err := platform.ValidateFirstPartyEdge(cfg); err != nil {
		return Spec{}, err
	}
	if err := platform.ValidateFirstPartyObservability(cfg); err != nil {
		return Spec{}, err
	}
	aws := cfg.Target.AWS
	presetName := cfg.Preset
	if strings.TrimSpace(presetName) == "" {
		presetName = cfg.Defaults.Preset
	}
	if strings.TrimSpace(cfg.Account) == "" || strings.TrimSpace(cfg.Defaults.Region) == "" || strings.TrimSpace(presetName) == "" {
		return Spec{}, fmt.Errorf("account, region, and preset must be resolved before deployment")
	}
	class := cfg.Class
	if strings.TrimSpace(class) == "" {
		return Spec{}, fmt.Errorf("environment class must be explicit; it is not inferred from the environment name")
	}
	preset := sdk.PresetID(presetName)
	cidr, err := netip.ParsePrefix(aws.VPCCIDR)
	if err != nil {
		return Spec{}, fmt.Errorf("target.aws.vpcCidr: %w", err)
	}
	expiresAt, err := parseExpiration(cfg.ExpiresAt)
	if err != nil {
		return Spec{}, err
	}
	engine := aws.Catalog.DatabaseEngine
	if engine == "" {
		engine = DatabaseEngineAuroraMySQL
		if preset == sdk.PresetPreview {
			engine = DatabaseEngineRDSMySQL
		}
	}
	natMode := aws.NatMode
	if natMode == "" {
		natMode = NatModeGateway
	}
	natTopology := network.ResolveNatTopology(aws.NatTopology, preset)
	natReplacementMode := network.ResolveNatReplacementMode(aws.NatReplacementMode, preset, natMode, natTopology)
	cpu := aws.Catalog.EKS.CPURequest
	if cpu == "" {
		cpu = "500m"
	}
	memory := aws.Catalog.EKS.MemoryRequest
	if memory == "" {
		memory = kube.DefaultApplicationMemoryRequest
	}
	kubernetesVersion := strings.TrimSpace(aws.Catalog.EKS.KubernetesVersion)
	if kubernetesVersion == "" {
		kubernetesVersion = DefaultKubernetesVersion
	}
	computeMode := strings.TrimSpace(aws.Catalog.EKS.ComputeMode)
	if computeMode == "" {
		computeMode = ComputeModeAuto
	}
	desired := aws.Catalog.EKS.DesiredWebReplicas
	if desired == 0 {
		switch preset {
		case sdk.PresetHighAvailability:
			desired = 3
		case sdk.PresetStandard:
			desired = 2
		default:
			desired = 1
		}
	}
	nodeInstanceType := strings.TrimSpace(aws.Catalog.EKS.NodeInstanceType)
	if nodeInstanceType == "" && computeMode != ComputeModeAuto && computeMode != ComputeModeFargate {
		nodeInstanceType = "m6i.large"
	}
	nodeMinSize := aws.Catalog.EKS.NodeMinSize
	nodeDesiredSize := aws.Catalog.EKS.NodeDesiredSize
	nodeMaxSize := aws.Catalog.EKS.NodeMaxSize
	if computeMode != ComputeModeAuto && computeMode != ComputeModeFargate {
		if nodeDesiredSize == 0 {
			nodeDesiredSize = desired
		}
		if nodeMinSize == 0 {
			nodeMinSize = 1
		}
		if nodeMaxSize == 0 {
			nodeMaxSize = nodeDesiredSize
		}
	}
	fargateNamespaces := append([]string(nil), aws.Catalog.EKS.FargateNamespaces...)
	searchMode := strings.TrimSpace(aws.Catalog.EKS.SearchMode)
	if searchMode == "" {
		searchMode = SearchModeDisabled
		if preset != sdk.PresetPreview {
			searchMode = SearchModeOpenSearch
		}
	}
	searchReplicas := aws.Catalog.EKS.SearchReplicas
	if searchMode == SearchModeOpenSearch && searchReplicas == 0 {
		searchReplicas = 1
		if preset == sdk.PresetHighAvailability {
			searchReplicas = 3
		}
	}
	queueMode := strings.TrimSpace(aws.Catalog.EKS.QueueMode)
	if queueMode == "" {
		queueMode = QueueModeDatabase
		if preset != sdk.PresetPreview {
			queueMode = QueueModeRabbitMQ
		}
	}
	queueReplicas := aws.Catalog.EKS.QueueReplicas
	if queueMode == QueueModeRabbitMQ && queueReplicas == 0 {
		queueReplicas = 1
		if preset == sdk.PresetHighAvailability {
			queueReplicas = 2
		}
	}
	queueConsumers := aws.Catalog.EKS.QueueConsumerCount
	if queueMode == QueueModeRabbitMQ && queueConsumers == 0 {
		queueConsumers = 1
		if preset == sdk.PresetHighAvailability {
			queueConsumers = 2
		}
	}
	queueConsumers = platform.MagentoConsumerProcessCount(cfg.Application.Magento.Consumers.Mode, queueConsumers)
	databaseName := aws.DatabaseName
	if databaseName == "" {
		databaseName = "magento"
	}
	masterUsername := aws.MasterUsername
	if masterUsername == "" {
		masterUsername = "magento"
	}
	backupDays := aws.Catalog.Retention.BackupDays
	if backupDays == 0 {
		backupDays = 1
		if preset != sdk.PresetPreview {
			backupDays = 7
		}
	}
	spec := Spec{
		Identity: Identity{
			Project: cfg.Project.Name, Environment: environment, AccountID: cfg.Account, Region: cfg.Defaults.Region,
			EnvironmentClass: class, Preset: preset,
			Tags: map[string]string{
				"magelift:managed-by": "magelift", "magelift:project": cfg.Project.Name,
				"magelift:environment": environment, "magelift:tier": "experimental",
			},
		},
		Application: Application{Edition: cfg.Application.Edition, Version: cfg.Application.Version, Mode: cfg.Application.Mode, WebRuntime: cfg.Application.WebRuntime, Magento: platform.NewMagentoOverlays(cfg.Application.Magento.FrontName, cfg.Application.Magento.CookieDomain, cfg.Application.Magento.UnsecureBaseURL, cfg.Application.Magento.SecureBaseURL, cfg.Application.Magento.StorefrontOrigin, cfg.Application.Magento.Consumers.Mode, cfg.Application.Magento.CORSOrigins, cfg.Application.Magento.Consumers.Names, cfg.Application.Magento.Variables)},
		Artifact:    Artifact{ImageDigest: aws.ImageDigest},
		Lifecycle:   Lifecycle{ExpiresAt: expiresAt, Protection: cfg.Protection},
		Policy:      NetworkPolicy{VPCCIDR: cidr, AvailabilityZones: append([]string(nil), aws.AvailabilityZones...), NatMode: natMode, NatTopology: natTopology, NatReplacementMode: natReplacementMode, NatInstanceType: aws.NatInstanceType},
		Catalog: CatalogSelection{
			DatabaseEngine: engine, KubernetesVersion: kubernetesVersion, ComputeMode: computeMode,
			NodeInstanceType: nodeInstanceType, NodeAMI: aws.Catalog.EKS.NodeAMI, NodeMinSize: nodeMinSize, NodeDesiredSize: nodeDesiredSize, NodeMaxSize: nodeMaxSize, FargateNamespaces: fargateNamespaces,
			SearchMode: searchMode, SearchReplicas: searchReplicas,
			QueueMode: queueMode, QueueReplicas: queueReplicas,
			AuroraMinACU: aws.Catalog.Aurora.MinimumACU, AuroraMaxACU: aws.Catalog.Aurora.MaximumACU,
			AuroraAutoPause: aws.Catalog.Aurora.AutoPauseSeconds, AuroraAutoPauseOK: aws.Catalog.Aurora.EngineSupportsAutoPause,
			InstanceClass: aws.Catalog.Aurora.InstanceClass, InstanceCount: aws.Catalog.Aurora.InstanceCount,
			ValkeyNodeType: aws.Catalog.Valkey.NodeType, ValkeyReplicaCount: aws.Catalog.Valkey.ReplicaCount,
			CPURequest: cpu, MemoryRequest: memory, DesiredWebReplicas: desired, QueueConsumerCount: queueConsumers,
			BackupDays: backupDays, DatabaseBackupWindow: aws.Catalog.DatabaseBackupWindow, DatabaseMaintenanceWindow: aws.Catalog.DatabaseMaintenanceWindow,
			DatabaseDeletionProtection: aws.Catalog.DatabaseDeletionProtection, DatabaseDeleteAutomatedBackups: aws.Catalog.DatabaseDeleteAutomatedBackups,
			CacheSnapshotRetentionLimit: aws.Catalog.CacheSnapshotRetentionLimit, CacheSnapshotWindow: aws.Catalog.CacheSnapshotWindow,
			AuroraMySQLVersion: config.CanonicalAuroraMySQLVersion(aws.Catalog.Versions.AuroraMySQL), MySQLVersion: aws.Catalog.Versions.MySQL, MariaDBVersion: aws.Catalog.Versions.MariaDB, ValkeyVersion: aws.Catalog.Versions.Valkey,
		},
		Dependencies: Dependencies{
			KMSKeyARN: aws.KMSKeyARN, CacheSecretARN: aws.CacheSecretARN, SessionSecretARN: aws.SessionSecretARN,
			EncryptionKeyARN: aws.EncryptionKeySecretARN, DatabaseName: databaseName, MasterUsername: masterUsername,
		},
	}
	if err := spec.Validate(); err != nil {
		if options.AllowExpiredPreview && strings.Contains(err.Error(), "preview environment has expired") {
			return spec, nil
		}
		return Spec{}, fmt.Errorf("AWS EKS deployment plan is invalid: %w", err)
	}
	return spec, nil
}

func parseExpiration(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	expiresAt, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("expiresAt must be RFC3339: %w", err)
	}
	return expiresAt, nil
}
