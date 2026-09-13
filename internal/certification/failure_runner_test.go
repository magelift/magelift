package certification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/magelift/magelift/sdk/v1"
)

type memoryFailureCheckpoint struct {
	value FailureScenarioCheckpoint
	found bool
	saves int
}

func (store *memoryFailureCheckpoint) Load(context.Context) (FailureScenarioCheckpoint, error) {
	if !store.found {
		return FailureScenarioCheckpoint{}, ErrFailureCheckpointNotFound
	}
	return store.value, nil
}

func (store *memoryFailureCheckpoint) Save(_ context.Context, checkpoint FailureScenarioCheckpoint) error {
	if err := checkpoint.Validate(); err != nil {
		return err
	}
	store.value = checkpoint
	store.found = true
	store.saves++
	return nil
}

type failureExecutor struct {
	injections  int
	observes    int
	cleanups    int
	injection   FailureInjection
	observation FailureScenarioObservation
	cleanup     FailureCleanupObservation
	observeErr  error
	cleanupErr  error
	afterInject func()
}

func (executor *failureExecutor) Inject(context.Context, ResilienceScenario) (FailureInjection, error) {
	executor.injections++
	if executor.afterInject != nil {
		executor.afterInject()
	}
	if executor.injection.ID != "" {
		return executor.injection, nil
	}
	return FailureInjection{ID: "fault-1", OperationRefs: []string{"operation:fault-1"}, ResourceRefs: []string{"resource:fault-1"}, Verified: true}, nil
}

func (executor *failureExecutor) Observe(context.Context, ResilienceScenario, FailureInjection) (FailureScenarioObservation, error) {
	executor.observes++
	if executor.observeErr != nil {
		return FailureScenarioObservation{}, executor.observeErr
	}
	return executor.observation, nil
}

func (executor *failureExecutor) Cleanup(context.Context, ResilienceScenario, FailureInjection) (FailureCleanupObservation, error) {
	executor.cleanups++
	return executor.cleanup, executor.cleanupErr
}

func TestRunFailureScenarioPersistsAndReusesCompleteExercise(t *testing.T) {
	scenario := testFailureScenario()
	executor := &failureExecutor{
		observation: FailureScenarioObservation{TrafficHealthVerified: true, IntegrityVerified: true, FencingVerified: true, MeasuredRTOSeconds: 12},
		cleanup:     FailureCleanupObservation{Complete: true, UnownedPreserved: true},
	}
	store := &memoryFailureCheckpoint{}
	now := func() time.Time { return time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC) }

	run, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, now)
	if err != nil {
		t.Fatalf("first failure run: %v", err)
	}
	if run.Proof.Status != StatusPass || run.Reused {
		t.Fatalf("first failure run = %#v", run)
	}
	if executor.injections != 1 || executor.observes != 1 || executor.cleanups != 1 {
		t.Fatalf("first executor calls = injections %d, observes %d, cleanups %d", executor.injections, executor.observes, executor.cleanups)
	}
	if store.value.State != FailureScenarioComplete {
		t.Fatalf("checkpoint state = %q", store.value.State)
	}

	reused, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, now)
	if err != nil {
		t.Fatalf("reused failure run: %v", err)
	}
	if !reused.Reused || reused.Proof.Status != StatusPass {
		t.Fatalf("reused failure run = %#v", reused)
	}
	if executor.injections != 1 || executor.observes != 1 || executor.cleanups != 1 {
		t.Fatalf("reused executor calls = injections %d, observes %d, cleanups %d", executor.injections, executor.observes, executor.cleanups)
	}
}

func TestRunFailureScenarioResumesCleanupWithoutRepeatingFaultInjection(t *testing.T) {
	scenario := testFailureScenario()
	executor := &failureExecutor{
		observation: FailureScenarioObservation{TrafficHealthVerified: true, IntegrityVerified: true, FencingVerified: true},
		cleanup:     FailureCleanupObservation{Complete: false, UnownedPreserved: true, ResourceRefs: []string{"resource:fault-1"}},
	}
	store := &memoryFailureCheckpoint{}
	now := func() time.Time { return time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC) }

	first, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, now)
	if err != nil {
		t.Fatalf("first cleanup-pending run: %v", err)
	}
	if first.Proof.Status != StatusPass || first.Cleanup.Complete || store.value.State != FailureScenarioCleanupPending {
		t.Fatalf("first cleanup-pending run = %#v, checkpoint = %#v", first, store.value)
	}

	executor.cleanup = FailureCleanupObservation{Complete: true, UnownedPreserved: true}
	second, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, now)
	if err != nil {
		t.Fatalf("resumed cleanup run: %v", err)
	}
	if second.Reused || !second.Cleanup.Complete || store.value.State != FailureScenarioComplete {
		t.Fatalf("resumed cleanup run = %#v, checkpoint = %#v", second, store.value)
	}
	if executor.injections != 1 || executor.observes != 1 || executor.cleanups != 2 {
		t.Fatalf("resumed executor calls = injections %d, observes %d, cleanups %d", executor.injections, executor.observes, executor.cleanups)
	}
}

