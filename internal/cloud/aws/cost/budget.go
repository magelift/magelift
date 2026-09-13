package cost

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/budgets"
	budgettypes "github.com/aws/aws-sdk-go-v2/service/budgets/types"

	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
	"github.com/magelift/magelift/internal/platform"
)

const awsBudgetPageSize int32 = 1000

const awsBudgetListLimit = 1000

var awsAccountIDPattern = regexp.MustCompile(`^[0-9]{12}$`)

// BudgetReader exposes only the AWS Budgets read paths needed by `cost
// --budget`. It has no create, update, delete, or notification-mutation path.
type BudgetReader struct {
	describeBudgets       func(context.Context, *budgets.DescribeBudgetsInput) (*budgets.DescribeBudgetsOutput, error)
	describeNotifications func(context.Context, *budgets.DescribeNotificationsForBudgetInput) (*budgets.DescribeNotificationsForBudgetOutput, error)
}

// NewBudgetReader creates a read-only AWS Budgets client for the selected
// region. The account ID is supplied to Read so the caller can keep the
// configured target account in the operation boundary.
func NewBudgetReader(ctx context.Context, region string) (BudgetReader, error) {
	if ctx == nil {
		return BudgetReader{}, errors.New("AWS budget context is required")
	}
	region = strings.TrimSpace(region)
	if region == "" {
		return BudgetReader{}, errors.New("AWS budget region is required")
	}
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return BudgetReader{}, err
	}
	configuration, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(region))
	if err != nil {
		return BudgetReader{}, fmt.Errorf("load AWS budget configuration: %w", err)
	}
	client := budgets.NewFromConfig(configuration, func(options *budgets.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	})
	return BudgetReader{
		describeBudgets: func(ctx context.Context, input *budgets.DescribeBudgetsInput) (*budgets.DescribeBudgetsOutput, error) {
			return client.DescribeBudgets(ctx, input)
		},
		describeNotifications: func(ctx context.Context, input *budgets.DescribeNotificationsForBudgetInput) (*budgets.DescribeNotificationsForBudgetOutput, error) {
			return client.DescribeNotificationsForBudget(ctx, input)
		},
	}, nil
}

// Read returns account-scoped AWS cost budgets. AWS does not provide a
// MageLift environment ownership marker in this read path, so ownership and
// deployment enforcement remain false even when the account scope matches.
func (r BudgetReader) Read(ctx context.Context, accountID string) (platform.CostBudgetReport, error) {
	if ctx == nil {
		return platform.CostBudgetReport{}, errors.New("AWS budget context is required")
	}
	accountID = strings.TrimSpace(accountID)
	if !awsAccountIDPattern.MatchString(accountID) {
		return platform.CostBudgetReport{}, errors.New("AWS account must be a 12-digit account ID for budget lookup")
	}
	if r.describeBudgets == nil || r.describeNotifications == nil {
		return platform.CostBudgetReport{}, errors.New("AWS budget reader is not configured")
	}

	now := time.Now().UTC()
	scope := "aws/account/" + accountID
	report := platform.CostBudgetReport{
		Scope: scope, Source: "AWS Budgets API", ObservedAt: now,
		Freshness: platform.CostFreshnessCurrent, State: platform.CostBudgetNotConfigured,
		Budgets: []platform.CostBudget{},
		Notice:  "AWS budgets are account-scoped; MageLift does not claim environment ownership or deployment enforcement for unmarked budgets",
	}
	budgetsForAccount, err := r.listBudgets(ctx, accountID)
	if err != nil {
		return platform.CostBudgetReport{}, fmt.Errorf("list AWS budgets for account %q: %w", accountID, err)
	}
	omittedNonCost := false
	omittedAbsolute := false
	for _, budget := range budgetsForAccount {
		if budget.BudgetType != budgettypes.BudgetTypeCost {
			omittedNonCost = true
			continue
		}
		name := strings.TrimSpace(awssdk.ToString(budget.BudgetName))
		if name == "" {
			return platform.CostBudgetReport{}, errors.New("AWS returned a cost budget without a name")
		}
		notifications, err := r.listNotifications(ctx, accountID, name)
		if err != nil {
			return platform.CostBudgetReport{}, fmt.Errorf("list notifications for AWS budget %q: %w", name, err)
		}
		normalized, skippedAbsolute, err := normalizeAWSBudget(budget, notifications, scope, report.Source, now)
		if err != nil {
			return platform.CostBudgetReport{}, fmt.Errorf("normalize AWS budget %q: %w", name, err)
		}
		omittedAbsolute = omittedAbsolute || skippedAbsolute
		report.Budgets = append(report.Budgets, normalized)
	}
	sort.Slice(report.Budgets, func(i, j int) bool { return report.Budgets[i].Name < report.Budgets[j].Name })
	if len(report.Budgets) > 0 {
		report.State = platform.CostBudgetConfigured
	}
	if omittedNonCost || omittedAbsolute {
		parts := []string{}
		if omittedNonCost {
			parts = append(parts, "non-cost budgets are omitted")
		}
		if omittedAbsolute {
			parts = append(parts, "absolute-value notifications are omitted because the core threshold contract is percentage-based")
		}
		report.Notice += "; " + strings.Join(parts, "; ")
	}
	if len(report.Budgets) == 0 {
		report.Notice += "; no AWS cost budget was found for this account"
	}
	return report, nil
}

