package observability

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// LifecycleResult is the provider-neutral result returned by an injected
// telemetry implementation. The implementation may be CloudWatch, Google
// Cloud Operations, Cockpit, OVH Logs Data Platform, New Relic, or a
// community collector; no vendor object crosses this boundary.
type LifecycleResult struct {
	OperationID         string
	ResourceRefs        []string
	ProofRefs           []string
	OwnershipVerified   bool
	IdempotencyVerified bool
}

// LifecycleClient supplies side effects and live probes to SDKAdapter. The
// core planner remains usable with a nil client for read-only plan inspection.
type LifecycleClient interface {
	VerificationProbe
	Apply(context.Context, Plan) (LifecycleResult, error)
	Destroy(context.Context, Plan, []string) (CleanupObservation, error)
}

// SDKAdapter maps the existing portable observability planner to the public
// lifecycle port. Its provider ID is configured by the target module so the
// same implementation can serve a native-only, New Relic-only, or combined
// destination without duplicating signal semantics.
type SDKAdapter struct {
	Provider sdk.ProviderID
	Adapter  Adapter
	Client   LifecycleClient
}

func NewSDKAdapter(provider sdk.ProviderID, adapter Adapter, client LifecycleClient) SDKAdapter {
	return SDKAdapter{Provider: provider, Adapter: adapter, Client: client}
}

func (adapter SDKAdapter) ObservabilityDescriptor() sdk.ObservabilityAdapterDescriptor {
	return sdk.ObservabilityAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: string(adapter.Provider) + ".observability", Provider: adapter.Provider, Version: "1.0.0",
		Capabilities: []sdk.ObservabilityAction{sdk.ObservabilityApply, sdk.ObservabilityVerify, sdk.ObservabilityDestroy},
	}
}

func (adapter SDKAdapter) PlanObservability(ctx context.Context, request sdk.ObservabilityPlanRequest) (sdk.ObservabilityPlan, error) {
	if ctx == nil {
		return sdk.ObservabilityPlan{}, errors.New("observability SDK planning context is required")
	}
	if err := ctx.Err(); err != nil {
		return sdk.ObservabilityPlan{}, err
	}
	if err := sdk.ValidateObservabilityPlanRequest(request); err != nil {
		return sdk.ObservabilityPlan{}, err
	}
	plan, err := adapter.Adapter.Plan(Request{TargetProvider: string(request.TargetProvider), TargetRuntime: string(request.TargetRuntime), Intent: request.Intent})
	if err != nil {
		return sdk.ObservabilityPlan{}, err
	}
	bindings := make([]sdk.ObservabilityBinding, 0, len(plan.Bindings))
	for _, binding := range plan.Bindings {
		bindings = append(bindings, sdk.ObservabilityBinding{Signal: binding.Signal, Destination: binding.Destination, Mode: binding.Mode, CredentialRef: binding.CredentialRef, Endpoint: binding.Endpoint, RetentionDays: binding.RetentionDays, RedactionPolicy: binding.RedactionPolicy})
	}
	var unavailable []string
	for _, status := range plan.SignalStatuses {
		if status.Status == "unavailable" {
			unavailable = append(unavailable, status.Signal)
		}
	}
	outputs := make([]sdk.AdapterOutput, 0, len(plan.Outputs))
	for _, output := range plan.Outputs {
		outputs = append(outputs, sdk.AdapterOutput{Key: output.Key, Value: output.Value})
	}
	publicPlan := sdk.ObservabilityPlan{
		AdapterID:             adapter.ObservabilityDescriptor().ID,
		TargetProvider:        request.TargetProvider,
		TargetRuntime:         request.TargetRuntime,
		OwnershipMarker:       plan.OwnershipMarker,
		NativeReference:       plan.NativeReference,
		Bindings:              bindings,
		Alerts:                append([]sdk.AlertIntent(nil), plan.Alerts...),
		Dashboards:            append([]sdk.DashboardIntent(nil), plan.Dashboards...),
		SLOs:                  append([]sdk.SLOIntent(nil), plan.SLOs...),
		DataResidency:         plan.DataResidency,
		UnavailableSignals:    sdk.SortedStrings(unavailable),
		UnavailableOperations: append([]string(nil), plan.UnavailableOperations...),
		Outputs:               outputs,
		Opaque:                plan,
	}
	if err := sdk.ValidateObservabilityPlan(publicPlan, request); err != nil {
		return sdk.ObservabilityPlan{}, fmt.Errorf("validate observability SDK plan: %w", err)
	}
	return publicPlan, nil
}

