package pricing

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/pricing"
)

type fakeAPI struct {
	output *pricing.GetProductsOutput
	err    error
	input  *pricing.GetProductsInput
}

func (f *fakeAPI) GetProducts(_ context.Context, input *pricing.GetProductsInput, _ ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
	f.input = input
	return f.output, f.err
}

func TestEstimateParsesOnDemandPriceAndRegionFilter(t *testing.T) {
	api := &fakeAPI{output: &pricing.GetProductsOutput{PriceList: []string{`{"product":{"attributes":{"usagetype":"EUW3-Fargate-vCPU-Hours"}},"terms":{"OnDemand":{"term":{"priceDimensions":{"dimension":{"unit":"vCPU-Hours","pricePerUnit":{"USD":"0.10"}}}}}}}`}}}
	client, err := NewWithAPI(api, "eu-west-3")
	if err != nil {
		t.Fatal(err)
	}
	price, err := client.Estimate(context.Background(), Query{
		Resource: "ECS Fargate vCPU", ServiceCode: "AmazonECS", UsageTypeContains: "Fargate-vCPU-Hours", Quantity: 2, Basis: "2 vCPU tasks",
	})
	if err != nil {
		t.Fatal(err)
	}
	if price.MonthlyCents != 14600 || price.Currency != "USD" || price.Basis != "2 vCPU tasks" {
		t.Fatalf("unexpected price: %#v", price)
	}
	if api.input == nil || len(api.input.Filters) != 1 || *api.input.Filters[0].Value != "Europe (Paris)" {
		t.Fatalf("region filter = %#v", api.input)
	}
}

func TestEstimateClassifiesMissingPrice(t *testing.T) {
	client, err := NewWithAPI(&fakeAPI{output: &pricing.GetProductsOutput{PriceList: []string{`{"product":{"attributes":{"usagetype":"other"}},"terms":{}}`}}}, "eu-west-3")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Estimate(context.Background(), Query{Resource: "missing", ServiceCode: "AmazonECS", Quantity: 1})
	if !errors.Is(err, ErrNoPrice) {
		t.Fatalf("error = %v, want ErrNoPrice", err)
	}
}

func TestEstimateRejectsInvalidQuery(t *testing.T) {
	client, err := NewWithAPI(&fakeAPI{}, "eu-west-3")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Estimate(context.Background(), Query{Resource: "bad", ServiceCode: "AmazonECS"})
	if err == nil {
		t.Fatal("invalid quantity was accepted")
	}
}

func TestEstimateFollowsPricingPagination(t *testing.T) {
	api := &pagedAPI{outputs: []*pricing.GetProductsOutput{
		{NextToken: stringPtr("page-2")},
		{PriceList: []string{`{"product":{"attributes":{"usagetype":"Fargate-vCPU-Hours"}},"terms":{"OnDemand":{"term":{"priceDimensions":{"dimension":{"pricePerUnit":{"USD":"0.10"}}}}}}}`}},
	}}
	client, err := NewWithAPI(api, "eu-west-3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Estimate(context.Background(), Query{Resource: "fargate", ServiceCode: "AmazonECS", UsageTypeContains: "Fargate-vCPU-Hours", Quantity: 1}); err != nil {
		t.Fatal(err)
	}
	if api.calls != 2 || api.tokens[1] != "page-2" {
		t.Fatalf("pagination calls=%d tokens=%v", api.calls, api.tokens)
	}
}

func TestNewRejectsUnknownRegion(t *testing.T) {
	if _, err := NewWithAPI(&fakeAPI{}, "moon-1"); err == nil {
		t.Fatal("unknown region was accepted")
	}
}

type pagedAPI struct {
	outputs []*pricing.GetProductsOutput
	calls   int
	tokens  []string
}

func (p *pagedAPI) GetProducts(_ context.Context, input *pricing.GetProductsInput, _ ...func(*pricing.Options)) (*pricing.GetProductsOutput, error) {
	p.calls++
	if input.NextToken != nil {
		p.tokens = append(p.tokens, *input.NextToken)
	} else {
		p.tokens = append(p.tokens, "")
	}
	return p.outputs[p.calls-1], nil
}

func stringPtr(value string) *string { return &value }
