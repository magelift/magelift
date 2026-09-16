package certification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/sdk"
)

type memoryRecoveryCheckpoint struct {
	value RecoveryCheckpoint
	loads int
	saves int
}

func (store *memoryRecoveryCheckpoint) Load(context.Context) (RecoveryCheckpoint, error) {
	store.loads++
	if store.value.PlanDigest == "" {
		return RecoveryCheckpoint{}, ErrRecoveryCheckpointNotFound
	}
	return store.value, nil
}

func (store *memoryRecoveryCheckpoint) Save(_ context.Context, value RecoveryCheckpoint) error {
	store.saves++
	store.value = value
	return nil
}

type contextCheckingRecoveryCheckpoint struct {
	memoryRecoveryCheckpoint
	canceledSave bool
}

func (store *contextCheckingRecoveryCheckpoint) Save(ctx context.Context, value RecoveryCheckpoint) error {
	if err := ctx.Err(); err != nil {
		store.canceledSave = true
		return err
	}
	return store.memoryRecoveryCheckpoint.Save(ctx, value)
}

type fakeRecoveryAdapter struct {
	plan            sdk.ResiliencePlan
	executed        []string
	requests        []sdk.ResilienceExecutionRequest
	failStage       string
	evidenceFixture string
	afterExecute    func()
	operationID     string
}

func (adapter *fakeRecoveryAdapter) ResilienceDescriptor() sdk.ResilienceAdapterDescriptor {
	return sdk.ResilienceAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "test.recovery", Provider: "aws", Version: "1.0.0", Capabilities: []sdk.ResilienceAction{sdk.ResilienceBackup, sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck, sdk.ResilienceFence, sdk.ResilienceFailover, sdk.ResilienceCleanup}}
}

func (adapter *fakeRecoveryAdapter) PlanResilience(context.Context, sdk.ResiliencePlanRequest) (sdk.ResiliencePlan, error) {
	return adapter.plan, nil
}

func (adapter *fakeRecoveryAdapter) ExecuteResilience(_ context.Context, request sdk.ResilienceExecutionRequest) (sdk.ResilienceExecutionResult, error) {
	adapter.requests = append(adapter.requests, request)
	adapter.executed = append(adapter.executed, request.StageID+":"+request.IdempotencyKey)
	if request.StageID == adapter.failStage {
		return sdk.ResilienceExecutionResult{}, errors.New("simulated provider interruption")
	}
	stage, _ := recoveryStageByID(request.Plan.Stages, request.StageID)
	result := sdk.ResilienceExecutionResult{
		Action:              stage.Action,
		OperationID:         "op-" + request.StageID,
		ProofRefs:           []string{"proof-" + request.StageID},
		OwnershipMarker:     request.OwnershipMarker,
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}
	if adapter.operationID != "" {
		result.OperationID = adapter.operationID
	}
	for _, dataClass := range stage.DataClasses {
		fixtureID := request.FixtureID
		if adapter.evidenceFixture != "" {
			fixtureID = adapter.evidenceFixture
		}
		proof := sdk.ResilienceProofEvidence{DataClass: dataClass, Destination: string(stage.Destination), FixtureID: fixtureID, RetentionDays: 30, EncryptionVerified: true, ProtectionVerified: true, PermissionsVerified: true, ServiceHealthVerified: true}
		switch stage.Action {
		case sdk.ResilienceBackup:
			proof.BackupID = "backup/" + stage.ID
		case sdk.ResilienceRestore:
			proof.RestoreID = "restore/" + stage.ID
		case sdk.ResilienceIntegrityCheck:
			proof.ManifestVerified = true
		}
		result.Evidence = append(result.Evidence, proof)
	}
	if adapter.afterExecute != nil {
		adapter.afterExecute()
	}
	return result, nil
}

func TestRunResilienceRejectsEvidenceOutsideTheRequestedFixture(t *testing.T) {
	architecture := testRecoveryArchitecture(t)
	adapter := &fakeRecoveryAdapter{
		evidenceFixture: "different-fixture",
		plan: sdk.ResiliencePlan{
			AdapterID:      "test.recovery",
			Stages:         []sdk.ResilienceStage{{ID: "backup-database", Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion, Idempotent: true}},
			RequiredProofs: []string{"backup:database"},
		},
	}
	_, err := RunResilience(context.Background(), adapter, sdk.ResiliencePlanRequest{Architecture: architecture, FixtureID: "fixture-1", OwnershipMarker: architecture.OwnershipMarker}, &memoryRecoveryCheckpoint{}, time.Now)
	if err == nil || !strings.Contains(err.Error(), "fixture") {
		t.Fatalf("unbound fixture evidence was accepted: %v", err)
	}
}

