package sdk

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateEdgeAdapterBoundary(t *testing.T) {
	descriptor := EdgeAdapterDescriptor{APIVersion: ExtensionAPIVersion, ID: "community.fastly", Provider: "fastly", Version: "1.0.0", Capabilities: []EdgeAction{EdgeApply, EdgeVerify, EdgeFailover, EdgeRollback, EdgeDestroy}}
	if err := ValidateEdgeAdapterDescriptor(descriptor); err != nil {
		t.Fatal(err)
	}
	request := EdgePlanRequest{
		TargetProvider: "aws", TargetRuntime: "ecs-fargate",
		Intent: EdgeIntent{
			ExternalProvider: "fastly", Lifecycle: ExternalLifecycleExtension, Certification: ExternalExperimental,
			Mode: "external", CredentialRefs: []string{"aws-secrets-manager://magelift/fastly"}, Domains: []string{"preview.example.com"},
			OriginHealthRef: "health/application", OwnershipMarker: "magelift/run-1",
		},
	}
	if err := ValidateEdgePlanRequest(request); err != nil {
		t.Fatal(err)
	}
	plan := EdgePlan{AdapterID: descriptor.ID, TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime, OwnershipMarker: request.Intent.OwnershipMarker}
	if err := ValidateEdgePlan(plan, request); err != nil {
		t.Fatal(err)
	}
	if err := ValidateEdgeExecutionRequest(EdgeExecutionRequest{Plan: plan, Action: EdgeApply, IdempotencyKey: "magelift/run-1/apply", OwnershipMarker: plan.OwnershipMarker}); err != nil {
		t.Fatal(err)
	}
	for _, action := range []EdgeAction{EdgeFailover, EdgeRollback} {
		if err := ValidateEdgeExecutionRequest(EdgeExecutionRequest{Plan: plan, Action: action, IdempotencyKey: "magelift/run-1/" + string(action), OwnershipMarker: plan.OwnershipMarker, ApprovalReference: "approval/run-1"}); err != nil {
			t.Fatalf("%s action rejected: %v", action, err)
		}
	}
	if err := ValidateEdgeExecutionRequest(EdgeExecutionRequest{Plan: plan, Action: EdgeDestroy, IdempotencyKey: "magelift/run-1/destroy", OwnershipMarker: "magelift/other"}); err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("ownership mismatch accepted: %v", err)
	}
}

