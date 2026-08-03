// Package pricing adapts the AWS Price List API to the small set of capacity
// estimates MageLift exposes. It deliberately returns a missing-price error
// instead of inventing a fallback when AWS has no matching product.
package pricing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	pricingtypes "github.com/aws/aws-sdk-go-v2/service/pricing/types"
	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
)

const (
	apiRegion       = "us-east-1"
	hoursPerMonth   = 730.0
	defaultCurrency = "USD"
)

// ErrNoPrice means AWS answered successfully but no usable on-demand price
// matched the requested product. Callers can classify this as unsupported
// without hiding authentication or transport failures.
var ErrNoPrice = errors.New("no matching AWS price")

// API is the subset of the AWS Pricing client required by Client. Keeping it
// narrow makes pricing tests deterministic and leaves the provider boundary
// open for another cloud in a future target package.
type API interface {
	GetProducts(context.Context, *pricing.GetProductsInput, ...func(*pricing.Options)) (*pricing.GetProductsOutput, error)
}

// Client queries on-demand AWS prices for one region.
type Client struct {
	api      API
	location string
}

// New creates a live AWS Price List client. AWS exposes the Pricing API in a
// small set of API regions, so requests are sent to us-east-1 while prices are
// filtered by the target region's human-readable location.
func New(ctx context.Context, targetRegion string) (*Client, error) {
	if strings.TrimSpace(targetRegion) == "" {
		return nil, errors.New("pricing region is required")
	}
	location, ok := regionLocation(targetRegion)
	if !ok {
		return nil, fmt.Errorf("AWS pricing location for region %q is not in the built-in catalog", targetRegion)
	}
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(apiRegion))
	if err != nil {
		return nil, fmt.Errorf("load AWS pricing configuration: %w", err)
	}
	options := []func(*pricing.Options){}
	if endpoint != "" {
		options = append(options, func(o *pricing.Options) { o.BaseEndpoint = awssdk.String(endpoint) })
	}
	return &Client{api: pricing.NewFromConfig(cfg, options...), location: location}, nil
}

// NewWithAPI creates a client around a fake or custom API implementation.
// It is intended for tests and provider adapters, not application YAML.
func NewWithAPI(api API, targetRegion string) (*Client, error) {
	if api == nil {
		return nil, errors.New("pricing API is required")
	}
	location, ok := regionLocation(targetRegion)
	if !ok {
		return nil, fmt.Errorf("AWS pricing location for region %q is not in the built-in catalog", targetRegion)
	}
	return &Client{api: api, location: location}, nil
}

// Query describes one billable capacity unit. Quantity is multiplied by the
// hourly on-demand price and a 730-hour month.
type Query struct {
	Resource          string
	ServiceCode       string
	Filters           map[string]string
	UsageTypeContains string
	Quantity          float64
	Basis             string
}

// Price is a monthly price in the requested currency.
type Price struct {
	Resource     string
	MonthlyCents int64
	HourlyRate   float64
	Currency     string
	Basis        string
}

