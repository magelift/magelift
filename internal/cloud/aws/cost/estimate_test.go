package cost_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	awscost "github.com/magelift/magelift/internal/cloud/aws/cost"
	awspricing "github.com/magelift/magelift/internal/cloud/aws/pricing"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type fakePlanned struct {
	env     string
	region  string
	runtime sdk.RuntimeID
}

func (f fakePlanned) StackName() string        { return "test" }
func (f fakePlanned) Provider() sdk.ProviderID { return "aws" }
func (f fakePlanned) Runtime() sdk.RuntimeID {
	if f.runtime != "" {
		return f.runtime
	}
	return "ecs-fargate"
}
func (f fakePlanned) Project() string     { return "shop" }
func (f fakePlanned) Environment() string { return f.env }
func (f fakePlanned) Region() string      { return f.region }
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
	runtime := f.Runtime()
	return sdk.TargetDescriptor{ID: sdk.TargetID("aws-" + string(runtime)), Provider: "aws", Runtime: runtime}
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

func TestAccountFreeEKSReportClassifiesArchitectureAndWorkloads(t *testing.T) {
	cfg := config.Config{
		Target: config.Target{Provider: "aws", Runtime: "eks", AWS: &config.AWSTarget{Catalog: config.AWSCatalog{
			EKS: config.AWSCatalogEKS{
				ComputeMode: "managed-node-groups", KubernetesVersion: "1.36", CPURequest: "500m", MemoryRequest: "1Gi",
				DesiredWebReplicas: 2, NodeInstanceType: "m6i.large", NodeMinSize: 2, NodeDesiredSize: 3, NodeMaxSize: 6,
				SearchMode: "opensearch", SearchReplicas: 3, QueueMode: "rabbitmq", QueueReplicas: 2, QueueConsumerCount: 2,
			},
			Valkey: config.AWSCatalogValkey{NodeType: "cache.r7g.large", ReplicaCount: 1},
			Aurora: config.AWSCatalogAurora{InstanceClass: "db.r7g.large", InstanceCount: 2},
		}}},
		Defaults: config.Defaults{Region: "eu-west-3"}, Preset: "high-availability",
	}
	report, err := awscost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "staging", region: "eu-west-3", runtime: "eks"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "account-free" || report.Region != "eu-west-3" || report.MonthlyTotalCents != nil || len(report.Priced) != 0 {
		t.Fatalf("unexpected EKS account-free report: %#v", report)
	}
	if len(report.Estimated) != 6 || len(report.Unsupported) != 2 {
		t.Fatalf("unexpected EKS classifications: %#v", report)
	}
	for _, resource := range []string{"Amazon EKS control plane", "EKS managed node groups", "ElastiCache Valkey", "Aurora MySQL", "OpenSearch on EKS", "RabbitMQ on EKS"} {
		if !hasEstimatedResource(report, resource) {
			t.Fatalf("missing EKS estimate %q: %#v", resource, report.Estimated)
		}
	}
}

func TestAccountFreeEKSReportsEachComputeMode(t *testing.T) {
	for _, test := range []struct {
		mode     string
		resource string
	}{
		{mode: "auto-mode", resource: "EKS Auto Mode compute"},
		{mode: "managed-node-groups", resource: "EKS managed node groups"},
		{mode: "self-managed", resource: "EKS self-managed nodes"},
		{mode: "fargate", resource: "EKS Fargate compute"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			cfg := config.Config{
				Target: config.Target{Provider: "aws", Runtime: "eks", AWS: &config.AWSTarget{Catalog: config.AWSCatalog{
					EKS: config.AWSCatalogEKS{ComputeMode: test.mode, DesiredWebReplicas: 1, CPURequest: "500m", MemoryRequest: "1Gi", NodeInstanceType: "m6i.large", NodeMinSize: 1, NodeDesiredSize: 1, NodeMaxSize: 2},
				}}},
				Defaults: config.Defaults{Region: "eu-west-3"}, Preset: "preview",
			}
			report, err := awscost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "preview", region: "eu-west-3", runtime: "eks"}, cfg, platform.CostOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !hasEstimatedResource(report, test.resource) {
				t.Fatalf("missing %q in %#v", test.resource, report.Estimated)
			}
		})
	}
}

func TestLiveEKSReportFailsClosedBeforePricingClient(t *testing.T) {
	cfg := config.Config{Target: config.Target{Provider: "aws", Runtime: "eks", AWS: &config.AWSTarget{Catalog: config.AWSCatalog{EKS: config.AWSCatalogEKS{ComputeMode: "auto-mode"}}}}, Defaults: config.Defaults{Region: "eu-west-3"}}
	called := false
	estimator := awscost.Estimator{NewPricing: func(context.Context, string) (awscost.PricingClient, error) {
		called = true
		return nil, nil
	}}
	_, err := estimator.Estimate(context.Background(), fakePlanned{env: "preview", region: "eu-west-3", runtime: "eks"}, cfg, platform.CostOptions{Live: true})
	if err == nil || !strings.Contains(err.Error(), "live cost estimation is not available") {
		t.Fatalf("expected explicit EKS live pricing refusal, got %v", err)
	}
	if called {
		t.Fatal("EKS live pricing refusal constructed a pricing client")
	}
}

func hasEstimatedResource(report platform.CostReport, resource string) bool {
	for _, item := range report.Estimated {
		if item.Resource == resource {
			return true
		}
	}
	return false
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

func TestAccountFreePreviewFlagsExpensiveCatalog(t *testing.T) {
	cfg := config.Config{
		Class: "preview",
		Target: config.Target{Provider: "aws", Runtime: "ecs-fargate", AWS: &config.AWSTarget{Catalog: config.AWSCatalog{
			QueueMode: "amazon-mq",
			Fargate:   config.AWSCatalogFargate{CPU: 256, MemoryMiB: 512, DesiredCount: 1},
			Valkey:    config.AWSCatalogValkey{NodeType: "cache.t4g.micro", ReplicaCount: 0},
			Aurora:    config.AWSCatalogAurora{InstanceClass: "db.t4g.medium", InstanceCount: 2},
			Search:    config.AWSCatalogSearch{InstanceType: "t3.medium.search", InstanceCount: 1},
			RabbitMQ:  config.AWSCatalogRabbitMQ{InstanceType: "mq.t3.micro"},
		}}},
		Defaults: config.Defaults{Region: "eu-west-3"}, Preset: "preview",
	}
	report, err := awscost.Estimator{}.Estimate(context.Background(), fakePlanned{env: "preview", region: "eu-west-3"}, cfg, platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report.Notice, "Amazon MQ") || !strings.Contains(report.Notice, "OpenSearch") || !strings.Contains(report.Notice, "Aurora") {
		t.Fatalf("expensive preview catalog was not flagged: %q", report.Notice)
	}
}
