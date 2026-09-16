package cost

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	billingbudgets "google.golang.org/api/billingbudgets/v1"
	cloudbilling "google.golang.org/api/cloudbilling/v1"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/option"

	"github.com/magelift/magelift/internal/platform"
)

const (
	cloudPlatformScope = "https://www.googleapis.com/auth/cloud-platform"
	budgetPageSize     = 100
	budgetListLimit    = 1000
)

// BudgetReader reads project-scoped budget definitions. It has no create,
// update, or delete path; budget alerts do not block MageLift operations.
type BudgetReader struct {
	billingInfo   func(context.Context, string) (*cloudbilling.ProjectBillingInfo, error)
	projectNumber func(context.Context, string) (int64, error)
	listBudgets   func(context.Context, string, string) ([]*billingbudgets.GoogleCloudBillingBudgetsV1Budget, error)
}

// NewBudgetReader creates a read-only Cloud Billing and Budget API adapter.
func NewBudgetReader(ctx context.Context) (BudgetReader, error) {
	if ctx == nil {
		return BudgetReader{}, errors.New("GCP budget context is required")
	}
	scopes := option.WithScopes(cloudPlatformScope)
	billing, err := cloudbilling.NewService(ctx, scopes)
	if err != nil {
		return BudgetReader{}, fmt.Errorf("create GCP Cloud Billing client: %w", err)
	}
	budgets, err := billingbudgets.NewService(ctx, scopes)
	if err != nil {
		return BudgetReader{}, fmt.Errorf("create GCP Cloud Billing Budget client: %w", err)
	}
	projects, err := cloudresourcemanager.NewService(ctx, scopes)
	if err != nil {
		return BudgetReader{}, fmt.Errorf("create GCP Resource Manager client: %w", err)
	}
	return BudgetReader{
		billingInfo: func(ctx context.Context, project string) (*cloudbilling.ProjectBillingInfo, error) {
			return billing.Projects.GetBillingInfo("projects/" + project).Context(ctx).Do()
		},
		projectNumber: func(ctx context.Context, project string) (int64, error) {
			identity, err := projects.Projects.Get(project).Context(ctx).Do()
			if err != nil {
				return 0, err
			}
			if identity == nil || identity.ProjectNumber <= 0 {
				return 0, errors.New("GCP Resource Manager returned no project number")
			}
			return identity.ProjectNumber, nil
		},
		listBudgets: func(ctx context.Context, parent, scope string) ([]*billingbudgets.GoogleCloudBillingBudgetsV1Budget, error) {
			result := make([]*billingbudgets.GoogleCloudBillingBudgetsV1Budget, 0)
			pageToken := ""
			for {
				call := budgets.BillingAccounts.Budgets.List(parent).Context(ctx).Scope(scope).PageSize(budgetPageSize)
				if pageToken != "" {
					call = call.PageToken(pageToken)
				}
				page, err := call.Do()
				if err != nil {
					return nil, err
				}
				if page != nil {
					result = append(result, page.Budgets...)
					if len(result) > budgetListLimit {
						return nil, fmt.Errorf("GCP budget list exceeds the safe read limit of %d entries", budgetListLimit)
					}
					pageToken = page.NextPageToken
				}
				if page == nil || pageToken == "" {
					return result, nil
				}
			}
		},
	}, nil
}

