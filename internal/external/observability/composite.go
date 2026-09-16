package observability

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/magelift/magelift/sdk"
)

// CompositeLifecycleClient fans one portable plan out to destination-owned
// lifecycle clients. The core still owns the plan and evidence contract; a
// destination client only receives bindings for its own provider identity.
// operationOwner is the destination that owns alerts, dashboards, and SLOs,
// because those objects are intentionally not duplicated across destinations.
type CompositeLifecycleClient struct {
	clients        map[string]LifecycleClient
	operationOwner string
}

type destinationLifecycleResult struct {
	destination string
	result      LifecycleResult
}

// NewCompositeLifecycleClient creates a composed lifecycle client for native,
// external, or combined observability. Empty destinations and duplicate
// clients are rejected before any provider SDK can be called.
func NewCompositeLifecycleClient(clients map[string]LifecycleClient, operationOwner string) (*CompositeLifecycleClient, error) {
	if len(clients) == 0 {
		return nil, errors.New("observability composite requires at least one destination client")
	}
	copyClients := make(map[string]LifecycleClient, len(clients))
	for destination, client := range clients {
		destination = strings.TrimSpace(destination)
		if destination == "" {
			return nil, errors.New("observability composite destination is required")
		}
		if isNilLifecycleClient(client) {
			return nil, fmt.Errorf("observability composite client for %q is required", destination)
		}
		if _, exists := copyClients[destination]; exists {
			return nil, fmt.Errorf("observability composite destination %q is duplicated", destination)
		}
		copyClients[destination] = client
	}
	operationOwner = strings.TrimSpace(operationOwner)
	if operationOwner == "" {
		owners := make([]string, 0, len(copyClients))
		for destination := range copyClients {
			owners = append(owners, destination)
		}
		sort.Strings(owners)
		operationOwner = owners[0]
	}
	if _, ok := copyClients[operationOwner]; !ok {
		return nil, fmt.Errorf("observability composite operation owner %q has no client", operationOwner)
	}
	return &CompositeLifecycleClient{clients: copyClients, operationOwner: operationOwner}, nil
}

// NewCompositeSDKAdapter combines destination lifecycle clients behind the
// existing public SDK adapter. Provider packages can use this for a native
// plus New Relic plan without duplicating the portable planner.
func NewCompositeSDKAdapter(provider sdk.ProviderID, clients map[string]LifecycleClient, operationOwner string) (sdk.ObservabilityAdapter, error) {
	composite, err := NewCompositeLifecycleClient(clients, operationOwner)
	if err != nil {
		return nil, err
	}
	return NewSDKAdapter(provider, New(), composite), nil
}

var _ LifecycleClient = (*CompositeLifecycleClient)(nil)

func (client *CompositeLifecycleClient) Apply(ctx context.Context, plan Plan) (LifecycleResult, error) {
	if err := validateLifecycleContext(ctx, plan); err != nil {
		return LifecycleResult{}, err
	}
	results, err := client.forEachDestination(ctx, plan, func(destination string, child LifecycleClient, childPlan Plan) (LifecycleResult, error) {
		if len(childPlan.Bindings) == 0 && !ownsOperations(destination, client.operationOwner, childPlan) {
			return LifecycleResult{}, nil
		}
		return child.Apply(ctx, childPlan)
	})
	if err != nil {
		return LifecycleResult{}, err
	}
	return combineLifecycleResults("composite:observability:apply:"+markerDigest(plan.OwnershipMarker), results), nil
}

func (client *CompositeLifecycleClient) VerifySignal(ctx context.Context, binding SignalBinding) (SignalObservation, error) {
	if ctx == nil {
		return SignalObservation{}, errors.New("observability composite signal context is required")
	}
	child, ok := client.clients[strings.TrimSpace(binding.Destination)]
	if !ok {
		return SignalObservation{}, fmt.Errorf("observability composite has no client for destination %q", binding.Destination)
	}
	return child.VerifySignal(ctx, binding)
}

func (client *CompositeLifecycleClient) VerifyOperations(ctx context.Context, plan Plan) (OperationalObservation, error) {
	if err := validateLifecycleContext(ctx, plan); err != nil {
		return OperationalObservation{}, err
	}
	owner := client.clients[client.operationOwner]
	return owner.VerifyOperations(ctx, client.childPlanForDestination(plan, client.operationOwner, nil))
}

