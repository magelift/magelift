// Package cost implements platform.CostEstimator for the certified AWS ECS Fargate target.
package cost

import (
	"context"
	"errors"
	"fmt"

	awspricing "github.com/acourtiol/magelift/internal/cloud/aws/pricing"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/platform"
)

// PricingClient looks up current AWS on-demand prices (live mode only).
type PricingClient interface {
	Estimate(context.Context, awspricing.Query) (awspricing.Price, error)
}

// Estimator is the AWS cost adapter.
type Estimator struct {
	// NewPricing builds a live PricingClient; nil uses awspricing.New.
	NewPricing func(context.Context, string) (PricingClient, error)
}

// Estimate reports account-free catalog capacity or live AWS Price List totals.
func (e Estimator) Estimate(ctx context.Context, planned platform.PlannedStack, cfg config.Config, opts platform.CostOptions) (platform.CostReport, error) {
	if planned == nil {
		return platform.CostReport{}, errors.New("planned stack is required")
	}
	runtime := string(planned.Runtime())
	if runtime == "" {
		runtime = "ecs-fargate"
	}
	if runtime != "ecs-fargate" {
		return platform.CostReport{}, fmt.Errorf("cost estimation is not available for runtime %q yet", runtime)
	}
	if !opts.Live {
		return accountFree(cfg, planned.Environment()), nil
	}
	newPricing := e.NewPricing
	if newPricing == nil {
		newPricing = func(ctx context.Context, region string) (PricingClient, error) {
			return awspricing.New(ctx, region)
		}
	}
	client, err := newPricing(ctx, planned.Region())
	if err != nil {
		return platform.CostReport{}, err
	}
	return live(ctx, cfg, planned.Environment(), client)
}

func accountFree(cfg config.Config, environment string) platform.CostReport {
	preset := cfg.Preset
	if preset == "" {
		preset = cfg.Defaults.Preset
	}
	report := platform.CostReport{
		Environment: environment, Provider: cfg.Target.Provider, Region: cfg.Defaults.Region, Preset: preset,
		Mode: "account-free", Currency: "USD", MonthlyBudgetCents: cfg.MonthlyBudgetCents,
		Priced: []platform.CostPricedItem{}, MonthlyTotalCents: nil,
		Notice: "Account-free mode does not call AWS or claim current prices. Capacity is derived from magelift.yaml; use magelift cost --live when current AWS prices are available.",
		Unsupported: []platform.CostUnsupportedItem{
			{Resource: "AWS unit prices", Reason: "current regional prices require live AWS Pricing data"},
			{Resource: "usage-based services", Reason: "data transfer, requests, storage growth, logs, WAF, CloudFront, and NAT processing depend on workload measurements"},
		},
	}
	if cfg.Target.AWS == nil {
		report.Estimated = []platform.CostEstimatedItem{{Resource: "MageLift preset", Configuration: valueOrUnknown(preset) + " topology; exact capacity is not configured"}}
		report.Unsupported = append(report.Unsupported, platform.CostUnsupportedItem{Resource: "service capacity", Reason: "target.aws.catalog is not configured"})
		return report
	}

	catalog := cfg.Target.AWS.Catalog
	report.Estimated = append(report.Estimated,
		platform.CostEstimatedItem{Resource: "ECS Fargate web service", Configuration: fmt.Sprintf("%d tasks, %d CPU units, %d MiB each", catalog.Fargate.DesiredCount, catalog.Fargate.CPU, catalog.Fargate.MemoryMiB)},
		platform.CostEstimatedItem{Resource: "ElastiCache Valkey", Configuration: fmt.Sprintf("%s, %d replicas", valueOrUnknown(catalog.Valkey.NodeType), catalog.Valkey.ReplicaCount)},
		databaseCostEstimate(catalog, preset),
	)
	if item, ok := searchCostEstimate(catalog, preset); ok {
		report.Estimated = append(report.Estimated, item)
	}
	report.Estimated = append(report.Estimated, queueCostEstimate(catalog.QueueMode, preset, catalog.RabbitMQ.InstanceType))
	return report
}

