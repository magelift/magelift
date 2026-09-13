// Package cost implements platform.CostEstimator for the experimental OVHcloud MKS target.
package cost

import (
	"context"
	"errors"
	"fmt"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
)

const mksRuntime = "mks"

// Estimator is the OVHcloud cost adapter. Account-free mode reports the
// selected capacity without contacting OVHcloud; live pricing is not wired.
type Estimator struct{}

// Estimate reports account-free OVHcloud MKS capacity. Current OVHcloud
// pricing is deliberately not queried until a provider price-list adapter
// exists, so --live fails before any provider client could be constructed.
func (Estimator) Estimate(ctx context.Context, planned platform.PlannedStack, cfg config.Config, opts platform.CostOptions) (platform.CostReport, error) {
	_ = ctx
	if planned == nil {
		return platform.CostReport{}, errors.New("planned stack is required")
	}
	runtime := string(planned.Runtime())
	if runtime == "" {
		runtime = mksRuntime
	}
	if runtime != mksRuntime {
		return platform.CostReport{}, fmt.Errorf("cost estimation is not available for runtime %q yet", runtime)
	}
	if opts.Live {
		return platform.CostReport{}, errors.New("OVH --live pricing is not wired yet; use account-free mode (omit --live)")
	}
	report := accountFree(planned, cfg)
	if opts.Budget {
		report.Budget = unavailableBudget(planned, cfg)
	}
	return report, nil
}

func unavailableBudget(planned platform.PlannedStack, cfg config.Config) *platform.CostBudgetReport {
	scope := "ovh/environment/" + planned.Environment()
	if cfg.Target.OVH != nil && cfg.Target.OVH.ServiceName != "" {
		scope = "ovh/service/" + cfg.Target.OVH.ServiceName
	}
	return platform.UnavailableCostBudgetReport(
		scope,
		"OVHcloud budget adapter",
		"OVHcloud budget lookup is not wired yet; monthlyBudgetCents is a MageLift planning input, not a provider budget definition",
	)
}