func (r BudgetReader) listBudgets(ctx context.Context, accountID string) ([]budgettypes.Budget, error) {
	result := make([]budgettypes.Budget, 0)
	var token *string
	for {
		page, err := r.describeBudgets(ctx, &budgets.DescribeBudgetsInput{
			AccountId: accountIDPointer(accountID), MaxResults: awssdk.Int32(awsBudgetPageSize), NextToken: token,
			ShowFilterExpression: awssdk.Bool(true),
		})
		if err != nil {
			return nil, err
		}
		if page == nil {
			return nil, errors.New("AWS returned an empty budget page")
		}
		result = append(result, page.Budgets...)
		if len(result) > awsBudgetListLimit {
			return nil, fmt.Errorf("AWS budget list exceeds the safe read limit of %d entries", awsBudgetListLimit)
		}
		if strings.TrimSpace(awssdk.ToString(page.NextToken)) == "" {
			return result, nil
		}
		token = page.NextToken
	}
}

func (r BudgetReader) listNotifications(ctx context.Context, accountID, budgetName string) ([]budgettypes.Notification, error) {
	result := make([]budgettypes.Notification, 0)
	var token *string
	for {
		page, err := r.describeNotifications(ctx, &budgets.DescribeNotificationsForBudgetInput{
			AccountId: accountIDPointer(accountID), BudgetName: awssdk.String(budgetName),
			MaxResults: awssdk.Int32(awsBudgetPageSize), NextToken: token,
		})
		if err != nil {
			return nil, err
		}
		if page == nil {
			return nil, errors.New("AWS returned an empty notification page")
		}
		result = append(result, page.Notifications...)
		if len(result) > awsBudgetListLimit {
			return nil, fmt.Errorf("AWS notification list exceeds the safe read limit of %d entries", awsBudgetListLimit)
		}
		if strings.TrimSpace(awssdk.ToString(page.NextToken)) == "" {
			return result, nil
		}
		token = page.NextToken
	}
}

