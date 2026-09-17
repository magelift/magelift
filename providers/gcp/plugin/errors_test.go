package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"github.com/magelift/magelift/internal/automation"
	gcpstate "github.com/magelift/magelift/providers/gcp/state"
	"github.com/magelift/magelift/sdk"
)

func TestHandlersRejectNilRequests(t *testing.T) {
	t.Parallel()
	server := &Server{}
	ctx := context.Background()
	cases := map[string]func() *sdk.OperationError{
		"describe":            func() *sdk.OperationError { _, e := server.Describe(ctx, nil); return e },
		"validate-config":     func() *sdk.OperationError { _, e := server.ValidateConfig(ctx, nil); return e },
		"plan":                func() *sdk.OperationError { _, e := server.Plan(ctx, nil); return e },
		"preview":             func() *sdk.OperationError { _, e := server.Preview(ctx, nil); return e },
		"apply":               func() *sdk.OperationError { _, e := server.Apply(ctx, nil); return e },
		"outputs":             func() *sdk.OperationError { _, e := server.Outputs(ctx, nil); return e },
		"destroy":             func() *sdk.OperationError { _, e := server.Destroy(ctx, nil); return e },
		"destroy-leftover":    func() *sdk.OperationError { _, e := server.DestroyLeftoverBackups(ctx, nil); return e },
		"bootstrap-verify":    func() *sdk.OperationError { _, e := server.BootstrapVerify(ctx, nil); return e },
		"bootstrap-ensure":    func() *sdk.OperationError { _, e := server.BootstrapEnsure(ctx, nil); return e },
		"state-status":        func() *sdk.OperationError { _, e := server.StateStatus(ctx, nil); return e },
		"state-lock":          func() *sdk.OperationError { _, e := server.StateLock(ctx, nil); return e },
		"state-unlock":        func() *sdk.OperationError { _, e := server.StateUnlock(ctx, nil); return e },
		"state-backup":        func() *sdk.OperationError { _, e := server.StateBackup(ctx, nil); return e },
		"state-restore":       func() *sdk.OperationError { _, e := server.StateRestore(ctx, nil); return e },
		"secret-list":         func() *sdk.OperationError { _, e := server.SecretList(ctx, nil); return e },
		"secret-set":          func() *sdk.OperationError { _, e := server.SecretSet(ctx, nil); return e },
		"secret-remove":       func() *sdk.OperationError { _, e := server.SecretRemove(ctx, nil); return e },
		"secret-read":         func() *sdk.OperationError { _, e := server.SecretRead(ctx, nil); return e },
		"tail-logs":           func() *sdk.OperationError { _, e := server.TailLogs(ctx, nil); return e },
		"check-runtime":       func() *sdk.OperationError { _, e := server.CheckRuntime(ctx, nil); return e },
		"prepare-exec":        func() *sdk.OperationError { _, e := server.PrepareExec(ctx, nil); return e },
		"prepare-tunnel":      func() *sdk.OperationError { _, e := server.PrepareTunnel(ctx, nil); return e },
		"cost-inputs":         func() *sdk.OperationError { _, e := server.CostInputs(ctx, nil); return e },
		"inventory":           func() *sdk.OperationError { _, e := server.Inventory(ctx, nil); return e },
		"delete":              func() *sdk.OperationError { _, e := server.Delete(ctx, nil); return e },
		"edge-plan":           func() *sdk.OperationError { _, e := server.EdgePlan(ctx, nil); return e },
		"edge-execute":        func() *sdk.OperationError { _, e := server.EdgeExecute(ctx, nil); return e },
		"resilience-plan":     func() *sdk.OperationError { _, e := server.ResiliencePlan(ctx, nil); return e },
		"resilience-execute":  func() *sdk.OperationError { _, e := server.ResilienceExecute(ctx, nil); return e },
		"nil-server-describe": func() *sdk.OperationError { _, e := (*Server)(nil).Describe(ctx, nil); return e },
	}
	for name, call := range cases {
		if operr := call(); operr == nil || operr.Code != sdk.ErrCodeInvalid {
			t.Errorf("%s: nil request error = %v", name, operr)
		}
	}
}

