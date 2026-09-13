package platform

import (
	"context"
	"time"

	"github.com/magelift/magelift/internal/config"
)

type CostEvidence string

const (
	CostEvidenceEstimate    CostEvidence = "estimate"
	CostEvidenceLivePrice   CostEvidence = "live-price"
	CostEvidenceActual      CostEvidence = "actual"
	CostEvidenceForecast    CostEvidence = "forecast"
	CostEvidenceUnavailable CostEvidence = "unavailable"
)

type CostFreshness string

const (
	CostFreshnessCurrent CostFreshness = "current"
	CostFreshnessStale   CostFreshness = "stale"
	CostFreshnessUnknown CostFreshness = "unknown"
)

// CostOptions selects account-free capacity reporting vs live provider prices.
type CostOptions struct {
	Live   bool
	Budget bool
}

// CostMoney keeps the provider's exact major and nano units. Converting a
// provider amount to cents before rendering can silently round a budget.
type CostMoney struct {
	Units      int64         `json:"units" yaml:"units"`
	Nanos      int64         `json:"nanos" yaml:"nanos"`
	Currency   string        `json:"currency" yaml:"currency"`
	Basis      string        `json:"basis,omitempty" yaml:"basis,omitempty"`
	Source     string        `json:"source,omitempty" yaml:"source,omitempty"`
	ObservedAt time.Time     `json:"observedAt,omitempty" yaml:"observedAt,omitempty"`
	Freshness  CostFreshness `json:"freshness,omitempty" yaml:"freshness,omitempty"`
}

type CostBudgetState string

const (
	CostBudgetConfigured    CostBudgetState = "configured"
	CostBudgetNotConfigured CostBudgetState = "not-configured"
	CostBudgetUnavailable   CostBudgetState = "unavailable"
)

// CostBudgetThreshold is an alert rule, not a deployment enforcement rule.
type CostBudgetThreshold struct {
	Percent float64 `json:"percent" yaml:"percent"`
	Basis   string  `json:"basis,omitempty" yaml:"basis,omitempty"`
	State   string  `json:"state,omitempty" yaml:"state,omitempty"`
}

// CostBudget is a provider budget definition. The provider may alert on this
// definition without blocking deployments; Enforced is therefore explicit.
type CostBudget struct {
	Name              string                `json:"name" yaml:"name"`
	DisplayName       string                `json:"displayName" yaml:"displayName"`
	Scope             string                `json:"scope" yaml:"scope"`
	Period            string                `json:"period" yaml:"period"`
	Amount            CostMoney             `json:"amount" yaml:"amount"`
	Thresholds        []CostBudgetThreshold `json:"thresholds" yaml:"thresholds"`
	Actual            *CostMoney            `json:"actual,omitempty" yaml:"actual,omitempty"`
	Forecast          *CostMoney            `json:"forecast,omitempty" yaml:"forecast,omitempty"`
	State             CostBudgetState       `json:"state" yaml:"state"`
	ScopeVerified     bool                  `json:"scopeVerified" yaml:"scopeVerified"`
	OwnershipVerified bool                  `json:"ownershipVerified" yaml:"ownershipVerified"`
	Enforced          bool                  `json:"enforced" yaml:"enforced"`
}

type CostBudgetReport struct {
	Scope      string          `json:"scope" yaml:"scope"`
	Source     string          `json:"source" yaml:"source"`
	ObservedAt time.Time       `json:"observedAt" yaml:"observedAt"`
	Freshness  CostFreshness   `json:"freshness" yaml:"freshness"`
	State      CostBudgetState `json:"state" yaml:"state"`
	Budgets    []CostBudget    `json:"budgets" yaml:"budgets"`
	Actual     *CostMoney      `json:"actual,omitempty" yaml:"actual,omitempty"`
	Forecast   *CostMoney      `json:"forecast,omitempty" yaml:"forecast,omitempty"`
	Notice     string          `json:"notice" yaml:"notice"`
}

// UnavailableCostBudgetReport keeps an unsupported budget API explicit. A
// configured monthlyBudgetCents value is not a provider budget definition.
func UnavailableCostBudgetReport(scope, source, notice string) *CostBudgetReport {
	return &CostBudgetReport{
		Scope:      scope,
		Source:     source,
		ObservedAt: time.Now().UTC(),
		Freshness:  CostFreshnessUnknown,
		State:      CostBudgetUnavailable,
		Budgets:    []CostBudget{},
		Notice:     notice,
	}
}

// CostReport is the provider-neutral cost CLI payload.
type CostReport struct {
	Environment        string                `json:"environment" yaml:"environment"`
	Provider           string                `json:"provider" yaml:"provider"`
	Region             string                `json:"region" yaml:"region"`
	Scope              string                `json:"scope" yaml:"scope"`
	Preset             string                `json:"preset" yaml:"preset"`
	Mode               string                `json:"mode" yaml:"mode"`
	Currency           string                `json:"currency" yaml:"currency"`
	Evidence           CostEvidence          `json:"evidence" yaml:"evidence"`
	Source             string                `json:"source" yaml:"source"`
	ObservedAt         time.Time             `json:"observedAt" yaml:"observedAt"`
	Freshness          CostFreshness         `json:"freshness" yaml:"freshness"`
	MonthlyBudgetCents int64                 `json:"monthlyBudgetCents,omitempty" yaml:"monthlyBudgetCents,omitempty"`
	Priced             []CostPricedItem      `json:"priced" yaml:"priced"`
	Estimated          []CostEstimatedItem   `json:"estimated" yaml:"estimated"`
	Unsupported        []CostUnsupportedItem `json:"unsupported" yaml:"unsupported"`
	MonthlyTotalCents  *int64                `json:"monthlyTotalCents" yaml:"monthlyTotalCents"`
	Actual             *CostMoney            `json:"actual,omitempty" yaml:"actual,omitempty"`
	Forecast           *CostMoney            `json:"forecast,omitempty" yaml:"forecast,omitempty"`
	Budget             *CostBudgetReport     `json:"budget,omitempty" yaml:"budget,omitempty"`
	Notice             string                `json:"notice" yaml:"notice"`
}

