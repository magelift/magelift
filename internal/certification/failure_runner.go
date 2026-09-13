package certification

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// FailureScenarioState is the durable progress marker for one controlled
// failure exercise. Injection is persisted before observation so an
// interrupted run can recover the same fault scope instead of injecting a
// second failure into the same runtime.
type FailureScenarioState string

const (
	FailureScenarioInjected                FailureScenarioState = "injected"
	FailureScenarioInjectionCleanupPending FailureScenarioState = "injection-cleanup-pending"
	FailureScenarioObserved                FailureScenarioState = "observed"
	FailureScenarioCleanupPending          FailureScenarioState = "cleanup-pending"
	FailureScenarioComplete                FailureScenarioState = "complete"
)

// FailureInjection identifies the provider-owned fault and the resources it
// touched. The values are opaque identities; provider credentials and fault
// payloads must never be stored here.
type FailureInjection struct {
	ID            string   `json:"id" yaml:"id"`
	OperationRefs []string `json:"operationRefs,omitempty" yaml:"operationRefs,omitempty"`
	ResourceRefs  []string `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	Verified      bool     `json:"verified" yaml:"verified"`
}

func (injection FailureInjection) Validate() error {
	if strings.TrimSpace(injection.ID) == "" || strings.ContainsAny(injection.ID, "\r\n\x00") {
		return errors.New("failure injection ID is required and must be single-line")
	}
	if err := validateFailureReferences("failure injection operation reference", injection.OperationRefs); err != nil {
		return err
	}
	if err := validateFailureReferences("failure injection resource reference", injection.ResourceRefs); err != nil {
		return err
	}
	if !injection.Verified {
		return errors.New("failure injection must verify that the requested fault was applied")
	}
	return nil
}

// FailureScenarioObservation is the provider-neutral result of observing the
// runtime while the injected fault is active. Provider adapters translate
// CloudWatch, Kubernetes, Fastly, or provider API facts into these booleans;
// the release gate remains shared.
type FailureScenarioObservation struct {
	TrafficHealthVerified  bool   `json:"trafficHealthVerified" yaml:"trafficHealthVerified"`
	QueueHealthVerified    bool   `json:"queueHealthVerified" yaml:"queueHealthVerified"`
	DatabaseHealthVerified bool   `json:"databaseHealthVerified" yaml:"databaseHealthVerified"`
	CacheLossClassified    bool   `json:"cacheLossClassified" yaml:"cacheLossClassified"`
	IntegrityVerified      bool   `json:"integrityVerified" yaml:"integrityVerified"`
	FencingVerified        bool   `json:"fencingVerified" yaml:"fencingVerified"`
	MeasuredRPOSeconds     int64  `json:"measuredRpoSeconds,omitempty" yaml:"measuredRpoSeconds,omitempty"`
	MeasuredRTOSeconds     int64  `json:"measuredRtoSeconds,omitempty" yaml:"measuredRtoSeconds,omitempty"`
	OperatorAction         string `json:"operatorAction,omitempty" yaml:"operatorAction,omitempty"`
	Reason                 string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

func (observation FailureScenarioObservation) Validate() error {
	if observation.MeasuredRPOSeconds < 0 || observation.MeasuredRTOSeconds < 0 {
		return errors.New("failure scenario observation contains a negative recovery measurement")
	}
	if strings.ContainsAny(observation.OperatorAction+observation.Reason, "\r\n\x00") {
		return errors.New("failure scenario observation contains a control character")
	}
	return nil
}

// FailureCleanupObservation is independent cleanup evidence for a failure
// drill. A passing traffic or integrity observation never implies that the
// injected fault resources were removed.
type FailureCleanupObservation struct {
	Complete         bool     `json:"complete" yaml:"complete"`
	UnownedPreserved bool     `json:"unownedPreserved" yaml:"unownedPreserved"`
	ResourceRefs     []string `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	Reason           string   `json:"reason,omitempty" yaml:"reason,omitempty"`
}

