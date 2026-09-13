package certification

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ScheduleExecutionResult contains provider operation identities returned by
// one execution unit. The scheduler does not interpret those identities; it
// only carries them into checkpoints so a provider adapter can resume its own
// idempotent operation.
type ScheduleExecutionResult struct {
	StackID          string
	ReuseFingerprint *ReuseFingerprint
	OperationIDs     []string
	ResourceRefs     []string
	BackupRefs       []string
	RestoreRefs      []string
	TelemetryRefs    []string
	EdgeRefs         []string
	ReuseMode        string
	CleanupState     string
	OwnershipMarker  string
}

// ScheduleExecutor is the only provider-specific callback in the scheduler.
// It receives one complete cold baseline or warm transition. Provider code
// owns native provisioning, polling, verification, and cleanup details.
type ScheduleExecutor func(context.Context, ExecutionUnit) (ScheduleExecutionResult, error)

// ScheduleCheckpointWriter persists one unit's cell checkpoints as one
// logical write. Implementations should make the write atomic enough that an
// interruption cannot produce a partially updated unit in the backing store.
type ScheduleCheckpointWriter func(context.Context, []SchedulerCheckpoint) error

type ScheduleExecutionOptions struct {
	CheckpointWriter ScheduleCheckpointWriter
	SessionRegistry  SessionRegistry
	// LocalCapacityReservation is the final local mutation gate. Every unit
	// acquires schedule.LocalCapacity before it can write IN_PROGRESS or call
	// the provider executor.
	LocalCapacityReservation LocalCapacityReservation
	ClaimReleaseTimeout      time.Duration
	// CheckpointWriteTimeout bounds terminal checkpoint persistence after a
	// provider callback returns. Terminal writes use a detached context so an
	// interrupted provider operation can still record its operation IDs and
	// cleanup state for a safe resume.
	CheckpointWriteTimeout time.Duration
	Now                    time.Time
}

const defaultScheduleCheckpointWriteTimeout = 30 * time.Second

// ScheduleExecution is the latest provider-neutral checkpoint view after an
// execution attempt. A failed unit is returned with its failure checkpoint;
// callers can pass the checkpoints back to BuildSchedule to resume the same
// fingerprint and ownership scope.
type ScheduleExecution struct {
	Checkpoints      []SchedulerCheckpoint `json:"checkpoints" yaml:"checkpoints"`
	CompletedUnitIDs []string              `json:"completedUnitIds" yaml:"completedUnitIds"`
	FailedUnitID     string                `json:"failedUnitId,omitempty" yaml:"failedUnitId,omitempty"`
}