// CostPricedItem is a live-priced capacity line.
type CostPricedItem struct {
	Resource     string       `json:"resource" yaml:"resource"`
	MonthlyCents int64        `json:"monthlyCents" yaml:"monthlyCents"`
	Basis        string       `json:"basis" yaml:"basis"`
	Evidence     CostEvidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	Source       string       `json:"source,omitempty" yaml:"source,omitempty"`
}

// CostEstimatedItem is capacity without a claimed unit price.
type CostEstimatedItem struct {
	Resource      string       `json:"resource" yaml:"resource"`
	Configuration string       `json:"configuration" yaml:"configuration"`
	Evidence      CostEvidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	Source        string       `json:"source,omitempty" yaml:"source,omitempty"`
}

// CostUnsupportedItem explains why a capacity cannot be priced yet.
type CostUnsupportedItem struct {
	Resource string       `json:"resource" yaml:"resource"`
	Reason   string       `json:"reason" yaml:"reason"`
	Evidence CostEvidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	Source   string       `json:"source,omitempty" yaml:"source,omitempty"`
}

// NormalizeCostReport makes the evidence class explicit without guessing
// actual spend from a configuration estimate.
func NormalizeCostReport(report CostReport, observedAt time.Time) CostReport {
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	report.ObservedAt = observedAt.UTC()
	if report.Scope == "" {
		report.Scope = report.Provider + "/" + report.Region + "/" + report.Environment
	}
	if report.Evidence == "" {
		report.Evidence = CostEvidenceEstimate
		if report.Mode == "live" {
			report.Evidence = CostEvidenceLivePrice
		}
	}
	if report.Source == "" {
		report.Source = "configuration"
		if report.Evidence == CostEvidenceLivePrice {
			report.Source = report.Provider + " price list"
		}
	}
	if report.Freshness == "" {
		report.Freshness = CostFreshnessCurrent
	}
	for i := range report.Priced {
		if report.Priced[i].Evidence == "" {
			report.Priced[i].Evidence = report.Evidence
		}
		if report.Priced[i].Source == "" {
			report.Priced[i].Source = report.Source
		}
	}
	for i := range report.Estimated {
		if report.Estimated[i].Evidence == "" {
			report.Estimated[i].Evidence = CostEvidenceEstimate
		}
		if report.Estimated[i].Source == "" {
			report.Estimated[i].Source = "configuration"
		}
	}
	for i := range report.Unsupported {
		if report.Unsupported[i].Evidence == "" {
			report.Unsupported[i].Evidence = CostEvidenceUnavailable
		}
		if report.Unsupported[i].Source == "" {
			report.Unsupported[i].Source = report.Provider
		}
	}
	if report.Actual != nil {
		normalizeCostMoney(report.Actual, report.ObservedAt, report.Source, report.Freshness)
	}
	if report.Forecast != nil {
		normalizeCostMoney(report.Forecast, report.ObservedAt, report.Source, report.Freshness)
	}
	if report.Budget != nil {
		if report.Budget.ObservedAt.IsZero() {
			report.Budget.ObservedAt = report.ObservedAt
		}
		if report.Budget.Source == "" {
			report.Budget.Source = report.Source
		}
		if report.Budget.Freshness == "" {
			report.Budget.Freshness = report.Freshness
		}
		for index := range report.Budget.Budgets {
			budget := &report.Budget.Budgets[index]
			normalizeCostMoney(&budget.Amount, report.Budget.ObservedAt, report.Budget.Source, report.Budget.Freshness)
			if budget.Actual != nil {
				normalizeCostMoney(budget.Actual, report.Budget.ObservedAt, report.Budget.Source, report.Budget.Freshness)
			}
			if budget.Forecast != nil {
				normalizeCostMoney(budget.Forecast, report.Budget.ObservedAt, report.Budget.Source, report.Budget.Freshness)
			}
		}
		if report.Budget.Actual != nil {
			normalizeCostMoney(report.Budget.Actual, report.Budget.ObservedAt, report.Budget.Source, report.Budget.Freshness)
		}
		if report.Budget.Forecast != nil {
			normalizeCostMoney(report.Budget.Forecast, report.Budget.ObservedAt, report.Budget.Source, report.Budget.Freshness)
		}
	}
	return report
}

func normalizeCostMoney(money *CostMoney, observedAt time.Time, source string, freshness CostFreshness) {
	if money.ObservedAt.IsZero() {
		money.ObservedAt = observedAt
	}
	if money.Source == "" {
		money.Source = source
	}
	if money.Freshness == "" {
		money.Freshness = freshness
	}
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
