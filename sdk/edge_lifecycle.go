package sdk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// EdgeOperationStatus is the provider-neutral state of one asynchronous edge
// operation. Provider adapters translate CloudFront, Google Cloud, Scaleway,
// OVHcloud, or community API states before they cross this port.
type EdgeOperationStatus string

const (
	EdgeOperationPending   EdgeOperationStatus = "pending"
	EdgeOperationRunning   EdgeOperationStatus = "running"
	EdgeOperationSucceeded EdgeOperationStatus = "succeeded"
	EdgeOperationFailed    EdgeOperationStatus = "failed"
)

// EdgeOperationRequest is the only operation input accepted by the shared
// edge lifecycle. The provider implementation owns the translation from the
// portable execution request to its SDK request model.
type EdgeOperationRequest struct {
	Provider ProviderID
	Action   EdgeAction
	Request  EdgeExecutionRequest
}

// EdgeOperationObservation contains only opaque identities and proof flags.
// Provider SDK response types and resolved credentials remain inside the
// provider implementation.
type EdgeOperationObservation struct {
	Status              EdgeOperationStatus
	Action              EdgeAction
	OperationID         string
	ResourceRefs        []string
	ProofRefs           []string
	Outputs             []AdapterOutput
	OwnershipMarker     string
	OwnershipVerified   bool
	IdempotencyVerified bool
	Detail              string
}

// EdgeInventoryResource is the provider-neutral view of one owning-service
// edge resource. Cleanup may remove a resource only when Owned is true and
// its marker matches the requested ownership scope.
type EdgeInventoryResource struct {
	Identity        string
	OwnershipMarker string
	Owned           bool
	Live            bool
}

// EdgeOperationAPI is implemented by a first-party or community provider
// package around its official SDK/API. It is intentionally narrow: the core
// owns lifecycle ordering, polling, validation, and cleanup safety while the
// implementation owns native request and response models.
type EdgeOperationAPI interface {
	Plan(context.Context, EdgePlanRequest) (EdgePlan, error)
	Start(context.Context, EdgeOperationRequest) (EdgeOperationObservation, error)
	Poll(context.Context, string) (EdgeOperationObservation, error)
	Inventory(context.Context, string) ([]EdgeInventoryResource, error)
}

// EdgeOperationPolicy bounds provider waits. A zero value is invalid so an
// edge control-plane wait cannot accidentally become unbounded.
type EdgeOperationPolicy struct {
	Timeout      time.Duration
	PollInterval time.Duration
	MaxAttempts  int
}

// DefaultEdgeOperationPolicy is suitable for managed edge control planes.
// Tests should pass a short explicit policy.
func DefaultEdgeOperationPolicy() EdgeOperationPolicy {
	return EdgeOperationPolicy{Timeout: 30 * time.Minute, PollInterval: 5 * time.Second, MaxAttempts: 360}
}

// OperationBackedEdgeAdapter is the reusable SDK implementation for native
// edge adapters. Provider packages supply only the descriptor and an API
// translator; planning, polling, ownership, idempotency, and cancellation
// stay identical across providers.
type OperationBackedEdgeAdapter struct {
	descriptor EdgeAdapterDescriptor
	api        EdgeOperationAPI
	policy     EdgeOperationPolicy
}

var _ EdgeAdapter = (*OperationBackedEdgeAdapter)(nil)

// NewOperationBackedEdgeAdapter validates the descriptor and operation port
// before any provider API call can occur.
func NewOperationBackedEdgeAdapter(descriptor EdgeAdapterDescriptor, api EdgeOperationAPI, policy EdgeOperationPolicy) (*OperationBackedEdgeAdapter, error) {
	if err := ValidateEdgeAdapterDescriptor(descriptor); err != nil {
		return nil, fmt.Errorf("validate operation-backed edge adapter: %w", err)
	}
	if api == nil {
		return nil, errors.New("operation-backed edge API is required")
	}
	if err := validateEdgeOperationPolicy(policy); err != nil {
		return nil, err
	}
	return &OperationBackedEdgeAdapter{descriptor: descriptor, api: api, policy: policy}, nil
}