// ExecuteSchedule runs dependency-safe batches concurrently while preserving
// the mutation-key and quota decisions made by BuildSchedule. Independent
// provider groups can overlap; units that share any mutation key are never
// started in the same batch. Checkpoints are written before and after every
// unit so an interrupted run never needs to guess whether a mutation started.
func ExecuteSchedule(ctx context.Context, schedule Schedule, execute ScheduleExecutor, options ScheduleExecutionOptions) (result ScheduleExecution, err error) {
	if ctx == nil {
		return ScheduleExecution{}, errors.New("schedule execution context is required")
	}
	if execute == nil {
		return ScheduleExecution{}, errors.New("schedule executor is required")
	}
	if options.CheckpointWriter == nil {
		return ScheduleExecution{}, errors.New("schedule checkpoint writer is required")
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	}
	if options.ClaimReleaseTimeout < 0 {
		return ScheduleExecution{}, errors.New("schedule claim release timeout cannot be negative")
	}
	if options.ClaimReleaseTimeout == 0 {
		options.ClaimReleaseTimeout = 30 * time.Second
	}
	if options.CheckpointWriteTimeout < 0 {
		return ScheduleExecution{}, errors.New("schedule checkpoint write timeout cannot be negative")
	}
	if options.CheckpointWriteTimeout == 0 {
		options.CheckpointWriteTimeout = defaultScheduleCheckpointWriteTimeout
	}
	defer func() {
		if options.SessionRegistry == nil || len(schedule.ReuseClaims) == 0 {
			return
		}
		releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), options.ClaimReleaseTimeout)
		defer cancel()
		err = errors.Join(err, ReleaseScheduleClaims(releaseContext, options.SessionRegistry, schedule))
	}()
	if err := validateExecutionSchedule(schedule); err != nil {
		return ScheduleExecution{}, err
	}
	if schedule.LocalCapacity != nil && !schedule.LocalCapacity.empty() && isNilLocalCapacityReservation(options.LocalCapacityReservation) {
		return ScheduleExecution{}, errors.New("schedule local capacity reservation is required")
	}

	units := make(map[string]ExecutionUnit, len(schedule.Units))
	for _, unit := range schedule.Units {
		units[unit.ID] = unit
	}
	result = ScheduleExecution{}
	checkpointByCell := make(map[string]SchedulerCheckpoint)
	var stateMu sync.Mutex
	var writeMu sync.Mutex

	write := func(ctx context.Context, checkpoints []SchedulerCheckpoint) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := options.CheckpointWriter(ctx, checkpoints); err != nil {
			return err
		}
		stateMu.Lock()
		defer stateMu.Unlock()
		for _, checkpoint := range checkpoints {
			checkpointByCell[checkpoint.CellID] = checkpoint
		}
		return nil
	}

	executionCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, batch := range schedule.Batches {
		if err := executionCtx.Err(); err != nil {
			return finishScheduleExecution(checkpointByCell, result, err)
		}
		results := make(chan scheduleUnitResult, len(batch))
		var waitGroup sync.WaitGroup
		for _, unitID := range batch {
			unit := units[unitID]
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				terminalWrite := func(checkpoints []SchedulerCheckpoint) error {
					checkpointCtx, checkpointCancel := context.WithTimeout(context.WithoutCancel(executionCtx), options.CheckpointWriteTimeout)
					defer checkpointCancel()
					return write(checkpointCtx, checkpoints)
				}
				results <- runScheduleUnitWithLocalCapacity(executionCtx, unit, schedule.LocalCapacity, options.LocalCapacityReservation, execute, write, terminalWrite, options.Now)
			}()
		}
		waitGroup.Wait()
		close(results)
		var batchErr error
		for item := range results {
			if item.err != nil {
				if result.FailedUnitID == "" {
					result.FailedUnitID = item.unitID
				}
				batchErr = errors.Join(batchErr, item.err)
				continue
			}
			result.CompletedUnitIDs = append(result.CompletedUnitIDs, item.unitID)
		}
		if batchErr != nil {
			cancel()
			return finishScheduleExecution(checkpointByCell, result, batchErr)
		}
	}
	return finishScheduleExecution(checkpointByCell, result, nil)
}

type scheduleUnitResult struct {
	unitID string
	err    error
}

func runScheduleUnitWithLocalCapacity(ctx context.Context, unit ExecutionUnit, request *LocalCapacityRequest, reservation LocalCapacityReservation, execute ScheduleExecutor, write ScheduleCheckpointWriter, terminalWrite func([]SchedulerCheckpoint) error, now time.Time) (result scheduleUnitResult) {
	result.unitID = unit.ID
	lease, err := acquireLocalCapacity(ctx, reservation, request)
	if err != nil {
		result.err = fmt.Errorf("reserve local capacity for unit %q: %w", unit.ID, err)
		return result
	}
	if lease != nil {
		defer func() {
			if releaseErr := lease.Release(); releaseErr != nil {
				result.err = errors.Join(result.err, fmt.Errorf("release local capacity for unit %q: %w", unit.ID, releaseErr))
			}
		}()
	}
	return runScheduleUnit(ctx, unit, execute, write, terminalWrite, now)
}

