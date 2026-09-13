package recovery

import (
	"context"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	ProjectionSearchIndex = "search-index"
	ProjectionCache       = "cache"
)

// ProjectionRequest is the provider-neutral input for rebuilding a
// reconstructible projection. Search indexes and caches intentionally share
// the lifecycle boundary but not the proof requirements: a cache loss is
// expected, while a search rebuild must prove that the projection is complete.
// Provider adapters receive opaque resource identities and translate them to
// ECS, Kubernetes, or another runtime's execution API.
type ProjectionRequest struct {
	Action          sdk.ResilienceAction
	DataClass       string
	SourceReference string
	TargetReference string
	OperationID     string
	Destination     sdk.RecoveryDestination
	FixtureID       string
	OwnershipMarker string
	IdempotencyKey  string
}

// ProjectionResult contains only normalized proof facts. It must never carry
// provider SDK responses, command output, or resolved credentials.
type ProjectionResult struct {
	Status                   sdk.ResilienceOperationStatus
	OperationID              string
	ResourceReference        string
	ProofReferences          []string
	FixtureID                string
	OwnershipMarker          string
	IdempotencyVerified      bool
	ManifestVerified         bool
	CountsVerified           bool
	ApplicationReadsVerified bool
	PermissionsVerified      bool
	SecretReferencesVerified bool
	ServiceHealthVerified    bool
	CacheLossClassified      bool
	RestoreDurationSeconds   int64
	Reason                   string
}

// ProjectionBackend is implemented by a provider/runtime adapter. The core
// owns the contract validation and lifecycle ordering; the adapter owns only
// the native command/API calls needed to rebuild and inspect the projection.
type ProjectionBackend interface {
	Rebuild(context.Context, ProjectionRequest) (ProjectionResult, error)
	Verify(context.Context, ProjectionRequest) (ProjectionResult, error)
}

// ProjectionCommandRequest is the provider-neutral semantic execution
// request. Providers receive the Magento command selected by the core but
// remain responsible for translating it to their native runtime transport.
type ProjectionCommandRequest struct {
	Projection ProjectionRequest
	Command    []string
}

// ProjectionCommandResult contains transient command output for the injected
// verifier and normalized execution facts for the shared proof contract.
// Stdout and Stderr are wiped by CommandProjectionBackend after verification.
type ProjectionCommandResult struct {
	Status                 sdk.ResilienceOperationStatus
	OperationID            string
	ResourceReference      string
	ProofReferences        []string
	Stdout                 []byte
	Stderr                 []byte
	RestoreDurationSeconds int64
	Reason                 string
}

// ProjectionCommandRunner is implemented by a provider/runtime adapter. It
// must not return provider SDK objects or resolved credentials.
type ProjectionCommandRunner interface {
	Run(context.Context, ProjectionCommandRequest) (ProjectionCommandResult, error)
}

// ProjectionVerifier validates known-content and application observations
// without returning raw command output or secret values in ProjectionResult.
type ProjectionVerifier func(context.Context, ProjectionRequest, ProjectionCommandRequest, ProjectionCommandResult) (ProjectionResult, error)

// CommandProjectionBackend centralizes the lifecycle glue shared by ECS and
// Kubernetes runtimes. Native adapters only select a transport and an
// application verifier; they do not invent separate result semantics.
type CommandProjectionBackend struct {
	runner   ProjectionCommandRunner
	verifier ProjectionVerifier
}

var _ ProjectionBackend = (*CommandProjectionBackend)(nil)

func NewCommandProjectionBackend(runner ProjectionCommandRunner, verifier ProjectionVerifier) (*CommandProjectionBackend, error) {
	if runner == nil {
		return nil, errors.New("projection command runner is required")
	}
	if verifier == nil {
		return nil, errors.New("projection verifier is required")
	}
	return &CommandProjectionBackend{runner: runner, verifier: verifier}, nil
}

func (backend *CommandProjectionBackend) Rebuild(ctx context.Context, request ProjectionRequest) (ProjectionResult, error) {
	if err := ValidateProjectionRequest(request, sdk.ResilienceRestore); err != nil {
		return ProjectionResult{}, err
	}
	return backend.execute(ctx, request)
}

func (backend *CommandProjectionBackend) Verify(ctx context.Context, request ProjectionRequest) (ProjectionResult, error) {
	if err := ValidateProjectionRequest(request, sdk.ResilienceIntegrityCheck); err != nil {
		return ProjectionResult{}, err
	}
	return backend.execute(ctx, request)
}

