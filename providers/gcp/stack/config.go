package stack

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/cloud/kube/queue"
	"github.com/magelift/magelift/internal/cloud/kube/search"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/topology"
	gcpdatabase "github.com/magelift/magelift/providers/gcp/database"
	gcpruntime "github.com/magelift/magelift/providers/gcp/runtime"
	providerschema "github.com/magelift/magelift/providers/gcp/schema"
	gcptarget "github.com/magelift/magelift/providers/gcp/target"
	"github.com/magelift/magelift/sdk"
)

// PlanInputs carries core-resolved protocol inputs. The envelope holds the
// effective project, environment, region, class, preset, budget, expiry,
// protection, and domain; Target holds the parsed target block; the edge
// and observability intents arrive validated from the core.
type PlanInputs struct {
	Envelope            sdk.Envelope
	Application         sdk.Application
	Target              providerschema.GCPTarget
	Edge                sdk.EdgeIntent
	Observability       sdk.ObservabilityIntent
	Runtime             sdk.RuntimeID
	LiveQueueReplicas   int
	ValkeyRequirement   string
	AllowUnsupported    bool
	AllowExpiredPreview bool
	AccountOnly         bool
}

func PlanFromInputs(in PlanInputs) (Spec, error) {
	environment := in.Envelope.Environment
	if strings.TrimSpace(environment) == "" {
		return Spec{}, fmt.Errorf("environment is required")
	}
	observability := in.Observability
	edge := in.Edge
	runtimeID := in.Runtime
	if runtimeID == "" {
		runtimeID = gcptarget.RuntimeAutopilotID
	}
	if runtimeID != gcptarget.RuntimeAutopilotID && runtimeID != gcptarget.RuntimeStandardID {
		return Spec{}, fmt.Errorf("unsupported GCP runtime %q", runtimeID)
	}
	gcp := in.Target
	presetName := in.Envelope.Preset
	region := gcp.Region
	if strings.TrimSpace(region) == "" {
		region = in.Envelope.Region
	}
	if strings.TrimSpace(gcp.Project) == "" || strings.TrimSpace(region) == "" || strings.TrimSpace(presetName) == "" {
		return Spec{}, fmt.Errorf("GCP project, region, and preset must be resolved before deployment")
	}
	preset := sdk.PresetID(presetName)
	class := in.Envelope.EnvironmentClass
	if strings.TrimSpace(class) == "" {
		return Spec{}, fmt.Errorf("environment class must be explicit; it is not inferred from the environment name")
	}
	desiredTopology, err := topology.ForPreset(preset)
	if err != nil {
		return Spec{}, fmt.Errorf("resolve GCP target topology: %w", err)
	}
	if err := (gcptarget.Target{}).Validate(context.Background(), sdk.TargetRequest{
		Application:      sdk.Application{Edition: in.Application.Edition, Version: in.Application.Version, Mode: in.Application.Mode},
		EnvironmentClass: class,
		Topology:         desiredTopology,
	}); err != nil {
		return Spec{}, fmt.Errorf("GCP target validation failed: %w", err)
	}
	cloudSQLDatabaseVersion, err := gcpdatabase.DatabaseVersionForMagento(in.Application.Version)
	if err != nil {
		return Spec{}, fmt.Errorf("resolve GCP Cloud SQL database version: %w", err)
	}
	cidr := gcp.NetworkCIDR
	if strings.TrimSpace(cidr) == "" {
		cidr = "10.20.0.0/16"
	}
	if _, err := netip.ParsePrefix(cidr); err != nil {
		return Spec{}, fmt.Errorf("target.gcp.networkCidr: %w", err)
	}
	zones := append([]string(nil), gcp.Zones...)
	if len(zones) == 0 {
		zones = []string{region + "-b", region + "-c"}
		if preset == sdk.PresetHighAvailability {
			zones = []string{region + "-b", region + "-c", region + "-d"}
		}
	}
	if preset == sdk.PresetHighAvailability && len(zones) < 3 {
		return Spec{}, fmt.Errorf("high-availability preset requires at least 3 zones")
	}
	expiresAt, err := parseExpiration(in.Envelope.ExpiresAt)
	if err != nil {
		return Spec{}, err
	}
	databaseName := gcp.DatabaseName
	if databaseName == "" {
		databaseName = "magento"
	}
	masterUsername := gcp.MasterUsername
	if masterUsername == "" {
		masterUsername = "magento"
	}
	desiredWeb := gcp.DesiredWebReplicas
	if desiredWeb == 0 {
		switch preset {
		case sdk.PresetHighAvailability:
			desiredWeb = 3
		case sdk.PresetStandard:
			desiredWeb = 2
		default:
			desiredWeb = 1
		}
	}
	cloudSQLTier := gcp.CloudSQLTier
	if cloudSQLTier == "" {
		cloudSQLTier = gcpdatabase.DefaultTier(cloudSQLDatabaseVersion, string(preset))
	}
	availability := strings.TrimSpace(gcp.CloudSQLAvailability)
	if availability == "" {
		availability = "ZONAL"
		if preset != sdk.PresetPreview {
			availability = "REGIONAL"
		}
	}
	cloudSQLBackupEnabled := availability == "REGIONAL"
	if gcp.CloudSQLBackupEnabled != nil {
		cloudSQLBackupEnabled = *gcp.CloudSQLBackupEnabled
	}
	cloudSQLBinaryLogEnabled := availability == "REGIONAL"
	if gcp.CloudSQLBinaryLogEnabled != nil {
		cloudSQLBinaryLogEnabled = *gcp.CloudSQLBinaryLogEnabled
	}
	cloudSQLDeletionProtection := false
	if gcp.CloudSQLDeletionProtection != nil {
		cloudSQLDeletionProtection = *gcp.CloudSQLDeletionProtection
	}
	memorystoreNodeType := gcp.MemorystoreNodeType
	if memorystoreNodeType == "" {
		memorystoreNodeType = "SHARED_CORE_NANO"
		if preset != sdk.PresetPreview {
			memorystoreNodeType = "STANDARD_SMALL"
		}
	}
	shardCount := 1
	if gcp.MemorystoreShardCount != nil {
		shardCount = *gcp.MemorystoreShardCount
	}
	memorystoreReplicas := 0
	if gcp.MemorystoreReplicas != nil {
		memorystoreReplicas = *gcp.MemorystoreReplicas
	} else {
		if preset == sdk.PresetStandard {
			memorystoreReplicas = 1
		}
		if preset == sdk.PresetHighAvailability {
			memorystoreReplicas = 2
		}
	}
	memorystoreMode := strings.TrimSpace(gcp.MemorystoreMode)
	if memorystoreMode == "" {
		memorystoreMode = "CLUSTER_DISABLED"
	}
	memorystoreZoneDistributionMode := strings.TrimSpace(gcp.MemorystoreZoneDistributionMode)
	if memorystoreZoneDistributionMode == "" {
		memorystoreZoneDistributionMode = "MULTI_ZONE"
	}
	memorystoreZone := strings.TrimSpace(gcp.MemorystoreZone)
	if memorystoreMode == "CLUSTER_DISABLED" && shardCount != 1 {
		return Spec{}, fmt.Errorf("target.gcp.memorystoreShardCount must be 1 when memorystoreMode is CLUSTER_DISABLED")
	}
	if memorystoreZoneDistributionMode == "SINGLE_ZONE" && memorystoreZone == "" {
		return Spec{}, fmt.Errorf("target.gcp.memorystoreZone is required when memorystoreZoneDistributionMode is SINGLE_ZONE")
	}
	memorystoreDeletionProtection := false
	if gcp.MemorystoreDeletionProtection != nil {
		memorystoreDeletionProtection = *gcp.MemorystoreDeletionProtection
	}
	memorystorePSCConnectionLimit := 2
	if gcp.MemorystorePSCConnectionLimit != nil {
		memorystorePSCConnectionLimit = *gcp.MemorystorePSCConnectionLimit
	}
	memorystoreRequirement := strings.TrimSpace(in.ValkeyRequirement)
	if memorystoreRequirement == "" {
		return Spec{}, fmt.Errorf("GCP Valkey compatibility requirement is required")
	}
	configuredMemorystoreEngineVersion := strings.TrimSpace(gcp.MemorystoreEngineVersion)
	if strings.HasPrefix(memorystoreRequirement, "8") && !in.AllowUnsupported {
		return Spec{}, fmt.Errorf("the managed GCP Valkey mapping for Adobe %s is VALKEY_8_0 while the Adobe requirement is Valkey %s; use an explicitly supported managed or self-hosted Valkey 8 architecture, or set compatibility.allowUnsupported: true for a non-certifying probe", in.Application.Version, memorystoreRequirement)
	}
	memorystoreEngineVersion, err := memorystoreAPIEngineVersionForMagento(in.Application.Version, configuredMemorystoreEngineVersion, memorystoreRequirement)
	if err != nil {
		return Spec{}, fmt.Errorf("resolve GCP Memorystore engine version: %w", err)
	}
	openSearchImage := strings.TrimSpace(gcp.OpenSearchImage)
	if openSearchImage == "" {
		openSearchImage = search.DefaultImage
	}
	rabbitMQImage := strings.TrimSpace(gcp.RabbitMQImage)
	if rabbitMQImage == "" {
		rabbitMQImage = queue.DefaultImage
	}
	cpuRequest := gcp.AutopilotCPURequest
	if cpuRequest == "" {
		cpuRequest = "500m"
	}
	memoryRequest := gcp.AutopilotMemoryRequest
	if memoryRequest == "" {
		memoryRequest = gcpruntime.DefaultApplicationMemoryRequest
	}
	searchMode := strings.TrimSpace(gcp.OpenSearchMode)
	if searchMode == "" {
		searchMode = "opensearch"
	}
	searchReplicas := gcp.OpenSearchReplicas
	if searchMode == "opensearch" && searchReplicas == 0 {
		searchReplicas = 1
		if preset == sdk.PresetHighAvailability {
			searchReplicas = 3
		}
	}
	queueMode := strings.TrimSpace(gcp.QueueMode)
	if queueMode == "" {
		queueMode = "database"
		if preset != sdk.PresetPreview {
			queueMode = "rabbitmq"
		}
	}
	queueReplicas := gcp.QueueReplicas
	queueConsumers := gcp.QueueConsumerCount
	if queueMode == "rabbitmq" {
		if queueReplicas == 0 {
			queueReplicas = 1
		}
		if queueConsumers == 0 {
			queueConsumers = 1
		}
		if preset == sdk.PresetHighAvailability {
			if gcp.QueueReplicas == 0 {
				queueReplicas = 2
			}
			if queueConsumers < 2 {
				queueConsumers = 2
			}
		}
	}
	queueConsumers = platform.MagentoConsumerProcessCount(in.Application.Magento.ConsumersMode, queueConsumers)
	enableCloudArmor := preset != sdk.PresetPreview || class == "production"
	if gcp.EnableCloudArmor != nil {
		enableCloudArmor = *gcp.EnableCloudArmor
	}
	labels := map[string]string{
		"magelift-managed-by":  "magelift",
		"magelift-project":     in.Envelope.Project,
		"magelift-environment": environment,
		"magelift-tier":        "experimental",
	}
	for key, value := range gcp.Labels {
		labels[key] = value
	}
	spec := Spec{
		Identity: Identity{
			Project: in.Envelope.Project, GCPProject: gcp.Project, Environment: environment, Region: region,
			EnvironmentClass: class, Preset: preset, Runtime: runtimeID, Labels: labels,
		},
		Application: Application{Edition: in.Application.Edition, Version: in.Application.Version, Mode: in.Application.Mode, WebRuntime: in.Application.WebRuntime, Magento: platform.NewMagentoOverlays(in.Application.Magento.FrontName, in.Application.Magento.CookieDomain, in.Application.Magento.UnsecureBaseURL, in.Application.Magento.SecureBaseURL, in.Application.Magento.StorefrontOrigin, in.Application.Magento.ConsumersMode, in.Application.Magento.CORSOrigins, in.Application.Magento.ConsumerNames, in.Application.Magento.Variables)},
		Artifact:    Artifact{ImageDigest: gcp.ImageDigest},
		Lifecycle:   Lifecycle{ExpiresAt: expiresAt, Protection: in.Envelope.Protected},
		Policy:      NetworkPolicy{NetworkCIDR: cidr, Zones: zones, ApplicationDomain: in.Envelope.Domain},
		Catalog: CatalogSelection{
			CloudSQLTier: cloudSQLTier, CloudSQLAvailability: availability, CloudSQLDatabaseVersion: cloudSQLDatabaseVersion,
			CloudSQLBackupEnabled: cloudSQLBackupEnabled, CloudSQLBinaryLogEnabled: cloudSQLBinaryLogEnabled,
			CloudSQLBackupRetentionCount: valueOrZero(gcp.CloudSQLBackupRetentionCount), CloudSQLTransactionLogRetention: valueOrZero(gcp.CloudSQLTransactionLogRetention),
			CloudSQLBackupStartTime: strings.TrimSpace(gcp.CloudSQLBackupStartTime), CloudSQLBackupLocation: strings.TrimSpace(gcp.CloudSQLBackupLocation), CloudSQLDeletionProtection: cloudSQLDeletionProtection,
			MemorystoreNodeType: memorystoreNodeType, MemorystoreShardCount: shardCount, MemorystoreEngineVersion: memorystoreEngineVersion, MemorystoreReplicas: memorystoreReplicas,
			MemorystoreMode: memorystoreMode, MemorystoreZoneDistributionMode: memorystoreZoneDistributionMode, MemorystoreZone: memorystoreZone,
			MemorystoreDeletionProtection: memorystoreDeletionProtection, MemorystorePSCConnectionLimit: memorystorePSCConnectionLimit,
			KubernetesVersion: strings.TrimSpace(gcp.KubernetesVersion), ReleaseChannel: strings.TrimSpace(gcp.ReleaseChannel),
			ClusterIPv4CIDR: strings.TrimSpace(gcp.ClusterIPv4CIDR), ServicesIPv4CIDR: strings.TrimSpace(gcp.ServicesIPv4CIDR),
			StandardNodeType: strings.TrimSpace(gcp.StandardNodeType), StandardNodeCount: gcp.StandardNodeCount,
			StandardNodeMinCount: gcp.StandardNodeMinCount, StandardNodeMaxCount: gcp.StandardNodeMaxCount,
			StandardNodeDiskType: strings.TrimSpace(gcp.StandardNodeDiskType), StandardNodeDiskSizeGiB: gcp.StandardNodeDiskSizeGiB,
			StandardNodeImageType: strings.TrimSpace(gcp.StandardNodeImageType), StandardNodeSpot: gcp.StandardNodeSpot,
			OpenSearchImage: openSearchImage, RabbitMQImage: rabbitMQImage,
			AutopilotCPURequest: cpuRequest, AutopilotMemoryRequest: memoryRequest,
			DesiredWebReplicas: desiredWeb, QueueConsumerCount: queueConsumers,
			SearchMode: searchMode, SearchReplicas: searchReplicas,
			QueueMode: queueMode, QueueReplicas: queueReplicas,
			LiveQueueReplicas: in.LiveQueueReplicas,
			ValkeyRequirement: memorystoreRequirement,
			EnableCloudArmor:  enableCloudArmor,
		},
		Dependencies: Dependencies{
			DatabaseName: databaseName, MasterUsername: masterUsername, EncryptionKeySecret: gcp.EncryptionKeySecret,
		},
		Email: EmailSelection{
			Mode: in.Application.Email.Mode, Host: strings.TrimSpace(in.Application.Email.Host),
			Port: in.Application.Email.Port, Username: strings.TrimSpace(in.Application.Email.Username),
			From: strings.TrimSpace(in.Application.Email.From), Credential: strings.TrimSpace(in.Application.Email.Credential),
		},
		Edge:          edge,
		Observability: observability,
	}
	spec.AllowExpiredPreview = in.AllowExpiredPreview
	spec.AccountOnly = in.AccountOnly
	validate := spec.Validate
	switch {
	case in.AccountOnly:
		validate = spec.ValidateAccount
	case in.AllowExpiredPreview:
		validate = spec.ValidateAllowExpiredPreview
	}
	if err := validate(); err != nil {
		return Spec{}, fmt.Errorf("GCP deployment plan is invalid: %w", err)
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

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