func runScheduleUnit(ctx context.Context, unit ExecutionUnit, execute ScheduleExecutor, write ScheduleCheckpointWriter, terminalWrite func([]SchedulerCheckpoint) error, now time.Time) (result scheduleUnitResult) {
	result.unitID = unit.ID
	if err := ctx.Err(); err != nil {
		result.err = err
		return result
	}
	inProgress := unitCheckpoints(unit, "IN_PROGRESS", now, ScheduleExecutionResult{})
	if err := write(ctx, inProgress); err != nil {
		result.err = fmt.Errorf("checkpoint unit %q in progress: %w", unit.ID, err)
		return result
	}
	execution, err := execute(ctx, unit)
	execution.ReuseFingerprint = executionReuseFingerprint(unit, execution.ReuseFingerprint)
	if execution.ReuseMode == "" {
		switch {
		case unit.ReuseSessionID != "":
			execution.ReuseMode = "reused-session"
		case unit.Cold:
			execution.ReuseMode = "cold"
		default:
			execution.ReuseMode = "warm-transition"
		}
	}
	if err != nil {
		failed := unitCheckpoints(unit, "FAIL", now, execution)
		if checkpointErr := terminalWrite(failed); checkpointErr != nil {
			result.err = errors.Join(fmt.Errorf("execute unit %q: %w", unit.ID, err), fmt.Errorf("checkpoint unit %q failure: %w", unit.ID, checkpointErr))
			return result
		}
		result.err = fmt.Errorf("execute unit %q: %w", unit.ID, err)
		return result
	}
	if err := validateScheduleExecutionResult(unit, execution); err != nil {
		failed := unitCheckpoints(unit, "FAIL", now, execution)
		if checkpointErr := terminalWrite(failed); checkpointErr != nil {
			result.err = errors.Join(err, fmt.Errorf("checkpoint unit %q validation failure: %w", unit.ID, checkpointErr))
			return result
		}
		result.err = err
		return result
	}
	if err := terminalWrite(unitCheckpoints(unit, "PASS", now, execution)); err != nil {
		result.err = fmt.Errorf("checkpoint unit %q passed: %w", unit.ID, err)
	}
	return result
}

func unitCheckpoints(unit ExecutionUnit, status string, now time.Time, execution ScheduleExecutionResult) []SchedulerCheckpoint {
	operationIDs := append([]string(nil), execution.OperationIDs...)
	checkpoints := make([]SchedulerCheckpoint, 0, len(unit.CellIDs))
	for _, cellID := range unit.CellIDs {
		checkpoints = append(checkpoints, SchedulerCheckpoint{
			CellID: cellID, Fingerprint: unit.Fingerprint, OwnershipMarker: unit.OwnershipMarker,
			ReuseFingerprint: executionReuseFingerprint(unit, execution.ReuseFingerprint),
			Status:           status, StackID: execution.StackID, OperationIDs: append([]string(nil), operationIDs...),
			ResourceRefs: append([]string(nil), execution.ResourceRefs...), BackupRefs: append([]string(nil), execution.BackupRefs...),
			RestoreRefs: append([]string(nil), execution.RestoreRefs...), TelemetryRefs: append([]string(nil), execution.TelemetryRefs...),
			EdgeRefs: append([]string(nil), execution.EdgeRefs...), ReuseMode: execution.ReuseMode, CleanupState: execution.CleanupState,
			UpdatedAt: now.Format(time.RFC3339),
		})
	}
	return checkpoints
}

