package observability

import (
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestCollectorSDKAdapterIsIdempotentReadyGatedAndCleansOwnedResources(t *testing.T) {
	backend := &fakeCollectorBackend{}
	adapter, err := NewCollectorSDKAdapter("aws.ecs.collector", "aws", "ecs", func(runtime sdk.RuntimeID) bool { return strings.HasPrefix(string(runtime), "ecs-") }, backend)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.CollectorDeploymentPlanRequest{
		TargetProvider: "aws", TargetRuntime: "ecs-fargate", Workload: "ecs", Distribution: "opentelemetry-collector-contrib",
		CredentialRef: "aws-secrets-manager://magelift/newrelic", Endpoint: "https://otlp.eu01.nr-data.net",
		NativeReference: "arn:aws:ecs:eu-west-3:123456789012:service/magelift", OwnershipMarker: "magelift/test/collector",
		Signals: []string{"logs", "metrics", "traces"},
	}
	plan, err := adapter.PlanCollector(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	execution := sdk.CollectorDeploymentExecutionRequest{Plan: plan, Action: sdk.CollectorApply, IdempotencyKey: "collector/apply/1", OwnershipMarker: request.OwnershipMarker}
	result, err := adapter.ExecuteCollector(context.Background(), execution)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified || !result.ReadyVerified || !result.HealthVerified || backend.ensureCalls != 1 {
		t.Fatalf("apply result = %#v backend = %#v", result, backend)
	}

	repeated, err := adapter.ExecuteCollector(context.Background(), execution)
	if err != nil {
		t.Fatalf("repeat apply: %v", err)
	}
	if repeated.OperationID != result.OperationID || backend.ensureCalls != 2 {
		t.Fatalf("repeat apply = %#v backend = %#v", repeated, backend)
	}

	verify := execution
	verify.Action = sdk.CollectorVerify
	verify.IdempotencyKey = "collector/verify/1"
	verified, err := adapter.ExecuteCollector(context.Background(), verify)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !verified.ReadyVerified || !verified.HealthVerified {
		t.Fatalf("verify result = %#v", verified)
	}

	destroy := execution
	destroy.Action = sdk.CollectorDestroy
	destroy.IdempotencyKey = "collector/destroy/1"
	destroyed, err := adapter.ExecuteCollector(context.Background(), destroy)
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if !destroyed.CleanupVerified || !destroyed.OwnershipVerified || backend.destroyCalls != 1 {
		t.Fatalf("destroy result = %#v backend = %#v", destroyed, backend)
	}
}

func TestCollectorSDKAdapterRollsBackWhenReadinessIsNotProven(t *testing.T) {
	backend := &fakeCollectorBackend{verification: CollectorVerification{Reason: "collector has no ready task"}}
	adapter, err := NewCollectorSDKAdapter("gcp.kubernetes.collector", "gcp", "kubernetes", func(runtime sdk.RuntimeID) bool { return strings.HasPrefix(string(runtime), "gke-") }, backend)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.PlanCollector(context.Background(), sdk.CollectorDeploymentPlanRequest{
		TargetProvider: "gcp", TargetRuntime: "gke-standard", Workload: "kubernetes", Distribution: "newrelic/nr-k8s-otel-collector",
		CredentialRef: "gcp-secret-manager://projects/demo/secrets/newrelic", Endpoint: "https://otlp.nr-data.net",
		NativeReference: "projects/demo/locations/europe-west1/clusters/magelift", OwnershipMarker: "magelift/test/gke-collector", Signals: []string{"logs"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.ExecuteCollector(context.Background(), sdk.CollectorDeploymentExecutionRequest{Plan: plan, Action: sdk.CollectorApply, IdempotencyKey: "collector/apply/failed", OwnershipMarker: plan.OwnershipMarker})
	if err == nil || !strings.Contains(err.Error(), "did not prove readiness") || backend.rollbackCalls != 1 {
		t.Fatalf("error = %v backend = %#v, want readiness failure and rollback", err, backend)
	}
}

func TestCollectorSDKAdapterRefusesUnownedResourcesBeforeMutation(t *testing.T) {
	backend := &fakeCollectorBackend{resource: &CollectorResource{Identity: "ecs:collector:user", TargetProvider: "aws", TargetRuntime: "ecs-fargate", Workload: "ecs", Distribution: "opentelemetry-collector-contrib", OwnershipMarker: "other", Owned: false}}
	adapter, err := NewCollectorSDKAdapter("aws.ecs.collector", "aws", "ecs", func(runtime sdk.RuntimeID) bool { return strings.HasPrefix(string(runtime), "ecs-") }, backend)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.PlanCollector(context.Background(), sdk.CollectorDeploymentPlanRequest{
		TargetProvider: "aws", TargetRuntime: "ecs-fargate", Workload: "ecs", Distribution: "opentelemetry-collector-contrib",
		CredentialRef: "aws-secrets-manager://magelift/newrelic", Endpoint: "https://otlp.nr-data.net", NativeReference: "service/ref",
		OwnershipMarker: "magelift/test/owned", Signals: []string{"logs"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.ExecuteCollector(context.Background(), sdk.CollectorDeploymentExecutionRequest{Plan: plan, Action: sdk.CollectorApply, IdempotencyKey: "collector/apply/owned", OwnershipMarker: plan.OwnershipMarker})
	if err == nil || !strings.Contains(err.Error(), "unowned") || backend.ensureCalls != 0 {
		t.Fatalf("error = %v backend = %#v, want no mutation", err, backend)
	}
}

func TestCollectorSDKAdapterRejectsTamperedUnsupportedDistributionBeforeMutation(t *testing.T) {
	backend := &fakeCollectorBackend{}
	adapter, err := NewCollectorSDKAdapterWithDistribution(
		"aws.ecs.collector", "aws", "ecs",
		func(runtime sdk.RuntimeID) bool { return strings.HasPrefix(string(runtime), "ecs-") },
		func(distribution string) bool { return distribution == "opentelemetry-collector-contrib" }, backend,
	)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.PlanCollector(context.Background(), sdk.CollectorDeploymentPlanRequest{
		TargetProvider: "aws", TargetRuntime: "ecs-fargate", Workload: "ecs", Distribution: "opentelemetry-collector-contrib",
		CredentialRef: "aws-secrets-manager://magelift/newrelic", Endpoint: "https://otlp.nr-data.net", NativeReference: "service/ref",
		OwnershipMarker: "magelift/test/distribution", Signals: []string{"logs"},
	})
	if err != nil {
		t.Fatal(err)
	}

	tampered := plan
	tampered.Distribution = "foreign-collector"
	_, err = adapter.ExecuteCollector(context.Background(), sdk.CollectorDeploymentExecutionRequest{
		Plan: tampered, Action: sdk.CollectorApply, IdempotencyKey: "collector/apply/tampered-distribution", OwnershipMarker: plan.OwnershipMarker,
	})
	if err == nil || !strings.Contains(err.Error(), "provider boundary") || backend.ensureCalls != 0 {
		t.Fatalf("error = %v backend = %#v, want capability rejection before mutation", err, backend)
	}
}

type fakeCollectorBackend struct {
	resource      *CollectorResource
	verification  CollectorVerification
	ensureCalls   int
	rollbackCalls int
	destroyCalls  int
}

func (backend *fakeCollectorBackend) Find(_ context.Context, plan sdk.CollectorDeploymentPlan) (CollectorResource, bool, error) {
	if backend.resource == nil {
		return CollectorResource{}, false, nil
	}
	return *backend.resource, true, nil
}

func (backend *fakeCollectorBackend) Ensure(_ context.Context, plan sdk.CollectorDeploymentPlan) (CollectorResource, error) {
	backend.ensureCalls++
	if backend.resource == nil {
		backend.resource = &CollectorResource{Identity: "collector:" + string(plan.TargetRuntime) + ":" + plan.OwnershipMarker, TargetProvider: plan.TargetProvider, TargetRuntime: plan.TargetRuntime, Workload: plan.Workload, Distribution: plan.Distribution, OwnershipMarker: plan.OwnershipMarker, Status: "ready", Owned: true}
	}
	return *backend.resource, nil
}

func (backend *fakeCollectorBackend) Verify(_ context.Context, _ sdk.CollectorDeploymentPlan, _ CollectorResource) (CollectorVerification, error) {
	if backend.verification.Reason != "" {
		return backend.verification, nil
	}
	return CollectorVerification{Ready: true, Healthy: true, SignalsDelivered: true}, nil
}

func (backend *fakeCollectorBackend) Rollback(_ context.Context, _ sdk.CollectorDeploymentPlan, _ CollectorResource) error {
	backend.rollbackCalls++
	backend.resource = nil
	return nil
}

func (backend *fakeCollectorBackend) Destroy(_ context.Context, _ sdk.CollectorDeploymentPlan, _ CollectorResource) error {
	backend.destroyCalls++
	backend.resource = nil
	return nil
}

func (backend *fakeCollectorBackend) Inventory(_ context.Context, _ sdk.ProviderID, _ sdk.RuntimeID, _ string) ([]CollectorResource, error) {
	if backend.resource == nil {
		return nil, nil
	}
	return []CollectorResource{*backend.resource}, nil
}

var _ CollectorBackend = (*fakeCollectorBackend)(nil)
