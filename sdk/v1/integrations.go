package v1

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// EdgeAction is a lifecycle capability exposed by an edge adapter. The core
// owns the portable intent and evidence gates; an adapter translates these
// actions to CloudFront, Cloud Armor, Fastly, or a community edge API.
type EdgeAction string

const (
	EdgeApply    EdgeAction = "apply"
	EdgeVerify   EdgeAction = "verify"
	EdgeFailover EdgeAction = "failover"
	EdgeRollback EdgeAction = "rollback"
	EdgeDestroy  EdgeAction = "destroy"
	EdgePurge    EdgeAction = "purge"
)

// EdgeProofOriginHealth is the provider-neutral semantic suffix for proof
// that the selected origin passed its application health gate. Adapters may
// namespace the suffix (for example, "aws.cloudfront.origin-health"); the
// shared lifecycle validates the semantic suffix without parsing provider
// identities.
const EdgeProofOriginHealth = "origin-health"

type EdgeAdapterDescriptor struct {
	APIVersion   string       `json:"apiVersion" yaml:"apiVersion"`
	ID           string       `json:"id" yaml:"id"`
	Provider     ProviderID   `json:"provider" yaml:"provider"`
	Version      string       `json:"version" yaml:"version"`
	Capabilities []EdgeAction `json:"capabilities" yaml:"capabilities"`
}

type AdapterOutput struct {
	Key   string `json:"key" yaml:"key"`
	Value string `json:"value" yaml:"value"`
}

type EdgePlanRequest struct {
	TargetProvider ProviderID     `json:"targetProvider" yaml:"targetProvider"`
	TargetRuntime  RuntimeID      `json:"targetRuntime" yaml:"targetRuntime"`
	Intent         EdgeIntent     `json:"intent" yaml:"intent"`
	Configuration  map[string]any `json:"configuration,omitempty" yaml:"configuration,omitempty"`
}

type EdgePlan struct {
	AdapterID       string          `json:"adapterId" yaml:"adapterId"`
	TargetProvider  ProviderID      `json:"targetProvider" yaml:"targetProvider"`
	TargetRuntime   RuntimeID       `json:"targetRuntime" yaml:"targetRuntime"`
	OwnershipMarker string          `json:"ownershipMarker" yaml:"ownershipMarker"`
	Outputs         []AdapterOutput `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	Opaque          any             `json:"-" yaml:"-"`
}

type EdgeExecutionRequest struct {
	Plan               EdgePlan   `json:"plan" yaml:"plan"`
	Action             EdgeAction `json:"action" yaml:"action"`
	IdempotencyKey     string     `json:"idempotencyKey" yaml:"idempotencyKey"`
	OwnershipMarker    string     `json:"ownershipMarker" yaml:"ownershipMarker"`
	ApprovalReference  string     `json:"approvalReference,omitempty" yaml:"approvalReference,omitempty"`
	ResourceReferences []string   `json:"resourceReferences,omitempty" yaml:"resourceReferences,omitempty"`
}

type EdgeExecutionResult struct {
	Action              EdgeAction      `json:"action" yaml:"action"`
	OperationID         string          `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	ResourceRefs        []string        `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	ProofRefs           []string        `json:"proofRefs,omitempty" yaml:"proofRefs,omitempty"`
	Outputs             []AdapterOutput `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	OwnershipMarker     string          `json:"ownershipMarker" yaml:"ownershipMarker"`
	OwnershipVerified   bool            `json:"ownershipVerified" yaml:"ownershipVerified"`
	IdempotencyVerified bool            `json:"idempotencyVerified" yaml:"idempotencyVerified"`
}

// EdgeCapabilityStatus is the adapter's semantic availability result for an
// edge action. A missing descriptor capability is an explicit unsupported
// boundary, not an implicit no-op.
type EdgeCapabilityStatus string

const (
	EdgeCapabilitySupported    EdgeCapabilityStatus = "supported"
	EdgeCapabilityCertified    EdgeCapabilityStatus = "certified"
	EdgeCapabilityExperimental EdgeCapabilityStatus = "experimental"
	EdgeCapabilityUnavailable  EdgeCapabilityStatus = "unavailable"
	EdgeCapabilityUnsupported  EdgeCapabilityStatus = "unsupported"
	EdgeCapabilityBlocked      EdgeCapabilityStatus = "blocked"
)

