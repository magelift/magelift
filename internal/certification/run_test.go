package certification

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	buildpipeline "github.com/magelift/magelift/internal/build/pipeline"
)

func testRunBoundary() ReuseBoundary {
	return ReuseBoundary{
		CompatibilityFingerprint: strings.Repeat("a", 64),
		Fixture:                  "fixture-2026-08-11",
		BackupSet:                "backup-2026-08-11",
		Observability:            "native+newrelic",
		Edge:                     "native",
		SchemaFingerprint:        "schema-2026-08-11",
		MigrationFingerprint:     "migration-2026-08-11",
		StateBackend:             "state://certification/run",
	}
}

func TestPrepareCertificationRunAdmitsBeforeBuildingAndUsesOneSnapshot(t *testing.T) {
	input := runAdmissionTestInput()
	registry := NewMemoryImmutableArtifactRegistry()
	var builds atomic.Int32
	request := CertificationRunRequest{
		TargetID:         "aws/ecs-fargate",
		Release:          "2.4.9",
		Edition:          "open-source",
		Preset:           "preview",
		AdmissionProbe:   readyProviderAdmissionProbe(),
		AdmissionOptions: CertificationRunAdmissionOptions{RequiredInputs: []AdmissionInput{input}},
		ArtifactRegistry: registry,
		ArtifactRequest:  testArtifactRequest("magelift/test/run"),
		ArtifactBuild: func(context.Context) (buildpipeline.Result, error) {
			builds.Add(1)
			return testPipelineResult(), nil
		},
		ArtifactSigning: testArtifactSigningOptions(nil),
		ReuseBoundary:   testRunBoundary(),
	}
	request.ArtifactSigning.Signer = artifactSignerFunc(func(context.Context, string) error { return nil })
	request.ArtifactSigning.Verifier = artifactVerifierFunc(func(context.Context, string, string, string) error { return nil })

	prepared, err := PrepareCertificationRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !prepared.Admission.Ready || prepared.Plan.Artifact == nil || prepared.AdmissionSnapshot == nil || prepared.AdmissionRun == nil {
		t.Fatalf("prepared run = %#v", prepared)
	}
	if prepared.ArtifactReused || builds.Load() != 1 {
		t.Fatalf("artifact state reused=%v builds=%d", prepared.ArtifactReused, builds.Load())
	}
	if prepared.Plan.Artifact.ImageDigest != prepared.Artifact.Artifact.ImageDigest {
		t.Fatalf("plan artifact = %#v, record = %#v", prepared.Plan.Artifact, prepared.Artifact)
	}
	for _, unit := range prepared.Plan.Execution.Units {
		if unit.ReuseFingerprint == nil {
			t.Fatalf("unit %q lost reusable boundary", unit.ID)
		}
	}
}

func TestPrepareCertificationRunReusesPublishedArtifactWithoutRebuilding(t *testing.T) {
	input := runAdmissionTestInput()
	registry := NewMemoryImmutableArtifactRegistry()
	request := CertificationRunRequest{
		TargetID:         "aws/ecs-fargate",
		Release:          "2.4.9",
		Edition:          "open-source",
		Preset:           "preview",
		AdmissionProbe:   readyProviderAdmissionProbe(),
		AdmissionOptions: CertificationRunAdmissionOptions{RequiredInputs: []AdmissionInput{input}},
		ArtifactRegistry: registry,
		ArtifactRequest:  testArtifactRequest("magelift/test/run-reuse"),
		ArtifactBuild: func(context.Context) (buildpipeline.Result, error) {
			return testPipelineResult(), nil
		},
		ArtifactSigning: testArtifactSigningOptions(nil),
		ReuseBoundary:   testRunBoundary(),
	}
	request.ArtifactSigning.Signer = artifactSignerFunc(func(context.Context, string) error { return nil })
	request.ArtifactSigning.Verifier = artifactVerifierFunc(func(context.Context, string, string, string) error { return nil })

	first, err := PrepareCertificationRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.ArtifactBuild = nil
	request.ArtifactSigning = ArtifactSigningOptions{}
	second, err := PrepareCertificationRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !second.ArtifactReused || second.Artifact != first.Artifact {
		t.Fatalf("second run artifact = %#v reused=%v, first=%#v", second.Artifact, second.ArtifactReused, first.Artifact)
	}
}

