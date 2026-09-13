package resilience_test

import (
	"context"
	"strings"
	"testing"

	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type projectionBackend struct{}

func (projectionBackend) Rebuild(_ context.Context, request cloudrecovery.ProjectionRequest) (cloudrecovery.ProjectionResult, error) {
	return projectionResult(request), nil
}

func (projectionBackend) Verify(_ context.Context, request cloudrecovery.ProjectionRequest) (cloudrecovery.ProjectionResult, error) {
	return projectionResult(request), nil
}

func projectionResult(request cloudrecovery.ProjectionRequest) cloudrecovery.ProjectionResult {
	return cloudrecovery.ProjectionResult{
		Status:                   sdk.ResilienceOperationSucceeded,
		OperationID:              "runtime-operation-1",
		ResourceReference:        request.TargetReference,
		ProofReferences:          []string{"runtime.projection"},
		FixtureID:                request.FixtureID,
		OwnershipMarker:          request.OwnershipMarker,
		IdempotencyVerified:      true,
		CountsVerified:           request.DataClass == cloudrecovery.ProjectionSearchIndex,
		ApplicationReadsVerified: true,
		PermissionsVerified:      true,
		SecretReferencesVerified: true,
		ServiceHealthVerified:    true,
		CacheLossClassified:      request.DataClass == cloudrecovery.ProjectionCache,
		RestoreDurationSeconds:   2,
		Reason:                   "runtime projection proof completed",
	}
}

func projectionState(action sdk.ResilienceAction, dataClass string) cloudrecovery.OperationState {
	return cloudrecovery.OperationState{
		Version:         cloudrecovery.OperationVersion,
		Action:          action,
		DataClass:       dataClass,
		Resource:        "runtime://source/" + dataClass,
		Target:          "runtime://target/" + dataClass,
		Destination:     sdk.RecoverySameRegionIsolated,
		FixtureID:       "fixture/known-content",
		OwnershipMarker: "magelift/test/projection",
		IdempotencyKey:  "projection/" + dataClass,
	}
}

func TestProjectionOperationSharesExecutionAndObservationSemantics(t *testing.T) {
	t.Parallel()
	lifecycle, err := cloudrecovery.NewProjectionLifecycle(projectionBackend{})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := cloudresilience.NewProjectionOperation("scaleway", lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	for _, dataClass := range []string{cloudrecovery.ProjectionSearchIndex, cloudrecovery.ProjectionCache} {
		dataClass := dataClass
		t.Run(dataClass, func(t *testing.T) {
			t.Parallel()
			state := projectionState(sdk.ResilienceRestore, dataClass)
			result, err := operation.Execute(context.Background(), state)
			if err != nil {
				t.Fatal(err)
			}
			observation := operation.Observation(state, "scaleway-recovery-operation", result)
			if observation.Status != string(sdk.ResilienceOperationSucceeded) || !observation.OwnershipVerified || !observation.IdempotencyVerified {
				t.Fatalf("observation = %#v", observation)
			}
			if len(observation.Evidence) != 1 || observation.Evidence[0].DataClass != dataClass {
				t.Fatalf("evidence = %#v", observation.Evidence)
			}
			if len(observation.ProofRefs) != 2 || observation.ProofRefs[1] != "scaleway."+dataClass+".projection" {
				t.Fatalf("proof references = %#v", observation.ProofRefs)
			}
		})
	}
}

func TestProjectionOperationRejectsDurableBackupAction(t *testing.T) {
	t.Parallel()
	lifecycle, err := cloudrecovery.NewProjectionLifecycle(projectionBackend{})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := cloudresilience.NewProjectionOperation("aws", lifecycle)
	if err != nil {
		t.Fatal(err)
	}
	_, err = operation.Execute(context.Background(), projectionState(sdk.ResilienceBackup, cloudrecovery.ProjectionSearchIndex))
	if err == nil || !strings.Contains(err.Error(), "does not implement action") {
		t.Fatalf("error = %v, want reconstructible-action refusal", err)
	}
}

func TestNewProjectionOperationRejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	if _, err := cloudresilience.NewProjectionOperation("aws", nil); err == nil || !strings.Contains(err.Error(), "lifecycle is required") {
		t.Fatalf("missing lifecycle error = %v", err)
	}
	lifecycle, err := cloudrecovery.NewProjectionLifecycle(projectionBackend{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cloudresilience.NewProjectionOperation("", lifecycle); err == nil || !strings.Contains(err.Error(), "provider is required") {
		t.Fatalf("missing provider error = %v", err)
	}
}