func TestRunResilienceCheckpointsEachStageAndResumesWithoutRepeatingCompletedStages(t *testing.T) {
	architecture := testRecoveryArchitecture(t)
	adapter := &fakeRecoveryAdapter{plan: sdk.ResiliencePlan{AdapterID: "test.recovery", FixtureID: "fixture-1", OwnershipMarker: architecture.OwnershipMarker, Stages: []sdk.ResilienceStage{
		{ID: "backup-database", Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion, Idempotent: true},
		{ID: "restore-database", Action: sdk.ResilienceRestore, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion, DependsOn: []string{"backup-database"}, Idempotent: true},
		{ID: "integrity-database", Action: sdk.ResilienceIntegrityCheck, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion, DependsOn: []string{"restore-database"}, Idempotent: true},
	}, RequiredProofs: []string{"backup:database", "restore:database", "integrity:database"}}}
	store := &memoryRecoveryCheckpoint{}
	now := func() time.Time { return time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC) }
	request := sdk.ResiliencePlanRequest{Architecture: architecture, FixtureID: "fixture-1", OwnershipMarker: architecture.OwnershipMarker}
	first, err := RunResilience(context.Background(), adapter, request, store, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Stages) != 3 || len(adapter.executed) != 3 || store.saves != 3 {
		t.Fatalf("first execution = %#v, executed = %#v, saves = %d", first, adapter.executed, store.saves)
	}
	if got := adapter.requests[1].BackupReferences["database"]; got != "backup/backup-database" {
		t.Fatalf("restore did not receive checkpointed backup reference: %q", got)
	}
	second, err := RunResilience(context.Background(), adapter, request, store, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Stages) != 3 || len(adapter.executed) != 3 {
		t.Fatalf("resume repeated provider work: execution = %#v, executed = %#v", second, adapter.executed)
	}
	for _, stage := range second.Stages {
		if !stage.Reused || stage.Status != StatusPass {
			t.Fatalf("resumed stage = %#v", stage)
		}
	}
}

func TestRunResiliencePersistsStageCheckpointAfterCallerCancellation(t *testing.T) {
	architecture := testRecoveryArchitecture(t)
	ctx, cancel := context.WithCancel(context.Background())
	adapter := &fakeRecoveryAdapter{
		afterExecute: cancel,
		plan: sdk.ResiliencePlan{
			AdapterID:       "test.recovery",
			FixtureID:       "fixture-1",
			OwnershipMarker: architecture.OwnershipMarker,
			Stages: []sdk.ResilienceStage{{
				ID: "backup-database", Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion, Idempotent: true,
			}},
			RequiredProofs: []string{"backup:database"},
		},
	}
	store := &contextCheckingRecoveryCheckpoint{}
	request := sdk.ResiliencePlanRequest{Architecture: architecture, FixtureID: "fixture-1", OwnershipMarker: architecture.OwnershipMarker}

	first, err := RunResilience(ctx, adapter, request, store, time.Now)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled recovery run error = %v, want context canceled", err)
	}
	if len(first.Stages) != 1 || first.Stages[0].Reused || first.Stages[0].Status != StatusPass {
		t.Fatalf("canceled recovery execution = %#v", first)
	}
	if store.canceledSave || store.value.Stages["backup-database"].Status != StatusPass || len(adapter.executed) != 1 {
		t.Fatalf("canceled recovery checkpoint/calls = checkpoint=%#v canceledSave=%t executed=%#v", store.value, store.canceledSave, adapter.executed)
	}

	adapter.afterExecute = nil
	second, err := RunResilience(context.Background(), adapter, request, store, time.Now)
	if err != nil {
		t.Fatalf("resume after canceled recovery run: %v", err)
	}
	if len(second.Stages) != 1 || !second.Stages[0].Reused || len(adapter.executed) != 1 {
		t.Fatalf("resumed canceled recovery execution = %#v, executed=%#v", second, adapter.executed)
	}
}

func TestRunResilienceRejectsSecretLikeProviderResultBeforeCheckpoint(t *testing.T) {
	architecture := testRecoveryArchitecture(t)
	adapter := &fakeRecoveryAdapter{
		operationID: "password=abcdefghijkl",
		plan: sdk.ResiliencePlan{
			AdapterID:       "test.recovery",
			FixtureID:       "fixture-1",
			OwnershipMarker: architecture.OwnershipMarker,
			Stages: []sdk.ResilienceStage{{
				ID: "backup-database", Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion, Idempotent: true,
			}},
			RequiredProofs: []string{"backup:database"},
		},
	}
	store := &memoryRecoveryCheckpoint{}
	request := sdk.ResiliencePlanRequest{Architecture: architecture, FixtureID: "fixture-1", OwnershipMarker: architecture.OwnershipMarker}

	if _, err := RunResilience(context.Background(), adapter, request, store, time.Now); err == nil || !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("secret-like provider result error = %v", err)
	}
	if store.saves != 0 || store.value.PlanDigest != "" {
		t.Fatalf("secret-like provider result was checkpointed: saves=%d checkpoint=%#v", store.saves, store.value)
	}
}

