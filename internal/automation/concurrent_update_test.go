package automation

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunnerWrapsBackendCause(t *testing.T) {
	backend := &mockBackend{err: errors.New("engine panicked on resource urn:pulumi:staging::x")}
	_, err := NewRunner(backend, &bytes.Buffer{}).Update(context.Background(), validRequest)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrUpdateFailed) {
		t.Fatalf("err = %v, want ErrUpdateFailed in chain", err)
	}
	if !strings.Contains(err.Error(), "engine panicked") {
		t.Fatalf("err = %v, want the backend cause in the message", err)
	}
}

func TestRunnerClassifiesConcurrentUpdate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cause    error
		sentinel error
		run      func(*Runner, context.Context, Request) (ChangeSummary, error)
	}{
		{"preview lock", errors.New("update failed: the stack is currently locked by alice"), ErrPreviewFailed, (*Runner).Preview},
		{"update conflict", errors.New("[409] Conflict: Another update is currently in progress."), ErrUpdateFailed, (*Runner).Update},
		{"destroy lock", errors.New("the stack is currently locked by bob"), ErrDestroyFailed, (*Runner).Destroy},
		{"typed passes through once", &ConcurrentUpdateError{Cause: errors.New("lock")}, ErrUpdateFailed, (*Runner).Update},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := &mockBackend{err: tc.cause}
			_, err := tc.run(NewRunner(backend, &bytes.Buffer{}), context.Background(), validRequest)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("err = %v, want the operation sentinel in chain", err)
			}
			var concurrentErr *ConcurrentUpdateError
			if !errors.As(err, &concurrentErr) {
				t.Fatalf("err = %v, want ConcurrentUpdateError in chain", err)
			}
			if got := strings.Count(err.Error(), "another Pulumi update is currently in progress"); got != 1 {
				t.Fatalf("err = %v, want the collision sentence exactly once", err)
			}
		})
	}
}

func TestIsConcurrentUpdate(t *testing.T) {
	if IsConcurrentUpdate(nil) {
		t.Fatal("nil must not classify")
	}
	if IsConcurrentUpdate(errors.New("some graph bug")) {
		t.Fatal("generic failure must not classify")
	}
	if !IsConcurrentUpdate(errors.New("x: the stack is currently locked by ci")) {
		t.Fatal("lock text must classify")
	}
	if !IsConcurrentUpdate(&ConcurrentUpdateError{}) {
		t.Fatal("typed error must classify")
	}
}
