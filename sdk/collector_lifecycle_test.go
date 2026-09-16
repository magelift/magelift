package sdk

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeCollectorDeploymentAdapter struct {
	plan             CollectorDeploymentPlan
	planErr          error
	result           func(CollectorDeploymentExecutionRequest) CollectorDeploymentExecutionResult
	executeErr       error
	planCalls        int
	executeCalls     int
	executionRequest CollectorDeploymentExecutionRequest
}

func (adapter *fakeCollectorDeploymentAdapter) PlanCollector(_ context.Context, request CollectorDeploymentPlanRequest) (CollectorDeploymentPlan, error) {
	adapter.planCalls++
	if adapter.planErr != nil {
		return CollectorDeploymentPlan{}, adapter.planErr
	}
	if adapter.plan.AdapterID == "" {
		adapter.plan = testCollectorDeploymentPlan(request)
	}
	return adapter.plan, nil
}

func (adapter *fakeCollectorDeploymentAdapter) ExecuteCollector(_ context.Context, request CollectorDeploymentExecutionRequest) (CollectorDeploymentExecutionResult, error) {
	adapter.executeCalls++
	adapter.executionRequest = request
	if adapter.executeErr != nil {
		return CollectorDeploymentExecutionResult{}, adapter.executeErr
	}
	if adapter.result != nil {
		return adapter.result(request), nil
	}
	return testCollectorDeploymentResult(request), nil
}

func TestRunCollectorDeploymentLifecycle(t *testing.T) {
	tests := []struct {
		name         string
		action       CollectorDeploymentAction
		wantReady    bool
		wantHealth   bool
		wantRollback bool
		wantCleanup  bool
	}{
		{name: "apply", action: CollectorApply, wantReady: true, wantHealth: true},
		{name: "verify", action: CollectorVerify, wantReady: true, wantHealth: true},
		{name: "rollback", action: CollectorRollback, wantRollback: true},
		{name: "destroy", action: CollectorDestroy, wantCleanup: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			adapter := &fakeCollectorDeploymentAdapter{}
			plan, result, err := RunCollectorDeploymentLifecycle(
				context.Background(), adapter, testCollectorDeploymentPlanRequest("magelift/test/collector"),
				test.action, "collector/"+test.name+"/1", []string{"collector:resource/1"}, "approval/collector/1",
			)
			if err != nil {
				t.Fatalf("RunCollectorDeploymentLifecycle() error = %v", err)
			}
			if plan.AdapterID != "test.collector" || plan.OwnershipMarker != "magelift/test/collector" {
				t.Fatalf("plan = %#v, want the adapter plan", plan)
			}
			if result.Action != test.action || result.OwnershipMarker != plan.OwnershipMarker {
				t.Fatalf("result = %#v, want action %q and ownership %q", result, test.action, plan.OwnershipMarker)
			}
			if result.ReadyVerified != test.wantReady || result.HealthVerified != test.wantHealth || result.RollbackVerified != test.wantRollback || result.CleanupVerified != test.wantCleanup {
				t.Fatalf("result proof flags = %#v, want ready=%t health=%t rollback=%t cleanup=%t", result, test.wantReady, test.wantHealth, test.wantRollback, test.wantCleanup)
			}
			if adapter.planCalls != 1 || adapter.executeCalls != 1 {
				t.Fatalf("adapter calls = plan %d, execute %d, want 1 each", adapter.planCalls, adapter.executeCalls)
			}
			if adapter.executionRequest.Action != test.action || adapter.executionRequest.IdempotencyKey != "collector/"+test.name+"/1" {
				t.Fatalf("execution request = %#v, want normalized action and idempotency key", adapter.executionRequest)
			}
		})
	}
}