func (backend *CommandProjectionBackend) execute(ctx context.Context, request ProjectionRequest) (ProjectionResult, error) {
	if backend == nil || backend.runner == nil || backend.verifier == nil {
		return ProjectionResult{}, errors.New("projection command backend is not configured")
	}
	if ctx == nil {
		return ProjectionResult{}, errors.New("projection command context is required")
	}
	if err := ctx.Err(); err != nil {
		return ProjectionResult{}, err
	}
	command, err := MagentoProjectionCommand(request)
	if err != nil {
		return ProjectionResult{}, err
	}
	executionRequest := ProjectionCommandRequest{Projection: request, Command: command}
	execution, err := backend.runner.Run(ctx, executionRequest)
	if err != nil {
		return ProjectionResult{}, fmt.Errorf("run %s projection command: %w", request.DataClass, err)
	}
	defer wipeProjectionOutput(execution.Stdout, execution.Stderr)
	if strings.ContainsAny(execution.Reason, "\r\n\x00") {
		return ProjectionResult{}, errors.New("projection execution reason must be single-line")
	}
	result, err := backend.verifier(ctx, request, executionRequest, execution)
	if err != nil {
		return ProjectionResult{}, fmt.Errorf("verify %s projection: %w", request.DataClass, err)
	}
	if result.Status != "" && result.Status != execution.Status {
		return ProjectionResult{}, errors.New("projection verifier returned a different operation status")
	}
	if result.OperationID != "" && result.OperationID != execution.OperationID {
		return ProjectionResult{}, errors.New("projection verifier returned a different operation identity")
	}
	if result.ResourceReference != "" && result.ResourceReference != execution.ResourceReference {
		return ProjectionResult{}, errors.New("projection verifier returned a different resource identity")
	}
	result.Status = execution.Status
	result.OperationID = execution.OperationID
	result.ResourceReference = execution.ResourceReference
	result.ProofReferences = append(append([]string(nil), execution.ProofReferences...), result.ProofReferences...)
	result.RestoreDurationSeconds = execution.RestoreDurationSeconds
	// The verifier can inspect transient output, but only the transport-owned,
	// single-line reason crosses into portable evidence.
	result.Reason = execution.Reason
	return result, nil
}

// MagentoProjectionCommand keeps the workload operation semantics in one
// provider-neutral place while leaving transport and proof verification to
// adapters.
func MagentoProjectionCommand(request ProjectionRequest) ([]string, error) {
	switch {
	case request.DataClass == ProjectionSearchIndex && request.Action == sdk.ResilienceRestore:
		return []string{"bin/magento", "indexer:reindex"}, nil
	case request.DataClass == ProjectionSearchIndex && request.Action == sdk.ResilienceIntegrityCheck:
		return []string{"bin/magento", "indexer:status"}, nil
	case request.DataClass == ProjectionCache && request.Action == sdk.ResilienceRestore:
		return []string{"bin/magento", "cache:flush"}, nil
	case request.DataClass == ProjectionCache && request.Action == sdk.ResilienceIntegrityCheck:
		return []string{"bin/magento", "cache:status"}, nil
	default:
		return nil, fmt.Errorf("unsupported projection command for action %q and data class %q", request.Action, request.DataClass)
	}
}

func wipeProjectionOutput(outputs ...[]byte) {
	for _, output := range outputs {
		clear(output)
	}
}

// ProjectionLifecycle centralizes validation shared by every provider. This
// prevents AWS, GCP, Scaleway, OVHcloud, and community implementations from
// gradually inventing different meanings for cache loss or search integrity.
type ProjectionLifecycle struct {
	backend ProjectionBackend
}

func NewProjectionLifecycle(backend ProjectionBackend) (*ProjectionLifecycle, error) {
	if backend == nil {
		return nil, errors.New("projection backend is required")
	}
	return &ProjectionLifecycle{backend: backend}, nil
}

func (l *ProjectionLifecycle) Rebuild(ctx context.Context, request ProjectionRequest) (ProjectionResult, error) {
	if err := validateProjectionContext(ctx); err != nil {
		return ProjectionResult{}, err
	}
	if err := ValidateProjectionRequest(request, sdk.ResilienceRestore); err != nil {
		return ProjectionResult{}, err
	}
	result, err := l.backend.Rebuild(ctx, request)
	if err != nil {
		return ProjectionResult{}, err
	}
	if err := ValidateProjectionResult(request, result); err != nil {
		return ProjectionResult{}, fmt.Errorf("validate projection rebuild result: %w", err)
	}
	return result, nil
}

