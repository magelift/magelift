package automation

import (
	"context"
	"errors"
	"io"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

var ErrPulumiStackRequired = errors.New("Pulumi stack is required")

type PulumiBackend struct {
	stack *auto.Stack
}

func NewPulumiBackend(stack *auto.Stack) *PulumiBackend {
	return &PulumiBackend{stack: stack}
}

func (b *PulumiBackend) Preview(ctx context.Context, _ Request, diagnostics io.Writer) (map[string]int, error) {
	if b == nil || b.stack == nil {
		return nil, ErrPulumiStackRequired
	}
	result, err := b.stack.Preview(ctx,
		optpreview.ProgressStreams(diagnostics),
		optpreview.ErrorProgressStreams(diagnostics),
	)
	if err != nil {
		return nil, err
	}
	changes := make(map[string]int, len(result.ChangeSummary))
	for operation, count := range result.ChangeSummary {
		changes[string(operation)] = count
	}
	return changes, nil
}

func (b *PulumiBackend) Update(ctx context.Context, _ Request, diagnostics io.Writer) (map[string]int, error) {
	if b == nil || b.stack == nil {
		return nil, ErrPulumiStackRequired
	}
	result, err := b.stack.Up(ctx,
		optup.ProgressStreams(diagnostics),
		optup.ErrorProgressStreams(diagnostics),
	)
	if err != nil {
		return nil, err
	}
	return resourceChanges(result.Summary.ResourceChanges), nil
}

func (b *PulumiBackend) Destroy(ctx context.Context, _ Request, diagnostics io.Writer) (map[string]int, error) {
	if b == nil || b.stack == nil {
		return nil, ErrPulumiStackRequired
	}
	result, err := b.stack.Destroy(ctx,
		optdestroy.ProgressStreams(diagnostics),
		optdestroy.ErrorProgressStreams(diagnostics),
	)
	if err != nil {
		return nil, err
	}
	return resourceChanges(result.Summary.ResourceChanges), nil
}

// Outputs returns the last successful stack outputs without exposing the
// Pulumi Automation API to CLI callers.
func (b *PulumiBackend) Outputs(ctx context.Context) (map[string]any, error) {
	if b == nil || b.stack == nil {
		return nil, ErrPulumiStackRequired
	}
	outputs, err := b.stack.Outputs(ctx)
	if err != nil {
		return nil, err
	}
	result := make(map[string]any, len(outputs))
	for name, output := range outputs {
		if output.Secret {
			result[name] = map[string]any{"secret": true}
			continue
		}
		result[name] = output.Value
	}
	return result, nil
}

func resourceChanges(changes *map[string]int) map[string]int {
	if changes == nil {
		return nil
	}
	result := make(map[string]int, len(*changes))
	for operation, count := range *changes {
		result[operation] = count
	}
	return result
}