func (adapter SDKAdapter) ExecuteObservability(ctx context.Context, request sdk.ObservabilityExecutionRequest) (sdk.ObservabilityExecutionResult, error) {
	if ctx == nil {
		return sdk.ObservabilityExecutionResult{}, errors.New("observability SDK execution context is required")
	}
	if adapter.Client == nil {
		return sdk.ObservabilityExecutionResult{}, errors.New("observability SDK lifecycle client is required")
	}
	if err := sdk.ValidateObservabilityExecutionRequest(request); err != nil {
		return sdk.ObservabilityExecutionResult{}, err
	}
	plan, ok := request.Plan.Opaque.(Plan)
	if !ok {
		return sdk.ObservabilityExecutionResult{}, errors.New("observability SDK plan is missing its adapter-owned plan")
	}
	if err := adapter.validateExecutionPlan(request, plan); err != nil {
		return sdk.ObservabilityExecutionResult{}, err
	}
	var lifecycle LifecycleResult
	var err error
	switch request.Action {
	case sdk.ObservabilityApply:
		if err := sdk.ValidateObservabilityApplyPlan(request.Plan); err != nil {
			return sdk.ObservabilityExecutionResult{}, err
		}
		lifecycle, err = adapter.Client.Apply(ctx, plan)
	case sdk.ObservabilityVerify:
		var verification Verification
		verification, err = adapter.Adapter.Verify(ctx, plan, adapter.Client)
		if err == nil && !verification.Complete {
			err = fmt.Errorf("observability verification incomplete: %v", verification.Failures)
		}
		lifecycle = LifecycleResult{OperationID: "verify:" + plan.TargetProvider + ":" + plan.TargetRuntime, ResourceRefs: verification.Cleanup.ResourceRefs, ProofRefs: observabilityProofRefs(verification), OwnershipVerified: verification.Cleanup.UnownedPreserved, IdempotencyVerified: true}
	case sdk.ObservabilityDestroy:
		cleanup, cleanupErr := adapter.Client.Destroy(ctx, plan, request.ResourceReferences)
		err = cleanupErr
		lifecycle = LifecycleResult{OperationID: "destroy:" + plan.TargetProvider + ":" + plan.TargetRuntime, ResourceRefs: cleanup.ResourceRefs, ProofRefs: []string{"observability:owning-service-inventory"}, OwnershipVerified: cleanup.UnownedPreserved, IdempotencyVerified: cleanup.Complete}
	default:
		return sdk.ObservabilityExecutionResult{}, fmt.Errorf("unsupported observability SDK action %q", request.Action)
	}
	if err != nil {
		return sdk.ObservabilityExecutionResult{}, err
	}
	return sdk.ObservabilityExecutionResult{Action: request.Action, OperationID: lifecycle.OperationID, ResourceRefs: append([]string(nil), lifecycle.ResourceRefs...), ProofRefs: append([]string(nil), lifecycle.ProofRefs...), OwnershipMarker: request.OwnershipMarker, OwnershipVerified: lifecycle.OwnershipVerified, IdempotencyVerified: lifecycle.IdempotencyVerified}, nil
}

func (adapter SDKAdapter) validateExecutionPlan(request sdk.ObservabilityExecutionRequest, plan Plan) error {
	descriptor := adapter.ObservabilityDescriptor()
	if err := sdk.ValidateObservabilityAdapterDescriptor(descriptor); err != nil {
		return fmt.Errorf("validate observability SDK adapter descriptor: %w", err)
	}
	if request.Plan.AdapterID != descriptor.ID || request.Plan.TargetProvider != descriptor.Provider || plan.TargetProvider != string(descriptor.Provider) {
		return fmt.Errorf("observability plan does not match the executing provider boundary")
	}
	if request.Plan.TargetRuntime != sdk.RuntimeID(plan.TargetRuntime) || plan.OwnershipMarker != request.Plan.OwnershipMarker || plan.NativeReference != request.Plan.NativeReference {
		return errors.New("observability plan does not match its adapter-owned target or ownership boundary")
	}
	if len(plan.Bindings) != len(request.Plan.Bindings) {
		return errors.New("observability plan bindings do not match its adapter-owned plan")
	}
	for index, binding := range plan.Bindings {
		public := request.Plan.Bindings[index]
		if binding.Signal != public.Signal || binding.Destination != public.Destination || binding.Mode != public.Mode || binding.CredentialRef != public.CredentialRef || binding.Endpoint != public.Endpoint || binding.RetentionDays != public.RetentionDays || binding.RedactionPolicy != public.RedactionPolicy {
			return errors.New("observability plan bindings do not match its adapter-owned plan")
		}
	}
	if !reflect.DeepEqual(plan.Alerts, request.Plan.Alerts) || !reflect.DeepEqual(plan.Dashboards, request.Plan.Dashboards) || !reflect.DeepEqual(plan.SLOs, request.Plan.SLOs) || plan.DataResidency != request.Plan.DataResidency || !sameStrings(plan.UnavailableOperations, request.Plan.UnavailableOperations) {
		return errors.New("observability plan intent does not match its adapter-owned plan")
	}
	unavailable := make([]string, 0, len(plan.SignalStatuses))
	for _, status := range plan.SignalStatuses {
		if status.Status == "unavailable" {
			unavailable = append(unavailable, status.Signal)
		}
	}
	sort.Strings(unavailable)
	expectedUnavailable := sdk.SortedStrings(request.Plan.UnavailableSignals)
	if !sameStrings(unavailable, expectedUnavailable) {
		return errors.New("observability plan signal availability does not match its adapter-owned plan")
	}
	if len(plan.Outputs) != len(request.Plan.Outputs) {
		return errors.New("observability plan outputs do not match its adapter-owned plan")
	}
	for index, output := range plan.Outputs {
		public := request.Plan.Outputs[index]
		if output.Key != public.Key || output.Value != public.Value {
			return errors.New("observability plan outputs do not match its adapter-owned plan")
		}
	}
	return nil
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func observabilityProofRefs(verification Verification) []string {
	refs := make([]string, 0, len(verification.Signals)+3)
	for _, signal := range verification.Signals {
		if signal.Delivered && signal.LabelsVerified && signal.RetentionVerified && signal.RedactionVerified {
			refs = append(refs, "observability:"+signal.Destination+":"+signal.Signal)
		}
	}
	if verification.Operations.AlertsVerified {
		refs = append(refs, "observability:alerts")
	}
	if verification.Operations.DashboardsVerified {
		refs = append(refs, "observability:dashboards")
	}
	if verification.Operations.SLOsVerified {
		refs = append(refs, "observability:slos")
	}
	return refs
}