func (adapter *OperationBackedEdgeAdapter) EdgeDescriptor() EdgeAdapterDescriptor {
	if adapter == nil {
		return EdgeAdapterDescriptor{}
	}
	return adapter.descriptor
}

func (adapter *OperationBackedEdgeAdapter) PlanEdge(ctx context.Context, request EdgePlanRequest) (EdgePlan, error) {
	if adapter == nil || adapter.api == nil {
		return EdgePlan{}, errors.New("operation-backed edge adapter is required")
	}
	if ctx == nil {
		return EdgePlan{}, errors.New("edge planning context is required")
	}
	if err := ctx.Err(); err != nil {
		return EdgePlan{}, err
	}
	if err := ValidateEdgePlanRequest(request); err != nil {
		return EdgePlan{}, err
	}
	plan, err := adapter.api.Plan(ctx, request)
	if err != nil {
		return EdgePlan{}, fmt.Errorf("plan provider edge: %w", err)
	}
	if plan.AdapterID != adapter.descriptor.ID {
		return EdgePlan{}, fmt.Errorf("provider edge plan adapter ID %q does not match %q", plan.AdapterID, adapter.descriptor.ID)
	}
	if err := ValidateEdgePlan(plan, request); err != nil {
		return EdgePlan{}, fmt.Errorf("validate provider edge plan: %w", err)
	}
	return plan, nil
}

func (adapter *OperationBackedEdgeAdapter) ExecuteEdge(ctx context.Context, request EdgeExecutionRequest) (EdgeExecutionResult, error) {
	if adapter == nil || adapter.api == nil {
		return EdgeExecutionResult{}, errors.New("operation-backed edge adapter is required")
	}
	if ctx == nil {
		return EdgeExecutionResult{}, errors.New("edge execution context is required")
	}
	if err := ValidateEdgeExecutionRequest(request); err != nil {
		return EdgeExecutionResult{}, err
	}
	if !containsEdgeAction(adapter.descriptor.Capabilities, request.Action) {
		return EdgeExecutionResult{}, EdgeCapabilityError{AdapterID: adapter.descriptor.ID, Action: request.Action, Status: EdgeCapabilityUnsupported, Reason: "the adapter descriptor does not declare this lifecycle action"}
	}
	observation, err := adapter.api.Start(ctx, EdgeOperationRequest{Provider: adapter.descriptor.Provider, Action: request.Action, Request: cloneEdgeExecutionRequest(request)})
	if err != nil {
		return EdgeExecutionResult{}, fmt.Errorf("start edge operation %q: %w", request.Action, err)
	}
	if observation.Status == EdgeOperationPending || observation.Status == EdgeOperationRunning {
		if strings.TrimSpace(observation.OperationID) == "" {
			return EdgeExecutionResult{}, errors.New("pending edge operation returned no operation identity")
		}
		observation, err = WaitForEdgeOperation(ctx, adapter.api, observation.OperationID, adapter.policy)
		if err != nil {
			return EdgeExecutionResult{}, err
		}
	}
	if observation.Status != EdgeOperationSucceeded {
		if observation.Status == EdgeOperationFailed && strings.TrimSpace(observation.Detail) == "" {
			observation.Detail = "provider reported edge operation failure"
		}
		return EdgeExecutionResult{}, fmt.Errorf("edge operation returned non-success status %q: %s", observation.Status, observation.Detail)
	}
	if observation.Action != "" && observation.Action != request.Action {
		return EdgeExecutionResult{}, fmt.Errorf("edge operation returned action %q, want %q", observation.Action, request.Action)
	}
	if observation.OwnershipMarker != request.OwnershipMarker || !observation.OwnershipVerified {
		return EdgeExecutionResult{}, errors.New("edge operation did not verify ownership scope")
	}
	if !observation.IdempotencyVerified {
		return EdgeExecutionResult{}, errors.New("edge operation did not verify idempotency")
	}
	if strings.TrimSpace(observation.OperationID) == "" && len(observation.ResourceRefs) == 0 && len(observation.ProofRefs) == 0 {
		return EdgeExecutionResult{}, errors.New("successful edge operation returned no operation, resource, or proof identity")
	}
	result := EdgeExecutionResult{
		Action:              request.Action,
		OperationID:         observation.OperationID,
		ResourceRefs:        append([]string(nil), observation.ResourceRefs...),
		ProofRefs:           append([]string(nil), observation.ProofRefs...),
		Outputs:             append([]AdapterOutput(nil), observation.Outputs...),
		OwnershipMarker:     observation.OwnershipMarker,
		OwnershipVerified:   observation.OwnershipVerified,
		IdempotencyVerified: observation.IdempotencyVerified,
	}
	if err := ValidateEdgeExecutionResult(request, result); err != nil {
		return EdgeExecutionResult{}, fmt.Errorf("validate edge operation result: %w", err)
	}
	if request.Action == EdgeDestroy {
		if _, err := WaitForEdgeCleanup(ctx, adapter.api, request.OwnershipMarker, adapter.policy); err != nil {
			return EdgeExecutionResult{}, err
		}
	}
	return result, nil
}