func validateScheduleExecutionResult(unit ExecutionUnit, result ScheduleExecutionResult) error {
	if err := ValidateSecretSafeValue(result); err != nil {
		return fmt.Errorf("unit %q returned secret-like material", unit.ID)
	}
	if result.OwnershipMarker != unit.OwnershipMarker {
		return fmt.Errorf("unit %q returned ownership marker %q, want %q", unit.ID, result.OwnershipMarker, unit.OwnershipMarker)
	}
	if result.ReuseFingerprint != nil {
		if err := result.ReuseFingerprint.Validate(); err != nil {
			return fmt.Errorf("unit %q returned an invalid reuse fingerprint", unit.ID)
		}
		digest, err := result.ReuseFingerprint.Digest()
		if err != nil {
			return fmt.Errorf("unit %q returned an invalid reuse fingerprint: %w", unit.ID, err)
		}
		if digest != unit.Fingerprint || result.ReuseFingerprint.OwnershipMarker != unit.OwnershipMarker {
			return fmt.Errorf("unit %q returned a reuse fingerprint different from its execution identity", unit.ID)
		}
	}
	for name, value := range map[string]string{"stack ID": result.StackID, "ownership marker": result.OwnershipMarker} {
		if value != "" && strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("unit %q returned a %s containing a control character", unit.ID, name)
		}
	}
	for name, values := range map[string][]string{
		"operation ID": result.OperationIDs, "resource reference": result.ResourceRefs, "backup reference": result.BackupRefs,
		"restore reference": result.RestoreRefs, "telemetry reference": result.TelemetryRefs, "edge reference": result.EdgeRefs,
	} {
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("unit %q returned an invalid %s", unit.ID, name)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("unit %q returned duplicate %s %q", unit.ID, name, value)
			}
			seen[value] = struct{}{}
		}
	}
	if result.ReuseMode != "cold" && result.ReuseMode != "warm-transition" && result.ReuseMode != "reused-session" {
		return fmt.Errorf("unit %q returned invalid reuse mode %q", unit.ID, result.ReuseMode)
	}
	if result.CleanupState != "" && result.CleanupState != "pending" && result.CleanupState != "complete" && result.CleanupState != "failed" && result.CleanupState != "protected" {
		return fmt.Errorf("unit %q returned invalid cleanup state %q", unit.ID, result.CleanupState)
	}
	return nil
}

func executionReuseFingerprint(unit ExecutionUnit, result *ReuseFingerprint) *ReuseFingerprint {
	if result != nil {
		copy := *result
		return &copy
	}
	if unit.ReuseFingerprint == nil {
		return nil
	}
	copy := *unit.ReuseFingerprint
	return &copy
}

