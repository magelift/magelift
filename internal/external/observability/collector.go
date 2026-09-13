package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// CollectorResource is the provider-neutral inventory view returned by an
// injected ECS, Helm, or client-go backend. Provider SDK resource objects stay
// behind CollectorBackend.
type CollectorResource struct {
	Identity        string
	TargetProvider  sdk.ProviderID
	TargetRuntime   sdk.RuntimeID
	Workload        string
	Distribution    string
	OwnershipMarker string
	Status          string
	Owned           bool
}

// CollectorVerification is the minimum readiness proof needed before a
// collector deployment can be considered usable. An accepted task definition
// or created Helm release is not delivery proof.
type CollectorVerification struct {
	Ready            bool
	Healthy          bool
	SignalsDelivered bool
	Reason           string
}

// CollectorBackend is the provider SDK port. AWS implementations map it to
// ECS task-definition/service APIs; Kubernetes implementations map it to Helm
// or client-go objects. The shared lifecycle engine owns idempotency,
// ownership, rollback ordering, and final inventory checks.
type CollectorBackend interface {
	Find(context.Context, sdk.CollectorDeploymentPlan) (CollectorResource, bool, error)
	Ensure(context.Context, sdk.CollectorDeploymentPlan) (CollectorResource, error)
	Verify(context.Context, sdk.CollectorDeploymentPlan, CollectorResource) (CollectorVerification, error)
	Rollback(context.Context, sdk.CollectorDeploymentPlan, CollectorResource) error
	Destroy(context.Context, sdk.CollectorDeploymentPlan, CollectorResource) error
	Inventory(context.Context, sdk.ProviderID, sdk.RuntimeID, string) ([]CollectorResource, error)
}

// CollectorSDKAdapter is the reusable SDK-facing lifecycle implementation.
// Target-specific constructors only provide validation predicates and a
// provider-owned backend; they do not duplicate lifecycle semantics.
type CollectorSDKAdapter struct {
	adapterID         string
	provider          sdk.ProviderID
	workload          string
	runtimeMatch      func(sdk.RuntimeID) bool
	distributionMatch func(string) bool
	backend           CollectorBackend
}

var _ sdk.CollectorDeploymentAdapter = (*CollectorSDKAdapter)(nil)

// NewCollectorSDKAdapter creates the shared collector lifecycle engine for an
// injected provider backend.
func NewCollectorSDKAdapter(adapterID string, provider sdk.ProviderID, workload string, runtimeMatch func(sdk.RuntimeID) bool, backend CollectorBackend) (*CollectorSDKAdapter, error) {
	return NewCollectorSDKAdapterWithDistribution(adapterID, provider, workload, runtimeMatch, func(string) bool { return true }, backend)
}

// NewCollectorSDKAdapterWithDistribution adds the provider/runtime-specific
// collector distribution gate while reusing the same lifecycle engine.
func NewCollectorSDKAdapterWithDistribution(adapterID string, provider sdk.ProviderID, workload string, runtimeMatch func(sdk.RuntimeID) bool, distributionMatch func(string) bool, backend CollectorBackend) (*CollectorSDKAdapter, error) {
	if strings.TrimSpace(adapterID) == "" || strings.TrimSpace(workload) == "" || runtimeMatch == nil || distributionMatch == nil || backend == nil {
		return nil, errors.New("collector adapter ID, workload, runtime matcher, distribution matcher, and backend are required")
	}
	return &CollectorSDKAdapter{adapterID: adapterID, provider: provider, workload: workload, runtimeMatch: runtimeMatch, distributionMatch: distributionMatch, backend: backend}, nil
}

func (adapter *CollectorSDKAdapter) PlanCollector(ctx context.Context, request sdk.CollectorDeploymentPlanRequest) (sdk.CollectorDeploymentPlan, error) {
	if ctx == nil {
		return sdk.CollectorDeploymentPlan{}, errors.New("collector planning context is required")
	}
	if err := ctx.Err(); err != nil {
		return sdk.CollectorDeploymentPlan{}, err
	}
	if err := sdk.ValidateCollectorDeploymentPlanRequest(request); err != nil {
		return sdk.CollectorDeploymentPlan{}, err
	}
	if request.TargetProvider != adapter.provider || request.Workload != adapter.workload || !adapter.runtimeMatch(request.TargetRuntime) || !adapter.distributionMatch(request.Distribution) {
		return sdk.CollectorDeploymentPlan{}, sdk.CollectorCapabilityError{AdapterID: adapter.adapterID, Action: "plan", Reason: fmt.Sprintf("collector path does not support provider %q, runtime %q, or workload %q", request.TargetProvider, request.TargetRuntime, request.Workload)}
	}
	plan := sdk.CollectorDeploymentPlan{
		AdapterID: adapter.adapterID, TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime,
		Workload: request.Workload, Distribution: request.Distribution, CredentialRef: request.CredentialRef,
		Endpoint: request.Endpoint, NativeReference: request.NativeReference, OwnershipMarker: request.OwnershipMarker,
		Signals: append([]string(nil), request.Signals...),
	}
	if err := sdk.ValidateCollectorDeploymentPlan(plan, request); err != nil {
		return sdk.CollectorDeploymentPlan{}, err
	}
	return plan, nil
}