func TestRunCollectorDeploymentLifecycleValidatesContextAndAdapter(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		adapter CollectorDeploymentAdapter
		want    string
	}{
		{name: "nil context", ctx: nil, adapter: &fakeCollectorDeploymentAdapter{}, want: "context is required"},
		{name: "nil adapter", ctx: context.Background(), adapter: nil, want: "adapter is required"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := RunCollectorDeploymentLifecycle(test.ctx, test.adapter, testCollectorDeploymentPlanRequest("magelift/test/collector"), CollectorApply, "collector/apply/1", nil, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RunCollectorDeploymentLifecycle() error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRunCollectorDeploymentLifecycleRejectsInvalidPlanRequestBeforeMutation(t *testing.T) {
	adapter := &fakeCollectorDeploymentAdapter{}
	request := testCollectorDeploymentPlanRequest("magelift/test/collector")
	request.Distribution = "opentelemetry collector"

	_, _, err := RunCollectorDeploymentLifecycle(context.Background(), adapter, request, CollectorApply, "collector/apply/1", nil, "")
	if err == nil || !strings.Contains(err.Error(), "distribution") {
		t.Fatalf("RunCollectorDeploymentLifecycle() error = %v, want distribution validation", err)
	}
	if adapter.planCalls != 0 || adapter.executeCalls != 0 {
		t.Fatalf("invalid request reached adapter: plan calls = %d, execute calls = %d", adapter.planCalls, adapter.executeCalls)
	}
}

func TestRunCollectorDeploymentLifecycleRejectsPlanDriftBeforeMutation(t *testing.T) {
	request := testCollectorDeploymentPlanRequest("magelift/test/collector")
	driftedPlan := testCollectorDeploymentPlan(request)
	driftedPlan.TargetRuntime = "ecs-fargate"
	adapter := &fakeCollectorDeploymentAdapter{plan: driftedPlan}

	_, _, err := RunCollectorDeploymentLifecycle(context.Background(), adapter, request, CollectorApply, "collector/apply/1", nil, "")
	if err == nil || !strings.Contains(err.Error(), "validate collector deployment plan") || !strings.Contains(err.Error(), "target or distribution") {
		t.Fatalf("RunCollectorDeploymentLifecycle() error = %v, want plan-drift validation", err)
	}
	if adapter.planCalls != 1 || adapter.executeCalls != 0 {
		t.Fatalf("drifted plan reached mutation: plan calls = %d, execute calls = %d", adapter.planCalls, adapter.executeCalls)
	}
}

func TestRunCollectorDeploymentLifecycleRejectsInvalidExecutionRequestBeforeMutation(t *testing.T) {
	tests := []struct {
		name           string
		action         CollectorDeploymentAction
		idempotencyKey string
		resourceRefs   []string
		want           string
	}{
		{name: "invalid action", action: CollectorDeploymentAction("pause"), idempotencyKey: "collector/pause/1", want: "invalid collector deployment action"},
		{name: "missing idempotency key", action: CollectorApply, want: "idempotency key"},
		{name: "non opaque resource reference", action: CollectorApply, idempotencyKey: "collector/apply/1", resourceRefs: []string{"collector:resource\n1"}, want: "resource references"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			adapter := &fakeCollectorDeploymentAdapter{}
			_, _, err := RunCollectorDeploymentLifecycle(context.Background(), adapter, testCollectorDeploymentPlanRequest("magelift/test/collector"), test.action, test.idempotencyKey, test.resourceRefs, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RunCollectorDeploymentLifecycle() error = %v, want %q", err, test.want)
			}
			if adapter.planCalls != 1 || adapter.executeCalls != 0 {
				t.Fatalf("invalid execution request reached mutation: plan calls = %d, execute calls = %d", adapter.planCalls, adapter.executeCalls)
			}
		})
	}
}

func TestRunCollectorDeploymentLifecycleRejectsResultDrift(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CollectorDeploymentExecutionResult)
		want   string
	}{
		{name: "action drift", mutate: func(result *CollectorDeploymentExecutionResult) { result.Action = CollectorVerify }, want: "action"},
		{name: "ownership drift", mutate: func(result *CollectorDeploymentExecutionResult) { result.OwnershipMarker = "magelift/other" }, want: "ownership marker"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			adapter := &fakeCollectorDeploymentAdapter{result: func(request CollectorDeploymentExecutionRequest) CollectorDeploymentExecutionResult {
				result := testCollectorDeploymentResult(request)
				test.mutate(&result)
				return result
			}}
			plan, result, err := RunCollectorDeploymentLifecycle(context.Background(), adapter, testCollectorDeploymentPlanRequest("magelift/test/collector"), CollectorApply, "collector/apply/1", nil, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RunCollectorDeploymentLifecycle() error = %v, want %q", err, test.want)
			}
			if plan.AdapterID == "" {
				t.Fatalf("plan = %#v, want the validated plan returned on result failure", plan)
			}
			if result.Action != "" || result.OperationID != "" || len(result.ResourceRefs) != 0 || len(result.ProofRefs) != 0 || result.OwnershipMarker != "" || result.OwnershipVerified || result.IdempotencyVerified || result.ReadyVerified || result.HealthVerified || result.CleanupVerified || result.RollbackVerified {
				t.Fatalf("result = %#v, want zero result on validation failure", result)
			}
			if adapter.executeCalls != 1 {
				t.Fatalf("execute calls = %d, want 1", adapter.executeCalls)
			}
		})
	}
}