// Read returns only budgets whose filter is exactly the requested project.
// Account-wide and multi-project budgets are intentionally excluded because
// they cannot be attributed to one MageLift target without guessing.
func (r BudgetReader) Read(ctx context.Context, project string) (platform.CostBudgetReport, error) {
	if ctx == nil {
		return platform.CostBudgetReport{}, errors.New("GCP budget context is required")
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return platform.CostBudgetReport{}, errors.New("GCP project is required for budget lookup")
	}
	if r.billingInfo == nil || r.projectNumber == nil || r.listBudgets == nil {
		return platform.CostBudgetReport{}, errors.New("GCP budget reader is not configured")
	}
	now := time.Now().UTC()
	scope := "projects/" + project
	report := platform.CostBudgetReport{
		Scope: scope, Source: "GCP Cloud Billing Budget API", ObservedAt: now,
		Freshness: platform.CostFreshnessCurrent, State: platform.CostBudgetNotConfigured,
		Budgets: []platform.CostBudget{},
		Notice:  "GCP returned no budget scoped exactly to this project; account-wide and multi-project budgets are omitted",
	}
	info, err := r.billingInfo(ctx, project)
	if err != nil {
		return platform.CostBudgetReport{}, fmt.Errorf("read GCP billing account for project %q: %w", project, err)
	}
	if info == nil || strings.TrimSpace(info.ProjectId) != project {
		return platform.CostBudgetReport{}, errors.New("GCP billing info does not match the requested project")
	}
	account := strings.TrimSpace(info.BillingAccountName)
	if !info.BillingEnabled || account == "" || !strings.HasPrefix(account, "billingAccounts/") {
		return platform.CostBudgetReport{}, fmt.Errorf("GCP project %q has no usable open billing account", project)
	}
	number, err := r.projectNumber(ctx, project)
	if err != nil {
		return platform.CostBudgetReport{}, fmt.Errorf("read GCP project number for %q: %w", project, err)
	}
	if number <= 0 {
		return platform.CostBudgetReport{}, fmt.Errorf("read GCP project number for %q: invalid project number", project)
	}
	budgets, err := r.listBudgets(ctx, account, scope)
	if err != nil {
		return platform.CostBudgetReport{}, fmt.Errorf("list GCP budgets for %q: %w", account, err)
	}
	for _, budget := range budgets {
		if !budgetMatchesProject(budget, scope, number) {
			continue
		}
		report.Budgets = append(report.Budgets, normalizeBudget(budget, scope, report.Source, now))
	}
	sort.Slice(report.Budgets, func(i, j int) bool {
		return report.Budgets[i].Name < report.Budgets[j].Name
	})
	if len(report.Budgets) > 0 {
		report.State = platform.CostBudgetConfigured
		report.Notice = "GCP budget definitions and alert thresholds are configured for the exact project scope; MageLift does not claim ownership, the Budget API does not return current or forecast spend, and the budget does not block deployments"
	}
	return report, nil
}

// Cloud Billing may encode a project filter as either the configured project
// ID or its numeric resource name, even when the list request used the ID.
func budgetMatchesProject(budget *billingbudgets.GoogleCloudBillingBudgetsV1Budget, scope string, projectNumber int64) bool {
	if budget == nil || budget.BudgetFilter == nil || len(budget.BudgetFilter.Projects) != 1 {
		return false
	}
	projectScope := strings.TrimSpace(budget.BudgetFilter.Projects[0])
	return projectScope == scope || projectScope == fmt.Sprintf("projects/%d", projectNumber)
}

func normalizeBudget(budget *billingbudgets.GoogleCloudBillingBudgetsV1Budget, scope, source string, observedAt time.Time) platform.CostBudget {
	amount := platform.CostMoney{Source: source, ObservedAt: observedAt, Freshness: platform.CostFreshnessCurrent}
	if budget.Amount != nil && budget.Amount.SpecifiedAmount != nil {
		amount.Units = budget.Amount.SpecifiedAmount.Units
		amount.Nanos = budget.Amount.SpecifiedAmount.Nanos
		amount.Currency = budget.Amount.SpecifiedAmount.CurrencyCode
		amount.Basis = "specified"
	} else if budget.Amount != nil && budget.Amount.LastPeriodAmount != nil {
		amount.Basis = "last-period"
	}
	period := "custom"
	if budget.BudgetFilter != nil && budget.BudgetFilter.CalendarPeriod != "" {
		period = strings.ToLower(budget.BudgetFilter.CalendarPeriod)
	}
	thresholds := make([]platform.CostBudgetThreshold, 0, len(budget.ThresholdRules))
	for _, threshold := range budget.ThresholdRules {
		if threshold == nil {
			continue
		}
		thresholds = append(thresholds, platform.CostBudgetThreshold{Percent: threshold.ThresholdPercent, Basis: strings.ToLower(threshold.SpendBasis)})
	}
	sort.Slice(thresholds, func(i, j int) bool {
		if thresholds[i].Percent == thresholds[j].Percent {
			return thresholds[i].Basis < thresholds[j].Basis
		}
		return thresholds[i].Percent < thresholds[j].Percent
	})
	return platform.CostBudget{
		Name: budget.Name, DisplayName: budget.DisplayName, Scope: scope, Period: period,
		Amount: amount, Thresholds: thresholds, State: platform.CostBudgetConfigured,
		ScopeVerified: true, OwnershipVerified: false, Enforced: false,
	}
}
