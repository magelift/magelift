package automation

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optpreview"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optup"
)

var ErrPulumiStackRequired = errors.New("Pulumi stack is required")

type PulumiBackend struct {
	stack pulumiStack
}

func NewPulumiBackend(stack *auto.Stack) *PulumiBackend {
	if stack == nil {
		return &PulumiBackend{}
	}
	return &PulumiBackend{stack: stack}
}

type pulumiStack interface {
	Preview(context.Context, ...optpreview.Option) (auto.PreviewResult, error)
	Up(context.Context, ...optup.Option) (auto.UpResult, error)
	Destroy(context.Context, ...optdestroy.Option) (auto.DestroyResult, error)
	Outputs(context.Context) (auto.OutputMap, error)
	ListTags(context.Context) (map[string]string, error)
	SetTag(context.Context, string, string) error
	Info(context.Context) (auto.StackSummary, error)
}

func newPulumiBackend(stack pulumiStack) *PulumiBackend {
	return &PulumiBackend{stack: stack}
}

func (b *PulumiBackend) ValidateRequest(ctx context.Context, request Request) error {
	if err := b.requireStack(); err != nil {
		return err
	}
	return b.preparePreview(ctx, request, request.Destroy)
}

func (b *PulumiBackend) Preview(ctx context.Context, request Request, diagnostics io.Writer) (map[string]int, error) {
	if err := b.requireStack(); err != nil {
		return nil, err
	}
	if err := b.preparePreview(ctx, request, request.Destroy); err != nil {
		return nil, err
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

func (b *PulumiBackend) Update(ctx context.Context, request Request, diagnostics io.Writer) (map[string]int, error) {
	if err := b.requireStack(); err != nil {
		return nil, err
	}
	if err := b.preparePreview(ctx, request, false); err != nil {
		return nil, err
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

func (b *PulumiBackend) Destroy(ctx context.Context, request Request, diagnostics io.Writer) (map[string]int, error) {
	if err := b.requireStack(); err != nil {
		return nil, err
	}
	if err := b.preparePreview(ctx, request, true); err != nil {
		return nil, err
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

func (b *PulumiBackend) requireStack() error {
	if b == nil || b.stack == nil {
		return ErrPulumiStackRequired
	}
	return nil
}

func (b *PulumiBackend) preparePreview(ctx context.Context, request Request, destroying bool) error {
	if request.Preview == nil {
		return nil
	}
	metadata := *request.Preview
	if err := metadata.Validate(); err != nil {
		return err
	}
	current, err := b.ReadPreviewOwnership(ctx)
	if err != nil {
		return err
	}
	if current == nil {
		if destroying {
			return ownershipError("destroy", ErrPreviewOwnershipMissing, "the stack has no persisted owner record", metadata, nil)
		}
		hasResources, err := b.StackHasResources(ctx)
		if err != nil {
			return fmt.Errorf("check preview stack ownership: %w", err)
		}
		if hasResources {
			return ownershipError("apply", ErrPreviewUnownedStack, "refusing to adopt an existing populated stack", metadata, nil)
		}
		return b.WritePreviewOwnership(ctx, metadata)
	}

	if !samePreviewOwner(metadata, *current) {
		return ownershipError(operationName(destroying), ErrPreviewOwnershipConflict, "the persisted owner does not match the requested repository and pull request", metadata, current)
	}
	if destroying {
		if metadata.Generation < current.Generation {
			return ownershipError("destroy", ErrPreviewOwnershipConflict, "the requested close generation is not current", metadata, current)
		}
		if metadata.Generation == current.Generation && !samePreviewRecord(metadata, *current) {
			return ownershipError("destroy", ErrPreviewOwnershipConflict, "the requested close metadata is not current", metadata, current)
		}
		return nil
	}
	if metadata.Generation < current.Generation {
		return ownershipError("apply", ErrPreviewOwnershipConflict, "the requested generation is older than the persisted record", metadata, current)
	}
	if metadata.Generation == current.Generation && !samePreviewRecord(metadata, *current) {
		return ownershipError("apply", ErrPreviewOwnershipConflict, "metadata changed without advancing the preview generation", metadata, current)
	}
	if metadata.Generation > current.Generation {
		return b.WritePreviewOwnership(ctx, metadata)
	}
	return nil
}

func (b *PulumiBackend) ReadPreviewOwnership(ctx context.Context) (*PreviewMetadata, error) {
	if err := b.requireStack(); err != nil {
		return nil, err
	}
	tags, err := b.stack.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("read preview ownership metadata: %w", err)
	}
	value, ok := tags[previewOwnershipTag]
	if !ok || value == "" {
		return nil, nil
	}
	metadata, err := decodePreviewMetadata(value)
	if err != nil {
		return nil, err
	}
	return &metadata, nil
}

func (b *PulumiBackend) WritePreviewOwnership(ctx context.Context, metadata PreviewMetadata) error {
	if err := b.requireStack(); err != nil {
		return err
	}
	payload, err := encodePreviewMetadata(metadata)
	if err != nil {
		return err
	}
	if err := b.stack.SetTag(ctx, previewOwnershipTag, payload); err != nil {
		return fmt.Errorf("persist preview ownership metadata: %w", err)
	}
	return nil
}

func (b *PulumiBackend) StackHasResources(ctx context.Context) (bool, error) {
	if err := b.requireStack(); err != nil {
		return false, err
	}
	info, err := b.stack.Info(ctx)
	if err != nil {
		return false, err
	}
	return info.ResourceCount != nil && *info.ResourceCount > 0, nil
}

func operationName(destroying bool) string {
	if destroying {
		return "destroy"
	}
	return "apply"
}

func samePreviewOwner(left, right PreviewMetadata) bool {
	return left.Project == right.Project &&
		left.Repository == right.Repository &&
		left.PullRequest == right.PullRequest &&
		left.Environment == right.Environment &&
		left.StackKey == right.StackKey &&
		left.Owner == right.Owner
}

func samePreviewRecord(left, right PreviewMetadata) bool {
	return samePreviewOwner(left, right) &&
		left.Branch == right.Branch &&
		left.CommitDigest == right.CommitDigest &&
		left.Domain == right.Domain &&
		left.ExpiresAt == right.ExpiresAt &&
		left.Generation == right.Generation
}

// Outputs returns the last successful stack outputs without exposing the
// Pulumi Automation API to CLI callers.
//
// Secret outputs are returned decrypted (Automation API already unwraps them
// with the stack passphrase). Day-2 kube Steps and ClientFromOutputs need the
// real kubeconfig string; redacting to {"secret": true} made deploy fail after
// a successful Up with "Pulumi output \"kubeconfig\" must be a non-empty string".
// CLI `magelift outputs` must call RedactedOutputs for user-facing JSON.
func (b *PulumiBackend) Outputs(ctx context.Context) (map[string]any, error) {
	if err := b.requireStack(); err != nil {
		return nil, err
	}
	outputs, err := b.stack.Outputs(ctx)
	if err != nil {
		return nil, err
	}
	return mapStackOutputs(outputs), nil
}

// RedactedOutputs is Outputs with secret values replaced by {"secret": true}
// for CLI display / evidence logs (never print kubeconfig or DB passwords).
func (b *PulumiBackend) RedactedOutputs(ctx context.Context) (map[string]any, error) {
	if err := b.requireStack(); err != nil {
		return nil, err
	}
	outputs, err := b.stack.Outputs(ctx)
	if err != nil {
		return nil, err
	}
	return redactSecretOutputs(outputs), nil
}

func mapStackOutputs(outputs auto.OutputMap) map[string]any {
	result := make(map[string]any, len(outputs))
	for name, output := range outputs {
		result[name] = output.Value
	}
	return result
}

func redactSecretOutputs(outputs auto.OutputMap) map[string]any {
	result := make(map[string]any, len(outputs))
	for name, output := range outputs {
		if output.Secret {
			result[name] = map[string]any{"secret": true}
			continue
		}
		result[name] = output.Value
	}
	return result
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