func databaseCostEstimate(catalog config.AWSCatalog, preset string) platform.CostEstimatedItem {
	engine := catalog.DatabaseEngine
	if engine == "" {
		engine = "aurora-mysql"
	}
	switch engine {
	case "rds-mysql":
		return platform.CostEstimatedItem{
			Resource:      "RDS MySQL",
			Configuration: fmt.Sprintf("%s × %d", valueOrUnknown(catalog.Aurora.InstanceClass), maxInt(catalog.Aurora.InstanceCount, 1)),
		}
	default:
		if preset == "preview" && catalog.Aurora.InstanceClass == "" {
			return platform.CostEstimatedItem{Resource: "Aurora Serverless v2", Configuration: fmt.Sprintf("%.2f-%.2f ACU", catalog.Aurora.MinimumACU, catalog.Aurora.MaximumACU)}
		}
		if catalog.Aurora.InstanceCount > 0 && catalog.Aurora.InstanceClass != "" {
			return platform.CostEstimatedItem{Resource: "Aurora MySQL", Configuration: fmt.Sprintf("%d x %s", catalog.Aurora.InstanceCount, valueOrUnknown(catalog.Aurora.InstanceClass))}
		}
		return platform.CostEstimatedItem{Resource: "Aurora Serverless v2", Configuration: fmt.Sprintf("%.2f-%.2f ACU", catalog.Aurora.MinimumACU, catalog.Aurora.MaximumACU)}
	}
}

func searchCostEstimate(catalog config.AWSCatalog, preset string) (platform.CostEstimatedItem, bool) {
	mode := catalog.SearchMode
	if mode == "" {
		if preset == "preview" {
			mode = "serverless"
		} else {
			mode = "provisioned"
		}
	}
	switch mode {
	case "disabled":
		return platform.CostEstimatedItem{}, false
	case "serverless":
		return platform.CostEstimatedItem{Resource: "OpenSearch Serverless", Configuration: fmt.Sprintf("up to %.2f indexing OCU and %.2f search OCU", catalog.Search.MaximumIndexingOCU, catalog.Search.MaximumSearchOCU)}, true
	default:
		if catalog.Search.InstanceCount < 1 || catalog.Search.InstanceType == "" {
			return platform.CostEstimatedItem{}, false
		}
		return platform.CostEstimatedItem{Resource: "OpenSearch", Configuration: fmt.Sprintf("%d x %s data nodes", catalog.Search.InstanceCount, valueOrUnknown(catalog.Search.InstanceType))}, true
	}
}

func queueCostEstimate(queueMode, preset, instanceType string) platform.CostEstimatedItem {
	mode := queueMode
	if mode == "" {
		if preset == "preview" {
			mode = "db"
		} else {
			mode = "amazon-mq"
		}
	}
	switch mode {
	case "db":
		return platform.CostEstimatedItem{Resource: "Magento queue", Configuration: "database-backed; no broker"}
	case "ecs-rabbitmq":
		return platform.CostEstimatedItem{Resource: "ECS RabbitMQ broker", Configuration: "1 Fargate task (512 CPU / 1024 MiB); typically far cheaper than Amazon MQ"}
	case "ecs-artemis":
		return platform.CostEstimatedItem{Resource: "ECS Artemis broker", Configuration: "1 Fargate task (512 CPU / 1024 MiB); experimental Magento AMQP path"}
	default:
		return platform.CostEstimatedItem{Resource: "Amazon MQ for RabbitMQ", Configuration: "3 x " + valueOrUnknown(instanceType)}
	}
}