func (observation FailureCleanupObservation) Validate() error {
	if err := validateFailureReferences("failure cleanup resource reference", observation.ResourceRefs); err != nil {
		return err
	}
	if strings.ContainsAny(observation.Reason, "\r\n\x00") {
		return errors.New("failure cleanup observation reason contains a control character")
	}
	return nil
}

// FailureScenarioCheckpoint is the durable state needed to resume one
// failure exercise. MatrixFingerprint binds it to the exact architecture;
// OwnershipMarker prevents a checkpoint from being reused for another
// resource scope.
type FailureScenarioCheckpoint struct {
	MatrixFingerprint string                      `json:"matrixFingerprint" yaml:"matrixFingerprint"`
	ScenarioID        string                      `json:"scenarioId" yaml:"scenarioId"`
	OwnershipMarker   string                      `json:"ownershipMarker" yaml:"ownershipMarker"`
	State             FailureScenarioState        `json:"state" yaml:"state"`
	Injection         FailureInjection            `json:"injection" yaml:"injection"`
	Observation       *FailureScenarioObservation `json:"observation,omitempty" yaml:"observation,omitempty"`
	Proof             *FailureScenarioProof       `json:"proof,omitempty" yaml:"proof,omitempty"`
	Cleanup           FailureCleanupObservation   `json:"cleanup" yaml:"cleanup"`
	UpdatedAt         string                      `json:"updatedAt" yaml:"updatedAt"`
}

func (checkpoint FailureScenarioCheckpoint) Validate() error {
	if err := ValidateSecretSafeValue(checkpoint); err != nil {
		return errors.New("failure checkpoint contains secret-like material")
	}
	if strings.TrimSpace(checkpoint.MatrixFingerprint) == "" || strings.ContainsAny(checkpoint.MatrixFingerprint, "\r\n\x00") {
		return errors.New("failure checkpoint matrix fingerprint is required and must be single-line")
	}
	if strings.TrimSpace(checkpoint.ScenarioID) == "" || strings.ContainsAny(checkpoint.ScenarioID, "\r\n\x00") {
		return errors.New("failure checkpoint scenario ID is required and must be single-line")
	}
	if strings.TrimSpace(checkpoint.OwnershipMarker) == "" || strings.ContainsAny(checkpoint.OwnershipMarker, "\r\n\x00") {
		return errors.New("failure checkpoint ownership marker is required and must be single-line")
	}
	if strings.TrimSpace(checkpoint.UpdatedAt) == "" || strings.ContainsAny(checkpoint.UpdatedAt, "\r\n\x00") {
		return errors.New("failure checkpoint update time is required and must be single-line")
	}
	if strings.TrimSpace(checkpoint.Injection.ID) != "" {
		if err := checkpoint.Injection.Validate(); err != nil {
			return err
		}
	}
	if checkpoint.Observation != nil {
		if err := checkpoint.Observation.Validate(); err != nil {
			return err
		}
	}
	if err := checkpoint.Cleanup.Validate(); err != nil {
		return err
	}
	switch checkpoint.State {
	case FailureScenarioInjected:
		if err := checkpoint.Injection.Validate(); err != nil {
			return err
		}
	case FailureScenarioInjectionCleanupPending:
		if err := checkpoint.Injection.Validate(); err != nil {
			return err
		}
	case FailureScenarioObserved:
		if err := checkpoint.Injection.Validate(); err != nil {
			return err
		}
		if checkpoint.Observation == nil || checkpoint.Proof == nil {
			return errors.New("observed failure checkpoint requires an observation and proof")
		}
	case FailureScenarioCleanupPending:
		if err := checkpoint.Injection.Validate(); err != nil {
			return err
		}
		if checkpoint.Observation == nil || checkpoint.Proof == nil {
			return errors.New("cleanup-pending failure checkpoint requires an observation and proof")
		}
	case FailureScenarioComplete:
		if checkpoint.Observation == nil || checkpoint.Proof == nil {
			return errors.New("complete failure checkpoint requires an observation and proof")
		}
		if checkpoint.Proof.Status != StatusSkip && checkpoint.Proof.Status != StatusBlocked && checkpoint.Proof.Status != StatusNotRun {
			if err := checkpoint.Injection.Validate(); err != nil {
				return err
			}
		}
		if !checkpoint.Cleanup.Complete || !checkpoint.Cleanup.UnownedPreserved {
			return errors.New("complete failure checkpoint requires verified cleanup")
		}
	default:
		return fmt.Errorf("invalid failure checkpoint state %q", checkpoint.State)
	}
	return nil
}