func TestRunCollectorDeploymentLifecycleRejectsMissingActionProofs(t *testing.T) {
	tests := []struct {
		name   string
		action CollectorDeploymentAction
		mutate func(*CollectorDeploymentExecutionResult)
		want   string
	}{
		{name: "apply readiness", action: CollectorApply, mutate: func(result *CollectorDeploymentExecutionResult) { result.ReadyVerified = false }, want: "readiness"},
		{name: "verify health", action: CollectorVerify, mutate: func(result *CollectorDeploymentExecutionResult) { result.HealthVerified = false }, want: "health"},
		{name: "apply signal delivery", action: CollectorApply, mutate: func(result *CollectorDeploymentExecutionResult) {
			result.ProofRefs = []string{"collector:ownership", "collector:readiness", "collector:health"}
		}, want: "signal-delivery"},
		{name: "apply unnamespaced signal delivery", action: CollectorApply, mutate: func(result *CollectorDeploymentExecutionResult) {
			result.ProofRefs = []string{"collector:ownership", "collector:readiness", "collector:health", "signal-delivery"}
		}, want: "signal-delivery"},
		{name: "rollback", action: CollectorRollback, mutate: func(result *CollectorDeploymentExecutionResult) { result.RollbackVerified = false }, want: "rollback"},
		{name: "destroy", action: CollectorDestroy, mutate: func(result *CollectorDeploymentExecutionResult) { result.CleanupVerified = false }, want: "cleanup"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			adapter := &fakeCollectorDeploymentAdapter{result: func(request CollectorDeploymentExecutionRequest) CollectorDeploymentExecutionResult {
				result := testCollectorDeploymentResult(request)
				test.mutate(&result)
				return result
			}}
			_, _, err := RunCollectorDeploymentLifecycle(context.Background(), adapter, testCollectorDeploymentPlanRequest("magelift/test/collector"), test.action, "collector/"+test.name, nil, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RunCollectorDeploymentLifecycle() error = %v, want %q", err, test.want)
			}
			if adapter.executeCalls != 1 {
				t.Fatalf("execute calls = %d, want 1", adapter.executeCalls)
			}
		})
	}
}

func TestValidateCollectorDeploymentExecutionResultRejectsOpaqueReferences(t *testing.T) {
	request := CollectorDeploymentExecutionRequest{
		Plan:            testCollectorDeploymentPlan(testCollectorDeploymentPlanRequest("magelift/test/collector")),
		Action:          CollectorApply,
		IdempotencyKey:  "collector/apply/1",
		OwnershipMarker: "magelift/test/collector",
	}

	tests := []struct {
		name   string
		mutate func(*CollectorDeploymentExecutionResult)
	}{
		{name: "operation ID", mutate: func(result *CollectorDeploymentExecutionResult) { result.OperationID = "operation\n1" }},
		{name: "resource reference", mutate: func(result *CollectorDeploymentExecutionResult) { result.ResourceRefs = []string{"resource\n1"} }},
		{name: "proof reference", mutate: func(result *CollectorDeploymentExecutionResult) {
			result.ProofRefs = []string{"collector:signal-delivery\n1"}
		}},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result := testCollectorDeploymentResult(request)
			test.mutate(&result)
			if err := ValidateCollectorDeploymentExecutionResult(request, result); err == nil {
				t.Fatal("ValidateCollectorDeploymentExecutionResult() error = nil, want opaque-reference validation")
			}
		})
	}
}