func TestValidateObservabilityAdapterBoundaryKeepsCredentialReferencesTyped(t *testing.T) {
	descriptor := ObservabilityAdapterDescriptor{APIVersion: ExtensionAPIVersion, ID: "community.newrelic", Provider: "newrelic", Version: "1.0.0", Capabilities: []ObservabilityAction{ObservabilityApply, ObservabilityVerify, ObservabilityDestroy}}
	if err := ValidateObservabilityAdapterDescriptor(descriptor); err != nil {
		t.Fatal(err)
	}
	request := ObservabilityPlanRequest{
		TargetProvider: "gcp", TargetRuntime: "gke-standard",
		Intent: ObservabilityIntent{
			ExternalProvider: "newrelic", Lifecycle: ExternalLifecycleExtension, Certification: ExternalExperimental,
			OwnershipMarker: "magelift/run-2", CredentialRefs: []string{"gcp-secret-manager://projects/p/secrets/newrelic"},
			Endpoint: "https://otlp.eu01.nr-data.net", Signals: []string{"metrics", "traces"},
		},
	}
	if err := ValidateObservabilityPlanRequest(request); err != nil {
		t.Fatal(err)
	}
	plan := ObservabilityPlan{
		AdapterID: descriptor.ID, TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime,
		OwnershipMarker: request.Intent.OwnershipMarker,
		Bindings: []ObservabilityBinding{
			{Signal: "metrics", Destination: "newrelic", Mode: "otlp", CredentialRef: request.Intent.CredentialRefs[0], RetentionDays: 30},
			{Signal: "traces", Destination: "newrelic", Mode: "otlp", CredentialRef: request.Intent.CredentialRefs[0], RetentionDays: 30},
		},
	}
	if err := ValidateObservabilityPlan(plan, request); err != nil {
		t.Fatal(err)
	}

	nativeRequest := ObservabilityPlanRequest{TargetProvider: "ovh", TargetRuntime: "mks", Intent: ObservabilityIntent{
		NativeProvider: "ovh-logs-data-platform", NativeReference: "stream-123", OwnershipMarker: "magelift/run-2/native", Signals: []string{"audit-events"},
	}}
	nativePlan := ObservabilityPlan{AdapterID: "ovh.observability", TargetProvider: nativeRequest.TargetProvider, TargetRuntime: nativeRequest.TargetRuntime, OwnershipMarker: nativeRequest.Intent.OwnershipMarker, NativeReference: nativeRequest.Intent.NativeReference, Bindings: []ObservabilityBinding{{Signal: "audit-events", Destination: "ovh-logs-data-platform", Mode: "native"}}}
	if err := ValidateObservabilityPlan(nativePlan, nativeRequest); err != nil {
		t.Fatal(err)
	}
	nativePlan.NativeReference = "other-stream"
	if err := ValidateObservabilityPlan(nativePlan, nativeRequest); err == nil || !strings.Contains(err.Error(), "native reference") {
		t.Fatalf("native reference drift was accepted: %v", err)
	}
	plan.UnavailableOperations = []string{"dashboard.operations"}
	if err := ValidateObservabilityPlan(plan, request); err != nil {
		t.Fatalf("valid unavailable operation was rejected: %v", err)
	}
	plan.UnavailableOperations = []string{"dashboard:operations"}
	if err := ValidateObservabilityPlan(plan, request); err == nil || !strings.Contains(err.Error(), "unavailable observability operation") {
		t.Fatalf("invalid unavailable operation was accepted: %v", err)
	}
	plan.UnavailableOperations = nil
	if err := ValidateObservabilityExecutionRequest(ObservabilityExecutionRequest{Plan: plan, Action: ObservabilityVerify, IdempotencyKey: "magelift/run-2/verify", OwnershipMarker: plan.OwnershipMarker}); err != nil {
		t.Fatal(err)
	}
	plan.Bindings[0].CredentialRef = "https://example.invalid/token"
	if err := ValidateObservabilityPlan(plan, request); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("unsafe credential reference accepted: %v", err)
	}
}

func TestAdapterDescriptorsRejectDuplicateOrUnknownActions(t *testing.T) {
	if err := ValidateEdgeAdapterDescriptor(EdgeAdapterDescriptor{APIVersion: ExtensionAPIVersion, ID: "edge", Provider: "aws", Version: "1", Capabilities: []EdgeAction{EdgeApply, EdgeApply}}); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate edge action accepted: %v", err)
	}
	if err := ValidateObservabilityAdapterDescriptor(ObservabilityAdapterDescriptor{APIVersion: ExtensionAPIVersion, ID: "obs", Provider: "gcp", Version: "1", Capabilities: []ObservabilityAction{"unknown"}}); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("unknown observability action accepted: %v", err)
	}
}

func TestValidateObservabilityPlanTreatsNoneAsNoDestination(t *testing.T) {
	request := ObservabilityPlanRequest{
		TargetProvider: "gcp",
		TargetRuntime:  "gke-standard",
		Intent: ObservabilityIntent{
			NativeProvider:   "none",
			ExternalProvider: "none",
			OwnershipMarker:  "magelift/run-3",
		},
	}
	plan := ObservabilityPlan{AdapterID: "community.none", TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime}
	if err := ValidateObservabilityPlan(plan, request); err != nil {
		t.Fatalf("none destinations should not require an active ownership marker: %v", err)
	}
}

func TestValidateObservabilityPlanPreservesOperationalIntent(t *testing.T) {
	request := ObservabilityPlanRequest{
		TargetProvider: "aws",
		TargetRuntime:  "ecs-fargate",
		Intent: ObservabilityIntent{
			NativeProvider:  "cloudwatch",
			OwnershipMarker: "magelift/ops-plan",
			DataResidency:   "eu",
			Alerts: []AlertIntent{{
				ID: "availability", Signal: "application-health", Severity: "critical", Operator: "lt", Threshold: 1,
				WindowSeconds: 60, Owner: "oncall", RunbookURL: "https://runbooks.example/availability", DeduplicationKey: "availability",
			}},
			Dashboards: []DashboardIntent{{ID: "operations", Signals: []string{"application-health"}, Owner: "oncall"}},
			SLOs: []SLOIntent{{
				ID: "availability", Signal: "application-health", Target: 0.999, WindowSeconds: 2592000,
				Owner: "oncall", RunbookURL: "https://runbooks.example/availability", ErrorBudgetPolicy: "page",
			}},
		},
	}
	plan := ObservabilityPlan{
		AdapterID:       "aws.observability",
		TargetProvider:  request.TargetProvider,
		TargetRuntime:   request.TargetRuntime,
		OwnershipMarker: request.Intent.OwnershipMarker,
		Alerts:          append([]AlertIntent(nil), request.Intent.Alerts...),
		Dashboards:      append([]DashboardIntent(nil), request.Intent.Dashboards...),
		SLOs:            append([]SLOIntent(nil), request.Intent.SLOs...),
		DataResidency:   "eu",
	}
	if err := ValidateObservabilityPlan(plan, request); err != nil {
		t.Fatal(err)
	}
	plan.SLOs = nil
	if err := ValidateObservabilityPlan(plan, request); err == nil || !strings.Contains(err.Error(), "preserve all requested") {
		t.Fatalf("dropped SLO was accepted: %v", err)
	}
}