// FailureScenarioCheckpointStore is the only persistence dependency of the
// runner. A live implementation should use the same durable session backend
// as certification checkpoints; the memory implementation is intentionally
// limited to deterministic tests.
type FailureScenarioCheckpointStore interface {
	Load(context.Context) (FailureScenarioCheckpoint, error)
	Save(context.Context, FailureScenarioCheckpoint) error
}

var ErrFailureCheckpointNotFound = errors.New("failure scenario checkpoint not found")

// FailureScenarioExecutor is the provider-specific fault-injection boundary.
// It owns the native API calls and translates their responses; the core owns
// resume, proof construction, cleanup ordering, and status semantics.
type FailureScenarioExecutor interface {
	Inject(context.Context, ResilienceScenario) (FailureInjection, error)
	Observe(context.Context, ResilienceScenario, FailureInjection) (FailureScenarioObservation, error)
	Cleanup(context.Context, ResilienceScenario, FailureInjection) (FailureCleanupObservation, error)
}

type FailureScenarioRun struct {
	Scenario    ResilienceScenario
	Injection   FailureInjection
	Observation FailureScenarioObservation
	Proof       FailureScenarioProof
	Cleanup     FailureCleanupObservation
	Reused      bool
}

// RunFailureScenario executes or resumes one controlled failure exercise.
// Unsupported scenarios are checkpointed as explicit SKIP results without
// invoking the provider executor. Expected provider failure outcomes are
// returned as proof with StatusFail; executor and persistence failures return
// an error. If persistence fails after injection, the runner first attempts
// cleanup and records either a blocked terminal checkpoint or an explicit
// pre-observation cleanup-pending checkpoint so a retry cannot inject a
// second fault silently.
func RunFailureScenario(ctx context.Context, scenario ResilienceScenario, matrixFingerprint, ownershipMarker string, executor FailureScenarioExecutor, store FailureScenarioCheckpointStore, now func() time.Time) (FailureScenarioRun, error) {
	if ctx == nil {
		return FailureScenarioRun{}, errors.New("failure scenario context is required")
	}
	if strings.TrimSpace(matrixFingerprint) == "" || strings.ContainsAny(matrixFingerprint, "\r\n\x00") {
		return FailureScenarioRun{}, errors.New("failure scenario matrix fingerprint is required and must be single-line")
	}
	if strings.TrimSpace(ownershipMarker) == "" || strings.ContainsAny(ownershipMarker, "\r\n\x00") {
		return FailureScenarioRun{}, errors.New("failure scenario ownership marker is required and must be single-line")
	}
	if store == nil {
		return FailureScenarioRun{}, errors.New("failure scenario checkpoint store is required")
	}
	if now == nil {
		now = time.Now
	}
	if now().IsZero() {
		return FailureScenarioRun{}, errors.New("failure scenario clock must return a non-zero time")
	}
	if err := ValidateResilienceScenarioMatrix([]ResilienceScenario{scenario}); err != nil {
		return FailureScenarioRun{}, err
	}
	if scenario.Mode != ScenarioUnsupported && executor == nil {
		return FailureScenarioRun{}, errors.New("failure scenario executor is required")
	}

	checkpoint, err := store.Load(ctx)
	if err != nil && !errors.Is(err, ErrFailureCheckpointNotFound) {
		return FailureScenarioRun{}, fmt.Errorf("load failure scenario checkpoint: %w", err)
	}
	if err == nil {
		if err := checkpoint.Validate(); err != nil {
			return FailureScenarioRun{}, fmt.Errorf("validate failure scenario checkpoint: %w", err)
		}
		if checkpoint.MatrixFingerprint != matrixFingerprint || checkpoint.ScenarioID != scenario.ID || checkpoint.OwnershipMarker != ownershipMarker {
			return FailureScenarioRun{}, errors.New("failure scenario checkpoint does not match the requested architecture, scenario, or ownership scope")
		}
		if checkpoint.State == FailureScenarioComplete {
			return failureRunFromCheckpoint(scenario, checkpoint, true), nil
		}
	}

	if scenario.Mode == ScenarioUnsupported {
		proof := FailureScenarioProof{ScenarioID: scenario.ID, Status: StatusSkip, Reason: scenario.Reason}
		if err := ValidateFailureScenarioProof(scenario, proof); err != nil {
			return FailureScenarioRun{}, err
		}
		checkpoint = FailureScenarioCheckpoint{MatrixFingerprint: matrixFingerprint, ScenarioID: scenario.ID, OwnershipMarker: ownershipMarker, State: FailureScenarioComplete, Observation: &FailureScenarioObservation{}, Proof: &proof, UpdatedAt: now().UTC().Format(time.RFC3339Nano), Cleanup: FailureCleanupObservation{Complete: true, UnownedPreserved: true}}
		if err := saveFailureCheckpoint(ctx, store, checkpoint); err != nil {
			return FailureScenarioRun{}, fmt.Errorf("save unsupported failure scenario checkpoint: %w", err)
		}
		return FailureScenarioRun{Scenario: scenario, Proof: proof, Cleanup: checkpoint.Cleanup}, nil
	}

	if checkpoint.State == FailureScenarioObserved || checkpoint.State == FailureScenarioCleanupPending || checkpoint.State == FailureScenarioInjectionCleanupPending {
		if checkpoint.State != FailureScenarioInjectionCleanupPending && (checkpoint.Observation == nil || checkpoint.Proof == nil) {
			return FailureScenarioRun{}, errors.New("failure scenario checkpoint cannot resume cleanup without an observation and proof")
		}
		cleanup, cleanupErr := executor.Cleanup(ctx, scenario, checkpoint.Injection)
		if cleanupErr != nil {
			if checkpoint.State != FailureScenarioInjectionCleanupPending {
				checkpoint.State = FailureScenarioCleanupPending
			}
			checkpoint.Cleanup = cloneFailureCleanup(cleanup)
			checkpoint.UpdatedAt = now().UTC().Format(time.RFC3339Nano)
			saveErr := saveFailureCheckpoint(ctx, store, checkpoint)
			return failureRunFromCheckpoint(scenario, checkpoint, false), errors.Join(fmt.Errorf("cleanup failure scenario %q: %w", scenario.ID, cleanupErr), saveErr)
		}
		if err := cleanup.Validate(); err != nil {
			return FailureScenarioRun{}, fmt.Errorf("validate cleanup for failure scenario %q: %w", scenario.ID, err)
		}
		checkpoint.Cleanup = cloneFailureCleanup(cleanup)
		if !cleanup.Complete || !cleanup.UnownedPreserved {
			if checkpoint.State != FailureScenarioInjectionCleanupPending {
				checkpoint.State = FailureScenarioCleanupPending
			}
			checkpoint.UpdatedAt = now().UTC().Format(time.RFC3339Nano)
			if saveErr := saveFailureCheckpoint(ctx, store, checkpoint); saveErr != nil {
				return FailureScenarioRun{}, fmt.Errorf("save incomplete failure cleanup: %w", saveErr)
			}
			return failureRunFromCheckpoint(scenario, checkpoint, false), nil
		}
		if checkpoint.State == FailureScenarioInjectionCleanupPending {
			proof := failureScenarioCleanupBeforeObservationProof(scenario)
			checkpoint.Observation = &FailureScenarioObservation{}
			checkpoint.Proof = &proof
		}
		checkpoint.State = FailureScenarioComplete
		checkpoint.UpdatedAt = now().UTC().Format(time.RFC3339Nano)
		if saveErr := saveFailureCheckpoint(ctx, store, checkpoint); saveErr != nil {
			return FailureScenarioRun{}, fmt.Errorf("save completed failure scenario checkpoint: %w", saveErr)
		}
		return failureRunFromCheckpoint(scenario, checkpoint, false), nil
	}

	if checkpoint.State == "" {
		injection, injectErr := executor.Inject(ctx, scenario)
		if injectErr != nil {
			return FailureScenarioRun{}, fmt.Errorf("inject failure scenario %q: %w", scenario.ID, injectErr)
		}
		if err := injection.Validate(); err != nil {
			return FailureScenarioRun{}, fmt.Errorf("validate failure injection for %q: %w", scenario.ID, err)
		}
		checkpoint = FailureScenarioCheckpoint{MatrixFingerprint: matrixFingerprint, ScenarioID: scenario.ID, OwnershipMarker: ownershipMarker, State: FailureScenarioInjected, Injection: cloneFailureInjection(injection), UpdatedAt: now().UTC().Format(time.RFC3339Nano), Cleanup: FailureCleanupObservation{UnownedPreserved: true}}
		if err := saveFailureCheckpoint(ctx, store, checkpoint); err != nil {
			return recoverFailureScenarioAfterCheckpointError(ctx, scenario, matrixFingerprint, ownershipMarker, executor, store, now, checkpoint, nil, nil, fmt.Errorf("save injected failure scenario checkpoint: %w", err))
		}
	}

	if err := ctx.Err(); err != nil {
		return FailureScenarioRun{}, err
	}
	observation, observeErr := executor.Observe(ctx, scenario, checkpoint.Injection)
	if observeErr != nil {
		return FailureScenarioRun{}, fmt.Errorf("observe failure scenario %q: %w", scenario.ID, observeErr)
	}
	if err := observation.Validate(); err != nil {
		return FailureScenarioRun{}, fmt.Errorf("validate failure observation for %q: %w", scenario.ID, err)
	}
	proof := failureProof(scenario, observation)
	if err := ValidateFailureScenarioProof(scenario, proof); err != nil {
		return FailureScenarioRun{}, err
	}
	checkpoint.State = FailureScenarioObserved
	checkpoint.Observation = cloneFailureObservation(observation)
	checkpoint.Proof = &proof
	checkpoint.UpdatedAt = now().UTC().Format(time.RFC3339Nano)
	if err := saveFailureCheckpoint(ctx, store, checkpoint); err != nil {
		return recoverFailureScenarioAfterCheckpointError(ctx, scenario, matrixFingerprint, ownershipMarker, executor, store, now, checkpoint, &observation, &proof, fmt.Errorf("save observed failure scenario checkpoint: %w", err))
	}

	cleanup, cleanupErr := executor.Cleanup(ctx, scenario, checkpoint.Injection)
	if cleanupErr != nil {
		checkpoint.State = FailureScenarioCleanupPending
		checkpoint.Cleanup = cloneFailureCleanup(cleanup)
		checkpoint.UpdatedAt = now().UTC().Format(time.RFC3339Nano)
		saveErr := saveFailureCheckpoint(ctx, store, checkpoint)
		return FailureScenarioRun{Scenario: scenario, Injection: checkpoint.Injection, Observation: observation, Proof: proof, Cleanup: cleanup}, errors.Join(fmt.Errorf("cleanup failure scenario %q: %w", scenario.ID, cleanupErr), saveErr)
	}
	if err := cleanup.Validate(); err != nil {
		return FailureScenarioRun{}, fmt.Errorf("validate cleanup for failure scenario %q: %w", scenario.ID, err)
	}
	if !cleanup.Complete || !cleanup.UnownedPreserved {
		checkpoint.State = FailureScenarioCleanupPending
		checkpoint.Cleanup = cloneFailureCleanup(cleanup)
		checkpoint.UpdatedAt = now().UTC().Format(time.RFC3339Nano)
		if saveErr := saveFailureCheckpoint(ctx, store, checkpoint); saveErr != nil {
			return FailureScenarioRun{}, fmt.Errorf("save incomplete failure cleanup: %w", saveErr)
		}
		return FailureScenarioRun{Scenario: scenario, Injection: checkpoint.Injection, Observation: observation, Proof: proof, Cleanup: cleanup}, nil
	}
	checkpoint.State = FailureScenarioComplete
	checkpoint.Cleanup = cloneFailureCleanup(cleanup)
	checkpoint.UpdatedAt = now().UTC().Format(time.RFC3339Nano)
	if err := saveFailureCheckpoint(ctx, store, checkpoint); err != nil {
		return FailureScenarioRun{}, fmt.Errorf("save completed failure scenario checkpoint: %w", err)
	}
	return FailureScenarioRun{Scenario: scenario, Injection: checkpoint.Injection, Observation: observation, Proof: proof, Cleanup: cleanup}, nil
}

