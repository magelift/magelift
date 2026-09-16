package sdk

import (
	"context"
	"strings"
	"testing"
	"time"
)

type deletionClientStub struct {
	observations []DeletionObservation
	calls        int
}

func (stub *deletionClientStub) PollDeletion(_ context.Context, _ string) (DeletionObservation, error) {
	stub.calls++
	index := stub.calls - 1
	if index >= len(stub.observations) {
		index = len(stub.observations) - 1
	}
	return stub.observations[index], nil
}

func TestWaitForDeletionRequiresOwningInventoryToBeEmpty(t *testing.T) {
	client := &deletionClientStub{observations: []DeletionObservation{
		{Status: ResilienceOperationSucceeded, ResourceRefs: []string{"service:pending"}},
		{Status: ResilienceOperationSucceeded},
	}}
	observation, err := WaitForDeletion(context.Background(), client, "magelift/test/cleanup", ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	if client.calls != 2 || observation.Status != ResilienceOperationSucceeded {
		t.Fatalf("observation = %#v calls=%d", observation, client.calls)
	}

	client = &deletionClientStub{observations: []DeletionObservation{{Status: ResilienceOperationSucceeded, DelayedResourceRefs: []string{"service:tombstone"}}}}
	if _, err := WaitForDeletion(context.Background(), client, "magelift/test/cleanup", ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 1}); err == nil || !strings.Contains(err.Error(), "did not complete") {
		t.Fatalf("delayed tombstone was accepted: %v", err)
	}
}

func TestWaitForDeletionRejectsUnknownAndFailedStates(t *testing.T) {
	for _, observation := range []DeletionObservation{
		{Status: "unknown"},
		{Status: ResilienceOperationFailed, Detail: "provider outage"},
	} {
		client := &deletionClientStub{observations: []DeletionObservation{observation}}
		if _, err := WaitForDeletion(context.Background(), client, "magelift/test/cleanup", ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 1}); err == nil {
			t.Fatalf("observation %#v was accepted", observation)
		}
	}
}