func (client *CompositeLifecycleClient) Destroy(ctx context.Context, plan Plan, resourceReferences []string) (CleanupObservation, error) {
	if err := validateLifecycleContext(ctx, plan); err != nil {
		return CleanupObservation{}, err
	}
	if err := validateReferences(resourceReferences); err != nil {
		return CleanupObservation{}, err
	}
	routedReferences, err := routeCompositeReferences(resourceReferences)
	if err != nil {
		return CleanupObservation{}, err
	}
	for destination := range routedReferences {
		if _, ok := client.clients[destination]; !ok {
			return CleanupObservation{}, fmt.Errorf("observability composite has no client for destination %q", destination)
		}
	}
	destinations := client.destinations()
	for _, destination := range destinations {
		childPlan := client.childPlanForDestination(plan, destination, nil)
		childReferences := routedReferences[destination]
		if len(childPlan.Bindings) == 0 && !ownsOperations(destination, client.operationOwner, childPlan) && len(childReferences) == 0 {
			continue
		}
		if _, err := client.clients[destination].Destroy(ctx, childPlan, childReferences); err != nil {
			return CleanupObservation{}, fmt.Errorf("observability destination %q: %w", destination, err)
		}
	}
	return client.VerifyCleanup(ctx, plan)
}

func (client *CompositeLifecycleClient) VerifyCleanup(ctx context.Context, plan Plan) (CleanupObservation, error) {
	if err := validateLifecycleContext(ctx, plan); err != nil {
		return CleanupObservation{}, err
	}
	results, err := client.forEachDestination(ctx, plan, func(destination string, child LifecycleClient, childPlan Plan) (LifecycleResult, error) {
		cleanup, err := child.VerifyCleanup(ctx, childPlan)
		if err != nil {
			return LifecycleResult{}, err
		}
		return LifecycleResult{
			OperationID:         "cleanup",
			ResourceRefs:        wrapReferences(destination, cleanup.ResourceRefs),
			OwnershipVerified:   cleanup.UnownedPreserved,
			IdempotencyVerified: cleanup.Complete,
		}, nil
	})
	if err != nil {
		return CleanupObservation{}, err
	}
	cleanup := CleanupObservation{Complete: true, UnownedPreserved: true}
	for _, result := range results {
		cleanup.Complete = cleanup.Complete && result.result.IdempotencyVerified
		cleanup.UnownedPreserved = cleanup.UnownedPreserved && result.result.OwnershipVerified
		cleanup.ResourceRefs = append(cleanup.ResourceRefs, result.result.ResourceRefs...)
	}
	sort.Strings(cleanup.ResourceRefs)
	if !cleanup.Complete {
		cleanup.Reason = "owned observability resources remain"
	} else if !cleanup.UnownedPreserved {
		cleanup.Reason = "unowned observability resources were not preserved"
	}
	return cleanup, nil
}

func (client *CompositeLifecycleClient) forEachDestination(ctx context.Context, plan Plan, run func(string, LifecycleClient, Plan) (LifecycleResult, error)) ([]destinationLifecycleResult, error) {
	destinations := client.destinations()
	bindings := make(map[string][]SignalBinding, len(destinations))
	for _, binding := range plan.Bindings {
		destination := strings.TrimSpace(binding.Destination)
		if _, ok := client.clients[destination]; !ok {
			return nil, fmt.Errorf("observability composite has no client for destination %q", binding.Destination)
		}
		bindings[destination] = append(bindings[destination], binding)
	}
	results := make([]destinationLifecycleResult, 0, len(destinations))
	for _, destination := range destinations {
		childPlan := client.childPlanForDestination(plan, destination, bindings[destination])
		result, err := run(destination, client.clients[destination], childPlan)
		if err != nil {
			return nil, fmt.Errorf("observability destination %q: %w", destination, err)
		}
		if strings.TrimSpace(result.OperationID) != "" {
			results = append(results, destinationLifecycleResult{destination: destination, result: result})
		}
	}
	return results, nil
}

func (client *CompositeLifecycleClient) destinations() []string {
	destinations := make([]string, 0, len(client.clients))
	for destination := range client.clients {
		destinations = append(destinations, destination)
	}
	sort.Strings(destinations)
	return destinations
}