func TestRunFailureScenarioPreservesInjectedCheckpointWhenObservationFails(t *testing.T) {
	scenario := testFailureScenario()
	executor := &failureExecutor{observeErr: errors.New("provider fault probe unavailable"), cleanup: FailureCleanupObservation{Complete: true, UnownedPreserved: true}}
	store := &memoryFailureCheckpoint{}
	now := func() time.Time { return time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC) }

	if _, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, now); err == nil {
		t.Fatal("observation failure unexpectedly passed")
	}
	if store.value.State != FailureScenarioInjected || executor.injections != 1 {
		t.Fatalf("observation failure did not preserve injection checkpoint: state=%q injections=%d", store.value.State, executor.injections)
	}

	executor.observeErr = nil
	executor.observation = FailureScenarioObservation{TrafficHealthVerified: true, IntegrityVerified: true, FencingVerified: true}
	run, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, now)
	if err != nil {
		t.Fatalf("resume after observation failure: %v", err)
	}
	if run.Proof.Status != StatusPass || executor.injections != 1 {
		t.Fatalf("resume after observation failure = %#v, injections=%d", run, executor.injections)
	}
}

func TestRunFailureScenarioCleansUpWhenInjectedCheckpointCannotBePersisted(t *testing.T) {
	scenario := testFailureScenario()
	executor := &failureExecutor{
		observation: FailureScenarioObservation{TrafficHealthVerified: true, IntegrityVerified: true, FencingVerified: true},
		cleanup:     FailureCleanupObservation{Complete: true, UnownedPreserved: true},
	}
	store := &failingAfterFailureCheckpointStore{failAt: 1, err: errors.New("checkpoint backend unavailable")}

	run, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now)
	if err == nil || !strings.Contains(err.Error(), "checkpoint backend unavailable") {
		t.Fatalf("injected checkpoint persistence error = %v, want checkpoint error", err)
	}
	if run.Proof.Status != StatusBlocked || !run.Cleanup.Complete || !run.Cleanup.UnownedPreserved {
		t.Fatalf("recovered injected checkpoint failure = %#v, want blocked proof with complete cleanup", run)
	}
	if store.value.State != FailureScenarioComplete || executor.injections != 1 || executor.observes != 0 || executor.cleanups != 1 {
		t.Fatalf("recovered injected checkpoint failure state/calls = state=%q injections=%d observes=%d cleanups=%d", store.value.State, executor.injections, executor.observes, executor.cleanups)
	}

	reused, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now)
	if err != nil {
		t.Fatalf("reusing quarantined checkpoint: %v", err)
	}
	if !reused.Reused || reused.Proof.Status != StatusBlocked || executor.injections != 1 || executor.observes != 0 || executor.cleanups != 1 {
		t.Fatalf("reused quarantined checkpoint = %#v, calls injections=%d observes=%d cleanups=%d", reused, executor.injections, executor.observes, executor.cleanups)
	}
}

func TestRunFailureScenarioPersistsPreObservationCleanupPendingState(t *testing.T) {
	scenario := testFailureScenario()
	executor := &failureExecutor{
		cleanup: FailureCleanupObservation{Complete: false, UnownedPreserved: true, ResourceRefs: []string{"resource:fault-1"}},
	}
	store := &failingAfterFailureCheckpointStore{failAt: 1, err: errors.New("checkpoint backend unavailable")}

	first, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now)
	if err == nil || !strings.Contains(err.Error(), "checkpoint backend unavailable") {
		t.Fatalf("pre-observation cleanup error = %v, want checkpoint error", err)
	}
	if store.value.State != FailureScenarioInjectionCleanupPending || first.Cleanup.Complete || executor.injections != 1 || executor.observes != 0 || executor.cleanups != 1 {
		t.Fatalf("pre-observation cleanup checkpoint = %#v, state=%q calls injections=%d observes=%d cleanups=%d", first, store.value.State, executor.injections, executor.observes, executor.cleanups)
	}

	executor.cleanup = FailureCleanupObservation{Complete: true, UnownedPreserved: true}
	second, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now)
	if err != nil {
		t.Fatalf("resume pre-observation cleanup: %v", err)
	}
	if second.Proof.Status != StatusBlocked || !second.Cleanup.Complete || store.value.State != FailureScenarioComplete || executor.injections != 1 || executor.observes != 0 || executor.cleanups != 2 {
		t.Fatalf("resumed pre-observation cleanup = %#v, state=%q calls injections=%d observes=%d cleanups=%d", second, store.value.State, executor.injections, executor.observes, executor.cleanups)
	}
}