func TestValidateObservabilityPlanRejectsSemanticAndSignalDrift(t *testing.T) {
	request := ObservabilityPlanRequest{
		TargetProvider: "aws", TargetRuntime: "ecs-fargate",
		Intent: ObservabilityIntent{
			NativeProvider: "cloudwatch", OwnershipMarker: "magelift/drift-test", Signals: []string{"logs", "metrics"},
			Alerts:     []AlertIntent{{ID: "availability", Signal: "logs", Severity: "critical", Operator: "gt", Threshold: 0, WindowSeconds: 60, Owner: "oncall", RunbookURL: "https://runbooks.example/availability", DeduplicationKey: "availability"}},
			Dashboards: []DashboardIntent{{ID: "operations", Signals: []string{"logs", "metrics"}, Owner: "oncall"}},
			SLOs:       []SLOIntent{{ID: "availability", Signal: "logs", Target: 0.999, WindowSeconds: 3600, Owner: "oncall", RunbookURL: "https://runbooks.example/availability", ErrorBudgetPolicy: "page"}},
		},
	}
	plan := ObservabilityPlan{
		AdapterID: "aws.observability", TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime,
		OwnershipMarker: request.Intent.OwnershipMarker,
		Bindings:        []ObservabilityBinding{{Signal: "logs", Destination: "cloudwatch", Mode: "native"}, {Signal: "metrics", Destination: "cloudwatch", Mode: "native"}},
		Alerts:          append([]AlertIntent(nil), request.Intent.Alerts...),
		Dashboards:      append([]DashboardIntent(nil), request.Intent.Dashboards...),
		SLOs:            append([]SLOIntent(nil), request.Intent.SLOs...),
	}
	if err := ValidateObservabilityPlan(plan, request); err != nil {
		t.Fatalf("matching plan rejected: %v", err)
	}

	plan.Alerts[0].Threshold = 1
	if err := ValidateObservabilityPlan(plan, request); err == nil || !strings.Contains(err.Error(), "preserve all requested") {
		t.Fatalf("alert semantic drift was accepted: %v", err)
	}
	plan.Alerts[0].Threshold = request.Intent.Alerts[0].Threshold
	plan.Bindings = plan.Bindings[:1]
	if err := ValidateObservabilityPlan(plan, request); err == nil || !strings.Contains(err.Error(), "dropped requested signal") {
		t.Fatalf("silent signal drop was accepted: %v", err)
	}
	plan.UnavailableSignals = []string{"metrics"}
	if err := ValidateObservabilityPlan(plan, request); err != nil {
		t.Fatalf("explicit unavailable signal was rejected: %v", err)
	}
}

func TestValidateObservabilityApplyPlanFailsClosedOnUnavailableCapabilities(t *testing.T) {
	tests := []struct {
		name string
		plan ObservabilityPlan
		want string
	}{
		{name: "signal", plan: ObservabilityPlan{UnavailableSignals: []string{"traces"}}, want: `signal "traces" is unavailable`},
		{name: "operation", plan: ObservabilityPlan{UnavailableOperations: []string{"slo.availability"}}, want: `operation "slo.availability" is unavailable`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateObservabilityApplyPlan(test.plan)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			var capabilityErr ObservabilityCapabilityError
			if !errors.As(err, &capabilityErr) {
				t.Fatalf("error = %v, want ObservabilityCapabilityError", err)
			}
		})
	}
}
