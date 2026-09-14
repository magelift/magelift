package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/automation"
	gcpstack "github.com/magelift/magelift/internal/cloud/gcp/stack"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// mockBackendScheme selects the mock executor. Test Dial sessions send a
// test:// backend URL so no cloud credentials or Pulumi backend are needed.
// Real backend URLs (including empty, which keeps Pulumi's normal selection)
// always take the Automation API path.
const mockBackendScheme = "test://"

// execute runs one stack lifecycle operation inside the provider process. A
// Pulumi RunFunc cannot cross gRPC, so execution lives here and only
// structured results return to the host. Ownership conflicts return as
// result payloads (Go error types do not survive gRPC); anything else
// returns an RPC error.
func execute(ctx context.Context, module gcpstack.Module, request providerhost.ExecuteRequest) (providerhost.ExecuteResult, error) {
	if err := request.Validate(); err != nil {
		return providerhost.ExecuteResult{}, err
	}
	program, err := programFromPlan(module, request)
	if err != nil {
		return providerhost.ExecuteResult{}, err
	}
	if strings.HasPrefix(strings.TrimSpace(request.BackendURL), mockBackendScheme) {
		return mockExecute(request), nil
	}
	stack, err := automation.NewInlineStackWithBackend(ctx, request.Plan.StackName, program, request.BackendURL)
	if err != nil {
		return providerhost.ExecuteResult{}, fmt.Errorf("create automation stack: %w", err)
	}
	backend := automation.NewPulumiBackend(stack)
	autoRequest := automation.Request{
		Target:  request.Plan.Target,
		Preview: request.Preview,
		Destroy: request.Operation == providerhost.ExecuteDestroy,
	}
	var diagnostics bytes.Buffer
	result := providerhost.ExecuteResult{Operation: request.Operation}
	switch request.Operation {
	case providerhost.ExecutePreview:
		result.Changes, err = backend.Preview(ctx, autoRequest, &diagnostics)
	case providerhost.ExecuteUp:
		result.Changes, err = backend.Update(ctx, autoRequest, &diagnostics)
	case providerhost.ExecuteDestroy:
		result.Changes, err = backend.Destroy(ctx, autoRequest, &diagnostics)
	case providerhost.ExecuteOutputs, providerhost.ExecuteRedactedOutputs:
		result.Outputs, err = executeOutputs(ctx, backend, request.Operation)
	case providerhost.ExecuteValidateRequest:
		err = backend.ValidateRequest(ctx, autoRequest)
	default:
		err = fmt.Errorf("execute operation %q is not supported", request.Operation)
	}
	if err != nil {
		return classifyExecuteError(result, &diagnostics, err)
	}
	result.Diagnostics, result.Truncated = collectDiagnostics(&diagnostics)
	return result, nil
}

// outputsSource is the narrow backend surface the outputs ops need.
type outputsSource interface {
	Outputs(context.Context) (map[string]any, error)
	RedactedOutputs(context.Context) (map[string]any, error)
}

// executeOutputs routes the two outputs ops: decrypted for in-process day-2
// consumers (exec, health, logs, deploy Steps need the real kubeconfig,
// matching in-process Outputs), redacted for user-facing display.
func executeOutputs(ctx context.Context, backend outputsSource, operation providerhost.ExecuteOperation) (map[string]any, error) {
	if operation == providerhost.ExecuteRedactedOutputs {
		return backend.RedactedOutputs(ctx)
	}
	return backend.Outputs(ctx)
}

// classifyExecuteError maps backend failures onto the wire: ownership
// conflicts and stack collisions travel as flags with diagnostics so the
// host can rebuild typed errors; everything else returns raw.
func classifyExecuteError(result providerhost.ExecuteResult, diagnostics *bytes.Buffer, err error) (providerhost.ExecuteResult, error) {
	if conflict := providerhost.OwnershipConflictFromError(err); conflict != nil {
		result.OwnershipConflict = conflict
		result.Diagnostics, result.Truncated = collectDiagnostics(diagnostics)
		return result, nil
	}
	if automation.IsConcurrentUpdate(err) {
		result.ConcurrentUpdate = true
		result.Diagnostics, result.Truncated = collectDiagnostics(diagnostics)
		return result, nil
	}
	return providerhost.ExecuteResult{}, err
}

func programFromPlan(module gcpstack.Module, request providerhost.ExecuteRequest) (pulumi.RunFunc, error) {
	payload, err := json.Marshal(request.Plan.Opaque)
	if err != nil {
		return nil, fmt.Errorf("encode GCP plan opaque: %w", err)
	}
	var spec gcpstack.Spec
	if err := json.Unmarshal(payload, &spec); err != nil {
		return nil, fmt.Errorf("decode GCP plan opaque: %w", err)
	}
	program, err := module.Program(gcpstack.Planned{Spec: spec})
	if err != nil {
		return nil, err
	}
	if program == nil {
		return nil, fmt.Errorf("GCP module returned a nil program")
	}
	return program, nil
}

func collectDiagnostics(buffer *bytes.Buffer) ([]string, bool) {
	text := strings.TrimSpace(buffer.String())
	if text == "" {
		return nil, false
	}
	return providerhost.TruncateDiagnostics(strings.Split(text, "\n"))
}
