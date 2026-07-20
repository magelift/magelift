package cli

import (
	"context"
	"errors"
	"fmt"

	awspricing "github.com/acourtiol/magelift/internal/cloud/aws/pricing"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/spf13/cobra"
)

type costEstimator interface {
	Estimate(context.Context, awspricing.Query) (awspricing.Price, error)
}

type costReport struct {
	Environment        string                `json:"environment" yaml:"environment"`
	Provider           string                `json:"provider" yaml:"provider"`
	Region             string                `json:"region" yaml:"region"`
	Preset             string                `json:"preset" yaml:"preset"`
	Mode               string                `json:"mode" yaml:"mode"`
	Currency           string                `json:"currency" yaml:"currency"`
	MonthlyBudgetCents int64                 `json:"monthlyBudgetCents,omitempty" yaml:"monthlyBudgetCents,omitempty"`
	Priced             []costPricedItem      `json:"priced" yaml:"priced"`
	Estimated          []costEstimatedItem   `json:"estimated" yaml:"estimated"`
	Unsupported        []costUnsupportedItem `json:"unsupported" yaml:"unsupported"`
	MonthlyTotalCents  *int64                `json:"monthlyTotalCents" yaml:"monthlyTotalCents"`
	Notice             string                `json:"notice" yaml:"notice"`
}

type costPricedItem struct {
	Resource     string `json:"resource" yaml:"resource"`
	MonthlyCents int64  `json:"monthlyCents" yaml:"monthlyCents"`
	Basis        string `json:"basis" yaml:"basis"`
}

type costEstimatedItem struct {
	Resource      string `json:"resource" yaml:"resource"`
	Configuration string `json:"configuration" yaml:"configuration"`
}

type costUnsupportedItem struct {
	Resource string `json:"resource" yaml:"resource"`
	Reason   string `json:"reason" yaml:"reason"`
}

func costCommand(o *options) *cobra.Command {
	var live bool
	command := &cobra.Command{
		Use:   "cost",
		Short: "Describe the selected environment's cost inputs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			effective, environment, err := o.resolveWithEnvironment()
			if err != nil {
				return invalid(err)
			}
			if err := requireAWSCostTarget(effective.Config); err != nil {
				return invalid(err)
			}
			if !live {
				return o.write(newCostReport(effective.Config, environment))
			}
			if o.newPricing == nil {
				return &exitError{code: 3, err: errors.New("live pricing is not configured")}
			}
			estimator, err := o.newPricing(cmd.Context(), effective.Config.Defaults.Region)
			if err != nil {
				return &exitError{code: 3, err: err}
			}
			report, err := newLiveCostReport(cmd.Context(), effective.Config, environment, estimator)
			if err != nil {
				return &exitError{code: 3, err: err}
			}
			return o.write(report)
		},
	}
	command.Flags().BoolVar(&live, "live", false, "query current AWS on-demand prices")
	return command
}

func requireAWSCostTarget(cfg config.Config) error {
	if cfg.Target.Provider != "aws" {
		return errors.New("cost estimation is certified for AWS ECS Fargate only")
	}
	runtime := cfg.Target.Runtime
	if runtime == "" {
		runtime = "ecs-fargate"
	}
	if runtime != "ecs-fargate" {
		return fmt.Errorf("cost estimation is not available for runtime %q yet", runtime)
	}
	return nil
}

func newCostReport(cfg config.Config, environment string) costReport {
	preset := cfg.Preset
	if preset == "" {
		preset = cfg.Defaults.Preset
	}
	report := costReport{
		Environment: environment, Provider: cfg.Target.Provider, Region: cfg.Defaults.Region, Preset: preset,
		Mode: "account-free", Currency: "USD", MonthlyBudgetCents: cfg.MonthlyBudgetCents,
		Priced: []costPricedItem{}, MonthlyTotalCents: nil,
		Notice: "Account-free mode does not call AWS or claim current prices. Capacity is derived from magelift.yaml; use magelift cost --live when current AWS prices are available.",
		Unsupported: []costUnsupportedItem{
			{Resource: "AWS unit prices", Reason: "current regional prices require live AWS Pricing data"},
			{Resource: "usage-based services", Reason: "data transfer, requests, storage growth, logs, WAF, CloudFront, and NAT processing depend on workload measurements"},
		},
	}
	if cfg.Target.AWS == nil {
		report.Estimated = presetAssumptions(preset)
		report.Unsupported = append(report.Unsupported, costUnsupportedItem{Resource: "service capacity", Reason: "target.aws.catalog is not configured"})
		return report
	}

	catalog := cfg.Target.AWS.Catalog
	report.Estimated = append(report.Estimated,
		costEstimatedItem{Resource: "ECS Fargate web service", Configuration: fmt.Sprintf("%d tasks, %d CPU units, %d MiB each", catalog.Fargate.DesiredCount, catalog.Fargate.CPU, catalog.Fargate.MemoryMiB)},
		costEstimatedItem{Resource: "ElastiCache Valkey", Configuration: fmt.Sprintf("%s, %d replicas", valueOrUnknown(catalog.Valkey.NodeType), catalog.Valkey.ReplicaCount)},
	)
	if preset == "preview" {
		report.Estimated = append(report.Estimated,
			costEstimatedItem{Resource: "Aurora Serverless v2", Configuration: fmt.Sprintf("%.2f-%.2f ACU", catalog.Aurora.MinimumACU, catalog.Aurora.MaximumACU)},
			costEstimatedItem{Resource: "OpenSearch Serverless", Configuration: fmt.Sprintf("up to %.2f indexing OCU and %.2f search OCU", catalog.Search.MaximumIndexingOCU, catalog.Search.MaximumSearchOCU)},
			costEstimatedItem{Resource: "Magento queue", Configuration: "database-backed; no Amazon MQ broker"},
		)
	} else {
		report.Estimated = append(report.Estimated,
			costEstimatedItem{Resource: "Aurora MySQL", Configuration: fmt.Sprintf("%d x %s", catalog.Aurora.InstanceCount, valueOrUnknown(catalog.Aurora.InstanceClass))},
			costEstimatedItem{Resource: "OpenSearch", Configuration: fmt.Sprintf("%d x %s data nodes", catalog.Search.InstanceCount, valueOrUnknown(catalog.Search.InstanceType))},
			costEstimatedItem{Resource: "Amazon MQ for RabbitMQ", Configuration: "3 x " + valueOrUnknown(catalog.RabbitMQ.InstanceType)},
		)
	}
	return report
}

