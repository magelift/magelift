package certification

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func reusableFingerprint() ReuseFingerprint {
	return ReuseFingerprint{
		Architecture:         "aws-ecs-fargate",
		Fixture:              "valkey-9.0",
		BackupSet:            "backup-set-a",
		Observability:        "native-cloudwatch",
		Edge:                 "native-cloudfront",
		ArtifactDigest:       strings.Repeat("a", 64),
		SchemaFingerprint:    strings.Repeat("b", 64),
		MigrationFingerprint: strings.Repeat("c", 64),
		OwnershipMarker:      "magelift/certification/run-1",
		StateBackend:         "s3://state-bucket/magelift",
	}
}

func reusableSession(t *testing.T, fingerprint ReuseFingerprint) SessionRecord {
	t.Helper()
	digest, err := fingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	return SessionRecord{
		SessionID:       "session-1",
		Fingerprint:     fingerprint,
		FingerprintHash: digest,
		StackID:         "stack-1",
		WriterID:        "writer-1",
		State:           SessionReady,
		Live:            true,
		OperationIDs:    []string{"operation-1"},
		ResourceRefs:    []string{"resource-1"},
		FixtureRefs:     []string{"fixture-1"},
		MigrationRefs:   []string{"migration-1"},
		BackupRefs:      []string{"backup-1"},
		TelemetryRefs:   []string{"telemetry-1"},
		EdgeRefs:        []string{"edge-1"},
		CleanupComplete: false,
	}
}

func TestFileSessionRegistryPersistsClaimsAndCleanupState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	first, err := NewFileSessionRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewFileSessionRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint := reusableFingerprint()
	record := reusableSession(t, fingerprint)
	if err := first.Put(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := second.Lookup(context.Background(), fingerprint)
	if err != nil || !found || loaded.SessionID != record.SessionID {
		t.Fatalf("persisted session = %#v, found=%v, err=%v", loaded, found, err)
	}
	claim, err := first.Claim(context.Background(), fingerprint, "run-1")
	if err != nil || claim.Decision != ReuseReady || claim.Record.ClaimOwner != "run-1" {
		t.Fatalf("session claim = %#v, err=%v", claim, err)
	}
	blocked, err := second.Claim(context.Background(), fingerprint, "run-2")
	if err != nil || blocked.Decision != ReuseBlocked {
		t.Fatalf("conflicting session claim = %#v, err=%v", blocked, err)
	}
	if err := first.ReleaseClaim(context.Background(), fingerprint, "run-1"); err != nil {
		t.Fatal(err)
	}
	if err := first.MarkPendingDeletion(context.Background(), fingerprint, []string{"resource:stack-1"}); err != nil {
		t.Fatal(err)
	}
	pending, err := second.Claim(context.Background(), fingerprint, "run-3")
	if err != nil || pending.Decision != ReuseBlocked || pending.Record.State != SessionPendingDelete {
		t.Fatalf("pending session claim = %#v, err=%v", pending, err)
	}
	if err := first.MarkCleanupVerified(context.Background(), fingerprint, record.StackID); err != nil {
		t.Fatal(err)
	}
	cleaned, found, err := second.Lookup(context.Background(), fingerprint)
	if err != nil || !found || cleaned.State != SessionCleanupVerified || cleaned.Live {
		t.Fatalf("cleaned session = %#v, found=%v, err=%v", cleaned, found, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file session registry mode = %o, want 600", info.Mode().Perm())
	}
}

func TestFileSessionRegistryHonorsContextWhileWaitingForLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	registry, err := NewFileSessionRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".lock", 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path + ".lock")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, lookupErr := registry.Lookup(ctx, reusableFingerprint())
		result <- lookupErr
	}()
	time.Sleep(2 * registry.lockPoll)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled session lookup = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("file session registry lookup did not honor cancellation while waiting for lock")
	}
}

