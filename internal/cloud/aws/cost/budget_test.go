package cost

import (
	"context"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	budgettypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

func TestBudgetReaderNormalizesAccountCostBudgetsAndNotifications(t *testing.T) {
	var budgetTokens []string
	var notificationTokens []string
	reader := BudgetReader{
		describeBudgets: func(_ context.Context, input *budgets.DescribeBudgetsInput) (*budgets.DescribeBudgetsOutput, error) {
			budgetTokens = append(budgetTokens, awssdk.ToString(input.NextToken))
			if len(budgetTokens) == 1 {
				return &budgets.DescribeBudgetsOutput{
					Budgets: []budgettypes.Budget{
						{BudgetName: awssdk.String("usage"), BudgetType: budgettypes.BudgetTypeUsage},
						{BudgetName: awssdk.String("shop"), BudgetType: budgettypes.BudgetTypeCost, TimeUnit: budgettypes.TimeUnitMonthly,
							BudgetLimit: &budgettypes.Spend{Amount: awssdk.String("123.45"), Unit: awssdk.String("USD")},
							CalculatedSpend: &budgettypes.CalculatedSpend{
								ActualSpend:     &budgettypes.Spend{Amount: awssdk.String("12.3"), Unit: awssdk.String("USD")},
								ForecastedSpend: &budgettypes.Spend{Amount: awssdk.String("140"), Unit: awssdk.String("USD")},
							},
						},
					},
					NextToken: awssdk.String("next"),
				}, nil
			}
			return &budgets.DescribeBudgetsOutput{}, nil
		},
		describeNotifications: func(_ context.Context, input *budgets.DescribeNotificationsForBudgetInput) (*budgets.DescribeNotificationsForBudgetOutput, error) {
			if awssdk.ToString(input.BudgetName) != "shop" || awssdk.ToString(input.AccountId) != "123456789012" {
				t.Fatalf("notification request = %#v", input)
			}
			notificationTokens = append(notificationTokens, awssdk.ToString(input.NextToken))
			if len(notificationTokens) == 1 {
				return &budgets.DescribeNotificationsForBudgetOutput{
					Notifications: []budgettypes.Notification{
						{Threshold: 80, ThresholdType: budgettypes.ThresholdTypePercentage, NotificationType: budgettypes.NotificationTypeActual, NotificationState: budgettypes.NotificationStateOk},
						{Threshold: 100, ThresholdType: budgettypes.ThresholdTypePercentage, NotificationType: budgettypes.NotificationTypeForecasted, NotificationState: budgettypes.NotificationStateAlarm},
						{Threshold: 50, ThresholdType: budgettypes.ThresholdTypeAbsoluteValue, NotificationType: budgettypes.NotificationTypeActual},
					},
					NextToken: awssdk.String("next-notification"),
				}, nil
			}
			return &budgets.DescribeNotificationsForBudgetOutput{}, nil
		},
	}
	report, err := reader.Read(context.Background(), "123456789012")
	if err != nil {
		t.Fatal(err)
	}
	if report.State != platform.CostBudgetConfigured || report.Scope != "aws/account/123456789012" || len(report.Budgets) != 1 {
		t.Fatalf("report = %#v", report)
	}
	budget := report.Budgets[0]
	if budget.Name != "shop" || budget.Period != "monthly" || !budget.ScopeVerified || budget.OwnershipVerified || budget.Enforced {
		t.Fatalf("budget = %#v", budget)
	}
	if budget.Amount.Units != 123 || budget.Amount.Nanos != 450000000 || budget.Amount.Currency != "USD" {
		t.Fatalf("amount = %#v", budget.Amount)
	}
	if budget.Actual == nil || budget.Actual.Units != 12 || budget.Actual.Nanos != 300000000 || budget.Forecast == nil || budget.Forecast.Units != 140 {
		t.Fatalf("spend = actual %#v forecast %#v", budget.Actual, budget.Forecast)
	}
	if len(budget.Thresholds) != 2 || budget.Thresholds[0].Percent != 80 || budget.Thresholds[0].State != "ok" || budget.Thresholds[1].Basis != "forecasted" || budget.Thresholds[1].State != "alarm" {
		t.Fatalf("thresholds = %#v", budget.Thresholds)
	}
	if !strings.Contains(report.Notice, "non-cost budgets are omitted") || !strings.Contains(report.Notice, "absolute-value notifications are omitted") {
		t.Fatalf("notice = %q", report.Notice)
	}
	if len(budgetTokens) != 2 || budgetTokens[0] != "" || budgetTokens[1] != "next" || len(notificationTokens) != 2 {
		t.Fatalf("pagination budget=%#v notifications=%#v", budgetTokens, notificationTokens)
	}
}

func TestBudgetReaderRejectsInvalidAccount(t *testing.T) {
	reader := BudgetReader{
		describeBudgets: func(context.Context, *budgets.DescribeBudgetsInput) (*budgets.DescribeBudgetsOutput, error) {
			return nil, nil
		},
		describeNotifications: func(context.Context, *budgets.DescribeNotificationsForBudgetInput) (*budgets.DescribeNotificationsForBudgetOutput, error) {
			return nil, nil
		},
	}
	if _, err := reader.Read(context.Background(), "not-an-account"); err == nil || !strings.Contains(err.Error(), "12-digit") {
		t.Fatalf("error = %v", err)
	}
}

func TestEstimatorBudgetFlagUsesReadOnlyReader(t *testing.T) {
	reader := BudgetReader{
		describeBudgets: func(context.Context, *budgets.DescribeBudgetsInput) (*budgets.DescribeBudgetsOutput, error) {
			return &budgets.DescribeBudgetsOutput{Budgets: []budgettypes.Budget{{BudgetName: awssdk.String("shop"), BudgetType: budgettypes.BudgetTypeCost, BudgetLimit: &budgettypes.Spend{Amount: awssdk.String("10"), Unit: awssdk.String("USD")}}}}, nil
		},
		describeNotifications: func(context.Context, *budgets.DescribeNotificationsForBudgetInput) (*budgets.DescribeNotificationsForBudgetOutput, error) {
			return &budgets.DescribeNotificationsForBudgetOutput{}, nil
		},
	}
	called := false
	estimator := Estimator{NewBudgetReader: func(context.Context, string) (BudgetReader, error) {
		called = true
		return reader, nil
	}}
	planned := budgetPlanned{environment: "staging", region: "eu-west-3"}
	report, err := estimator.Estimate(context.Background(), planned, config.Config{
		Account: "123456789012", Target: config.Target{Provider: "aws", Runtime: "ecs-fargate"}, Defaults: config.Defaults{Region: "eu-west-3"},
	}, platform.CostOptions{Budget: true})
	if err != nil {
		t.Fatal(err)
	}
	if !called || report.Budget == nil || report.Budget.State != platform.CostBudgetConfigured {
		t.Fatalf("called=%v budget=%#v", called, report.Budget)
	}
}

func TestEstimatorPreviewBudgetDoesNotInheritAccountBudgets(t *testing.T) {
	reader := BudgetReader{
		describeBudgets: func(context.Context, *budgets.DescribeBudgetsInput) (*budgets.DescribeBudgetsOutput, error) {
			return &budgets.DescribeBudgetsOutput{Budgets: []budgettypes.Budget{{BudgetName: awssdk.String("production"), BudgetType: budgettypes.BudgetTypeCost, BudgetLimit: &budgettypes.Spend{Amount: awssdk.String("9999"), Unit: awssdk.String("USD")}}}}, nil
		},
		describeNotifications: func(context.Context, *budgets.DescribeNotificationsForBudgetInput) (*budgets.DescribeNotificationsForBudgetOutput, error) {
			return &budgets.DescribeNotificationsForBudgetOutput{}, nil
		},
	}
	estimator := Estimator{NewBudgetReader: func(context.Context, string) (BudgetReader, error) {
		return reader, nil
	}}
	report, err := estimator.Estimate(context.Background(), budgetPlanned{environment: "pr-12", region: "eu-west-3"}, config.Config{
		Account: "123456789012", Class: "preview", Target: config.Target{Provider: "aws", Runtime: "ecs-fargate"}, Defaults: config.Defaults{Region: "eu-west-3"},
	}, platform.CostOptions{Budget: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Budget == nil || report.Budget.State != platform.CostBudgetNotConfigured || len(report.Budget.Budgets) != 0 {
		t.Fatalf("preview inherited account budgets: %#v", report.Budget)
	}
	if !strings.Contains(report.Budget.Notice, "do not inherit") {
		t.Fatalf("preview budget notice = %q", report.Budget.Notice)
	}
	if report.Budget.Scope != "preview/pr-12" {
		t.Fatalf("preview budget scope = %q", report.Budget.Scope)
	}
}

type budgetPlanned struct {
	environment string
	region      string
}

func (p budgetPlanned) StackName() string        { return "shop-staging" }
func (p budgetPlanned) Provider() sdk.ProviderID { return "aws" }
func (p budgetPlanned) Runtime() sdk.RuntimeID   { return "ecs-fargate" }
func (p budgetPlanned) Project() string          { return "shop" }
func (p budgetPlanned) Environment() string      { return p.environment }
func (p budgetPlanned) Region() string           { return p.region }
func (p budgetPlanned) CertificationTier() platform.CertificationTier {
	return platform.TierCertified
}
func (p budgetPlanned) EnvironmentClass() string { return "staging" }
func (p budgetPlanned) Protected() bool          { return false }
func (p budgetPlanned) ImageDigest() string      { return "" }
func (p budgetPlanned) WithImageDigest(string) (platform.PlannedStack, error) {
	return p, nil
}
func (p budgetPlanned) TargetDescriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "aws-ecs-fargate", Provider: "aws", Runtime: "ecs-fargate"}
}

func TestParseAWSDecimal(t *testing.T) {
	for _, test := range []struct {
		input string
		units int64
		nanos int64
	}{
		{input: "0.000000001", units: 0, nanos: 1},
		{input: "-0.5", units: -1, nanos: 500000000},
		{input: "42", units: 42, nanos: 0},
	} {
		got, err := parseAWSDecimal(test.input)
		if err != nil || got.units != test.units || got.nanos != test.nanos {
			t.Fatalf("parseAWSDecimal(%q) = %#v, %v", test.input, got, err)
		}
	}
	if _, err := parseAWSDecimal("0.1234567891"); err == nil {
		t.Fatal("accepted more than nine fractional digits")
	}
}
