package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestPreviewChecksIdentityBeforeInvokingPulumi(t *testing.T) {
	request := testPlanRequest()
	var planned sdk.PlanResult
	server := &Server{}
	if err := server.Plan(request, &planned); err != nil || planned.Error != nil {
		t.Fatalf("plan: %v, %v", err, planned.Error)
	}
	called := false
	server.preview = func(_ context.Context, plan storedPlan, metadata []byte) (sdk.ChangeSummary, error) {
		called = true
		if plan.Spec.ApplicationImage != planned.Plan.ImageDigest || len(metadata) != 0 {
			t.Fatal("preview inputs differ from plan")
		}
		return sdk.ChangeSummary{Create: 5}, nil
	}
	call := &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan}
	var result sdk.LifecycleResult
	if err := server.Preview(call, &result); err != nil || result.Error != nil || result.Summary.Create != 5 || !called {
		t.Fatalf("preview = %#v, err = %v, called = %v", result, err, called)
	}
	called = false
	call.Envelope.Environment = "production"
	result = sdk.LifecycleResult{}
	if err := server.Preview(call, &result); err != nil || result.Error == nil || called {
		t.Fatalf("mismatched preview = %#v, err = %v, called = %v", result, err, called)
	}
}

func TestPreviewRedactsSensitiveError(t *testing.T) {
	request := testPlanRequest()
	var planned sdk.PlanResult
	server := &Server{}
	if err := server.Plan(request, &planned); err != nil || planned.Error != nil {
		t.Fatalf("plan: %v, %v", err, planned.Error)
	}
	server.preview = func(context.Context, storedPlan, []byte) (sdk.ChangeSummary, error) {
		return sdk.ChangeSummary{}, errors.New("password=super-secret-password")
	}
	call := &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: request.Envelope, Plan: planned.Plan}
	var result sdk.LifecycleResult
	if err := server.Preview(call, &result); err != nil || result.Error == nil {
		t.Fatalf("preview = %#v, err = %v", result, err)
	}
	if strings.Contains(result.Error.Message, "super-secret-password") {
		t.Fatal("preview exposed password")
	}
}
