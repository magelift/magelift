package providerhost

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestSubprocessBackendRebuildsConcurrentUpdateError(t *testing.T) {
	fake := &fakeExecuteAPI{execute: func(request ExecuteRequest) (ExecuteResult, error) {
		return ExecuteResult{Operation: request.Operation, ConcurrentUpdate: true}, nil
	}}
	backend := NewSubprocessBackend(fake, sdk.ModulePlan{StackName: "shop-staging"}, "")
	_, err := backend.Update(context.Background(), automation.Request{}, nil)
	var concurrentErr *automation.ConcurrentUpdateError
	if !errors.As(err, &concurrentErr) {
		t.Fatalf("err = %T %v, want concurrent-update error", err, err)
	}
	// The rebuilt error must still classify through the Runner so the CLI
	// exit mapping fires on the subprocess path.
	_, err = automation.NewRunner(backend, io.Discard).Update(context.Background(), automation.Request{Target: sdk.TargetDescriptor{ID: "gcp.gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot"}})
	if !errors.As(err, &concurrentErr) {
		t.Fatalf("runner err = %v, want concurrent-update error", err)
	}
	if !errors.Is(err, automation.ErrUpdateFailed) {
		t.Fatalf("runner err = %v, want the update sentinel in chain", err)
	}
}
