package certification

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type admissionProbeFunc func(context.Context, AdmissionInput) (AdmissionProbeResult, error)

func (f admissionProbeFunc) Probe(ctx context.Context, input AdmissionInput) (AdmissionProbeResult, error) {
	return f(ctx, input)
}

func validAdmissionInput() AdmissionInput {
	return AdmissionInput{
		Provider: "aws", AccountOrProjectRef: "account-1", Region: "eu-west-1", NetworkRef: "vpc-1",
		StateBackendRef: "s3://state-bucket/magelift", FixtureID: "fixture-1", OwnershipMarker: "magelift/run-1",
		CredentialRefs: []string{"aws-identity://role/magelift-certifier"}, RequireCredentials: true,
		RequiredQuota: map[string]int{"vpc": 1}, AvailableQuota: map[string]int{"vpc": 2},
		RequiredLocalCPUMilli: 500, AvailableLocalCPUMilli: 1000, RequiredLocalMemoryMB: 512, AvailableLocalMemoryMB: 1024,
		MutationKeys: []string{"aws/account-1/eu-west-1", "state/s3://state-bucket/magelift"},
	}
}

func TestCheckAdmissionReportsReadyOnlyWhenAllCapacityChecksPass(t *testing.T) {
	report, err := CheckAdmission(validAdmissionInput())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Ready || len(report.BlockingReasons) != 0 {
		t.Fatalf("admission report = %#v", report)
	}

	input := validAdmissionInput()
	input.AvailableQuota["vpc"] = 0
	report, err = CheckAdmission(input)
	if err != nil {
		t.Fatal(err)
	}
	if report.Ready || len(report.BlockingReasons) != 1 || !strings.Contains(report.BlockingReasons[0], "quota") {
		t.Fatalf("blocked admission report = %#v", report)
	}
}

func TestCheckAdmissionRejectsMalformedOrUnsafeReferences(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AdmissionInput)
	}{
		{name: "missing mutation key", mutate: func(input *AdmissionInput) { input.MutationKeys = nil }},
		{name: "plaintext credential", mutate: func(input *AdmissionInput) { input.CredentialRefs = []string{"token=abcdefghijkl"} }},
		{name: "network credential", mutate: func(input *AdmissionInput) { input.CredentialRefs = []string{"https://example.invalid/token"} }},
		{name: "duplicate mutation key", mutate: func(input *AdmissionInput) { input.MutationKeys = append(input.MutationKeys, input.MutationKeys[0]) }},
		{name: "negative quota", mutate: func(input *AdmissionInput) { input.RequiredQuota["vpc"] = -1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validAdmissionInput()
			test.mutate(&input)
			if _, err := CheckAdmission(input); err == nil {
				t.Fatal("unsafe admission input was accepted")
			}
		})
	}
}

func TestBuildScheduleRunsAdmissionBeforeReturningUnits(t *testing.T) {
	cells := []ScheduleCell{{ID: "baseline", Fingerprint: strings.Repeat("a", 64), WarmBoundary: "aws", MutationKeys: []string{"aws/account-1/eu-west-1"}, OwnershipMarker: "magelift/run-1", Status: CapabilityCompatible}}
	input := validAdmissionInput()
	input.AvailableLocalCPUMilli = 0
	if _, err := BuildSchedule(cells, SchedulerOptions{MaxParallel: 1, Admission: &input}); err == nil || !strings.Contains(err.Error(), "admission blocked") {
		t.Fatalf("blocked scheduler admission error = %v", err)
	}
}

func TestCheckAdmissionWithProbeRequiresProviderFacts(t *testing.T) {
	input := validAdmissionInput()
	probe := admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		return AdmissionProbeResult{
			CredentialsReady: true, AccountOrProjectReady: true, NetworkReady: true,
			StateBackendReady: true, FixtureReady: true, OwnershipReady: true,
			AvailableQuota:         map[string]int{"vpc": 2},
			AvailableLocalCPUMilli: 1000, AvailableLocalMemoryMB: 1024,
		}, nil
	})
	report, err := CheckAdmissionWithProbe(context.Background(), input, probe)
	if err != nil || !report.Ready {
		t.Fatalf("ready provider admission = %#v, %v", report, err)
	}

	probe = admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		return AdmissionProbeResult{AccountOrProjectReady: true, NetworkReady: true, StateBackendReady: true, FixtureReady: true, OwnershipReady: true, AvailableQuota: map[string]int{"vpc": 2}}, nil
	})
	report, err = CheckAdmissionWithProbe(context.Background(), input, probe)
	if err != nil || report.Ready || !strings.Contains(strings.Join(report.BlockingReasons, ";"), "provider admission requirement") {
		t.Fatalf("blocked provider admission = %#v, %v", report, err)
	}
}

