package certification

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// ScheduleCell is the scheduler's provider-neutral input. A provider adapter
// supplies mutation keys such as account/region/network or an external edge
// service; the scheduler never needs to know the native API behind a key.
type ScheduleCell struct {
	ID               string                     `json:"id" yaml:"id"`
	Fingerprint      string                     `json:"fingerprint" yaml:"fingerprint"`
	ReuseFingerprint *ReuseFingerprint          `json:"reuseFingerprint,omitempty" yaml:"reuseFingerprint,omitempty"`
	WarmBoundary     string                     `json:"warmBoundary" yaml:"warmBoundary"`
	WarmFrom         string                     `json:"warmFrom,omitempty" yaml:"warmFrom,omitempty"`
	WarmTransition   sdk.CoverageTransitionKind `json:"warmTransition,omitempty" yaml:"warmTransition,omitempty"`
	Coverage         *sdk.CoverageBoundary      `json:"coverage,omitempty" yaml:"coverage,omitempty"`
	Dependencies     []string                   `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	MutationKeys     []string                   `json:"mutationKeys" yaml:"mutationKeys"`
	OwnershipMarker  string                     `json:"ownershipMarker" yaml:"ownershipMarker"`
	Estimate         ExecutionEstimate          `json:"estimate" yaml:"estimate"`
	Status           CapabilityStatus           `json:"status" yaml:"status"`
	Required         bool                       `json:"required" yaml:"required"`
}

// ExecutionEstimate is a provider-neutral planning estimate. Provider
// adapters may populate it from measured historical runs or a current
// pricing lookup; the scheduler only validates and aggregates the values.
// A zero budget means that dimension is not bounded. Unknown cost is
// represented by CostKnown=false, never by a zero spend estimate.
type ExecutionEstimate struct {
	CloudOperationSeconds   int64 `json:"cloudOperationSeconds" yaml:"cloudOperationSeconds"`
	ResourceLifetimeSeconds int64 `json:"resourceLifetimeSeconds" yaml:"resourceLifetimeSeconds"`
	EstimatedCostCents      int64 `json:"estimatedCostCents" yaml:"estimatedCostCents"`
	CleanupLatencySeconds   int64 `json:"cleanupLatencySeconds" yaml:"cleanupLatencySeconds"`
	CostKnown               bool  `json:"costKnown" yaml:"costKnown"`
}

// SchedulerCheckpoint is the durable, provider-neutral subset of an
// acceptance checkpoint. Provider operation IDs and backup IDs remain opaque
// strings but are retained so an interrupted run resumes the same scope.
type SchedulerCheckpoint struct {
	CellID           string            `json:"cellId" yaml:"cellId"`
	Fingerprint      string            `json:"fingerprint" yaml:"fingerprint"`
	ReuseFingerprint *ReuseFingerprint `json:"reuseFingerprint,omitempty" yaml:"reuseFingerprint,omitempty"`
	OwnershipMarker  string            `json:"ownershipMarker" yaml:"ownershipMarker"`
	Status           string            `json:"status" yaml:"status"`
	StackID          string            `json:"stackId,omitempty" yaml:"stackId,omitempty"`
	OperationIDs     []string          `json:"operationIds,omitempty" yaml:"operationIds,omitempty"`
	ResourceRefs     []string          `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	BackupRefs       []string          `json:"backupRefs,omitempty" yaml:"backupRefs,omitempty"`
	RestoreRefs      []string          `json:"restoreRefs,omitempty" yaml:"restoreRefs,omitempty"`
	TelemetryRefs    []string          `json:"telemetryRefs,omitempty" yaml:"telemetryRefs,omitempty"`
	EdgeRefs         []string          `json:"edgeRefs,omitempty" yaml:"edgeRefs,omitempty"`
	ReuseMode        string            `json:"reuseMode,omitempty" yaml:"reuseMode,omitempty"`
	CleanupState     string            `json:"cleanupState,omitempty" yaml:"cleanupState,omitempty"`
	UpdatedAt        string            `json:"updatedAt,omitempty" yaml:"updatedAt,omitempty"`
}

