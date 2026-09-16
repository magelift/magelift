package certification

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/magelift/magelift/sdk"
)

// CertificationRunRequest is the provider-neutral input for preparing one
// certification run. Provider SDK clients are supplied only through
// AdmissionProbe, PipelineBuildFunc, and the artifact signing interfaces; no
// provider-specific type crosses this boundary.
//
// The request deliberately has one target and one reusable boundary. A
// matrix runner can prepare one run per compatible runtime contract and reuse
// the returned artifact, admission snapshot, and boundary across its cells.
type CertificationRunRequest struct {
	TargetID string
	Release  string
	Edition  string
	Preset   string

	AdmissionProbe   AdmissionProbe
	AdmissionOptions CertificationRunAdmissionOptions

	ArtifactRegistry ImmutableArtifactRegistry
	ArtifactRequest  ArtifactBuildRequest
	ArtifactBuild    PipelineBuildFunc
	ArtifactSigning  ArtifactSigningOptions
	ReuseBoundary    ReuseBoundary
	// ScheduleOptions carries the run's durable resume, reuse, and budget
	// policy into plan construction. Admission fields are overwritten with the
	// single snapshot produced by PrepareCertificationRun.
	ScheduleOptions    *SchedulerOptions
	ScheduleInputIndex int
}

// CertificationRun is the prepared, provider-neutral state shared by all
// execution cells in one run. Artifact publication and provider admission are
// each performed once; callers pass Plan and AdmissionSnapshot to their
// executor and retain Admission for evidence.
type CertificationRun struct {
	Plan              CertificationPlan
	Artifact          ImmutableArtifactRecord
	ArtifactReused    bool
	Admission         CertificationRunAdmissionReport
	AdmissionSnapshot *AdmissionSnapshot
	AdmissionRun      *CertificationRunAdmission
}

// CertificationRunExecutionOptions contains the provider-owned execution
// callback and the durable checkpoint sink for one prepared run. The core
// scheduler owns ordering, bounded concurrency, resume, reuse claims, and
// cleanup-claim release; the callback owns provider SDK operations.
type CertificationRunExecutionOptions struct {
	CheckpointWriter         ScheduleCheckpointWriter
	Executor                 ScheduleExecutor
	LocalCapacityReservation LocalCapacityReservation
	ClaimReleaseTimeout      time.Duration
	Now                      time.Time
}

// CertificationRunExecution is the prepared run plus its latest scheduler
// checkpoint view. A failed execution still returns partial checkpoints so a
// caller can persist or inspect the exact provider operation boundary.
type CertificationRunExecution struct {
	Run       CertificationRun
	Execution ScheduleExecution
}

// CertificationRunBlockedError is returned when provider-side admission
// produces a redacted blocked report. It is intentionally separate from
// orchestration errors so callers can record a blocked cell without treating
// it as a successful certification or retrying it as green.
type CertificationRunBlockedError struct {
	Report CertificationRunAdmissionReport
}

func (err CertificationRunBlockedError) Error() string {
	if len(err.Report.Scopes) == 0 {
		return "certification run admission blocked"
	}
	var reasons []string
	for _, scope := range err.Report.Scopes {
		if scope.Admission.Ready {
			continue
		}
		for _, reason := range scope.Admission.BlockingReasons {
			reason = strings.TrimSpace(reason)
			if reason != "" {
				reasons = append(reasons, fmt.Sprintf("scope %d (%s/%s/%s): %s", scope.InputIndex, scope.Provider, scope.AccountOrProjectRef, scope.Region, reason))
			}
		}
	}
	if len(reasons) == 0 {
		return "certification run admission blocked"
	}
	return "certification run admission blocked: " + strings.Join(reasons, "; ")
}