func TestCheckAdmissionWithProbeUsesProviderLocalCapacity(t *testing.T) {
	input := validAdmissionInput()
	input.AvailableLocalCPUMilli = 0
	input.AvailableLocalMemoryMB = 0
	probe := admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		return AdmissionProbeResult{
			CredentialsReady: true, AccountOrProjectReady: true, NetworkReady: true,
			StateBackendReady: true, FixtureReady: true, OwnershipReady: true,
			AvailableQuota:         map[string]int{"vpc": 2},
			AvailableLocalCPUMilli: 1000, AvailableLocalMemoryMB: 1024,
		}, nil
	})
	report, err := CheckAdmissionWithProbe(context.Background(), input, probe)
	if err != nil || !report.Ready {
		t.Fatalf("provider local capacity admission = %#v, %v", report, err)
	}

	probe = admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		return AdmissionProbeResult{
			CredentialsReady: true, AccountOrProjectReady: true, NetworkReady: true,
			StateBackendReady: true, FixtureReady: true, OwnershipReady: true,
			AvailableQuota:         map[string]int{"vpc": 2},
			AvailableLocalCPUMilli: 250, AvailableLocalMemoryMB: 1024,
		}, nil
	})
	report, err = CheckAdmissionWithProbe(context.Background(), input, probe)
	if err != nil || report.Ready || !strings.Contains(strings.Join(report.BlockingReasons, ";"), "local CPU") {
		t.Fatalf("insufficient provider local capacity = %#v, %v", report, err)
	}
}

func TestCheckAdmissionWithProbeTreatsProbeErrorsAsBlocked(t *testing.T) {
	report, err := CheckAdmissionWithProbe(context.Background(), validAdmissionInput(), admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		return AdmissionProbeResult{}, errors.New("secret should not escape")
	}))
	if err != nil || report.Ready || len(report.BlockingReasons) != 1 || strings.Contains(strings.Join(report.BlockingReasons, ";"), "secret") {
		t.Fatalf("probe failure admission = %#v, %v", report, err)
	}
}

func TestCheckAdmissionWithProbePropagatesCancellationAndRejectsTypedNil(t *testing.T) {
	var typedNil *ProviderAdmissionProbe
	if _, err := CheckAdmissionWithProbe(context.Background(), validAdmissionInput(), typedNil); err == nil || !strings.Contains(err.Error(), "admission probe is required") {
		t.Fatalf("typed nil probe error = %v", err)
	}

	report, err := CheckAdmissionWithProbe(context.Background(), validAdmissionInput(), admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		return AdmissionProbeResult{}, context.Canceled
	}))
	if !errors.Is(err, context.Canceled) || report.Ready {
		t.Fatalf("provider cancellation = %#v, %v", report, err)
	}
}

func TestCheckAdmissionWithProbeBlocksMalformedProviderCapacity(t *testing.T) {
	input := validAdmissionInput()
	input.RequiredLocalCPUMilli = 0
	input.RequiredLocalMemoryMB = 0
	report, err := CheckAdmissionWithProbe(context.Background(), input, admissionProbeFunc(func(context.Context, AdmissionInput) (AdmissionProbeResult, error) {
		result := readyAdmissionProbeResult()
		result.AvailableLocalCPUMilli = -1
		return result, nil
	}))
	if err != nil || report.Ready || len(report.BlockingReasons) != 1 || !strings.Contains(report.BlockingReasons[0], "invalid") {
		t.Fatalf("malformed provider capacity = %#v, %v", report, err)
	}
}
