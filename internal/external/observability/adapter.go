// Package observability implements the provider-neutral telemetry planning
// boundary. Native cloud destinations and New Relic use the same semantic
// signal bindings; provider SDK resource schemas stay inside their adapters.
package observability

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/magelift/magelift/sdk"
)

type Request struct {
	TargetProvider string
	TargetRuntime  string
	Intent         sdk.ObservabilityIntent
}

type SignalBinding struct {
	Signal      string `json:"signal" yaml:"signal"`
	Destination string `json:"destination" yaml:"destination"`
	Mode        string `json:"mode" yaml:"mode"`
	// OwnershipMarker is internal adapter state. It is deliberately omitted
	// from serialized plans because the enclosing Plan already carries it, but
	// provider verifiers need it to address the exact owning-service identity.
	OwnershipMarker string `json:"-" yaml:"-"`
	CredentialRef   string `json:"credentialRef,omitempty" yaml:"credentialRef,omitempty"`
	Endpoint        string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	RetentionDays   int    `json:"retentionDays,omitempty" yaml:"retentionDays,omitempty"`
	RedactionPolicy string `json:"redactionPolicy,omitempty" yaml:"redactionPolicy,omitempty"`
}

type SignalStatus struct {
	Signal      string `json:"signal" yaml:"signal"`
	Destination string `json:"destination" yaml:"destination"`
	Status      string `json:"status" yaml:"status"`
	Reason      string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type Output struct {
	Key   string `json:"key" yaml:"key"`
	Value string `json:"value" yaml:"value"`
}

type Plan struct {
	TargetProvider  string `json:"targetProvider" yaml:"targetProvider"`
	TargetRuntime   string `json:"targetRuntime" yaml:"targetRuntime"`
	OwnershipMarker string `json:"ownershipMarker" yaml:"ownershipMarker"`
	// NativeReference is an opaque identity for the selected native
	// destination. Provider adapters may use it to address an existing native
	// resource, but the core never parses or constructs provider identifiers.
	NativeReference       string                `json:"nativeReference,omitempty" yaml:"nativeReference,omitempty"`
	Bindings              []SignalBinding       `json:"bindings" yaml:"bindings"`
	SignalStatuses        []SignalStatus        `json:"signalStatuses" yaml:"signalStatuses"`
	UnavailableOperations []string              `json:"unavailableOperations,omitempty" yaml:"unavailableOperations,omitempty"`
	Alerts                []sdk.AlertIntent     `json:"alerts" yaml:"alerts"`
	Dashboards            []sdk.DashboardIntent `json:"dashboards" yaml:"dashboards"`
	SLOs                  []sdk.SLOIntent       `json:"slos" yaml:"slos"`
	DataResidency         string                `json:"dataResidency,omitempty" yaml:"dataResidency,omitempty"`
	Outputs               []Output              `json:"outputs" yaml:"outputs"`
}

// SignalObservation is returned by a provider-specific probe after a
// telemetry destination has received data. A plan alone cannot prove delivery
// or retention, so certification requires every relevant boolean to be true.
type SignalObservation struct {
	Signal            string `json:"signal" yaml:"signal"`
	Destination       string `json:"destination" yaml:"destination"`
	Delivered         bool   `json:"delivered" yaml:"delivered"`
	LabelsVerified    bool   `json:"labelsVerified" yaml:"labelsVerified"`
	RetentionVerified bool   `json:"retentionVerified" yaml:"retentionVerified"`
	RedactionVerified bool   `json:"redactionVerified" yaml:"redactionVerified"`
	Reason            string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type OperationalObservation struct {
	AlertsVerified     bool   `json:"alertsVerified" yaml:"alertsVerified"`
	DashboardsVerified bool   `json:"dashboardsVerified" yaml:"dashboardsVerified"`
	SLOsVerified       bool   `json:"slosVerified" yaml:"slosVerified"`
	Reason             string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type CleanupObservation struct {
	Complete         bool     `json:"complete" yaml:"complete"`
	UnownedPreserved bool     `json:"unownedPreserved" yaml:"unownedPreserved"`
	ResourceRefs     []string `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	Reason           string   `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type Verification struct {
	Complete   bool                   `json:"complete" yaml:"complete"`
	Signals    []SignalObservation    `json:"signals" yaml:"signals"`
	Operations OperationalObservation `json:"operations" yaml:"operations"`
	Cleanup    CleanupObservation     `json:"cleanup" yaml:"cleanup"`
	Failures   []string               `json:"failures,omitempty" yaml:"failures,omitempty"`
}

// VerificationProbe is the provider-specific boundary for live telemetry
// checks. It may query CloudWatch, Google Cloud Operations, Cockpit, Logs Data
// Platform, or New Relic, but the portable adapter only consumes this semantic
// result and never imports those SDK response types.
type VerificationProbe interface {
	VerifySignal(context.Context, SignalBinding) (SignalObservation, error)
	VerifyOperations(context.Context, Plan) (OperationalObservation, error)
	VerifyCleanup(context.Context, Plan) (CleanupObservation, error)
}

type Adapter struct{}

func New() Adapter { return Adapter{} }

// Plan is side-effect free. It selects native destinations where the current
// target has one and uses OTLP for New Relic, allowing the same collector and
// signal contract to work across ECS, EKS/GKE, Kapsule, and MKS.
func (Adapter) Plan(request Request) (Plan, error) {
	if strings.TrimSpace(request.TargetProvider) == "" || strings.TrimSpace(request.TargetRuntime) == "" {
		return Plan{}, errors.New("observability target provider and runtime are required")
	}
	intent := request.Intent
	if intent.ExternalProvider != "" && intent.Lifecycle == "" {
		intent.Lifecycle = sdk.ExternalLifecycleExtension
	}
	if intent.ExternalProvider != "" && intent.Certification == "" {
		intent.Certification = sdk.ExternalExperimental
	}
	if err := sdk.ValidateObservabilityIntent(intent); err != nil {
		return Plan{}, fmt.Errorf("validate observability intent: %w", err)
	}
	native := intent.NativeProvider
	if native == "none" {
		native = ""
	}
	external := intent.ExternalProvider
	if external == "none" {
		external = ""
	}
	if native != "" && !nativeMatchesTarget(native, request.TargetProvider) {
		return Plan{}, fmt.Errorf("native observability provider %q does not match target provider %q", native, request.TargetProvider)
	}
	if len(intent.Signals) == 0 && native == "" && external == "" {
		return Plan{
			TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime, OwnershipMarker: intent.OwnershipMarker, NativeReference: intent.NativeReference,
			Alerts: append([]sdk.AlertIntent(nil), intent.Alerts...), Dashboards: append([]sdk.DashboardIntent(nil), intent.Dashboards...),
			SLOs: append([]sdk.SLOIntent(nil), intent.SLOs...), DataResidency: intent.DataResidency,
			Outputs: observabilityOutputs(request.TargetProvider, request.TargetRuntime, nil, nil, intent.Alerts, intent.Dashboards, intent.SLOs),
		}, nil
	}
	if external == "" && native == "" {
		return Plan{}, errors.New("observability requires a native or external destination")
	}
	bindings := make([]SignalBinding, 0, len(request.Intent.Signals)*2)
	statuses := make([]SignalStatus, 0, len(request.Intent.Signals)*2)
	for _, signal := range sdk.SortedStrings(intent.Signals) {
		if native != "" {
			status := nativeSignalStatus(native, signal, intent.NativeReference)
			statuses = append(statuses, status)
			if status.Status == "configured" {
				bindings = append(bindings, SignalBinding{Signal: signal, Destination: native, Mode: "native", OwnershipMarker: intent.OwnershipMarker, RetentionDays: intent.RetentionDays, RedactionPolicy: intent.RedactionPolicyRef})
			}
		}
		if external != "" {
			mode, endpoint := externalModeEndpoint(external, intent.Endpoint)
			binding := SignalBinding{Signal: signal, Destination: external, Mode: mode, OwnershipMarker: intent.OwnershipMarker, Endpoint: endpoint, RetentionDays: intent.RetentionDays, RedactionPolicy: intent.RedactionPolicyRef}
			if len(intent.CredentialRefs) > 0 {
				binding.CredentialRef = intent.CredentialRefs[0]
			}
			bindings = append(bindings, binding)
			statuses = append(statuses, SignalStatus{Signal: signal, Destination: external, Status: "configured"})
		}
	}
	sort.Slice(bindings, func(i, j int) bool {
		if bindings[i].Signal != bindings[j].Signal {
			return bindings[i].Signal < bindings[j].Signal
		}
		return bindings[i].Destination < bindings[j].Destination
	})
	sort.Slice(statuses, func(i, j int) bool {
		if statuses[i].Signal != statuses[j].Signal {
			return statuses[i].Signal < statuses[j].Signal
		}
		return statuses[i].Destination < statuses[j].Destination
	})
	outputs := observabilityOutputs(request.TargetProvider, request.TargetRuntime, bindings, statuses, intent.Alerts, intent.Dashboards, intent.SLOs)
	return Plan{TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime, OwnershipMarker: intent.OwnershipMarker, NativeReference: intent.NativeReference, Bindings: bindings, SignalStatuses: statuses, UnavailableOperations: unavailableOperations(native, external, intent), Alerts: append([]sdk.AlertIntent(nil), intent.Alerts...), Dashboards: append([]sdk.DashboardIntent(nil), intent.Dashboards...), SLOs: append([]sdk.SLOIntent(nil), intent.SLOs...), DataResidency: intent.DataResidency, Outputs: outputs}, nil
}

// Verify turns provider-specific observations into a release-gate result. It
// deliberately returns an incomplete result without an error when a declared
// signal is unavailable or a check is false; callers can then retain the
// explicit reason in evidence instead of collapsing it into an adapter error.
func (Adapter) Verify(ctx context.Context, plan Plan, probe VerificationProbe) (Verification, error) {
	if ctx == nil {
		return Verification{}, errors.New("observability verification context is required")
	}
	if probe == nil {
		return Verification{}, errors.New("observability verification probe is required")
	}
	verification := Verification{
		Signals: make([]SignalObservation, 0, len(plan.Bindings)),
		Operations: OperationalObservation{
			AlertsVerified:     len(plan.Alerts) == 0,
			DashboardsVerified: len(plan.Dashboards) == 0,
			SLOsVerified:       len(plan.SLOs) == 0,
		},
	}
	for _, status := range plan.SignalStatuses {
		if status.Status == "unavailable" {
			verification.Failures = append(verification.Failures, fmt.Sprintf("signal %s at %s is unavailable: %s", status.Signal, status.Destination, status.Reason))
		}
	}
	for _, operation := range plan.UnavailableOperations {
		verification.Failures = append(verification.Failures, "observability operation "+operation+" is unavailable")
	}
	for _, binding := range plan.Bindings {
		observation, err := probe.VerifySignal(ctx, binding)
		if err != nil {
			return Verification{}, fmt.Errorf("verify observability signal %s at %s: %w", binding.Signal, binding.Destination, err)
		}
		if observation.Signal != binding.Signal || observation.Destination != binding.Destination {
			return Verification{}, fmt.Errorf("observability probe returned %s at %s for %s at %s", observation.Signal, observation.Destination, binding.Signal, binding.Destination)
		}
		verification.Signals = append(verification.Signals, observation)
		if !observation.Delivered {
			verification.Failures = append(verification.Failures, "signal "+binding.Signal+" at "+binding.Destination+" was not delivered")
		}
		if !observation.LabelsVerified {
			verification.Failures = append(verification.Failures, "signal "+binding.Signal+" at "+binding.Destination+" did not retain required labels")
		}
		if !observation.RetentionVerified {
			verification.Failures = append(verification.Failures, "signal "+binding.Signal+" at "+binding.Destination+" did not prove retention")
		}
		if !observation.RedactionVerified {
			verification.Failures = append(verification.Failures, "signal "+binding.Signal+" at "+binding.Destination+" did not prove redaction")
		}
	}
	if len(plan.Alerts) > 0 || len(plan.Dashboards) > 0 || len(plan.SLOs) > 0 {
		operations, err := probe.VerifyOperations(ctx, plan)
		if err != nil {
			return Verification{}, fmt.Errorf("verify observability operational objects: %w", err)
		}
		verification.Operations = operations
		if len(plan.Alerts) > 0 && !operations.AlertsVerified {
			verification.Failures = append(verification.Failures, "declared observability alerts were not verified")
		}
		if len(plan.Dashboards) > 0 && !operations.DashboardsVerified {
			verification.Failures = append(verification.Failures, "declared observability dashboards were not verified")
		}
		if len(plan.SLOs) > 0 && !operations.SLOsVerified {
			verification.Failures = append(verification.Failures, "declared observability SLOs were not verified")
		}
	}
	cleanup, err := probe.VerifyCleanup(ctx, plan)
	if err != nil {
		return Verification{}, fmt.Errorf("verify observability cleanup: %w", err)
	}
	verification.Cleanup = cleanup
	if !cleanup.Complete {
		verification.Failures = append(verification.Failures, "owned observability resources remain")
	}
	if !cleanup.UnownedPreserved {
		verification.Failures = append(verification.Failures, "unowned observability resources were not proven preserved")
	}
	sort.Strings(verification.Failures)
	verification.Complete = len(verification.Failures) == 0
	return verification, nil
}

func observabilityOutputs(targetProvider, targetRuntime string, bindings []SignalBinding, statuses []SignalStatus, alerts []sdk.AlertIntent, dashboards []sdk.DashboardIntent, slos []sdk.SLOIntent) []Output {
	outputs := []Output{{Key: "observability.target_provider", Value: targetProvider}, {Key: "observability.target_runtime", Value: targetRuntime}}
	for index, binding := range bindings {
		outputs = append(outputs, Output{Key: fmt.Sprintf("observability.binding.%d", index), Value: binding.Destination + ":" + binding.Signal + ":" + binding.Mode})
	}
	for index, status := range statuses {
		value := status.Destination + ":" + status.Signal + ":" + status.Status
		if status.Reason != "" {
			value += ":" + status.Reason
		}
		outputs = append(outputs, Output{Key: fmt.Sprintf("observability.signal.%d", index), Value: value})
	}
	for index, alert := range alerts {
		outputs = append(outputs, Output{Key: fmt.Sprintf("observability.alert.%d", index), Value: alert.ID + ":" + alert.Signal + ":" + alert.Severity})
	}
	for index, dashboard := range dashboards {
		outputs = append(outputs, Output{Key: fmt.Sprintf("observability.dashboard.%d", index), Value: dashboard.ID + ":" + strings.Join(sdk.SortedStrings(dashboard.Signals), ",")})
	}
	for index, slo := range slos {
		outputs = append(outputs, Output{Key: fmt.Sprintf("observability.slo.%d", index), Value: slo.ID + ":" + slo.Signal})
	}
	return outputs
}

func nativeMatchesTarget(provider, target string) bool {
	switch target {
	case "aws":
		return provider == "cloudwatch"
	case "gcp":
		return provider == "google-cloud-operations"
	case "scaleway":
		return provider == "scaleway-cockpit"
	case "ovh":
		return provider == "ovh-logs-data-platform"
	default:
		return false
	}
}

func externalModeEndpoint(provider, configured string) (string, string) {
	if provider == "newrelic" {
		return "otlp", configured
	}
	return "exporter", configured
}

func nativeSignalStatus(provider, signal, nativeReference string) SignalStatus {
	status := SignalStatus{Signal: signal, Destination: provider, Status: "configured"}
	switch provider {
	case "cloudwatch":
		switch signal {
		case "logs", "metrics":
		default:
			status.Status = "unavailable"
			status.Reason = "the CloudWatch lifecycle adapter currently owns only log-group and custom-metric probes; use the provider-specific integration or an external OTLP destination for this signal"
		}
	case "google-cloud-operations":
		switch signal {
		case "logs", "metrics":
		default:
			status.Status = "unavailable"
			status.Reason = "the Google Cloud lifecycle adapter currently owns only Logging probes and custom Monitoring metrics; use the GKE/provider integration or an external OTLP destination for this signal"
		}
	case "scaleway-cockpit":
		switch signal {
		case "logs", "metrics", "traces":
		default:
			status.Status = "unavailable"
			status.Reason = "the Scaleway Cockpit lifecycle adapter currently owns only data-source lifecycle; provider health, audit, and alert semantics need an official adapter-backed integration or an external destination"
		}
	case "ovh-logs-data-platform":
		if strings.TrimSpace(nativeReference) == "" {
			status.Status = "unavailable"
			status.Reason = "OVH native audit delivery requires an opaque Logs Data Platform stream reference"
			return status
		}
		switch signal {
		case "audit-events", "provider-operations":
		default:
			status.Status = "unavailable"
			status.Reason = "OVH's first-party adapter currently provisions Kubernetes audit delivery only; workload logs, metrics, traces, and recovery signals require another adapter"
		}
	default:
		status.Status = "unavailable"
		status.Reason = "native provider is not registered by the observability adapter"
	}
	return status
}

func unavailableOperations(native, external string, intent sdk.ObservabilityIntent) []string {
	result := make([]string, 0, len(intent.Alerts)+len(intent.Dashboards)+len(intent.SLOs))
	appendOperation := func(kind, id string) {
		if strings.TrimSpace(id) != "" {
			result = append(result, kind+"."+id)
		}
	}
	// CloudWatch alarms and dashboards have provider-native lifecycle APIs;
	// CloudWatch has no portable SLO object in this adapter yet.
	if native == "cloudwatch" {
		for _, slo := range intent.SLOs {
			appendOperation("slo", slo.ID)
		}
	}
	// Google Cloud SLOs are children of an existing Monitoring Service. The
	// provider adapter can own them when that service identity is supplied as
	// the opaque native reference; keep the missing prerequisite explicit at
	// planning time rather than allowing a later mutation to fail.
	if native == "google-cloud-operations" {
		if strings.TrimSpace(intent.NativeReference) == "" {
			for _, slo := range intent.SLOs {
				appendOperation("slo", slo.ID)
			}
		}
	}
	// Cockpit alert references and OVH stream subscriptions are not a
	// provider-neutral alert/dashboard/SLO lifecycle. Do not imply ownership.
	// New Relic is intentionally absent: its provider-local NerdGraph adapter
	// owns these objects when configured, and returns an explicit incomplete
	// verification otherwise.
	if native == "scaleway-cockpit" || native == "ovh-logs-data-platform" {
		for _, alert := range intent.Alerts {
			appendOperation("alert", alert.ID)
		}
		for _, dashboard := range intent.Dashboards {
			appendOperation("dashboard", dashboard.ID)
		}
		for _, slo := range intent.SLOs {
			appendOperation("slo", slo.ID)
		}
	}
	sort.Strings(result)
	return result
}