func accountFree(planned platform.PlannedStack, cfg config.Config) platform.CostReport {
	preset := cfg.Preset
	if preset == "" {
		preset = cfg.Defaults.Preset
	}
	provider := cfg.Target.Provider
	if provider == "" {
		provider = string(planned.Provider())
	}
	region := planned.Region()
	if region == "" && cfg.Target.OVH != nil {
		region = cfg.Target.OVH.Region
	}
	if region == "" {
		region = cfg.Defaults.Region
	}
	report := platform.CostReport{
		Environment:        planned.Environment(),
		Provider:           provider,
		Region:             region,
		Preset:             preset,
		Mode:               "account-free",
		Currency:           "EUR",
		MonthlyBudgetCents: cfg.MonthlyBudgetCents,
		Priced:             []platform.CostPricedItem{},
		MonthlyTotalCents:  nil,
		Notice:             "Account-free mode does not call OVHcloud or claim current prices. Capacity is derived from magelift.yaml; OVHcloud live pricing is not wired yet.",
		Unsupported: []platform.CostUnsupportedItem{
			{Resource: "OVHcloud unit prices", Reason: "current regional prices require live OVHcloud pricing data"},
			{Resource: "usage-based services", Reason: "egress, load-balancer traffic, object storage, backups, and logs depend on workload measurements"},
		},
	}
	if cfg.Target.OVH == nil {
		report.Estimated = []platform.CostEstimatedItem{{
			Resource:      "MageLift preset",
			Configuration: valueOrUnknown(preset) + " topology; exact capacity is not configured",
		}}
		report.Unsupported = append(report.Unsupported, platform.CostUnsupportedItem{
			Resource: "service capacity",
			Reason:   "target.ovh is not configured",
		})
		return report
	}

	ovh := cfg.Target.OVH
	webReplicas, workerNodes, databaseNodes, valkeyNodes, queueConsumers := defaultsForPreset(preset)
	if ovh.DesiredWebReplicas > 0 {
		webReplicas = ovh.DesiredWebReplicas
	}
	if ovh.NodeCount > 0 {
		workerNodes = ovh.NodeCount
	}
	if ovh.DatabaseNodeCount > 0 {
		databaseNodes = ovh.DatabaseNodeCount
	}
	if ovh.ValkeyNodeCount > 0 {
		valkeyNodes = ovh.ValkeyNodeCount
	}
	if ovh.QueueConsumerCount > 0 {
		queueConsumers = ovh.QueueConsumerCount
	}

	mksPlan := ovh.MKSPlan
	if mksPlan == "" {
		mksPlan = "standard"
	}
	nodeFlavor := ovh.NodeFlavor
	if nodeFlavor == "" {
		nodeFlavor = "b3-8"
	}
	databaseFlavor := ovh.DatabaseFlavor
	if databaseFlavor == "" {
		databaseFlavor = "b3-8"
	}
	valkeyFlavor := ovh.ValkeyFlavor
	if valkeyFlavor == "" {
		valkeyFlavor = "b3-8"
	}
	databasePlan := ovh.DatabasePlan
	if databasePlan == "" {
		databasePlan = defaultManagedServicePlan(preset)
	}
	valkeyPlan := ovh.ValkeyPlan
	if valkeyPlan == "" {
		valkeyPlan = defaultManagedServicePlan(preset)
	}
	databaseVersion := ovh.DatabaseVersion
	if databaseVersion == "" {
		databaseVersion = "8.4"
	}
	valkeyVersion := ovh.ValkeyVersion
	if valkeyVersion == "" {
		valkeyVersion = "8.1"
	}
	zoneCount := len(ovh.Zones)
	if zoneCount == 0 {
		zoneCount = 1
	}

	cpuRequest := ovh.CPURequest
	if cpuRequest == "" {
		cpuRequest = "500m"
	}
	memoryRequest := ovh.MemoryRequest
	if memoryRequest == "" {
		memoryRequest = "1Gi"
	}
	workerConfiguration := fmt.Sprintf("%d x %s across %d availability zone(s); %d web replicas at %s CPU / %s memory each", workerNodes, nodeFlavor, zoneCount, webReplicas, cpuRequest, memoryRequest)
	if ovh.AttachFloatingIPs {
		workerConfiguration += "; floating IPs attached"
	}
	report.Estimated = []platform.CostEstimatedItem{
		{Resource: "OVH MKS control plane", Configuration: mksPlan + " plan"},
		{Resource: "OVH MKS worker nodes", Configuration: workerConfiguration},
		{Resource: "OVH Managed MySQL", Configuration: fmt.Sprintf("%s, %s plan, %s, %d node(s)", databaseVersion, databasePlan, databaseFlavor, databaseNodes)},
		{Resource: "OVH Managed Valkey", Configuration: fmt.Sprintf("%s, %s plan, %s, %d node(s)", valkeyVersion, valkeyPlan, valkeyFlavor, valkeyNodes)},
		{Resource: "OVH Kubernetes LoadBalancer", Configuration: "Kubernetes Service type LoadBalancer"},
		{Resource: "Magento queue", Configuration: queueConfiguration(queueConsumers)},
	}
	return report
}

func defaultsForPreset(preset string) (webReplicas, workerNodes, databaseNodes, valkeyNodes, queueConsumers int) {
	webReplicas = 1
	workerNodes = 1
	databaseNodes = 1
	valkeyNodes = 1
	if preset != "preview" && preset != "" {
		webReplicas = 2
		workerNodes = 2
		databaseNodes = 2
		valkeyNodes = 2
		queueConsumers = 1
	}
	if preset == "high-availability" {
		webReplicas = 3
		queueConsumers = 2
	}
	return webReplicas, workerNodes, databaseNodes, valkeyNodes, queueConsumers
}

func defaultManagedServicePlan(preset string) string {
	if preset == "preview" || preset == "" {
		return "discovery"
	}
	return "production"
}

func queueConfiguration(consumers int) string {
	if consumers == 0 {
		return "database-backed; no broker"
	}
	return fmt.Sprintf("database-backed; %d consumer replicas", consumers)
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
