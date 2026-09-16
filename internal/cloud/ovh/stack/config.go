package stack

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/cloud/kube"
	ovhtarget "github.com/magelift/magelift/internal/cloud/ovh/target"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/topology"
	"github.com/magelift/magelift/sdk"
)

type PlanOptions struct {
	AllowExpiredPreview bool
}

func PlanFromConfig(cfg config.Config, environment string) (Spec, error) {
	if err := platform.ValidateFirstPartyEdge(cfg); err != nil {
		return Spec{}, err
	}
	if err := platform.ValidateFirstPartyObservability(cfg); err != nil {
		return Spec{}, err
	}
	return PlanFromConfigWithOptions(cfg, environment, PlanOptions{})
}

func PlanFromConfigWithOptions(cfg config.Config, environment string, options PlanOptions) (Spec, error) {
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
	if strings.TrimSpace(environment) == "" {
		return Spec{}, fmt.Errorf("environment is required")
	}
	if cfg.Target.Provider != string(ovhtarget.ProviderID) || cfg.Target.Runtime != string(ovhtarget.RuntimeID) {
		return Spec{}, fmt.Errorf("unsupported target %q/%q; OVH stack requires %q/%q", cfg.Target.Provider, cfg.Target.Runtime, ovhtarget.ProviderID, ovhtarget.RuntimeID)
	}
	if cfg.Target.OVH == nil {
		return Spec{}, fmt.Errorf("target.ovh is required for OVH deployment")
	}
	ovh := cfg.Target.OVH
	presetName := cfg.Preset
	if strings.TrimSpace(presetName) == "" {
		presetName = cfg.Defaults.Preset
	}
	region := ovh.Region
	if strings.TrimSpace(region) == "" {
		region = cfg.Defaults.Region
	}
	if strings.TrimSpace(ovh.ServiceName) == "" || strings.TrimSpace(region) == "" || strings.TrimSpace(presetName) == "" {
		return Spec{}, fmt.Errorf("OVH serviceName, region, and preset must be resolved before deployment")
	}
	apiEndpoint := strings.TrimSpace(ovh.APIEndpoint)
	if apiEndpoint == "" {
		apiEndpoint = ovhDefaultAPIEndpoint
	}
	preset := sdk.PresetID(presetName)
	class := cfg.Class
	if strings.TrimSpace(class) == "" {
		return Spec{}, fmt.Errorf("environment class must be explicit; it is not inferred from the environment name")
	}
	desiredTopology, err := topology.ForPreset(preset)
	if err != nil {
		return Spec{}, fmt.Errorf("resolve OVH target topology: %w", err)
	}
	if err := (ovhtarget.Target{}).Validate(context.Background(), sdk.TargetRequest{
		Application:      sdk.Application{Edition: cfg.Application.Edition, Version: cfg.Application.Version, Mode: cfg.Application.Mode},
		EnvironmentClass: class,
		Topology:         desiredTopology,
	}); err != nil {
		return Spec{}, fmt.Errorf("OVH target validation failed: %w", err)
	}
	cidr := ovh.NetworkCIDR
	if strings.TrimSpace(cidr) == "" {
		cidr = "10.30.0.0/16"
	}
	if _, err := netip.ParsePrefix(cidr); err != nil {
		return Spec{}, fmt.Errorf("target.ovh.networkCidr: %w", err)
	}
	zonesExplicit := len(ovh.Zones) > 0
	zones := append([]string(nil), ovh.Zones...)
	if len(zones) == 0 {
		zones = []string{region}
	}
	expiresAt, err := parseExpiration(cfg.ExpiresAt)
	if err != nil {
		return Spec{}, err
	}
	databaseName := ovh.DatabaseName
	if databaseName == "" {
		databaseName = "magento"
	}
	masterUsername := ovh.MasterUsername
	if masterUsername == "" {
		masterUsername = "magento"
	}
	desiredWeb := ovh.DesiredWebReplicas
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
	dbFlavor := ovh.DatabaseFlavor
	if dbFlavor == "" {
		dbFlavor = ovhDefaultDatabaseFlavor
	}
	dbPlan := ovh.DatabasePlan
	if dbPlan == "" {
		dbPlan = ovhDefaultManagedServicePlan(preset)
	}
	dbVersion := ovh.DatabaseVersion
	if dbVersion == "" {
		dbVersion = "8.4"
	}
	databaseNodeCount := ovh.DatabaseNodeCount
	if databaseNodeCount == 0 {
		databaseNodeCount = ovhDefaultManagedServiceNodeCount(preset, dbPlan)
	}
	valkeyFlavor := ovh.ValkeyFlavor
	if valkeyFlavor == "" {
		valkeyFlavor = ovhDefaultDatabaseFlavor
	}
	valkeyPlan := ovh.ValkeyPlan
	if valkeyPlan == "" {
		valkeyPlan = ovhDefaultManagedServicePlan(preset)
	}
	valkeyVersion := ovh.ValkeyVersion
	if valkeyVersion == "" {
		valkeyVersion = ovhDefaultValkeyVersion
	}
	valkeyNodeCount := ovh.ValkeyNodeCount
	if valkeyNodeCount == 0 {
		valkeyNodeCount = ovhDefaultManagedServiceNodeCount(preset, valkeyPlan)
	}
	mksPlan := ovh.MKSPlan
	if mksPlan == "" {
		mksPlan = "standard"
	}
	nodeFlavor := ovh.NodeFlavor
	if nodeFlavor == "" {
		nodeFlavor = "b3-8"
	}
	nodeCountExplicit := ovh.NodeCount > 0
	nodeCount := ovh.NodeCount
	if nodeCount == 0 {
		nodeCount = 1
		if preset != sdk.PresetPreview {
			nodeCount = 2
		}
		if nodeCount < len(zones) {
			nodeCount = len(zones)
		}
	}
	cpuRequest := ovh.CPURequest
	if cpuRequest == "" {
		cpuRequest = "500m"
	}
	memoryRequest := ovh.MemoryRequest
	if memoryRequest == "" {
		memoryRequest = kube.DefaultApplicationMemoryRequest
	}
	labels := map[string]string{
		"magelift-managed-by":  "magelift",
		"magelift-project":     cfg.Project.Name,
		"magelift-environment": environment,
		"magelift-tier":        "experimental",
	}
	for key, value := range ovh.Labels {
		labels[key] = value
	}
	spec := Spec{
		Identity: Identity{
			Project: cfg.Project.Name, ServiceName: ovh.ServiceName, APIEndpoint: apiEndpoint, Environment: environment, Region: region,
			EnvironmentClass: class, Preset: preset, Labels: labels,
		},
		Application: Application{Edition: cfg.Application.Edition, Version: cfg.Application.Version, Mode: cfg.Application.Mode, WebRuntime: cfg.Application.WebRuntime, Magento: platform.NewMagentoOverlays(cfg.Application.Magento.FrontName, cfg.Application.Magento.CookieDomain, cfg.Application.Magento.UnsecureBaseURL, cfg.Application.Magento.SecureBaseURL, cfg.Application.Magento.StorefrontOrigin, cfg.Application.Magento.Consumers.Mode, cfg.Application.Magento.CORSOrigins, cfg.Application.Magento.Consumers.Names, cfg.Application.Magento.Variables)},
		Artifact:    Artifact{ImageDigest: ovh.ImageDigest},
		Lifecycle:   Lifecycle{ExpiresAt: expiresAt, Protection: cfg.Protection},
		Policy:      NetworkPolicy{NetworkCIDR: cidr, Zones: zones, ZonesExplicit: zonesExplicit},
		Catalog: CatalogSelection{
			DatabaseFlavor: dbFlavor, DatabasePlan: dbPlan, DatabaseVersion: dbVersion, DatabaseNodeCount: databaseNodeCount,
			DatabaseBackupTime: strings.TrimSpace(ovh.DatabaseBackupTime), DatabaseBackupRegions: append([]string(nil), ovh.DatabaseBackupRegions...), DatabaseDeletionProtection: ovh.DatabaseDeletionProtection,
			ValkeyFlavor: valkeyFlavor, ValkeyPlan: valkeyPlan, ValkeyVersion: valkeyVersion, ValkeyNodeCount: valkeyNodeCount,
			ValkeyBackupTime: strings.TrimSpace(ovh.ValkeyBackupTime), ValkeyBackupRegions: append([]string(nil), ovh.ValkeyBackupRegions...), ValkeyDeletionProtection: ovh.ValkeyDeletionProtection,
			MKSPlan: mksPlan, AttachFloatingIPs: ovh.AttachFloatingIPs,
			PrivateNetworkRoutingAsDefault: ovh.PrivateNetworkRoutingAsDefault,
			NodeFlavor:                     nodeFlavor, NodeCount: nodeCount, NodeCountExplicit: nodeCountExplicit, CPURequest: cpuRequest, MemoryRequest: memoryRequest,
			DesiredWebReplicas: desiredWeb, QueueConsumerCount: platform.MagentoConsumerProcessCount(cfg.Application.Magento.Consumers.Mode, ovh.QueueConsumerCount),
		},
		Dependencies: Dependencies{
			DatabaseName: databaseName, MasterUsername: masterUsername, EncryptionKeySecret: ovh.EncryptionKeySecret,
			StateBucket: ovh.StateBucket, StateEndpoint: ovh.StateEndpoint, StateRegion: ovh.StateRegion,
		},
		Edge:          edgeIntent,
		Observability: observability,
	}
	spec.AllowExpiredPreview = options.AllowExpiredPreview
	validate := spec.Validate
	if options.AllowExpiredPreview {
		validate = spec.ValidateAllowExpiredPreview
	}
	if err := validate(); err != nil {
		return Spec{}, fmt.Errorf("OVH deployment plan is invalid: %w", err)
	}
	return spec, nil
}

func ovhDefaultManagedServicePlan(preset sdk.PresetID) string {
	if preset == sdk.PresetPreview {
		return "discovery"
	}
	return "production"
}

func ovhDefaultManagedServiceNodeCount(preset sdk.PresetID, plan string) int {
	switch plan {
	case "discovery", "essential":
		return 1
	case "business", "production":
		return 2
	case "enterprise", "advanced":
		return 3
	default:
		if preset == sdk.PresetPreview {
			return 1
		}
		return 2
	}
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
