package cost

import (
	"context"
	"testing"

	billingbudgets "google.golang.org/api/billingbudgets/v1"
	cloudbilling "google.golang.org/api/cloudbilling/v1"

	"github.com/magelift/magelift/internal/platform"
)

func TestBudgetReaderKeepsOnlyExactProjectScopes(t *testing.T) {
	reader := BudgetReader{
		billingInfo: func(context.Context, string) (*cloudbilling.ProjectBillingInfo, error) {
			return &cloudbilling.ProjectBillingInfo{ProjectId: "shop-project", BillingEnabled: true, BillingAccountName: "billingAccounts/123"}, nil
		},
		projectNumber: func(context.Context, string) (int64, error) {
			return 397359300925, nil
		},
		listBudgets: func(context.Context, string, string) ([]*billingbudgets.GoogleCloudBillingBudgetsV1Budget, error) {
			return []*billingbudgets.GoogleCloudBillingBudgetsV1Budget{
				{Name: "billingAccounts/123/budgets/exact", DisplayName: "Shop", BudgetFilter: &billingbudgets.GoogleCloudBillingBudgetsV1Filter{Projects: []string{"projects/397359300925"}, CalendarPeriod: "MONTH"}, Amount: &billingbudgets.GoogleCloudBillingBudgetsV1BudgetAmount{SpecifiedAmount: &billingbudgets.GoogleTypeMoney{Units: 200, Nanos: 500000000, CurrencyCode: "USD"}}, ThresholdRules: []*billingbudgets.GoogleCloudBillingBudgetsV1ThresholdRule{
					{ThresholdPercent: 0.9, SpendBasis: "FORECASTED_SPEND"},
					{ThresholdPercent: 0.5, SpendBasis: "CURRENT_SPEND"},
				}},
				{Name: "billingAccounts/123/budgets/account", DisplayName: "Account", BudgetFilter: &billingbudgets.GoogleCloudBillingBudgetsV1Filter{}, Amount: &billingbudgets.GoogleCloudBillingBudgetsV1BudgetAmount{LastPeriodAmount: &billingbudgets.GoogleCloudBillingBudgetsV1LastPeriodAmount{}}},
				{Name: "billingAccounts/123/budgets/foreign", DisplayName: "Foreign", BudgetFilter: &billingbudgets.GoogleCloudBillingBudgetsV1Filter{Projects: []string{"projects/123456789"}}},
				{Name: "billingAccounts/123/budgets/multi", DisplayName: "Multi", BudgetFilter: &billingbudgets.GoogleCloudBillingBudgetsV1Filter{Projects: []string{"projects/shop-project", "projects/other"}}},
			}, nil
		},
	}
	report, err := reader.Read(context.Background(), "shop-project")
	if err != nil {
		t.Fatal(err)
	}
	if report.State != platform.CostBudgetConfigured || len(report.Budgets) != 1 {
		t.Fatalf("report = %#v", report)
	}
	budget := report.Budgets[0]
	if budget.Name != "billingAccounts/123/budgets/exact" || budget.Scope != "projects/shop-project" || budget.Period != "month" || !budget.ScopeVerified || budget.OwnershipVerified || budget.Enforced {
		t.Fatalf("budget = %#v", budget)
	}
	if budget.Amount.Units != 200 || budget.Amount.Nanos != 500000000 || budget.Amount.Currency != "USD" {
		t.Fatalf("amount = %#v", budget.Amount)
	}
	if len(budget.Thresholds) != 2 || budget.Thresholds[0].Percent != 0.5 || budget.Thresholds[1].Basis != "forecasted_spend" {
		t.Fatalf("thresholds = %#v", budget.Thresholds)
	}
	if report.Actual != nil || report.Forecast != nil {
		t.Fatal("budget API result must not claim actual or forecast spend")
	}
}

func TestBudgetMatchesProjectAcceptsIDOrNumber(t *testing.T) {
	for name, projectScope := range map[string]string{
		"project ID":     "projects/shop-project",
		"project number": "projects/397359300925",
	} {
		t.Run(name, func(t *testing.T) {
			budget := &billingbudgets.GoogleCloudBillingBudgetsV1Budget{
				BudgetFilter: &billingbudgets.GoogleCloudBillingBudgetsV1Filter{Projects: []string{projectScope}},
			}
			if !budgetMatchesProject(budget, "projects/shop-project", 397359300925) {
				t.Fatalf("budget scope %q was not accepted", projectScope)
			}
		})
	}
}

func TestBudgetReaderReportsMissingExactScope(t *testing.T) {
	reader := BudgetReader{
		billingInfo: func(context.Context, string) (*cloudbilling.ProjectBillingInfo, error) {
			return &cloudbilling.ProjectBillingInfo{ProjectId: "shop-project", BillingEnabled: true, BillingAccountName: "billingAccounts/123"}, nil
		},
		projectNumber: func(context.Context, string) (int64, error) {
			return 397359300925, nil
		},
		listBudgets: func(context.Context, string, string) ([]*billingbudgets.GoogleCloudBillingBudgetsV1Budget, error) {
			return []*billingbudgets.GoogleCloudBillingBudgetsV1Budget{{Name: "account-wide", BudgetFilter: &billingbudgets.GoogleCloudBillingBudgetsV1Filter{}}}, nil
		},
	}
	report, err := reader.Read(context.Background(), "shop-project")
	if err != nil {
		t.Fatal(err)
	}
	if report.State != platform.CostBudgetNotConfigured || len(report.Budgets) != 0 {
		t.Fatalf("report = %#v", report)
	}
}