func newLiveCostReport(ctx context.Context, cfg config.Config, environment string, estimator costEstimator) (costReport, error) {
	report := newCostReport(cfg, environment)
	report.Mode = "live"
	report.Priced = []costPricedItem{}
	report.Estimated = liveUnpricedEstimates(cfg)
	report.Unsupported = []costUnsupportedItem{
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
				report.Unsupported = append(report.Unsupported, costUnsupportedItem{Resource: query.Resource, Reason: err.Error()})
				continue
			}
			return report, err
		}
		total += price.MonthlyCents
		if price.Currency != "" {
			currency = price.Currency
		}
		report.Priced = append(report.Priced, costPricedItem{Resource: price.Resource, MonthlyCents: price.MonthlyCents, Basis: price.Basis})
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

func liveUnpricedEstimates(cfg config.Config) []costEstimatedItem {
	if cfg.Target.AWS == nil {
		return nil
	}
	catalog := cfg.Target.AWS.Catalog
	preset := cfg.Preset
	if preset == "" {
		preset = cfg.Defaults.Preset
	}
	if preset == "preview" {
		return []costEstimatedItem{
			{Resource: "Aurora Serverless v2", Configuration: fmt.Sprintf("%.2f-%.2f ACU; workload utilization is not priced", catalog.Aurora.MinimumACU, catalog.Aurora.MaximumACU)},
			{Resource: "OpenSearch Serverless", Configuration: fmt.Sprintf("up to %.2f indexing OCU and %.2f search OCU; workload utilization is not priced", catalog.Search.MaximumIndexingOCU, catalog.Search.MaximumSearchOCU)},
			{Resource: "Magento queue", Configuration: "database-backed; no Amazon MQ broker"},
		}
	}
	return []costEstimatedItem{
		{Resource: "CloudWatch, S3, CloudFront, WAF, and NAT", Configuration: "usage-based charges require workload measurements"},
		{Resource: "OpenSearch storage and dedicated masters", Configuration: "catalog capacity is reported; current price dimensions are not queried"},
	}
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
		queries = append(queries, awspricing.Query{
			Resource: "Aurora MySQL", ServiceCode: "AmazonRDS", UsageTypeContains: "InstanceUsage",
			Filters:  map[string]string{"productFamily": "Database Instance", "databaseEngine": "Aurora MySQL", "instanceType": catalog.Aurora.InstanceClass},
			Quantity: float64(catalog.Aurora.InstanceCount), Basis: fmt.Sprintf("%d instances × 730 hours", catalog.Aurora.InstanceCount),
		})
	}
	if catalog.Search.InstanceType != "" && catalog.Search.InstanceCount > 0 {
		queries = append(queries, awspricing.Query{
			Resource: "OpenSearch data nodes", ServiceCode: "AmazonES", UsageTypeContains: "InstanceUsage",
			Filters:  map[string]string{"productFamily": "Amazon OpenSearch Service", "instanceType": catalog.Search.InstanceType},
			Quantity: float64(catalog.Search.InstanceCount), Basis: fmt.Sprintf("%d data nodes × 730 hours", catalog.Search.InstanceCount),
		})
	}
	if catalog.RabbitMQ.InstanceType != "" {
		queries = append(queries, awspricing.Query{
			Resource: "Amazon MQ for RabbitMQ", ServiceCode: "AmazonMQ", UsageTypeContains: "BrokerUsage",
			Filters:  map[string]string{"productFamily": "RabbitMQ Broker", "instanceType": catalog.RabbitMQ.InstanceType},
			Quantity: 3, Basis: "3 brokers × 730 hours",
		})
	}
	return queries
}

func presetAssumptions(preset string) []costEstimatedItem {
	return []costEstimatedItem{{Resource: "MageLift preset", Configuration: valueOrUnknown(preset) + " topology; exact capacity is not configured"}}
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}