func (l *ProjectionLifecycle) Verify(ctx context.Context, request ProjectionRequest) (ProjectionResult, error) {
	if err := validateProjectionContext(ctx); err != nil {
		return ProjectionResult{}, err
	}
	if err := ValidateProjectionRequest(request, sdk.ResilienceIntegrityCheck); err != nil {
		return ProjectionResult{}, err
	}
	result, err := l.backend.Verify(ctx, request)
	if err != nil {
		return ProjectionResult{}, err
	}
	if err := ValidateProjectionResult(request, result); err != nil {
		return ProjectionResult{}, fmt.Errorf("validate projection verification result: %w", err)
	}
	return result, nil
}

func ValidateProjectionRequest(request ProjectionRequest, action sdk.ResilienceAction) error {
	if request.Action != action {
		return fmt.Errorf("projection action %q does not match %q", request.Action, action)
	}
	if request.DataClass != ProjectionSearchIndex && request.DataClass != ProjectionCache {
		return fmt.Errorf("projection data class %q is not reconstructible", request.DataClass)
	}
	for name, value := range map[string]string{
		"source reference": request.SourceReference,
		"target reference": request.TargetReference,
		"fixture ID":       request.FixtureID,
		"ownership marker": request.OwnershipMarker,
		"idempotency key":  request.IdempotencyKey,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("projection %s is required and must be single-line", name)
		}
	}
	if request.Destination == "" {
		return errors.New("projection recovery destination is required")
	}
	return nil
}

func ValidateProjectionResult(request ProjectionRequest, result ProjectionResult) error {
	if result.Status != sdk.ResilienceOperationSucceeded && result.Status != sdk.ResilienceOperationPending && result.Status != sdk.ResilienceOperationRunning {
		return fmt.Errorf("projection returned unsupported status %q", result.Status)
	}
	if result.Status != sdk.ResilienceOperationSucceeded && strings.TrimSpace(result.OperationID) == "" {
		return errors.New("unfinished projection result requires an operation ID")
	}
	if result.Status == sdk.ResilienceOperationSucceeded && strings.TrimSpace(result.ResourceReference) == "" {
		return errors.New("successful projection result requires a resource reference")
	}
	if result.FixtureID != request.FixtureID {
		return fmt.Errorf("projection result fixture %q does not match %q", result.FixtureID, request.FixtureID)
	}
	if result.OwnershipMarker != request.OwnershipMarker {
		return fmt.Errorf("projection result ownership marker %q does not match %q", result.OwnershipMarker, request.OwnershipMarker)
	}
	if !result.IdempotencyVerified {
		return errors.New("projection result did not verify idempotency")
	}
	if result.RestoreDurationSeconds < 0 {
		return errors.New("projection restore duration cannot be negative")
	}
	if result.Status != sdk.ResilienceOperationSucceeded {
		return nil
	}
	if request.Action == sdk.ResilienceRestore && result.RestoreDurationSeconds <= 0 {
		return errors.New("successful projection restore requires a positive restore duration")
	}
	if !result.ServiceHealthVerified || !result.ApplicationReadsVerified || !result.PermissionsVerified || !result.SecretReferencesVerified {
		return errors.New("successful projection result requires application-read, permission, secret-reference, and service-health verification")
	}
	switch request.DataClass {
	case ProjectionSearchIndex:
		if !result.CountsVerified && !result.ManifestVerified {
			return errors.New("search rebuild requires document counts or a verified manifest")
		}
	case ProjectionCache:
		if !result.CacheLossClassified {
			return errors.New("cache reconstruction must classify cache loss separately from durable recovery")
		}
	}
	return nil
}

// ProjectionProof converts a validated result to the portable evidence shape.
// Retention and encryption remain false by design: reconstructible projections
// are not durable backup claims.
func ProjectionProof(request ProjectionRequest, result ProjectionResult) sdk.ResilienceProofEvidence {
	return sdk.ResilienceProofEvidence{
		DataClass:                request.DataClass,
		Destination:              string(request.Destination),
		RestoreID:                result.ResourceReference,
		FixtureID:                request.FixtureID,
		ManifestVerified:         result.ManifestVerified,
		CountsVerified:           result.CountsVerified,
		ApplicationReadsVerified: result.ApplicationReadsVerified,
		PermissionsVerified:      result.PermissionsVerified,
		SecretReferencesVerified: result.SecretReferencesVerified,
		ServiceHealthVerified:    result.ServiceHealthVerified,
		MeasuredDurationSeconds:  result.RestoreDurationSeconds,
		Reason:                   result.Reason,
	}
}

func validateProjectionContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("projection context is required")
	}
	return ctx.Err()
}
