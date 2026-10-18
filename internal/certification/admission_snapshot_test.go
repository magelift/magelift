package certification

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAdmissionSnapshotMemoizesWithinOneAdmissionScope(t *testing.T) {
	var calls atomic.Int32
	probe := admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		calls.Add(1)
		return readyAdmissionProbeResult(), nil
	})
	snapshot, err := NewAdmissionSnapshot(probe, AdmissionSnapshotOptions{Schema: "schema-1", MaxConcurrent: 2})
	if err != nil {
		t.Fatal(err)
	}
	input := validAdmissionInput()
	result, err := snapshot.Probe(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	result.AvailableQuota["vpc"] = 999
	if _, err := snapshot.Probe(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("probe calls = %d, want one run-scoped read", got)
	}
	result, err = snapshot.Probe(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.AvailableQuota["vpc"] != 2 {
		t.Fatalf("cached quota was mutated through returned result: %#v", result.AvailableQuota)
	}
}

func TestAdmissionSnapshotCoalescesConcurrentProbe(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	probe := admissionProbeFunc(func(ctx context.Context, input AdmissionInput) (AdmissionProbeResult, error) {
		calls.Add(1)
		close(started)
		select {
		case <-release:
			return readyAdmissionProbeResult(), nil
		case <-ctx.Done():
			return AdmissionProbeResult{}, ctx.Err()
		}
	})
	snapshot, err := NewAdmissionSnapshot(probe, AdmissionSnapshotOptions{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	input := validAdmissionInput()
	results := make(chan error, 2)
	for range 2 {
		go func() {
			_, probeErr := snapshot.Probe(context.Background(), input)
			results <- probeErr
		}()
	}
	<-started
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("concurrent probe calls = %d, want one", got)
	}
}

func TestAdmissionSnapshotSeparatesProviderScopeSchemaAndIndependentEvidence(t *testing.T) {
	var calls atomic.Int32
	probe := admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		calls.Add(1)
		return readyAdmissionProbeResult(), nil
	})
	snapshot, err := NewAdmissionSnapshot(probe, AdmissionSnapshotOptions{Schema: "schema-1", MaxConcurrent: 2})
	if err != nil {
		t.Fatal(err)
	}
	input := validAdmissionInput()
	variants := []AdmissionInput{
		input,
		func() AdmissionInput {
			value := input
			value.Region = "eu-west-2"
			return value
		}(),
		func() AdmissionInput {
			value := input
			value.AccountOrProjectRef = "account-2"
			return value
		}(),
		func() AdmissionInput {
			value := input
			value.NetworkRef = "vpc-2"
			return value
		}(),
	}
	for _, value := range variants {
		if _, err := snapshot.Probe(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != int32(len(variants)) {
		t.Fatalf("probe calls = %d, want %d independent scopes", got, len(variants))
	}

	otherSchema, err := NewAdmissionSnapshot(probe, AdmissionSnapshotOptions{Schema: "schema-2", MaxConcurrent: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := otherSchema.Probe(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != int32(len(variants)+1) {
		t.Fatalf("probe calls after schema change = %d, want %d", got, len(variants)+1)
	}
}

func TestAdmissionSnapshotBoundsDistinctProbeConcurrency(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	probe := admissionProbeFunc(func(ctx context.Context, input AdmissionInput) (AdmissionProbeResult, error) {
		started <- struct{}{}
		current := active.Add(1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		defer active.Add(-1)
		select {
		case <-release:
			return readyAdmissionProbeResult(), nil
		case <-ctx.Done():
			return AdmissionProbeResult{}, ctx.Err()
		}
	})
	snapshot, err := NewAdmissionSnapshot(probe, AdmissionSnapshotOptions{MaxConcurrent: 2})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := []AdmissionInput{validAdmissionInput()}
	for index := 1; index < 4; index++ {
		input := validAdmissionInput()
		input.Region = "eu-west-" + string(rune('0'+index+1))
		inputs = append(inputs, input)
	}
	results := make(chan error, len(inputs))
	var waitGroup sync.WaitGroup
	for _, input := range inputs {
		waitGroup.Add(1)
		go func(input AdmissionInput) {
			defer waitGroup.Done()
			_, probeErr := snapshot.Probe(ctx, input)
			results <- probeErr
		}(input)
	}
	for range inputs[:2] {
		<-started
	}
	select {
	case <-started:
		t.Fatal("a third probe bypassed the snapshot concurrency bound")
	default:
	}
	close(release)
	waitGroup.Wait()
	close(results)
	for probeErr := range results {
		if probeErr != nil {
			t.Fatal(probeErr)
		}
	}
	if got := maximum.Load(); got > 2 {
		t.Fatalf("maximum concurrent probes = %d, want at most 2", got)
	}
}

func TestAdmissionSnapshotWaitingCallerCanCancel(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	probe := admissionProbeFunc(func(ctx context.Context, input AdmissionInput) (AdmissionProbeResult, error) {
		close(started)
		select {
		case <-release:
			return readyAdmissionProbeResult(), nil
		case <-ctx.Done():
			return AdmissionProbeResult{}, ctx.Err()
		}
	})
	snapshot, err := NewAdmissionSnapshot(probe, AdmissionSnapshotOptions{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	input := validAdmissionInput()
	ownerResult := make(chan error, 1)
	go func() {
		_, probeErr := snapshot.Probe(context.Background(), input)
		ownerResult <- probeErr
	}()
	<-started

	waiterContext, cancel := context.WithCancel(context.Background())
	cancel()
	waiterInput := input
	waiterInput.Region = "eu-west-2"
	if _, err := snapshot.Probe(waiterContext, waiterInput); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v, want context canceled", err)
	}
	close(release)
	if err := <-ownerResult; err != nil {
		t.Fatal(err)
	}
}

func TestAdmissionSnapshotRejectsSecretLikeResultAndDoesNotCacheIt(t *testing.T) {
	var calls atomic.Int32
	probe := admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		calls.Add(1)
		return AdmissionProbeResult{Reasons: map[string]string{"provider": "password=plaintext-secret"}}, nil
	})
	snapshot, err := NewAdmissionSnapshot(probe, AdmissionSnapshotOptions{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		_, probeErr := snapshot.Probe(context.Background(), validAdmissionInput())
		if probeErr == nil || strings.Contains(probeErr.Error(), "plaintext-secret") {
			t.Fatalf("unsafe snapshot result error = %v", probeErr)
		}
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("unsafe result was cached after %d probe calls, want 2", got)
	}
}

func TestAdmissionSnapshotDoesNotReturnProviderResultOnError(t *testing.T) {
	snapshot, err := NewAdmissionSnapshot(admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		return AdmissionProbeResult{Reasons: map[string]string{"provider": "password=plaintext-secret"}}, errors.New("provider unavailable")
	}), AdmissionSnapshotOptions{MaxConcurrent: 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := snapshot.Probe(context.Background(), validAdmissionInput())
	if err == nil || result.CredentialsReady || result.AccountOrProjectReady || result.NetworkReady || result.StateBackendReady || result.FixtureReady || result.OwnershipReady || len(result.AvailableQuota) != 0 || len(result.Reasons) != 0 || result.AvailableLocalCPUMilli != 0 || result.AvailableLocalMemoryMB != 0 {
		t.Fatalf("provider error result = %#v, %v", result, err)
	}
}

func readyAdmissionProbeResult() AdmissionProbeResult {
	return AdmissionProbeResult{
		CredentialsReady: true, AccountOrProjectReady: true, NetworkReady: true,
		StateBackendReady: true, FixtureReady: true, OwnershipReady: true,
		AvailableQuota:         map[string]int{"vpc": 2},
		AvailableLocalCPUMilli: 1000, AvailableLocalMemoryMB: 1024,
	}
}