// EdgeCapabilityError identifies an edge action that the selected adapter
// cannot execute. It is safe to persist in a plan or evidence record because
// it contains no provider SDK object or credential value.
type EdgeCapabilityError struct {
	AdapterID string               `json:"adapterId" yaml:"adapterId"`
	Action    EdgeAction           `json:"action" yaml:"action"`
	Status    EdgeCapabilityStatus `json:"status" yaml:"status"`
	Reason    string               `json:"reason" yaml:"reason"`
}

func (e EdgeCapabilityError) Error() string {
	if e.AdapterID == "" && e.Action == "" && e.Reason == "" {
		return "edge capability is unavailable"
	}
	return fmt.Sprintf("edge capability %s for adapter %q and action %q: %s", e.Status, e.AdapterID, e.Action, e.Reason)
}

// EdgeAdapter is optional. A target that does not expose it cannot claim
// automated native or external edge lifecycle certification.
type EdgeAdapter interface {
	EdgeDescriptor() EdgeAdapterDescriptor
	PlanEdge(context.Context, EdgePlanRequest) (EdgePlan, error)
	ExecuteEdge(context.Context, EdgeExecutionRequest) (EdgeExecutionResult, error)
}

func ValidateEdgeAdapterDescriptor(descriptor EdgeAdapterDescriptor) error {
	var problems []error
	if descriptor.APIVersion != ExtensionAPIVersion {
		problems = append(problems, fmt.Errorf("edge adapter API version %q is not supported", descriptor.APIVersion))
	}
	problems = append(problems,
		validateID("edge adapter ID", descriptor.ID),
		validateID("edge adapter provider ID", string(descriptor.Provider)),
		validateExtensionVersion(descriptor.Version),
	)
	if len(descriptor.Capabilities) == 0 {
		problems = append(problems, errors.New("edge adapter must declare at least one capability"))
	}
	seen := make(map[EdgeAction]struct{}, len(descriptor.Capabilities))
	for _, capability := range descriptor.Capabilities {
		if !validEdgeAction(capability) {
			problems = append(problems, fmt.Errorf("invalid edge adapter capability %q", capability))
		}
		if _, exists := seen[capability]; exists {
			problems = append(problems, fmt.Errorf("duplicate edge adapter capability %q", capability))
		}
		seen[capability] = struct{}{}
	}
	return errors.Join(problems...)
}

func ValidateEdgePlanRequest(request EdgePlanRequest) error {
	var problems []error
	problems = append(problems,
		validateID("edge target provider", string(request.TargetProvider)),
		validateID("edge target runtime", string(request.TargetRuntime)),
		ValidateEdgeIntent(request.Intent),
	)
	return errors.Join(problems...)
}

func ValidateEdgePlan(plan EdgePlan, request EdgePlanRequest) error {
	var problems []error
	if strings.TrimSpace(plan.AdapterID) == "" {
		problems = append(problems, errors.New("edge plan adapter ID is required"))
	}
	if plan.TargetProvider != request.TargetProvider || plan.TargetRuntime != request.TargetRuntime {
		problems = append(problems, errors.New("edge plan target does not match the plan request"))
	}
	if strings.TrimSpace(plan.OwnershipMarker) == "" && request.Intent.Mode != "none" {
		problems = append(problems, errors.New("edge plan ownership marker is required for an active edge"))
	}
	for _, output := range plan.Outputs {
		if strings.TrimSpace(output.Key) == "" || strings.ContainsAny(output.Key+output.Value, "\r\n\x00") {
			problems = append(problems, errors.New("edge plan outputs must be non-empty and single-line"))
		}
	}
	return errors.Join(problems...)
}

func ValidateEdgeExecutionRequest(request EdgeExecutionRequest) error {
	if !validEdgeAction(request.Action) {
		return fmt.Errorf("invalid edge execution action %q", request.Action)
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" || strings.ContainsAny(request.IdempotencyKey, "\r\n\x00") {
		return errors.New("edge execution idempotency key is required and must be single-line")
	}
	if request.OwnershipMarker != request.Plan.OwnershipMarker {
		return errors.New("edge execution ownership marker does not match the plan")
	}
	if strings.ContainsAny(request.ApprovalReference, "\r\n\x00") {
		return errors.New("edge execution approval reference must be single-line")
	}
	if err := validateOpaqueReferences("edge execution resource reference", request.ResourceReferences); err != nil {
		return err
	}
	return nil
}