func (client *CompositeLifecycleClient) childPlanForDestination(plan Plan, destination string, bindings []SignalBinding) Plan {
	if bindings == nil {
		bindings = make([]SignalBinding, 0, len(plan.Bindings))
		for _, binding := range plan.Bindings {
			if strings.TrimSpace(binding.Destination) == destination {
				bindings = append(bindings, binding)
			}
		}
	}
	childPlan := plan
	childPlan.Bindings = append([]SignalBinding(nil), bindings...)
	if destination != client.operationOwner {
		childPlan.Alerts = nil
		childPlan.Dashboards = nil
		childPlan.SLOs = nil
		childPlan.UnavailableOperations = nil
	}
	return childPlan
}

func ownsOperations(destination, operationOwner string, plan Plan) bool {
	return destination == operationOwner && (len(plan.Alerts) > 0 || len(plan.Dashboards) > 0 || len(plan.SLOs) > 0)
}

func combineLifecycleResults(operationID string, results []destinationLifecycleResult) LifecycleResult {
	result := LifecycleResult{OperationID: operationID, OwnershipVerified: true, IdempotencyVerified: true}
	seenRefs := make(map[string]struct{})
	seenProofs := make(map[string]struct{})
	for _, child := range results {
		result.OwnershipVerified = result.OwnershipVerified && child.result.OwnershipVerified
		result.IdempotencyVerified = result.IdempotencyVerified && child.result.IdempotencyVerified
		for _, ref := range wrapReferences(child.destination, child.result.ResourceRefs) {
			if _, ok := seenRefs[ref]; !ok {
				seenRefs[ref] = struct{}{}
				result.ResourceRefs = append(result.ResourceRefs, ref)
			}
		}
		for _, ref := range child.result.ProofRefs {
			if _, ok := seenProofs[ref]; !ok {
				seenProofs[ref] = struct{}{}
				result.ProofRefs = append(result.ProofRefs, ref)
			}
		}
	}
	sort.Strings(result.ResourceRefs)
	sort.Strings(result.ProofRefs)
	return result
}

func markerDigest(marker string) string {
	// The destination adapters already derive their own opaque identities. The
	// composite operation ID only needs a stable, non-secret scope component.
	digest := sha256.Sum256([]byte(marker))
	return hex.EncodeToString(digest[:8])
}

const compositeReferencePrefix = "composite:"

func wrapReferences(destination string, references []string) []string {
	if len(references) == 0 {
		return nil
	}
	wrapped := make([]string, 0, len(references))
	for _, reference := range references {
		wrapped = append(wrapped, wrapReference(destination, reference))
	}
	return wrapped
}

func wrapReference(destination, reference string) string {
	return compositeReferencePrefix + base64.RawURLEncoding.EncodeToString([]byte(destination)) + ":" + base64.RawURLEncoding.EncodeToString([]byte(reference))
}

func routeCompositeReferences(references []string) (map[string][]string, error) {
	routed := make(map[string][]string)
	for _, reference := range references {
		destination, inner, err := unwrapReference(reference)
		if err != nil {
			return nil, err
		}
		routed[destination] = append(routed[destination], inner)
	}
	return routed, nil
}

func unwrapReference(reference string) (string, string, error) {
	if !strings.HasPrefix(reference, compositeReferencePrefix) {
		return "", "", fmt.Errorf("observability composite received an unscoped resource reference %q", reference)
	}
	value := strings.TrimPrefix(reference, compositeReferencePrefix)
	encodedDestination, encodedReference, ok := strings.Cut(value, ":")
	if !ok || encodedDestination == "" || encodedReference == "" {
		return "", "", fmt.Errorf("observability composite received an invalid resource reference %q", reference)
	}
	destinationBytes, err := base64.RawURLEncoding.DecodeString(encodedDestination)
	if err != nil || len(destinationBytes) == 0 {
		return "", "", fmt.Errorf("observability composite received an invalid destination reference %q", reference)
	}
	resourceBytes, err := base64.RawURLEncoding.DecodeString(encodedReference)
	if err != nil || len(resourceBytes) == 0 {
		return "", "", fmt.Errorf("observability composite received an invalid child resource reference %q", reference)
	}
	destination := string(destinationBytes)
	resource := string(resourceBytes)
	if strings.TrimSpace(destination) == "" || strings.ContainsAny(destination+resource, "\r\n\x00") {
		return "", "", fmt.Errorf("observability composite received an unsafe resource reference %q", reference)
	}
	return destination, resource, nil
}

func isNilLifecycleClient(client LifecycleClient) bool {
	if client == nil {
		return true
	}
	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