// Inventory reads the provider's owning-service inventory without allowing
// the core to infer deletion from a successful delete request.
func (adapter *OperationBackedEdgeAdapter) Inventory(ctx context.Context, marker string) ([]EdgeInventoryResource, error) {
	if adapter == nil || adapter.api == nil {
		return nil, errors.New("operation-backed edge adapter is required")
	}
	if ctx == nil {
		return nil, errors.New("edge inventory context is required")
	}
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("edge inventory ownership marker is required and must be single-line")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resources, err := adapter.api.Inventory(ctx, marker)
	if err != nil {
		return nil, fmt.Errorf("inventory provider edge resources: %w", err)
	}
	if err := validateEdgeInventory(resources, marker); err != nil {
		return nil, err
	}
	return resources, nil
}

// WaitForEdgeCleanup polls the owning-service inventory after a destroy
// operation. A provider's successful delete response is not cleanup proof
// while an owned resource remains live; unowned resources are preserved.
func WaitForEdgeCleanup(ctx context.Context, api EdgeOperationAPI, marker string, policy EdgeOperationPolicy) ([]EdgeInventoryResource, error) {
	if ctx == nil {
		return nil, errors.New("edge cleanup context is required")
	}
	if api == nil {
		return nil, errors.New("edge cleanup API is required")
	}
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("edge cleanup ownership marker is required and must be single-line")
	}
	if err := validateEdgeOperationPolicy(policy); err != nil {
		return nil, err
	}
	cleanupCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
	defer cancel()
	var lastResources []EdgeInventoryResource
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := cleanupCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("edge cleanup for %q timed out after %s", marker, policy.Timeout)
			}
			return nil, err
		}
		resources, err := api.Inventory(cleanupCtx, marker)
		if err == nil {
			if err := validateEdgeInventory(resources, marker); err != nil {
				return nil, err
			}
			lastResources = resources
			ownedLive := false
			for _, resource := range resources {
				ownedLive = ownedLive || (resource.Owned && resource.Live)
			}
			if !ownedLive {
				return resources, nil
			}
		} else if attempt == policy.MaxAttempts {
			return nil, fmt.Errorf("inventory edge cleanup for %q after %d attempts: %w", marker, attempt, err)
		}
		if attempt == policy.MaxAttempts {
			return lastResources, fmt.Errorf("owned edge resources remain after %d cleanup attempts", attempt)
		}
		timer := time.NewTimer(policy.PollInterval)
		select {
		case <-cleanupCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			if errors.Is(cleanupCtx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("edge cleanup for %q timed out after %s", marker, policy.Timeout)
			}
			return nil, cleanupCtx.Err()
		case <-timer.C:
		}
	}
	return lastResources, fmt.Errorf("edge cleanup for %q did not complete", marker)
}