func (adapter *CollectorSDKAdapter) ExecuteCollector(ctx context.Context, request sdk.CollectorDeploymentExecutionRequest) (sdk.CollectorDeploymentExecutionResult, error) {
	if ctx == nil {
		return sdk.CollectorDeploymentExecutionResult{}, errors.New("collector execution context is required")
	}
	if err := ctx.Err(); err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, err
	}
	if err := sdk.ValidateCollectorDeploymentExecutionRequest(request); err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, err
	}
	if err := validateCollectorPlan(request.Plan); err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, err
	}
	if request.Plan.AdapterID != adapter.adapterID || request.Plan.TargetProvider != adapter.provider || request.Plan.Workload != adapter.workload || !adapter.runtimeMatch(request.Plan.TargetRuntime) || !adapter.distributionMatch(request.Plan.Distribution) {
		return sdk.CollectorDeploymentExecutionResult{}, sdk.CollectorCapabilityError{AdapterID: adapter.adapterID, Action: string(request.Action), Reason: "collector plan does not match the injected provider boundary"}
	}
	switch request.Action {
	case sdk.CollectorApply:
		return adapter.apply(ctx, request)
	case sdk.CollectorVerify:
		return adapter.verify(ctx, request)
	case sdk.CollectorRollback:
		return adapter.rollback(ctx, request)
	case sdk.CollectorDestroy:
		return adapter.destroy(ctx, request)
	default:
		return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("unsupported collector action %q", request.Action)
	}
}

func validateCollectorPlan(plan sdk.CollectorDeploymentPlan) error {
	request := sdk.CollectorDeploymentPlanRequest{
		TargetProvider: plan.TargetProvider, TargetRuntime: plan.TargetRuntime, Workload: plan.Workload,
		Distribution: plan.Distribution, CredentialRef: plan.CredentialRef, Endpoint: plan.Endpoint,
		NativeReference: plan.NativeReference, OwnershipMarker: plan.OwnershipMarker, Signals: plan.Signals,
	}
	if err := sdk.ValidateCollectorDeploymentPlanRequest(request); err != nil {
		return fmt.Errorf("validate collector plan: %w", err)
	}
	if err := sdk.ValidateCollectorDeploymentPlan(plan, request); err != nil {
		return fmt.Errorf("validate collector plan references: %w", err)
	}
	return nil
}

func (adapter *CollectorSDKAdapter) apply(ctx context.Context, request sdk.CollectorDeploymentExecutionRequest) (sdk.CollectorDeploymentExecutionResult, error) {
	resource, found, err := adapter.backend.Find(ctx, request.Plan)
	if err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("inspect collector before apply: %w", err)
	}
	if found && (!resource.Owned || resource.OwnershipMarker != request.OwnershipMarker) {
		return sdk.CollectorDeploymentExecutionResult{}, errors.New("refusing to mutate an unowned collector resource")
	}
	resource, err = adapter.backend.Ensure(ctx, request.Plan)
	if err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("ensure collector deployment: %w", err)
	}
	if err := validateCollectorResource(resource, request.Plan); err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, err
	}
	verification, verifyErr := adapter.backend.Verify(ctx, request.Plan, resource)
	if verifyErr != nil || !verification.Ready || !verification.Healthy || !verification.SignalsDelivered {
		rollbackErr := adapter.backend.Rollback(ctx, request.Plan, resource)
		return sdk.CollectorDeploymentExecutionResult{}, errors.Join(
			fmt.Errorf("collector deployment did not prove readiness, health, and signal delivery: %s", verification.Reason),
			verifyErr,
			rollbackErr,
		)
	}
	return collectorResult(request, resource, verification, "apply", true), nil
}

func (adapter *CollectorSDKAdapter) verify(ctx context.Context, request sdk.CollectorDeploymentExecutionRequest) (sdk.CollectorDeploymentExecutionResult, error) {
	resource, found, err := adapter.backend.Find(ctx, request.Plan)
	if err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("inspect collector for verification: %w", err)
	}
	if !found {
		return sdk.CollectorDeploymentExecutionResult{}, errors.New("collector deployment is missing")
	}
	if err := validateCollectorResource(resource, request.Plan); err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, err
	}
	verification, err := adapter.backend.Verify(ctx, request.Plan, resource)
	if err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("verify collector deployment: %w", err)
	}
	if !verification.Ready || !verification.Healthy || !verification.SignalsDelivered {
		return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("collector verification incomplete: %s", verification.Reason)
	}
	return collectorResult(request, resource, verification, "verify", true), nil
}