func TestMapError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		err       error
		code      sdk.OperationErrorCode
		retryable bool
	}{
		{"credential-401", &googleapi.Error{Code: 401}, sdk.ErrCodeCredential, false},
		{"credential-403", &googleapi.Error{Code: 403}, sdk.ErrCodeCredential, false},
		{"not-found", &googleapi.Error{Code: 404}, sdk.ErrCodeNotFound, false},
		{"conflict", &googleapi.Error{Code: 409}, sdk.ErrCodeConflict, false},
		{"upstream-retryable", &googleapi.Error{Code: 503}, sdk.ErrCodeUpstream, true},
		{"upstream", &googleapi.Error{Code: 400}, sdk.ErrCodeUpstream, false},
		{"concurrent", errors.New("[409] Conflict: Another update is currently in progress."), sdk.ErrCodeConflict, true},
		{"refresh-failure", &oauth2.RetrieveError{ErrorCode: "invalid_grant"}, sdk.ErrCodeCredential, false},
		{"locked", gcpstate.ErrLocked, sdk.ErrCodeConflict, false},
		{"ownership", &automation.PreviewOwnershipError{}, sdk.ErrCodeConflict, false},
		{"generic", errors.New("boom"), sdk.ErrCodeUpstream, false},
	}
	for _, test := range cases {
		if operr := mapError(test.err); operr == nil || operr.Code != test.code || operr.Retryable != test.retryable {
			t.Errorf("%s: error = %v", test.name, operr)
		}
	}
	if operr := mapError(&oauth2.RetrieveError{ErrorCode: "invalid_grant"}); !strings.Contains(operr.Message, "re-authenticate") {
		t.Errorf("refresh failure message = %q, want re-authentication guidance", operr.Message)
	}
	if mapError(nil) != nil {
		t.Error("mapError(nil) is not nil")
	}
}

func TestDefaultTimeoutsCoverAllOperations(t *testing.T) {
	t.Parallel()
	timeouts := DefaultTimeouts()
	if len(timeouts) != len(sdk.PluginMethods) {
		t.Fatalf("timeouts = %d, want %d", len(timeouts), len(sdk.PluginMethods))
	}
	for operation := range sdk.PluginMethods {
		timeout, ok := timeouts[operation]
		if !ok || timeout.Duration <= 0 {
			t.Errorf("operation %q has no timeout", operation)
		}
	}
	mutations := []sdk.Operation{
		sdk.OpApply, sdk.OpDestroy, sdk.OpDestroyLeftoverBackups,
		sdk.OpBootstrapEnsure,
		sdk.OpStateLock, sdk.OpStateUnlock, sdk.OpStateBackup, sdk.OpStateRestore,
		sdk.OpSecretSet, sdk.OpSecretRemove,
		sdk.OpDelete, sdk.OpEdgeExecute, sdk.OpResilienceExecute,
		sdk.OpDeployAppPhase,
	}
	for _, operation := range mutations {
		if timeouts[operation].RetryableOnTimeout {
			t.Errorf("mutation %q is retryable on timeout", operation)
		}
	}
	reads := []sdk.Operation{
		sdk.OpDescribe, sdk.OpValidateConfig, sdk.OpPlan, sdk.OpPreview, sdk.OpOutputs,
		sdk.OpBootstrapVerify, sdk.OpStateStatus,
		sdk.OpSecretList, sdk.OpSecretRead,
		sdk.OpTailLogs, sdk.OpCheckRuntime, sdk.OpPrepareExec, sdk.OpPrepareTunnel,
		sdk.OpCostInputs, sdk.OpInventory,
		sdk.OpEdgePlan, sdk.OpResiliencePlan,
	}
	for _, operation := range reads {
		if !timeouts[operation].RetryableOnTimeout {
			t.Errorf("read %q is not retryable on timeout", operation)
		}
	}
	if len(mutations)+len(reads) != len(sdk.PluginMethods) {
		t.Errorf("classified %d of %d operations; every op needs an effect class", len(mutations)+len(reads), len(sdk.PluginMethods))
	}
}