// WaitForEdgeOperation polls one owning-provider operation with cancellation,
// timeout, and attempt limits. Unknown or unfinished states never pass.
func WaitForEdgeOperation(ctx context.Context, api EdgeOperationAPI, operationID string, policy EdgeOperationPolicy) (EdgeOperationObservation, error) {
	if ctx == nil {
		return EdgeOperationObservation{}, errors.New("edge operation context is required")
	}
	if api == nil {
		return EdgeOperationObservation{}, errors.New("edge operation API is required")
	}
	if strings.TrimSpace(operationID) == "" || strings.ContainsAny(operationID, "\r\n\x00") {
		return EdgeOperationObservation{}, errors.New("edge operation ID is required and must be single-line")
	}
	if err := validateEdgeOperationPolicy(policy); err != nil {
		return EdgeOperationObservation{}, err
	}
	operationCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
	defer cancel()
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := operationCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return EdgeOperationObservation{}, fmt.Errorf("edge operation %q timed out after %s", operationID, policy.Timeout)
			}
			return EdgeOperationObservation{}, err
		}
		observation, err := api.Poll(operationCtx, operationID)
		if err == nil {
			if observation.OperationID != operationID {
				return EdgeOperationObservation{}, fmt.Errorf("edge operation poll returned identity %q, want %q", observation.OperationID, operationID)
			}
			switch observation.Status {
			case EdgeOperationSucceeded:
				return observation, nil
			case EdgeOperationFailed:
				if strings.TrimSpace(observation.Detail) == "" {
					observation.Detail = "provider reported edge operation failure"
				}
				return observation, fmt.Errorf("edge operation %q failed: %s", operationID, observation.Detail)
			case EdgeOperationPending, EdgeOperationRunning:
			default:
				return EdgeOperationObservation{}, fmt.Errorf("edge operation %q returned unknown status %q", operationID, observation.Status)
			}
		} else if attempt == policy.MaxAttempts {
			return EdgeOperationObservation{}, fmt.Errorf("poll edge operation %q after %d attempts: %w", operationID, attempt, err)
		}
		if attempt == policy.MaxAttempts {
			return EdgeOperationObservation{}, fmt.Errorf("edge operation %q did not complete after %d attempts", operationID, attempt)
		}
		timer := time.NewTimer(policy.PollInterval)
		select {
		case <-operationCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			if errors.Is(operationCtx.Err(), context.DeadlineExceeded) {
				return EdgeOperationObservation{}, fmt.Errorf("edge operation %q timed out after %s", operationID, policy.Timeout)
			}
			return EdgeOperationObservation{}, operationCtx.Err()
		case <-timer.C:
		}
	}
	return EdgeOperationObservation{}, fmt.Errorf("edge operation %q did not complete", operationID)
}

func validateEdgeOperationPolicy(policy EdgeOperationPolicy) error {
	if policy.Timeout <= 0 || policy.PollInterval <= 0 || policy.MaxAttempts <= 0 {
		return errors.New("edge operation policy requires positive timeout, poll interval, and max attempts")
	}
	return nil
}

func cloneEdgeExecutionRequest(request EdgeExecutionRequest) EdgeExecutionRequest {
	clone := request
	clone.ResourceReferences = append([]string(nil), request.ResourceReferences...)
	clone.Plan.Outputs = append([]AdapterOutput(nil), request.Plan.Outputs...)
	return clone
}

func containsEdgeAction(actions []EdgeAction, candidate EdgeAction) bool {
	for _, action := range actions {
		if action == candidate {
			return true
		}
	}
	return false
}

func validateEdgeInventory(resources []EdgeInventoryResource, marker string) error {
	seen := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		if strings.TrimSpace(resource.Identity) == "" || strings.ContainsAny(resource.Identity, "\r\n\x00") {
			return errors.New("edge inventory returned an invalid resource identity")
		}
		if _, exists := seen[resource.Identity]; exists {
			return fmt.Errorf("edge inventory returned duplicate resource %q", resource.Identity)
		}
		seen[resource.Identity] = struct{}{}
		if resource.Owned && resource.OwnershipMarker != marker {
			return errors.New("edge inventory returned an ownership mismatch")
		}
		if resource.OwnershipMarker == marker && !resource.Owned {
			return errors.New("edge inventory contradicted ownership")
		}
	}
	return nil
}
