package cli

import (
	"context"
	"encoding/json"
	"testing"

	awspricing "github.com/acourtiol/magelift/internal/cloud/aws/pricing"
	"github.com/acourtiol/magelift/internal/config"
)

type fakeCostEstimator struct {
	prices  map[string]awspricing.Price
	queries []awspricing.Query
}

func (f *fakeCostEstimator) Estimate(_ context.Context, query awspricing.Query) (awspricing.Price, error) {
	f.queries = append(f.queries, query)
	if price, ok := f.prices[query.Resource]; ok {
		return price, nil
	}
	return awspricing.Price{}, awspricing.ErrNoPrice
}

func TestAccountFreeCostReportClassifiesInputs(t *testing.T) {
	cfg := config.Config{
		Target: config.Target{Provider: "aws", AWS: &config.AWSTarget{Catalog: config.AWSCatalog{
			Fargate:  config.AWSCatalogFargate{CPU: 1024, MemoryMiB: 2048, DesiredCount: 2},
			Valkey:   config.AWSCatalogValkey{NodeType: "cache.t4g.small", ReplicaCount: 1},
			Aurora:   config.AWSCatalogAurora{InstanceClass: "db.r7g.large", InstanceCount: 2},
			Search:   config.AWSCatalogSearch{InstanceType: "r7g.large.search", InstanceCount: 3},
			RabbitMQ: config.AWSCatalogRabbitMQ{InstanceType: "mq.m7g.large"},
		}}},
		Defaults: config.Defaults{Region: "eu-west-3"}, Preset: "standard", MonthlyBudgetCents: 50000,
	}
	report := newCostReport(cfg, "staging")

	if report.Mode != "account-free" || report.MonthlyTotalCents != nil || len(report.Priced) != 0 {
		t.Fatalf("account-free report claimed a price: %#v", report)
	}
	if len(report.Estimated) != 5 || len(report.Unsupported) != 2 {
		t.Fatalf("unexpected classifications: %#v", report)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	if err := json.Unmarshal(data, &shape); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"priced", "estimated", "unsupported", "monthlyTotalCents"} {
		if _, found := shape[field]; !found {
			t.Fatalf("required field %q is absent from %s", field, data)
		}
	}
}

func TestCostReportWithoutCatalogIsExplicit(t *testing.T) {
	report := newCostReport(config.Config{Target: config.Target{Provider: "aws"}, Defaults: config.Defaults{Region: "eu-west-3", Preset: "preview"}}, "preview")
	if len(report.Estimated) != 1 || len(report.Unsupported) != 3 {
		t.Fatalf("missing catalog was not classified: %#v", report)
	}
}

func TestLiveCostReportSumsCurrentPricesAndSeparatesMissingProducts(t *testing.T) {
	cfg := config.Config{
		Target: config.Target{Provider: "aws", AWS: &config.AWSTarget{Catalog: config.AWSCatalog{
			Fargate: config.AWSCatalogFargate{CPU: 1024, MemoryMiB: 2048, DesiredCount: 2},
			Valkey:  config.AWSCatalogValkey{NodeType: "cache.t4g.small", ReplicaCount: 1},
			Aurora:  config.AWSCatalogAurora{InstanceClass: "db.r7g.large", InstanceCount: 2},
		}}},
		Defaults: config.Defaults{Region: "eu-west-3"}, Preset: "standard",
	}
	estimator := &fakeCostEstimator{prices: map[string]awspricing.Price{
		"ECS Fargate vCPU":   {Resource: "ECS Fargate vCPU", MonthlyCents: 1000, Currency: "USD", Basis: "vCPU"},
		"ECS Fargate memory": {Resource: "ECS Fargate memory", MonthlyCents: 2000, Currency: "USD", Basis: "memory"},
		"ElastiCache Valkey": {Resource: "ElastiCache Valkey", MonthlyCents: 3000, Currency: "USD", Basis: "cache"},
	}}
	report, err := newLiveCostReport(context.Background(), cfg, "staging", estimator)
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "live" || report.MonthlyTotalCents == nil || *report.MonthlyTotalCents != 6000 {
		t.Fatalf("unexpected live total: %#v", report)
	}
	if len(report.Priced) != 3 || len(report.Estimated) != 2 || len(report.Unsupported) != 2 {
		t.Fatalf("unexpected live classifications: %#v", report)
	}
	if len(estimator.queries) != 4 {
		t.Fatalf("query count = %d, want 4", len(estimator.queries))
	}
}

func TestLiveCostReportPropagatesProviderFailure(t *testing.T) {
	cfg := config.Config{Target: config.Target{Provider: "aws", AWS: &config.AWSTarget{Catalog: config.AWSCatalog{Fargate: config.AWSCatalogFargate{CPU: 1024, MemoryMiB: 2048, DesiredCount: 1}}}}, Defaults: config.Defaults{Region: "eu-west-3"}}
	estimator := &failingCostEstimator{}
	if _, err := newLiveCostReport(context.Background(), cfg, "preview", estimator); err == nil {
		t.Fatal("provider failure was hidden")
	}
}

type failingCostEstimator struct{}

func (*failingCostEstimator) Estimate(context.Context, awspricing.Query) (awspricing.Price, error) {
	return awspricing.Price{}, context.DeadlineExceeded
}