func normalizeAWSBudget(budget budgettypes.Budget, notifications []budgettypes.Notification, scope, source string, observedAt time.Time) (platform.CostBudget, bool, error) {
	amount, err := normalizeAWSSpend(budget.BudgetLimit, source, observedAt, "limit")
	if err != nil {
		return platform.CostBudget{}, false, err
	}
	name := strings.TrimSpace(awssdk.ToString(budget.BudgetName))
	result := platform.CostBudget{
		Name: name, DisplayName: name, Scope: scope, Period: strings.ToLower(string(budget.TimeUnit)),
		Amount: amount, Thresholds: []platform.CostBudgetThreshold{}, State: platform.CostBudgetConfigured,
		ScopeVerified: true, OwnershipVerified: false, Enforced: false,
	}
	if budget.CalculatedSpend != nil {
		if budget.CalculatedSpend.ActualSpend != nil {
			actual, err := normalizeAWSSpend(budget.CalculatedSpend.ActualSpend, source, observedAt, "actual")
			if err != nil {
				return platform.CostBudget{}, false, fmt.Errorf("actual spend: %w", err)
			}
			result.Actual = &actual
		}
		if budget.CalculatedSpend.ForecastedSpend != nil {
			forecast, err := normalizeAWSSpend(budget.CalculatedSpend.ForecastedSpend, source, observedAt, "forecast")
			if err != nil {
				return platform.CostBudget{}, false, fmt.Errorf("forecasted spend: %w", err)
			}
			result.Forecast = &forecast
		}
	}
	skippedAbsolute := false
	for _, notification := range notifications {
		if notification.ThresholdType != budgettypes.ThresholdTypePercentage {
			skippedAbsolute = true
			continue
		}
		result.Thresholds = append(result.Thresholds, platform.CostBudgetThreshold{
			Percent: notification.Threshold,
			Basis:   strings.ToLower(string(notification.NotificationType)),
			State:   strings.ToLower(string(notification.NotificationState)),
		})
	}
	sort.Slice(result.Thresholds, func(i, j int) bool {
		if result.Thresholds[i].Percent == result.Thresholds[j].Percent {
			return result.Thresholds[i].Basis < result.Thresholds[j].Basis
		}
		return result.Thresholds[i].Percent < result.Thresholds[j].Percent
	})
	return result, skippedAbsolute, nil
}

func normalizeAWSSpend(spend *budgettypes.Spend, source string, observedAt time.Time, basis string) (platform.CostMoney, error) {
	if spend == nil || strings.TrimSpace(awssdk.ToString(spend.Amount)) == "" {
		return platform.CostMoney{}, errors.New("AWS spend amount is missing")
	}
	unit := strings.TrimSpace(awssdk.ToString(spend.Unit))
	if unit == "" {
		return platform.CostMoney{}, errors.New("AWS spend unit is missing")
	}
	amount, err := parseAWSDecimal(awssdk.ToString(spend.Amount))
	if err != nil {
		return platform.CostMoney{}, err
	}
	return platform.CostMoney{
		Units: amount.units, Nanos: amount.nanos, Currency: unit, Basis: basis,
		Source: source, ObservedAt: observedAt, Freshness: platform.CostFreshnessCurrent,
	}, nil
}

type awsMoneyAmount struct {
	units int64
	nanos int64
}

func parseAWSDecimal(raw string) (awsMoneyAmount, error) {
	rational, ok := new(big.Rat).SetString(strings.TrimSpace(raw))
	if !ok {
		return awsMoneyAmount{}, fmt.Errorf("invalid AWS decimal amount %q", raw)
	}
	scale := big.NewInt(1_000_000_000)
	numerator := new(big.Int).Mul(rational.Num(), scale)
	quotient, remainder := new(big.Int).QuoRem(numerator, rational.Denom(), new(big.Int))
	if remainder.Sign() != 0 {
		return awsMoneyAmount{}, fmt.Errorf("AWS decimal amount %q has more than 9 fractional digits", raw)
	}
	units, nanos := new(big.Int).QuoRem(quotient, scale, new(big.Int))
	if nanos.Sign() < 0 {
		units.Sub(units, big.NewInt(1))
		nanos.Add(nanos, scale)
	}
	if !units.IsInt64() || !nanos.IsInt64() {
		return awsMoneyAmount{}, fmt.Errorf("AWS decimal amount %q is outside the supported money range", raw)
	}
	return awsMoneyAmount{units: units.Int64(), nanos: nanos.Int64()}, nil
}

func accountIDPointer(accountID string) *string {
	return awssdk.String(accountID)
}