func live(ctx context.Context, cfg config.Config, environment string, estimator PricingClient) (platform.CostReport, error) {
	report := accountFree(cfg, environment)
	report.Mode = "live"
	report.Priced = []platform.CostPricedItem{}
	report.Estimated = liveUnpricedEstimates(cfg)
	report.Unsupported = []platform.CostUnsupportedItem{
		{Resource: "usage-based services", Reason: "data transfer, requests, storage growth, logs, WAF, CloudFront, and NAT processing depend on workload measurements"},
	}
	if cfg.Target.AWS == nil {
		return report, errors.New("target.aws is required for live pricing")
	}
	queries := livePriceQueries(cfg)
	if len(queries) == 0 {
		return report, errors.New("the selected catalog has no priceable capacity inputs")
	}
	var total int64
	currency := "USD"
	for _, query := range queries {
		price, err := estimator.Estimate(ctx, query)
		if err != nil {
			if errors.Is(err, awspricing.ErrNoPrice) {
				report.Unsupported = append(report.Unsupported, platform.CostUnsupportedItem{Resource: query.Resource, Reason: err.Error()})
				continue
			}
			return report, err
		}
		total += price.MonthlyCents
		if price.Currency != "" {
			currency = price.Currency
		}
		report.Priced = append(report.Priced, platform.CostPricedItem{Resource: price.Resource, MonthlyCents: price.MonthlyCents, Basis: price.Basis})
	}
	report.Currency = currency
	if len(report.Priced) > 0 {
		report.MonthlyTotalCents = &total
	}
	if len(report.Unsupported) == 1 {
		report.Notice = "Live mode queried AWS on-demand prices for the selected capacity. Usage-based charges remain unsupported until workload measurements are available."
	} else {
		report.Notice = "Live mode queried AWS on-demand prices, but some selected capacities had no matching product. Review unsupported items before treating the total as a budget."
	}
	return report, nil
}

func liveUnpricedEstimates(cfg config.Config) []platform.CostEstimatedItem {
	if cfg.Target.AWS == nil {
		return nil
	}
	catalog := cfg.Target.AWS.Catalog
	preset := cfg.Preset
	if preset == "" {
		preset = cfg.Defaults.Preset
	}
	items := []platform.CostEstimatedItem{
		{Resource: "CloudWatch, S3, CloudFront, WAF, and NAT", Configuration: "usage-based charges require workload measurements"},
	}
	db := databaseCostEstimate(catalog, preset)
	db.Configuration += "; workload utilization is not priced"
	items = append(items, db)
	if search, ok := searchCostEstimate(catalog, preset); ok {
		search.Configuration += "; workload utilization is not priced"
		items = append(items, search)
	}
	items = append(items, queueCostEstimate(catalog.QueueMode, preset, catalog.RabbitMQ.InstanceType))
	return items
}

