package platform

import (
	"context"
	"strings"
	"testing"
)

type admissionTestModule struct {
	fakeModule
	admission PlanAdmission
}

func (m admissionTestModule) PlanAdmission() PlanAdmission { return m.admission }

type admissionTestAdapter struct {
	called bool
	next   PlannedStack
}

func (a *admissionTestAdapter) Admit(context.Context, PlannedStack) (PlannedStack, error) {
	a.called = true
	return a.next, nil
}

func TestAdmitPlanRunsProviderAdmissionAndPreservesIdentity(t *testing.T) {
	planned := fakePlannedStack{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate"}
	adapter := &admissionTestAdapter{next: planned}
	module := admissionTestModule{
		fakeModule: fakeModule{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierCertified, keys: RequiredOutputKeys()},
		admission:  adapter,
	}
	got, err := AdmitPlan(context.Background(), module, planned)
	if err != nil {
		t.Fatal(err)
	}
	if got != planned || !adapter.called {
		t.Fatalf("admitted plan = %#v, called = %t", got, adapter.called)
	}
}

func TestAdmitPlanRejectsProviderChangingStableIdentity(t *testing.T) {
	planned := fakePlannedStack{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate"}
	changed := admissionIdentityPlanned{fakePlannedStack: planned, region: "other-region"}
	module := admissionTestModule{
		fakeModule: fakeModule{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierCertified, keys: RequiredOutputKeys()},
		admission:  &admissionTestAdapter{next: changed},
	}
	_, err := AdmitPlan(context.Background(), module, planned)
	if err == nil || !strings.Contains(err.Error(), "region changed") {
		t.Fatalf("identity change error = %v", err)
	}
}

type admissionIdentityPlanned struct {
	fakePlannedStack
	region string
}

func (p admissionIdentityPlanned) Region() string { return p.region }