func failureProof(scenario ResilienceScenario, observation FailureScenarioObservation) FailureScenarioProof {
	status := StatusPass
	reason := strings.TrimSpace(observation.Reason)
	if scenario.TrafficHealthRequired && !observation.TrafficHealthVerified {
		reason = joinFailureReason(reason, "traffic health was not verified")
	}
	if scenario.QueueHealthRequired && !observation.QueueHealthVerified {
		reason = joinFailureReason(reason, "queue health was not verified")
	}
	if scenario.DatabaseHealthRequired && !observation.DatabaseHealthVerified {
		reason = joinFailureReason(reason, "database health was not verified")
	}
	if scenario.CacheLossRequired && !observation.CacheLossClassified {
		reason = joinFailureReason(reason, "cache loss was not classified")
	}
	if scenario.IntegrityRequired && !observation.IntegrityVerified {
		reason = joinFailureReason(reason, "data integrity was not verified")
	}
	if scenario.FencingRequired && !observation.FencingVerified {
		reason = joinFailureReason(reason, "fencing was not verified")
	}
	if scenario.Mode == ScenarioOperatorAssisted && strings.TrimSpace(observation.OperatorAction) == "" {
		reason = joinFailureReason(reason, "operator action was not recorded")
	}
	if reason != "" {
		status = StatusFail
	}
	return FailureScenarioProof{ScenarioID: scenario.ID, Status: status, InjectionVerified: true, TrafficHealthVerified: observation.TrafficHealthVerified, QueueHealthVerified: observation.QueueHealthVerified, DatabaseHealthVerified: observation.DatabaseHealthVerified, CacheLossClassified: observation.CacheLossClassified, IntegrityVerified: observation.IntegrityVerified, FencingVerified: observation.FencingVerified, MeasuredRPOSeconds: observation.MeasuredRPOSeconds, MeasuredRTOSeconds: observation.MeasuredRTOSeconds, OperatorAction: observation.OperatorAction, Reason: reason}
}

