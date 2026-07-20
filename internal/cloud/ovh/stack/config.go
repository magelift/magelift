package stack

import (
	"context"
	"fmt"
	"net/netip"
	"strings"
	"time"

	ovhtarget "github.com/acourtiol/magelift/internal/cloud/ovh/target"
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
		dbFlavor = "db1-4"
	}
	dbPlan := ovh.DatabasePlan
	if dbPlan == "" {
		dbPlan = "essential"
	}
	valkeyFlavor := ovh.ValkeyFlavor
	if valkeyFlavor == "" {
		valkeyFlavor = "db1-4"
	}
	valkeyPlan := ovh.ValkeyPlan
	if valkeyPlan == "" {
		valkeyPlan = "essential"
	}
	nodeFlavor := ovh.NodeFlavor
	if nodeFlavor == "" {
		nodeFlavor = "b3-8"
	}
	nodeCount := ovh.NodeCount
	if nodeCount == 0 {
		nodeCount = 1
		if preset != sdk.PresetPreview {
			nodeCount = 2
		}
	}
	cpuRequest := ovh.CPURequest
	if cpuRequest == "" {
		cpuRequest = "500m"
	}
	memoryRequest := ovh.MemoryRequest
	if memoryRequest == "" {
		memoryRequest = "1Gi"
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
			Project: cfg.Project.Name, ServiceName: ovh.ServiceName, Environment: environment, Region: region,
			EnvironmentClass: class, Preset: preset, Labels: labels,
		},
		Application: Application{Edition: cfg.Application.Edition, Version: cfg.Application.Version, Mode: cfg.Application.Mode, WebRuntime: cfg.Application.WebRuntime},
		Artifact:    Artifact{ImageDigest: ovh.ImageDigest},
		Lifecycle:   Lifecycle{ExpiresAt: expiresAt, Protection: cfg.Protection},
		Policy:      NetworkPolicy{NetworkCIDR: cidr, Zones: zones},
		Catalog: CatalogSelection{
			DatabaseFlavor: dbFlavor, DatabasePlan: dbPlan, ValkeyFlavor: valkeyFlavor, ValkeyPlan: valkeyPlan,
			NodeFlavor: nodeFlavor, NodeCount: nodeCount, CPURequest: cpuRequest, MemoryRequest: memoryRequest,
			DesiredWebReplicas: desiredWeb, QueueConsumerCount: ovh.QueueConsumerCount,
		},
		Dependencies: Dependencies{
			DatabaseName: databaseName, MasterUsername: masterUsername, EncryptionKeySecret: ovh.EncryptionKeySecret,
		},
	}
	if err := spec.Validate(); err != nil {
		if options.AllowExpiredPreview && isOnlyExpirationError(err) {
			return spec, nil
		}
		return Spec{}, fmt.Errorf("OVH deployment plan is invalid: %w", err)
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
