package v1

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ResilienceOperationStatus is the provider-neutral state of one asynchronous
// recovery operation. Provider adapters translate native states into these
// values before they cross the SDK boundary.
type ResilienceOperationStatus string

const (
	ResilienceOperationPending   ResilienceOperationStatus = "pending"
	ResilienceOperationRunning   ResilienceOperationStatus = "running"
	ResilienceOperationSucceeded ResilienceOperationStatus = "succeeded"
	ResilienceOperationFailed    ResilienceOperationStatus = "failed"
)

// ResilienceProofEvidence contains only safe, independently verifiable facts
// about a recovery stage. It intentionally has no provider SDK fields or
// resolved secret values. One operation may return several proofs for a
// fencing, failover, or cleanup stage.
type ResilienceProofEvidence struct {
	DataClass                string `json:"dataClass" yaml:"dataClass"`
	Destination              string `json:"destination,omitempty" yaml:"destination,omitempty"`
	BackupID                 string `json:"backupId,omitempty" yaml:"backupId,omitempty"`
	RestoreID                string `json:"restoreId,omitempty" yaml:"restoreId,omitempty"`
	FixtureID                string `json:"fixtureId,omitempty" yaml:"fixtureId,omitempty"`
	RetentionDays            int    `json:"retentionDays,omitempty" yaml:"retentionDays,omitempty"`
	EncryptionVerified       bool   `json:"encryptionVerified" yaml:"encryptionVerified"`
	ProtectionVerified       bool   `json:"protectionVerified" yaml:"protectionVerified"`
	ManifestVerified         bool   `json:"manifestVerified" yaml:"manifestVerified"`
	CountsVerified           bool   `json:"countsVerified" yaml:"countsVerified"`
	ApplicationReadsVerified bool   `json:"applicationReadsVerified" yaml:"applicationReadsVerified"`
	PermissionsVerified      bool   `json:"permissionsVerified" yaml:"permissionsVerified"`
	SecretReferencesVerified bool   `json:"secretReferencesVerified" yaml:"secretReferencesVerified"`
	ServiceHealthVerified    bool   `json:"serviceHealthVerified" yaml:"serviceHealthVerified"`
	// WriterIdentityRef and WriterEpoch are opaque, non-secret identities. A
	// provider must never place a lease token or credential value in evidence.
	WriterIdentityRef       string `json:"writerIdentityRef,omitempty" yaml:"writerIdentityRef,omitempty"`
	WriterEpoch             string `json:"writerEpoch,omitempty" yaml:"writerEpoch,omitempty"`
	SingleWriterVerified    bool   `json:"singleWriterVerified" yaml:"singleWriterVerified"`
	SplitBrainAbsent        bool   `json:"splitBrainAbsent" yaml:"splitBrainAbsent"`
	StaleOriginRejected     bool   `json:"staleOriginRejected" yaml:"staleOriginRejected"`
	ReconciliationVerified  bool   `json:"reconciliationVerified" yaml:"reconciliationVerified"`
	ApprovalVerified        bool   `json:"approvalVerified" yaml:"approvalVerified"`
	MeasuredDurationSeconds int64  `json:"measuredDurationSeconds,omitempty" yaml:"measuredDurationSeconds,omitempty"`
	MeasuredRPOSeconds      int64  `json:"measuredRpoSeconds,omitempty" yaml:"measuredRpoSeconds,omitempty"`
	MeasuredRTOSeconds      int64  `json:"measuredRtoSeconds,omitempty" yaml:"measuredRtoSeconds,omitempty"`
	Reason                  string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// ResilienceOperationRequest is the only input a provider lifecycle client
// receives. ResourceReferences are opaque single-line identities selected by
// the provider adapter; they are not vendor-shaped configuration.
type ResilienceOperationRequest struct {
	Action                  ResilienceAction    `json:"action" yaml:"action"`
	DataClasses             []string            `json:"dataClasses" yaml:"dataClasses"`
	Destination             RecoveryDestination `json:"destination,omitempty" yaml:"destination,omitempty"`
	FixtureID               string              `json:"fixtureId" yaml:"fixtureId"`
	OwnershipMarker         string              `json:"ownershipMarker" yaml:"ownershipMarker"`
	IdempotencyKey          string              `json:"idempotencyKey" yaml:"idempotencyKey"`
	ResourceReferences      map[string]string   `json:"resourceReferences,omitempty" yaml:"resourceReferences,omitempty"`
	BackupReferences        map[string]string   `json:"backupReferences,omitempty" yaml:"backupReferences,omitempty"`
	ApprovalReference       string              `json:"approvalReference,omitempty" yaml:"approvalReference,omitempty"`
	ReconciliationReference string              `json:"reconciliationReference,omitempty" yaml:"reconciliationReference,omitempty"`
}

// ResilienceOperationObservation is the normalized result of starting or
// polling a provider operation. A provider must attest ownership and
// idempotency before a successful observation can be checkpointed.
type ResilienceOperationObservation struct {
	Status              ResilienceOperationStatus `json:"status" yaml:"status"`
	Action              ResilienceAction          `json:"action" yaml:"action"`
	OperationID         string                    `json:"operationId" yaml:"operationId"`
	ResourceRefs        []string                  `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	ProofRefs           []string                  `json:"proofRefs,omitempty" yaml:"proofRefs,omitempty"`
	OwnershipMarker     string                    `json:"ownershipMarker" yaml:"ownershipMarker"`
	OwnershipVerified   bool                      `json:"ownershipVerified" yaml:"ownershipVerified"`
	IdempotencyVerified bool                      `json:"idempotencyVerified" yaml:"idempotencyVerified"`
	Evidence            []ResilienceProofEvidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	Detail              string                    `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// ResilienceInventoryResource is an owning-service inventory observation.
// Identity is opaque and Owned is required to be true before a cleanup
// operation may remove the resource.
type ResilienceInventoryResource struct {
	Identity string `json:"identity" yaml:"identity"`
	Owned    bool   `json:"owned" yaml:"owned"`
	Live     bool   `json:"live" yaml:"live"`
}

// ResilienceOperationClient is implemented by a first-party or community
// provider adapter. It is deliberately smaller than any cloud SDK client and
// keeps provider API types at the edge of the system.
type ResilienceOperationClient interface {
	Start(context.Context, ResilienceOperationRequest) (ResilienceOperationObservation, error)
	Poll(context.Context, string) (ResilienceOperationObservation, error)
	Inventory(context.Context, string) ([]ResilienceInventoryResource, error)
}

// ResilienceOperationPolicy bounds provider waits. A zero value is invalid so
// callers cannot accidentally turn a cloud control-plane wait into an
// unbounded process.
type ResilienceOperationPolicy struct {
	Timeout      time.Duration `json:"timeout" yaml:"timeout"`
	PollInterval time.Duration `json:"pollInterval" yaml:"pollInterval"`
	MaxAttempts  int           `json:"maxAttempts" yaml:"maxAttempts"`
}

// DefaultResilienceOperationPolicy is intentionally conservative for real
// managed-service operations. Tests should pass a short explicit policy.
func DefaultResilienceOperationPolicy() ResilienceOperationPolicy {
	return ResilienceOperationPolicy{Timeout: 30 * time.Minute, PollInterval: 5 * time.Second, MaxAttempts: 360}
}

// WaitForResilienceOperation polls a provider operation with cancellation,
// timeout, and attempt limits. Unknown or unfinished states never pass.
func WaitForResilienceOperation(ctx context.Context, client ResilienceOperationClient, operationID string, policy ResilienceOperationPolicy) (ResilienceOperationObservation, error) {
	if ctx == nil {
		return ResilienceOperationObservation{}, errors.New("resilience operation context is required")
	}
	if client == nil {
		return ResilienceOperationObservation{}, errors.New("resilience operation client is required")
	}
	if strings.TrimSpace(operationID) == "" {
		return ResilienceOperationObservation{}, errors.New("resilience operation ID is required")
	}
	if err := validateResilienceOperationPolicy(policy); err != nil {
		return ResilienceOperationObservation{}, err
	}
	operationCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
	defer cancel()
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := operationCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return ResilienceOperationObservation{}, fmt.Errorf("resilience operation %q timed out after %s", operationID, policy.Timeout)
			}
			return ResilienceOperationObservation{}, err
		}
		observation, err := client.Poll(operationCtx, operationID)
		if err != nil {
			if attempt == policy.MaxAttempts {
				return ResilienceOperationObservation{}, fmt.Errorf("poll resilience operation %q after %d attempts: %w", operationID, attempt, err)
			}
		} else {
			if observation.OperationID != operationID {
				return ResilienceOperationObservation{}, fmt.Errorf("resilience operation poll returned identity %q, want %q", observation.OperationID, operationID)
			}
			switch observation.Status {
			case ResilienceOperationSucceeded:
				return observation, nil
			case ResilienceOperationFailed:
				if strings.TrimSpace(observation.Detail) == "" {
					observation.Detail = "provider reported resilience operation failure"
				}
				return observation, fmt.Errorf("resilience operation %q failed: %s", operationID, observation.Detail)
			case ResilienceOperationPending, ResilienceOperationRunning:
			default:
				return ResilienceOperationObservation{}, fmt.Errorf("resilience operation %q returned unknown status %q", operationID, observation.Status)
			}
		}
		if attempt == policy.MaxAttempts {
			return ResilienceOperationObservation{}, fmt.Errorf("resilience operation %q did not complete after %d attempts", operationID, attempt)
		}
		timer := time.NewTimer(policy.PollInterval)
		select {
		case <-operationCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			if errors.Is(operationCtx.Err(), context.DeadlineExceeded) {
				return ResilienceOperationObservation{}, fmt.Errorf("resilience operation %q timed out after %s", operationID, policy.Timeout)
			}
			return ResilienceOperationObservation{}, operationCtx.Err()
		case <-timer.C:
		}
	}
	return ResilienceOperationObservation{}, fmt.Errorf("resilience operation %q did not complete", operationID)
}

