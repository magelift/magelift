package cost_test

import (
	"context"
	"encoding/json"
	"testing"

	awscost "github.com/magelift/magelift/internal/cloud/aws/cost"
	awspricing "github.com/magelift/magelift/internal/cloud/aws/pricing"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type fakePlanned struct {
	env    string
	region string
}

func (f fakePlanned) StackName() string        { return "test" }
func (f fakePlanned) Provider() sdk.ProviderID { return "aws" }
func (f fakePlanned) Runtime() sdk.RuntimeID   { return "ecs-fargate" }
func (f fakePlanned) Project() string          { return "shop" }
func (f fakePlanned) Environment() string      { return f.env }
func (f fakePlanned) Region() string           { return f.region }
func (f fakePlanned) CertificationTier() platform.CertificationTier {
	return platform.TierCertified
}
func (f fakePlanned) EnvironmentClass() string { return "preview" }
func (f fakePlanned) Protected() bool          { return false }
func (f fakePlanned) ImageDigest() string      { return "" }
func (f fakePlanned) WithImageDigest(string) (platform.PlannedStack, error) {
	return f, nil
}
func (f fakePlanned) TargetDescriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "aws-ecs-fargate", Provider: "aws", Runtime: "ecs-fargate"}
}

type fakePricing struct {
	prices  map[string]awspricing.Price
	queries []awspricing.Query
}

func (f *fakePricing) Estimate(_ context.Context, query awspricing.Query) (awspricing.Price, error) {
	f.queries = append(f.queries, query)
	if price, ok := f.prices[query.Resource]; ok {
		return price, nil
	}
	return awspricing.Price{}, awspricing.ErrNoPrice
}

func TestAccountFreeCostReportClassifiesInputs(t *testing.T) {
	cfg := config.Config{
		Target: config.Target{Provider: "aws", Runtime: "ecs-fargate", AWS: &config.AWSTarget{Catalog: config.AWSCatalog{
			Fargate:  config.AWSCatalogFargate{CPU: 1024, MemoryMiB: 2048, DesiredCount: 2},
			Valkey:   config.AWSCatalogValkey{NodeType: "cache.t4g.small", ReplicaCount: 1},
			Aurora:   config.AWSCatalogAurora{InstanceClass: "db.r7g.large", InstanceCount: 2},
			Search:   config.AWSCatalogSearch{InstanceType: "r7g.large.search", InstanceCount: 3},
			RabbitMQ: config.AWSCatalogRabbitMQ{InstanceType: "mq.m7g.large"},
		}}},
		Defaults: config.Defaults{Region: "eu-west-3"}, Preset: "standard", MonthlyBudgetCents: 50000,
	}
	report, err := awscost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "staging", region: "eu-west-3"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
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
	cfg := config.Config{Target: config.Target{Provider: "aws", Runtime: "ecs-fargate"}, Defaults: config.Defaults{Region: "eu-west-3", Preset: "preview"}}
	report, err := awscost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "preview", region: "eu-west-3"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Estimated) != 1 || len(report.Unsupported) != 3 {
		t.Fatalf("missing catalog was not classified: %#v", report)
	}
}

func TestLiveCostReportSumsCurrentPricesAndSeparatesMissingProducts(t *testing.T) {
	cfg := config.Config{
		Target: config.Target{Provider: "aws", Runtime: "ecs-fargate", AWS: &config.AWSTarget{Catalog: config.AWSCatalog{
			Fargate: config.AWSCatalogFargate{CPU: 1024, MemoryMiB: 2048, DesiredCount: 2},
			Valkey:  config.AWSCatalogValkey{NodeType: "cache.t4g.small", ReplicaCount: 1},
			Aurora:  config.AWSCatalogAurora{InstanceClass: "db.r7g.large", InstanceCount: 2},
		}}},
		Defaults: config.Defaults{Region: "eu-west-3"}, Preset: "standard",
	}
	fake := &fakePricing{prices: map[string]awspricing.Price{
		"ECS Fargate vCPU":   {Resource: "ECS Fargate vCPU", MonthlyCents: 1000, Currency: "USD", Basis: "vCPU"},
		"ECS Fargate memory": {Resource: "ECS Fargate memory", MonthlyCents: 2000, Currency: "USD", Basis: "memory"},
		"ElastiCache Valkey": {Resource: "ElastiCache Valkey", MonthlyCents: 3000, Currency: "USD", Basis: "cache"},
	}}
	estimator := awscost.Estimator{NewPricing: func(context.Context, string) (awscost.PricingClient, error) { return fake, nil }}
	report, err := estimator.Estimate(context.Background(), fakePlanned{env: "staging", region: "eu-west-3"}, cfg, platform.CostOptions{Live: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "live" || report.MonthlyTotalCents == nil || *report.MonthlyTotalCents != 6000 {
		t.Fatalf("unexpected live total: %#v", report)
	}
	if len(report.Priced) != 3 || len(report.Estimated) != 3 || len(report.Unsupported) != 2 {
		t.Fatalf("unexpected live classifications: %#v", report)
	}
	if len(fake.queries) != 4 {
		t.Fatalf("query count = %d, want 4", len(fake.queries))
	}
}

func TestLiveCostReportPropagatesProviderFailure(t *testing.T) {
	cfg := config.Config{Target: config.Target{Provider: "aws", Runtime: "ecs-fargate", AWS: &config.AWSTarget{Catalog: config.AWSCatalog{Fargate: config.AWSCatalogFargate{CPU: 1024, MemoryMiB: 2048, DesiredCount: 1}}}}, Defaults: config.Defaults{Region: "eu-west-3"}}
	estimator := awscost.Estimator{NewPricing: func(context.Context, string) (awscost.PricingClient, error) {
		return failingPricing{}, nil
	}}
	if _, err := estimator.Estimate(context.Background(), fakePlanned{env: "preview", region: "eu-west-3"}, cfg, platform.CostOptions{Live: true}); err == nil {
		t.Fatal("provider failure was hidden")
	}
}

type failingPricing struct{}

func (failingPricing) Estimate(context.Context, awspricing.Query) (awspricing.Price, error) {
	return awspricing.Price{}, context.DeadlineExceeded
}
