package observability

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// InventoryResource is the provider-neutral view of one telemetry-owned
// resource. Provider adapters translate CloudWatch, Monitoring, Cockpit, or
// Logs Data Platform objects into this shape before it reaches the core.
type InventoryResource struct {
	Identity        string
	OwnershipMarker string
	Owned           bool
	Live            bool
}

// Backend is the only provider-specific seam required by the shared
// observability lifecycle. It deliberately uses the portable Plan and proof
// types so provider SDK response models cannot leak into the core.
type Backend interface {
	Apply(context.Context, Plan) (LifecycleResult, error)
	VerifySignal(context.Context, SignalBinding) (SignalObservation, error)
	VerifyOperations(context.Context, Plan) (OperationalObservation, error)
	Destroy(context.Context, Plan, []string) error
	Inventory(context.Context, string) ([]InventoryResource, error)
}

// SignalProbePublisher is an optional provider capability for acceptance
// harnesses whose destination has asynchronous data-plane activation. It
// republishes a marker-scoped probe without creating or changing lifecycle
// resources. Verification itself remains read-only; callers explicitly opt
// into publication between verification attempts.
type SignalProbePublisher interface {
	PublishSignalProbe(context.Context, SignalBinding) error
}

// ScopedInventoryBackend can provide inventory that depends on the complete
// portable plan. Most provider resources can be inventoried by ownership
// marker alone; some native APIs, such as Google Cloud SLOs, require an
// existing parent resource to enumerate children safely. The optional seam
// keeps that provider constraint out of the shared Backend contract.
type ScopedInventoryBackend interface {
	InventoryForPlan(context.Context, Plan) ([]InventoryResource, error)
}

// ManagedLifecycleClient owns the common cleanup and identity checks for
// provider backends. This is intentionally shared by native cloud and
// external adapters; only the API translator remains provider-specific.
type ManagedLifecycleClient struct {
	backend Backend
}

var _ LifecycleClient = (*ManagedLifecycleClient)(nil)

func NewManagedLifecycleClient(backend Backend) (*ManagedLifecycleClient, error) {
	if backend == nil {
		return nil, errors.New("observability backend is required")
	}
	return &ManagedLifecycleClient{backend: backend}, nil
}

func (client *ManagedLifecycleClient) Apply(ctx context.Context, plan Plan) (LifecycleResult, error) {
	if err := validateLifecycleContext(ctx, plan); err != nil {
		return LifecycleResult{}, err
	}
	result, err := client.backend.Apply(ctx, plan)
	if err != nil {
		return LifecycleResult{}, err
	}
	if err := validateLifecycleResult(result); err != nil {
		return LifecycleResult{}, err
	}
	return result, nil
}

func (client *ManagedLifecycleClient) VerifySignal(ctx context.Context, binding SignalBinding) (SignalObservation, error) {
	if ctx == nil {
		return SignalObservation{}, errors.New("observability signal verification context is required")
	}
	if client == nil || client.backend == nil {
		return SignalObservation{}, errors.New("observability backend is required")
	}
	if err := ctx.Err(); err != nil {
		return SignalObservation{}, err
	}
	if strings.TrimSpace(binding.Signal) == "" || strings.TrimSpace(binding.Destination) == "" {
		return SignalObservation{}, errors.New("observability signal binding identity is required")
	}
	observation, err := client.backend.VerifySignal(ctx, binding)
	if err != nil {
		return SignalObservation{}, err
	}
	if observation.Signal != binding.Signal || observation.Destination != binding.Destination {
		return SignalObservation{}, fmt.Errorf("observability backend returned %s at %s for %s at %s", observation.Signal, observation.Destination, binding.Signal, binding.Destination)
	}
	return observation, nil
}