func validateResilienceOperationPolicy(policy ResilienceOperationPolicy) error {
	if policy.Timeout <= 0 || policy.PollInterval <= 0 || policy.MaxAttempts <= 0 {
		return errors.New("resilience operation policy requires positive timeout, poll interval, and max attempts")
	}
	return nil
}

// OperationBackedResilienceAdapter is the reusable SDK implementation for
// provider adapters. Provider packages supply only the descriptor and the
// client that translates semantic operations to native APIs.
type OperationBackedResilienceAdapter struct {
	descriptor ResilienceAdapterDescriptor
	client     ResilienceOperationClient
	policy     ResilienceOperationPolicy
}

var _ ResilienceAdapter = (*OperationBackedResilienceAdapter)(nil)

// NewOperationBackedResilienceAdapter validates the provider descriptor and
// returns a lifecycle adapter that shares planning, polling, ownership, and
// idempotency behavior across providers.
func NewOperationBackedResilienceAdapter(descriptor ResilienceAdapterDescriptor, client ResilienceOperationClient, policy ResilienceOperationPolicy) (*OperationBackedResilienceAdapter, error) {
	if err := ValidateResilienceAdapterDescriptor(descriptor); err != nil {
		return nil, fmt.Errorf("validate operation-backed resilience adapter: %w", err)
	}
	if client == nil {
		return nil, errors.New("operation-backed resilience adapter client is required")
	}
	if err := validateResilienceOperationPolicy(policy); err != nil {
		return nil, err
	}
	return &OperationBackedResilienceAdapter{descriptor: descriptor, client: client, policy: policy}, nil
}

