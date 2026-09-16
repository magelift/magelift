package sdk

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ResilienceAction is a provider-neutral operation in a recovery plan. A
// provider adapter translates an action into its native backup, restore,
// failover, or fencing API; the core never assumes that those APIs are
// interchangeable.
type ResilienceAction string

const (
	ResilienceBackup         ResilienceAction = "backup"
	ResilienceRestore        ResilienceAction = "restore"
	ResilienceIntegrityCheck ResilienceAction = "integrity-check"
	ResilienceFailover       ResilienceAction = "failover"
	ResilienceFailback       ResilienceAction = "failback"
	ResilienceFence          ResilienceAction = "fence"
	ResilienceCleanup        ResilienceAction = "cleanup"
)

// ResilienceStage is one idempotent unit in a recovery plan. Dependencies are
// explicit so a scheduler can resume after interruption without guessing
// whether a restore or a fencing action already happened.
type ResilienceStage struct {
	ID               string              `json:"id" yaml:"id"`
	Action           ResilienceAction    `json:"action" yaml:"action"`
	DataClasses      []string            `json:"dataClasses,omitempty" yaml:"dataClasses,omitempty"`
	Destination      RecoveryDestination `json:"destination,omitempty" yaml:"destination,omitempty"`
	DependsOn        []string            `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`
	Idempotent       bool                `json:"idempotent" yaml:"idempotent"`
	RequiresApproval bool                `json:"requiresApproval,omitempty" yaml:"requiresApproval,omitempty"`
	// These references are opaque admission identities captured when a plan
	// explicitly requests a stage transition. Execution must present the same
	// identities; they are never interpreted or logged by the portable core.
	ApprovalReference       string `json:"approvalReference,omitempty" yaml:"approvalReference,omitempty"`
	ReconciliationReference string `json:"reconciliationReference,omitempty" yaml:"reconciliationReference,omitempty"`
	// These flags make the safety contract executable instead of leaving
	// fencing and stale-origin protection as prose in a runbook. Provider
	// adapters must return matching proof fields before the stage is accepted.
	RequiresSingleWriter     bool `json:"requiresSingleWriter,omitempty" yaml:"requiresSingleWriter,omitempty"`
	RequiresSplitBrainCheck  bool `json:"requiresSplitBrainCheck,omitempty" yaml:"requiresSplitBrainCheck,omitempty"`
	RequiresStaleOriginCheck bool `json:"requiresStaleOriginCheck,omitempty" yaml:"requiresStaleOriginCheck,omitempty"`
	RequiresReconciliation   bool `json:"requiresReconciliation,omitempty" yaml:"requiresReconciliation,omitempty"`
}

// ResilienceCapabilityStatus is the adapter's semantic availability result.
// It is deliberately independent from any provider product lifecycle enum so
// the core can render unsupported and unexercised boundaries consistently.
type ResilienceCapabilityStatus string

const (
	ResilienceCapabilitySupported    ResilienceCapabilityStatus = "supported"
	ResilienceCapabilityCertified    ResilienceCapabilityStatus = "certified"
	ResilienceCapabilityExperimental ResilienceCapabilityStatus = "experimental"
	ResilienceCapabilityUnavailable  ResilienceCapabilityStatus = "unavailable"
	ResilienceCapabilityUnsupported  ResilienceCapabilityStatus = "unsupported"
	ResilienceCapabilityBlocked      ResilienceCapabilityStatus = "blocked"
)

