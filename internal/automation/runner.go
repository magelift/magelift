package automation

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/magelift/magelift/internal/secretsafe"
	"github.com/magelift/magelift/sdk"
)

var (
	ErrBackendRequired      = errors.New("infrastructure automation backend is required")
	ErrDiagnosticsRequired  = errors.New("infrastructure diagnostics writer is required")
	ErrPreviewFailed        = errors.New("infrastructure preview failed")
	ErrUpdateFailed         = errors.New("infrastructure update failed")
	ErrDestroyFailed        = errors.New("infrastructure destroy failed")
	ErrPreviewGuardRequired = errors.New("preview ownership guard is required")
)

type Request struct {
	Target  sdk.TargetDescriptor
	Preview *PreviewMetadata
	Destroy bool
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

type RequestGuard interface {
	ValidateRequest(context.Context, Request) error
}

func ValidateRequest(ctx context.Context, backend Backend, request Request) error {
	if err := sdk.ValidateTargetDescriptor(request.Target); err != nil {
		return err
	}
	if request.Preview == nil {
		return nil
	}
	if err := request.Preview.Validate(); err != nil {
		return err
	}
	guard, ok := backend.(RequestGuard)
	if !ok {
		return ErrPreviewGuardRequired
	}
	return guard.ValidateRequest(ctx, request)
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
	request.Destroy = true
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
	if err := ValidateRequest(ctx, r.backend, request); err != nil {
		var ownershipErr *PreviewOwnershipError
		if errors.As(err, &ownershipErr) {
			return ChangeSummary{}, fmt.Errorf("%w: %w", operationError, ownershipErr)
		}
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
		var ownershipErr *PreviewOwnershipError
		if errors.As(err, &ownershipErr) {
			return ChangeSummary{}, fmt.Errorf("%w: %w", operationError, ownershipErr)
		}
		// A typed collision passes through once; lock text from a backend
		// that cannot carry Go types gets typed here. Every other cause is
		// wrapped, never dropped. Classification runs on the raw cause;
		// only redacted text enters the returned chain, so credential
		// shapes from provider output can never print.
		var concurrentErr *ConcurrentUpdateError
		if errors.As(err, &concurrentErr) {
			return ChangeSummary{}, fmt.Errorf("%w: %w", operationError, concurrentErr)
		}
		if IsConcurrentUpdate(err) {
			return ChangeSummary{}, fmt.Errorf("%w: %w", operationError, &ConcurrentUpdateError{Cause: redactedCause(err)})
		}
		return ChangeSummary{}, fmt.Errorf("%w: %w", operationError, redactedCause(err))
	}
	return summarize(changes), nil
}

// redactedCause keeps the backend cause readable while replacing
// credential-shaped material with the shared redactor. The raw cause
// never enters the returned chain.
func redactedCause(err error) error {
	redacted, _ := secretsafe.RedactSensitiveText(err.Error())
	return errors.New(redacted)
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
