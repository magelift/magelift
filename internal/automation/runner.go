package automation

import (
	"context"
	"errors"
	"io"
	"sort"

	sdk "github.com/acourtiol/magelift/sdk/v1"
)

var (
	ErrBackendRequired     = errors.New("infrastructure automation backend is required")
	ErrDiagnosticsRequired = errors.New("infrastructure diagnostics writer is required")
	ErrPreviewFailed       = errors.New("infrastructure preview failed")
	ErrUpdateFailed        = errors.New("infrastructure update failed")
	ErrDestroyFailed       = errors.New("infrastructure destroy failed")
)

type Request struct {
	Target sdk.TargetDescriptor
}

type Change struct {
	Operation string `json:"operation" yaml:"operation"`
	Count     int    `json:"count" yaml:"count"`
}

type ChangeSummary struct {
	Changes []Change `json:"changes" yaml:"changes"`
	Total   int      `json:"total" yaml:"total"`
}

type Backend interface {
	Preview(context.Context, Request, io.Writer) (map[string]int, error)
	Update(context.Context, Request, io.Writer) (map[string]int, error)
	Destroy(context.Context, Request, io.Writer) (map[string]int, error)
}

type Runner struct {
	backend     Backend
	diagnostics io.Writer
}

func NewRunner(backend Backend, diagnostics io.Writer) *Runner {
	return &Runner{backend: backend, diagnostics: diagnostics}
}

func (r *Runner) Preview(ctx context.Context, request Request) (ChangeSummary, error) {
	return r.run(ctx, request, ErrPreviewFailed, r.backendOperation(operationPreview))
}

func (r *Runner) Update(ctx context.Context, request Request) (ChangeSummary, error) {
	return r.run(ctx, request, ErrUpdateFailed, r.backendOperation(operationUpdate))
}

func (r *Runner) Destroy(ctx context.Context, request Request) (ChangeSummary, error) {
	return r.run(ctx, request, ErrDestroyFailed, r.backendOperation(operationDestroy))
}

type operation uint8

const (
	operationPreview operation = iota
	operationUpdate
	operationDestroy
)

type backendOperation func(context.Context, Request, io.Writer) (map[string]int, error)

func (r *Runner) backendOperation(operation operation) backendOperation {
	if r == nil || r.backend == nil {
		return nil
	}
	switch operation {
	case operationPreview:
		return r.backend.Preview
	case operationUpdate:
		return r.backend.Update
	case operationDestroy:
		return r.backend.Destroy
	default:
		return nil
	}
}

func (r *Runner) run(ctx context.Context, request Request, operationError error, operation backendOperation) (ChangeSummary, error) {
	if operation == nil {
		return ChangeSummary{}, ErrBackendRequired
	}
	if r.diagnostics == nil {
		return ChangeSummary{}, ErrDiagnosticsRequired
	}
	if err := sdk.ValidateTargetDescriptor(request.Target); err != nil {
		return ChangeSummary{}, err
	}
	if cause := context.Cause(ctx); cause != nil {
		return ChangeSummary{}, cause
	}
	changes, err := operation(ctx, request, r.diagnostics)
	if cause := context.Cause(ctx); cause != nil {
		return ChangeSummary{}, cause
	}
	if err != nil {
		return ChangeSummary{}, operationError
	}
	return summarize(changes), nil
}

func summarize(changes map[string]int) ChangeSummary {
	operations := make([]string, 0, len(changes))
	for operation, count := range changes {
		if count > 0 {
			operations = append(operations, operation)
		}
	}
	sort.Strings(operations)

	summary := ChangeSummary{Changes: make([]Change, 0, len(operations))}
	for _, operation := range operations {
		count := changes[operation]
		summary.Changes = append(summary.Changes, Change{Operation: operation, Count: count})
		summary.Total += count
	}
	return summary
}
