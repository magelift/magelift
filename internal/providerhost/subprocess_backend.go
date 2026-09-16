package providerhost

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/sdk"
)

// OpaqueSpecProvider is the narrow optional interface a planned stack
// implements to expose its provider-owned plan spec for subprocess
// execution. Structural typing keeps this package free of provider imports:
// only the proof provider implements it.
type OpaqueSpecProvider interface {
	OpaquePlanSpec() any
}

// SubprocessBackend adapts a Dialed provider session to automation.Backend
// plus automation.RequestGuard. Preview/Update/Destroy map to the matching
// Execute operation; Outputs maps to the outputs op (secrets decrypted, like
// in-process Outputs, for day-2 consumers); RedactedOutputs maps to the
// redacted-outputs op for user-facing display; ValidateRequest maps to the
// validate-request op. Ownership conflicts arrive as result payloads and are
// rebuilt into typed errors so the Runner classifies them exactly like
// in-process verdicts.
type SubprocessBackend struct {
	api        API
	plan       sdk.ModulePlan
	backendURL string
}

var (
	_ automation.Backend      = (*SubprocessBackend)(nil)
	_ automation.RequestGuard = (*SubprocessBackend)(nil)
)

// NewSubprocessBackend binds a provider API (usually a Dialed *Session,
// which implements API) to one planned stack. The plan must carry a stack
// name and the provider-owned Opaque spec; backendURL is the non-secret
// backend identity passed through to the subprocess. Session lifecycle stays
// with the caller.
func NewSubprocessBackend(api API, plan sdk.ModulePlan, backendURL string) *SubprocessBackend {
	return &SubprocessBackend{api: api, plan: plan, backendURL: backendURL}
}

// Plan returns the bound module plan, mainly for wiring tests.
func (b *SubprocessBackend) Plan() sdk.ModulePlan {
	if b == nil {
		return sdk.ModulePlan{}
	}
	return b.plan
}

func (b *SubprocessBackend) Preview(ctx context.Context, request automation.Request, diagnostics io.Writer) (map[string]int, error) {
	result, err := b.execute(ctx, ExecutePreview, request, diagnostics)
	if err != nil {
		return nil, err
	}
	return result.Changes, nil
}

func (b *SubprocessBackend) Update(ctx context.Context, request automation.Request, diagnostics io.Writer) (map[string]int, error) {
	result, err := b.execute(ctx, ExecuteUp, request, diagnostics)
	if err != nil {
		return nil, err
	}
	return result.Changes, nil
}

func (b *SubprocessBackend) Destroy(ctx context.Context, request automation.Request, diagnostics io.Writer) (map[string]int, error) {
	result, err := b.execute(ctx, ExecuteDestroy, request, diagnostics)
	if err != nil {
		return nil, err
	}
	return result.Changes, nil
}

func (b *SubprocessBackend) Outputs(ctx context.Context) (map[string]any, error) {
	return b.outputsOp(ctx, ExecuteOutputs)
}

// RedactedOutputs mirrors PulumiBackend.RedactedOutputs over the Execute RPC
// for user-facing display: secret values arrive as {"secret": true} and must
// never feed day-2 consumers. The outputsCommand display path type-asserts
// this method; without it display would fall through to decrypted Outputs.
func (b *SubprocessBackend) RedactedOutputs(ctx context.Context) (map[string]any, error) {
	return b.outputsOp(ctx, ExecuteRedactedOutputs)
}

func (b *SubprocessBackend) outputsOp(ctx context.Context, operation ExecuteOperation) (map[string]any, error) {
	if b == nil || b.api == nil {
		return nil, errors.New("subprocess backend has no provider API")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := b.api.Execute(ctx, ExecuteRequest{
		Operation:  operation,
		Plan:       b.plan,
		BackendURL: b.backendURL,
	})
	if err != nil {
		return nil, err
	}
	if result.OwnershipConflict != nil {
		return nil, result.OwnershipConflict.ToOwnershipError(automation.PreviewMetadata{})
	}
	return result.Outputs, nil
}

func (b *SubprocessBackend) ValidateRequest(ctx context.Context, request automation.Request) error {
	_, err := b.execute(ctx, ExecuteValidateRequest, request, io.Discard)
	return err
}

func (b *SubprocessBackend) execute(ctx context.Context, operation ExecuteOperation, request automation.Request, diagnostics io.Writer) (ExecuteResult, error) {
	if b == nil || b.api == nil {
		return ExecuteResult{}, errors.New("subprocess backend has no provider API")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	result, err := b.api.Execute(ctx, ExecuteRequest{
		Operation:  operation,
		Plan:       b.plan,
		BackendURL: b.backendURL,
		Preview:    request.Preview,
	})
	if err != nil {
		return ExecuteResult{}, err
	}
	writeDiagnostics(diagnostics, result.Diagnostics)
	if result.OwnershipConflict != nil {
		var requested automation.PreviewMetadata
		if request.Preview != nil {
			requested = *request.Preview
		}
		return ExecuteResult{}, result.OwnershipConflict.ToOwnershipError(requested)
	}
	if result.ConcurrentUpdate {
		return ExecuteResult{}, &automation.ConcurrentUpdateError{}
	}
	return result, nil
}

func writeDiagnostics(writer io.Writer, diagnostics []string) {
	if writer == nil {
		return
	}
	for _, line := range diagnostics {
		fmt.Fprintln(writer, line)
	}
}