func TestPrepareCertificationRunBlockedAdmissionDoesNotBuildOrPublish(t *testing.T) {
	input := runAdmissionTestInput()
	probe := readyProviderAdmissionProbe()
	probe.Network = secretAdmissionFact{}
	var builds atomic.Int32
	request := CertificationRunRequest{
		TargetID:         "aws/ecs-fargate",
		Release:          "2.4.9",
		Edition:          "open-source",
		Preset:           "preview",
		AdmissionProbe:   probe,
		AdmissionOptions: CertificationRunAdmissionOptions{RequiredInputs: []AdmissionInput{input}},
		ArtifactRegistry: NewMemoryImmutableArtifactRegistry(),
		ArtifactRequest:  testArtifactRequest("magelift/test/run-blocked"),
		ArtifactBuild: func(context.Context) (buildpipeline.Result, error) {
			builds.Add(1)
			return testPipelineResult(), nil
		},
		ReuseBoundary: testRunBoundary(),
	}

	prepared, err := PrepareCertificationRun(context.Background(), request)
	var blocked CertificationRunBlockedError
	if !errors.As(err, &blocked) || prepared.Admission.Ready || builds.Load() != 0 {
		t.Fatalf("blocked run = %#v, err=%v, builds=%d", prepared, err, builds.Load())
	}
	if strings.Contains(err.Error(), "super-secret-value") {
		t.Fatalf("blocked error leaked secret: %v", err)
	}
}

func TestPrepareCertificationRunCarriesSchedulerStateIntoPlan(t *testing.T) {
	input := runAdmissionTestInput()
	registry := NewMemorySessionRegistry()
	request := CertificationRunRequest{
		TargetID:         "aws/ecs-fargate",
		Release:          "2.4.9",
		Edition:          "open-source",
		Preset:           "preview",
		AdmissionProbe:   readyProviderAdmissionProbe(),
		AdmissionOptions: CertificationRunAdmissionOptions{RequiredInputs: []AdmissionInput{input}},
		ArtifactRegistry: NewMemoryImmutableArtifactRegistry(),
		ArtifactRequest:  testArtifactRequest("magelift/test/run-scheduler-state"),
		ArtifactBuild: func(context.Context) (buildpipeline.Result, error) {
			return testPipelineResult(), nil
		},
		ArtifactSigning: testArtifactSigningOptions(nil),
		ReuseBoundary:   testRunBoundary(),
		ScheduleOptions: &SchedulerOptions{
			SessionRegistry: registry,
			MaxParallel:     1,
			QuotaByKey:      map[string]int{"provider/aws": 1},
		},
	}
	request.ArtifactSigning.Signer = artifactSignerFunc(func(context.Context, string) error { return nil })
	request.ArtifactSigning.Verifier = artifactVerifierFunc(func(context.Context, string, string, string) error { return nil })

	first, err := PrepareCertificationRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Plan.Execution.Units) == 0 {
		t.Fatalf("first plan has no executable units: %#v", first.Plan.Execution)
	}
	unit := first.Plan.Execution.Units[0]
	request.ArtifactBuild = nil
	request.ArtifactSigning = ArtifactSigningOptions{}
	request.ScheduleOptions.Checkpoints = []SchedulerCheckpoint{{
		CellID: unit.CellIDs[0], Fingerprint: unit.Fingerprint, ReuseFingerprint: unit.ReuseFingerprint,
		OwnershipMarker: unit.OwnershipMarker, Status: "PASS",
	}}
	second, err := PrepareCertificationRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(second.Plan.Execution.CompletedCellIDs, unit.CellIDs[0]) {
		t.Fatalf("scheduler checkpoints were not propagated: %#v", second.Plan.Execution)
	}
	if len(second.Plan.Execution.Batches) == 0 || len(second.Plan.Execution.Batches[0]) > 1 {
		t.Fatalf("scheduler parallelism was not propagated: %#v", second.Plan.Execution.Batches)
	}
}