func livePriceQueries(cfg config.Config) []awspricing.Query {
	catalog := cfg.Target.AWS.Catalog
	queries := make([]awspricing.Query, 0, 8)
	if catalog.Fargate.DesiredCount > 0 && catalog.Fargate.CPU > 0 {
		queries = append(queries, awspricing.Query{
			Resource: "ECS Fargate vCPU", ServiceCode: "AmazonECS", UsageTypeContains: "Fargate-vCPU-Hours",
			Filters:  map[string]string{"productFamily": "Compute", "operatingSystem": "Linux", "preInstalledSw": "NA", "capacitystatus": "Used"},
			Quantity: float64(catalog.Fargate.DesiredCount) * float64(catalog.Fargate.CPU) / 1024, Basis: fmt.Sprintf("%d tasks × %.2f vCPU × 730 hours", catalog.Fargate.DesiredCount, float64(catalog.Fargate.CPU)/1024),
		})
	}
	if catalog.Fargate.DesiredCount > 0 && catalog.Fargate.MemoryMiB > 0 {
		queries = append(queries, awspricing.Query{
			Resource: "ECS Fargate memory", ServiceCode: "AmazonECS", UsageTypeContains: "Fargate-GB-Hours",
			Filters:  map[string]string{"productFamily": "Compute", "operatingSystem": "Linux", "preInstalledSw": "NA", "capacitystatus": "Used"},
			Quantity: float64(catalog.Fargate.DesiredCount) * float64(catalog.Fargate.MemoryMiB) / 1024, Basis: fmt.Sprintf("%d tasks × %.2f GiB × 730 hours", catalog.Fargate.DesiredCount, float64(catalog.Fargate.MemoryMiB)/1024),
		})
	}
	if catalog.Valkey.NodeType != "" {
		queries = append(queries, awspricing.Query{
			Resource: "ElastiCache Valkey", ServiceCode: "AmazonElastiCache", UsageTypeContains: "NodeUsage",
			Filters:  map[string]string{"productFamily": "ElastiCache Instance", "cacheEngine": "Valkey", "instanceType": catalog.Valkey.NodeType},
			Quantity: float64(catalog.Valkey.ReplicaCount + 1), Basis: fmt.Sprintf("%d nodes × 730 hours", catalog.Valkey.ReplicaCount+1),
		})
	}
	if catalog.Aurora.InstanceClass != "" && catalog.Aurora.InstanceCount > 0 {
		engine := catalog.DatabaseEngine
		if engine == "" {
			engine = "aurora-mysql"
		}
		if engine == "rds-mysql" {
			queries = append(queries, awspricing.Query{
				Resource: "RDS MySQL", ServiceCode: "AmazonRDS", UsageTypeContains: "InstanceUsage",
				Filters:  map[string]string{"productFamily": "Database Instance", "databaseEngine": "MySQL", "instanceType": catalog.Aurora.InstanceClass},
				Quantity: float64(catalog.Aurora.InstanceCount), Basis: fmt.Sprintf("%d instances × 730 hours", catalog.Aurora.InstanceCount),
			})
		} else {
			queries = append(queries, awspricing.Query{
				Resource: "Aurora MySQL", ServiceCode: "AmazonRDS", UsageTypeContains: "InstanceUsage",
				Filters:  map[string]string{"productFamily": "Database Instance", "databaseEngine": "Aurora MySQL", "instanceType": catalog.Aurora.InstanceClass},
				Quantity: float64(catalog.Aurora.InstanceCount), Basis: fmt.Sprintf("%d instances × 730 hours", catalog.Aurora.InstanceCount),
			})
		}
	}
	if catalog.Search.InstanceType != "" && catalog.Search.InstanceCount > 0 && catalog.SearchMode != "disabled" {
		queries = append(queries, awspricing.Query{
			Resource: "OpenSearch data nodes", ServiceCode: "AmazonES", UsageTypeContains: "InstanceUsage",
			Filters:  map[string]string{"productFamily": "Amazon OpenSearch Service", "instanceType": catalog.Search.InstanceType},
			Quantity: float64(catalog.Search.InstanceCount), Basis: fmt.Sprintf("%d data nodes × 730 hours", catalog.Search.InstanceCount),
		})
	}
	if catalog.RabbitMQ.InstanceType != "" {
		mode := catalog.QueueMode
		if mode == "" {
			preset := cfg.Preset
			if preset == "" {
				preset = cfg.Defaults.Preset
			}
			if preset == "preview" {
				mode = "db"
			} else {
				mode = "amazon-mq"
			}
		}
		if mode == "amazon-mq" {
			queries = append(queries, awspricing.Query{
				Resource: "Amazon MQ for RabbitMQ", ServiceCode: "AmazonMQ", UsageTypeContains: "BrokerUsage",
				Filters:  map[string]string{"productFamily": "RabbitMQ Broker", "instanceType": catalog.RabbitMQ.InstanceType},
				Quantity: 3, Basis: "3 brokers × 730 hours",
			})
		}
	}
	return queries
}

func maxInt(v, floor int) int {
	if v < floor {
		return floor
	}
	return v
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