const failureCheckpointSaveTimeout = 30 * time.Second

const failureCheckpointRecoveryTimeout = 30 * time.Second

func saveFailureCheckpoint(ctx context.Context, store FailureScenarioCheckpointStore, checkpoint FailureScenarioCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return fmt.Errorf("validate failure checkpoint before save: %w", err)
	}
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), failureCheckpointSaveTimeout)
	defer cancel()
	return store.Save(saveCtx, checkpoint)
}

func recoverFailureScenarioAfterCheckpointError(ctx context.Context, scenario ResilienceScenario, matrixFingerprint, ownershipMarker string, executor FailureScenarioExecutor, store FailureScenarioCheckpointStore, now func() time.Time, checkpoint FailureScenarioCheckpoint, observation *FailureScenarioObservation, proof *FailureScenarioProof, persistenceErr error) (FailureScenarioRun, error) {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), failureCheckpointRecoveryTimeout)
	defer cancel()

	cleanup, cleanupErr := executor.Cleanup(cleanupCtx, scenario, checkpoint.Injection)
	cleanup = cloneFailureCleanup(cleanup)
	if err := cleanup.Validate(); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("validate cleanup after checkpoint persistence failure for %q: %w", scenario.ID, err))
	}

	checkpoint.MatrixFingerprint = matrixFingerprint
	checkpoint.ScenarioID = scenario.ID
	checkpoint.OwnershipMarker = ownershipMarker
	checkpoint.Cleanup = cleanup
	checkpoint.UpdatedAt = now().UTC().Format(time.RFC3339Nano)
	if observation != nil {
		checkpoint.Observation = cloneFailureObservation(*observation)
	}
	if proof != nil {
		proofCopy := *proof
		checkpoint.Proof = &proofCopy
	}

	cleanupComplete := cleanupErr == nil && cleanup.Complete && cleanup.UnownedPreserved
	if cleanupComplete {
		if checkpoint.Observation == nil || checkpoint.Proof == nil {
			blockedProof := failureScenarioCleanupBeforeObservationProof(scenario)
			checkpoint.Observation = &FailureScenarioObservation{}
			checkpoint.Proof = &blockedProof
		}
		checkpoint.State = FailureScenarioComplete
	} else if checkpoint.Observation == nil || checkpoint.Proof == nil {
		checkpoint.State = FailureScenarioInjectionCleanupPending
	} else {
		checkpoint.State = FailureScenarioCleanupPending
	}

	saveErr := saveFailureCheckpoint(ctx, store, checkpoint)
	run := failureRunFromCheckpoint(scenario, checkpoint, false)
	errs := []error{persistenceErr}
	if cleanupErr != nil {
		errs = append(errs, fmt.Errorf("cleanup after checkpoint persistence failure for %q: %w", scenario.ID, cleanupErr))
	} else if !cleanup.Complete || !cleanup.UnownedPreserved {
		errs = append(errs, fmt.Errorf("cleanup after checkpoint persistence failure for %q is incomplete", scenario.ID))
	}
	if saveErr != nil {
		errs = append(errs, fmt.Errorf("save failure scenario recovery checkpoint: %w", saveErr))
	}
	return run, errors.Join(errs...)
}