func TestMemorySessionRegistryReusesOnlyAnExactReadyFingerprint(t *testing.T) {
	ctx := context.Background()
	fingerprint := reusableFingerprint()
	registry := NewMemorySessionRegistry()
	if err := registry.Put(ctx, reusableSession(t, fingerprint)); err != nil {
		t.Fatal(err)
	}

	lookup, err := FindReusableSession(ctx, registry, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if lookup.Decision != ReuseReady || lookup.Record.SessionID != "session-1" {
		t.Fatalf("ready lookup = %#v", lookup)
	}

	changed := fingerprint
	changed.Edge = "fastly"
	lookup, err = FindReusableSession(ctx, registry, changed)
	if err != nil {
		t.Fatal(err)
	}
	if lookup.Decision != ReuseMiss || lookup.Record.SessionID != "" {
		t.Fatalf("changed fingerprint lookup = %#v", lookup)
	}
}

func TestMemorySessionRegistryClaimsAReadySessionAtomically(t *testing.T) {
	ctx := context.Background()
	fingerprint := reusableFingerprint()
	registry := NewMemorySessionRegistry()
	if err := registry.Put(ctx, reusableSession(t, fingerprint)); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan ReuseLookup, 2)
	var group sync.WaitGroup
	for _, owner := range []string{"run-a", "run-b"} {
		group.Add(1)
		go func(owner string) {
			defer group.Done()
			<-start
			lookup, err := ClaimReusableSession(ctx, registry, fingerprint, owner)
			if err != nil {
				t.Errorf("claim %q: %v", owner, err)
				return
			}
			results <- lookup
		}(owner)
	}
	close(start)
	group.Wait()
	close(results)

	var ready, blocked int
	var winner string
	for lookup := range results {
		switch lookup.Decision {
		case ReuseReady:
			ready++
			winner = lookup.Record.ClaimOwner
		case ReuseBlocked:
			blocked++
		default:
			t.Fatalf("unexpected claim decision = %#v", lookup)
		}
	}
	if ready != 1 || blocked != 1 {
		t.Fatalf("atomic claims = ready %d blocked %d", ready, blocked)
	}

	lookup, err := ClaimReusableSession(ctx, registry, fingerprint, winner)
	if err != nil {
		t.Fatal(err)
	}
	if lookup.Decision != ReuseReady {
		t.Fatalf("same owner claim = %#v", lookup)
	}
	if err := registry.ReleaseClaim(ctx, fingerprint, winner); err != nil {
		t.Fatal(err)
	}
	lookup, err = ClaimReusableSession(ctx, registry, fingerprint, "run-b")
	if err != nil {
		t.Fatal(err)
	}
	if lookup.Decision != ReuseReady || lookup.Record.ClaimOwner != "run-b" {
		t.Fatalf("claim after release = %#v", lookup)
	}
}