func TestRunFailureScenarioQuarantinesObservedCheckpointWhenObservationPersistenceFails(t *testing.T) {
	scenario := testFailureScenario()
	executor := &failureExecutor{
		observation: FailureScenarioObservation{TrafficHealthVerified: true, IntegrityVerified: true, FencingVerified: true},
		cleanup:     FailureCleanupObservation{Complete: true, UnownedPreserved: true},
	}
	store := &failingAfterFailureCheckpointStore{failAt: 2, err: errors.New("checkpoint backend unavailable")}

	run, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now)
	if err == nil || !strings.Contains(err.Error(), "checkpoint backend unavailable") {
		t.Fatalf("observed checkpoint persistence error = %v, want checkpoint error", err)
	}
	if run.Proof.Status != StatusPass || !run.Cleanup.Complete || store.value.State != FailureScenarioComplete {
		t.Fatalf("quarantined observed checkpoint = %#v, state=%q", run, store.value.State)
	}
	if executor.injections != 1 || executor.observes != 1 || executor.cleanups != 1 {
		t.Fatalf("quarantined observed checkpoint calls injections=%d observes=%d cleanups=%d", executor.injections, executor.observes, executor.cleanups)
	}

	reused, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now)
	if err != nil {
		t.Fatalf("reusing quarantined observed checkpoint: %v", err)
	}
	if !reused.Reused || reused.Proof.Status != StatusPass || executor.injections != 1 || executor.observes != 1 || executor.cleanups != 1 {
		t.Fatalf("reused quarantined observed checkpoint = %#v, calls injections=%d observes=%d cleanups=%d", reused, executor.injections, executor.observes, executor.cleanups)
	}
}

func TestRunFailureScenarioSkipsUnsupportedWithoutExecutor(t *testing.T) {
	scenario := testFailureScenario()
	scenario.Mode = ScenarioUnsupported
	scenario.Reason = "provider has no safe fault injector"
	store := &memoryFailureCheckpoint{}

	run, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", nil, store, time.Now)
	if err != nil {
		t.Fatalf("unsupported failure run: %v", err)
	}
	if run.Proof.Status != StatusSkip || store.value.State != FailureScenarioComplete {
		t.Fatalf("unsupported failure run = %#v, checkpoint = %#v", run, store.value)
	}
	if _, err := RunFailureScenario(context.Background(), scenario, "different-digest", "marker-1", nil, store, time.Now); err == nil {
		t.Fatal("mismatched unsupported checkpoint unexpectedly reused")
	}
}

func TestRunFailureScenarioRejectsSecretLikeInjectedIdentityBeforeCheckpoint(t *testing.T) {
	scenario := testFailureScenario()
	executor := &failureExecutor{
		injection: FailureInjection{ID: "token=abcdefghijkl", OperationRefs: []string{"operation:fault-1"}, ResourceRefs: []string{"resource:fault-1"}, Verified: true},
		cleanup:   FailureCleanupObservation{Complete: true, UnownedPreserved: true},
	}
	store := &memoryFailureCheckpoint{}

	run, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now)
	if err == nil || !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("secret-like injected identity error = %v", err)
	}
	if store.found || executor.injections != 1 || executor.observes != 0 || executor.cleanups != 1 || !run.Cleanup.Complete {
		t.Fatalf("secret-like injected identity recovery = run=%#v store=%#v calls injections=%d observes=%d cleanups=%d", run, store, executor.injections, executor.observes, executor.cleanups)
	}
}

func TestRunFailureScenarioRejectsSecretLikeLoadedCheckpointBeforeExecutor(t *testing.T) {
	scenario := testFailureScenario()
	executor := &failureExecutor{
		observation: FailureScenarioObservation{TrafficHealthVerified: true, IntegrityVerified: true, FencingVerified: true},
		cleanup:     FailureCleanupObservation{Complete: true, UnownedPreserved: true},
	}
	store := &memoryFailureCheckpoint{}
	if _, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now); err != nil {
		t.Fatalf("seed failure run: %v", err)
	}
	store.value.Injection.ID = "token=abcdefghijkl"
	injections, observes, cleanups := executor.injections, executor.observes, executor.cleanups

	if _, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now); err == nil || !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("secret-like loaded checkpoint error = %v", err)
	}
	if executor.injections != injections || executor.observes != observes || executor.cleanups != cleanups {
		t.Fatalf("secret-like loaded checkpoint invoked executor: before=%d/%d/%d after=%d/%d/%d", injections, observes, cleanups, executor.injections, executor.observes, executor.cleanups)
	}
}

