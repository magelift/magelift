package gcpprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type AdmissionSelection struct {
	ProjectID          string
	Region             string
	Zones              []string
	RequiredServices   []string
	RequiredQuotas     map[string]float64
	CloudSQLVersion    string
	CloudSQLTier       string
	MemorystoreVersion string
	MemorystoreNode    string
	MemorystoreMode    string
	MemorystoreZone    string
	KubernetesVersion  string
	ReleaseChannel     string
	MachineType        string
	StandardNodeCount  int
	StandardNodeMax    int
	Runtime            string
}

// ValidateAccountPrep checks the project identity, billing, and the
// storage API bootstrap needs for the state bucket. It does not query
// Cloud SQL, Memorystore, or GKE catalogs; those belong to deploy.
func ValidateAccountPrep(ctx context.Context, client CapabilityAPI, selection AdmissionSelection) error {
	if ctx == nil {
		return errors.New("GCP plan admission context is required")
	}
	if client == nil {
		return errors.New("GCP capability client is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	projectID := strings.TrimSpace(selection.ProjectID)
	if projectID == "" {
		return errors.New("GCP plan admission project is required")
	}
	identity, err := client.Project(ctx, projectID)
	if err != nil {
		return fmt.Errorf("read GCP project identity: %w", err)
	}
	if strings.TrimSpace(identity.ID) != projectID {
		return fmt.Errorf("GCP credentials resolved to project %q, but the plan targets %q", identity.ID, projectID)
	}
	billingEnabled, err := client.BillingEnabled(ctx, projectID)
	if err != nil {
		return fmt.Errorf("check GCP billing account: %w", err)
	}
	if !billingEnabled {
		return fmt.Errorf("GCP project %q has no open billing account", projectID)
	}
	const storageAPI = "storage.googleapis.com"
	services, err := client.EnabledServices(ctx, identity, []string{storageAPI})
	if err != nil {
		return fmt.Errorf("check GCP service states: %w", err)
	}
	if !services[storageAPI] {
		return fmt.Errorf("GCP service %q is not enabled for project %q", storageAPI, projectID)
	}
	return nil
}

func ValidateSelection(ctx context.Context, client CapabilityAPI, selection AdmissionSelection) error {
	if ctx == nil {
		return errors.New("GCP plan admission context is required")
	}
	if client == nil {
		return errors.New("GCP capability client is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	projectID, region := strings.TrimSpace(selection.ProjectID), strings.TrimSpace(selection.Region)
	if projectID == "" || region == "" {
		return errors.New("GCP plan admission project and region are required")
	}
	identity, err := client.Project(ctx, projectID)
	if err != nil {
		return fmt.Errorf("read GCP project identity: %w", err)
	}
	if strings.TrimSpace(identity.ID) != projectID {
		return fmt.Errorf("GCP credentials resolved to project %q, but the plan targets %q", identity.ID, projectID)
	}
	billingEnabled, err := client.BillingEnabled(ctx, projectID)
	if err != nil {
		return fmt.Errorf("check GCP billing account: %w", err)
	}
	if !billingEnabled {
		return fmt.Errorf("GCP project %q has no open billing account", projectID)
	}
	if len(selection.RequiredServices) > 0 {
		services, err := client.EnabledServices(ctx, identity, selection.RequiredServices)
		if err != nil {
			return fmt.Errorf("check GCP service states: %w", err)
		}
		for _, serviceName := range selection.RequiredServices {
			serviceName = strings.TrimSpace(serviceName)
			if serviceName == "" {
				return errors.New("GCP plan contains an empty required service")
			}
			if !services[serviceName] {
				return fmt.Errorf("GCP service %q is not enabled for project %q", serviceName, projectID)
			}
		}
	}
	availableZones, err := client.RegionZones(ctx, projectID, region)
	if err != nil {
		return fmt.Errorf("read GCP region zones: %w", err)
	}
	if err := validateZones(selection.Zones, availableZones); err != nil {
		return err
	}
	for metric, required := range selection.RequiredQuotas {
		metric = strings.TrimSpace(metric)
		if metric == "" || required < 0 {
			return fmt.Errorf("GCP quota requirement %q is invalid", metric)
		}
		available, err := client.RegionQuota(ctx, projectID, region, metric)
		if err != nil {
			return fmt.Errorf("check GCP regional quota %q: %w", metric, err)
		}
		if available < required {
			return fmt.Errorf("GCP regional quota %q has %.2f available, %.2f required", metric, available, required)
		}
	}

	version := strings.TrimSpace(selection.CloudSQLVersion)
	if version == "" {
		return errors.New("GCP Cloud SQL database version is required")
	}
	available, err := client.CloudSQLDatabaseVersion(ctx, projectID, version)
	if err != nil {
		return fmt.Errorf("check GCP Cloud SQL database version %q: %w", version, err)
	}
	if !available {
		return fmt.Errorf("GCP Cloud SQL database version %q is not supported by MageLift's current provider catalog", version)
	}
	tier := strings.TrimSpace(selection.CloudSQLTier)
	if tier == "" {
		return errors.New("GCP Cloud SQL tier is required")
	}
	available, err = client.CloudSQLTier(ctx, projectID, region, tier)
	if err != nil {
		return fmt.Errorf("check GCP Cloud SQL tier %q: %w", tier, err)
	}
	if !available {
		return fmt.Errorf("GCP Cloud SQL tier %q is not available in %q", tier, region)
	}

	memorystoreVersion := strings.TrimSpace(selection.MemorystoreVersion)
	memorystoreNode := strings.TrimSpace(selection.MemorystoreNode)
	if memorystoreVersion == "" || memorystoreNode == "" {
		return errors.New("GCP Memorystore admission requires engine version and node type")
	}
	available, err = client.MemorystoreValkey(ctx, projectID, region, memorystoreVersion, memorystoreNode, selection.MemorystoreMode)
	if err != nil {
		return fmt.Errorf("check GCP Memorystore Valkey catalog: %w", err)
	}
	if !available {
		return fmt.Errorf("GCP Memorystore Valkey shape %q/%q is not available in %q", memorystoreVersion, memorystoreNode, region)
	}
	if zone := strings.TrimSpace(selection.MemorystoreZone); zone != "" && !containsName(availableZones, zone) {
		return fmt.Errorf("GCP Memorystore zone %q is not available in %q", zone, region)
	}

	if runtime := strings.TrimSpace(selection.Runtime); runtime == "" || runtime == "gke-autopilot" || runtime == "gke-standard" {
		available, err = client.KubernetesVersion(ctx, projectID, region, selection.ReleaseChannel, selection.KubernetesVersion)
		if err != nil {
			return fmt.Errorf("check GCP GKE Kubernetes version: %w", err)
		}
		if !available {
			return fmt.Errorf("GCP GKE Kubernetes version %q is not available in %q for release channel %q", selection.KubernetesVersion, region, selection.ReleaseChannel)
		}
		if runtime == "gke-standard" {
			machineType := strings.TrimSpace(selection.MachineType)
			if machineType == "" {
				return errors.New("GCP Standard GKE machine type is required")
			}
			nodeCount := selection.StandardNodeMax
			if nodeCount <= 0 {
				nodeCount = selection.StandardNodeCount
			}
			if nodeCount <= 0 {
				return errors.New("GCP Standard GKE node count is required for quota admission")
			}
			cpuCount, err := client.MachineTypeCPUs(ctx, projectID, selection.Zones[0], machineType)
			if err != nil {
				return fmt.Errorf("check GCP machine type %q CPU capacity: %w", machineType, err)
			}
			availableCPU, err := client.RegionQuota(ctx, projectID, region, "CPUS")
			if err != nil {
				return fmt.Errorf("check GCP regional CPU quota: %w", err)
			}
			requiredCPU := float64(cpuCount * int64(nodeCount))
			if availableCPU < requiredCPU {
				return fmt.Errorf("GCP regional CPU quota has %.2f available, %.2f required for %d %s nodes", availableCPU, requiredCPU, nodeCount, machineType)
			}
			for _, zone := range selection.Zones {
				available, err := client.MachineType(ctx, projectID, zone, machineType)
				if err != nil {
					return fmt.Errorf("check GCP machine type %q in %q: %w", machineType, zone, err)
				}
				if !available {
					return fmt.Errorf("GCP machine type %q is not available in %q", machineType, zone)
				}
			}
		}
	}
	return nil
}

func validateZones(selected, available []string) error {
	if len(selected) == 0 {
		return errors.New("GCP plan admission requires at least one zone")
	}
	availableSet := make(map[string]struct{}, len(available))
	for _, zone := range available {
		zone = strings.TrimSpace(zone)
		if zone != "" {
			availableSet[zone] = struct{}{}
		}
	}
	if len(availableSet) == 0 {
		return errors.New("GCP returned no region zones")
	}
	seen := make(map[string]struct{}, len(selected))
	for _, zone := range selected {
		zone = strings.TrimSpace(zone)
		if zone == "" {
			return errors.New("GCP plan contains a blank zone")
		}
		if _, ok := seen[zone]; ok {
			return fmt.Errorf("GCP plan repeats zone %q", zone)
		}
		seen[zone] = struct{}{}
		if _, ok := availableSet[zone]; !ok {
			return fmt.Errorf("GCP zone %q is not available", zone)
		}
	}
	return nil
}

func containsName(values []string, wanted string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}