func TestMemorySessionRegistryBlocksDuplicateWritersDuringDelayedDeletion(t *testing.T) {
	ctx := context.Background()
	fingerprint := reusableFingerprint()
	registry := NewMemorySessionRegistry()
	if err := registry.Put(ctx, reusableSession(t, fingerprint)); err != nil {
		t.Fatal(err)
	}
	if err := registry.MarkPendingDeletion(ctx, fingerprint, []string{"resource-1"}); err != nil {
		t.Fatal(err)
	}

	lookup, err := FindReusableSession(ctx, registry, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if lookup.Decision != ReuseBlocked || !strings.Contains(lookup.Reason, "pending") {
		t.Fatalf("pending deletion lookup = %#v", lookup)
	}

	duplicate := reusableSession(t, fingerprint)
	duplicate.SessionID = "session-2"
	duplicate.StackID = "stack-2"
	duplicate.WriterID = "writer-2"
	if err := registry.Put(ctx, duplicate); err == nil || !strings.Contains(err.Error(), "duplicate live writer") {
		t.Fatalf("duplicate writer error = %v", err)
	}
}

func TestMemorySessionRegistryRequiresVerifiedCleanupBeforeStartingFresh(t *testing.T) {
	ctx := context.Background()
	fingerprint := reusableFingerprint()
	registry := NewMemorySessionRegistry()
	if err := registry.Put(ctx, reusableSession(t, fingerprint)); err != nil {
		t.Fatal(err)
	}
	if err := registry.MarkPendingDeletion(ctx, fingerprint, []string{"resource-1"}); err != nil {
		t.Fatal(err)
	}
	if err := registry.MarkCleanupVerified(ctx, fingerprint, "stack-1"); err != nil {
		t.Fatal(err)
	}

	lookup, err := FindReusableSession(ctx, registry, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if lookup.Decision != ReuseMiss || !strings.Contains(lookup.Reason, "cleaned") {
		t.Fatalf("cleaned lookup = %#v", lookup)
	}

	fresh := reusableSession(t, fingerprint)
	fresh.SessionID = "session-2"
	fresh.StackID = "stack-2"
	fresh.WriterID = "writer-2"
	if err := registry.Put(ctx, fresh); err != nil {
		t.Fatalf("fresh session after verified cleanup: %v", err)
	}
}

func TestSessionRegistriesRequirePendingDeletionBeforeCleanupVerification(t *testing.T) {
	tests := []struct {
		name     string
		registry func(t *testing.T) SessionRegistry
	}{
		{
			name: "memory",
			registry: func(*testing.T) SessionRegistry {
				return NewMemorySessionRegistry()
			},
		},
		{
			name: "file",
			registry: func(t *testing.T) SessionRegistry {
				registry, err := NewFileSessionRegistry(filepath.Join(t.TempDir(), "sessions.json"))
				if err != nil {
					t.Fatal(err)
				}
				return registry
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			fingerprint := reusableFingerprint()
			registry := test.registry(t)
			if err := registry.Put(ctx, reusableSession(t, fingerprint)); err != nil {
				t.Fatal(err)
			}

			if err := registry.MarkCleanupVerified(ctx, fingerprint, "stack-1"); err == nil || !strings.Contains(err.Error(), "pending deletion") {
				t.Fatalf("cleanup verification without pending deletion error = %v", err)
			}
			lookup, err := FindReusableSession(ctx, registry, fingerprint)
			if err != nil {
				t.Fatal(err)
			}
			if lookup.Decision != ReuseReady || lookup.Record.State != SessionReady {
				t.Fatalf("session changed after rejected cleanup verification = %#v", lookup)
			}
		})
	}
}

func TestMemorySessionRegistryRejectsInvalidRecordsAndCancellation(t *testing.T) {
	fingerprint := reusableFingerprint()
	registry := NewMemorySessionRegistry()
	invalid := reusableSession(t, fingerprint)
	invalid.FingerprintHash = "wrong"
	if err := registry.Put(context.Background(), invalid); err == nil || !strings.Contains(err.Error(), "fingerprint hash") {
		t.Fatalf("invalid record error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := FindReusableSession(ctx, registry, fingerprint); err == nil {
		t.Fatal("canceled lookup succeeded")
	}
}

func TestBuildScheduleUsesReadySessionWithoutAuthorizingPartialFingerprints(t *testing.T) {
	fingerprint := reusableFingerprint()
	digest, err := fingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	registry := NewMemorySessionRegistry()
	if err := registry.Put(context.Background(), reusableSession(t, fingerprint)); err != nil {
		t.Fatal(err)
	}
	cell := ScheduleCell{
		ID: "baseline", Fingerprint: digest, ReuseFingerprint: &fingerprint,
		WarmBoundary: "aws/account-1/eu-west-1", MutationKeys: []string{"aws/account-1"},
		OwnershipMarker: fingerprint.OwnershipMarker, Status: CapabilityCompatible, Required: true,
	}
	schedule, err := BuildSchedule([]ScheduleCell{cell}, SchedulerOptions{MaxParallel: 1, SessionRegistry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if schedule.ReusedSessionCount != 1 || schedule.ColdUnitCount != 0 || len(schedule.Units) != 1 || schedule.Units[0].ReuseSessionID != "session-1" {
		t.Fatalf("session reuse schedule = %#v", schedule)
	}
	if len(schedule.ReuseClaims) != 1 || schedule.ReuseClaims[0].SessionID != "session-1" {
		t.Fatalf("schedule reuse claims = %#v", schedule.ReuseClaims)
	}
	if schedule.ReuseClaims[0].Owner == fingerprint.OwnershipMarker || schedule.ReuseClaims[0].Owner == "" {
		t.Fatalf("schedule reused the stable ownership marker as its claim owner: %#v", schedule.ReuseClaims[0])
	}
	if err := ReleaseScheduleClaims(context.Background(), registry, schedule); err != nil {
		t.Fatal(err)
	}

	partial := cell
	partial.ID = "partial"
	partial.Fingerprint = strings.Repeat("d", 64)
	partial.ReuseFingerprint = nil
	partial.OwnershipMarker = "magelift/certification/run-2"
	partial.WarmBoundary = "aws/account-1/eu-west-1/run-2"
	partial.MutationKeys = []string{"aws/account-1/run-2"}
	partial.Required = false
	schedule, err = BuildSchedule([]ScheduleCell{partial}, SchedulerOptions{MaxParallel: 1, SessionRegistry: registry})
	if err != nil {
		t.Fatal(err)
	}
	if schedule.ReusedSessionCount != 0 || schedule.ColdUnitCount != 1 {
		t.Fatalf("partial fingerprint unexpectedly reused a session = %#v", schedule)
	}
}

func TestBuildScheduleUsesDistinctClaimOwnersForConcurrentCompatibleRuns(t *testing.T) {
	fingerprint := reusableFingerprint()
	digest, err := fingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	registry := NewMemorySessionRegistry()
	if err := registry.Put(context.Background(), reusableSession(t, fingerprint)); err != nil {
		t.Fatal(err)
	}
	cell := ScheduleCell{
		ID: "concurrent", Fingerprint: digest, ReuseFingerprint: &fingerprint,
		WarmBoundary: "aws/account-1/eu-west-1", MutationKeys: []string{"aws/account-1"},
		OwnershipMarker: fingerprint.OwnershipMarker, Status: CapabilityCompatible,
	}
	first, err := BuildSchedule([]ScheduleCell{cell}, SchedulerOptions{MaxParallel: 1, SessionRegistry: registry})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ReleaseScheduleClaims(context.Background(), registry, first) }()
	if _, err := BuildSchedule([]ScheduleCell{cell}, SchedulerOptions{MaxParallel: 1, SessionRegistry: registry}); err == nil || !strings.Contains(err.Error(), "blocked from session reuse") {
		t.Fatalf("second compatible schedule was not blocked by the first claim: %v", err)
	}

	if _, err := BuildSchedule([]ScheduleCell{cell}, SchedulerOptions{MaxParallel: 1, SessionRegistry: registry, ClaimOwner: "resume/run-1"}); err == nil || !strings.Contains(err.Error(), "blocked from session reuse") {
		t.Fatalf("explicitly different claim owner was not blocked: %v", err)
	}
}

func TestBuildScheduleReleasesReusableClaimWhenPlanningFails(t *testing.T) {
	fingerprint := reusableFingerprint()
	digest, err := fingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	registry := NewMemorySessionRegistry()
	if err := registry.Put(context.Background(), reusableSession(t, fingerprint)); err != nil {
		t.Fatal(err)
	}
	cell := ScheduleCell{
		ID: "budgeted", Fingerprint: digest, ReuseFingerprint: &fingerprint,
		WarmBoundary: "aws/account-1/eu-west-1", MutationKeys: []string{"aws/account-1"},
		OwnershipMarker: fingerprint.OwnershipMarker, Status: CapabilityCompatible,
		Estimate: ExecutionEstimate{CloudOperationSeconds: 10, CostKnown: true},
	}
	if _, err := BuildSchedule([]ScheduleCell{cell}, SchedulerOptions{
		MaxParallel: 1, SessionRegistry: registry,
		Budget: CertificationBudget{MaxCloudOperationSeconds: 1},
	}); err == nil || !strings.Contains(err.Error(), "cloud operation time budget exceeded") {
		t.Fatalf("budget planning error = %v", err)
	}
	lookup, err := ClaimReusableSession(context.Background(), registry, fingerprint, "another-run")
	if err != nil {
		t.Fatal(err)
	}
	if lookup.Decision != ReuseReady {
		t.Fatalf("claim was not released after planning failure = %#v", lookup)
	}
}

func TestBuildScheduleReleasesEarlierClaimsWhenLaterClaimFails(t *testing.T) {
	firstFingerprint := reusableFingerprint()
	secondFingerprint := firstFingerprint
	secondFingerprint.ArtifactDigest = strings.Repeat("d", 64)
	secondFingerprint.OwnershipMarker = "magelift/certification/run-2"
	registry := NewMemorySessionRegistry()
	if err := registry.Put(context.Background(), reusableSession(t, firstFingerprint)); err != nil {
		t.Fatal(err)
	}
	if err := registry.Put(context.Background(), reusableSession(t, secondFingerprint)); err != nil {
		t.Fatal(err)
	}
	secondDigest, err := secondFingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if lookup, err := ClaimReusableSession(context.Background(), registry, secondFingerprint, "other-run"); err != nil || lookup.Decision != ReuseReady {
		t.Fatalf("pre-claim second session = %#v, %v", lookup, err)
	}
	defer func() { _ = registry.ReleaseClaim(context.Background(), secondFingerprint, "other-run") }()

	firstDigest, err := firstFingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	cells := []ScheduleCell{
		{ID: "first", Fingerprint: firstDigest, ReuseFingerprint: &firstFingerprint, WarmBoundary: "aws/account-1/eu-west-1", MutationKeys: []string{"aws/account-1"}, OwnershipMarker: firstFingerprint.OwnershipMarker, Status: CapabilityCompatible},
		{ID: "second", Fingerprint: secondDigest, ReuseFingerprint: &secondFingerprint, WarmBoundary: "aws/account-2/eu-west-1", MutationKeys: []string{"aws/account-2"}, OwnershipMarker: secondFingerprint.OwnershipMarker, Status: CapabilityCompatible},
	}
	if _, err := BuildSchedule(cells, SchedulerOptions{MaxParallel: 2, SessionRegistry: registry}); err == nil || !strings.Contains(err.Error(), "blocked from session reuse") {
		t.Fatalf("later claim failure = %v", err)
	}
	lookup, err := ClaimReusableSession(context.Background(), registry, firstFingerprint, "next-run")
	if err != nil || lookup.Decision != ReuseReady {
		t.Fatalf("earlier claim was not released = %#v, %v", lookup, err)
	}
}

func TestBuildScheduleRejectsMixedReusableAuthorizationWithinOneUnit(t *testing.T) {
	fingerprint := reusableFingerprint()
	digest, err := fingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	registry := NewMemorySessionRegistry()
	if err := registry.Put(context.Background(), reusableSession(t, fingerprint)); err != nil {
		t.Fatal(err)
	}
	cells := []ScheduleCell{
		{ID: "authorized", Fingerprint: digest, ReuseFingerprint: &fingerprint, WarmBoundary: "aws/account-1/eu-west-1", MutationKeys: []string{"aws/account-1"}, OwnershipMarker: fingerprint.OwnershipMarker, Status: CapabilityCompatible, Required: true},
		{ID: "partial", Fingerprint: digest, WarmBoundary: "aws/account-1/eu-west-1", MutationKeys: []string{"aws/account-1"}, OwnershipMarker: fingerprint.OwnershipMarker, Status: CapabilityCompatible},
	}
	if _, err := BuildSchedule(cells, SchedulerOptions{MaxParallel: 1, SessionRegistry: registry}); err == nil || !strings.Contains(err.Error(), "reusable and non-reusable") {
		t.Fatalf("mixed reusable unit = %v", err)
	}
	lookup, err := ClaimReusableSession(context.Background(), registry, fingerprint, "next-run")
	if err != nil || lookup.Decision != ReuseReady {
		t.Fatalf("mixed-unit claim was not released = %#v, %v", lookup, err)
	}
}

func TestBuildScheduleFailsClosedOnPendingReusableSession(t *testing.T) {
	fingerprint := reusableFingerprint()
	registry := NewMemorySessionRegistry()
	if err := registry.Put(context.Background(), reusableSession(t, fingerprint)); err != nil {
		t.Fatal(err)
	}
	if err := registry.MarkPendingDeletion(context.Background(), fingerprint, []string{"resource-1"}); err != nil {
		t.Fatal(err)
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	cell := ScheduleCell{
		ID: "blocked", Fingerprint: digest, ReuseFingerprint: &fingerprint,
		WarmBoundary: "aws/account-1/eu-west-1", MutationKeys: []string{"aws/account-1"},
		OwnershipMarker: fingerprint.OwnershipMarker, Status: CapabilityCompatible,
	}
	if _, err := BuildSchedule([]ScheduleCell{cell}, SchedulerOptions{MaxParallel: 1, SessionRegistry: registry}); err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("pending session schedule error = %v", err)
	}
}

func TestExecuteScheduleReleasesClaimsAfterExecutionFailure(t *testing.T) {
	fingerprint := reusableFingerprint()
	digest, err := fingerprint.Digest()
	if err != nil {
		t.Fatal(err)
	}
	registry := NewMemorySessionRegistry()
	if err := registry.Put(context.Background(), reusableSession(t, fingerprint)); err != nil {
		t.Fatal(err)
	}
	schedule, err := BuildSchedule([]ScheduleCell{{
		ID: "claimed", Fingerprint: digest, ReuseFingerprint: &fingerprint,
		WarmBoundary: "aws/account-1/eu-west-1", MutationKeys: []string{"aws/account-1"},
		OwnershipMarker: fingerprint.OwnershipMarker, Status: CapabilityCompatible,
	}}, SchedulerOptions{MaxParallel: 1, SessionRegistry: registry})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ExecuteSchedule(context.Background(), schedule, func(context.Context, ExecutionUnit) (ScheduleExecutionResult, error) {
		return ScheduleExecutionResult{OwnershipMarker: fingerprint.OwnershipMarker}, errors.New("provider execution failed")
	}, ScheduleExecutionOptions{
		CheckpointWriter: func(context.Context, []SchedulerCheckpoint) error { return nil },
		SessionRegistry:  registry,
	})
	if err == nil || !strings.Contains(err.Error(), "provider execution failed") {
		t.Fatalf("execution error = %v", err)
	}
	lookup, err := ClaimReusableSession(context.Background(), registry, fingerprint, "next-run")
	if err != nil {
		t.Fatal(err)
	}
	if lookup.Decision != ReuseReady || lookup.Record.ClaimOwner != "next-run" {
		t.Fatalf("claim after failed execution = %#v", lookup)
	}
}
