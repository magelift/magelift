package observability

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

type fakeVerificationProbe struct {
	signal     SignalObservation
	operations OperationalObservation
	cleanup    CleanupObservation
}

type fakeLifecycleClient struct {
	fakeVerificationProbe
}

func (client fakeLifecycleClient) Apply(context.Context, Plan) (LifecycleResult, error) {
	return LifecycleResult{OperationID: "telemetry-apply-1", ResourceRefs: []string{"telemetry:owned"}, ProofRefs: []string{"telemetry:apply"}, OwnershipVerified: true, IdempotencyVerified: true}, nil
}

func (client fakeLifecycleClient) Destroy(context.Context, Plan, []string) (CleanupObservation, error) {
	return CleanupObservation{Complete: true, UnownedPreserved: true}, nil
}

func (probe fakeVerificationProbe) VerifySignal(_ context.Context, binding SignalBinding) (SignalObservation, error) {
	observation := probe.signal
	observation.Signal = binding.Signal
	observation.Destination = binding.Destination
	return observation, nil
}

func (probe fakeVerificationProbe) VerifyOperations(_ context.Context, _ Plan) (OperationalObservation, error) {
	return probe.operations, nil
}

func (probe fakeVerificationProbe) VerifyCleanup(_ context.Context, _ Plan) (CleanupObservation, error) {
	return probe.cleanup, nil
}

