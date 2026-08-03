// Package cost implements platform.CostEstimator for the experimental GCP GKE Autopilot target.
package cost

import (
	"context"
	"errors"
	"fmt"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
)

// Estimator is the GCP cost adapter.
type Estimator struct{}

// Estimate reports account-free catalog capacity. Live Catalog API is not wired yet.
func (Estimator) Estimate(ctx context.Context, planned platform.PlannedStack, cfg config.Config, opts platform.CostOptions) (platform.CostReport, error) {
	_ = ctx
	if planned == nil {
		return platform.CostReport{}, errors.New("planned stack is required")
	}
	runtime := string(planned.Runtime())
	if runtime == "" {
		runtime = "gke-autopilot"
	}
	if runtime != "gke-autopilot" {
		return platform.CostReport{}, fmt.Errorf("cost estimation is not available for runtime %q yet", runtime)
	}
	if opts.Live {
		return platform.CostReport{}, errors.New("GCP --live Catalog pricing is not wired yet; use account-free mode (omit --live)")
	}
	return accountFree(cfg, planned.Environment()), nil
}

func accountFree(cfg config.Config, environment string) platform.CostReport {
	preset := cfg.Preset
	if preset == "" {
		preset = cfg.Defaults.Preset
	}
	region := cfg.Defaults.Region
	if cfg.Target.GCP != nil && cfg.Target.GCP.Region != "" {
		region = cfg.Target.GCP.Region
	}
	report := platform.CostReport{
		Environment: environment, Provider: cfg.Target.Provider, Region: region, Preset: preset,
		Mode: "account-free", Currency: "USD", MonthlyBudgetCents: cfg.MonthlyBudgetCents,
		Priced: []platform.CostPricedItem{}, MonthlyTotalCents: nil,
		Notice: "Account-free mode does not call the GCP Catalog API or claim current Google list prices. Capacity is derived from magelift.yaml; Catalog live pricing is optional and not wired yet.",
		Unsupported: []platform.CostUnsupportedItem{
			{Resource: "GCP unit prices", Reason: "current regional prices require live Cloud Billing Catalog data"},
			{Resource: "usage-based services", Reason: "egress, requests, storage growth, logging, and Cloud Armor request volume depend on workload measurements"},
		},
	}
	if cfg.Target.GCP == nil {
		report.Estimated = []platform.CostEstimatedItem{{Resource: "MageLift preset", Configuration: valueOrUnknown(preset) + " topology; exact capacity is not configured"}}
		report.Unsupported = append(report.Unsupported, platform.CostUnsupportedItem{Resource: "service capacity", Reason: "target.gcp is not configured"})
		return report
	}

	gcp := cfg.Target.GCP
	desiredWeb := gcp.DesiredWebReplicas
	if desiredWeb == 0 {
		switch preset {
		case "high-availability":
			desiredWeb = 3
		case "standard":
			desiredWeb = 2
		default:
			desiredWeb = 1
		}
	}
	cpuRequest := gcp.AutopilotCPURequest
	if cpuRequest == "" {
		cpuRequest = "500m"
	}
	memoryRequest := gcp.AutopilotMemoryRequest
	if memoryRequest == "" {
		memoryRequest = "1Gi"
	}
	cloudSQLTier := gcp.CloudSQLTier
	if cloudSQLTier == "" {
		cloudSQLTier = "db-custom-1-3840"
		if preset != "preview" {
			cloudSQLTier = "db-custom-2-7680"
		}
	}
	availability := "ZONAL"
	if preset != "preview" {
		availability = "REGIONAL"
	}
	memorystoreNodeType := gcp.MemorystoreNodeType
	if memorystoreNodeType == "" {
		memorystoreNodeType = "SHARED_CORE_NANO"
		if preset != "preview" {
			memorystoreNodeType = "STANDARD_SMALL"
		}
	}
	memorystoreReplicas := 0
	if preset == "standard" {
		memorystoreReplicas = 1
	}
	if preset == "high-availability" {
		memorystoreReplicas = 2
	}
	queueMode := "database"
	queueConsumers := gcp.QueueConsumerCount
	if preset != "preview" {
		queueMode = "rabbitmq"
		if queueConsumers == 0 {
			queueConsumers = 1
		}
		if preset == "high-availability" && queueConsumers < 2 {
			queueConsumers = 2
		}
	}
	edge := "Cloud Load Balancing HTTPS frontend"
	if preset != "preview" {
		edge += " + Cloud Armor"
	}

	report.Estimated = []platform.CostEstimatedItem{
		{Resource: "GKE Autopilot web", Configuration: fmt.Sprintf("%d replicas, %s CPU, %s memory each", desiredWeb, valueOrUnknown(cpuRequest), valueOrUnknown(memoryRequest))},
		{Resource: "Cloud SQL MySQL", Configuration: fmt.Sprintf("%s, %s", valueOrUnknown(cloudSQLTier), availability)},
		{Resource: "Memorystore for Valkey", Configuration: fmt.Sprintf("%s, %d replicas", valueOrUnknown(memorystoreNodeType), memorystoreReplicas)},
		{Resource: "Edge / load balancer", Configuration: edge},
		queueCostEstimate(queueMode, queueConsumers),
	}
	return report
}

func queueCostEstimate(queueMode string, consumers int) platform.CostEstimatedItem {
	switch queueMode {
	case "database":
		return platform.CostEstimatedItem{Resource: "Magento queue", Configuration: "database-backed; no broker"}
	default:
		return platform.CostEstimatedItem{Resource: "RabbitMQ on GKE Autopilot", Configuration: fmt.Sprintf("%s mode, %d consumer replicas", queueMode, consumers)}
	}
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
