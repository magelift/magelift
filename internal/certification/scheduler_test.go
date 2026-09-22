package certification

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/magelift/magelift/sdk"
)

func TestBuildScheduleGroupsExactProfilesAndOrdersWarmTransition(t *testing.T) {
	fingerprintA := strings.Repeat("a", 64)
	fingerprintB := strings.Repeat("b", 64)
	cells := []ScheduleCell{
		{ID: "baseline", Fingerprint: fingerprintA, WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/123", "aws/region/eu-west-1"}, OwnershipMarker: "run-1", Status: CapabilityCompatible, Required: true},
		{ID: "same-profile", Fingerprint: fingerprintA, WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/123", "aws/region/eu-west-1"}, OwnershipMarker: "run-1", Status: CapabilityCompatible},
		{ID: "release-transition", Fingerprint: fingerprintB, WarmBoundary: "aws/account/region", WarmFrom: "baseline", MutationKeys: []string{"aws/account/123", "aws/region/eu-west-1"}, OwnershipMarker: "run-1", Status: CapabilityExperimental, Required: true},
		{ID: "independent", Fingerprint: strings.Repeat("c", 64), WarmBoundary: "gcp/project/region", MutationKeys: []string{"gcp/project/1", "gcp/region/europe-west1"}, OwnershipMarker: "run-2", Status: CapabilityCompatible},
	}
	schedule, err := BuildSchedule(cells, SchedulerOptions{MaxParallel: 2, QuotaByKey: map[string]int{"aws/account/123": 1, "gcp/project/1": 1}})
	if err != nil {
		t.Fatal(err)
	}
	if schedule.ColdUnitCount != 2 || schedule.WarmTransitionCount != 1 {
		t.Fatalf("unit counts = cold %d warm %d, want 2 and 1: %#v", schedule.ColdUnitCount, schedule.WarmTransitionCount, schedule)
	}
	if schedule.ReusedCellCount != 2 {
		t.Fatalf("reused cell count = %d, want 2: %#v", schedule.ReusedCellCount, schedule)
	}
	if len(schedule.Units) != 3 || len(schedule.Units[0].CellIDs) == 0 {
		t.Fatalf("units = %#v", schedule.Units)
	}
	transitionIndex := -1
	for index, unit := range schedule.Units {
		if unit.BaselineCellID == "baseline" && !unit.Cold {
			transitionIndex = index
			if len(unit.DependsOn) != 1 {
				t.Fatalf("transition dependencies = %#v", unit.DependsOn)
			}
		}
	}
	if transitionIndex < 0 {
		t.Fatalf("warm transition unit missing: %#v", schedule.Units)
	}
	for batchIndex, batch := range schedule.Batches {
		for i := range batch {
			for j := i + 1; j < len(batch); j++ {
				if batch[i] == batch[j] {
					t.Fatalf("duplicate unit in batch %d: %#v", batchIndex, batch)
				}
			}
		}
	}
	if len(schedule.Batches) != 2 {
		t.Fatalf("batches = %#v, want baseline/independent then transition", schedule.Batches)
	}
}

func TestBuildScheduleReportsEstimatesAndEnforcesBudgets(t *testing.T) {
	fingerprint := strings.Repeat("a", 64)
	estimate := ExecutionEstimate{
		CloudOperationSeconds:   30,
		ResourceLifetimeSeconds: 600,
		EstimatedCostCents:      125,
		CleanupLatencySeconds:   45,
		CostKnown:               true,
	}
	cells := []ScheduleCell{
		{ID: "baseline", Fingerprint: fingerprint, WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/123"}, OwnershipMarker: "run-1", Estimate: estimate, Status: CapabilityCompatible, Required: true},
		{ID: "reused", Fingerprint: fingerprint, WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/123"}, OwnershipMarker: "run-1", Estimate: estimate, Status: CapabilityCompatible},
	}
	schedule, err := BuildSchedule(cells, SchedulerOptions{
		MaxParallel: 1,
		Budget: CertificationBudget{
			MaxCloudOperationSeconds:   30,
			MaxResourceLifetimeSeconds: 600,
			MaxEstimatedCostCents:      125,
			MaxCleanupLatencySeconds:   45,
			RequireCostEstimate:        true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if schedule.CloudOperationSeconds != 30 || schedule.ResourceLifetimeSeconds != 600 || schedule.EstimatedCostCents != 125 || schedule.CleanupLatencySeconds != 45 || !schedule.CostKnown {
		t.Fatalf("schedule estimates = %#v", schedule)
	}
	if schedule.ReusedCellCount != 1 {
		t.Fatalf("reused cell count = %d, want 1", schedule.ReusedCellCount)
	}

	_, err = BuildSchedule(cells, SchedulerOptions{
		MaxParallel: 1,
		Budget:      CertificationBudget{MaxCloudOperationSeconds: 29},
	})
	if err == nil || !strings.Contains(err.Error(), "cloud operation time budget exceeded") {
		t.Fatalf("cloud budget error = %v", err)
	}

	unknownCost := cells[0]
	unknownCost.ID = "unknown-cost"
	unknownCost.OwnershipMarker = "run-2"
	unknownCost.Estimate.CostKnown = false
	_, err = BuildSchedule([]ScheduleCell{unknownCost}, SchedulerOptions{
		MaxParallel: 1,
		Budget:      CertificationBudget{RequireCostEstimate: true},
	})
	if err == nil || !strings.Contains(err.Error(), "known cost estimate") {
		t.Fatalf("unknown cost error = %v", err)
	}
}

func TestBuildScheduleRejectsInconsistentReusableEstimates(t *testing.T) {
	fingerprint := strings.Repeat("b", 64)
	base := ScheduleCell{
		ID: "baseline", Fingerprint: fingerprint, WarmBoundary: "boundary", MutationKeys: []string{"account/1"}, OwnershipMarker: "run",
		Estimate: ExecutionEstimate{CloudOperationSeconds: 10}, Status: CapabilityCompatible,
	}
	reused := base
	reused.ID = "reused"
	reused.Estimate.CloudOperationSeconds = 11
	if _, err := BuildSchedule([]ScheduleCell{base, reused}, SchedulerOptions{MaxParallel: 1}); err == nil || !strings.Contains(err.Error(), "different from its reusable execution unit") {
		t.Fatalf("inconsistent estimate error = %v", err)
	}
}

func TestBuildScheduleRejectsEstimateOverflowBeforeBudgetEnforcement(t *testing.T) {
	maxInt64 := int64(math.MaxInt64)
	cells := []ScheduleCell{
		{ID: "first", Fingerprint: strings.Repeat("1", 64), WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/1"}, OwnershipMarker: "run-1", Estimate: ExecutionEstimate{CloudOperationSeconds: maxInt64}, Status: CapabilityCompatible},
		{ID: "second", Fingerprint: strings.Repeat("2", 64), WarmBoundary: "gcp/project/region", MutationKeys: []string{"gcp/project/1"}, OwnershipMarker: "run-2", Estimate: ExecutionEstimate{CloudOperationSeconds: maxInt64}, Status: CapabilityCompatible},
	}
	if _, err := BuildSchedule(cells, SchedulerOptions{MaxParallel: 2, Budget: CertificationBudget{MaxCloudOperationSeconds: maxInt64}}); err == nil || !strings.Contains(err.Error(), "overflows int64") {
		t.Fatalf("estimate overflow was accepted: %v", err)
	}
}

func TestBuildScheduleRejectsIncompatibleExplicitWarmReuseBoundary(t *testing.T) {
	baselineFingerprint := reusableFingerprint()
	transitionFingerprint := baselineFingerprint
	transitionFingerprint.SchemaFingerprint = strings.Repeat("d", 64)
	baselineDigest, err := baselineFingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	transitionDigest, err := transitionFingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	cells := []ScheduleCell{
		{ID: "baseline", Fingerprint: baselineDigest, ReuseFingerprint: &baselineFingerprint, WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/1"}, OwnershipMarker: baselineFingerprint.OwnershipMarker, Status: CapabilityCompatible},
		{ID: "transition", Fingerprint: transitionDigest, ReuseFingerprint: &transitionFingerprint, WarmFrom: "baseline", WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/1"}, OwnershipMarker: transitionFingerprint.OwnershipMarker, Status: CapabilityExperimental},
	}
	if _, err := BuildSchedule(cells, SchedulerOptions{MaxParallel: 1}); err == nil || !strings.Contains(err.Error(), "cold reusable boundary") {
		t.Fatalf("incompatible warm boundary was accepted: %v", err)
	}
}

func TestBuildScheduleResumesCompletedFingerprintAndRejectsStaleCheckpoint(t *testing.T) {
	fingerprint := strings.Repeat("d", 64)
	cells := []ScheduleCell{
		{ID: "done", Fingerprint: fingerprint, WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/123"}, OwnershipMarker: "run-1", Status: CapabilityCompatible, Required: true},
		{ID: "next", Fingerprint: fingerprint, WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/123"}, OwnershipMarker: "run-1", Status: CapabilityCompatible, Required: true},
	}
	schedule, err := BuildSchedule(cells, SchedulerOptions{MaxParallel: 1, Checkpoints: []SchedulerCheckpoint{{CellID: "done", Fingerprint: fingerprint, OwnershipMarker: "run-1", Status: "PASS"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(schedule.CompletedCellIDs) != 1 || schedule.CompletedCellIDs[0] != "done" {
		t.Fatalf("completed cells = %#v", schedule.CompletedCellIDs)
	}
	if len(schedule.Units) != 1 || schedule.Units[0].Cold || schedule.Units[0].BaselineCellID != "done" {
		t.Fatalf("resume did not reuse completed baseline: %#v", schedule.Units)
	}
	_, err = BuildSchedule(cells, SchedulerOptions{MaxParallel: 1, Checkpoints: []SchedulerCheckpoint{{CellID: "done", Fingerprint: strings.Repeat("e", 64), OwnershipMarker: "run-1", Status: "PASS"}}})
	if err == nil || !strings.Contains(err.Error(), "stale checkpoint") {
		t.Fatalf("stale checkpoint error = %v", err)
	}
}

func TestExecuteScheduleCarriesCompleteReuseFingerprintIntoCheckpoints(t *testing.T) {
	fingerprint := ReuseFingerprint{
		Architecture:         "aws/ecs-fargate",
		Fixture:              "fixture/known-content",
		BackupSet:            "backup/set-1",
		Observability:        "cloudwatch/logs-metrics",
		Edge:                 "cloudfront/none",
		ArtifactDigest:       "registry.example.invalid/app@sha256:" + strings.Repeat("a", 64),
		SchemaFingerprint:    strings.Repeat("b", 64),
		MigrationFingerprint: strings.Repeat("c", 64),
		OwnershipMarker:      "magelift/run-boundary",
		StateBackend:         "s3://state/opaque",
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	cell := ScheduleCell{
		ID: "baseline", Fingerprint: digest, ReuseFingerprint: &fingerprint,
		WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/1"},
		OwnershipMarker: fingerprint.OwnershipMarker, Status: CapabilityCompatible, Required: true,
	}
	schedule, err := BuildSchedule([]ScheduleCell{cell}, SchedulerOptions{MaxParallel: 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ExecuteSchedule(context.Background(), schedule, func(_ context.Context, unit ExecutionUnit) (ScheduleExecutionResult, error) {
		return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker, CleanupState: "complete"}, nil
	}, ScheduleExecutionOptions{CheckpointWriter: func(context.Context, []SchedulerCheckpoint) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Checkpoints) != 1 || result.Checkpoints[0].ReuseFingerprint == nil {
		t.Fatalf("checkpoint boundary = %#v", result.Checkpoints)
	}
	if got, err := result.Checkpoints[0].ReuseFingerprint.Digest(); err != nil || got != digest {
		t.Fatalf("checkpoint reuse digest = %q, err = %v, want %q", got, err, digest)
	}
}

func TestBuildScheduleRejectsCheckpointWithoutCompleteReuseFingerprint(t *testing.T) {
	fingerprint := ReuseFingerprint{
		Architecture: "gcp/gke-standard", Fixture: "fixture/known-content", BackupSet: "backup/set-1",
		Observability: "google-cloud-operations", Edge: "none",
		ArtifactDigest: "registry.example.invalid/app@sha256:" + strings.Repeat("d", 64), SchemaFingerprint: strings.Repeat("e", 64),
		MigrationFingerprint: strings.Repeat("f", 64), OwnershipMarker: "magelift/run-boundary", StateBackend: "gs://state/opaque",
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	cells := []ScheduleCell{{ID: "baseline", Fingerprint: digest, ReuseFingerprint: &fingerprint, WarmBoundary: "gcp/project/region", MutationKeys: []string{"gcp/project/1"}, OwnershipMarker: fingerprint.OwnershipMarker, Status: CapabilityCompatible}}
	_, err = BuildSchedule(cells, SchedulerOptions{MaxParallel: 1, Checkpoints: []SchedulerCheckpoint{{CellID: "baseline", Fingerprint: digest, OwnershipMarker: fingerprint.OwnershipMarker, Status: "PASS"}}})
	if err == nil || !strings.Contains(err.Error(), "missing the complete reuse fingerprint") {
		t.Fatalf("incomplete checkpoint error = %v", err)
	}
}

func TestBuildScheduleRejectsQuotaDeletionAndDuplicateOwnership(t *testing.T) {
	base := ScheduleCell{ID: "cell", Fingerprint: strings.Repeat("f", 64), WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/123"}, OwnershipMarker: "run-1", Status: CapabilityCompatible}
	if _, err := BuildSchedule([]ScheduleCell{base}, SchedulerOptions{MaxParallel: 1, PendingDeletions: []PendingDeletion{{OwnershipMarker: "old-run", MutationKeys: []string{"aws/account/123"}}}}); err == nil || !strings.Contains(err.Error(), "unresolved asynchronous deletion") {
		t.Fatalf("pending deletion error = %v", err)
	}
	other := base
	other.ID = "other"
	other.Fingerprint = strings.Repeat("0", 64)
	other.OwnershipMarker = "run-2"
	if _, err := BuildSchedule([]ScheduleCell{base, other}, SchedulerOptions{MaxParallel: 2, QuotaByKey: map[string]int{"aws/account/123": 0}}); err == nil || !strings.Contains(err.Error(), "quota") {
		t.Fatalf("quota error = %v", err)
	}
	other.OwnershipMarker = base.OwnershipMarker
	other.WarmBoundary = base.WarmBoundary + "/different"
	if _, err := BuildSchedule([]ScheduleCell{base, other}, SchedulerOptions{MaxParallel: 2}); err == nil || !strings.Contains(err.Error(), "duplicate ownership marker") {
		t.Fatalf("duplicate ownership error = %v", err)
	}
}

func TestBuildScheduleRejectsDependencyCycle(t *testing.T) {
	first := ScheduleCell{ID: "a", Fingerprint: strings.Repeat("1", 64), WarmBoundary: "boundary", MutationKeys: []string{"a"}, OwnershipMarker: "run", Status: CapabilityCompatible, Dependencies: []string{"b"}, WarmFrom: "b"}
	second := ScheduleCell{ID: "b", Fingerprint: strings.Repeat("2", 64), WarmBoundary: "boundary", MutationKeys: []string{"b"}, OwnershipMarker: "run", Status: CapabilityCompatible, Dependencies: []string{"a"}, WarmFrom: "a"}
	// The explicit warm transition itself is invalid here because it would
	// create a cycle, which should be diagnosed before batching.
	if _, err := BuildSchedule([]ScheduleCell{first, second}, SchedulerOptions{MaxParallel: 1}); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestBuildScheduleEnforcesExplicitCoverageBoundaries(t *testing.T) {
	baseCoverage := schedulerCoverageBoundary()
	serviceCoverage := schedulerCoverageBoundary()
	serviceCoverage.ServiceMajors = map[string]string{"database": "8.4", "cache": "10"}
	base := ScheduleCell{
		ID: "baseline", Fingerprint: strings.Repeat("1", 64), WarmBoundary: "aws/account/region", Coverage: &baseCoverage,
		MutationKeys: []string{"aws/account/1"}, OwnershipMarker: "magelift/run-coverage", Status: CapabilityCompatible,
	}
	transition := base
	transition.ID = "service-upgrade"
	transition.Fingerprint = strings.Repeat("2", 64)
	transition.WarmFrom = base.ID
	transition.WarmTransition = sdk.CoverageTransitionServiceMajor
	transition.Coverage = &serviceCoverage
	schedule, err := BuildSchedule([]ScheduleCell{base, transition}, SchedulerOptions{MaxParallel: 1})
	if err != nil {
		t.Fatal(err)
	}
	transitionUnitFound := false
	for _, unit := range schedule.Units {
		if unit.WarmTransition == sdk.CoverageTransitionServiceMajor {
			transitionUnitFound = true
		}
	}
	if schedule.ColdUnitCount != 1 || schedule.WarmTransitionCount != 1 || !transitionUnitFound {
		t.Fatalf("coverage transition schedule = %#v", schedule)
	}

	artifactCoverage := schedulerCoverageBoundary()
	artifactCoverage.ArtifactDigest = "registry.example.invalid/app@sha256:" + strings.Repeat("f", 64)
	transition.Coverage = &artifactCoverage
	if _, err := BuildSchedule([]ScheduleCell{base, transition}, SchedulerOptions{MaxParallel: 1}); err == nil || !strings.Contains(err.Error(), "cold boundary") {
		t.Fatalf("artifact change was accepted as warm coverage: %v", err)
	}
}

func schedulerCoverageBoundary() sdk.CoverageBoundary {
	return sdk.CoverageBoundary{
		Provider: "aws", Regions: []string{"eu-west-1"}, ComputeMode: "fargate", KubernetesTopology: "none",
		ManagedSelfHostedBoundary: "managed", ServiceMajors: map[string]string{"database": "8.4", "cache": "9"},
		ResilienceProfileID: "aws.ecs", ObservabilityDestination: "cloudwatch", EdgePath: "none",
		ArtifactDigest: "registry.example.invalid/app@sha256:" + strings.Repeat("a", 64), SchemaFingerprint: strings.Repeat("b", 64), MigrationFingerprint: strings.Repeat("c", 64), RecoveryFixtureID: "fixture-v1", RecoveryDestination: "none", OwnershipMarker: "magelift/run-coverage",
	}
}

func TestExecuteScheduleRunsIndependentUnitsAndPersistsUnitCheckpoints(t *testing.T) {
	fingerprintA := strings.Repeat("a", 64)
	fingerprintB := strings.Repeat("b", 64)
	schedule, err := BuildSchedule([]ScheduleCell{
		{ID: "aws", Fingerprint: fingerprintA, WarmBoundary: "aws/account/region", MutationKeys: []string{"aws/account/1"}, OwnershipMarker: "magelift/run-1-aws", Status: CapabilityCompatible},
		{ID: "gcp", Fingerprint: fingerprintB, WarmBoundary: "gcp/project/region", MutationKeys: []string{"gcp/project/1"}, OwnershipMarker: "magelift/run-1-gcp", Status: CapabilityCompatible},
	}, SchedulerOptions{MaxParallel: 2})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	var current atomic.Int32
	var maximum atomic.Int32
	var writesMu sync.Mutex
	var writes [][]SchedulerCheckpoint
	go func() {
		<-started
		<-started
		close(release)
	}()
	result, err := ExecuteSchedule(context.Background(), schedule, func(ctx context.Context, unit ExecutionUnit) (ScheduleExecutionResult, error) {
		// Count before signaling. The releaser closes once both units
		// have signaled, so a count taken after that signal can miss
		// the overlap and report a maximum of 1.
		value := current.Add(1)
		for {
			previous := maximum.Load()
			if value <= previous || maximum.CompareAndSwap(previous, value) {
				break
			}
		}
		started <- struct{}{}
		defer current.Add(-1)
		select {
		case <-release:
			return ScheduleExecutionResult{
				StackID: unit.ID, OperationIDs: []string{"operation-" + unit.ID}, ResourceRefs: []string{"resource:" + unit.ID},
				BackupRefs: []string{"backup:" + unit.ID}, RestoreRefs: []string{"restore:" + unit.ID},
				TelemetryRefs: []string{"telemetry:" + unit.ID}, EdgeRefs: []string{"edge:" + unit.ID}, CleanupState: "complete",
				OwnershipMarker: unit.OwnershipMarker,
			}, nil
		case <-ctx.Done():
			return ScheduleExecutionResult{}, ctx.Err()
		}
	}, ScheduleExecutionOptions{
		Now: timeForSchedulerTest(),
		CheckpointWriter: func(_ context.Context, checkpoints []SchedulerCheckpoint) error {
			writesMu.Lock()
			defer writesMu.Unlock()
			writes = append(writes, append([]SchedulerCheckpoint(nil), checkpoints...))
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 2 {
		t.Fatalf("maximum concurrent units = %d, want 2", maximum.Load())
	}
	if len(result.CompletedUnitIDs) != 2 || len(result.Checkpoints) != 2 {
		t.Fatalf("execution result = %#v", result)
	}
	if len(writes) != 4 {
		t.Fatalf("checkpoint writes = %d, want in-progress and pass per unit", len(writes))
	}
	for _, checkpoint := range result.Checkpoints {
		if checkpoint.Status != "PASS" || !strings.HasPrefix(checkpoint.OwnershipMarker, "magelift/run-1-") || len(checkpoint.OperationIDs) != 1 || len(checkpoint.BackupRefs) != 1 || len(checkpoint.RestoreRefs) != 1 || len(checkpoint.TelemetryRefs) != 1 || len(checkpoint.EdgeRefs) != 1 || checkpoint.ReuseMode != "cold" || checkpoint.CleanupState != "complete" {
			t.Fatalf("checkpoint = %#v", checkpoint)
		}
	}
}

func TestBuildScheduleUsesAdmissionSnapshotAcrossRunScopedPlans(t *testing.T) {
	var calls atomic.Int32
	snapshot, err := NewAdmissionSnapshot(admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		calls.Add(1)
		return readyAdmissionProbeResult(), nil
	}), AdmissionSnapshotOptions{Schema: "scheduler-schema", MaxConcurrent: 2})
	if err != nil {
		t.Fatal(err)
	}
	input := validAdmissionInput()
	cells := []ScheduleCell{{
		ID: "baseline", Fingerprint: strings.Repeat("a", 64), WarmBoundary: "aws/account/region",
		MutationKeys: []string{"aws/account-1/eu-west-1"}, OwnershipMarker: "magelift/run-1", Status: CapabilityCompatible,
	}}
	for range 2 {
		if _, err := BuildSchedule(cells, SchedulerOptions{
			MaxParallel: 1, Admission: &input, AdmissionSnapshot: snapshot,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("run-scoped admission calls = %d, want one", got)
	}
}

func TestExecuteScheduleSerializesUnitsInDifferentBatchesByProviderMutationKey(t *testing.T) {
	unit := func(id string) ExecutionUnit {
		return ExecutionUnit{ID: id, CellIDs: []string{id}, BaselineCellID: id, Fingerprint: strings.Repeat(id, 64), WarmBoundary: "aws/account/region", OwnershipMarker: "magelift/run-2", MutationKeys: []string{"aws/account/1"}}
	}
	schedule := Schedule{Units: []ExecutionUnit{unit("a"), unit("b")}, Batches: [][]string{{"a"}, {"b"}}}
	var active atomic.Int32
	var maximum atomic.Int32
	_, err := ExecuteSchedule(context.Background(), schedule, func(_ context.Context, unit ExecutionUnit) (ScheduleExecutionResult, error) {
		value := active.Add(1)
		for {
			previous := maximum.Load()
			if value <= previous || maximum.CompareAndSwap(previous, value) {
				break
			}
		}
		active.Add(-1)
		return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker}, nil
	}, ScheduleExecutionOptions{CheckpointWriter: func(context.Context, []SchedulerCheckpoint) error { return nil }})
	if err != nil {
		t.Fatal(err)
	}
	if maximum.Load() != 1 {
		t.Fatalf("maximum concurrent shared-key units = %d, want 1", maximum.Load())
	}
}

func TestExecuteScheduleWritesFailureAndDoesNotStartLaterBatches(t *testing.T) {
	first := ExecutionUnit{ID: "first", CellIDs: []string{"first"}, BaselineCellID: "first", Fingerprint: strings.Repeat("1", 64), WarmBoundary: "aws/account/region", OwnershipMarker: "magelift/run-3", MutationKeys: []string{"aws/account/1"}}
	second := first
	second.ID = "second"
	second.CellIDs = []string{"second"}
	second.Fingerprint = strings.Repeat("2", 64)
	schedule := Schedule{Units: []ExecutionUnit{first, second}, Batches: [][]string{{"first"}, {"second"}}}
	var started atomic.Int32
	result, err := ExecuteSchedule(context.Background(), schedule, func(_ context.Context, unit ExecutionUnit) (ScheduleExecutionResult, error) {
		started.Add(1)
		if unit.ID == "first" {
			return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker}, errors.New("provider rejected the mutation")
		}
		return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker}, nil
	}, ScheduleExecutionOptions{CheckpointWriter: func(context.Context, []SchedulerCheckpoint) error { return nil }})
	if err == nil || !strings.Contains(err.Error(), "provider rejected") {
		t.Fatalf("execution error = %v", err)
	}
	if started.Load() != 1 || result.FailedUnitID != "first" {
		t.Fatalf("failed execution = %#v, started = %d", result, started.Load())
	}
	if len(result.Checkpoints) != 1 || result.Checkpoints[0].Status != "FAIL" {
		t.Fatalf("failure checkpoint = %#v", result.Checkpoints)
	}
}

func TestExecuteSchedulePersistsTerminalCheckpointAfterCancellation(t *testing.T) {
	unit := ExecutionUnit{
		ID: "cancelled", CellIDs: []string{"cancelled"}, BaselineCellID: "cancelled",
		Fingerprint: strings.Repeat("c", 64), WarmBoundary: "aws/account/region",
		OwnershipMarker: "magelift/run-cancelled", MutationKeys: []string{"aws/account/1"},
	}
	schedule := Schedule{Units: []ExecutionUnit{unit}, Batches: [][]string{{unit.ID}}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var terminalStatus string
	var terminalContextErr error
	result, err := ExecuteSchedule(ctx, schedule, func(callbackCtx context.Context, _ ExecutionUnit) (ScheduleExecutionResult, error) {
		cancel()
		return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker, OperationIDs: []string{"operation-cancelled"}}, callbackCtx.Err()
	}, ScheduleExecutionOptions{
		CheckpointWriteTimeout: time.Second,
		CheckpointWriter: func(writerCtx context.Context, checkpoints []SchedulerCheckpoint) error {
			if len(checkpoints) == 1 && checkpoints[0].Status == "FAIL" {
				terminalStatus = checkpoints[0].Status
				terminalContextErr = writerCtx.Err()
			}
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("cancellation error = %v", err)
	}
	if result.FailedUnitID != unit.ID || len(result.Checkpoints) != 1 || result.Checkpoints[0].Status != "FAIL" {
		t.Fatalf("cancellation execution = %#v", result)
	}
	if terminalStatus != "FAIL" || terminalContextErr != nil {
		t.Fatalf("terminal checkpoint context = status %q, err %v", terminalStatus, terminalContextErr)
	}
	if len(result.Checkpoints[0].OperationIDs) != 1 || result.Checkpoints[0].OperationIDs[0] != "operation-cancelled" {
		t.Fatalf("terminal operation IDs = %#v", result.Checkpoints[0].OperationIDs)
	}
}

func timeForSchedulerTest() time.Time {
	return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
}

type recordingLocalCapacityReservation struct {
	delegate LocalCapacityReservation
	calls    chan LocalCapacityRequest
}

func (reservation *recordingLocalCapacityReservation) Acquire(ctx context.Context, request LocalCapacityRequest) (LocalCapacityLease, error) {
	reservation.calls <- request
	return reservation.delegate.Acquire(ctx, request)
}

func localCapacityTestSchedule(id string, request LocalCapacityRequest) Schedule {
	unit := ExecutionUnit{
		ID: id, CellIDs: []string{id}, BaselineCellID: id, Fingerprint: strings.Repeat("a", 64),
		WarmBoundary: "local-capacity", OwnershipMarker: "run-" + id,
	}
	return Schedule{Units: []ExecutionUnit{unit}, Batches: [][]string{{id}}, LocalCapacity: &request}
}

func TestBuildScheduleCarriesRequiredLocalCapacityIntoExecutionSchedule(t *testing.T) {
	input := validAdmissionInput()
	schedule, err := BuildSchedule([]ScheduleCell{{
		ID: "baseline", Fingerprint: strings.Repeat("a", 64), WarmBoundary: "aws/account/region",
		MutationKeys: []string{"aws/account-1/eu-west-1"}, OwnershipMarker: input.OwnershipMarker, Status: CapabilityCompatible,
	}}, SchedulerOptions{MaxParallel: 1, Admission: &input})
	if err != nil {
		t.Fatal(err)
	}
	want := LocalCapacityRequest{CPUMilli: input.RequiredLocalCPUMilli, MemoryMB: input.RequiredLocalMemoryMB}
	if schedule.LocalCapacity == nil || *schedule.LocalCapacity != want {
		t.Fatalf("schedule local capacity = %#v, want %#v", schedule.LocalCapacity, want)
	}
}

func TestExecuteScheduleReservesLocalCapacityAcrossConcurrentRuns(t *testing.T) {
	request := LocalCapacityRequest{CPUMilli: 1000, MemoryMB: 1024}
	capacity, err := NewMemoryLocalCapacityReservation(request)
	if err != nil {
		t.Fatal(err)
	}
	reservation := &recordingLocalCapacityReservation{delegate: capacity, calls: make(chan LocalCapacityRequest, 2)}
	firstSchedule := localCapacityTestSchedule("first", request)
	secondSchedule := localCapacityTestSchedule("second", request)
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	secondStarted := make(chan struct{})
	run := func(ctx context.Context, unit ExecutionUnit) (ScheduleExecutionResult, error) {
		switch unit.ID {
		case "first":
			close(firstStarted)
			select {
			case <-firstRelease:
			case <-ctx.Done():
				return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker}, ctx.Err()
			}
		case "second":
			close(secondStarted)
		}
		return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker}, nil
	}
	options := func() ScheduleExecutionOptions {
		return ScheduleExecutionOptions{
			CheckpointWriter:         func(context.Context, []SchedulerCheckpoint) error { return nil },
			LocalCapacityReservation: reservation,
		}
	}
	firstDone := make(chan error, 1)
	go func() {
		_, firstErr := ExecuteSchedule(context.Background(), firstSchedule, run, options())
		firstDone <- firstErr
	}()
	select {
	case <-firstStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("first unit did not start")
	}
	select {
	case <-reservation.calls:
	case <-time.After(2 * time.Second):
		t.Fatal("first reservation acquire was not observed")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, secondErr := ExecuteSchedule(context.Background(), secondSchedule, run, options())
		secondDone <- secondErr
	}()
	select {
	case <-reservation.calls:
	case <-time.After(2 * time.Second):
		t.Fatal("second reservation acquire was not observed")
	}
	select {
	case <-secondStarted:
		t.Fatal("second unit started while the first lease was held")
	default:
	}

	close(firstRelease)
	select {
	case firstErr := <-firstDone:
		if firstErr != nil {
			t.Fatalf("first schedule error = %v", firstErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first schedule did not finish")
	}
	select {
	case <-secondStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("second unit did not start after release")
	}
	select {
	case secondErr := <-secondDone:
		if secondErr != nil {
			t.Fatalf("second schedule error = %v", secondErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second schedule did not finish")
	}
}

func TestExecuteScheduleLocalCapacityCancellationDoesNotStartProviderOrStrandLease(t *testing.T) {
	request := LocalCapacityRequest{CPUMilli: 100, MemoryMB: 128}
	reservation, err := NewMemoryLocalCapacityReservation(request)
	if err != nil {
		t.Fatal(err)
	}
	holding, err := reservation.Acquire(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	defer holding.Release()

	recording := &recordingLocalCapacityReservation{delegate: reservation, calls: make(chan LocalCapacityRequest, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var providerStarted atomic.Bool
	done := make(chan error, 1)
	go func() {
		_, runErr := ExecuteSchedule(ctx, localCapacityTestSchedule("cancel", request), func(_ context.Context, unit ExecutionUnit) (ScheduleExecutionResult, error) {
			providerStarted.Store(true)
			return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker}, nil
		}, ScheduleExecutionOptions{
			CheckpointWriter:         func(context.Context, []SchedulerCheckpoint) error { return nil },
			LocalCapacityReservation: recording,
		})
		done <- runErr
	}()
	select {
	case <-recording.calls:
	case <-time.After(2 * time.Second):
		t.Fatal("local capacity acquire was not attempted")
	}
	cancel()
	select {
	case runErr := <-done:
		if !errors.Is(runErr, context.Canceled) {
			t.Fatalf("canceled schedule error = %v", runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("canceled schedule did not return")
	}
	if providerStarted.Load() {
		t.Fatal("provider executor started after local capacity cancellation")
	}

	if err := holding.Release(); err != nil {
		t.Fatal(err)
	}
	lease, err := reservation.Acquire(context.Background(), request)
	if err != nil {
		t.Fatalf("capacity was stranded after scheduler cancellation: %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteScheduleReleasesLocalCapacityAfterProviderFailure(t *testing.T) {
	request := LocalCapacityRequest{CPUMilli: 100, MemoryMB: 128}
	reservation, err := NewMemoryLocalCapacityReservation(request)
	if err != nil {
		t.Fatal(err)
	}
	schedule := localCapacityTestSchedule("failure", request)
	checkpointWriter := func(context.Context, []SchedulerCheckpoint) error { return nil }
	_, err = ExecuteSchedule(context.Background(), schedule, func(_ context.Context, unit ExecutionUnit) (ScheduleExecutionResult, error) {
		return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker}, errors.New("provider failure")
	}, ScheduleExecutionOptions{CheckpointWriter: checkpointWriter, LocalCapacityReservation: reservation})
	if err == nil || !strings.Contains(err.Error(), "provider failure") {
		t.Fatalf("provider failure = %v", err)
	}

	_, err = ExecuteSchedule(context.Background(), schedule, func(_ context.Context, unit ExecutionUnit) (ScheduleExecutionResult, error) {
		return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker}, nil
	}, ScheduleExecutionOptions{CheckpointWriter: checkpointWriter, LocalCapacityReservation: reservation})
	if err != nil {
		t.Fatalf("schedule after provider failure = %v", err)
	}
}

func TestExecuteScheduleRequiresLocalCapacityReservation(t *testing.T) {
	request := LocalCapacityRequest{CPUMilli: 100, MemoryMB: 128}
	called := false
	_, err := ExecuteSchedule(context.Background(), localCapacityTestSchedule("missing", request), func(_ context.Context, unit ExecutionUnit) (ScheduleExecutionResult, error) {
		called = true
		return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker}, nil
	}, ScheduleExecutionOptions{CheckpointWriter: func(context.Context, []SchedulerCheckpoint) error { return nil }})
	if err == nil || !strings.Contains(err.Error(), "local capacity reservation is required") {
		t.Fatalf("missing reservation error = %v", err)
	}
	if called {
		t.Fatal("provider executor ran without a local capacity reservation")
	}
}