func TestPlanComposesNativeCloudAndNewRelicOTLPBindings(t *testing.T) {
	plan, err := New().Plan(Request{
		TargetProvider: "aws", TargetRuntime: "ecs-fargate",
		Intent: sdk.ObservabilityIntent{
			NativeProvider: "cloudwatch", ExternalProvider: "newrelic", OwnershipMarker: "magelift/architecture/test", CredentialRefs: []string{"aws-secrets-manager://magelift/newrelic"}, Endpoint: "https://otlp.eu01.nr-data.net", Signals: []string{"traces", "metrics", "logs"}, RetentionDays: 30, RedactionPolicyRef: "policy://redact",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Bindings) != 5 {
		t.Fatalf("bindings = %#v", plan.Bindings)
	}
	if len(plan.SignalStatuses) != 6 {
		t.Fatalf("signal statuses = %#v", plan.SignalStatuses)
	}
	if plan.OwnershipMarker != "magelift/architecture/test" {
		t.Fatalf("observability ownership scope = %q", plan.OwnershipMarker)
	}
	for _, binding := range plan.Bindings {
		if binding.Destination == "newrelic" && (binding.Mode != "otlp" || binding.CredentialRef == "") {
			t.Fatalf("New Relic binding = %#v", binding)
		}
	}
	if !strings.Contains(plan.Outputs[2].Value, "logs") {
		t.Fatalf("stable output = %#v", plan.Outputs)
	}
}

func TestPlanComposesNativeAndNewRelicAcrossFirstPartyTargets(t *testing.T) {
	tests := []struct {
		name          string
		target        string
		runtime       string
		native        string
		nativeRef     string
		signals       []string
		nativeBinding int
	}{
		{name: "aws-ecs", target: "aws", runtime: "ecs-fargate", native: "cloudwatch", signals: []string{"logs", "metrics"}, nativeBinding: 2},
		{name: "gcp-gke", target: "gcp", runtime: "gke-standard", native: "google-cloud-operations", signals: []string{"logs", "metrics"}, nativeBinding: 2},
		{name: "scaleway-kapsule", target: "scaleway", runtime: "kapsule", native: "scaleway-cockpit", signals: []string{"logs", "metrics", "traces"}, nativeBinding: 3},
		{name: "ovh-mks", target: "ovh", runtime: "mks", native: "ovh-logs-data-platform", nativeRef: "stream-123", signals: []string{"audit-events"}, nativeBinding: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := New().Plan(Request{
				TargetProvider: test.target, TargetRuntime: test.runtime,
				Intent: sdk.ObservabilityIntent{
					NativeProvider: test.native, NativeReference: test.nativeRef,
					ExternalProvider: "newrelic", CredentialRefs: []string{"aws-secrets-manager://magelift/test/newrelic"},
					Endpoint: "https://otlp.eu01.nr-data.net", OwnershipMarker: "magelift/observability/" + test.name,
					Signals: test.signals,
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := len(plan.Bindings), test.nativeBinding+len(test.signals); got != want {
				t.Fatalf("bindings = %d, want %d: %#v", got, want, plan.Bindings)
			}
			if got, want := len(plan.SignalStatuses), test.nativeBinding+len(test.signals); got != want {
				t.Fatalf("signal statuses = %d, want %d: %#v", got, want, plan.SignalStatuses)
			}
			for _, binding := range plan.Bindings {
				if binding.Destination == "newrelic" {
					if binding.Mode != "otlp" || binding.CredentialRef == "" || binding.Endpoint == "" {
						t.Fatalf("New Relic binding = %#v", binding)
					}
					continue
				}
				if binding.Destination != test.native || binding.Mode != "native" {
					t.Fatalf("native binding = %#v", binding)
				}
			}
			for _, status := range plan.SignalStatuses {
				if status.Status != "configured" || status.Reason != "" {
					t.Fatalf("configured matrix status = %#v", status)
				}
			}
		})
	}
}

func TestPlanReportsNativeSignalGapsInsteadOfDroppingThem(t *testing.T) {
	plan, err := New().Plan(Request{TargetProvider: "ovh", TargetRuntime: "mks", Intent: sdk.ObservabilityIntent{NativeProvider: "ovh-logs-data-platform", NativeReference: "stream-123", OwnershipMarker: "magelift/architecture/test", Signals: []string{"audit-events", "metrics"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Bindings) != 1 || len(plan.SignalStatuses) != 2 {
		t.Fatalf("plan = %#v", plan)
	}
	for _, status := range plan.SignalStatuses {
		if status.Signal == "metrics" && (status.Status != "unavailable" || status.Reason == "") {
			t.Fatalf("metrics gap = %#v", status)
		}
		if status.Signal == "audit-events" && status.Status != "configured" {
			t.Fatalf("audit signal = %#v", status)
		}
	}
}

func TestPlanReportsOperationalObjectGapsAcrossFirstPartyTargets(t *testing.T) {
	baseOperations := func() sdk.ObservabilityIntent {
		return sdk.ObservabilityIntent{
			OwnershipMarker: "magelift/observability/operations-matrix",
			Alerts: []sdk.AlertIntent{{
				ID: "health", Signal: "metrics", Severity: "critical", Operator: "gt", Threshold: 1,
				WindowSeconds: 60, Owner: "oncall", RunbookURL: "https://runbooks.example/health", DeduplicationKey: "health",
			}},
			Dashboards: []sdk.DashboardIntent{{ID: "operations", Signals: []string{"metrics"}, Owner: "oncall"}},
			SLOs: []sdk.SLOIntent{{
				ID: "availability", Signal: "metrics", Target: 0.999, WindowSeconds: 3600,
				Owner: "oncall", RunbookURL: "https://runbooks.example/availability", ErrorBudgetPolicy: "page",
			}},
		}
	}
	tests := []struct {
		name      string
		target    string
		runtime   string
		native    string
		nativeRef string
		want      []string
	}{
		{name: "aws-cloudwatch", target: "aws", runtime: "ecs-fargate", native: "cloudwatch", want: []string{"slo.availability"}},
		{name: "gcp-monitoring-service", target: "gcp", runtime: "gke-standard", native: "google-cloud-operations", nativeRef: "services/app", want: []string{}},
		{name: "scaleway-cockpit", target: "scaleway", runtime: "kapsule", native: "scaleway-cockpit", want: []string{"alert.health", "dashboard.operations", "slo.availability"}},
		{name: "ovh-logs", target: "ovh", runtime: "mks", native: "ovh-logs-data-platform", nativeRef: "stream-123", want: []string{"alert.health", "dashboard.operations", "slo.availability"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intent := baseOperations()
			intent.NativeProvider = test.native
			intent.NativeReference = test.nativeRef
			if test.target == "ovh" {
				intent.Signals = []string{"audit-events"}
			} else {
				intent.Signals = []string{"metrics"}
			}
			plan, err := New().Plan(Request{TargetProvider: test.target, TargetRuntime: test.runtime, Intent: intent})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(plan.UnavailableOperations, test.want) {
				t.Fatalf("unavailable operations = %v, want %v", plan.UnavailableOperations, test.want)
			}
		})
	}
}

func TestNativePlansKeepUnprovisionedTraceAndAuditSourcesExplicit(t *testing.T) {
	for _, test := range []struct {
		provider string
		target   string
		signal   string
	}{
		{provider: "cloudwatch", target: "aws", signal: "traces"},
		{provider: "cloudwatch", target: "aws", signal: "audit-events"},
		{provider: "google-cloud-operations", target: "gcp", signal: "traces"},
	} {
		t.Run(test.provider+"/"+test.signal, func(t *testing.T) {
			plan, err := New().Plan(Request{
				TargetProvider: test.target, TargetRuntime: "test",
				Intent: sdk.ObservabilityIntent{
					NativeProvider: test.provider, OwnershipMarker: "magelift/architecture/test",
					Signals: []string{test.signal},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Bindings) != 0 || len(plan.SignalStatuses) != 1 || plan.SignalStatuses[0].Status != "unavailable" || plan.SignalStatuses[0].Reason == "" {
				t.Fatalf("native gap was not explicit: %#v", plan)
			}
		})
	}
}

func TestScalewayCockpitDeclaresTracesAndKeepsAuditExplicit(t *testing.T) {
	plan, err := New().Plan(Request{
		TargetProvider: "scaleway", TargetRuntime: "kapsule",
		Intent: sdk.ObservabilityIntent{
			NativeProvider: "scaleway-cockpit", OwnershipMarker: "magelift/architecture/test",
			Signals: []string{"metrics", "logs", "traces", "audit-events"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range plan.SignalStatuses {
		switch status.Signal {
		case "metrics", "logs", "traces":
			if status.Status != "configured" {
				t.Fatalf("Scaleway native signal = %#v", status)
			}
		case "audit-events":
			if status.Status != "unavailable" || status.Reason == "" {
				t.Fatalf("Scaleway audit gap = %#v", status)
			}
		}
	}
}

func TestPlanRejectsWrongNativeDestinationAndKeepsSecretsOutOfOutputs(t *testing.T) {
	_, err := New().Plan(Request{TargetProvider: "gcp", TargetRuntime: "gke", Intent: sdk.ObservabilityIntent{NativeProvider: "cloudwatch", OwnershipMarker: "magelift/architecture/test", Signals: []string{"metrics"}}})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("wrong native provider error = %v", err)
	}
	secret := "aws-secrets-manager://magelift/newrelic"
	plan, err := New().Plan(Request{TargetProvider: "gcp", TargetRuntime: "gke", Intent: sdk.ObservabilityIntent{ExternalProvider: "newrelic", OwnershipMarker: "magelift/architecture/test", CredentialRefs: []string{secret}, Endpoint: "https://otlp.nr-data.net", Signals: []string{"metrics"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range plan.Outputs {
		if strings.Contains(output.Value, secret) {
			t.Fatalf("secret leaked into stable output: %#v", output)
		}
	}
}

func TestPlanCarriesActionableAlertsAndSLOsWithoutVendorObjects(t *testing.T) {
	plan, err := New().Plan(Request{TargetProvider: "gcp", TargetRuntime: "gke-standard", Intent: sdk.ObservabilityIntent{
		NativeProvider: "google-cloud-operations", OwnershipMarker: "magelift/architecture/test", Signals: []string{"application-health"}, DataResidency: "eu",
		Alerts:     []sdk.AlertIntent{{ID: "health", Signal: "application-health", Severity: "critical", Operator: "lt", Threshold: 1, WindowSeconds: 60, Owner: "oncall", RunbookURL: "https://runbooks.example/health", DeduplicationKey: "health"}},
		Dashboards: []sdk.DashboardIntent{{ID: "health", Signals: []string{"application-health"}, Owner: "oncall"}},
		SLOs:       []sdk.SLOIntent{{ID: "availability", Signal: "application-health", Target: 0.999, WindowSeconds: 2592000, Owner: "oncall", RunbookURL: "https://runbooks.example/availability", ErrorBudgetPolicy: "page"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Alerts) != 1 || len(plan.Dashboards) != 1 || len(plan.SLOs) != 1 || plan.DataResidency != "eu" {
		t.Fatalf("observability action plan = %#v", plan)
	}
	for _, output := range plan.Outputs {
		if strings.Contains(output.Value, "runbooks.example") {
			t.Fatalf("runbook URL leaked into stable output: %#v", output)
		}
	}
}

func TestPlanEmitsOperationalObjectsWhenSignalBindingsAreEmpty(t *testing.T) {
	plan, err := New().Plan(Request{TargetProvider: "aws", TargetRuntime: "ecs-fargate", Intent: sdk.ObservabilityIntent{
		NativeProvider: "cloudwatch", OwnershipMarker: "magelift/architecture/test",
		Dashboards: []sdk.DashboardIntent{{ID: "operations", Signals: []string{"cleanup-failure"}, Owner: "oncall"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Outputs) != 3 || plan.Outputs[2].Key != "observability.dashboard.0" {
		t.Fatalf("dashboard-only outputs = %#v", plan.Outputs)
	}
}

func TestVerifyRequiresSignalDeliveryOperationalObjectsAndOwnedCleanup(t *testing.T) {
	plan, err := New().Plan(Request{TargetProvider: "aws", TargetRuntime: "ecs-fargate", Intent: sdk.ObservabilityIntent{
		NativeProvider: "cloudwatch", OwnershipMarker: "magelift/architecture/test", Signals: []string{"metrics"}, RetentionDays: 7,
		Alerts: []sdk.AlertIntent{{ID: "health", Signal: "metrics", Severity: "critical", Operator: "lt", Threshold: 1, WindowSeconds: 60, Owner: "oncall", RunbookURL: "https://runbooks.example/health", DeduplicationKey: "health"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	probe := fakeVerificationProbe{
		signal:     SignalObservation{Delivered: true, LabelsVerified: true, RetentionVerified: true, RedactionVerified: true},
		operations: OperationalObservation{AlertsVerified: true},
		cleanup:    CleanupObservation{Complete: true, UnownedPreserved: true},
	}
	verification, err := New().Verify(context.Background(), plan, probe)
	if err != nil {
		t.Fatal(err)
	}
	if !verification.Complete || len(verification.Failures) != 0 || len(verification.Signals) != 1 {
		t.Fatalf("verification = %#v", verification)
	}
	probe.signal.LabelsVerified = false
	verification, err = New().Verify(context.Background(), plan, probe)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Complete || !strings.Contains(strings.Join(verification.Failures, "\n"), "required labels") {
		t.Fatalf("missing-label verification = %#v", verification)
	}
}

func TestVerifyKeepsUnavailableNativeSignalsExplicit(t *testing.T) {
	plan, err := New().Plan(Request{TargetProvider: "ovh", TargetRuntime: "mks", Intent: sdk.ObservabilityIntent{
		NativeProvider: "ovh-logs-data-platform", NativeReference: "stream-123", OwnershipMarker: "magelift/architecture/test", Signals: []string{"audit-events", "metrics"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	verification, err := New().Verify(context.Background(), plan, fakeVerificationProbe{cleanup: CleanupObservation{Complete: true, UnownedPreserved: true}})
	if err != nil {
		t.Fatal(err)
	}
	if verification.Complete || !strings.Contains(strings.Join(verification.Failures, "\n"), "metrics") {
		t.Fatalf("unavailable signal verification = %#v", verification)
	}
}

func TestSDKAdapterRunsNativeOnlyExternalOnlyAndCombinedDestinations(t *testing.T) {
	client := fakeLifecycleClient{fakeVerificationProbe{
		signal:  SignalObservation{Delivered: true, LabelsVerified: true, RetentionVerified: true, RedactionVerified: true},
		cleanup: CleanupObservation{Complete: true, UnownedPreserved: true},
	}}
	adapter := NewSDKAdapter("aws", New(), client)
	tests := []struct {
		name   string
		intent sdk.ObservabilityIntent
	}{
		{name: "native-only", intent: sdk.ObservabilityIntent{NativeProvider: "cloudwatch", OwnershipMarker: "magelift/test/native", Signals: []string{"metrics"}}},
		{name: "newrelic-only", intent: sdk.ObservabilityIntent{ExternalProvider: "newrelic", Lifecycle: sdk.ExternalLifecycleExtension, Certification: sdk.ExternalExperimental, CredentialRefs: []string{"aws-secrets-manager://magelift/test/newrelic"}, Endpoint: "https://otlp.eu01.nr-data.net", OwnershipMarker: "magelift/test/newrelic", Signals: []string{"metrics"}}},
		{name: "combined", intent: sdk.ObservabilityIntent{NativeProvider: "cloudwatch", ExternalProvider: "newrelic", Lifecycle: sdk.ExternalLifecycleExtension, Certification: sdk.ExternalExperimental, CredentialRefs: []string{"aws-secrets-manager://magelift/test/newrelic"}, Endpoint: "https://otlp.eu01.nr-data.net", OwnershipMarker: "magelift/test/combined", Signals: []string{"metrics"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := sdk.ObservabilityPlanRequest{TargetProvider: "aws", TargetRuntime: "ecs-fargate", Intent: test.intent}
			_, applied, err := sdk.RunObservabilityLifecycle(context.Background(), adapter, request, sdk.ObservabilityApply, "apply-"+test.name, nil, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(applied.ResourceRefs) != 1 || !applied.OwnershipVerified || !applied.IdempotencyVerified {
				t.Fatalf("apply result = %#v", applied)
			}
			_, verified, err := sdk.RunObservabilityLifecycle(context.Background(), adapter, request, sdk.ObservabilityVerify, "verify-"+test.name, applied.ResourceRefs, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(verified.ProofRefs) == 0 {
				t.Fatalf("verify result = %#v", verified)
			}
			_, destroyed, err := sdk.RunObservabilityLifecycle(context.Background(), adapter, request, sdk.ObservabilityDestroy, "destroy-"+test.name, applied.ResourceRefs, "")
			if err != nil {
				t.Fatal(err)
			}
			if destroyed.Action != sdk.ObservabilityDestroy {
				t.Fatalf("destroy result = %#v", destroyed)
			}
		})
	}
}

func TestSDKAdapterDoesNotApplyUnavailableCapabilities(t *testing.T) {
	client := &countingLifecycleClient{}
	adapter := NewSDKAdapter("aws", New(), client)
	request := sdk.ObservabilityPlanRequest{TargetProvider: "aws", TargetRuntime: "ecs-fargate", Intent: sdk.ObservabilityIntent{
		NativeProvider: "cloudwatch", OwnershipMarker: "magelift/test/unavailable", Signals: []string{"metrics"},
		SLOs: []sdk.SLOIntent{{ID: "availability", Signal: "application-health", Target: 0.999, WindowSeconds: 3600, Owner: "oncall", RunbookURL: "https://runbooks.example/availability", ErrorBudgetPolicy: "page"}},
	}}
	_, _, err := sdk.RunObservabilityLifecycle(context.Background(), adapter, request, sdk.ObservabilityApply, "apply-unavailable", nil, "")
	if err == nil || !strings.Contains(err.Error(), "blocked before mutation") {
		t.Fatalf("unavailable apply error = %v", err)
	}
	if client.applyCalls != 0 {
		t.Fatalf("provider apply calls = %d, want zero", client.applyCalls)
	}
}

func TestSDKAdapterRejectsTamperedPlanBeforeProviderMutation(t *testing.T) {
	client := &countingLifecycleClient{}
	adapter := NewSDKAdapter("aws", New(), client)
	request := sdk.ObservabilityPlanRequest{
		TargetProvider: "aws", TargetRuntime: "ecs-fargate",
		Intent: sdk.ObservabilityIntent{NativeProvider: "cloudwatch", OwnershipMarker: "magelift/test/plan-boundary", Signals: []string{"metrics"}},
	}
	plan, err := adapter.PlanObservability(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}

	tampered := plan
	tampered.AdapterID = "gcp.observability"
	_, err = adapter.ExecuteObservability(context.Background(), sdk.ObservabilityExecutionRequest{
		Plan: tampered, Action: sdk.ObservabilityApply, IdempotencyKey: "observability/apply/tampered-plan", OwnershipMarker: plan.OwnershipMarker,
	})
	if err == nil || !strings.Contains(err.Error(), "provider boundary") || client.applyCalls != 0 {
		t.Fatalf("error = %v client = %#v, want boundary rejection before mutation", err, client)
	}
}

func TestSDKAdapterPreservesOpaqueNativeReferenceInPublicPlan(t *testing.T) {
	adapter := NewSDKAdapter("ovh", New(), &countingLifecycleClient{})
	request := sdk.ObservabilityPlanRequest{
		TargetProvider: "ovh", TargetRuntime: "mks",
		Intent: sdk.ObservabilityIntent{NativeProvider: "ovh-logs-data-platform", NativeReference: "stream-123", OwnershipMarker: "magelift/test/native-reference", Signals: []string{"audit-events"}},
	}
	plan, err := adapter.PlanObservability(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NativeReference != request.Intent.NativeReference {
		t.Fatalf("public plan native reference = %q, want %q", plan.NativeReference, request.Intent.NativeReference)
	}
}

type countingLifecycleClient struct {
	applyCalls int
}

func (*countingLifecycleClient) VerifySignal(context.Context, SignalBinding) (SignalObservation, error) {
	return SignalObservation{}, errors.New("verification should not run")
}

func (*countingLifecycleClient) VerifyOperations(context.Context, Plan) (OperationalObservation, error) {
	return OperationalObservation{}, errors.New("verification should not run")
}

func (*countingLifecycleClient) VerifyCleanup(context.Context, Plan) (CleanupObservation, error) {
	return CleanupObservation{}, errors.New("verification should not run")
}

func (client *countingLifecycleClient) Apply(context.Context, Plan) (LifecycleResult, error) {
	client.applyCalls++
	return LifecycleResult{}, nil
}

func (*countingLifecycleClient) Destroy(context.Context, Plan, []string) (CleanupObservation, error) {
	return CleanupObservation{}, errors.New("destroy should not run")
}