func TestRunCollectorDeploymentLifecyclePropagatesAdapterErrors(t *testing.T) {
	planErr := errors.New("plan failed")
	adapter := &fakeCollectorDeploymentAdapter{planErr: planErr}
	_, _, err := RunCollectorDeploymentLifecycle(context.Background(), adapter, testCollectorDeploymentPlanRequest("magelift/test/collector"), CollectorApply, "collector/apply/1", nil, "")
	if !errors.Is(err, planErr) || !strings.Contains(err.Error(), "plan collector deployment lifecycle") {
		t.Fatalf("plan error = %v, want wrapped plan error", err)
	}
	if adapter.executeCalls != 0 {
		t.Fatalf("execute calls = %d, want 0 after plan failure", adapter.executeCalls)
	}

	executeErr := errors.New("execute failed")
	adapter = &fakeCollectorDeploymentAdapter{executeErr: executeErr}
	_, _, err = RunCollectorDeploymentLifecycle(context.Background(), adapter, testCollectorDeploymentPlanRequest("magelift/test/collector"), CollectorApply, "collector/apply/1", nil, "")
	if !errors.Is(err, executeErr) || !strings.Contains(err.Error(), "execute collector deployment lifecycle") {
		t.Fatalf("execute error = %v, want wrapped execute error", err)
	}
	if adapter.planCalls != 1 || adapter.executeCalls != 1 {
		t.Fatalf("adapter calls after execute failure = plan %d, execute %d, want 1 each", adapter.planCalls, adapter.executeCalls)
	}
}

func testCollectorDeploymentPlanRequest(ownershipMarker string) CollectorDeploymentPlanRequest {
	return CollectorDeploymentPlanRequest{
		TargetProvider:  "gcp",
		TargetRuntime:   "gke-standard",
		Workload:        "kubernetes",
		Distribution:    "opentelemetry-collector-contrib",
		Endpoint:        "https://otlp.example.test",
		NativeReference: "cluster/example",
		OwnershipMarker: ownershipMarker,
		Signals:         []string{"logs", "metrics", "traces"},
	}
}

func testCollectorDeploymentPlan(request CollectorDeploymentPlanRequest) CollectorDeploymentPlan {
	return CollectorDeploymentPlan{
		AdapterID:       "test.collector",
		TargetProvider:  request.TargetProvider,
		TargetRuntime:   request.TargetRuntime,
		Workload:        request.Workload,
		Distribution:    request.Distribution,
		CredentialRef:   request.CredentialRef,
		Endpoint:        request.Endpoint,
		NativeReference: request.NativeReference,
		OwnershipMarker: request.OwnershipMarker,
		Signals:         append([]string(nil), request.Signals...),
	}
}

func testCollectorDeploymentResult(request CollectorDeploymentExecutionRequest) CollectorDeploymentExecutionResult {
	result := CollectorDeploymentExecutionResult{
		Action:              request.Action,
		OperationID:         "collector:operation/1",
		ResourceRefs:        []string{"collector:resource/1"},
		ProofRefs:           []string{"collector:ownership", "collector:readiness", "collector:health", "collector:signal-delivery"},
		OwnershipMarker:     request.OwnershipMarker,
		OwnershipVerified:   true,
		IdempotencyVerified: true,
		ReadyVerified:       request.Action == CollectorApply || request.Action == CollectorVerify,
		HealthVerified:      request.Action == CollectorApply || request.Action == CollectorVerify,
		RollbackVerified:    request.Action == CollectorRollback,
		CleanupVerified:     request.Action == CollectorDestroy,
	}
	return result
}
