// Package cost implements platform.CostEstimator for the experimental
// Scaleway Kapsule target.
package cost

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
)

// Estimator is the Scaleway cost adapter.
type Estimator struct{}

// Estimate reports account-free catalog capacity. Scaleway live pricing is
// deliberately not wired yet; --live must not fall back to this report.
func (Estimator) Estimate(ctx context.Context, planned platform.PlannedStack, cfg config.Config, opts platform.CostOptions) (platform.CostReport, error) {
	_ = ctx
	if planned == nil {
		return platform.CostReport{}, errors.New("planned stack is required")
	}
	runtime := string(planned.Runtime())
	if runtime == "" {
		runtime = "kapsule"
	}
	if runtime != "kapsule" {
		return platform.CostReport{}, fmt.Errorf("cost estimation is not available for runtime %q yet", runtime)
	}
	if opts.Live {
		return platform.CostReport{}, errors.New("Scaleway --live pricing is not implemented; use account-free mode (omit --live)")
	}
	report := accountFree(cfg, planned.Environment(), planned.Region())
	if opts.Budget {
		report.Budget = unavailableBudget(planned, cfg)
	}
	return report, nil
}

func unavailableBudget(planned platform.PlannedStack, cfg config.Config) *platform.CostBudgetReport {
	scope := "scaleway/environment/" + planned.Environment()
	if cfg.Target.Scaleway != nil && cfg.Target.Scaleway.ProjectID != "" {
		scope = "scaleway/project/" + cfg.Target.Scaleway.ProjectID
	}
	return platform.UnavailableCostBudgetReport(
		scope,
		"Scaleway budget adapter",
		"Scaleway budget lookup is not wired yet; monthlyBudgetCents is a MageLift planning input, not a provider budget definition",
	)
}

func accountFree(cfg config.Config, environment, plannedRegion string) platform.CostReport {
	provider := cfg.Target.Provider
	if provider == "" {
		provider = "scaleway"
	}
	region := strings.TrimSpace(plannedRegion)
	if region == "" && cfg.Target.Scaleway != nil {
		region = strings.TrimSpace(cfg.Target.Scaleway.Region)
	}
	if region == "" {
		region = strings.TrimSpace(cfg.Defaults.Region)
	}
	preset := cfg.Preset
	if preset == "" {
		preset = cfg.Defaults.Preset
	}
	report := platform.CostReport{
		Environment:        environment,
		Provider:           provider,
		Region:             region,
		Preset:             preset,
		Mode:               "account-free",
		Currency:           "EUR",
		MonthlyBudgetCents: cfg.MonthlyBudgetCents,
		Priced:             []platform.CostPricedItem{},
		MonthlyTotalCents:  nil,
		Notice:             "Account-free mode does not call Scaleway APIs or claim current prices. Capacity is derived from magelift.yaml; Scaleway live pricing is not wired yet.",
		Unsupported: []platform.CostUnsupportedItem{
			{Resource: "Scaleway unit prices", Reason: "current regional prices require live Scaleway pricing data"},
			{Resource: "usage-based services", Reason: "data transfer, requests, storage growth, logs, load-balancer traffic, and Cockpit usage depend on workload measurements"},
			{Resource: "search service", Reason: "Scaleway has no managed search product in the current target catalog"},
		},
	}
	if cfg.Target.Scaleway == nil {
		report.Estimated = []platform.CostEstimatedItem{{
			Resource:      "MageLift preset",
			Configuration: valueOrUnknown(preset) + " topology; exact Scaleway capacity is not configured",
		}}
		report.Unsupported = append(report.Unsupported, platform.CostUnsupportedItem{
			Resource: "service capacity",
			Reason:   "target.scaleway is not configured",
		})
		return report
	}

	scw := cfg.Target.Scaleway
	webReplicas := scw.DesiredWebReplicas
	if webReplicas == 0 {
		webReplicas = defaultWebReplicas(preset)
	}
	cpuRequest := valueOrDefault(scw.CPURequest, "500m")
	memoryRequest := valueOrDefault(scw.MemoryRequest, kube.DefaultApplicationMemoryRequest)
	kapsuleVersion := valueOrDefault(scw.KapsuleVersion, "1.36.1")
	nodeType := valueOrDefault(scw.NodeType, "DEV1-M")
	nodeCount := scw.NodeCount
	if nodeCount == 0 {
		nodeCount = 2
	}
	zones := configuredZones(scw, region)
	databaseNodeType := valueOrDefault(scw.DatabaseNodeType, defaultDatabaseNodeType(preset))
	redisNodeType := valueOrDefault(scw.RedisNodeType, defaultRedisNodeType(preset))
	redisVersion := valueOrDefault(scw.RedisVersion, "8.6.3")
	redisClusterSize := scw.RedisClusterSize
	if redisClusterSize == 0 {
		redisClusterSize = defaultRedisClusterSize(preset)
	}

	report.Estimated = []platform.CostEstimatedItem{
		{
			Resource:      "Scaleway Kapsule control plane",
			Configuration: fmt.Sprintf("1 cluster, Kubernetes %s, %d availability zones", kapsuleVersion, len(zones)),
		},
		{
			Resource:      "Scaleway Kapsule worker pool",
			Configuration: fmt.Sprintf("%d x %s nodes across %s", nodeCount, nodeType, strings.Join(zones, ", ")),
		},
		{
			Resource:      "Kapsule web workload",
			Configuration: fmt.Sprintf("%d replicas, %s CPU, %s memory each", webReplicas, cpuRequest, memoryRequest),
		},
		{
			Resource:      "Scaleway Managed Database for MySQL",
			Configuration: fmt.Sprintf("%s, %s", databaseNodeType, availabilityLabel(scw.DatabaseHighAvailability)),
		},
		{
			Resource:      "Scaleway Managed Redis",
			Configuration: fmt.Sprintf("%s, %d nodes, Redis %s", redisNodeType, redisClusterSize, redisVersion),
		},
		{
			Resource:      "Scaleway load balancer",
			Configuration: "Kubernetes Service type LoadBalancer",
		},
		{
			Resource:      "Magento queue",
			Configuration: fmt.Sprintf("database-backed, %d consumer replicas", scw.QueueConsumerCount),
		},
	}
	return report
}

func configuredZones(target *config.ScalewayTarget, region string) []string {
	if len(target.Zones) > 0 {
		return append([]string(nil), target.Zones...)
	}
	zone := strings.TrimSpace(target.Zone)
	if zone == "" && region != "" {
		zone = region + "-1"
	}
	if zone == "" {
		return []string{"unknown"}
	}
	return []string{zone}
}

func defaultWebReplicas(preset string) int {
	switch preset {
	case "high-availability":
		return 3
	case "standard":
		return 2
	default:
		return 1
	}
}

func defaultDatabaseNodeType(preset string) string {
	if preset == "preview" {
		return "DB-DEV-S"
	}
	return "DB-GP-S"
}

func defaultRedisNodeType(preset string) string {
	if preset == "preview" {
		return "RED1-MICRO"
	}
	return "RED1-S"
}

func defaultRedisClusterSize(preset string) int {
	if preset == "high-availability" {
		return 2
	}
	return 1
}

func availabilityLabel(highAvailability bool) string {
	if highAvailability {
		return "high availability"
	}
	return "single instance"
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}