// PublishSignalProbe republishes one provider-owned data-plane probe when the
// backend supports the optional capability. The shared client validates the
// portable identity boundary; the provider adapter owns the actual API call.
func (client *ManagedLifecycleClient) PublishSignalProbe(ctx context.Context, binding SignalBinding) error {
	if ctx == nil {
		return errors.New("observability signal publication context is required")
	}
	if client == nil || client.backend == nil {
		return errors.New("observability backend is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(binding.Signal) == "" || strings.TrimSpace(binding.Destination) == "" || strings.TrimSpace(binding.OwnershipMarker) == "" {
		return errors.New("observability signal publication identity is required")
	}
	publisher, ok := client.backend.(SignalProbePublisher)
	if !ok {
		return errors.New("observability backend does not support signal probe publication")
	}
	if err := publisher.PublishSignalProbe(ctx, binding); err != nil {
		return fmt.Errorf("publish observability signal probe: %w", err)
	}
	return nil
}

func (client *ManagedLifecycleClient) VerifyOperations(ctx context.Context, plan Plan) (OperationalObservation, error) {
	if err := validateLifecycleContext(ctx, plan); err != nil {
		return OperationalObservation{}, err
	}
	return client.backend.VerifyOperations(ctx, plan)
}

func (client *ManagedLifecycleClient) Destroy(ctx context.Context, plan Plan, resourceReferences []string) (CleanupObservation, error) {
	if err := validateLifecycleContext(ctx, plan); err != nil {
		return CleanupObservation{}, err
	}
	if err := validateReferences(resourceReferences); err != nil {
		return CleanupObservation{}, err
	}
	if err := client.backend.Destroy(ctx, plan, resourceReferences); err != nil {
		return CleanupObservation{}, err
	}
	return client.cleanup(ctx, plan)
}

func (client *ManagedLifecycleClient) VerifyCleanup(ctx context.Context, plan Plan) (CleanupObservation, error) {
	if err := validateLifecycleContext(ctx, plan); err != nil {
		return CleanupObservation{}, err
	}
	return client.cleanup(ctx, plan)
}

func (client *ManagedLifecycleClient) cleanup(ctx context.Context, plan Plan) (CleanupObservation, error) {
	marker := plan.OwnershipMarker
	resources, err := client.inventory(ctx, plan)
	if err != nil {
		return CleanupObservation{}, err
	}
	refs := make([]string, 0, len(resources))
	ownedLive := false
	unownedPreserved := true
	for _, resource := range resources {
		if strings.TrimSpace(resource.Identity) == "" || strings.ContainsAny(resource.Identity, "\r\n\x00") {
			return CleanupObservation{}, errors.New("observability inventory returned an invalid resource identity")
		}
		if resource.OwnershipMarker == marker && !resource.Owned {
			return CleanupObservation{}, errors.New("observability inventory contradicted ownership")
		}
		if resource.Owned && resource.OwnershipMarker != marker {
			return CleanupObservation{}, errors.New("observability inventory returned an ownership mismatch")
		}
		if resource.Live {
			refs = append(refs, resource.Identity)
		}
		if resource.Owned && resource.Live {
			ownedLive = true
		}
		if resource.Owned && resource.OwnershipMarker != marker {
			unownedPreserved = false
		}
	}
	sort.Strings(refs)
	return CleanupObservation{
		Complete:         !ownedLive,
		UnownedPreserved: unownedPreserved,
		ResourceRefs:     refs,
		Reason:           cleanupReason(ownedLive, unownedPreserved),
	}, nil
}

func (client *ManagedLifecycleClient) inventory(ctx context.Context, plan Plan) ([]InventoryResource, error) {
	if scoped, ok := client.backend.(ScopedInventoryBackend); ok {
		return scoped.InventoryForPlan(ctx, plan)
	}
	return client.backend.Inventory(ctx, plan.OwnershipMarker)
}

func validateLifecycleContext(ctx context.Context, plan Plan) error {
	if ctx == nil {
		return errors.New("observability lifecycle context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(plan.OwnershipMarker) == "" || strings.ContainsAny(plan.OwnershipMarker, "\r\n\x00") {
		return errors.New("observability lifecycle ownership marker is required")
	}
	return nil
}

func validateLifecycleResult(result LifecycleResult) error {
	if strings.TrimSpace(result.OperationID) == "" || strings.ContainsAny(result.OperationID, "\r\n\x00") {
		return errors.New("observability lifecycle operation identity is required")
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified {
		return errors.New("observability lifecycle result must verify ownership and idempotency")
	}
	return validateReferences(append(append([]string(nil), result.ResourceRefs...), result.ProofRefs...))
}

func validateReferences(values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return errors.New("observability lifecycle resource references must be non-empty and single-line")
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate observability lifecycle resource reference %q", value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func cleanupReason(ownedLive, unownedPreserved bool) string {
	switch {
	case ownedLive:
		return "owned observability resources remain"
	case !unownedPreserved:
		return "unowned observability resources were not preserved"
	default:
		return ""
	}
}
