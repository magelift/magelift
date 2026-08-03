package platform

import (
	"context"
	"testing"

	"github.com/magelift/magelift/internal/config"
	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type costStubModule struct{}

func (costStubModule) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "test.plain", Provider: "test", Runtime: "plain"}
}
func (costStubModule) CertificationTier() CertificationTier { return TierCertified }
func (costStubModule) Plan(config.Config, string, PlanOptions) (PlannedStack, error) {
	return nil, nil
}
func (costStubModule) Program(PlannedStack) (pulumi.RunFunc, error) { return nil, nil }
func (costStubModule) OutputKeys() []string                         { return RequiredOutputKeys() }

type costEstimatorModule struct {
	costStubModule
	estimator CostEstimator
}

func (m costEstimatorModule) CostEstimator() CostEstimator { return m.estimator }

type stubCostEstimator struct{}

func (stubCostEstimator) Estimate(context.Context, PlannedStack, config.Config, CostOptions) (CostReport, error) {
	return CostReport{Environment: "staging"}, nil
}

func TestModuleCostEstimator(t *testing.T) {
	t.Run("nil module", func(t *testing.T) {
		if ModuleCostEstimator(nil) != nil {
			t.Fatal("nil module should return nil estimator")
		}
	})
	t.Run("without cost interface", func(t *testing.T) {
		if ModuleCostEstimator(costStubModule{}) != nil {
			t.Fatal("module without HasCostEstimator should return nil")
		}
	})
	t.Run("with cost interface", func(t *testing.T) {
		want := stubCostEstimator{}
		got := ModuleCostEstimator(costEstimatorModule{estimator: want})
		if got == nil {
			t.Fatal("expected estimator")
		}
		report, err := got.Estimate(context.Background(), nil, config.Config{}, CostOptions{})
		if err != nil || report.Environment != "staging" {
			t.Fatalf("estimator = %#v err=%v", report, err)
		}
	})
}