// ResilienceDataClassCapability is provider-owned capability metadata. It
// describes the semantic operations an adapter can execute without exposing
// AWS, GCP, Scaleway, OVHcloud, or community SDK types in the core contract.
type ResilienceDataClassCapability struct {
	Name                string                     `json:"name" yaml:"name"`
	Status              ResilienceCapabilityStatus `json:"status" yaml:"status"`
	Actions             []ResilienceAction         `json:"actions,omitempty" yaml:"actions,omitempty"`
	Destinations        []RecoveryDestination      `json:"destinations,omitempty" yaml:"destinations,omitempty"`
	Strategy            string                     `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	BackupMechanism     string                     `json:"backupMechanism,omitempty" yaml:"backupMechanism,omitempty"`
	RestoreMechanism    string                     `json:"restoreMechanism,omitempty" yaml:"restoreMechanism,omitempty"`
	IntegrityMethod     string                     `json:"integrityMethod,omitempty" yaml:"integrityMethod,omitempty"`
	RetentionPolicy     string                     `json:"retentionPolicy,omitempty" yaml:"retentionPolicy,omitempty"`
	EncryptionBoundary  string                     `json:"encryptionBoundary,omitempty" yaml:"encryptionBoundary,omitempty"`
	ProtectionMechanism string                     `json:"protectionMechanism,omitempty" yaml:"protectionMechanism,omitempty"`
	PollingRequired     bool                       `json:"pollingRequired" yaml:"pollingRequired"`
	RetentionRequired   bool                       `json:"retentionRequired" yaml:"retentionRequired"`
	EncryptionRequired  bool                       `json:"encryptionRequired" yaml:"encryptionRequired"`
	ProtectionRequired  bool                       `json:"protectionRequired" yaml:"protectionRequired"`
	Reason              string                     `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// ResilienceCapabilityError is returned when a selected operation is not
// executable for the adapter's provider, region, or service boundary. A
// typed error keeps unavailable paths visible to callers and certification
// reports instead of allowing a provider adapter to silently no-op.
type ResilienceCapabilityError struct {
	AdapterID string                     `json:"adapterId" yaml:"adapterId"`
	Operation ResilienceAction           `json:"operation" yaml:"operation"`
	DataClass string                     `json:"dataClass,omitempty" yaml:"dataClass,omitempty"`
	Status    ResilienceCapabilityStatus `json:"status" yaml:"status"`
	Reason    string                     `json:"reason" yaml:"reason"`
}

// ResilienceAdmissionError means that an execution request does not match the
// identities captured by its compiled recovery plan. It intentionally omits
// the compared values because approval and reconciliation references are
// opaque workflow identities and must remain safe to surface in diagnostics.
type ResilienceAdmissionError struct {
	StageID string           `json:"stageId" yaml:"stageId"`
	Action  ResilienceAction `json:"action" yaml:"action"`
	Field   string           `json:"field" yaml:"field"`
	Reason  string           `json:"reason" yaml:"reason"`
}

func (e ResilienceAdmissionError) Error() string {
	parts := []string{"resilience admission denied"}
	if strings.TrimSpace(e.StageID) != "" {
		parts = append(parts, "stage="+e.StageID)
	}
	if e.Action != "" {
		parts = append(parts, "action="+string(e.Action))
	}
	if strings.TrimSpace(e.Field) != "" {
		parts = append(parts, "field="+e.Field)
	}
	if strings.TrimSpace(e.Reason) != "" {
		parts = append(parts, e.Reason)
	}
	return strings.Join(parts, ": ")
}

func (e ResilienceCapabilityError) Error() string {
	parts := []string{"resilience capability " + string(e.Status)}
	if strings.TrimSpace(e.AdapterID) != "" {
		parts = append(parts, "adapter="+e.AdapterID)
	}
	if e.Operation != "" {
		parts = append(parts, "operation="+string(e.Operation))
	}
	if strings.TrimSpace(e.DataClass) != "" {
		parts = append(parts, "dataClass="+e.DataClass)
	}
	if strings.TrimSpace(e.Reason) != "" {
		parts = append(parts, e.Reason)
	}
	return strings.Join(parts, ": ")
}

// ResilienceAdapterDescriptor identifies a provider implementation without
// importing a provider SDK. Capabilities are semantic action IDs, not native
// product names.
type ResilienceAdapterDescriptor struct {
	APIVersion   string                          `json:"apiVersion" yaml:"apiVersion"`
	ID           string                          `json:"id" yaml:"id"`
	Provider     ProviderID                      `json:"provider" yaml:"provider"`
	Version      string                          `json:"version" yaml:"version"`
	Capabilities []ResilienceAction              `json:"capabilities" yaml:"capabilities"`
	DataClasses  []ResilienceDataClassCapability `json:"dataClasses,omitempty" yaml:"dataClasses,omitempty"`
}

// ResiliencePlanRequest is side-effect free. Adapters may use the request to
// select a native strategy, but must not provision anything while planning.
type ResiliencePlanRequest struct {
	Architecture       ArchitectureIntent `json:"architecture" yaml:"architecture"`
	FixtureID          string             `json:"fixtureId" yaml:"fixtureId"`
	OwnershipMarker    string             `json:"ownershipMarker" yaml:"ownershipMarker"`
	ApprovalReferences map[string]string  `json:"approvalReferences,omitempty" yaml:"approvalReferences,omitempty"`
	// ReconciliationReferences are opaque operator or workflow identities. A
	// reference is required for an explicitly requested failback so a restored
	// primary cannot become writable merely because it is reachable again.
	ReconciliationReferences map[string]string `json:"reconciliationReferences,omitempty" yaml:"reconciliationReferences,omitempty"`
}

// ResiliencePlan is the portable recovery graph returned by an adapter. The
// fixture and ownership bindings prevent a stage from being executed against
// a different recovery scope than the one that was planned.
type ResiliencePlan struct {
	AdapterID       string            `json:"adapterId" yaml:"adapterId"`
	FixtureID       string            `json:"fixtureId" yaml:"fixtureId"`
	OwnershipMarker string            `json:"ownershipMarker" yaml:"ownershipMarker"`
	Stages          []ResilienceStage `json:"stages" yaml:"stages"`
	RequiredProofs  []string          `json:"requiredProofs" yaml:"requiredProofs"`
}

// ResilienceExecutionRequest identifies one planned stage. IdempotencyKey is
// mandatory because cloud APIs can report an ambiguous timeout after creating
// a backup or initiating failover.
type ResilienceExecutionRequest struct {
	Plan                    ResiliencePlan    `json:"plan" yaml:"plan"`
	StageID                 string            `json:"stageId" yaml:"stageId"`
	FixtureID               string            `json:"fixtureId,omitempty" yaml:"fixtureId,omitempty"`
	IdempotencyKey          string            `json:"idempotencyKey" yaml:"idempotencyKey"`
	OwnershipMarker         string            `json:"ownershipMarker" yaml:"ownershipMarker"`
	ResourceReferences      map[string]string `json:"resourceReferences,omitempty" yaml:"resourceReferences,omitempty"`
	BackupReferences        map[string]string `json:"backupReferences,omitempty" yaml:"backupReferences,omitempty"`
	ApprovalReference       string            `json:"approvalReference,omitempty" yaml:"approvalReference,omitempty"`
	ReconciliationReference string            `json:"reconciliationReference,omitempty" yaml:"reconciliationReference,omitempty"`
}

type ResilienceExecutionResult struct {
	Action              ResilienceAction          `json:"action" yaml:"action"`
	OperationID         string                    `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	ResourceRefs        []string                  `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	ProofRefs           []string                  `json:"proofRefs,omitempty" yaml:"proofRefs,omitempty"`
	Evidence            []ResilienceProofEvidence `json:"evidence,omitempty" yaml:"evidence,omitempty"`
	OwnershipMarker     string                    `json:"ownershipMarker" yaml:"ownershipMarker"`
	OwnershipVerified   bool                      `json:"ownershipVerified" yaml:"ownershipVerified"`
	IdempotencyVerified bool                      `json:"idempotencyVerified" yaml:"idempotencyVerified"`
}

// ResilienceAdapter is the optional state-recovery surface of a deployable
// extension. Planning is separate from execution so certification can inspect
// a complete strategy before cloud credentials or mutable clients are used.
type ResilienceAdapter interface {
	ResilienceDescriptor() ResilienceAdapterDescriptor
	PlanResilience(context.Context, ResiliencePlanRequest) (ResiliencePlan, error)
	ExecuteResilience(context.Context, ResilienceExecutionRequest) (ResilienceExecutionResult, error)
}

func ValidateResilienceAdapterDescriptor(descriptor ResilienceAdapterDescriptor) error {
	var problems []error
	if descriptor.APIVersion != ExtensionAPIVersion {
		problems = append(problems, fmt.Errorf("resilience adapter API version %q is not supported", descriptor.APIVersion))
	}
	problems = append(problems,
		validateID("resilience adapter ID", descriptor.ID),
		validateID("resilience adapter provider ID", string(descriptor.Provider)),
		validateExtensionVersion(descriptor.Version),
	)
	if len(descriptor.Capabilities) == 0 {
		problems = append(problems, errors.New("resilience adapter must declare at least one capability"))
	}
	seen := make(map[ResilienceAction]struct{}, len(descriptor.Capabilities))
	for _, capability := range descriptor.Capabilities {
		if !validResilienceAction(capability) {
			problems = append(problems, fmt.Errorf("invalid resilience adapter capability %q", capability))
		}
		if _, exists := seen[capability]; exists {
			problems = append(problems, fmt.Errorf("duplicate resilience adapter capability %q", capability))
		}
		seen[capability] = struct{}{}
	}
	seenDataClasses := make(map[string]struct{}, len(descriptor.DataClasses))
	for _, capability := range descriptor.DataClasses {
		problems = append(problems, validateID("resilience adapter data-class capability name", capability.Name))
		switch capability.Status {
		case ResilienceCapabilitySupported, ResilienceCapabilityCertified, ResilienceCapabilityExperimental:
			if len(capability.Actions) == 0 {
				problems = append(problems, fmt.Errorf("resilience adapter data-class capability %q must declare at least one action for status %q", capability.Name, capability.Status))
			}
		case ResilienceCapabilityUnavailable, ResilienceCapabilityUnsupported, ResilienceCapabilityBlocked:
			if strings.TrimSpace(capability.Reason) == "" {
				problems = append(problems, fmt.Errorf("resilience adapter data-class capability %q requires a reason for status %q", capability.Name, capability.Status))
			}
			if len(capability.Actions) != 0 || len(capability.Destinations) != 0 {
				problems = append(problems, fmt.Errorf("resilience adapter data-class capability %q must not declare executable operations for status %q", capability.Name, capability.Status))
			}
		default:
			problems = append(problems, fmt.Errorf("invalid resilience adapter data-class status %q", capability.Status))
		}
		if _, exists := seenDataClasses[capability.Name]; exists {
			problems = append(problems, fmt.Errorf("duplicate resilience adapter data-class capability %q", capability.Name))
		}
		seenDataClasses[capability.Name] = struct{}{}
		if len(capability.Actions) > 0 && !capability.PollingRequired {
			problems = append(problems, fmt.Errorf("resilience adapter data-class capability %q must require polling for executable operations", capability.Name))
		}
		if capability.RetentionRequired && strings.TrimSpace(capability.RetentionPolicy) == "" {
			problems = append(problems, fmt.Errorf("resilience adapter data-class capability %q requires a retention policy", capability.Name))
		}
		if capability.EncryptionRequired && strings.TrimSpace(capability.EncryptionBoundary) == "" {
			problems = append(problems, fmt.Errorf("resilience adapter data-class capability %q requires an encryption boundary", capability.Name))
		}
		if capability.ProtectionRequired && strings.TrimSpace(capability.ProtectionMechanism) == "" {
			problems = append(problems, fmt.Errorf("resilience adapter data-class capability %q requires a protection mechanism", capability.Name))
		}
		seenActions := make(map[ResilienceAction]struct{}, len(capability.Actions))
		for _, action := range capability.Actions {
			if !validResilienceAction(action) {
				problems = append(problems, fmt.Errorf("invalid resilience adapter data-class action %q", action))
			}
			if _, exists := seenActions[action]; exists {
				problems = append(problems, fmt.Errorf("duplicate resilience adapter data-class action %q", action))
			}
			seenActions[action] = struct{}{}
		}
		if _, restores := seenActions[ResilienceRestore]; restores && len(capability.Destinations) == 0 {
			problems = append(problems, fmt.Errorf("resilience adapter data-class capability %q must declare restore destinations", capability.Name))
		}
		seenDestinations := make(map[RecoveryDestination]struct{}, len(capability.Destinations))
		for _, destination := range capability.Destinations {
			if !validRecoveryDestination(destination) {
				problems = append(problems, fmt.Errorf("invalid resilience adapter data-class destination %q", destination))
			}
			if _, exists := seenDestinations[destination]; exists {
				problems = append(problems, fmt.Errorf("duplicate resilience adapter data-class destination %q", destination))
			}
			seenDestinations[destination] = struct{}{}
		}
	}
	return errors.Join(problems...)
}

func ValidateResiliencePlanRequest(request ResiliencePlanRequest) error {
	var problems []error
	problems = append(problems, request.Architecture.Validate())
	if strings.TrimSpace(request.FixtureID) == "" {
		problems = append(problems, errors.New("resilience fixture ID is required"))
	}
	if strings.TrimSpace(request.OwnershipMarker) == "" {
		problems = append(problems, errors.New("resilience ownership marker is required"))
	}
	if request.OwnershipMarker != request.Architecture.OwnershipMarker {
		problems = append(problems, errors.New("resilience ownership marker must match architecture ownership marker"))
	}
	for stageID, reference := range request.ApprovalReferences {
		if err := validateID("resilience approval stage ID", stageID); err != nil {
			problems = append(problems, err)
		}
		if strings.TrimSpace(reference) == "" || strings.ContainsAny(reference, "\r\n\x00") {
			problems = append(problems, fmt.Errorf("resilience approval reference for stage %q must be non-empty and single-line", stageID))
		}
	}
	for stageID, reference := range request.ReconciliationReferences {
		if err := validateID("resilience reconciliation stage ID", stageID); err != nil {
			problems = append(problems, err)
		}
		if strings.TrimSpace(reference) == "" || strings.ContainsAny(reference, "\r\n\x00") {
			problems = append(problems, fmt.Errorf("resilience reconciliation reference for stage %q must be non-empty and single-line", stageID))
		}
	}
	return errors.Join(problems...)
}

func ValidateResiliencePlan(plan ResiliencePlan, resilience ResilienceIntent) error {
	var problems []error
	if strings.TrimSpace(plan.AdapterID) == "" {
		problems = append(problems, errors.New("resilience plan adapter ID is required"))
	}
	if strings.TrimSpace(plan.FixtureID) == "" || strings.ContainsAny(plan.FixtureID, "\r\n\x00") {
		problems = append(problems, errors.New("resilience plan fixture ID is required and must be single-line"))
	}
	if strings.TrimSpace(plan.OwnershipMarker) == "" || strings.ContainsAny(plan.OwnershipMarker, "\r\n\x00") {
		problems = append(problems, errors.New("resilience plan ownership marker is required and must be single-line"))
	}
	if len(plan.Stages) == 0 {
		problems = append(problems, errors.New("resilience plan must contain at least one stage"))
	}
	knownClasses := make(map[string]struct{}, len(resilience.DataClasses))
	for _, dataClass := range resilience.DataClasses {
		knownClasses[dataClass.Name] = struct{}{}
	}
	knownStages := make(map[string]struct{}, len(plan.Stages))
	for _, stage := range plan.Stages {
		if err := validateID("resilience stage ID", stage.ID); err != nil {
			problems = append(problems, err)
		}
		if !validResilienceAction(stage.Action) {
			problems = append(problems, fmt.Errorf("resilience stage %q has invalid action %q", stage.ID, stage.Action))
		}
		if !stage.Idempotent {
			problems = append(problems, fmt.Errorf("resilience stage %q must be idempotent for interruption-safe resume", stage.ID))
		}
		if len(stage.DataClasses) == 0 {
			problems = append(problems, fmt.Errorf("resilience stage %q must name at least one data class", stage.ID))
		}
		seenClasses := make(map[string]struct{}, len(stage.DataClasses))
		for _, dataClass := range stage.DataClasses {
			if _, ok := knownClasses[dataClass]; !ok {
				problems = append(problems, fmt.Errorf("resilience stage %q references unknown data class %q", stage.ID, dataClass))
			}
			if _, exists := seenClasses[dataClass]; exists {
				problems = append(problems, fmt.Errorf("resilience stage %q contains duplicate data class %q", stage.ID, dataClass))
			}
			seenClasses[dataClass] = struct{}{}
		}
		if stage.Destination != "" && !validRecoveryDestination(stage.Destination) {
			problems = append(problems, fmt.Errorf("resilience stage %q has invalid destination %q", stage.ID, stage.Destination))
		}
		if stage.ApprovalReference != "" && (strings.TrimSpace(stage.ApprovalReference) == "" || strings.ContainsAny(stage.ApprovalReference, "\r\n\x00")) {
			problems = append(problems, fmt.Errorf("resilience stage %q approval reference must be non-empty and single-line", stage.ID))
		}
		if stage.ReconciliationReference != "" && (strings.TrimSpace(stage.ReconciliationReference) == "" || strings.ContainsAny(stage.ReconciliationReference, "\r\n\x00")) {
			problems = append(problems, fmt.Errorf("resilience stage %q reconciliation reference must be non-empty and single-line", stage.ID))
		}
		if stage.ApprovalReference != "" && !stage.RequiresApproval {
			problems = append(problems, fmt.Errorf("resilience stage %q cannot bind an approval reference without requiring approval", stage.ID))
		}
		if stage.ReconciliationReference != "" && !stage.RequiresReconciliation {
			problems = append(problems, fmt.Errorf("resilience stage %q cannot bind a reconciliation reference without requiring reconciliation", stage.ID))
		}
		if stage.RequiresSingleWriter && stage.Action != ResilienceFence && stage.Action != ResilienceFailover && stage.Action != ResilienceFailback {
			problems = append(problems, fmt.Errorf("resilience stage %q cannot require single-writer protection for action %q", stage.ID, stage.Action))
		}
		if stage.RequiresSplitBrainCheck && stage.Action != ResilienceFence && stage.Action != ResilienceFailover && stage.Action != ResilienceFailback {
			problems = append(problems, fmt.Errorf("resilience stage %q cannot require split-brain verification for action %q", stage.ID, stage.Action))
		}
		if stage.RequiresStaleOriginCheck && stage.Action != ResilienceFailover && stage.Action != ResilienceFailback {
			problems = append(problems, fmt.Errorf("resilience stage %q cannot require stale-origin verification for action %q", stage.ID, stage.Action))
		}
		if stage.RequiresReconciliation && stage.Action != ResilienceFailback {
			problems = append(problems, fmt.Errorf("resilience stage %q cannot require reconciliation for action %q", stage.ID, stage.Action))
		}
		if stage.Action == ResilienceFailback && (!stage.RequiresApproval || !stage.RequiresSingleWriter || !stage.RequiresSplitBrainCheck || !stage.RequiresStaleOriginCheck || !stage.RequiresReconciliation || stage.ApprovalReference == "" || stage.ReconciliationReference == "") {
			problems = append(problems, fmt.Errorf("resilience failback stage %q must require approval, single-writer, split-brain, stale-origin, and reconciliation checks with bound admission references", stage.ID))
		}
		if _, exists := knownStages[stage.ID]; exists {
			problems = append(problems, fmt.Errorf("duplicate resilience stage %q", stage.ID))
		}
		knownStages[stage.ID] = struct{}{}
	}
	for _, stage := range plan.Stages {
		seenDependencies := make(map[string]struct{}, len(stage.DependsOn))
		for _, dependency := range stage.DependsOn {
			if _, ok := knownStages[dependency]; !ok {
				problems = append(problems, fmt.Errorf("resilience stage %q depends on unknown stage %q", stage.ID, dependency))
			}
			if dependency == stage.ID {
				problems = append(problems, fmt.Errorf("resilience stage %q cannot depend on itself", stage.ID))
			}
			if _, exists := seenDependencies[dependency]; exists {
				problems = append(problems, fmt.Errorf("resilience stage %q contains duplicate dependency %q", stage.ID, dependency))
			}
			seenDependencies[dependency] = struct{}{}
		}
	}
	problems = append(problems, resiliencePlanCycles(plan.Stages))
	if len(plan.RequiredProofs) == 0 {
		problems = append(problems, errors.New("resilience plan must declare required proofs"))
	}
	return errors.Join(problems...)
}

func ValidateResilienceExecutionRequest(request ResilienceExecutionRequest) error {
	if strings.TrimSpace(request.StageID) == "" {
		return errors.New("resilience execution stage ID is required")
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" {
		return errors.New("resilience execution idempotency key is required")
	}
	if strings.TrimSpace(request.FixtureID) == "" {
		return errors.New("resilience execution fixture ID is required")
	}
	if strings.TrimSpace(request.OwnershipMarker) == "" {
		return errors.New("resilience execution ownership marker is required")
	}
	if strings.ContainsAny(request.FixtureID, "\r\n\x00") {
		return errors.New("resilience execution fixture ID must be single-line")
	}
	if strings.ContainsAny(request.OwnershipMarker, "\r\n\x00") {
		return errors.New("resilience execution ownership marker must be single-line")
	}
	if strings.ContainsAny(request.ApprovalReference+request.ReconciliationReference, "\r\n\x00") {
		return errors.New("resilience execution approval and reconciliation references must be single-line")
	}
	for dataClass, reference := range request.ResourceReferences {
		if err := validateID("resilience resource reference data class", dataClass); err != nil {
			return err
		}
		if strings.TrimSpace(reference) == "" || strings.ContainsAny(reference, "\r\n\x00") {
			return fmt.Errorf("resilience resource reference for data class %q must be non-empty and single-line", dataClass)
		}
	}
	for dataClass, reference := range request.BackupReferences {
		if err := validateID("resilience backup reference data class", dataClass); err != nil {
			return err
		}
		if strings.TrimSpace(reference) == "" || strings.ContainsAny(reference, "\r\n\x00") {
			return fmt.Errorf("resilience backup reference for data class %q must be non-empty and single-line", dataClass)
		}
	}
	stage, found := findResilienceStage(request.Plan, request.StageID)
	if !found {
		return fmt.Errorf("resilience execution references unknown stage %q", request.StageID)
	}
	if strings.TrimSpace(request.Plan.FixtureID) == "" || strings.ContainsAny(request.Plan.FixtureID, "\r\n\x00") {
		return ResilienceAdmissionError{StageID: stage.ID, Action: stage.Action, Field: "fixture", Reason: "compiled plan fixture identity is missing or unsafe"}
	}
	if request.FixtureID != request.Plan.FixtureID {
		return ResilienceAdmissionError{StageID: stage.ID, Action: stage.Action, Field: "fixture", Reason: "fixture identity does not match the compiled plan"}
	}
	if strings.TrimSpace(request.Plan.OwnershipMarker) == "" || strings.ContainsAny(request.Plan.OwnershipMarker, "\r\n\x00") {
		return ResilienceAdmissionError{StageID: stage.ID, Action: stage.Action, Field: "ownership", Reason: "compiled plan ownership marker is missing or unsafe"}
	}
	if request.OwnershipMarker != request.Plan.OwnershipMarker {
		return ResilienceAdmissionError{StageID: stage.ID, Action: stage.Action, Field: "ownership", Reason: "ownership marker does not match the compiled plan"}
	}
	if stage.RequiresApproval && strings.TrimSpace(request.ApprovalReference) == "" {
		return fmt.Errorf("resilience stage %q requires an approval reference", stage.ID)
	}
	if stage.ApprovalReference != "" && request.ApprovalReference != stage.ApprovalReference {
		return ResilienceAdmissionError{StageID: stage.ID, Action: stage.Action, Field: "approval", Reason: "approval reference does not match the compiled plan"}
	}
	if stage.RequiresReconciliation && strings.TrimSpace(request.ReconciliationReference) == "" {
		return fmt.Errorf("resilience stage %q requires a reconciliation reference", stage.ID)
	}
	if stage.ReconciliationReference != "" && request.ReconciliationReference != stage.ReconciliationReference {
		return ResilienceAdmissionError{StageID: stage.ID, Action: stage.Action, Field: "reconciliation", Reason: "reconciliation reference does not match the compiled plan"}
	}
	return nil
}

func validResilienceAction(action ResilienceAction) bool {
	switch action {
	case ResilienceBackup, ResilienceRestore, ResilienceIntegrityCheck, ResilienceFailover, ResilienceFailback, ResilienceFence, ResilienceCleanup:
		return true
	default:
		return false
	}
}

func validRecoveryDestination(destination RecoveryDestination) bool {
	switch destination {
	case RecoverySameRegion, RecoverySameRegionIsolated, RecoveryAlternateRegion, RecoveryAlternateProvider:
		return true
	default:
		return false
	}
}

func resiliencePlanCycles(stages []ResilienceStage) error {
	known := make(map[string]ResilienceStage, len(stages))
	for _, stage := range stages {
		known[stage.ID] = stage
	}
	visiting := make(map[string]bool, len(stages))
	visited := make(map[string]bool, len(stages))
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("resilience stage dependency cycle at %q", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range known[id].DependsOn {
			if _, exists := known[dependency]; exists {
				if err := visit(dependency); err != nil {
					return err
				}
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	ids := make([]string, 0, len(stages))
	for _, stage := range stages {
		ids = append(ids, stage.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