func validEdgeAction(action EdgeAction) bool {
	switch action {
	case EdgeApply, EdgeVerify, EdgeFailover, EdgeRollback, EdgeDestroy, EdgePurge:
		return true
	default:
		return false
	}
}

// ObservabilityAction is a lifecycle capability exposed by a telemetry
// adapter. It is intentionally separate from EdgeAction so a provider can
// support native metrics without pretending to own edge resources.
type ObservabilityAction string

const (
	ObservabilityApply   ObservabilityAction = "apply"
	ObservabilityVerify  ObservabilityAction = "verify"
	ObservabilityDestroy ObservabilityAction = "destroy"
)

type ObservabilityAdapterDescriptor struct {
	APIVersion   string                `json:"apiVersion" yaml:"apiVersion"`
	ID           string                `json:"id" yaml:"id"`
	Provider     ProviderID            `json:"provider" yaml:"provider"`
	Version      string                `json:"version" yaml:"version"`
	Capabilities []ObservabilityAction `json:"capabilities" yaml:"capabilities"`
}

type ObservabilityPlanRequest struct {
	TargetProvider ProviderID          `json:"targetProvider" yaml:"targetProvider"`
	TargetRuntime  RuntimeID           `json:"targetRuntime" yaml:"targetRuntime"`
	Intent         ObservabilityIntent `json:"intent" yaml:"intent"`
	Configuration  map[string]any      `json:"configuration,omitempty" yaml:"configuration,omitempty"`
}

type ObservabilityBinding struct {
	Signal          string `json:"signal" yaml:"signal"`
	Destination     string `json:"destination" yaml:"destination"`
	Mode            string `json:"mode" yaml:"mode"`
	CredentialRef   string `json:"credentialRef,omitempty" yaml:"credentialRef,omitempty"`
	Endpoint        string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	RetentionDays   int    `json:"retentionDays,omitempty" yaml:"retentionDays,omitempty"`
	RedactionPolicy string `json:"redactionPolicy,omitempty" yaml:"redactionPolicy,omitempty"`
}

type ObservabilityPlan struct {
	AdapterID       string     `json:"adapterId" yaml:"adapterId"`
	TargetProvider  ProviderID `json:"targetProvider" yaml:"targetProvider"`
	TargetRuntime   RuntimeID  `json:"targetRuntime" yaml:"targetRuntime"`
	OwnershipMarker string     `json:"ownershipMarker" yaml:"ownershipMarker"`
	// NativeReference is an opaque provider-owned identity carried through the
	// normalized plan. The SDK validates its shape but never parses it.
	NativeReference string                 `json:"nativeReference,omitempty" yaml:"nativeReference,omitempty"`
	Bindings        []ObservabilityBinding `json:"bindings,omitempty" yaml:"bindings,omitempty"`
	// Operational objects stay in the portable plan as semantic intent. The
	// provider adapter owns the native alert, dashboard, and SLO resource
	// models, but the core must retain the requested objects for validation,
	// evidence, and fail-closed capability checks.
	Alerts             []AlertIntent     `json:"alerts,omitempty" yaml:"alerts,omitempty"`
	Dashboards         []DashboardIntent `json:"dashboards,omitempty" yaml:"dashboards,omitempty"`
	SLOs               []SLOIntent       `json:"slos,omitempty" yaml:"slos,omitempty"`
	DataResidency      string            `json:"dataResidency,omitempty" yaml:"dataResidency,omitempty"`
	UnavailableSignals []string          `json:"unavailableSignals,omitempty" yaml:"unavailableSignals,omitempty"`
	// UnavailableOperations keeps provider gaps for alerts, dashboards, and
	// SLOs explicit instead of allowing an adapter to silently drop them.
	UnavailableOperations []string        `json:"unavailableOperations,omitempty" yaml:"unavailableOperations,omitempty"`
	Outputs               []AdapterOutput `json:"outputs,omitempty" yaml:"outputs,omitempty"`
	Opaque                any             `json:"-" yaml:"-"`
}

