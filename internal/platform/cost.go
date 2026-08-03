package platform

import (
	"context"

	"github.com/magelift/magelift/internal/config"
)

// CostOptions selects account-free capacity reporting vs live provider prices.
type CostOptions struct {
	Live bool
}

// CostReport is the provider-neutral cost CLI payload.
type CostReport struct {
	Environment        string                `json:"environment" yaml:"environment"`
	Provider           string                `json:"provider" yaml:"provider"`
	Region             string                `json:"region" yaml:"region"`
	Preset             string                `json:"preset" yaml:"preset"`
	Mode               string                `json:"mode" yaml:"mode"`
	Currency           string                `json:"currency" yaml:"currency"`
	MonthlyBudgetCents int64                 `json:"monthlyBudgetCents,omitempty" yaml:"monthlyBudgetCents,omitempty"`
	Priced             []CostPricedItem      `json:"priced" yaml:"priced"`
	Estimated          []CostEstimatedItem   `json:"estimated" yaml:"estimated"`
	Unsupported        []CostUnsupportedItem `json:"unsupported" yaml:"unsupported"`
	MonthlyTotalCents  *int64                `json:"monthlyTotalCents" yaml:"monthlyTotalCents"`
	Notice             string                `json:"notice" yaml:"notice"`
}

// CostPricedItem is a live-priced capacity line.
type CostPricedItem struct {
	Resource     string `json:"resource" yaml:"resource"`
	MonthlyCents int64  `json:"monthlyCents" yaml:"monthlyCents"`
	Basis        string `json:"basis" yaml:"basis"`
}

// CostEstimatedItem is capacity without a claimed unit price.
type CostEstimatedItem struct {
	Resource      string `json:"resource" yaml:"resource"`
	Configuration string `json:"configuration" yaml:"configuration"`
}

// CostUnsupportedItem explains why a capacity cannot be priced yet.
type CostUnsupportedItem struct {
	Resource string `json:"resource" yaml:"resource"`
	Reason   string `json:"reason" yaml:"reason"`
}

// CostEstimator is the per-provider cost port (ADR 0002: cost stays target-specific).
// Account-free mode never calls the cloud; Live may query the provider price list.
type CostEstimator interface {
	Estimate(ctx context.Context, planned PlannedStack, cfg config.Config, opts CostOptions) (CostReport, error)
}

// HasCostEstimator is implemented by StackModules that expose cost estimation.
type HasCostEstimator interface {
	CostEstimator() CostEstimator
}

// ModuleCostEstimator returns CostEstimator when the module implements it.
func ModuleCostEstimator(module StackModule) CostEstimator {
	if module == nil {
		return nil
	}
	if provider, ok := module.(HasCostEstimator); ok {
		return provider.CostEstimator()
	}
	return nil
}
