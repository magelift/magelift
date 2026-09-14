package providerhost

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestSubprocessBackendMapsOperations(t *testing.T) {
	plan := sdk.ModulePlan{StackName: "shop-staging"}
	var got []ExecuteRequest
	fake := &fakeExecuteAPI{execute: func(request ExecuteRequest) (ExecuteResult, error) {
		got = append(got, request)
		return ExecuteResult{Operation: request.Operation, Changes: map[string]int{"create": 2}}, nil
	}}
	backend := NewSubprocessBackend(fake, plan, "file:///state")
	request := automation.Request{Target: sdk.TargetDescriptor{Provider: "gcp"}}
	var diagnostics bytes.Buffer
	changes, err := backend.Preview(context.Background(), request, &diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if changes["create"] != 2 {
		t.Fatalf("preview changes = %v", changes)
	}
	if _, err := backend.Update(context.Background(), request, &diagnostics); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Destroy(context.Background(), request, &diagnostics); err != nil {
		t.Fatal(err)
	}
	if err := backend.ValidateRequest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	wantOps := []ExecuteOperation{ExecutePreview, ExecuteUp, ExecuteDestroy, ExecuteValidateRequest}
	if len(got) != len(wantOps) {
		t.Fatalf("operations = %v", got)
	}
	for i, want := range wantOps {
		if got[i].Operation != want || got[i].Plan.StackName != "shop-staging" || got[i].BackendURL != "file:///state" {
			t.Fatalf("request %d = %+v", i, got[i])
		}
	}
	if fake.calls != len(wantOps) {
		t.Fatalf("calls = %d", fake.calls)
	}
}

func TestSubprocessBackendWritesDiagnostics(t *testing.T) {
	fake := &fakeExecuteAPI{execute: func(request ExecuteRequest) (ExecuteResult, error) {
		return ExecuteResult{Operation: request.Operation, Diagnostics: []string{"line one", "line two"}}, nil
	}}
	backend := NewSubprocessBackend(fake, sdk.ModulePlan{StackName: "shop-staging"}, "")
	var diagnostics bytes.Buffer
	if _, err := backend.Preview(context.Background(), automation.Request{}, &diagnostics); err != nil {
		t.Fatal(err)
	}
	if diagnostics.String() != "line one\nline two\n" {
		t.Fatalf("diagnostics = %q", diagnostics.String())
	}
	// Nil diagnostics must not fail the operation.
	if _, err := backend.Preview(context.Background(), automation.Request{}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestSubprocessBackendRebuildsOwnershipError(t *testing.T) {
	conflict := &OwnershipConflict{Operation: "preview", Reason: "stale generation", CurrentGeneration: 4, HasCurrent: true}
	fake := &fakeExecuteAPI{execute: func(request ExecuteRequest) (ExecuteResult, error) {
		return ExecuteResult{Operation: request.Operation, OwnershipConflict: conflict}, nil
	}}
	backend := NewSubprocessBackend(fake, sdk.ModulePlan{StackName: "shop-preview"}, "")
	preview := &automation.PreviewMetadata{Generation: 2}
	err := backend.ValidateRequest(context.Background(), automation.Request{Preview: preview})
	var ownershipErr *automation.PreviewOwnershipError
	if !errors.As(err, &ownershipErr) {
		t.Fatalf("err = %T %v, want ownership error", err, err)
	}
	if ownershipErr.Operation != "preview" || ownershipErr.Reason != "stale generation" {
		t.Fatalf("ownership = %+v", ownershipErr)
	}
	if ownershipErr.Current == nil || ownershipErr.Current.Generation != 4 {
		t.Fatalf("current = %+v", ownershipErr.Current)
	}
	if ownershipErr.Requested.Generation != 2 {
		t.Fatalf("requested = %+v", ownershipErr.Requested)
	}
	if _, err := backend.Preview(context.Background(), automation.Request{Preview: preview}, &bytes.Buffer{}); !errors.Is(err, automation.ErrPreviewOwnershipConflict) {
		t.Fatalf("preview err = %v, want conflict", err)
	}
}

func TestSubprocessBackendOutputs(t *testing.T) {
	fake := &fakeExecuteAPI{execute: func(request ExecuteRequest) (ExecuteResult, error) {
		if request.Operation != ExecuteOutputs {
			t.Fatalf("operation = %q", request.Operation)
		}
		if request.Preview != nil {
			t.Fatal("outputs must not carry preview metadata")
		}
		return ExecuteResult{Operation: request.Operation, Outputs: map[string]any{"applicationURL": "https://shop.example"}}, nil
	}}
	backend := NewSubprocessBackend(fake, sdk.ModulePlan{StackName: "shop-staging"}, "")
	outputs, err := backend.Outputs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if outputs["applicationURL"] != "https://shop.example" {
		t.Fatalf("outputs = %v", outputs)
	}
}

func TestSubprocessBackendRedactedOutputs(t *testing.T) {
	fake := &fakeExecuteAPI{execute: func(request ExecuteRequest) (ExecuteResult, error) {
		if request.Operation != ExecuteRedactedOutputs {
			t.Fatalf("operation = %q", request.Operation)
		}
		if request.Preview != nil {
			t.Fatal("redacted outputs must not carry preview metadata")
		}
		return ExecuteResult{Operation: request.Operation, Outputs: map[string]any{"kubeconfig": map[string]any{"secret": true}}}, nil
	}}
	backend := NewSubprocessBackend(fake, sdk.ModulePlan{StackName: "shop-staging"}, "")
	outputs, err := backend.RedactedOutputs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	redacted, ok := outputs["kubeconfig"].(map[string]any)
	if !ok || redacted["secret"] != true {
		t.Fatalf("outputs = %v", outputs)
	}
}

func TestSubprocessBackendPropagatesErrors(t *testing.T) {
	boom := errors.New("subprocess boom")
	fake := &fakeExecuteAPI{execute: func(ExecuteRequest) (ExecuteResult, error) {
		return ExecuteResult{}, boom
	}}
	backend := NewSubprocessBackend(fake, sdk.ModulePlan{StackName: "shop-staging"}, "")
	if _, err := backend.Preview(context.Background(), automation.Request{}, &bytes.Buffer{}); !errors.Is(err, boom) {
		t.Fatalf("preview err = %v", err)
	}
	if _, err := backend.Outputs(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("outputs err = %v", err)
	}
	if err := backend.ValidateRequest(context.Background(), automation.Request{}); !errors.Is(err, boom) {
		t.Fatalf("validate err = %v", err)
	}
	var nilBackend *SubprocessBackend
	if _, err := nilBackend.Preview(context.Background(), automation.Request{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "no provider API") {
		t.Fatalf("nil backend err = %v", err)
	}
	if _, err := NewSubprocessBackend(nil, sdk.ModulePlan{}, "").Outputs(context.Background()); err == nil {
		t.Fatal("nil API outputs should fail")
	}
}