func (a *OperationBackedResilienceAdapter) ResilienceDescriptor() ResilienceAdapterDescriptor {
	if a == nil {
		return ResilienceAdapterDescriptor{}
	}
	return a.descriptor
}

func (a *OperationBackedResilienceAdapter) PlanResilience(ctx context.Context, request ResiliencePlanRequest) (ResiliencePlan, error) {
	if a == nil {
		return ResiliencePlan{}, errors.New("operation-backed resilience adapter is required")
	}
	if ctx == nil {
		return ResiliencePlan{}, errors.New("resilience planning context is required")
	}
	if err := ctx.Err(); err != nil {
		return ResiliencePlan{}, err
	}
	return CompileResiliencePlan(a.descriptor, request)
}

func (a *OperationBackedResilienceAdapter) ExecuteResilience(ctx context.Context, request ResilienceExecutionRequest) (ResilienceExecutionResult, error) {
	if a == nil {
		return ResilienceExecutionResult{}, errors.New("operation-backed resilience adapter is required")
	}
	if ctx == nil {
		return ResilienceExecutionResult{}, errors.New("resilience execution context is required")
	}
	if err := ValidateResilienceExecutionRequest(request); err != nil {
		return ResilienceExecutionResult{}, err
	}
	if strings.TrimSpace(request.FixtureID) == "" {
		return ResilienceExecutionResult{}, errors.New("operation-backed resilience execution fixture ID is required")
	}
	stage, found := findResilienceStage(request.Plan, request.StageID)
	if !found {
		return ResilienceExecutionResult{}, fmt.Errorf("resilience stage %q is not present in the plan", request.StageID)
	}
	operationRequest := ResilienceOperationRequest{
		Action:                  stage.Action,
		DataClasses:             append([]string(nil), stage.DataClasses...),
		Destination:             stage.Destination,
		FixtureID:               request.FixtureID,
		OwnershipMarker:         request.OwnershipMarker,
		IdempotencyKey:          request.IdempotencyKey,
		ResourceReferences:      cloneStringMap(request.ResourceReferences),
		BackupReferences:        cloneStringMap(request.BackupReferences),
		ApprovalReference:       request.ApprovalReference,
		ReconciliationReference: request.ReconciliationReference,
	}
	observation, err := a.client.Start(ctx, operationRequest)
	if err != nil {
		return ResilienceExecutionResult{}, fmt.Errorf("start resilience operation %q: %w", stage.ID, err)
	}
	if observation.OperationID == "" && observation.Status != ResilienceOperationSucceeded {
		return ResilienceExecutionResult{}, fmt.Errorf("resilience stage %q returned no operation identity", stage.ID)
	}
	if observation.Status == ResilienceOperationPending || observation.Status == ResilienceOperationRunning {
		observation, err = WaitForResilienceOperation(ctx, a.client, observation.OperationID, a.policy)
		if err != nil {
			return ResilienceExecutionResult{}, fmt.Errorf("wait for resilience stage %q: %w", stage.ID, err)
		}
	}
	if observation.Status != ResilienceOperationSucceeded {
		return ResilienceExecutionResult{}, fmt.Errorf("resilience stage %q returned non-success status %q", stage.ID, observation.Status)
	}
	if observation.Action != "" && observation.Action != stage.Action {
		return ResilienceExecutionResult{}, fmt.Errorf("resilience stage %q returned action %q, want %q", stage.ID, observation.Action, stage.Action)
	}
	if observation.OwnershipMarker != request.OwnershipMarker || !observation.OwnershipVerified {
		return ResilienceExecutionResult{}, fmt.Errorf("resilience stage %q did not verify ownership scope", stage.ID)
	}
	if !observation.IdempotencyVerified {
		return ResilienceExecutionResult{}, fmt.Errorf("resilience stage %q did not verify idempotency", stage.ID)
	}
	if err := validateResilienceOperationEvidence(a.descriptor, stage, operationRequest, observation); err != nil {
		return ResilienceExecutionResult{}, fmt.Errorf("validate resilience stage %q evidence: %w", stage.ID, err)
	}
	if observation.OperationID == "" && len(observation.ResourceRefs) == 0 && len(observation.ProofRefs) == 0 {
		return ResilienceExecutionResult{}, fmt.Errorf("resilience stage %q returned no operation, resource, or proof identity", stage.ID)
	}
	return ResilienceExecutionResult{
		Action:              stage.Action,
		OperationID:         observation.OperationID,
		ResourceRefs:        append([]string(nil), observation.ResourceRefs...),
		ProofRefs:           append([]string(nil), observation.ProofRefs...),
		OwnershipMarker:     observation.OwnershipMarker,
		OwnershipVerified:   observation.OwnershipVerified,
		IdempotencyVerified: observation.IdempotencyVerified,
		Evidence:            append([]ResilienceProofEvidence(nil), observation.Evidence...),
	}, nil
}

