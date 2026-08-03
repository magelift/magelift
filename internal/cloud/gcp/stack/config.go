package stack

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	gcptarget "github.com/magelift/magelift/internal/cloud/gcp/target"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/topology"
	sdk "github.com/magelift/magelift/sdk/v1"
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
	if cfg.Target.Provider != string(gcptarget.ProviderID) || cfg.Target.Runtime != string(gcptarget.RuntimeID) {
		return Spec{}, fmt.Errorf("unsupported target %q/%q; GCP stack requires %q/%q", cfg.Target.Provider, cfg.Target.Runtime, gcptarget.ProviderID, gcptarget.RuntimeID)
	}
	if cfg.Target.GCP == nil {
		return Spec{}, fmt.Errorf("target.gcp is required for GCP deployment")
	}
	gcp := cfg.Target.GCP
	presetName := cfg.Preset
	if strings.TrimSpace(presetName) == "" {
		presetName = cfg.Defaults.Preset
	}
	region := gcp.Region
	if strings.TrimSpace(region) == "" {
		region = cfg.Defaults.Region
	}
	if strings.TrimSpace(gcp.Project) == "" || strings.TrimSpace(region) == "" || strings.TrimSpace(presetName) == "" {
		return Spec{}, fmt.Errorf("GCP project, region, and preset must be resolved before deployment")
	}
	preset := sdk.PresetID(presetName)
	class := cfg.Class
	if strings.TrimSpace(class) == "" {
		return Spec{}, fmt.Errorf("environment class must be explicit; it is not inferred from the environment name")
	}
	desiredTopology, err := topology.ForPreset(preset)
	if err != nil {
		return Spec{}, fmt.Errorf("resolve GCP target topology: %w", err)
	}
	if err := (gcptarget.Target{}).Validate(context.Background(), sdk.TargetRequest{
		Application:      sdk.Application{Edition: cfg.Application.Edition, Version: cfg.Application.Version, Mode: cfg.Application.Mode},
		EnvironmentClass: class,
		Topology:         desiredTopology,
	}); err != nil {
		return Spec{}, fmt.Errorf("GCP target validation failed: %w", err)
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
	expiresAt, err := parseExpiration(cfg.ExpiresAt)
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
		cloudSQLTier = "db-custom-1-3840"
		if preset != sdk.PresetPreview {
			cloudSQLTier = "db-custom-2-7680"
		}
	}
	availability := "ZONAL"
	if preset != sdk.PresetPreview {
		availability = "REGIONAL"
	}
	memorystoreNodeType := gcp.MemorystoreNodeType
	if memorystoreNodeType == "" {
		memorystoreNodeType = "SHARED_CORE_NANO"
		if preset != sdk.PresetPreview {
			memorystoreNodeType = "STANDARD_SMALL"
		}
	}
	memorystoreReplicas := 0
	if preset == sdk.PresetStandard {
		memorystoreReplicas = 1
	}
	if preset == sdk.PresetHighAvailability {
		memorystoreReplicas = 2
	}
	cpuRequest := gcp.AutopilotCPURequest
	if cpuRequest == "" {
		cpuRequest = "500m"
	}
	memoryRequest := gcp.AutopilotMemoryRequest
	if memoryRequest == "" {
		memoryRequest = "1Gi"
	}
	searchMode := "opensearch"
	searchReplicas := 1
	if preset == sdk.PresetHighAvailability {
		searchReplicas = 3
	}
	queueMode := "database"
	queueReplicas := 0
	queueConsumers := gcp.QueueConsumerCount
	if preset != sdk.PresetPreview {
		queueMode = "rabbitmq"
		queueReplicas = 1
		if queueConsumers == 0 {
			queueConsumers = 1
		}
		if preset == sdk.PresetHighAvailability {
			queueReplicas = 2
			if queueConsumers < 2 {
				queueConsumers = 2
			}
		}
	}
	labels := map[string]string{
		"magelift-managed-by":  "magelift",
		"magelift-project":     cfg.Project.Name,
		"magelift-environment": environment,
		"magelift-tier":        "experimental",
	}
	for key, value := range gcp.Labels {
		labels[key] = value
	}
	spec := Spec{
		Identity: Identity{
			Project: cfg.Project.Name, GCPProject: gcp.Project, Environment: environment, Region: region,
			EnvironmentClass: class, Preset: preset, Labels: labels,
		},
		Application: Application{Edition: cfg.Application.Edition, Version: cfg.Application.Version, Mode: cfg.Application.Mode, WebRuntime: cfg.Application.WebRuntime},
		Artifact:    Artifact{ImageDigest: gcp.ImageDigest},
		Lifecycle:   Lifecycle{ExpiresAt: expiresAt, Protection: cfg.Protection},
		Policy:      NetworkPolicy{NetworkCIDR: cidr, Zones: zones},
		Catalog: CatalogSelection{
			CloudSQLTier: cloudSQLTier, CloudSQLAvailability: availability,
			MemorystoreNodeType: memorystoreNodeType, MemorystoreReplicas: memorystoreReplicas,
			AutopilotCPURequest: cpuRequest, AutopilotMemoryRequest: memoryRequest,
			DesiredWebReplicas: desiredWeb, QueueConsumerCount: queueConsumers,
			SearchMode: searchMode, SearchReplicas: searchReplicas,
			QueueMode: queueMode, QueueReplicas: queueReplicas,
			EnableCloudArmor: preset != sdk.PresetPreview || class == "production",
		},
		Dependencies: Dependencies{
			DatabaseName: databaseName, MasterUsername: masterUsername, EncryptionKeySecret: gcp.EncryptionKeySecret,
		},
	}
	if err := spec.Validate(); err != nil {
		if options.AllowExpiredPreview && isOnlyExpirationError(err) {
			return spec, nil
		}
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

func isOnlyExpirationError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "preview environment has expired")
}