type ObservabilityExecutionRequest struct {
	Plan               ObservabilityPlan   `json:"plan" yaml:"plan"`
	Action             ObservabilityAction `json:"action" yaml:"action"`
	IdempotencyKey     string              `json:"idempotencyKey" yaml:"idempotencyKey"`
	OwnershipMarker    string              `json:"ownershipMarker" yaml:"ownershipMarker"`
	ApprovalReference  string              `json:"approvalReference,omitempty" yaml:"approvalReference,omitempty"`
	ResourceReferences []string            `json:"resourceReferences,omitempty" yaml:"resourceReferences,omitempty"`
}

type ObservabilityExecutionResult struct {
	Action              ObservabilityAction `json:"action" yaml:"action"`
	OperationID         string              `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	ResourceRefs        []string            `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	ProofRefs           []string            `json:"proofRefs,omitempty" yaml:"proofRefs,omitempty"`
	OwnershipMarker     string              `json:"ownershipMarker" yaml:"ownershipMarker"`
	OwnershipVerified   bool                `json:"ownershipVerified" yaml:"ownershipVerified"`
	IdempotencyVerified bool                `json:"idempotencyVerified" yaml:"idempotencyVerified"`
}

// ObservabilityCapabilityError reports a planned observability operation that
// the selected adapter cannot execute. It is returned before apply so a
// provider cannot create a partial native or external telemetry setup and
// only discover the gap during verification.
type ObservabilityCapabilityError struct {
	Signal    string
	Operation string
	Reason    string
}

func (e ObservabilityCapabilityError) Error() string {
	if e.Signal != "" {
		if e.Reason == "" {
			return fmt.Sprintf("observability signal %q is unavailable", e.Signal)
		}
		return fmt.Sprintf("observability signal %q is unavailable: %s", e.Signal, e.Reason)
	}
	if e.Operation != "" {
		if e.Reason == "" {
			return fmt.Sprintf("observability operation %q is unavailable", e.Operation)
		}
		return fmt.Sprintf("observability operation %q is unavailable: %s", e.Operation, e.Reason)
	}
	return "observability capability is unavailable"
}

// ValidateObservabilityApplyPlan is the shared pre-mutation gate for
// observability adapters. Verification may inspect an unavailable plan to
// produce evidence, and destroy may reconcile resources created by an older
// run, but apply must fail closed before any provider API mutation.
func ValidateObservabilityApplyPlan(plan ObservabilityPlan) error {
	for _, signal := range plan.UnavailableSignals {
		if err := validateID("unavailable observability signal", signal); err != nil {
			return err
		}
	}
	for _, operation := range plan.UnavailableOperations {
		if err := validateID("unavailable observability operation", operation); err != nil {
			return err
		}
	}
	if len(plan.UnavailableSignals) > 0 {
		return ObservabilityCapabilityError{Signal: plan.UnavailableSignals[0]}
	}
	if len(plan.UnavailableOperations) > 0 {
		return ObservabilityCapabilityError{Operation: plan.UnavailableOperations[0]}
	}
	return nil
}

// ObservabilityAdapter is optional. A target that does not expose it cannot
// claim native or New Relic lifecycle certification, even if it can emit a
// configured exporter from application code.
type ObservabilityAdapter interface {
	ObservabilityDescriptor() ObservabilityAdapterDescriptor
	PlanObservability(context.Context, ObservabilityPlanRequest) (ObservabilityPlan, error)
	ExecuteObservability(context.Context, ObservabilityExecutionRequest) (ObservabilityExecutionResult, error)
}

