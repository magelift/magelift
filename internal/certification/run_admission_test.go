package certification

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	provider "github.com/magelift/magelift/internal/provider"
)

func TestNewCertificationRunAdmissionRequiresFullDeclaredInputs(t *testing.T) {
	probe := &ProviderAdmissionProbe{}
	tests := []struct {
		name   string
		inputs []AdmissionInput
		want   string
	}{
		{name: "nil inputs", want: "at least one full admission input"},
		{name: "missing provider", inputs: []AdmissionInput{mutateRunAdmissionInput(func(input *AdmissionInput) { input.Provider = "" })}, want: "input 0 is invalid"},
		{name: "missing account or project", inputs: []AdmissionInput{mutateRunAdmissionInput(func(input *AdmissionInput) { input.AccountOrProjectRef = "" })}, want: "input 0 is invalid"},
		{name: "missing region", inputs: []AdmissionInput{mutateRunAdmissionInput(func(input *AdmissionInput) { input.Region = "" })}, want: "input 0 is invalid"},
		{name: "missing full boundary", inputs: []AdmissionInput{mutateRunAdmissionInput(func(input *AdmissionInput) { input.StateBackendRef = "" })}, want: "input 0 is invalid"},
		{name: "unsafe credential value", inputs: []AdmissionInput{mutateRunAdmissionInput(func(input *AdmissionInput) { input.CredentialRefs = []string{"token=super-secret-value"} })}, want: "input 0 is invalid"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewCertificationRunAdmission(probe, CertificationRunAdmissionOptions{RequiredInputs: test.inputs})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("constructor error = %v, want substring %q", err, test.want)
			}
			if strings.Contains(err.Error(), "super-secret-value") {
				t.Fatal("constructor error leaked credential material")
			}
		})
	}
}