// Estimate returns a single on-demand monthly estimate. The API response can
// contain multiple products; the first product with a USD on-demand dimension
// is selected after all requested attributes match.
func (c *Client) Estimate(ctx context.Context, query Query) (Price, error) {
	if c == nil || c.api == nil {
		return Price{}, errors.New("pricing client is not configured")
	}
	if strings.TrimSpace(query.Resource) == "" || strings.TrimSpace(query.ServiceCode) == "" {
		return Price{}, errors.New("pricing resource and service code are required")
	}
	if query.Quantity <= 0 || math.IsNaN(query.Quantity) || math.IsInf(query.Quantity, 0) {
		return Price{}, fmt.Errorf("pricing quantity for %q must be positive and finite", query.Resource)
	}
	filters := make([]pricingtypes.Filter, 0, len(query.Filters)+1)
	filters = append(filters, pricingtypes.Filter{Field: awssdk.String("location"), Type: pricingtypes.FilterTypeTermMatch, Value: awssdk.String(c.location)})
	for field, value := range query.Filters {
		if strings.TrimSpace(field) == "" || strings.TrimSpace(value) == "" {
			return Price{}, fmt.Errorf("pricing filter for %q must have a field and value", query.Resource)
		}
		filters = append(filters, pricingtypes.Filter{Field: awssdk.String(field), Type: pricingtypes.FilterTypeTermMatch, Value: awssdk.String(value)})
	}
	input := &pricing.GetProductsInput{
		ServiceCode:   awssdk.String(query.ServiceCode),
		Filters:       filters,
		FormatVersion: awssdk.String("aws_v1"),
		MaxResults:    awssdk.Int32(100),
	}
	var products []string
	for {
		output, err := c.api.GetProducts(ctx, input)
		if err != nil {
			return Price{}, fmt.Errorf("query AWS price for %q: %w", query.Resource, err)
		}
		products = append(products, output.PriceList...)
		if output.NextToken == nil || strings.TrimSpace(*output.NextToken) == "" {
			break
		}
		if input.NextToken != nil && *input.NextToken == *output.NextToken {
			return Price{}, fmt.Errorf("query AWS price for %q: pagination token did not advance", query.Resource)
		}
		input.NextToken = output.NextToken
	}
	rate, currency, err := findOnDemandRate(products, query.UsageTypeContains)
	if err != nil {
		return Price{}, fmt.Errorf("query AWS price for %q: %w", query.Resource, errors.Join(ErrNoPrice, err))
	}
	monthly := int64(math.Round(rate * query.Quantity * hoursPerMonth * 100))
	if monthly < 0 {
		return Price{}, fmt.Errorf("AWS price for %q produced a negative total", query.Resource)
	}
	return Price{Resource: query.Resource, MonthlyCents: monthly, HourlyRate: rate, Currency: currency, Basis: query.Basis}, nil
}

type priceProduct struct {
	Product struct {
		Attributes map[string]string `json:"attributes"`
	} `json:"product"`
	Terms map[string]map[string]priceTerm `json:"terms"`
}

type priceTerm struct {
	PriceDimensions map[string]priceDimension `json:"priceDimensions"`
}

type priceDimension struct {
	Unit         string            `json:"unit"`
	BeginRange   string            `json:"beginRange"`
	EndRange     string            `json:"endRange"`
	PricePerUnit map[string]string `json:"pricePerUnit"`
}

func findOnDemandRate(products []string, usageTypeContains string) (float64, string, error) {
	for _, raw := range products {
		var product priceProduct
		if err := json.Unmarshal([]byte(raw), &product); err != nil {
			return 0, "", fmt.Errorf("decode AWS price product: %w", err)
		}
		if usageTypeContains != "" && !strings.Contains(strings.ToLower(product.Product.Attributes["usagetype"]), strings.ToLower(usageTypeContains)) {
			continue
		}
		terms := product.Terms["OnDemand"]
		for _, term := range terms {
			for _, dimension := range term.PriceDimensions {
				value, ok := dimension.PricePerUnit[defaultCurrency]
				if !ok || strings.TrimSpace(value) == "" {
					continue
				}
				rate, err := strconv.ParseFloat(value, 64)
				if err != nil || rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
					continue
				}
				return rate, defaultCurrency, nil
			}
		}
	}
	return 0, "", errors.New("AWS returned no USD on-demand price dimension")
}

func regionLocation(region string) (string, bool) {
	locations := map[string]string{
		"eu-west-3":      "Europe (Paris)",
		"eu-west-1":      "EU (Ireland)",
		"eu-west-2":      "EU (London)",
		"eu-central-1":   "EU (Frankfurt)",
		"eu-north-1":     "EU (Stockholm)",
		"eu-south-1":     "Europe (Milan)",
		"us-east-1":      "US East (N. Virginia)",
		"us-east-2":      "US East (Ohio)",
		"us-west-1":      "US West (N. California)",
		"us-west-2":      "US West (Oregon)",
		"ap-southeast-1": "Asia Pacific (Singapore)",
		"ap-southeast-2": "Asia Pacific (Sydney)",
		"ap-northeast-1": "Asia Pacific (Tokyo)",
		"ap-northeast-2": "Asia Pacific (Seoul)",
		"ap-south-1":     "Asia Pacific (Mumbai)",
		"ca-central-1":   "Canada (Central)",
	}
	value, ok := locations[strings.ToLower(strings.TrimSpace(region))]
	return value, ok
}