// PrepareCertificationRun performs the fixed, cost-safe ordering for a live
// run:
//
//  1. validate the complete reusable boundary and artifact request;
//  2. construct and execute one bounded, run-scoped read-only admission;
//  3. build/sign/verify/publish or reuse one immutable artifact;
//  4. build the executable plan against the same admission snapshot.
//
// No provider mutation is reachable before step 3, and a blocked admission
// never invokes the build or signing callbacks. A published artifact remains
// reusable by later runs through the injected registry; runtime resources are
// owned and cleaned up by the provider executor after the returned plan.
func PrepareCertificationRun(ctx context.Context, request CertificationRunRequest) (CertificationRun, error) {
	if ctx == nil {
		return CertificationRun{}, errors.New("certification run context is required")
	}
	if err := ctx.Err(); err != nil {
		return CertificationRun{}, err
	}
	if strings.TrimSpace(request.TargetID) == "" || strings.TrimSpace(request.Release) == "" || strings.TrimSpace(request.Edition) == "" || strings.TrimSpace(request.Preset) == "" {
		return CertificationRun{}, errors.New("certification run target, release, edition, and preset are required")
	}
	if request.ReuseBoundary.Artifact != (sdk.ImmutableArtifactContract{}) {
		return CertificationRun{}, errors.New("certification run reuse boundary artifact must be omitted; the artifact registry owns it")
	}
	if err := request.ReuseBoundary.ValidateIdentity(); err != nil {
		return CertificationRun{}, fmt.Errorf("certification run reuse boundary: %w", err)
	}
	if request.ArtifactRequest.CompatibilityFingerprint != request.ReuseBoundary.CompatibilityFingerprint {
		return CertificationRun{}, errors.New("certification run artifact and reuse boundary fingerprints do not match")
	}
	if err := request.ArtifactRequest.Validate(); err != nil {
		return CertificationRun{}, fmt.Errorf("certification run artifact request: %w", err)
	}
	if request.ScheduleInputIndex < 0 || request.ScheduleInputIndex >= len(request.AdmissionOptions.RequiredInputs) {
		return CertificationRun{}, errors.New("certification run schedule admission input index is out of range")
	}

	admissionRun, err := NewCertificationRunAdmission(request.AdmissionProbe, request.AdmissionOptions)
	if err != nil {
		return CertificationRun{}, err
	}
	admissionReport, err := admissionRun.Admit(ctx)
	if err != nil {
		return CertificationRun{}, fmt.Errorf("certification run admission: %w", err)
	}
	if !admissionReport.Ready {
		return CertificationRun{Admission: admissionReport, AdmissionSnapshot: admissionRun.Snapshot(), AdmissionRun: admissionRun}, CertificationRunBlockedError{Report: admissionReport}
	}

	artifact, reused, err := EnsurePipelineArtifact(ctx, request.ArtifactRegistry, request.ArtifactRequest, request.ArtifactBuild, request.ArtifactSigning)
	if err != nil {
		return CertificationRun{Admission: admissionReport, AdmissionSnapshot: admissionRun.Snapshot(), AdmissionRun: admissionRun}, fmt.Errorf("certification run artifact: %w", err)
	}
	boundary := request.ReuseBoundary
	boundary.Artifact = artifact.Artifact
	if err := boundary.Validate(); err != nil {
		return CertificationRun{}, fmt.Errorf("certification run artifact boundary: %w", err)
	}
	inputs := admissionRun.RequiredInputs()
	plan, err := PlanForTargetWithBoundaryAndAdmissionOptions(ctx, request.TargetID, request.Release, request.Edition, request.Preset, boundary, inputs[request.ScheduleInputIndex], admissionRun.Snapshot(), request.ScheduleOptions)
	if err != nil {
		return CertificationRun{Artifact: artifact, ArtifactReused: reused, Admission: admissionReport, AdmissionSnapshot: admissionRun.Snapshot(), AdmissionRun: admissionRun}, fmt.Errorf("certification run plan: %w", err)
	}
	return CertificationRun{
		Plan:              plan,
		Artifact:          artifact,
		ArtifactReused:    reused,
		Admission:         admissionReport,
		AdmissionSnapshot: admissionRun.Snapshot(),
		AdmissionRun:      admissionRun,
	}, nil
}

// ExecuteCertificationRun prepares and executes one provider-neutral run. It
// is the single orchestration entry point for callers that want the cost-safe
// admission -> artifact -> plan -> bounded execution ordering. Provider
// implementations enter only through the request's admission/build/signing
// ports and the executor callback; no provider SDK type enters this package.
func ExecuteCertificationRun(ctx context.Context, request CertificationRunRequest, options CertificationRunExecutionOptions) (CertificationRunExecution, error) {
	if ctx == nil {
		return CertificationRunExecution{}, errors.New("certification run execution context is required")
	}
	if options.CheckpointWriter == nil {
		return CertificationRunExecution{}, errors.New("certification run execution checkpoint writer is required")
	}
	if options.Executor == nil {
		return CertificationRunExecution{}, errors.New("certification run execution executor is required")
	}
	prepared, err := PrepareCertificationRun(ctx, request)
	if err != nil {
		return CertificationRunExecution{Run: prepared}, err
	}
	var sessionRegistry SessionRegistry
	if request.ScheduleOptions != nil {
		sessionRegistry = request.ScheduleOptions.SessionRegistry
	}
	localCapacityReservation := options.LocalCapacityReservation
	if isNilLocalCapacityReservation(localCapacityReservation) && request.ScheduleOptions != nil {
		localCapacityReservation = request.ScheduleOptions.LocalCapacityReservation
	}
	execution, err := ExecuteSchedule(ctx, prepared.Plan.Execution, options.Executor, ScheduleExecutionOptions{
		CheckpointWriter:         options.CheckpointWriter,
		SessionRegistry:          sessionRegistry,
		LocalCapacityReservation: localCapacityReservation,
		ClaimReleaseTimeout:      options.ClaimReleaseTimeout,
		Now:                      options.Now,
	})
	if err != nil {
		return CertificationRunExecution{Run: prepared, Execution: execution}, fmt.Errorf("certification run schedule: %w", err)
	}
	return CertificationRunExecution{Run: prepared, Execution: execution}, nil
}