func failureScenarioCleanupBeforeObservationProof(scenario ResilienceScenario) FailureScenarioProof {
	return FailureScenarioProof{
		ScenarioID: scenario.ID,
		Status:     StatusBlocked,
		Reason:     "checkpoint persistence failed after fault injection; cleanup completed before observation",
	}
}

func joinFailureReason(current, next string) string {
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}
	return current + "; " + next
}

func failureRunFromCheckpoint(scenario ResilienceScenario, checkpoint FailureScenarioCheckpoint, reused bool) FailureScenarioRun {
	var observation FailureScenarioObservation
	if checkpoint.Observation != nil {
		observation = *checkpoint.Observation
	}
	var proof FailureScenarioProof
	if checkpoint.Proof != nil {
		proof = *checkpoint.Proof
	}
	return FailureScenarioRun{Scenario: scenario, Injection: cloneFailureInjection(checkpoint.Injection), Observation: observation, Proof: proof, Cleanup: cloneFailureCleanup(checkpoint.Cleanup), Reused: reused}
}

func cloneFailureInjection(injection FailureInjection) FailureInjection {
	injection.OperationRefs = append([]string(nil), injection.OperationRefs...)
	injection.ResourceRefs = append([]string(nil), injection.ResourceRefs...)
	return injection
}

func cloneFailureObservation(observation FailureScenarioObservation) *FailureScenarioObservation {
	return &observation
}

func cloneFailureCleanup(observation FailureCleanupObservation) FailureCleanupObservation {
	observation.ResourceRefs = append([]string(nil), observation.ResourceRefs...)
	sort.Strings(observation.ResourceRefs)
	return observation
}

func validateFailureReferences(label string, references []string) error {
	seen := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if strings.TrimSpace(reference) == "" || strings.ContainsAny(reference, "\r\n\x00") {
			return fmt.Errorf("%s must be non-empty and single-line", label)
		}
		if _, exists := seen[reference]; exists {
			return fmt.Errorf("%s %q is duplicated", label, reference)
		}
		seen[reference] = struct{}{}
	}
	return nil
}
