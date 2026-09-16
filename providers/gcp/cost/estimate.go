// Package cost implements platform.CostEstimator for the experimental GCP GKE Autopilot target.
package cost

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	gcpdatabase "github.com/magelift/magelift/providers/gcp/database"
	gcpruntime "github.com/magelift/magelift/providers/gcp/runtime"
	providerschema "github.com/magelift/magelift/providers/gcp/schema"
	"github.com/magelift/magelift/sdk"
)

// Estimator is the GCP cost adapter.
type Estimator struct {
	NewBudgetReader func(context.Context) (BudgetReader, error)
}

// EstimateInputs carries core-resolved cost inputs. Target is nil when the
// target block is absent; the report then explains what is unconfigured.
type EstimateInputs struct {
	Envelope sdk.Envelope
	Target   *providerschema.GCPTarget
	Runtime  string
	Live     bool
	Budget   bool
}

// Estimate reports account-free catalog capacity. Live Catalog API is not wired yet.
func (e Estimator) Estimate(ctx context.Context, in EstimateInputs) (platform.CostReport, error) {
	_ = ctx
	if strings.TrimSpace(in.Envelope.Environment) == "" {
		return platform.CostReport{}, errors.New("deployment envelope is required")
	}
	runtime := in.Runtime
	if runtime == "" {
		runtime = "gke-autopilot"
	}
	if runtime != "gke-autopilot" && runtime != "gke-standard" {
		return platform.CostReport{}, fmt.Errorf("cost estimation is not available for runtime %q yet", runtime)
	}
	if in.Live {
		return platform.CostReport{}, errors.New("GCP --live Catalog pricing is not wired yet; use account-free mode (omit --live)")
	}
	report := accountFree(in)
	if !in.Budget {
		return report, nil
	}
	if in.Target == nil {
		return platform.CostReport{}, errors.New("GCP budget lookup requires target.gcp.project")
	}
	newReader := e.NewBudgetReader
	if newReader == nil {
		newReader = NewBudgetReader
	}
	reader, err := newReader(ctx)
	if err != nil {
		return platform.CostReport{}, fmt.Errorf("create GCP budget reader: %w", err)
	}
	budget, err := reader.Read(ctx, in.Target.Project)
	if err != nil {
		return platform.CostReport{}, err
	}
	report.Budget = &budget
	return report, nil
}

func accountFree(in EstimateInputs) platform.CostReport {
	preset := in.Envelope.Preset
	region := in.Envelope.Region
	if in.Target != nil && in.Target.Region != "" {
		region = in.Target.Region
	}
	report := platform.CostReport{
		Environment: in.Envelope.Environment, Provider: "gcp", Region: region, Preset: preset,
		Mode: "account-free", Currency: "USD", MonthlyBudgetCents: in.Envelope.MonthlyBudgetCents,
		Priced: []platform.CostPricedItem{}, MonthlyTotalCents: nil,
		Notice: "Account-free mode does not call the GCP Catalog API or claim current Google list prices. Capacity is derived from magelift.yaml; Catalog live pricing is optional and not wired yet.",
		Unsupported: []platform.CostUnsupportedItem{
			{Resource: "GCP unit prices", Reason: "current regional prices require live Cloud Billing Catalog data"},
			{Resource: "usage-based services", Reason: "egress, requests, storage growth, logging, and Cloud Armor request volume depend on workload measurements"},
		},
	}
	if in.Target == nil {
		report.Estimated = []platform.CostEstimatedItem{{Resource: "MageLift preset", Configuration: valueOrUnknown(preset) + " topology; exact capacity is not configured"}}
		report.Unsupported = append(report.Unsupported, platform.CostUnsupportedItem{Resource: "service capacity", Reason: "target.gcp is not configured"})
		return report
	}

	gcp := in.Target
	runtime := in.Runtime
	if runtime == "" {
		runtime = "gke-autopilot"
	}
	clusterResource := "GKE Autopilot"
	if runtime == "gke-standard" {
		clusterResource = "GKE Standard"
	}
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
		memoryRequest = gcpruntime.DefaultApplicationMemoryRequest
	}
	cloudSQLTier := gcp.CloudSQLTier
	if cloudSQLTier == "" {
		if databaseVersion, err := gcpdatabase.DatabaseVersionForMagento(in.Envelope.AppVersion); err == nil {
			cloudSQLTier = gcpdatabase.DefaultTier(databaseVersion, preset)
		} else {
			cloudSQLTier = gcpdatabase.DefaultTier(gcpdatabase.DatabaseVersionMySQL80, preset)
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
	if gcp.MemorystoreReplicas != nil {
		memorystoreReplicas = *gcp.MemorystoreReplicas
	} else {
		if preset == "standard" {
			memorystoreReplicas = 1
		}
		if preset == "high-availability" {
			memorystoreReplicas = 2
		}
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
		{Resource: clusterResource + " web", Configuration: fmt.Sprintf("%d replicas, %s CPU, %s memory each", desiredWeb, valueOrUnknown(cpuRequest), valueOrUnknown(memoryRequest))},
		{Resource: "Cloud SQL MySQL", Configuration: fmt.Sprintf("%s, %s", valueOrUnknown(cloudSQLTier), availability)},
		{Resource: "Memorystore for Valkey", Configuration: fmt.Sprintf("%s, %d replicas", valueOrUnknown(memorystoreNodeType), memorystoreReplicas)},
		{Resource: "Edge / load balancer", Configuration: edge},
		queueCostEstimate(queueMode, queueConsumers, clusterResource),
	}
	return report
}

func queueCostEstimate(queueMode string, consumers int, clusterResource string) platform.CostEstimatedItem {
	switch queueMode {
	case "database":
		return platform.CostEstimatedItem{Resource: "Magento queue", Configuration: "database-backed; no broker"}
	default:
		return platform.CostEstimatedItem{Resource: "RabbitMQ on " + clusterResource, Configuration: fmt.Sprintf("%s mode, %d consumer replicas", queueMode, consumers)}
	}
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
