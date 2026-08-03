package automation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

var validRequest = Request{Target: sdk.TargetDescriptor{ID: "aws.ecs-fargate", Provider: "aws", Runtime: "ecs-fargate"}}

type mockBackend struct {
	mu          sync.Mutex
	operation   string
	request     Request
	diagnostics io.Writer
	changes     map[string]int
	err         error
}

func (b *mockBackend) record(operation string, request Request, diagnostics io.Writer) (map[string]int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.operation = operation
	b.request = request
	b.diagnostics = diagnostics
	result := make(map[string]int, len(b.changes))
	for name, count := range b.changes {
		result[name] = count
	}
	return result, b.err
}

func (b *mockBackend) Preview(_ context.Context, request Request, diagnostics io.Writer) (map[string]int, error) {
	return b.record("preview", request, diagnostics)
}

func (b *mockBackend) Update(_ context.Context, request Request, diagnostics io.Writer) (map[string]int, error) {
	return b.record("update", request, diagnostics)
}

func (b *mockBackend) Destroy(_ context.Context, request Request, diagnostics io.Writer) (map[string]int, error) {
	return b.record("destroy", request, diagnostics)
}

func TestRunnerReturnsStableStructuredSummary(t *testing.T) {
	diagnostics := &bytes.Buffer{}
	backend := &mockBackend{changes: map[string]int{"update": 2, "same": 0, "create": 1}}
	summary, err := NewRunner(backend, diagnostics).Preview(context.Background(), validRequest)
	if err != nil {
		t.Fatal(err)
	}
	want := ChangeSummary{Changes: []Change{{Operation: "create", Count: 1}, {Operation: "update", Count: 2}}, Total: 3}
	if !reflect.DeepEqual(summary, want) {
		t.Fatalf("summary = %#v, want %#v", summary, want)
	}
	if backend.operation != "preview" || backend.request != validRequest || backend.diagnostics != diagnostics {
		t.Fatal("preview request was not passed to the backend")
	}
}

func TestRunnerSelectsEachOperation(t *testing.T) {
	tests := []struct {
		name string
		run  func(*Runner) (ChangeSummary, error)
	}{
		{name: "preview", run: func(r *Runner) (ChangeSummary, error) { return r.Preview(context.Background(), validRequest) }},
		{name: "update", run: func(r *Runner) (ChangeSummary, error) { return r.Update(context.Background(), validRequest) }},
		{name: "destroy", run: func(r *Runner) (ChangeSummary, error) { return r.Destroy(context.Background(), validRequest) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &mockBackend{}
			if _, err := test.run(NewRunner(backend, io.Discard)); err != nil {
				t.Fatal(err)
			}
			if backend.operation != test.name {
				t.Fatalf("operation = %q", backend.operation)
			}
		})
	}
}

func TestRunnerPreservesCancellation(t *testing.T) {
	cause := errors.New("preview canceled")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	backend := &mockBackend{}
	_, err := NewRunner(backend, io.Discard).Preview(ctx, validRequest)
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v", err)
	}
	if backend.operation != "" {
		t.Fatal("backend was called after cancellation")
	}
}

func TestRunnerRedactsBackendFailure(t *testing.T) {
	backend := &mockBackend{err: errors.New("provider output with sensitive-token")}
	_, err := NewRunner(backend, io.Discard).Update(context.Background(), validRequest)
	if !errors.Is(err, ErrUpdateFailed) || strings.Contains(err.Error(), "sensitive-token") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunnerValidatesBoundaryBeforeBackend(t *testing.T) {
	tests := []struct {
		name    string
		runner  *Runner
		request Request
	}{
		{name: "missing backend", runner: NewRunner(nil, io.Discard), request: validRequest},
		{name: "missing diagnostics", runner: NewRunner(&mockBackend{}, nil), request: validRequest},
		{name: "invalid target", runner: NewRunner(&mockBackend{}, io.Discard), request: Request{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.runner.Destroy(context.Background(), test.request); err == nil {
				t.Fatal("invalid request was accepted")
			}
		})
	}
}

func TestPulumiBackendRejectsMissingStack(t *testing.T) {
	backend := NewPulumiBackend(nil)
	if _, err := backend.Preview(context.Background(), validRequest, io.Discard); !errors.Is(err, ErrPulumiStackRequired) {
		t.Fatalf("error = %v", err)
	}
}