func (adapter *CollectorSDKAdapter) rollback(ctx context.Context, request sdk.CollectorDeploymentExecutionRequest) (sdk.CollectorDeploymentExecutionResult, error) {
	resource, found, err := adapter.backend.Find(ctx, request.Plan)
	if err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("inspect collector for rollback: %w", err)
	}
	if !found {
		return collectorResult(request, resource, CollectorVerification{}, "rollback", true), nil
	}
	if err := validateCollectorResource(resource, request.Plan); err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, err
	}
	if err := adapter.backend.Rollback(ctx, request.Plan, resource); err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("rollback collector deployment: %w", err)
	}
	remaining, stillFound, err := adapter.backend.Find(ctx, request.Plan)
	if err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("verify collector rollback: %w", err)
	}
	if stillFound || remaining.Identity != "" {
		return sdk.CollectorDeploymentExecutionResult{}, errors.New("collector rollback left an owned resource behind")
	}
	return collectorResult(request, resource, CollectorVerification{}, "rollback", true), nil
}

func (adapter *CollectorSDKAdapter) destroy(ctx context.Context, request sdk.CollectorDeploymentExecutionRequest) (sdk.CollectorDeploymentExecutionResult, error) {
	resources, err := adapter.backend.Inventory(ctx, adapter.provider, request.Plan.TargetRuntime, request.OwnershipMarker)
	if err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("inventory collector resources for cleanup: %w", err)
	}
	refs := make([]string, 0, len(resources))
	for _, resource := range resources {
		if resource.OwnershipMarker != request.OwnershipMarker || !resource.Owned {
			return sdk.CollectorDeploymentExecutionResult{}, errors.New("refusing collector cleanup without exact ownership proof")
		}
		if err := validateCollectorResource(resource, request.Plan); err != nil {
			return sdk.CollectorDeploymentExecutionResult{}, err
		}
		if err := adapter.backend.Destroy(ctx, request.Plan, resource); err != nil {
			return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("destroy collector resource %q: %w", resource.Identity, err)
		}
		refs = append(refs, resource.Identity)
	}
	remaining, err := adapter.backend.Inventory(ctx, adapter.provider, request.Plan.TargetRuntime, request.OwnershipMarker)
	if err != nil {
		return sdk.CollectorDeploymentExecutionResult{}, fmt.Errorf("verify collector cleanup: %w", err)
	}
	if len(remaining) != 0 {
		return sdk.CollectorDeploymentExecutionResult{}, errors.New("owned collector resources remain after cleanup")
	}
	sort.Strings(refs)
	return sdk.CollectorDeploymentExecutionResult{Action: request.Action, OperationID: "collector:destroy:" + collectorMarkerDigest(request.OwnershipMarker), ResourceRefs: refs, ProofRefs: []string{"collector:owning-service-inventory", "collector:ownership-cleanup"}, OwnershipMarker: request.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true, CleanupVerified: true}, nil
}

func validateCollectorResource(resource CollectorResource, plan sdk.CollectorDeploymentPlan) error {
	if strings.TrimSpace(resource.Identity) == "" || !resource.Owned || resource.OwnershipMarker != plan.OwnershipMarker || resource.TargetProvider != plan.TargetProvider || resource.TargetRuntime != plan.TargetRuntime || resource.Workload != plan.Workload || resource.Distribution != plan.Distribution {
		return errors.New("collector resource identity, ownership, or target does not match the plan")
	}
	return nil
}

func collectorResult(request sdk.CollectorDeploymentExecutionRequest, resource CollectorResource, verification CollectorVerification, action string, idempotent bool) sdk.CollectorDeploymentExecutionResult {
	result := sdk.CollectorDeploymentExecutionResult{
		Action: request.Action, OperationID: "collector:" + action + ":" + collectorMarkerDigest(request.OwnershipMarker),
		ProofRefs:       []string{"collector:ownership", "collector:readiness", "collector:health", "collector:signal-delivery"},
		OwnershipMarker: request.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: idempotent,
		ReadyVerified: verification.Ready, HealthVerified: verification.Healthy,
	}
	if strings.TrimSpace(resource.Identity) != "" {
		result.ResourceRefs = []string{resource.Identity}
	}
	if request.Action == sdk.CollectorRollback {
		result.RollbackVerified = true
	}
	return result
}

func collectorMarkerDigest(marker string) string {
	digest := sha256.Sum256([]byte(marker))
	return hex.EncodeToString(digest[:])[:16]
}
