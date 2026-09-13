package recovery

import (
	"context"
	"strings"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

type fakeProjectionBackend struct {
	rebuild ProjectionResult
	verify  ProjectionResult
}

func (f fakeProjectionBackend) Rebuild(context.Context, ProjectionRequest) (ProjectionResult, error) {
	return f.rebuild, nil
}

func (f fakeProjectionBackend) Verify(context.Context, ProjectionRequest) (ProjectionResult, error) {
	return f.verify, nil
}

func validProjectionRequest(dataClass string, action sdk.ResilienceAction) ProjectionRequest {
	return ProjectionRequest{
		Action: action, DataClass: dataClass, SourceReference: "runtime://source",
		TargetReference: "runtime://target", Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: "fixture-1", OwnershipMarker: "magelift/test/projection", IdempotencyKey: "projection-1",
	}
}

func validProjectionResult(request ProjectionRequest) ProjectionResult {
	return ProjectionResult{
		Status: sdk.ResilienceOperationSucceeded, ResourceReference: request.TargetReference,
		FixtureID: request.FixtureID, OwnershipMarker: request.OwnershipMarker,
		IdempotencyVerified: true, CountsVerified: true, ApplicationReadsVerified: true,
		PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true, RestoreDurationSeconds: 4,
	}
}

func TestProjectionLifecycleValidatesSearchAndCacheProofsInOneCorePath(t *testing.T) {
	t.Parallel()
	for _, dataClass := range []string{ProjectionSearchIndex, ProjectionCache} {
		dataClass := dataClass
		t.Run(dataClass, func(t *testing.T) {
			t.Parallel()
			request := validProjectionRequest(dataClass, sdk.ResilienceRestore)
			result := validProjectionResult(request)
			if dataClass == ProjectionCache {
				result.CountsVerified = false
				result.CacheLossClassified = true
			}
			lifecycle, err := NewProjectionLifecycle(fakeProjectionBackend{rebuild: result})
			if err != nil {
				t.Fatal(err)
			}
			got, err := lifecycle.Rebuild(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if got.ResourceReference != request.TargetReference {
				t.Fatalf("resource reference = %q, want %q", got.ResourceReference, request.TargetReference)
			}
			proof := ProjectionProof(request, got)
			if proof.DataClass != dataClass || !proof.ApplicationReadsVerified || !proof.ServiceHealthVerified {
				t.Fatalf("projection proof = %#v", proof)
			}
		})
	}
}

func TestProjectionLifecycleRejectsCacheWithoutLossClassification(t *testing.T) {
	t.Parallel()
	request := validProjectionRequest(ProjectionCache, sdk.ResilienceRestore)
	result := validProjectionResult(request)
	lifecycle, err := NewProjectionLifecycle(fakeProjectionBackend{rebuild: result})
	if err != nil {
		t.Fatal(err)
	}
	_, err = lifecycle.Rebuild(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "classify cache loss") {
		t.Fatalf("error = %v, want cache-loss classification failure", err)
	}
}

func TestProjectionLifecycleRejectsSearchWithoutCompletenessProof(t *testing.T) {
	t.Parallel()
	request := validProjectionRequest(ProjectionSearchIndex, sdk.ResilienceRestore)
	result := validProjectionResult(request)
	result.CountsVerified = false
	lifecycle, err := NewProjectionLifecycle(fakeProjectionBackend{rebuild: result})
	if err != nil {
		t.Fatal(err)
	}
	_, err = lifecycle.Rebuild(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "document counts or a verified manifest") {
		t.Fatalf("error = %v, want search completeness failure", err)
	}
}

func TestProjectionLifecycleRejectsSuccessfulResultWithoutPermissionsOrSecretReferences(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*ProjectionResult){
		"permissions":       func(result *ProjectionResult) { result.PermissionsVerified = false },
		"secret references": func(result *ProjectionResult) { result.SecretReferencesVerified = false },
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			request := validProjectionRequest(ProjectionSearchIndex, sdk.ResilienceRestore)
			result := validProjectionResult(request)
			mutate(&result)
			lifecycle, err := NewProjectionLifecycle(fakeProjectionBackend{rebuild: result})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := lifecycle.Rebuild(context.Background(), request); err == nil || !strings.Contains(err.Error(), "permission, secret-reference") {
				t.Fatalf("error = %v, want missing proof failure", err)
			}
		})
	}
}

func TestProjectionLifecycleRejectsRestoreWithoutMeasuredDuration(t *testing.T) {
	t.Parallel()
	request := validProjectionRequest(ProjectionSearchIndex, sdk.ResilienceRestore)
	result := validProjectionResult(request)
	result.RestoreDurationSeconds = 0
	lifecycle, err := NewProjectionLifecycle(fakeProjectionBackend{rebuild: result})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.Rebuild(context.Background(), request); err == nil || !strings.Contains(err.Error(), "positive restore duration") {
		t.Fatalf("error = %v, want positive-duration failure", err)
	}
}

func TestProjectionLifecycleAcceptsPendingResultAndRequiresOperationID(t *testing.T) {
	t.Parallel()
	request := validProjectionRequest(ProjectionSearchIndex, sdk.ResilienceRestore)
	result := validProjectionResult(request)
	result.Status = sdk.ResilienceOperationPending
	result.ResourceReference = ""
	result.OperationID = "projection-operation-1"
	lifecycle, err := NewProjectionLifecycle(fakeProjectionBackend{rebuild: result})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.Rebuild(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	result.OperationID = ""
	lifecycle, err = NewProjectionLifecycle(fakeProjectionBackend{rebuild: result})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.Rebuild(context.Background(), request); err == nil || !strings.Contains(err.Error(), "operation ID") {
		t.Fatal("pending projection without operation ID was accepted")
	}
}

func TestProjectionLifecycleRequiresContextAndBackend(t *testing.T) {
	t.Parallel()
	if _, err := NewProjectionLifecycle(nil); err == nil {
		t.Fatal("nil projection backend was accepted")
	}
	lifecycle, err := NewProjectionLifecycle(fakeProjectionBackend{})
	if err != nil {
		t.Fatal(err)
	}
	var missingContext context.Context
	_, err = lifecycle.Rebuild(missingContext, validProjectionRequest(ProjectionSearchIndex, sdk.ResilienceRestore))
	if err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("error = %v, want context validation", err)
	}
}