func TestRunFailureScenarioReturnsFailProofForUnmetDeclaredGate(t *testing.T) {
	scenario := testFailureScenario()
	executor := &failureExecutor{
		observation: FailureScenarioObservation{TrafficHealthVerified: false, IntegrityVerified: true, FencingVerified: true},
		cleanup:     FailureCleanupObservation{Complete: true, UnownedPreserved: true},
	}
	store := &memoryFailureCheckpoint{}

	run, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now)
	if err != nil {
		t.Fatalf("failed failure drill: %v", err)
	}
	if run.Proof.Status != StatusFail || run.Proof.Reason == "" {
		t.Fatalf("failed failure drill = %#v", run.Proof)
	}
}

func TestRunFailureScenarioReturnsCleanupPersistenceError(t *testing.T) {
	scenario := testFailureScenario()
	executor := &failureExecutor{
		observation: FailureScenarioObservation{TrafficHealthVerified: true, IntegrityVerified: true, FencingVerified: true},
		cleanupErr:  errors.New("cleanup API unavailable"),
		cleanup:     FailureCleanupObservation{ResourceRefs: []string{"resource:fault-1"}},
	}
	store := &failingAfterFailureCheckpointStore{failAt: 3, err: errors.New("checkpoint backend unavailable")}

	run, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now)
	if err == nil || !strings.Contains(err.Error(), "checkpoint backend unavailable") {
		t.Fatalf("cleanup persistence error = %v, want checkpoint error", err)
	}
	if len(run.Cleanup.ResourceRefs) != 1 || run.Cleanup.ResourceRefs[0] != "resource:fault-1" {
		t.Fatalf("cleanup result = %#v, want returned cleanup observation", run.Cleanup)
	}
}

func TestRunFailureScenarioPersistsInjectionWhenContextIsCanceledAfterFault(t *testing.T) {
	scenario := testFailureScenario()
	ctx, cancel := context.WithCancel(context.Background())
	executor := &failureExecutor{
		afterInject: cancel,
		observation: FailureScenarioObservation{TrafficHealthVerified: true, DatabaseHealthVerified: true, IntegrityVerified: true, FencingVerified: true},
		cleanup:     FailureCleanupObservation{Complete: true, UnownedPreserved: true},
	}
	store := &memoryFailureCheckpoint{}

	if _, err := RunFailureScenario(ctx, scenario, "architecture-digest", "marker-1", executor, store, time.Now); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled failure run error = %v, want context canceled", err)
	}
	if store.value.State != FailureScenarioInjected || executor.injections != 1 || executor.observes != 0 {
		t.Fatalf("canceled failure run did not preserve injected checkpoint: state=%q injections=%d observes=%d", store.value.State, executor.injections, executor.observes)
	}

	executor.afterInject = nil
	run, err := RunFailureScenario(context.Background(), scenario, "architecture-digest", "marker-1", executor, store, time.Now)
	if err != nil {
		t.Fatalf("resumed canceled failure run: %v", err)
	}
	if run.Proof.Status != StatusPass || executor.injections != 1 || executor.observes != 1 || executor.cleanups != 1 {
		t.Fatalf("resumed canceled failure run = %#v, calls injections=%d observes=%d cleanups=%d", run, executor.injections, executor.observes, executor.cleanups)
	}
}

type failingAfterFailureCheckpointStore struct {
	memoryFailureCheckpoint
	failAt int
	err    error
	saves  int
}

func (store *failingAfterFailureCheckpointStore) Load(ctx context.Context) (FailureScenarioCheckpoint, error) {
	return store.memoryFailureCheckpoint.Load(ctx)
}

func (store *failingAfterFailureCheckpointStore) Save(ctx context.Context, checkpoint FailureScenarioCheckpoint) error {
	store.saves++
	if store.saves == store.failAt {
		return store.err
	}
	return store.memoryFailureCheckpoint.Save(ctx, checkpoint)
}

func testFailureScenario() ResilienceScenario {
	return ResilienceScenario{
		ID: "scenario-zone-loss", Kind: ScenarioZoneLoss, Scope: "zone", Mode: ScenarioAutomated,
		RequiredActions: []sdk.ResilienceAction{sdk.ResilienceFailover, sdk.ResilienceIntegrityCheck},
		FencingRequired: true, TrafficHealthRequired: true, IntegrityRequired: true,
		Owner: "platform", RunbookURL: "runbook://resilience/zone-loss",
	}
}