func ValidateObservabilityAdapterDescriptor(descriptor ObservabilityAdapterDescriptor) error {
	var problems []error
	if descriptor.APIVersion != ExtensionAPIVersion {
		problems = append(problems, fmt.Errorf("observability adapter API version %q is not supported", descriptor.APIVersion))
	}
	problems = append(problems,
		validateID("observability adapter ID", descriptor.ID),
		validateID("observability adapter provider ID", string(descriptor.Provider)),
		validateExtensionVersion(descriptor.Version),
	)
	if len(descriptor.Capabilities) == 0 {
		problems = append(problems, errors.New("observability adapter must declare at least one capability"))
	}
	seen := make(map[ObservabilityAction]struct{}, len(descriptor.Capabilities))
	for _, capability := range descriptor.Capabilities {
		if capability != ObservabilityApply && capability != ObservabilityVerify && capability != ObservabilityDestroy {
			problems = append(problems, fmt.Errorf("invalid observability adapter capability %q", capability))
		}
		if _, exists := seen[capability]; exists {
			problems = append(problems, fmt.Errorf("duplicate observability adapter capability %q", capability))
		}
		seen[capability] = struct{}{}
	}
	return errors.Join(problems...)
}

func observabilityActionSupported(capabilities []ObservabilityAction, action ObservabilityAction) bool {
	for _, capability := range capabilities {
		if capability == action {
			return true
		}
	}
	return false
}

func ValidateObservabilityPlanRequest(request ObservabilityPlanRequest) error {
	var problems []error
	problems = append(problems,
		validateID("observability target provider", string(request.TargetProvider)),
		validateID("observability target runtime", string(request.TargetRuntime)),
		ValidateObservabilityIntent(request.Intent),
	)
	return errors.Join(problems...)
}

func ValidateObservabilityPlan(plan ObservabilityPlan, request ObservabilityPlanRequest) error {
	var problems []error
	if strings.TrimSpace(plan.AdapterID) == "" {
		problems = append(problems, errors.New("observability plan adapter ID is required"))
	}
	if plan.TargetProvider != request.TargetProvider || plan.TargetRuntime != request.TargetRuntime {
		problems = append(problems, errors.New("observability plan target does not match the plan request"))
	}
	if strings.TrimSpace(plan.OwnershipMarker) == "" && (activeProvider(request.Intent.NativeProvider) || activeProvider(request.Intent.ExternalProvider)) {
		problems = append(problems, errors.New("observability plan ownership marker is required for an active destination"))
	}
	if strings.ContainsAny(plan.DataResidency, "\r\n\x00") {
		problems = append(problems, errors.New("observability plan data residency must be single-line and NUL-free"))
	}
	if strings.ContainsAny(plan.NativeReference, "\r\n\x00") {
		problems = append(problems, errors.New("observability plan native reference must be single-line and NUL-free"))
	}
	if plan.NativeReference != request.Intent.NativeReference {
		problems = append(problems, errors.New("observability plan must preserve the native reference"))
	}
	if plan.DataResidency != request.Intent.DataResidency {
		problems = append(problems, errors.New("observability plan must preserve data residency"))
	}
	if !observabilityAlertsMatch(plan.Alerts, request.Intent.Alerts) || !observabilityDashboardsMatch(plan.Dashboards, request.Intent.Dashboards) || !observabilitySLOsMatch(plan.SLOs, request.Intent.SLOs) {
		problems = append(problems, errors.New("observability plan must preserve all requested alerts, dashboards, and SLOs"))
	}
	problems = append(problems, validateObservabilitySignalProjection(plan, request.Intent)...)
	for _, alert := range plan.Alerts {
		problems = append(problems, validateAlertIntent(alert))
	}
	for _, dashboard := range plan.Dashboards {
		problems = append(problems, validateDashboardIntent(dashboard))
	}
	for _, slo := range plan.SLOs {
		problems = append(problems, validateSLOIntent(slo))
	}
	for _, binding := range plan.Bindings {
		if err := validateID("observability binding signal", binding.Signal); err != nil {
			problems = append(problems, err)
		}
		if err := validateID("observability binding destination", binding.Destination); err != nil {
			problems = append(problems, err)
		}
		if binding.CredentialRef != "" {
			if err := ValidateCredentialReference(binding.CredentialRef); err != nil {
				problems = append(problems, fmt.Errorf("observability binding credential: %w", err))
			}
		}
		if binding.RetentionDays < 0 {
			problems = append(problems, fmt.Errorf("observability binding %q retention cannot be negative", binding.Signal))
		}
	}
	for _, signal := range plan.UnavailableSignals {
		if err := validateID("unavailable observability signal", signal); err != nil {
			problems = append(problems, err)
		}
	}
	for _, operation := range plan.UnavailableOperations {
		if err := validateID("unavailable observability operation", operation); err != nil {
			problems = append(problems, err)
		}
	}
	return errors.Join(problems...)
}

