package stack

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	scwtarget "github.com/acourtiol/magelift/internal/cloud/scaleway/target"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/topology"
	sdk "github.com/acourtiol/magelift/sdk/v1"
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
	if cfg.Target.Provider != string(scwtarget.ProviderID) || cfg.Target.Runtime != string(scwtarget.RuntimeID) {
		return Spec{}, fmt.Errorf("unsupported target %q/%q; Scaleway stack requires %q/%q", cfg.Target.Provider, cfg.Target.Runtime, scwtarget.ProviderID, scwtarget.RuntimeID)
	}
	if cfg.Target.Scaleway == nil {
		return Spec{}, fmt.Errorf("target.scaleway is required for Scaleway deployment")
	}
	scw := cfg.Target.Scaleway
	presetName := cfg.Preset
	if strings.TrimSpace(presetName) == "" {
		presetName = cfg.Defaults.Preset
	}
	region := scw.Region
	if strings.TrimSpace(region) == "" {
		region = cfg.Defaults.Region
	}
	if strings.TrimSpace(scw.ProjectID) == "" || strings.TrimSpace(region) == "" || strings.TrimSpace(presetName) == "" {
		return Spec{}, fmt.Errorf("Scaleway project ID, region, and preset must be resolved before deployment")
	}
	zone := scw.Zone
	if strings.TrimSpace(zone) == "" {
		zone = region + "-1"
	}
	preset := sdk.PresetID(presetName)
	class := cfg.Class
	if strings.TrimSpace(class) == "" {
		return Spec{}, fmt.Errorf("environment class must be explicit; it is not inferred from the environment name")
	}
	desiredTopology, err := topology.ForPreset(preset)
	if err != nil {
		return Spec{}, fmt.Errorf("resolve Scaleway target topology: %w", err)
	}
	if err := (scwtarget.Target{}).Validate(context.Background(), sdk.TargetRequest{
		Application:      sdk.Application{Edition: cfg.Application.Edition, Version: cfg.Application.Version, Mode: cfg.Application.Mode},
		EnvironmentClass: class,
		Topology:         desiredTopology,
	}); err != nil {
		return Spec{}, fmt.Errorf("Scaleway target validation failed: %w", err)
	}
	cidr := scw.NetworkCIDR
	if strings.TrimSpace(cidr) == "" {
		cidr = "172.16.0.0/22"
	}
	if _, err := netip.ParsePrefix(cidr); err != nil {
		return Spec{}, fmt.Errorf("target.scaleway.networkCidr: %w", err)
	}
	zones := append([]string(nil), scw.Zones...)
	if len(zones) == 0 {
		zones = []string{zone}
	}
	expiresAt, err := parseExpiration(cfg.ExpiresAt)
	if err != nil {
		return Spec{}, err
	}
	databaseName := scw.DatabaseName
	if databaseName == "" {
		databaseName = "magento"
	}
	masterUsername := scw.MasterUsername
	if masterUsername == "" {
		masterUsername = "magento"
	}
	desiredWeb := scw.DesiredWebReplicas
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
	databaseNodeType := scw.DatabaseNodeType
	if databaseNodeType == "" {
		databaseNodeType = "DB-DEV-S"
		if preset != sdk.PresetPreview {
			databaseNodeType = "DB-GP-S"
		}
	}
	redisNodeType := scw.RedisNodeType
	if redisNodeType == "" {
		redisNodeType = "RED1-MICRO"
		if preset != sdk.PresetPreview {
			redisNodeType = "RED1-S"
		}
	}
	cacheMode := scw.CacheMode
	if cacheMode == "" {
		cacheMode = "redis"
	}
	cpuRequest := scw.CPURequest
	if cpuRequest == "" {
		cpuRequest = "500m"
	}
	memoryRequest := scw.MemoryRequest
	if memoryRequest == "" {
		memoryRequest = "1Gi"
	}
	kapsuleVersion := scw.KapsuleVersion
	if kapsuleVersion == "" {
		kapsuleVersion = "1.29.1"
	}
	nodeType := scw.NodeType
	if nodeType == "" {
		nodeType = "DEV1-M"
	}
	nodeCount := scw.NodeCount
	if nodeCount == 0 {
		nodeCount = 2
	}
	labels := map[string]string{
		"magelift-managed-by":  "magelift",
		"magelift-project":     cfg.Project.Name,
		"magelift-environment": environment,
		"magelift-tier":        "experimental",
	}
	for key, value := range scw.Labels {
		labels[key] = value
	}
	spec := Spec{
		Identity: Identity{
			Project: cfg.Project.Name, ScalewayProject: scw.ProjectID, Environment: environment, Region: region, Zone: zone,
			EnvironmentClass: class, Preset: preset, Labels: labels,
		},
		Application: Application{Edition: cfg.Application.Edition, Version: cfg.Application.Version, Mode: cfg.Application.Mode, WebRuntime: cfg.Application.WebRuntime},
		Artifact:    Artifact{ImageDigest: scw.ImageDigest},
		Lifecycle:   Lifecycle{ExpiresAt: expiresAt, Protection: cfg.Protection},
		Policy:      NetworkPolicy{NetworkCIDR: cidr, Zones: zones},
		Catalog: CatalogSelection{
			DatabaseNodeType: databaseNodeType, RedisNodeType: redisNodeType, CacheMode: cacheMode,
			KapsuleVersion: kapsuleVersion, NodeType: nodeType, NodeCount: nodeCount,
			CPURequest: cpuRequest, MemoryRequest: memoryRequest,
			DesiredWebReplicas: desiredWeb, QueueConsumerCount: scw.QueueConsumerCount,
		},
		Dependencies: Dependencies{
			DatabaseName: databaseName, MasterUsername: masterUsername, EncryptionKeySecret: scw.EncryptionKeySecret,
			StateBucket: scw.StateBucket, StateEndpoint: scw.StateEndpoint, StateRegion: scw.StateRegion,
		},
	}
	if err := spec.Validate(); err != nil {
		if options.AllowExpiredPreview && isOnlyExpirationError(err) {
			return spec, nil
		}
		return Spec{}, fmt.Errorf("Scaleway deployment plan is invalid: %w", err)
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