func TestRunResilienceRejectsSecretLikeLoadedCheckpoint(t *testing.T) {
	architecture := testRecoveryArchitecture(t)
	adapter := &fakeRecoveryAdapter{plan: sdk.ResiliencePlan{
		AdapterID:       "test.recovery",
		FixtureID:       "fixture-1",
		OwnershipMarker: architecture.OwnershipMarker,
		Stages: []sdk.ResilienceStage{{
			ID: "backup-database", Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion, Idempotent: true,
		}},
		RequiredProofs: []string{"backup:database"},
	}}
	store := &memoryRecoveryCheckpoint{}
	request := sdk.ResiliencePlanRequest{Architecture: architecture, FixtureID: "fixture-1", OwnershipMarker: architecture.OwnershipMarker}
	if _, err := RunResilience(context.Background(), adapter, request, store, time.Now); err != nil {
		t.Fatalf("seed recovery run: %v", err)
	}
	checkpointStage := store.value.Stages["backup-database"]
	checkpointStage.OperationID = "token=abcdefghijkl"
	store.value.Stages["backup-database"] = checkpointStage
	executed := len(adapter.executed)

	if _, err := RunResilience(context.Background(), adapter, request, store, time.Now); err == nil || !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("secret-like loaded checkpoint error = %v", err)
	}
	if len(adapter.executed) != executed {
		t.Fatalf("secret-like loaded checkpoint triggered provider execution: before=%d after=%d", executed, len(adapter.executed))
	}
}

func TestRunResilienceKeepsCheckpointWhenProviderStageFails(t *testing.T) {
	architecture := testRecoveryArchitecture(t)
	adapter := &fakeRecoveryAdapter{failStage: "restore-database", plan: sdk.ResiliencePlan{AdapterID: "test.recovery", FixtureID: "fixture-1", OwnershipMarker: architecture.OwnershipMarker, Stages: []sdk.ResilienceStage{
		{ID: "backup-database", Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion, Idempotent: true},
		{ID: "restore-database", Action: sdk.ResilienceRestore, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion, DependsOn: []string{"backup-database"}, Idempotent: true},
	}, RequiredProofs: []string{"backup:database", "restore:database"}}}
	store := &memoryRecoveryCheckpoint{}
	request := sdk.ResiliencePlanRequest{Architecture: architecture, FixtureID: "fixture-1", OwnershipMarker: architecture.OwnershipMarker}
	if _, err := RunResilience(context.Background(), adapter, request, store, time.Now); err == nil {
		t.Fatal("provider interruption was accepted")
	}
	if store.value.Stages["backup-database"].Status != StatusPass || store.value.Stages["restore-database"].Status == StatusPass {
		t.Fatalf("checkpoint after failure = %#v", store.value)
	}
}

func TestRunResilienceRejectsMismatchedCheckpoint(t *testing.T) {
	architecture := testRecoveryArchitecture(t)
	adapter := &fakeRecoveryAdapter{plan: sdk.ResiliencePlan{AdapterID: "test.recovery", Stages: []sdk.ResilienceStage{{ID: "cleanup", Action: sdk.ResilienceCleanup, DataClasses: []string{"database"}, Destination: sdk.RecoverySameRegion, Idempotent: true}}, RequiredProofs: []string{"cleanup"}}}
	store := &memoryRecoveryCheckpoint{value: RecoveryCheckpoint{PlanDigest: "stale", OwnershipMarker: architecture.OwnershipMarker, FixtureID: "fixture-1", Stages: map[string]RecoveryStageState{}}}
	request := sdk.ResiliencePlanRequest{Architecture: architecture, FixtureID: "fixture-1", OwnershipMarker: architecture.OwnershipMarker}
	if _, err := RunResilience(context.Background(), adapter, request, store, time.Now); err == nil {
		t.Fatal("stale checkpoint was accepted")
	}
}

func testRecoveryArchitecture(t *testing.T) sdk.ArchitectureIntent {
	t.Helper()
	return sdk.ArchitectureIntent{
		ProfileID: "test-architecture", Provider: "aws", Runtime: "ecs-fargate", AccountOrProjectRef: "account", Region: "eu-west-1", Regions: []string{"eu-west-1"}, Zones: []string{"eu-west-1a", "eu-west-1b"}, ComputeMode: "fargate", NetworkMode: "private", IngressMode: "load-balancer", ArtifactDigest: "registry.example.invalid/magento@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SchemaFingerprint: "schema", MigrationFingerprint: "migration", OwnershipMarker: "magelift/test/recovery", Resilience: sdk.ResilienceIntent{ProfileID: "aws.ecs", AvailabilityTarget: "99.9", RPOSeconds: 300, RTOSeconds: 1800, RetentionDays: 30, RecoveryScope: "application", FailoverOwner: "operator", FencingPolicy: "single-writer", RecoveryDestinations: []sdk.RecoveryDestination{sdk.RecoverySameRegion}, DataClasses: []sdk.DataClassIntent{{Name: "database", SourceOfTruth: "rds", FailureDomains: []string{"zone"}, BackupMethod: "snapshot", RestoreMethod: "restore", IntegrityMethod: "checksum", LossSemantics: "no-loss-claim", RetentionDays: 30, Encrypted: true, Immutable: true, OwnershipMarker: "magelift/test/recovery/database"}}},
	}
}

func recoveryStageByID(stages []sdk.ResilienceStage, id string) (sdk.ResilienceStage, bool) {
	for _, stage := range stages {
		if stage.ID == id {
			return stage, true
		}
	}
	return sdk.ResilienceStage{}, false
}
