package certification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/magelift/magelift/sdk"
)

// RecoveryCheckpoint is the durable, provider-neutral state of one recovery
// run. Provider operation IDs and proof references are intentionally opaque;
// the provider adapter owns their meaning while the core owns resume safety.
type RecoveryCheckpoint struct {
	PlanDigest      string                        `json:"planDigest" yaml:"planDigest"`
	OwnershipMarker string                        `json:"ownershipMarker" yaml:"ownershipMarker"`
	FixtureID       string                        `json:"fixtureId" yaml:"fixtureId"`
	Stages          map[string]RecoveryStageState `json:"stages" yaml:"stages"`
	UpdatedAt       string                        `json:"updatedAt" yaml:"updatedAt"`
}

// RecoveryStageState records a stage only after the adapter returned a
// validated result. A cancelled or failed stage is not marked complete and is
// retried with the same idempotency key on resume.
type RecoveryStageState struct {
	Status              Status                        `json:"status" yaml:"status"`
	Action              sdk.ResilienceAction          `json:"action" yaml:"action"`
	OperationID         string                        `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	ResourceRefs        []string                      `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	ProofRefs           []string                      `json:"proofRefs,omitempty" yaml:"proofRefs,omitempty"`
	Evidence            []sdk.ResilienceProofEvidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	OwnershipMarker     string                        `json:"ownershipMarker,omitempty" yaml:"ownershipMarker,omitempty"`
	OwnershipVerified   bool                          `json:"ownershipVerified" yaml:"ownershipVerified"`
	IdempotencyVerified bool                          `json:"idempotencyVerified" yaml:"idempotencyVerified"`
	CompletedAt         string                        `json:"completedAt,omitempty" yaml:"completedAt,omitempty"`
	Reason              string                        `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// RecoveryCheckpointStore is the only persistence dependency required by the
// runner. Implementations can use the existing encrypted Pulumi state
// backend, a local test file, or a community-owned durable store.
type RecoveryCheckpointStore interface {
	Load(context.Context) (RecoveryCheckpoint, error)
	Save(context.Context, RecoveryCheckpoint) error
}

// RecoveryExecution records the result of every stage, including stages
// reused from a checkpoint. Reused stages are evidence of safe resume, not a
// second provider operation.
type RecoveryExecution struct {
	PlanDigest      string                   `json:"planDigest" yaml:"planDigest"`
	OwnershipMarker string                   `json:"ownershipMarker" yaml:"ownershipMarker"`
	FixtureID       string                   `json:"fixtureId" yaml:"fixtureId"`
	Stages          []RecoveryStageExecution `json:"stages" yaml:"stages"`
}

type RecoveryStageExecution struct {
	StageID             string                        `json:"stageId" yaml:"stageId"`
	Action              sdk.ResilienceAction          `json:"action" yaml:"action"`
	Status              Status                        `json:"status" yaml:"status"`
	Reused              bool                          `json:"reused" yaml:"reused"`
	IdempotencyKey      string                        `json:"idempotencyKey" yaml:"idempotencyKey"`
	OperationID         string                        `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	ResourceRefs        []string                      `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	ProofRefs           []string                      `json:"proofRefs,omitempty" yaml:"proofRefs,omitempty"`
	Evidence            []sdk.ResilienceProofEvidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	CompletedAt         string                        `json:"completedAt,omitempty" yaml:"completedAt,omitempty"`
	Reason              string                        `json:"reason,omitempty" yaml:"reason,omitempty"`
	OwnershipVerified   bool                          `json:"ownershipVerified" yaml:"ownershipVerified"`
	IdempotencyVerified bool                          `json:"idempotencyVerified" yaml:"idempotencyVerified"`
}

// RunResilience asks the selected provider adapter to execute the already
// compiled recovery graph. It never invents provider operations: every
// side-effect is made through sdk.ResilienceAdapter and is checkpointed only
// after a successful, identity-bearing result.
func RunResilience(ctx context.Context, adapter sdk.ResilienceAdapter, request sdk.ResiliencePlanRequest, store RecoveryCheckpointStore, now func() time.Time) (RecoveryExecution, error) {
	if ctx == nil {
		return RecoveryExecution{}, errors.New("resilience execution context is required")
	}
	if adapter == nil {
		return RecoveryExecution{}, errors.New("resilience adapter is required")
	}
	if err := sdk.ValidateResiliencePlanRequest(request); err != nil {
		return RecoveryExecution{}, fmt.Errorf("validate resilience request: %w", err)
	}
	if store == nil {
		return RecoveryExecution{}, errors.New("resilience checkpoint store is required")
	}
	if now == nil {
		now = time.Now
	}
	plan, err := adapter.PlanResilience(ctx, request)
	if err != nil {
		return RecoveryExecution{}, fmt.Errorf("plan resilience recovery: %w", err)
	}
	if err := sdk.ValidateResiliencePlan(plan, request.Architecture.Resilience); err != nil {
		return RecoveryExecution{}, fmt.Errorf("validate resilience plan: %w", err)
	}
	digest, err := resiliencePlanDigest(plan)
	if err != nil {
		return RecoveryExecution{}, err
	}
	checkpoint, err := store.Load(ctx)
	if err != nil && !errors.Is(err, ErrRecoveryCheckpointNotFound) {
		return RecoveryExecution{}, fmt.Errorf("load resilience checkpoint: %w", err)
	}
	if err == nil {
		if err := validateRecoveryCheckpoint(checkpoint); err != nil {
			return RecoveryExecution{}, fmt.Errorf("validate resilience checkpoint: %w", err)
		}
	}
	if checkpoint.Stages == nil {
		checkpoint.Stages = make(map[string]RecoveryStageState)
	}
	if checkpoint.PlanDigest != "" && (checkpoint.PlanDigest != digest || checkpoint.OwnershipMarker != request.OwnershipMarker || checkpoint.FixtureID != request.FixtureID) {
		return RecoveryExecution{}, fmt.Errorf("resilience checkpoint does not match plan, fixture, or ownership scope")
	}
	checkpoint.PlanDigest = digest
	checkpoint.OwnershipMarker = request.OwnershipMarker
	checkpoint.FixtureID = request.FixtureID

	ordered, err := topologicalRecoveryStages(plan.Stages)
	if err != nil {
		return RecoveryExecution{}, err
	}
	knownStages := make(map[string]sdk.ResilienceStage, len(ordered))
	for _, stage := range ordered {
		knownStages[stage.ID] = stage
	}
	for stageID, state := range checkpoint.Stages {
		stage, exists := knownStages[stageID]
		if !exists {
			return RecoveryExecution{}, fmt.Errorf("resilience checkpoint contains unknown stage %q", stageID)
		}
		if state.Action != stage.Action {
			return RecoveryExecution{}, fmt.Errorf("resilience checkpoint stage %q action %q does not match plan action %q", stageID, state.Action, stage.Action)
		}
	}
	execution := RecoveryExecution{PlanDigest: digest, OwnershipMarker: request.OwnershipMarker, FixtureID: request.FixtureID, Stages: make([]RecoveryStageExecution, 0, len(ordered))}
	completed := make(map[string]struct{}, len(ordered))
	for _, stage := range ordered {
		for _, dependency := range stage.DependsOn {
			if _, ok := completed[dependency]; !ok {
				return execution, fmt.Errorf("resilience stage %q dependency %q did not complete", stage.ID, dependency)
			}
		}
		key := recoveryIdempotencyKey(digest, request.OwnershipMarker, stage.ID)
		if previous, ok := checkpoint.Stages[stage.ID]; ok && previous.Status == StatusPass && previous.Action == stage.Action && previous.OwnershipMarker == request.OwnershipMarker && previous.OwnershipVerified && previous.IdempotencyVerified {
			completed[stage.ID] = struct{}{}
			execution.Stages = append(execution.Stages, RecoveryStageExecution{StageID: stage.ID, Action: stage.Action, Status: StatusPass, Reused: true, IdempotencyKey: key, OperationID: previous.OperationID, ResourceRefs: append([]string(nil), previous.ResourceRefs...), ProofRefs: append([]string(nil), previous.ProofRefs...), Evidence: append([]sdk.ResilienceProofEvidence(nil), previous.Evidence...), CompletedAt: previous.CompletedAt, OwnershipVerified: true, IdempotencyVerified: true})
			continue
		}
		approvalReference := request.ApprovalReferences[stage.ID]
		resourceReferences := architectureResourceReferences(request.Architecture)
		mergeStringMap(resourceReferences, checkpointRestoreReferences(checkpoint))
		executionRequest := sdk.ResilienceExecutionRequest{Plan: plan, StageID: stage.ID, FixtureID: request.FixtureID, IdempotencyKey: key, OwnershipMarker: request.OwnershipMarker, ResourceReferences: resourceReferences, BackupReferences: checkpointBackupReferences(checkpoint), ApprovalReference: approvalReference, ReconciliationReference: request.ReconciliationReferences[stage.ID]}
		if err := sdk.ValidateResilienceExecutionRequest(executionRequest); err != nil {
			return execution, err
		}
		result, executeErr := adapter.ExecuteResilience(ctx, executionRequest)
		if executeErr != nil {
			return execution, fmt.Errorf("execute resilience stage %q: %w", stage.ID, executeErr)
		}
		if err := validateStageResult(stage, request.FixtureID, request.OwnershipMarker, result); err != nil {
			return execution, err
		}
		completedAt := now().UTC().Format(time.RFC3339Nano)
		state := RecoveryStageState{Status: StatusPass, Action: result.Action, OperationID: result.OperationID, ResourceRefs: append([]string(nil), result.ResourceRefs...), ProofRefs: append([]string(nil), result.ProofRefs...), Evidence: append([]sdk.ResilienceProofEvidence(nil), result.Evidence...), OwnershipMarker: result.OwnershipMarker, OwnershipVerified: result.OwnershipVerified, IdempotencyVerified: result.IdempotencyVerified, CompletedAt: completedAt}
		checkpoint.Stages[stage.ID] = state
		checkpoint.UpdatedAt = completedAt
		if err := saveRecoveryCheckpoint(ctx, store, checkpoint); err != nil {
			return execution, fmt.Errorf("save resilience checkpoint after stage %q: %w", stage.ID, err)
		}
		completed[stage.ID] = struct{}{}
		execution.Stages = append(execution.Stages, RecoveryStageExecution{StageID: stage.ID, Action: result.Action, Status: StatusPass, IdempotencyKey: key, OperationID: result.OperationID, ResourceRefs: append([]string(nil), result.ResourceRefs...), ProofRefs: append([]string(nil), result.ProofRefs...), Evidence: append([]sdk.ResilienceProofEvidence(nil), result.Evidence...), CompletedAt: completedAt, OwnershipVerified: result.OwnershipVerified, IdempotencyVerified: result.IdempotencyVerified})
		if err := ctx.Err(); err != nil {
			return execution, err
		}
	}
	return execution, nil
}

func architectureResourceReferences(architecture sdk.ArchitectureIntent) map[string]string {
	references := make(map[string]string)
	for _, dataClass := range architecture.Resilience.DataClasses {
		for _, boundary := range architecture.Boundaries {
			if boundary.Role == dataClass.Name && strings.TrimSpace(boundary.ResourceReference) != "" {
				references[dataClass.Name] = boundary.ResourceReference
				break
			}
		}
	}
	return references
}

func checkpointBackupReferences(checkpoint RecoveryCheckpoint) map[string]string {
	references := make(map[string]string)
	for _, stage := range checkpoint.Stages {
		if stage.Status != StatusPass || stage.Action != sdk.ResilienceBackup {
			continue
		}
		for _, evidence := range stage.Evidence {
			if strings.TrimSpace(evidence.DataClass) == "" || strings.TrimSpace(evidence.BackupID) == "" {
				continue
			}
			references[evidence.DataClass] = evidence.BackupID
		}
	}
	if len(references) == 0 {
		return nil
	}
	return references
}

// checkpointRestoreReferences carries the last validated isolated restore
// identity into later integrity/failover stages. The core owns this handoff so
// provider adapters do not need an in-memory registry or provider-shaped
// checkpoint format. Evidence is preferred over a generic resource list
// because it names the data class explicitly.
func checkpointRestoreReferences(checkpoint RecoveryCheckpoint) map[string]string {
	type candidate struct {
		dataClass string
		identity  string
		completed string
		stageID   string
	}
	candidates := make([]candidate, 0)
	for stageID, stage := range checkpoint.Stages {
		if stage.Status != StatusPass || stage.Action != sdk.ResilienceRestore {
			continue
		}
		for _, evidence := range stage.Evidence {
			if strings.TrimSpace(evidence.DataClass) == "" || strings.TrimSpace(evidence.RestoreID) == "" {
				continue
			}
			candidates = append(candidates, candidate{dataClass: evidence.DataClass, identity: evidence.RestoreID, completed: stage.CompletedAt, stageID: stageID})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].completed != candidates[j].completed {
			return candidates[i].completed < candidates[j].completed
		}
		return candidates[i].stageID < candidates[j].stageID
	})
	references := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		references[candidate.dataClass] = candidate.identity
	}
	return references
}

func mergeStringMap(target, values map[string]string) {
	for key, value := range values {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		target[key] = value
	}
}

var ErrRecoveryCheckpointNotFound = errors.New("recovery checkpoint not found")

const recoveryCheckpointSaveTimeout = 30 * time.Second

func saveRecoveryCheckpoint(ctx context.Context, store RecoveryCheckpointStore, checkpoint RecoveryCheckpoint) error {
	if err := validateRecoveryCheckpoint(checkpoint); err != nil {
		return fmt.Errorf("validate recovery checkpoint before save: %w", err)
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recoveryCheckpointSaveTimeout)
	defer cancel()
	return store.Save(saveCtx, checkpoint)
}

func validateStageResult(stage sdk.ResilienceStage, fixtureID, ownershipMarker string, result sdk.ResilienceExecutionResult) error {
	if err := ValidateSecretSafeValue(result); err != nil {
		return errors.New("resilience stage result contains secret-like material")
	}
	if result.Action != stage.Action {
		return fmt.Errorf("resilience stage %q returned action %q, want %q", stage.ID, result.Action, stage.Action)
	}
	if len(result.ResourceRefs) == 0 && len(result.ProofRefs) == 0 && strings.TrimSpace(result.OperationID) == "" {
		return fmt.Errorf("resilience stage %q returned no operation, resource, or proof identity", stage.ID)
	}
	if result.OwnershipMarker != ownershipMarker {
		return fmt.Errorf("resilience stage %q returned ownership marker %q, want %q", stage.ID, result.OwnershipMarker, ownershipMarker)
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified {
		return fmt.Errorf("resilience stage %q did not attest ownership and idempotency", stage.ID)
	}
	if err := validateRecoveryStageEvidence(stage, fixtureID, result.Evidence); err != nil {
		return fmt.Errorf("resilience stage %q evidence: %w", stage.ID, err)
	}
	return nil
}

func validateRecoveryStageEvidence(stage sdk.ResilienceStage, fixtureID string, evidence []sdk.ResilienceProofEvidence) error {
	needsEvidence := stage.Action == sdk.ResilienceBackup || stage.Action == sdk.ResilienceRestore || stage.Action == sdk.ResilienceIntegrityCheck || stage.Action == sdk.ResilienceFence || stage.Action == sdk.ResilienceFailover || stage.Action == sdk.ResilienceFailback
	if !needsEvidence {
		return nil
	}
	if len(evidence) == 0 {
		return errors.New("stage requires independent data-class evidence")
	}
	expected := make(map[string]struct{}, len(stage.DataClasses))
	for _, dataClass := range stage.DataClasses {
		expected[dataClass] = struct{}{}
	}
	seen := make(map[string]struct{}, len(evidence))
	for _, proof := range evidence {
		if strings.TrimSpace(proof.DataClass) == "" {
			return errors.New("proof data class is required")
		}
		if _, ok := expected[proof.DataClass]; !ok {
			return fmt.Errorf("proof references data class %q outside the stage", proof.DataClass)
		}
		if _, duplicate := seen[proof.DataClass]; duplicate {
			return fmt.Errorf("duplicate proof for data class %q", proof.DataClass)
		}
		seen[proof.DataClass] = struct{}{}
		if proof.FixtureID != fixtureID {
			return fmt.Errorf("proof for data class %q has fixture %q, want %q", proof.DataClass, proof.FixtureID, fixtureID)
		}
		if stage.Destination != "" && proof.Destination != string(stage.Destination) {
			return fmt.Errorf("proof for data class %q has destination %q, want %q", proof.DataClass, proof.Destination, stage.Destination)
		}
		if proof.RetentionDays < 0 || proof.MeasuredDurationSeconds < 0 || proof.MeasuredRPOSeconds < 0 || proof.MeasuredRTOSeconds < 0 {
			return fmt.Errorf("proof for data class %q contains a negative measurement", proof.DataClass)
		}
		if stage.RequiresSingleWriter || stage.RequiresSplitBrainCheck || stage.RequiresStaleOriginCheck || stage.RequiresReconciliation || stage.RequiresApproval {
			if strings.TrimSpace(proof.WriterIdentityRef) == "" || strings.TrimSpace(proof.WriterEpoch) == "" {
				return fmt.Errorf("proof for data class %q requires an opaque writer identity and epoch", proof.DataClass)
			}
			if stage.RequiresSingleWriter && !proof.SingleWriterVerified {
				return fmt.Errorf("proof for data class %q did not verify single-writer ownership", proof.DataClass)
			}
			if stage.RequiresSplitBrainCheck && !proof.SplitBrainAbsent {
				return fmt.Errorf("proof for data class %q did not verify split-brain absence", proof.DataClass)
			}
			if stage.RequiresStaleOriginCheck && !proof.StaleOriginRejected {
				return fmt.Errorf("proof for data class %q did not reject the stale origin", proof.DataClass)
			}
			if stage.RequiresReconciliation && !proof.ReconciliationVerified {
				return fmt.Errorf("proof for data class %q did not verify reconciliation", proof.DataClass)
			}
			if stage.RequiresApproval && !proof.ApprovalVerified {
				return fmt.Errorf("proof for data class %q did not verify the approved transition", proof.DataClass)
			}
		}
		switch stage.Action {
		case sdk.ResilienceBackup:
			if strings.TrimSpace(proof.BackupID) == "" {
				return fmt.Errorf("backup proof for data class %q is missing a backup identity", proof.DataClass)
			}
			if proof.RetentionDays <= 0 || !proof.EncryptionVerified || !proof.ProtectionVerified {
				return fmt.Errorf("backup proof for data class %q requires positive retention, encryption, and protection verification", proof.DataClass)
			}
		case sdk.ResilienceRestore:
			if strings.TrimSpace(proof.RestoreID) == "" || !proof.ServiceHealthVerified {
				return fmt.Errorf("restore proof for data class %q requires a restore identity and service-health verification", proof.DataClass)
			}
		case sdk.ResilienceIntegrityCheck:
			if !proof.ManifestVerified && !proof.CountsVerified && !proof.ApplicationReadsVerified {
				return fmt.Errorf("integrity proof for data class %q must verify a manifest, count, or application read", proof.DataClass)
			}
			if !proof.PermissionsVerified || !proof.ServiceHealthVerified {
				return fmt.Errorf("integrity proof for data class %q requires permissions and service-health verification", proof.DataClass)
			}
			if strings.Contains(proof.DataClass, "secret") && !proof.SecretReferencesVerified {
				return fmt.Errorf("integrity proof for data class %q requires secret-reference verification", proof.DataClass)
			}
		case sdk.ResilienceFailover:
			if !proof.ServiceHealthVerified {
				return fmt.Errorf("failover proof for data class %q requires service-health verification", proof.DataClass)
			}
		}
	}
	for dataClass := range expected {
		if _, ok := seen[dataClass]; !ok {
			return fmt.Errorf("missing proof for data class %q", dataClass)
		}
	}
	return nil
}

func validateRecoveryCheckpoint(checkpoint RecoveryCheckpoint) error {
	if err := ValidateSecretSafeValue(checkpoint); err != nil {
		return errors.New("recovery checkpoint contains secret-like material")
	}
	if checkpoint.PlanDigest != "" && !recoveryDigestPattern.MatchString(checkpoint.PlanDigest) {
		return errors.New("checkpoint plan digest must be a lowercase SHA-256 digest")
	}
	if strings.TrimSpace(checkpoint.OwnershipMarker) == "" || strings.ContainsAny(checkpoint.OwnershipMarker, "\r\n\x00") {
		return errors.New("checkpoint ownership marker must be non-empty and single-line")
	}
	if strings.TrimSpace(checkpoint.FixtureID) == "" || strings.ContainsAny(checkpoint.FixtureID, "\r\n\x00") {
		return errors.New("checkpoint fixture ID must be non-empty and single-line")
	}
	for stageID, state := range checkpoint.Stages {
		if strings.TrimSpace(stageID) == "" {
			return errors.New("checkpoint stage ID must be non-empty")
		}
		if state.Status != StatusPass || !state.OwnershipVerified || !state.IdempotencyVerified {
			return fmt.Errorf("checkpoint stage %q is not a verified PASS state", stageID)
		}
		if state.Action == "" || strings.TrimSpace(state.CompletedAt) == "" {
			return fmt.Errorf("checkpoint stage %q is missing action or completion time", stageID)
		}
		for _, value := range append(append(append([]string{}, state.OperationID), state.ResourceRefs...), state.ProofRefs...) {
			if strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("checkpoint stage %q contains a multi-line identity", stageID)
			}
		}
	}
	return nil
}

func resiliencePlanDigest(plan sdk.ResiliencePlan) (string, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return "", fmt.Errorf("encode resilience plan digest: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func recoveryIdempotencyKey(planDigest, ownershipMarker, stageID string) string {
	sum := sha256.Sum256([]byte(planDigest + "\x00" + ownershipMarker + "\x00" + stageID))
	return hex.EncodeToString(sum[:])
}

func topologicalRecoveryStages(stages []sdk.ResilienceStage) ([]sdk.ResilienceStage, error) {
	known := make(map[string]sdk.ResilienceStage, len(stages))
	for _, stage := range stages {
		if _, exists := known[stage.ID]; exists {
			return nil, fmt.Errorf("duplicate resilience stage %q", stage.ID)
		}
		known[stage.ID] = stage
	}
	indegree := make(map[string]int, len(stages))
	dependents := make(map[string][]string, len(stages))
	for _, stage := range stages {
		indegree[stage.ID] = len(stage.DependsOn)
		for _, dependency := range stage.DependsOn {
			if _, exists := known[dependency]; !exists {
				return nil, fmt.Errorf("resilience stage %q depends on unknown stage %q", stage.ID, dependency)
			}
			dependents[dependency] = append(dependents[dependency], stage.ID)
		}
	}
	ready := make([]string, 0)
	for id, degree := range indegree {
		if degree == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	ordered := make([]sdk.ResilienceStage, 0, len(stages))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		ordered = append(ordered, known[id])
		children := append([]string(nil), dependents[id]...)
		sort.Strings(children)
		for _, child := range children {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
			}
		}
		sort.Strings(ready)
	}
	if len(ordered) != len(stages) {
		return nil, errors.New("resilience stage dependency graph contains a cycle")
	}
	return ordered, nil
}
