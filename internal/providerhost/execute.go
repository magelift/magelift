package providerhost

import (
	"errors"
	"fmt"

	"github.com/magelift/magelift/internal/automation"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// ExecuteOperation is a stack lifecycle operation the host asks a provider
// subprocess to run. A Pulumi RunFunc cannot cross gRPC, so the subprocess
// executes with its own Automation API stack and returns structured results.
type ExecuteOperation string

const (
	ExecutePreview         ExecuteOperation = "preview"
	ExecuteUp              ExecuteOperation = "up"
	ExecuteDestroy         ExecuteOperation = "destroy"
	ExecuteOutputs         ExecuteOperation = "outputs"
	ExecuteRedactedOutputs ExecuteOperation = "redacted-outputs"
	ExecuteValidateRequest ExecuteOperation = "validate-request"
)

// Execute wire size caps. Diagnostics beyond the cap are truncated with
// Truncated set; the operation result itself must fit.
const (
	executeRequestLimit = 1 << 20
	executeResultLimit  = 1 << 20
	maxDiagnostics      = 200
)

// ExecuteRequest carries everything a subprocess needs to run one stack
// operation. Preview holds identifiers only, never secret values. BackendURL
// is non-secret backend identity; the test://mock URL selects the mock
// executor used by Dial tests instead of Automation API.
type ExecuteRequest struct {
	Operation  ExecuteOperation            `json:"operation"`
	Plan       sdk.ModulePlan              `json:"plan"`
	BackendURL string                      `json:"backendURL,omitempty"`
	Preview    *automation.PreviewMetadata `json:"preview,omitempty"`
}

// OwnershipConflict carries a preview ownership verdict across gRPC, where
// Go error types do not survive. The host rebuilds an
// automation.PreviewOwnershipError from it plus the request it sent.
type OwnershipConflict struct {
	Operation         string `json:"operation"`
	Reason            string `json:"reason"`
	CurrentGeneration uint64 `json:"currentGeneration,omitempty"`
	HasCurrent        bool   `json:"hasCurrent,omitempty"`
}

// ExecuteResult is the subprocess answer. Changes serves preview/up/destroy;
// Outputs serves the outputs op with secrets decrypted for in-process day-2
// consumers (kubeconfig, like in-process Outputs) and the redacted-outputs
// op with secrets replaced by {"secret": true} for user-facing display;
// OwnershipConflict is set instead of Changes when the ownership guard
// refuses the operation.
type ExecuteResult struct {
	Operation         ExecuteOperation   `json:"operation"`
	Changes           map[string]int     `json:"changes,omitempty"`
	Outputs           map[string]any     `json:"outputs,omitempty"`
	OwnershipConflict *OwnershipConflict `json:"ownershipConflict,omitempty"`
	// ConcurrentUpdate is set instead of Changes when the subprocess
	// classifies the failure as a Pulumi stack lock or 409 conflict. Go
	// error types do not survive gRPC, so the host rebuilds an
	// automation.ConcurrentUpdateError from this flag.
	ConcurrentUpdate bool     `json:"concurrentUpdate,omitempty"`
	Diagnostics      []string `json:"diagnostics,omitempty"`
	Truncated        bool     `json:"truncated,omitempty"`
}

func (o ExecuteOperation) valid() bool {
	switch o {
	case ExecutePreview, ExecuteUp, ExecuteDestroy, ExecuteOutputs, ExecuteRedactedOutputs, ExecuteValidateRequest:
		return true
	default:
		return false
	}
}

// Validate rejects unknown operations and plans without a stack name before
// anything crosses the subprocess boundary.
func (r ExecuteRequest) Validate() error {
	if !r.Operation.valid() {
		return fmt.Errorf("execute operation %q is not supported", r.Operation)
	}
	if r.Plan.StackName == "" {
		return errors.New("execute plan stack name is required")
	}
	if r.Preview != nil {
		if err := r.Preview.Validate(); err != nil {
			return fmt.Errorf("execute preview metadata: %w", err)
		}
	}
	return nil
}

// ToOwnershipError rebuilds the typed ownership error from a result payload
// plus the request metadata the host sent.
func (c *OwnershipConflict) ToOwnershipError(requested automation.PreviewMetadata) error {
	if c == nil {
		return nil
	}
	conflict := &automation.PreviewOwnershipError{
		Cause:     automation.ErrPreviewOwnershipConflict,
		Operation: c.Operation,
		Reason:    c.Reason,
		Requested: requested,
	}
	if c.HasCurrent {
		conflict.Current = &automation.PreviewMetadata{Generation: c.CurrentGeneration}
	}
	return conflict
}

// OwnershipConflictFromError converts an ownership error into its wire form.
// It returns nil when err carries no ownership failure.
func OwnershipConflictFromError(err error) *OwnershipConflict {
	var ownershipErr *automation.PreviewOwnershipError
	if !errors.As(err, &ownershipErr) || ownershipErr == nil {
		return nil
	}
	conflict := &OwnershipConflict{
		Operation: ownershipErr.Operation,
		Reason:    ownershipErr.Reason,
	}
	if ownershipErr.Current != nil {
		conflict.CurrentGeneration = ownershipErr.Current.Generation
		conflict.HasCurrent = true
	}
	return conflict
}

// TruncateDiagnostics caps collected diagnostics, oldest dropped first.
func TruncateDiagnostics(diagnostics []string) ([]string, bool) {
	if len(diagnostics) <= maxDiagnostics {
		return diagnostics, false
	}
	return diagnostics[len(diagnostics)-maxDiagnostics:], true
}
