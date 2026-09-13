package certification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type scriptedPoller struct {
	observations []OperationObservation
	errors       []error
	calls        int
}

func (poller *scriptedPoller) Poll(context.Context, string) (OperationObservation, error) {
	index := poller.calls
	poller.calls++
	if index < len(poller.errors) && poller.errors[index] != nil {
		return OperationObservation{}, poller.errors[index]
	}
	if index >= len(poller.observations) {
		return OperationObservation{Status: OperationPending}, nil
	}
	return poller.observations[index], nil
}

func TestWaitForOperationRetriesAndReturnsProviderIdentity(t *testing.T) {
	poller := &scriptedPoller{
		errors:       []error{errors.New("transient")},
		observations: []OperationObservation{{Status: OperationRunning, OperationID: "operation-1"}, {Status: OperationSucceeded, OperationID: "operation-1", ResourceRefs: []string{"resource-1"}}},
	}
	result, err := WaitForOperation(context.Background(), poller, "operation-1", OperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 4})
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationID != "operation-1" || result.Status != OperationSucceeded || poller.calls != 2 {
		t.Fatalf("operation result = %#v calls=%d", result, poller.calls)
	}
}

func TestWaitForOperationNeverTreatsTimeoutOrFailureAsSuccess(t *testing.T) {
	poller := &scriptedPoller{observations: []OperationObservation{{Status: OperationFailed, OperationID: "operation-1", Detail: "denied"}}}
	if _, err := WaitForOperation(context.Background(), poller, "operation-1", OperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2}); err == nil || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("failed operation error = %v", err)
	}

	poller = &scriptedPoller{observations: []OperationObservation{{Status: OperationPending, OperationID: "operation-1"}, {Status: OperationPending, OperationID: "operation-1"}}}
	if _, err := WaitForOperation(context.Background(), poller, "operation-1", OperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2}); err == nil || !strings.Contains(err.Error(), "did not complete") {
		t.Fatalf("bounded operation error = %v", err)
	}
}

func TestWaitForOperationRejectsMissingOrMismatchedIdentity(t *testing.T) {
	for _, observation := range []OperationObservation{
		{Status: OperationSucceeded},
		{Status: OperationSucceeded, OperationID: "operation-2"},
	} {
		poller := &scriptedPoller{observations: []OperationObservation{observation}}
		if _, err := WaitForOperation(context.Background(), poller, "operation-1", OperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 1}); err == nil || !strings.Contains(err.Error(), "identity") {
			t.Fatalf("observation %#v was accepted: %v", observation, err)
		}
	}
}

func TestWaitForOperationHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := WaitForOperation(ctx, &scriptedPoller{}, "operation-1", OperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}
