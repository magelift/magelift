package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestAzureLifecycleRoutesAndProtectsDestroy(t *testing.T) {
	request := testPlanRequest()
	server := &Server{}
	var planned sdk.PlanResult
	if err := server.Plan(request, &planned); err != nil || planned.Error != nil {
		t.Fatalf("plan: %v / %v", err, planned.Error)
	}
	called := 0
	server.mutate = func(_ context.Context, _ storedPlan, _ []byte, destroy bool) (sdk.ChangeSummary, error) {
		called++
		if destroy {
			return sdk.ChangeSummary{Delete: 2}, nil
		}
		return sdk.ChangeSummary{Create: 3}, nil
	}
	call := &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan}
	var applied sdk.LifecycleResult
	if err := server.Apply(call, &applied); err != nil || applied.Error != nil || applied.Summary.Create != 3 || called != 1 {
		t.Fatalf("apply = %#v, %v; calls %d", applied, err, called)
	}
	var destroyed sdk.LifecycleResult
	if err := server.Destroy(call, &destroyed); err != nil || destroyed.Error == nil || called != 1 {
		t.Fatalf("wrong-plan destroy = %#v, %v; calls %d", destroyed, err, called)
	}
	request.AllowExpiredPreview = true
	request.Envelope.Protected = true
	if err := server.Plan(request, &planned); err != nil || planned.Error != nil {
		t.Fatalf("protected plan: %v / %v", err, planned.Error)
	}
	call.Envelope, call.Plan = request.Envelope, planned.Plan
	destroyed = sdk.LifecycleResult{}
	if err := server.Destroy(call, &destroyed); err != nil || destroyed.Error == nil || called != 1 {
		t.Fatalf("protected destroy = %#v, %v; calls %d", destroyed, err, called)
	}
	request.Envelope.Protected = false
	if err := server.Plan(request, &planned); err != nil || planned.Error != nil {
		t.Fatalf("destroy plan: %v / %v", err, planned.Error)
	}
	call.Envelope, call.Plan = request.Envelope, planned.Plan
	destroyed = sdk.LifecycleResult{}
	if err := server.Destroy(call, &destroyed); err != nil || destroyed.Error != nil || destroyed.Summary.Delete != 2 || called != 2 {
		t.Fatalf("destroy = %#v, %v; calls %d", destroyed, err, called)
	}
}

func TestAzureOutputsKeepSecretMarkers(t *testing.T) {
	request := testPlanRequest()
	server := &Server{}
	var planned sdk.PlanResult
	if err := server.Plan(request, &planned); err != nil || planned.Error != nil {
		t.Fatalf("plan: %v / %v", err, planned.Error)
	}
	server.outputs = func(context.Context, storedPlan) (map[string]any, []string, error) {
		return map[string]any{"kubeconfig": "secret-value", "clusterName": "shop"}, []string{"kubeconfig"}, nil
	}
	var result sdk.OutputsResult
	call := &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan}
	if err := server.Outputs(call, &result); err != nil || result.Error != nil || len(result.SecretKeys) != 1 || result.SecretKeys[0] != "kubeconfig" {
		t.Fatalf("outputs = %#v, %v", result, err)
	}
	server.outputs = func(context.Context, storedPlan) (map[string]any, []string, error) {
		return nil, nil, errors.New("token=secret-password")
	}
	result = sdk.OutputsResult{}
	if err := server.Outputs(call, &result); err != nil || result.Error == nil || strings.Contains(result.Error.Message, "secret-password") {
		t.Fatalf("redacted error = %#v, %v", result, err)
	}
}