func TestNewCertificationRunAdmissionRejectsInvalidProbeAndConcurrency(t *testing.T) {
	input := runAdmissionTestInput()
	for _, test := range []struct {
		name  string
		probe AdmissionProbe
		max   int
		want  string
	}{
		{name: "nil probe", want: "probe is required"},
		{name: "zero is default", probe: &ProviderAdmissionProbe{}, max: 0},
		{name: "negative concurrency", probe: &ProviderAdmissionProbe{}, max: -1, want: "max concurrency"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "zero is default" {
				run, err := NewCertificationRunAdmission(test.probe, CertificationRunAdmissionOptions{RequiredInputs: []AdmissionInput{input}})
				if err != nil || run.Snapshot() == nil {
					t.Fatalf("default snapshot = %v, %v", run, err)
				}
				return
			}
			_, err := NewCertificationRunAdmission(test.probe, CertificationRunAdmissionOptions{
				RequiredInputs: []AdmissionInput{input},
				Snapshot:       AdmissionSnapshotOptions{MaxConcurrent: test.max},
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("constructor error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestCertificationRunAdmissionConstructsOneSnapshotAndCopiesInputs(t *testing.T) {
	input := runAdmissionTestInput()
	probe := readyProviderAdmissionProbe()
	run, err := NewCertificationRunAdmission(probe, CertificationRunAdmissionOptions{RequiredInputs: []AdmissionInput{input}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := run.Snapshot()
	if snapshot == nil || snapshot != run.Snapshot() {
		t.Fatal("run did not retain exactly one snapshot")
	}

	input.MutationKeys[0] = "mutated-after-construction"
	input.RequiredQuota["vpc"] = 0
	inputs := run.RequiredInputs()
	if inputs[0].MutationKeys[0] == input.MutationKeys[0] || inputs[0].RequiredQuota["vpc"] == input.RequiredQuota["vpc"] {
		t.Fatalf("run input was not defensively copied: %#v", inputs)
	}
	inputs[0].MutationKeys[0] = "mutated-return-value"
	if run.RequiredInputs()[0].MutationKeys[0] == "mutated-return-value" {
		t.Fatal("required-input accessor exposed mutable state")
	}
}

func TestCertificationRunAdmissionAdmitsDeclaredScopesAndRejectsUndeclaredScope(t *testing.T) {
	input := runAdmissionTestInput()
	var probes atomic.Int32
	probe := readyProviderAdmissionProbe()
	probe.Network = countingAdmissionFact{count: &probes, delegate: StaticAdmissionFact(true)}
	run, err := NewCertificationRunAdmission(probe, CertificationRunAdmissionOptions{RequiredInputs: []AdmissionInput{input}})
	if err != nil {
		t.Fatal(err)
	}

	result, err := run.Probe(context.Background(), input)
	if err != nil || !result.AccountOrProjectReady {
		t.Fatalf("declared scope probe = %#v, %v", result, err)
	}
	undeclared := input
	undeclared.NetworkRef = "network-2"
	undeclared.MutationKeys = []string{"aws/account-1/eu-west-1/network-2"}
	if _, err := run.Probe(context.Background(), undeclared); err == nil || !strings.Contains(err.Error(), "not a declared scope") {
		t.Fatalf("undeclared scope error = %v", err)
	}
	if got := probes.Load(); got != 1 {
		t.Fatalf("provider probes = %d, want one", got)
	}
}

func TestCertificationRunAdmissionAdmitCoalescesAndReportsAllScopes(t *testing.T) {
	input := runAdmissionTestInput()
	var probes atomic.Int32
	probe := readyProviderAdmissionProbe()
	probe.Network = countingAdmissionFact{count: &probes, delegate: StaticAdmissionFact(true)}
	run, err := NewCertificationRunAdmission(probe, CertificationRunAdmissionOptions{
		RequiredInputs: []AdmissionInput{input, input},
		Snapshot:       AdmissionSnapshotOptions{MaxConcurrent: 2},
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := run.Admit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Ready || len(report.Scopes) != 2 {
		t.Fatalf("run admission report = %#v", report)
	}
	if report.Scopes[0].InputIndex != 0 || report.Scopes[1].InputIndex != 1 {
		t.Fatalf("scope report order = %#v", report.Scopes)
	}
	if got := probes.Load(); got != 1 {
		t.Fatalf("coalesced provider probes = %d, want one", got)
	}
}

func TestCertificationRunAdmissionBoundsConcurrentProviderProbes(t *testing.T) {
	inputs := make([]AdmissionInput, 4)
	for index := range inputs {
		inputs[index] = runAdmissionTestInput()
		inputs[index].NetworkRef = "network-" + string(rune('a'+index))
		inputs[index].MutationKeys = []string{"aws/account-1/eu-west-1/network-" + string(rune('a'+index))}
	}
	started := make(chan struct{}, len(inputs))
	release := make(chan struct{})
	probe := readyProviderAdmissionProbe()
	probe.Network = gatedAdmissionFact{started: started, release: release}
	run, err := NewCertificationRunAdmission(probe, CertificationRunAdmissionOptions{
		RequiredInputs: inputs,
		Snapshot:       AdmissionSnapshotOptions{MaxConcurrent: 2},
	})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := run.Admit(context.Background())
		done <- err
	}()
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for bounded provider probes")
		}
	}
	select {
	case <-started:
		t.Fatal("provider probe concurrency exceeded configured bound")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("run admission did not finish after provider probes were released")
	}
}

func TestCertificationRunAdmissionProviderFailureIsBlockedWithoutSecret(t *testing.T) {
	probe := readyProviderAdmissionProbe()
	probe.Network = secretAdmissionFact{}
	run, err := NewCertificationRunAdmission(probe, CertificationRunAdmissionOptions{RequiredInputs: []AdmissionInput{runAdmissionTestInput()}})
	if err != nil {
		t.Fatal(err)
	}

	report, err := run.Admit(context.Background())
	if err != nil || report.Ready || len(report.Scopes) != 1 {
		t.Fatalf("blocked run admission = %#v, %v", report, err)
	}
	if report.Scopes[0].Admission.Ready || len(report.Scopes[0].Admission.BlockingReasons) == 0 {
		t.Fatalf("blocked scope report = %#v", report.Scopes[0])
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "super-secret-value") || strings.Contains(string(data), "aws-identity://") {
		t.Fatalf("run admission report contains credential material: %s", data)
	}
}

func TestCertificationRunAdmissionCancellationReturnsAndDoesNotLeakWorkers(t *testing.T) {
	probe := readyProviderAdmissionProbe()
	probe.Network = gatedAdmissionFact{started: make(chan struct{}, 1), release: make(chan struct{})}
	run, err := NewCertificationRunAdmission(probe, CertificationRunAdmissionOptions{RequiredInputs: []AdmissionInput{runAdmissionTestInput()}})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := run.Admit(ctx)
		done <- err
	}()
	select {
	case <-probe.Network.(gatedAdmissionFact).started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for provider probe")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled run admission did not return")
	}
}

func readyProviderAdmissionProbe() *ProviderAdmissionProbe {
	return &ProviderAdmissionProbe{
		Identity: runAdmissionIdentityClientFunc(func(_ context.Context, request provider.IdentityCheckRequest) (provider.IdentityObservation, error) {
			return provider.IdentityObservation{
				Provider: request.Provider, AccountOrProjectRef: request.AccountOrProjectRef,
				Region: request.Region, PrincipalRef: "principal", LeastPrivilegeVerified: true,
			}, nil
		}),
		Credentials: provider.CredentialResolverFunc(func(_ context.Context, _ string, consume func([]byte) error) error {
			return consume([]byte("opaque"))
		}),
		Network:      StaticAdmissionFact(true),
		StateBackend: StaticAdmissionFact(true), Fixture: StaticAdmissionFact(true), Ownership: StaticAdmissionFact(true),
		Quota:          StaticAdmissionQuota{"vpc": 2},
		LocalResources: StaticAdmissionLocalResources{CPUMilli: 1000, MemoryMB: 1024},
	}
}

func mutateRunAdmissionInput(mutate func(*AdmissionInput)) AdmissionInput {
	input := runAdmissionTestInput()
	mutate(&input)
	return input
}

func runAdmissionTestInput() AdmissionInput {
	return AdmissionInput{
		Provider: "aws", AccountOrProjectRef: "account-1", Region: "eu-west-1", NetworkRef: "vpc-1",
		StateBackendRef: "s3://state-bucket/magelift", FixtureID: "fixture-1", OwnershipMarker: "magelift/run-1",
		CredentialRefs: []string{"aws-identity://role/magelift-certifier"}, RequireCredentials: true,
		RequiredQuota: map[string]int{"vpc": 1}, AvailableQuota: map[string]int{"vpc": 2},
		RequiredLocalCPUMilli: 500, AvailableLocalCPUMilli: 1000, RequiredLocalMemoryMB: 512, AvailableLocalMemoryMB: 1024,
		MutationKeys: []string{"aws/account-1/eu-west-1", "state/s3://state-bucket/magelift"},
	}
}

type runAdmissionIdentityClientFunc func(context.Context, provider.IdentityCheckRequest) (provider.IdentityObservation, error)

func (f runAdmissionIdentityClientFunc) CheckIdentity(ctx context.Context, request provider.IdentityCheckRequest) (provider.IdentityObservation, error) {
	return f(ctx, request)
}

type countingAdmissionFact struct {
	count    *atomic.Int32
	delegate AdmissionFactChecker
}

func (fact countingAdmissionFact) Check(ctx context.Context, input AdmissionInput) (AdmissionFact, error) {
	fact.count.Add(1)
	return fact.delegate.Check(ctx, input)
}

type gatedAdmissionFact struct {
	started chan struct{}
	release <-chan struct{}
}

type secretAdmissionFact struct{}

func (secretAdmissionFact) Check(context.Context, AdmissionInput) (AdmissionFact, error) {
	return AdmissionFact{Reason: "token=super-secret-value"}, nil
}

func (fact gatedAdmissionFact) Check(ctx context.Context, input AdmissionInput) (AdmissionFact, error) {
	select {
	case fact.started <- struct{}{}:
	case <-ctx.Done():
		return AdmissionFact{}, ctx.Err()
	}
	select {
	case <-fact.release:
		return AdmissionFact{Ready: true}, nil
	case <-ctx.Done():
		return AdmissionFact{}, ctx.Err()
	}
}