func validateObservabilitySignalProjection(plan ObservabilityPlan, intent ObservabilityIntent) []error {
	requested := make(map[string]struct{}, len(intent.Signals))
	for _, signal := range intent.Signals {
		requested[signal] = struct{}{}
	}
	projected := make(map[string]struct{}, len(plan.Bindings))
	for _, binding := range plan.Bindings {
		if _, exists := requested[binding.Signal]; !exists {
			return []error{fmt.Errorf("observability plan contains unrequested signal %q", binding.Signal)}
		}
		projected[binding.Signal] = struct{}{}
	}
	unavailable := make(map[string]struct{}, len(plan.UnavailableSignals))
	for _, signal := range plan.UnavailableSignals {
		if _, exists := requested[signal]; !exists {
			return []error{fmt.Errorf("observability plan marks unrequested signal %q unavailable", signal)}
		}
		if _, exists := unavailable[signal]; exists {
			return []error{fmt.Errorf("duplicate unavailable observability signal %q", signal)}
		}
		unavailable[signal] = struct{}{}
	}
	for signal := range requested {
		if _, bound := projected[signal]; bound {
			continue
		}
		if _, explicitlyUnavailable := unavailable[signal]; explicitlyUnavailable {
			continue
		}
		return []error{fmt.Errorf("observability plan dropped requested signal %q without an unavailable capability", signal)}
	}
	return nil
}

func observabilityAlertsMatch(actual, expected []AlertIntent) bool {
	if len(actual) != len(expected) {
		return false
	}
	byID := make(map[string]AlertIntent, len(expected))
	for _, alert := range expected {
		byID[alert.ID] = alert
	}
	for _, alert := range actual {
		expectedAlert, ok := byID[alert.ID]
		if !ok || alert != expectedAlert {
			return false
		}
	}
	return true
}

func observabilityDashboardsMatch(actual, expected []DashboardIntent) bool {
	if len(actual) != len(expected) {
		return false
	}
	byID := make(map[string]DashboardIntent, len(expected))
	for _, dashboard := range expected {
		byID[dashboard.ID] = dashboard
	}
	for _, dashboard := range actual {
		expectedDashboard, ok := byID[dashboard.ID]
		if !ok || dashboard.Owner != expectedDashboard.Owner || !equalStringSet(dashboard.Signals, expectedDashboard.Signals) {
			return false
		}
	}
	return true
}

func observabilitySLOsMatch(actual, expected []SLOIntent) bool {
	if len(actual) != len(expected) {
		return false
	}
	byID := make(map[string]SLOIntent, len(expected))
	for _, slo := range expected {
		byID[slo.ID] = slo
	}
	for _, slo := range actual {
		expectedSLO, ok := byID[slo.ID]
		if !ok || slo != expectedSLO {
			return false
		}
	}
	return true
}

func equalStringSet(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	actualSorted := SortedStrings(actual)
	expectedSorted := SortedStrings(expected)
	for index := range actualSorted {
		if actualSorted[index] != expectedSorted[index] {
			return false
		}
	}
	return true
}

func activeProvider(provider string) bool {
	provider = strings.TrimSpace(provider)
	return provider != "" && provider != "none"
}

func ValidateObservabilityExecutionRequest(request ObservabilityExecutionRequest) error {
	if request.Action != ObservabilityApply && request.Action != ObservabilityVerify && request.Action != ObservabilityDestroy {
		return fmt.Errorf("invalid observability execution action %q", request.Action)
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" || strings.ContainsAny(request.IdempotencyKey, "\r\n\x00") {
		return errors.New("observability execution idempotency key is required and must be single-line")
	}
	if request.OwnershipMarker != request.Plan.OwnershipMarker {
		return errors.New("observability execution ownership marker does not match the plan")
	}
	if strings.ContainsAny(request.ApprovalReference, "\r\n\x00") {
		return errors.New("observability execution approval reference must be single-line")
	}
	if err := validateOpaqueReferences("observability execution resource reference", request.ResourceReferences); err != nil {
		return err
	}
	return nil
}

func validateOpaqueReferences(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("%s must be non-empty and single-line", name)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate %s %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}