// PendingDeletion prevents a new run from reusing a provider identity while
// the owning service still reports an asynchronous deletion. The scheduler
// treats this as blocked, never as successful cleanup.
type PendingDeletion struct {
	OwnershipMarker string   `json:"ownershipMarker" yaml:"ownershipMarker"`
	MutationKeys    []string `json:"mutationKeys" yaml:"mutationKeys"`
	ResourceRefs    []string `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	RetryAfter      string   `json:"retryAfter,omitempty" yaml:"retryAfter,omitempty"`
	Resolved        bool     `json:"resolved" yaml:"resolved"`
}

type SchedulerOptions struct {
	MaxParallel     int
	QuotaByKey      map[string]int
	SessionRegistry SessionRegistry
	// LocalCapacityReservation is forwarded by ExecuteCertificationRun to the
	// final per-unit scheduler admission gate. Direct ExecuteSchedule callers
	// provide the same implementation through ScheduleExecutionOptions.
	LocalCapacityReservation LocalCapacityReservation
	// ClaimOwner identifies this scheduler invocation when it reserves a
	// reusable session. Callers that resume a durable run may supply the
	// persisted owner; otherwise BuildSchedule creates a fresh owner so two
	// compatible runs cannot accidentally share a claim.
	ClaimOwner     string
	Admission      *AdmissionInput
	AdmissionProbe AdmissionProbe
	// AdmissionSnapshot shares read-only admission facts across schedule builds
	// in one run. It is mutually exclusive with AdmissionProbe.
	AdmissionSnapshot *AdmissionSnapshot
	AdmissionContext  context.Context
	Checkpoints       []SchedulerCheckpoint
	PendingDeletions  []PendingDeletion
	Budget            CertificationBudget
	Now               time.Time
}

// CertificationBudget bounds the work admitted by the scheduler. A zero
// maximum means unlimited for that dimension. Spend limits are enforced only
// when every scheduled unit has a known cost estimate, or when
// RequireCostEstimate is true.
type CertificationBudget struct {
	MaxCloudOperationSeconds   int64
	MaxResourceLifetimeSeconds int64
	MaxEstimatedCostCents      int64
	MaxCleanupLatencySeconds   int64
	RequireCostEstimate        bool
}

type ExecutionUnit struct {
	ID               string                     `json:"id" yaml:"id"`
	CellIDs          []string                   `json:"cellIds" yaml:"cellIds"`
	BaselineCellID   string                     `json:"baselineCellId" yaml:"baselineCellId"`
	Fingerprint      string                     `json:"fingerprint" yaml:"fingerprint"`
	WarmBoundary     string                     `json:"warmBoundary" yaml:"warmBoundary"`
	WarmTransition   sdk.CoverageTransitionKind `json:"warmTransition,omitempty" yaml:"warmTransition,omitempty"`
	Coverage         *sdk.CoverageBoundary      `json:"coverage,omitempty" yaml:"coverage,omitempty"`
	OwnershipMarker  string                     `json:"ownershipMarker" yaml:"ownershipMarker"`
	Cold             bool                       `json:"cold" yaml:"cold"`
	MigrationOwner   bool                       `json:"migrationOwner" yaml:"migrationOwner"`
	DependsOn        []string                   `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`
	MutationKeys     []string                   `json:"mutationKeys" yaml:"mutationKeys"`
	Estimate         ExecutionEstimate          `json:"estimate" yaml:"estimate"`
	Required         bool                       `json:"required" yaml:"required"`
	ReuseSessionID   string                     `json:"reuseSessionId,omitempty" yaml:"reuseSessionId,omitempty"`
	ReuseFingerprint *ReuseFingerprint          `json:"reuseFingerprint,omitempty" yaml:"reuseFingerprint,omitempty"`
}

// SessionClaim identifies a reusable session reserved by a schedule. The
// scheduler keeps the claim through execution; the runner must release it
// after the session has either been handed back to the registry or moved to a
// terminal cleanup state.
type SessionClaim struct {
	Fingerprint ReuseFingerprint `json:"fingerprint" yaml:"fingerprint"`
	Owner       string           `json:"owner" yaml:"owner"`
	SessionID   string           `json:"sessionId" yaml:"sessionId"`
}

type Schedule struct {
	Units   []ExecutionUnit `json:"units" yaml:"units"`
	Batches [][]string      `json:"batches" yaml:"batches"`
	// LocalCapacity is acquired independently by every runnable unit immediately
	// before its checkpoint and provider callback. Keeping the request in the
	// serializable schedule makes the mutation gate explicit without storing an
	// in-process lease in durable plan data.
	LocalCapacity           *LocalCapacityRequest `json:"localCapacity,omitempty" yaml:"localCapacity,omitempty"`
	ReuseClaims             []SessionClaim        `json:"reuseClaims,omitempty" yaml:"reuseClaims,omitempty"`
	CompletedCellIDs        []string              `json:"completedCellIds,omitempty" yaml:"completedCellIds,omitempty"`
	DeferredCellIDs         []string              `json:"deferredCellIds,omitempty" yaml:"deferredCellIds,omitempty"`
	BlockedCellIDs          []string              `json:"blockedCellIds,omitempty" yaml:"blockedCellIds,omitempty"`
	ColdUnitCount           int                   `json:"coldUnitCount" yaml:"coldUnitCount"`
	WarmTransitionCount     int                   `json:"warmTransitionCount" yaml:"warmTransitionCount"`
	ReusedCellCount         int                   `json:"reusedCellCount" yaml:"reusedCellCount"`
	CloudOperationSeconds   int64                 `json:"cloudOperationSeconds" yaml:"cloudOperationSeconds"`
	ResourceLifetimeSeconds int64                 `json:"resourceLifetimeSeconds" yaml:"resourceLifetimeSeconds"`
	EstimatedCostCents      int64                 `json:"estimatedCostCents" yaml:"estimatedCostCents"`
	CleanupLatencySeconds   int64                 `json:"cleanupLatencySeconds" yaml:"cleanupLatencySeconds"`
	CostKnown               bool                  `json:"costKnown" yaml:"costKnown"`
	ReusedSessionCount      int                   `json:"reusedSessionCount" yaml:"reusedSessionCount"`
}

// BuildSchedule validates the complete matrix, filters only explicitly
// completed cells, groups identical fingerprints into reusable units, and
// emits deterministic dependency-safe batches within quota limits.
func BuildSchedule(cells []ScheduleCell, options SchedulerOptions) (schedule Schedule, err error) {
	if options.MaxParallel < 1 {
		return Schedule{}, errors.New("scheduler max parallel must be at least one")
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	}
	if err := validateScheduleCells(cells); err != nil {
		return Schedule{}, err
	}
	if err := validateCertificationBudget(options.Budget); err != nil {
		return Schedule{}, err
	}
	var localCapacity *LocalCapacityRequest
	if options.Admission != nil {
		request := LocalCapacityRequest{
			CPUMilli: options.Admission.RequiredLocalCPUMilli,
			MemoryMB: options.Admission.RequiredLocalMemoryMB,
		}
		if err := request.Validate(); err != nil {
			return Schedule{}, fmt.Errorf("scheduler local capacity: %w", err)
		}
		if !request.empty() {
			localCapacity = &request
		}
	}
	claimOwner, err := resolveScheduleClaimOwner(options)
	if err != nil {
		return Schedule{}, err
	}
	if options.AdmissionSnapshot != nil && options.AdmissionProbe != nil {
		return Schedule{}, errors.New("scheduler admission probe and snapshot are mutually exclusive")
	}
	if options.AdmissionSnapshot != nil && options.Admission == nil {
		return Schedule{}, errors.New("scheduler admission snapshot requires admission input")
	}
	if options.Admission != nil {
		var report AdmissionReport
		var err error
		admissionProbe := options.AdmissionProbe
		if options.AdmissionSnapshot != nil {
			admissionProbe = options.AdmissionSnapshot
		}
		if admissionProbe != nil {
			admissionContext := options.AdmissionContext
			if admissionContext == nil {
				admissionContext = context.Background()
			}
			report, err = CheckAdmissionWithProbe(admissionContext, *options.Admission, admissionProbe)
		} else {
			report, err = CheckAdmission(*options.Admission)
		}
		if err != nil {
			return Schedule{}, fmt.Errorf("scheduler admission: %w", err)
		}
		if !report.Ready {
			return Schedule{}, fmt.Errorf("scheduler admission blocked: %s", strings.Join(report.BlockingReasons, "; "))
		}
	}
	checkpoints, err := validateCheckpoints(cells, options.Checkpoints)
	if err != nil {
		return Schedule{}, err
	}
	if err := validatePendingDeletions(options.PendingDeletions); err != nil {
		return Schedule{}, err
	}

	completed := make(map[string]struct{})
	for _, cell := range cells {
		checkpoint, found := checkpoints[cell.ID]
		if found && checkpoint.Status == "PASS" {
			completed[cell.ID] = struct{}{}
		}
	}
	for _, cell := range cells {
		if cell.Status == CapabilityUnsupported || cell.Status == CapabilityUnavailable || cell.Status == CapabilityBlocked {
			continue
		}
		if _, done := completed[cell.ID]; done {
			continue
		}
		for _, dependency := range cell.Dependencies {
			if _, done := completed[dependency]; done {
				continue
			}
			dependencyCell := findScheduleCell(cells, dependency)
			if dependencyCell.Status == CapabilityUnsupported || dependencyCell.Status == CapabilityUnavailable || dependencyCell.Status == CapabilityBlocked {
				return Schedule{}, fmt.Errorf("cell %q depends on non-runnable cell %q (%s)", cell.ID, dependency, dependencyCell.Status)
			}
		}
	}
	if err := rejectPendingDeletionReuse(cells, completed, options.PendingDeletions); err != nil {
		return Schedule{}, err
	}
	reusableSessions := make(map[string]SessionRecord)
	claimsReleased := false
	defer func() {
		if claimsReleased || options.SessionRegistry == nil || isNilSessionRegistry(options.SessionRegistry) {
			return
		}
		releaseContext := context.WithoutCancel(contextOrBackground(options.AdmissionContext))
		if releaseErr := releaseReusableScheduleSessions(releaseContext, options.SessionRegistry, reusableSessions); releaseErr != nil {
			err = errors.Join(err, releaseErr)
		}
	}()
	reusableSessions, err = findReusableScheduleSessions(contextOrBackground(options.AdmissionContext), cells, completed, options.SessionRegistry, claimOwner)
	if err != nil {
		return Schedule{}, err
	}

	deferred := make([]string, 0)
	for _, cell := range cells {
		if cell.Status == CapabilityUnsupported || cell.Status == CapabilityUnavailable || cell.Status == CapabilityBlocked {
			deferred = append(deferred, cell.ID)
		}
	}
	units, cellToUnit := makeExecutionUnits(cells, completed)
	if err := applyReusableSessions(units, cells, reusableSessions); err != nil {
		return Schedule{}, err
	}
	if err := addUnitDependencies(units, cellToUnit, cells, completed); err != nil {
		return Schedule{}, err
	}
	batches, err := makeBatches(units, options.MaxParallel, options.QuotaByKey)
	if err != nil {
		return Schedule{}, err
	}
	schedule = Schedule{
		Units:           units,
		Batches:         batches,
		LocalCapacity:   localCapacity,
		ReuseClaims:     reusableSessionClaims(reusableSessions),
		DeferredCellIDs: sortedScheduleStrings(deferred),
		CostKnown:       len(units) > 0,
	}
	for cellID := range completed {
		schedule.CompletedCellIDs = append(schedule.CompletedCellIDs, cellID)
	}
	sort.Strings(schedule.CompletedCellIDs)
	for _, unit := range units {
		if unit.Cold && unit.ReuseSessionID == "" {
			schedule.ColdUnitCount++
		} else if unit.ReuseSessionID == "" {
			schedule.WarmTransitionCount++
		}
		if err := addScheduleEstimate(&schedule.CloudOperationSeconds, unit.Estimate.CloudOperationSeconds, "cloud operation time"); err != nil {
			return Schedule{}, err
		}
		if err := addScheduleEstimate(&schedule.ResourceLifetimeSeconds, unit.Estimate.ResourceLifetimeSeconds, "resource lifetime"); err != nil {
			return Schedule{}, err
		}
		if err := addScheduleEstimate(&schedule.EstimatedCostCents, unit.Estimate.EstimatedCostCents, "estimated cost"); err != nil {
			return Schedule{}, err
		}
		if err := addScheduleEstimate(&schedule.CleanupLatencySeconds, unit.Estimate.CleanupLatencySeconds, "cleanup latency"); err != nil {
			return Schedule{}, err
		}
		schedule.CostKnown = schedule.CostKnown && unit.Estimate.CostKnown
		if len(unit.CellIDs) > 1 {
			schedule.ReusedCellCount += len(unit.CellIDs) - 1
		}
		if !unit.Cold {
			schedule.ReusedCellCount++
		}
		if unit.ReuseSessionID != "" {
			schedule.ReusedSessionCount++
		}
	}
	if err := enforceCertificationBudget(schedule, options.Budget); err != nil {
		return Schedule{}, err
	}
	claimsReleased = true
	return schedule, nil
}

// ReleaseScheduleClaims releases the reusable sessions reserved by a
// successful schedule. It is safe to call after provider cleanup has already
// cleared a claim; the registry treats that as an idempotent terminal state.
func ReleaseScheduleClaims(ctx context.Context, registry SessionRegistry, schedule Schedule) error {
	if len(schedule.ReuseClaims) == 0 {
		return nil
	}
	if registry == nil || isNilSessionRegistry(registry) {
		return errors.New("session registry is required to release schedule claims")
	}
	if ctx == nil {
		return errors.New("session registry context is required")
	}
	released := make(map[string]struct{}, len(schedule.ReuseClaims))
	var problems []error
	for _, claim := range schedule.ReuseClaims {
		if strings.TrimSpace(claim.SessionID) == "" || strings.ContainsAny(claim.SessionID, "\r\n\x00") {
			problems = append(problems, errors.New("release session claim requires a single-line session ID"))
			continue
		}
		if strings.TrimSpace(claim.Owner) == "" || strings.ContainsAny(claim.Owner, "\r\n\x00") {
			problems = append(problems, fmt.Errorf("release session claim %q requires a single-line owner", claim.SessionID))
			continue
		}
		if err := claim.Fingerprint.Validate(); err != nil {
			problems = append(problems, fmt.Errorf("release session claim %q: %w", claim.SessionID, err))
			continue
		}
		digest, err := claim.Fingerprint.Digest()
		if err != nil {
			problems = append(problems, fmt.Errorf("release session claim %q: %w", claim.SessionID, err))
			continue
		}
		if _, found := released[digest]; found {
			continue
		}
		released[digest] = struct{}{}
		if err := registry.ReleaseClaim(ctx, claim.Fingerprint, claim.Owner); err != nil {
			problems = append(problems, fmt.Errorf("release session claim %q: %w", claim.SessionID, err))
		}
	}
	return errors.Join(problems...)
}

func validateScheduleCells(cells []ScheduleCell) error {
	if len(cells) == 0 {
		return errors.New("scheduler requires at least one cell")
	}
	known := make(map[string]ScheduleCell, len(cells))
	ownership := make(map[string][]ScheduleCell, len(cells))
	estimates := make(map[string]ExecutionEstimate, len(cells))
	for _, cell := range cells {
		if err := ValidateSecretSafeValue(cell); err != nil {
			return fmt.Errorf("scheduler cell %q contains secret-like material", cell.ID)
		}
		if strings.TrimSpace(cell.ID) == "" {
			return errors.New("scheduler cell ID is required")
		}
		if _, exists := known[cell.ID]; exists {
			return fmt.Errorf("duplicate scheduler cell %q", cell.ID)
		}
		if !fingerprintPattern.MatchString(cell.Fingerprint) {
			return fmt.Errorf("scheduler cell %q requires a SHA-256 fingerprint", cell.ID)
		}
		if cell.ReuseFingerprint != nil {
			if err := cell.ReuseFingerprint.Validate(); err != nil {
				return fmt.Errorf("scheduler cell %q reuse fingerprint: %w", cell.ID, err)
			}
			if cell.ReuseFingerprint.OwnershipMarker != cell.OwnershipMarker {
				return fmt.Errorf("scheduler cell %q reuse fingerprint ownership marker does not match the cell scope", cell.ID)
			}
			digest, err := cell.ReuseFingerprint.Digest()
			if err != nil {
				return fmt.Errorf("scheduler cell %q reuse fingerprint digest: %w", cell.ID, err)
			}
			if digest != cell.Fingerprint {
				return fmt.Errorf("scheduler cell %q fingerprint does not match its complete reuse fingerprint", cell.ID)
			}
		}
		if strings.TrimSpace(cell.WarmBoundary) == "" {
			return fmt.Errorf("scheduler cell %q warm boundary is required", cell.ID)
		}
		if cell.Coverage != nil {
			if err := cell.Coverage.Validate(); err != nil {
				return fmt.Errorf("scheduler cell %q coverage boundary: %w", cell.ID, err)
			}
		}
		if cell.WarmTransition != "" && cell.WarmFrom == "" {
			return fmt.Errorf("scheduler cell %q declares a warm transition without a baseline", cell.ID)
		}
		if strings.TrimSpace(cell.OwnershipMarker) == "" {
			return fmt.Errorf("scheduler cell %q ownership marker is required", cell.ID)
		}
		if len(cell.MutationKeys) == 0 {
			return fmt.Errorf("scheduler cell %q requires mutation keys", cell.ID)
		}
		if err := validateExecutionEstimate(cell.ID, cell.Estimate); err != nil {
			return err
		}
		if hasDuplicate(cell.MutationKeys) {
			return fmt.Errorf("scheduler cell %q contains duplicate mutation keys", cell.ID)
		}
		if cell.Status != CapabilityCompatible && cell.Status != CapabilityExperimental && cell.Status != CapabilityUnsupported && cell.Status != CapabilityUnavailable && cell.Status != CapabilityBlocked {
			return fmt.Errorf("scheduler cell %q has invalid status %q", cell.ID, cell.Status)
		}
		if cell.WarmFrom != "" {
			if cell.WarmFrom == cell.ID {
				return fmt.Errorf("scheduler cell %q cannot warm from itself", cell.ID)
			}
		}
		known[cell.ID] = cell
		ownership[cell.OwnershipMarker] = append(ownership[cell.OwnershipMarker], cell)
		estimateKey := scheduleCellIdentity(cell)
		if estimate, exists := estimates[estimateKey]; exists && estimate != cell.Estimate {
			return fmt.Errorf("scheduler cell %q has an estimate different from its reusable execution unit", cell.ID)
		}
		estimates[estimateKey] = cell.Estimate
	}
	for _, cell := range cells {
		seen := make(map[string]struct{}, len(cell.Dependencies))
		for _, dependency := range cell.Dependencies {
			if _, ok := known[dependency]; !ok {
				return fmt.Errorf("scheduler cell %q depends on unknown cell %q", cell.ID, dependency)
			}
			if dependency == cell.ID {
				return fmt.Errorf("scheduler cell %q cannot depend on itself", cell.ID)
			}
			if _, exists := seen[dependency]; exists {
				return fmt.Errorf("scheduler cell %q contains duplicate dependency %q", cell.ID, dependency)
			}
			seen[dependency] = struct{}{}
		}
		if cell.WarmFrom != "" {
			baseline, ok := known[cell.WarmFrom]
			if !ok {
				return fmt.Errorf("scheduler cell %q warms from unknown cell %q", cell.ID, cell.WarmFrom)
			}
			if baseline.WarmBoundary != cell.WarmBoundary || baseline.OwnershipMarker != cell.OwnershipMarker {
				return fmt.Errorf("scheduler warm transition %q crosses its warm boundary or ownership marker", cell.ID)
			}
			if baseline.Fingerprint == cell.Fingerprint {
				return fmt.Errorf("scheduler warm transition %q does not change its fingerprint", cell.ID)
			}
			if (baseline.ReuseFingerprint == nil) != (cell.ReuseFingerprint == nil) {
				return fmt.Errorf("scheduler warm transition %q has incomplete reusable boundary", cell.ID)
			}
			if baseline.ReuseFingerprint != nil && !compatibleWarmReuseFingerprints(*baseline.ReuseFingerprint, *cell.ReuseFingerprint) {
				return fmt.Errorf("scheduler warm transition %q changes a cold reusable boundary", cell.ID)
			}
			if (baseline.Coverage == nil) != (cell.Coverage == nil) {
				return fmt.Errorf("scheduler warm transition %q has incomplete coverage boundaries", cell.ID)
			}
			if baseline.Coverage != nil {
				baselineFingerprint, err := baseline.Coverage.Fingerprint()
				if err != nil {
					return fmt.Errorf("scheduler warm transition %q baseline coverage: %w", cell.ID, err)
				}
				nextFingerprint, err := cell.Coverage.Fingerprint()
				if err != nil {
					return fmt.Errorf("scheduler warm transition %q next coverage: %w", cell.ID, err)
				}
				if baselineFingerprint != nextFingerprint {
					if err := sdk.ValidateCoverageTransition(*baseline.Coverage, *cell.Coverage, cell.WarmTransition); err != nil {
						return fmt.Errorf("scheduler warm transition %q coverage: %w", cell.ID, err)
					}
				}
			}
		}
	}
	for marker, markerCells := range ownership {
		baselineKey := scheduleCellIdentity(markerCells[0])
		for _, cell := range markerCells {
			cellKey := scheduleCellIdentity(cell)
			if cellKey != baselineKey && cell.WarmFrom == "" {
				return fmt.Errorf("duplicate ownership marker %q crosses incompatible cells without an explicit warm transition", marker)
			}
		}
	}
	if err := scheduleDependencyCycle(cells); err != nil {
		return err
	}
	return nil
}

func validateCheckpoints(cells []ScheduleCell, values []SchedulerCheckpoint) (map[string]SchedulerCheckpoint, error) {
	known := make(map[string]ScheduleCell, len(cells))
	for _, cell := range cells {
		known[cell.ID] = cell
	}
	result := make(map[string]SchedulerCheckpoint, len(values))
	for _, checkpoint := range values {
		if err := ValidateSecretSafeValue(checkpoint); err != nil {
			return nil, fmt.Errorf("checkpoint for cell %q contains secret-like material", checkpoint.CellID)
		}
		cell, ok := known[checkpoint.CellID]
		if !ok {
			return nil, fmt.Errorf("checkpoint references unknown cell %q", checkpoint.CellID)
		}
		if _, exists := result[checkpoint.CellID]; exists {
			return nil, fmt.Errorf("duplicate checkpoint for cell %q", checkpoint.CellID)
		}
		if checkpoint.Fingerprint != cell.Fingerprint {
			return nil, fmt.Errorf("stale checkpoint for cell %q: fingerprint %q does not match %q", checkpoint.CellID, checkpoint.Fingerprint, cell.Fingerprint)
		}
		if checkpoint.OwnershipMarker != cell.OwnershipMarker {
			return nil, fmt.Errorf("checkpoint ownership marker for cell %q does not match the planned scope", checkpoint.CellID)
		}
		if cell.ReuseFingerprint != nil {
			if checkpoint.ReuseFingerprint == nil {
				return nil, fmt.Errorf("checkpoint for cell %q is missing the complete reuse fingerprint", checkpoint.CellID)
			}
			if err := checkpoint.ReuseFingerprint.Validate(); err != nil {
				return nil, fmt.Errorf("checkpoint for cell %q reuse fingerprint: %w", checkpoint.CellID, err)
			}
			plannedDigest, err := cell.ReuseFingerprint.Digest()
			if err != nil {
				return nil, fmt.Errorf("planned reuse fingerprint for cell %q: %w", checkpoint.CellID, err)
			}
			checkpointDigest, err := checkpoint.ReuseFingerprint.Digest()
			if err != nil {
				return nil, fmt.Errorf("checkpoint reuse fingerprint for cell %q: %w", checkpoint.CellID, err)
			}
			if checkpointDigest != plannedDigest {
				return nil, fmt.Errorf("checkpoint for cell %q has a reuse fingerprint different from the planned boundary", checkpoint.CellID)
			}
		}
		if checkpoint.Status != "PASS" && checkpoint.Status != "FAIL" && checkpoint.Status != "PENDING" && checkpoint.Status != "IN_PROGRESS" {
			return nil, fmt.Errorf("checkpoint for cell %q has invalid status %q", checkpoint.CellID, checkpoint.Status)
		}
		if checkpoint.UpdatedAt != "" {
			if parsed, err := time.Parse(time.RFC3339, checkpoint.UpdatedAt); err != nil || parsed.After(time.Now().UTC().Add(24*time.Hour)) {
				return nil, fmt.Errorf("checkpoint for cell %q has invalid updatedAt", checkpoint.CellID)
			}
		}
		result[checkpoint.CellID] = checkpoint
	}
	return result, nil
}

func validatePendingDeletions(values []PendingDeletion) error {
	seen := make(map[string]struct{}, len(values))
	for _, deletion := range values {
		if strings.TrimSpace(deletion.OwnershipMarker) == "" {
			return errors.New("pending deletion ownership marker is required")
		}
		if len(deletion.MutationKeys) == 0 || hasDuplicate(deletion.MutationKeys) {
			return fmt.Errorf("pending deletion %q requires unique mutation keys", deletion.OwnershipMarker)
		}
		if _, exists := seen[deletion.OwnershipMarker]; exists {
			return fmt.Errorf("duplicate pending deletion ownership marker %q", deletion.OwnershipMarker)
		}
		seen[deletion.OwnershipMarker] = struct{}{}
	}
	return nil
}

func validateCertificationBudget(budget CertificationBudget) error {
	for name, value := range map[string]int64{
		"max cloud operation seconds":   budget.MaxCloudOperationSeconds,
		"max resource lifetime seconds": budget.MaxResourceLifetimeSeconds,
		"max estimated cost cents":      budget.MaxEstimatedCostCents,
		"max cleanup latency seconds":   budget.MaxCleanupLatencySeconds,
	} {
		if value < 0 {
			return fmt.Errorf("scheduler %s cannot be negative", name)
		}
	}
	return nil
}

func validateExecutionEstimate(cellID string, estimate ExecutionEstimate) error {
	for name, value := range map[string]int64{
		"cloud operation seconds":   estimate.CloudOperationSeconds,
		"resource lifetime seconds": estimate.ResourceLifetimeSeconds,
		"estimated cost cents":      estimate.EstimatedCostCents,
		"cleanup latency seconds":   estimate.CleanupLatencySeconds,
	} {
		if value < 0 {
			return fmt.Errorf("scheduler cell %q has negative %s", cellID, name)
		}
	}
	return nil
}

func enforceCertificationBudget(schedule Schedule, budget CertificationBudget) error {
	if budget.MaxCloudOperationSeconds > 0 && schedule.CloudOperationSeconds > budget.MaxCloudOperationSeconds {
		return fmt.Errorf("scheduler cloud operation time budget exceeded: %d seconds > %d", schedule.CloudOperationSeconds, budget.MaxCloudOperationSeconds)
	}
	if budget.MaxResourceLifetimeSeconds > 0 && schedule.ResourceLifetimeSeconds > budget.MaxResourceLifetimeSeconds {
		return fmt.Errorf("scheduler resource lifetime budget exceeded: %d seconds > %d", schedule.ResourceLifetimeSeconds, budget.MaxResourceLifetimeSeconds)
	}
	if budget.RequireCostEstimate && !schedule.CostKnown {
		return errors.New("scheduler requires a known cost estimate for every scheduled unit")
	}
	if budget.MaxEstimatedCostCents > 0 {
		if !schedule.CostKnown {
			return errors.New("scheduler cannot enforce a spend budget because the cost estimate is unknown")
		}
		if schedule.EstimatedCostCents > budget.MaxEstimatedCostCents {
			return fmt.Errorf("scheduler spend budget exceeded: %d cents > %d", schedule.EstimatedCostCents, budget.MaxEstimatedCostCents)
		}
	}
	if budget.MaxCleanupLatencySeconds > 0 && schedule.CleanupLatencySeconds > budget.MaxCleanupLatencySeconds {
		return fmt.Errorf("scheduler cleanup latency budget exceeded: %d seconds > %d", schedule.CleanupLatencySeconds, budget.MaxCleanupLatencySeconds)
	}
	return nil
}

func addScheduleEstimate(total *int64, value int64, name string) error {
	if value < 0 {
		return fmt.Errorf("scheduler %s estimate cannot be negative", name)
	}
	if *total > math.MaxInt64-value {
		return fmt.Errorf("scheduler %s estimate overflows int64", name)
	}
	*total += value
	return nil
}

func compatibleWarmReuseFingerprints(baseline, next ReuseFingerprint) bool {
	return baseline.Fixture == next.Fixture &&
		baseline.BackupSet == next.BackupSet &&
		baseline.Observability == next.Observability &&
		baseline.Edge == next.Edge &&
		baseline.ArtifactDigest == next.ArtifactDigest &&
		baseline.SchemaFingerprint == next.SchemaFingerprint &&
		baseline.MigrationFingerprint == next.MigrationFingerprint &&
		baseline.OwnershipMarker == next.OwnershipMarker &&
		baseline.StateBackend == next.StateBackend
}

func rejectPendingDeletionReuse(cells []ScheduleCell, completed map[string]struct{}, deletions []PendingDeletion) error {
	for _, deletion := range deletions {
		if deletion.Resolved {
			continue
		}
		for _, cell := range cells {
			if _, done := completed[cell.ID]; done {
				continue
			}
			if intersects(cell.MutationKeys, deletion.MutationKeys) {
				return fmt.Errorf("cell %q is blocked by unresolved asynchronous deletion for ownership marker %q", cell.ID, deletion.OwnershipMarker)
			}
		}
	}
	return nil
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func resolveScheduleClaimOwner(options SchedulerOptions) (string, error) {
	if options.ClaimOwner != "" {
		if strings.TrimSpace(options.ClaimOwner) == "" || strings.ContainsAny(options.ClaimOwner, "\r\n\x00") {
			return "", errors.New("scheduler claim owner must be a non-empty single-line value")
		}
		return options.ClaimOwner, nil
	}
	if options.SessionRegistry == nil || isNilSessionRegistry(options.SessionRegistry) {
		return "", nil
	}
	return "certification-schedule/" + uuid.NewString(), nil
}

// findReusableScheduleSessions is deliberately opt-in per cell: a caller
// supplies ReuseFingerprint only when it has the complete boundary identity
// needed to safely reuse an existing session. A partial scheduler fingerprint
// can group cells, but it cannot authorize provider-resource reuse.
func findReusableScheduleSessions(ctx context.Context, cells []ScheduleCell, completed map[string]struct{}, registry SessionRegistry, claimOwner string) (map[string]SessionRecord, error) {
	if registry == nil {
		return nil, nil
	}
	if isNilSessionRegistry(registry) {
		return nil, errors.New("session registry is required")
	}
	if ctx == nil {
		return nil, errors.New("session registry context is required")
	}
	result := make(map[string]SessionRecord)
	for _, cell := range cells {
		if cell.ReuseFingerprint == nil || cell.WarmFrom != "" {
			continue
		}
		if _, done := completed[cell.ID]; done {
			continue
		}
		lookup, err := ClaimReusableSession(ctx, registry, *cell.ReuseFingerprint, claimOwner)
		if err != nil {
			return result, fmt.Errorf("lookup reusable session for cell %q: %w", cell.ID, err)
		}
		switch lookup.Decision {
		case ReuseReady:
			result[cell.ID] = lookup.Record
		case ReuseMiss:
			continue
		case ReuseBlocked:
			return result, fmt.Errorf("cell %q is blocked from session reuse: %s", cell.ID, lookup.Reason)
		default:
			return result, fmt.Errorf("cell %q returned unknown session reuse decision %q", cell.ID, lookup.Decision)
		}
	}
	return result, nil
}

func releaseReusableScheduleSessions(ctx context.Context, registry SessionRegistry, records map[string]SessionRecord) error {
	if registry == nil || len(records) == 0 {
		return nil
	}
	released := make(map[string]struct{}, len(records))
	var problems []error
	for _, record := range records {
		if record.ClaimOwner == "" {
			continue
		}
		digest, err := record.Fingerprint.Digest()
		if err != nil {
			problems = append(problems, err)
			continue
		}
		if _, found := released[digest]; found {
			continue
		}
		released[digest] = struct{}{}
		if err := registry.ReleaseClaim(ctx, record.Fingerprint, record.ClaimOwner); err != nil {
			problems = append(problems, fmt.Errorf("release reusable session claim %q: %w", record.SessionID, err))
		}
	}
	return errors.Join(problems...)
}

func reusableSessionClaims(records map[string]SessionRecord) []SessionClaim {
	if len(records) == 0 {
		return nil
	}
	claims := make([]SessionClaim, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if record.ClaimOwner == "" {
			continue
		}
		digest, err := record.Fingerprint.Digest()
		if err != nil {
			continue
		}
		if _, found := seen[digest]; found {
			continue
		}
		seen[digest] = struct{}{}
		claims = append(claims, SessionClaim{Fingerprint: record.Fingerprint, Owner: record.ClaimOwner, SessionID: record.SessionID})
	}
	sort.Slice(claims, func(i, j int) bool {
		if claims[i].SessionID != claims[j].SessionID {
			return claims[i].SessionID < claims[j].SessionID
		}
		return claims[i].Owner < claims[j].Owner
	})
	return claims
}

func applyReusableSessions(units []ExecutionUnit, cells []ScheduleCell, records map[string]SessionRecord) error {
	if len(records) == 0 {
		return nil
	}
	byCell := make(map[string]ScheduleCell, len(cells))
	for _, cell := range cells {
		byCell[cell.ID] = cell
	}
	for index := range units {
		var reused SessionRecord
		for _, cellID := range units[index].CellIDs {
			record, found := records[cellID]
			if !found {
				continue
			}
			if reused.SessionID != "" && reused.SessionID != record.SessionID {
				return fmt.Errorf("execution unit %q combines incompatible reusable sessions", units[index].ID)
			}
			reused = record
		}
		if reused.SessionID == "" {
			continue
		}
		reusedDigest, err := reused.Fingerprint.Digest()
		if err != nil {
			return fmt.Errorf("execution unit %q reusable session fingerprint: %w", units[index].ID, err)
		}
		for _, cellID := range units[index].CellIDs {
			cell := byCell[cellID]
			if cell.ReuseFingerprint == nil {
				return fmt.Errorf("execution unit %q combines reusable and non-reusable cells", units[index].ID)
			}
			cellDigest, err := cell.ReuseFingerprint.Digest()
			if err != nil || cellDigest != reusedDigest {
				return fmt.Errorf("execution unit %q combines incompatible reusable fingerprints", units[index].ID)
			}
		}
		baseline := byCell[units[index].BaselineCellID]
		if _, found := records[baseline.ID]; !found {
			return fmt.Errorf("execution unit %q reuses a session without a reusable baseline", units[index].ID)
		}
		units[index].ReuseSessionID = reused.SessionID
		units[index].Cold = false
	}
	return nil
}

func makeExecutionUnits(cells []ScheduleCell, completed map[string]struct{}) ([]ExecutionUnit, map[string]string) {
	groups := make(map[string]*ExecutionUnit)
	cellToUnit := make(map[string]string, len(cells))
	completedBaselines := make(map[string]ScheduleCell)
	for _, cell := range cells {
		if _, done := completed[cell.ID]; done && cell.Status != CapabilityUnsupported && cell.Status != CapabilityUnavailable && cell.Status != CapabilityBlocked && cell.WarmFrom == "" {
			key := scheduleCellIdentity(cell)
			completedBaselines[key] = cell
		}
	}
	for _, cell := range cells {
		if _, done := completed[cell.ID]; done || cell.Status == CapabilityUnsupported || cell.Status == CapabilityUnavailable || cell.Status == CapabilityBlocked {
			continue
		}
		if cell.WarmFrom != "" {
			unit := executionUnitForTransition(cell)
			groups[unit.ID] = &unit
			cellToUnit[cell.ID] = unit.ID
			continue
		}
		key := scheduleCellIdentity(cell)
		unit := groups[key]
		if unit == nil {
			created := executionUnitForBaseline(cell)
			if baseline, found := completedBaselines[key]; found {
				created.BaselineCellID = baseline.ID
				created.Cold = false
				created.MutationKeys = unionSorted(created.MutationKeys, baseline.MutationKeys)
			}
			unit = &created
			groups[key] = unit
		}
		unit.CellIDs = append(unit.CellIDs, cell.ID)
		unit.MutationKeys = unionSorted(unit.MutationKeys, cell.MutationKeys)
		unit.Required = unit.Required || cell.Required
		if cell.Required && (unit.BaselineCellID == "" || cell.ID < unit.BaselineCellID) {
			unit.BaselineCellID = cell.ID
		}
		cellToUnit[cell.ID] = unit.ID
	}
	units := make([]ExecutionUnit, 0, len(groups))
	for _, unit := range groups {
		sort.Strings(unit.CellIDs)
		if unit.BaselineCellID == "" {
			unit.BaselineCellID = unit.CellIDs[0]
		}
		units = append(units, *unit)
	}
	sort.Slice(units, func(i, j int) bool { return units[i].ID < units[j].ID })
	return units, cellToUnit
}

func executionUnitForBaseline(cell ScheduleCell) ExecutionUnit {
	return ExecutionUnit{ID: "unit-" + shortHash(scheduleCellIdentity(cell)), BaselineCellID: cell.ID, Fingerprint: cell.Fingerprint, WarmBoundary: cell.WarmBoundary, Coverage: cloneCoverageBoundary(cell.Coverage), OwnershipMarker: cell.OwnershipMarker, Cold: true, MutationKeys: append([]string(nil), cell.MutationKeys...), Estimate: cell.Estimate, Required: cell.Required, ReuseFingerprint: cloneReuseFingerprint(cell.ReuseFingerprint)}
}

func executionUnitForTransition(cell ScheduleCell) ExecutionUnit {
	return ExecutionUnit{ID: "transition-" + shortHash(cell.ID+"\x00"+cell.Fingerprint+"\x00"+cell.OwnershipMarker), CellIDs: []string{cell.ID}, BaselineCellID: cell.WarmFrom, Fingerprint: cell.Fingerprint, WarmBoundary: cell.WarmBoundary, WarmTransition: cell.WarmTransition, Coverage: cloneCoverageBoundary(cell.Coverage), OwnershipMarker: cell.OwnershipMarker, Cold: false, MigrationOwner: true, MutationKeys: append([]string(nil), cell.MutationKeys...), Estimate: cell.Estimate, Required: cell.Required, ReuseFingerprint: cloneReuseFingerprint(cell.ReuseFingerprint)}
}

func cloneReuseFingerprint(fingerprint *ReuseFingerprint) *ReuseFingerprint {
	if fingerprint == nil {
		return nil
	}
	clone := *fingerprint
	return &clone
}

func scheduleCellIdentity(cell ScheduleCell) string {
	coverage := ""
	if cell.Coverage != nil {
		if fingerprint, err := cell.Coverage.Fingerprint(); err == nil {
			coverage = fingerprint
		}
	}
	return cell.WarmBoundary + "\x00" + cell.Fingerprint + "\x00" + cell.OwnershipMarker + "\x00" + coverage
}

func cloneCoverageBoundary(boundary *sdk.CoverageBoundary) *sdk.CoverageBoundary {
	if boundary == nil {
		return nil
	}
	clone := *boundary
	clone.Regions = append([]string(nil), boundary.Regions...)
	clone.ServiceMajors = make(map[string]string, len(boundary.ServiceMajors))
	for role, major := range boundary.ServiceMajors {
		clone.ServiceMajors[role] = major
	}
	return &clone
}

func addUnitDependencies(units []ExecutionUnit, cellToUnit map[string]string, cells []ScheduleCell, completed map[string]struct{}) error {
	byID := make(map[string]*ExecutionUnit, len(units))
	for i := range units {
		byID[units[i].ID] = &units[i]
	}
	for _, cell := range cells {
		unitID, scheduled := cellToUnit[cell.ID]
		if !scheduled {
			continue
		}
		unit := byID[unitID]
		for _, dependency := range cell.Dependencies {
			if _, done := completed[dependency]; done {
				continue
			}
			dependencyUnit, exists := cellToUnit[dependency]
			if !exists {
				return fmt.Errorf("cell %q dependency %q is not schedulable", cell.ID, dependency)
			}
			if dependencyUnit != unitID {
				unit.DependsOn = appendUnique(unit.DependsOn, dependencyUnit)
			}
		}
		if cell.WarmFrom != "" {
			baselineUnit, exists := cellToUnit[cell.WarmFrom]
			if !exists {
				if _, done := completed[cell.WarmFrom]; !done {
					return fmt.Errorf("warm transition cell %q references unscheduled baseline %q", cell.ID, cell.WarmFrom)
				}
			} else if baselineUnit != unitID {
				unit.DependsOn = appendUnique(unit.DependsOn, baselineUnit)
			}
		}
	}
	for i := range units {
		sort.Strings(units[i].DependsOn)
	}
	return nil
}

func makeBatches(units []ExecutionUnit, maxParallel int, quotaByKey map[string]int) ([][]string, error) {
	remaining := make(map[string]ExecutionUnit, len(units))
	for _, unit := range units {
		remaining[unit.ID] = unit
	}
	batches := make([][]string, 0)
	completed := make(map[string]struct{}, len(units))
	for len(remaining) > 0 {
		ready := make([]ExecutionUnit, 0, len(remaining))
		for _, unit := range remaining {
			allDone := true
			for _, dependency := range unit.DependsOn {
				if _, done := completed[dependency]; !done {
					allDone = false
					break
				}
			}
			if allDone {
				ready = append(ready, unit)
			}
		}
		if len(ready) == 0 {
			return nil, errors.New("scheduler dependency graph cannot make progress")
		}
		sort.Slice(ready, func(i, j int) bool { return ready[i].ID < ready[j].ID })
		batch := make([]string, 0, maxParallel)
		usage := make(map[string]int)
		for _, unit := range ready {
			if len(batch) >= maxParallel {
				break
			}
			if intersectsBatch(unit.MutationKeys, batch, remaining) {
				continue
			}
			if exceedsQuota(unit.MutationKeys, usage, quotaByKey) {
				continue
			}
			batch = append(batch, unit.ID)
			for _, key := range unit.MutationKeys {
				usage[key]++
			}
		}
		if len(batch) == 0 {
			return nil, errors.New("scheduler quota or mutation-key constraints prevent progress")
		}
		for _, unitID := range batch {
			delete(remaining, unitID)
			completed[unitID] = struct{}{}
		}
		batches = append(batches, batch)
	}
	return batches, nil
}

func intersectsBatch(keys []string, batch []string, remaining map[string]ExecutionUnit) bool {
	for _, unitID := range batch {
		if intersects(keys, remaining[unitID].MutationKeys) {
			return true
		}
	}
	return false
}

func exceedsQuota(keys []string, usage map[string]int, quotas map[string]int) bool {
	for _, key := range keys {
		limit, constrained := quotas[key]
		if constrained && limit < 1 {
			return true
		}
		if constrained && usage[key]+1 > limit {
			return true
		}
	}
	return false
}

func scheduleDependencyCycle(cells []ScheduleCell) error {
	known := make(map[string]ScheduleCell, len(cells))
	for _, cell := range cells {
		known[cell.ID] = cell
	}
	visiting := make(map[string]bool, len(cells))
	visited := make(map[string]bool, len(cells))
	var visit func(string) error
	visit = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("scheduler dependency cycle at %q", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range known[id].Dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	ids := make([]string, 0, len(cells))
	for _, cell := range cells {
		ids = append(ids, cell.ID)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func findScheduleCell(cells []ScheduleCell, id string) ScheduleCell {
	for _, cell := range cells {
		if cell.ID == id {
			return cell
		}
	}
	return ScheduleCell{}
}

func hasDuplicate(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			return true
		}
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func intersects(left, right []string) bool {
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	for _, value := range right {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}

func unionSorted(left, right []string) []string {
	values := append(append([]string(nil), left...), right...)
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func sortedScheduleStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func shortHash(value string) string {
	// Fingerprint identity is already validated at the cell boundary. This
	// short unit ID is only a deterministic display key, not a security proof.
	var hash uint64 = 14695981039346656037
	for i := 0; i < len(value); i++ {
		hash ^= uint64(value[i])
		hash *= 1099511628211
	}
	return fmt.Sprintf("%016x", hash)
}