func validateExecutionSchedule(schedule Schedule) error {
	if err := validateScheduleClaims(schedule.ReuseClaims); err != nil {
		return err
	}
	if schedule.LocalCapacity != nil {
		if err := schedule.LocalCapacity.Validate(); err != nil {
			return fmt.Errorf("schedule local capacity: %w", err)
		}
	}
	if len(schedule.Units) == 0 {
		if len(schedule.Batches) != 0 {
			return errors.New("schedule batches exist without executable units")
		}
		return nil
	}
	units := make(map[string]ExecutionUnit, len(schedule.Units))
	for _, unit := range schedule.Units {
		if strings.TrimSpace(unit.ID) == "" || len(unit.CellIDs) == 0 || strings.TrimSpace(unit.OwnershipMarker) == "" {
			return errors.New("schedule units require an ID, cell IDs, and ownership marker")
		}
		if unit.Coverage != nil {
			if err := unit.Coverage.Validate(); err != nil {
				return fmt.Errorf("schedule unit %q coverage boundary: %w", unit.ID, err)
			}
		}
		if unit.ReuseFingerprint != nil {
			if err := unit.ReuseFingerprint.Validate(); err != nil {
				return fmt.Errorf("schedule unit %q reuse fingerprint: %w", unit.ID, err)
			}
			digest, err := unit.ReuseFingerprint.Digest()
			if err != nil {
				return fmt.Errorf("schedule unit %q reuse fingerprint digest: %w", unit.ID, err)
			}
			if digest != unit.Fingerprint || unit.ReuseFingerprint.OwnershipMarker != unit.OwnershipMarker {
				return fmt.Errorf("schedule unit %q reuse fingerprint does not match its execution identity", unit.ID)
			}
		}
		if hasDuplicate(unit.CellIDs) || hasDuplicate(unit.MutationKeys) {
			return fmt.Errorf("schedule unit %q contains duplicate cell or mutation IDs", unit.ID)
		}
		if _, exists := units[unit.ID]; exists {
			return fmt.Errorf("schedule contains duplicate unit %q", unit.ID)
		}
		units[unit.ID] = unit
	}
	seenUnits := make(map[string]int, len(units))
	lastBatch := make(map[string]int, len(units))
	for batchIndex, batch := range schedule.Batches {
		batchKeys := make([]string, 0)
		for _, unitID := range batch {
			unit, exists := units[unitID]
			if !exists {
				return fmt.Errorf("schedule batch references unknown unit %q", unitID)
			}
			if seenUnits[unitID] > 0 {
				return fmt.Errorf("schedule unit %q appears more than once", unitID)
			}
			seenUnits[unitID]++
			lastBatch[unitID] = batchIndex
			for _, key := range unit.MutationKeys {
				for _, priorKey := range batchKeys {
					if key == priorKey {
						return fmt.Errorf("schedule batch %d overlaps mutation key %q", batchIndex, key)
					}
				}
				batchKeys = append(batchKeys, key)
			}
			for _, dependency := range unit.DependsOn {
				if _, exists := units[dependency]; !exists {
					return fmt.Errorf("schedule unit %q depends on unknown unit %q", unitID, dependency)
				}
				if dependencyBatch, scheduled := lastBatch[dependency]; scheduled && dependencyBatch >= batchIndex {
					return fmt.Errorf("schedule unit %q dependency %q is not ordered before its batch", unitID, dependency)
				}
			}
		}
	}
	if len(seenUnits) != len(units) {
		return errors.New("schedule batches do not cover every execution unit")
	}
	for _, unit := range units {
		for _, dependency := range unit.DependsOn {
			if lastBatch[dependency] >= lastBatch[unit.ID] {
				return fmt.Errorf("schedule unit %q dependency %q is not ordered before its batch", unit.ID, dependency)
			}
		}
	}
	return nil
}

func validateScheduleClaims(claims []SessionClaim) error {
	seen := make(map[string]struct{}, len(claims))
	for _, claim := range claims {
		if strings.TrimSpace(claim.SessionID) == "" || strings.ContainsAny(claim.SessionID, "\r\n\x00") {
			return errors.New("schedule session claims require a single-line session ID")
		}
		if strings.TrimSpace(claim.Owner) == "" || strings.ContainsAny(claim.Owner, "\r\n\x00") {
			return fmt.Errorf("schedule session claim %q requires a single-line owner", claim.SessionID)
		}
		if err := claim.Fingerprint.Validate(); err != nil {
			return fmt.Errorf("schedule session claim %q fingerprint: %w", claim.SessionID, err)
		}
		digest, err := claim.Fingerprint.Digest()
		if err != nil {
			return fmt.Errorf("schedule session claim %q fingerprint digest: %w", claim.SessionID, err)
		}
		if _, found := seen[digest]; found {
			return fmt.Errorf("schedule contains duplicate session claim for %q", digest)
		}
		seen[digest] = struct{}{}
	}
	return nil
}

func finishScheduleExecution(checkpointByCell map[string]SchedulerCheckpoint, result ScheduleExecution, err error) (ScheduleExecution, error) {
	result.Checkpoints = make([]SchedulerCheckpoint, 0, len(checkpointByCell))
	for _, checkpoint := range checkpointByCell {
		result.Checkpoints = append(result.Checkpoints, checkpoint)
	}
	sort.Slice(result.Checkpoints, func(i, j int) bool { return result.Checkpoints[i].CellID < result.Checkpoints[j].CellID })
	sort.Strings(result.CompletedUnitIDs)
	return result, err
}