func validateResilienceOperationEvidence(descriptor ResilienceAdapterDescriptor, stage ResilienceStage, request ResilienceOperationRequest, observation ResilienceOperationObservation) error {
	capabilities := make(map[string]ResilienceDataClassCapability, len(descriptor.DataClasses))
	for _, capability := range descriptor.DataClasses {
		capabilities[capability.Name] = capability
	}

	needsClassEvidence := stage.Action == ResilienceBackup || stage.Action == ResilienceRestore || stage.Action == ResilienceIntegrityCheck || stage.Action == ResilienceFence || stage.Action == ResilienceFailover || stage.Action == ResilienceFailback
	if !needsClassEvidence {
		return nil
	}
	byClass := make(map[string]ResilienceProofEvidence, len(observation.Evidence))
	for _, evidence := range observation.Evidence {
		if err := validateID("resilience proof data class", evidence.DataClass); err != nil {
			return err
		}
		if _, exists := byClass[evidence.DataClass]; exists {
			return fmt.Errorf("duplicate resilience proof evidence for data class %q", evidence.DataClass)
		}
		if evidence.FixtureID != request.FixtureID {
			return fmt.Errorf("resilience proof for data class %q has fixture %q, want %q", evidence.DataClass, evidence.FixtureID, request.FixtureID)
		}
		if stage.Destination != "" && evidence.Destination != string(stage.Destination) {
			return fmt.Errorf("resilience proof for data class %q has destination %q, want %q", evidence.DataClass, evidence.Destination, stage.Destination)
		}
		if evidence.RetentionDays < 0 || evidence.MeasuredDurationSeconds < 0 || evidence.MeasuredRPOSeconds < 0 || evidence.MeasuredRTOSeconds < 0 {
			return fmt.Errorf("resilience proof for data class %q contains a negative measurement", evidence.DataClass)
		}
		if stage.RequiresSingleWriter || stage.RequiresSplitBrainCheck || stage.RequiresStaleOriginCheck || stage.RequiresReconciliation || stage.RequiresApproval {
			if strings.TrimSpace(evidence.WriterIdentityRef) == "" || strings.TrimSpace(evidence.WriterEpoch) == "" {
				return fmt.Errorf("resilience proof for data class %q requires an opaque writer identity and epoch", evidence.DataClass)
			}
			if stage.RequiresSingleWriter && !evidence.SingleWriterVerified {
				return fmt.Errorf("resilience proof for data class %q did not verify single-writer ownership", evidence.DataClass)
			}
			if stage.RequiresSplitBrainCheck && !evidence.SplitBrainAbsent {
				return fmt.Errorf("resilience proof for data class %q did not verify split-brain absence", evidence.DataClass)
			}
			if stage.RequiresStaleOriginCheck && !evidence.StaleOriginRejected {
				return fmt.Errorf("resilience proof for data class %q did not reject the stale origin", evidence.DataClass)
			}
			if stage.RequiresReconciliation && !evidence.ReconciliationVerified {
				return fmt.Errorf("resilience proof for data class %q did not verify reconciliation", evidence.DataClass)
			}
			if stage.RequiresApproval && !evidence.ApprovalVerified {
				return fmt.Errorf("resilience proof for data class %q did not verify the approved transition", evidence.DataClass)
			}
		}
		capability, exists := capabilities[evidence.DataClass]
		if !exists {
			return fmt.Errorf("resilience proof references undeclared data class %q", evidence.DataClass)
		}
		if capability.RetentionRequired && evidence.RetentionDays <= 0 {
			return fmt.Errorf("resilience proof for durable data class %q must include positive retention", evidence.DataClass)
		}
		if capability.EncryptionRequired && !evidence.EncryptionVerified {
			return fmt.Errorf("resilience proof for durable data class %q did not verify encryption", evidence.DataClass)
		}
		if capability.ProtectionRequired && !evidence.ProtectionVerified {
			return fmt.Errorf("resilience proof for durable data class %q did not verify protection", evidence.DataClass)
		}
		byClass[evidence.DataClass] = evidence
	}
	for _, dataClass := range stage.DataClasses {
		evidence, exists := byClass[dataClass]
		if !exists {
			return fmt.Errorf("resilience stage %q has no independent evidence for data class %q", stage.ID, dataClass)
		}
		switch stage.Action {
		case ResilienceBackup:
			if strings.TrimSpace(evidence.BackupID) == "" {
				return fmt.Errorf("backup proof for data class %q is missing a backup identity", dataClass)
			}
		case ResilienceRestore:
			if strings.TrimSpace(evidence.RestoreID) == "" || !evidence.ServiceHealthVerified {
				return fmt.Errorf("restore proof for data class %q requires a restore identity and service-health verification", dataClass)
			}
		case ResilienceIntegrityCheck:
			if !evidence.ManifestVerified && !evidence.CountsVerified && !evidence.ApplicationReadsVerified {
				return fmt.Errorf("integrity proof for data class %q must verify a manifest, count, or application read", dataClass)
			}
			if !evidence.PermissionsVerified || !evidence.ServiceHealthVerified {
				return fmt.Errorf("integrity proof for data class %q requires permissions and service-health verification", dataClass)
			}
			if strings.Contains(dataClass, "secret") && !evidence.SecretReferencesVerified {
				return fmt.Errorf("integrity proof for data class %q requires secret-reference verification", dataClass)
			}
		case ResilienceFailover:
			if !evidence.ServiceHealthVerified {
				return fmt.Errorf("failover proof for data class %q requires service-health verification", dataClass)
			}
		case ResilienceFence, ResilienceFailback:
			// The safety flags above carry the provider-neutral fencing proof.
		}
	}
	return nil
}

// Inventory exposes the provider-owned final cleanup truth without extending
// the portable ResilienceAdapter interface. Certification callers can use the
// optional interface when they need a direct owning-service assertion.
func (a *OperationBackedResilienceAdapter) Inventory(ctx context.Context, ownershipMarker string) ([]ResilienceInventoryResource, error) {
	if a == nil || a.client == nil {
		return nil, errors.New("operation-backed resilience adapter is required")
	}
	if strings.TrimSpace(ownershipMarker) == "" || strings.ContainsAny(ownershipMarker, "\r\n\x00") {
		return nil, errors.New("resilience inventory ownership marker is required and must be single-line")
	}
	return a.client.Inventory(ctx, ownershipMarker)
}

func findResilienceStage(plan ResiliencePlan, id string) (ResilienceStage, bool) {
	for _, stage := range plan.Stages {
		if stage.ID == id {
			return stage, true
		}
	}
	return ResilienceStage{}, false
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