func TestPrepareCertificationRunReusesACompatibleSessionThroughSchedulerOptions(t *testing.T) {
	input := runAdmissionTestInput()
	registry := NewMemorySessionRegistry()
	request := CertificationRunRequest{
		TargetID:         "aws/ecs-fargate",
		Release:          "2.4.9",
		Edition:          "open-source",
		Preset:           "preview",
		AdmissionProbe:   readyProviderAdmissionProbe(),
		AdmissionOptions: CertificationRunAdmissionOptions{RequiredInputs: []AdmissionInput{input}},
		ArtifactRegistry: NewMemoryImmutableArtifactRegistry(),
		ArtifactRequest:  testArtifactRequest("magelift/test/run-session-reuse"),
		ArtifactBuild: func(context.Context) (buildpipeline.Result, error) {
			return testPipelineResult(), nil
		},
		ArtifactSigning: testArtifactSigningOptions(nil),
		ReuseBoundary:   testRunBoundary(),
		ScheduleOptions: &SchedulerOptions{SessionRegistry: registry, MaxParallel: 1, QuotaByKey: map[string]int{"provider/aws": 1}},
	}
	request.ArtifactSigning.Signer = artifactSignerFunc(func(context.Context, string) error { return nil })
	request.ArtifactSigning.Verifier = artifactVerifierFunc(func(context.Context, string, string, string) error { return nil })

	first, err := PrepareCertificationRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var unit ExecutionUnit
	foundColdUnit := false
	for _, candidate := range first.Plan.Execution.Units {
		if candidate.Cold && candidate.ReuseFingerprint != nil {
			unit = candidate
			foundColdUnit = true
			break
		}
	}
	if !foundColdUnit {
		t.Fatalf("first plan has no reusable unit: %#v", first.Plan.Execution)
	}
	fingerprint := *unit.ReuseFingerprint
	fingerprintHash, err := fingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Put(context.Background(), SessionRecord{
		SessionID: "session-1", Fingerprint: fingerprint, FingerprintHash: fingerprintHash,
		StackID: "stack-1", WriterID: "writer-1", State: SessionReady, Live: true,
		OperationIDs: []string{"operation-1"}, ResourceRefs: []string{"resource-1"},
		FixtureRefs: []string{"fixture-1"}, MigrationRefs: []string{"migration-1"},
		BackupRefs: []string{"backup-1"}, TelemetryRefs: []string{"telemetry-1"}, EdgeRefs: []string{"edge-1"},
	}); err != nil {
		t.Fatal(err)
	}
	request.ArtifactBuild = nil
	request.ArtifactSigning = ArtifactSigningOptions{}
	second, err := PrepareCertificationRun(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Plan.Execution.ReusedSessionCount == 0 || len(second.Plan.Execution.ReuseClaims) == 0 {
		t.Fatalf("compatible session was not reused: %#v", second.Plan.Execution)
	}
}

func TestExecuteCertificationRunUsesOnePreparedScheduleAndReturnsCheckpoints(t *testing.T) {
	input := runAdmissionTestInput()
	localCapacityReservation, err := NewMemoryLocalCapacityReservation(LocalCapacityRequest{
		CPUMilli: input.RequiredLocalCPUMilli, MemoryMB: input.RequiredLocalMemoryMB,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := CertificationRunRequest{
		TargetID:         "aws/ecs-fargate",
		Release:          "2.4.9",
		Edition:          "open-source",
		Preset:           "preview",
		AdmissionProbe:   readyProviderAdmissionProbe(),
		AdmissionOptions: CertificationRunAdmissionOptions{RequiredInputs: []AdmissionInput{input}},
		ArtifactRegistry: NewMemoryImmutableArtifactRegistry(),
		ArtifactRequest:  testArtifactRequest("magelift/test/run-execute"),
		ArtifactBuild: func(context.Context) (buildpipeline.Result, error) {
			return testPipelineResult(), nil
		},
		ArtifactSigning: testArtifactSigningOptions(nil),
		ReuseBoundary:   testRunBoundary(),
		ScheduleOptions: &SchedulerOptions{
			MaxParallel: 1, QuotaByKey: map[string]int{"provider/aws": 1}, LocalCapacityReservation: localCapacityReservation,
		},
	}
	request.ArtifactSigning.Signer = artifactSignerFunc(func(context.Context, string) error { return nil })
	request.ArtifactSigning.Verifier = artifactVerifierFunc(func(context.Context, string, string, string) error { return nil })

	var checkpointWrites atomic.Int32
	result, err := ExecuteCertificationRun(context.Background(), request, CertificationRunExecutionOptions{
		CheckpointWriter: func(_ context.Context, checkpoints []SchedulerCheckpoint) error {
			if len(checkpoints) == 0 {
				return errors.New("checkpoint writer received an empty batch")
			}
			checkpointWrites.Add(1)
			return nil
		},
		Executor: func(_ context.Context, unit ExecutionUnit) (ScheduleExecutionResult, error) {
			return ScheduleExecutionResult{OwnershipMarker: unit.OwnershipMarker, ReuseFingerprint: unit.ReuseFingerprint, CleanupState: "complete"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Run.Admission.Ready || len(result.Run.Plan.Execution.Units) == 0 {
		t.Fatalf("executed run = %#v", result.Run)
	}
	if len(result.Execution.CompletedUnitIDs) != len(result.Run.Plan.Execution.Units) {
		t.Fatalf("completed units = %d, want %d", len(result.Execution.CompletedUnitIDs), len(result.Run.Plan.Execution.Units))
	}
	if checkpointWrites.Load() < 2 {
		t.Fatalf("checkpoint writes = %d, want in-progress and terminal writes", checkpointWrites.Load())
	}
}
