// Package edge contains provider-neutral composition for edge lifecycle
// adapters. Native and external edge implementations remain SDK clients; this
// package only owns intent splitting, opaque-reference routing, and result
// validation once.
package edge

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

// CompositeAdapter fans a portable edge intent out to the selected native and
// external destinations. A child adapter receives a copy of the intent with
// the other destination removed, so provider implementations never need to
// understand composition or route another provider's resource references.
type CompositeAdapter struct {
	provider sdk.ProviderID
	clients  map[string]sdk.EdgeAdapter
}

type compositePlan struct {
	children           map[string]sdk.EdgePlan
	adapterID          string
	targetProvider     sdk.ProviderID
	targetRuntime      sdk.RuntimeID
	ownershipMarker    string
	childRoutingDigest string
}

// NewCompositeAdapter constructs a provider-neutral edge composition. The map
// keys are destination IDs such as cloudfront, google-cloud-load-balancing,
// or fastly; the values remain provider-owned SDK adapters.
func NewCompositeAdapter(provider sdk.ProviderID, clients map[string]sdk.EdgeAdapter) (sdk.EdgeAdapter, error) {
	if strings.TrimSpace(string(provider)) == "" {
		return nil, errors.New("composite edge provider is required")
	}
	if len(clients) == 0 {
		return nil, errors.New("composite edge requires at least one destination adapter")
	}
	copyClients := make(map[string]sdk.EdgeAdapter, len(clients))
	for destination, client := range clients {
		destination = strings.TrimSpace(destination)
		if destination == "" {
			return nil, errors.New("composite edge destination is required")
		}
		if isNilAdapter(client) {
			return nil, fmt.Errorf("composite edge adapter for %q is required", destination)
		}
		if err := sdk.ValidateEdgeAdapterDescriptor(client.EdgeDescriptor()); err != nil {
			return nil, fmt.Errorf("validate composite edge child %q: %w", destination, err)
		}
		for _, required := range []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy} {
			if !containsAction(client.EdgeDescriptor().Capabilities, required) {
				return nil, fmt.Errorf("composite edge child %q does not support %q", destination, required)
			}
		}
		if _, exists := copyClients[destination]; exists {
			return nil, fmt.Errorf("composite edge destination %q is duplicated", destination)
		}
		copyClients[destination] = client
	}
	return &CompositeAdapter{provider: provider, clients: copyClients}, nil
}

// ValidateAdapterProvider checks a provider-owned SDK adapter before it is
// composed or registered. It keeps descriptor and target identity validation at
// the shared seam so every first-party and community bridge behaves the same.
func ValidateAdapterProvider(adapter sdk.EdgeAdapter, expected sdk.ProviderID) error {
	if isNilAdapter(adapter) {
		return errors.New("edge SDK adapter is required")
	}
	if err := sdk.ValidateEdgeAdapterDescriptor(adapter.EdgeDescriptor()); err != nil {
		return fmt.Errorf("validate edge SDK adapter: %w", err)
	}
	if expected != "" && adapter.EdgeDescriptor().Provider != expected {
		return fmt.Errorf("edge SDK adapter provider %q does not match %q", adapter.EdgeDescriptor().Provider, expected)
	}
	return nil
}

var _ sdk.EdgeAdapter = (*CompositeAdapter)(nil)

func (adapter *CompositeAdapter) EdgeDescriptor() sdk.EdgeAdapterDescriptor {
	if adapter == nil {
		return sdk.EdgeAdapterDescriptor{}
	}
	return sdk.EdgeAdapterDescriptor{
		APIVersion:   sdk.ExtensionAPIVersion,
		ID:           string(adapter.provider) + ".edge.composite",
		Provider:     adapter.provider,
		Version:      "1.0.0",
		Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy},
	}
}

func (adapter *CompositeAdapter) PlanEdge(ctx context.Context, request sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	if adapter == nil {
		return sdk.EdgePlan{}, errors.New("composite edge adapter is required")
	}
	if ctx == nil {
		return sdk.EdgePlan{}, errors.New("composite edge planning context is required")
	}
	if err := ctx.Err(); err != nil {
		return sdk.EdgePlan{}, err
	}
	if request.TargetProvider != adapter.provider {
		return sdk.EdgePlan{}, fmt.Errorf("composite edge provider %q does not match target provider %q", adapter.provider, request.TargetProvider)
	}
	if err := sdk.ValidateEdgePlanRequest(request); err != nil {
		return sdk.EdgePlan{}, err
	}
	children := make(map[string]sdk.EdgePlan)
	for _, destination := range adapter.destinations() {
		childRequest, selected := childPlanRequest(request, destination)
		if !selected {
			continue
		}
		child, ok := adapter.clients[destination]
		if !ok {
			return sdk.EdgePlan{}, fmt.Errorf("composite edge has no adapter for destination %q", destination)
		}
		childPlan, err := child.PlanEdge(ctx, childRequest)
		if err != nil {
			return sdk.EdgePlan{}, fmt.Errorf("plan edge destination %q: %w", destination, err)
		}
		if childPlan.TargetProvider != request.TargetProvider || childPlan.TargetRuntime != request.TargetRuntime {
			return sdk.EdgePlan{}, fmt.Errorf("edge destination %q returned a mismatched target", destination)
		}
		if err := sdk.ValidateEdgePlan(childPlan, childRequest); err != nil {
			return sdk.EdgePlan{}, fmt.Errorf("validate edge destination %q plan: %w", destination, err)
		}
		if childPlan.OwnershipMarker != request.Intent.OwnershipMarker {
			return sdk.EdgePlan{}, fmt.Errorf("edge destination %q returned a mismatched ownership marker", destination)
		}
		children[destination] = childPlan
	}
	outputs := make([]sdk.AdapterOutput, 0)
	for _, destination := range adapter.destinations() {
		childPlan, ok := children[destination]
		if !ok {
			continue
		}
		for _, output := range childPlan.Outputs {
			outputs = append(outputs, sdk.AdapterOutput{Key: destination + "." + output.Key, Value: output.Value})
		}
	}
	plan := sdk.EdgePlan{
		AdapterID:       adapter.EdgeDescriptor().ID,
		TargetProvider:  request.TargetProvider,
		TargetRuntime:   request.TargetRuntime,
		OwnershipMarker: request.Intent.OwnershipMarker,
		Outputs:         outputs,
		Opaque: compositePlan{
			children:           children,
			adapterID:          adapter.EdgeDescriptor().ID,
			targetProvider:     request.TargetProvider,
			targetRuntime:      request.TargetRuntime,
			ownershipMarker:    request.Intent.OwnershipMarker,
			childRoutingDigest: compositeChildRoutingDigest(children),
		},
	}
	if err := sdk.ValidateEdgePlan(plan, request); err != nil {
		return sdk.EdgePlan{}, fmt.Errorf("validate composite edge plan: %w", err)
	}
	return plan, nil
}

func (adapter *CompositeAdapter) ExecuteEdge(ctx context.Context, request sdk.EdgeExecutionRequest) (sdk.EdgeExecutionResult, error) {
	if adapter == nil {
		return sdk.EdgeExecutionResult{}, errors.New("composite edge adapter is required")
	}
	if ctx == nil {
		return sdk.EdgeExecutionResult{}, errors.New("composite edge execution context is required")
	}
	if err := ctx.Err(); err != nil {
		return sdk.EdgeExecutionResult{}, err
	}
	if err := sdk.ValidateEdgeExecutionRequest(request); err != nil {
		return sdk.EdgeExecutionResult{}, err
	}
	plan, ok := request.Plan.Opaque.(compositePlan)
	if !ok {
		return sdk.EdgeExecutionResult{}, errors.New("composite edge plan is missing child plans")
	}
	if err := validateCompositePlan(adapter, request, plan); err != nil {
		return sdk.EdgeExecutionResult{}, err
	}
	routed, err := routeReferences(request.ResourceReferences)
	if err != nil {
		return sdk.EdgeExecutionResult{}, err
	}
	for destination := range routed {
		if _, ok := adapter.clients[destination]; !ok {
			return sdk.EdgeExecutionResult{}, fmt.Errorf("composite edge has no adapter for destination %q", destination)
		}
	}
	result := sdk.EdgeExecutionResult{
		Action:              request.Action,
		OwnershipMarker:     request.OwnershipMarker,
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}
	for _, destination := range adapter.destinations() {
		childPlan, selected := plan.children[destination]
		if !selected {
			if len(routed[destination]) > 0 {
				return sdk.EdgeExecutionResult{}, fmt.Errorf("resource references exist for unplanned edge destination %q", destination)
			}
			continue
		}
		childRequest := sdk.EdgeExecutionRequest{
			Plan:               childPlan,
			Action:             request.Action,
			IdempotencyKey:     request.IdempotencyKey + ":" + destination,
			OwnershipMarker:    request.OwnershipMarker,
			ApprovalReference:  request.ApprovalReference,
			ResourceReferences: routed[destination],
		}
		childResult, err := adapter.clients[destination].ExecuteEdge(ctx, childRequest)
		if err != nil {
			return sdk.EdgeExecutionResult{}, fmt.Errorf("execute edge destination %q: %w", destination, err)
		}
		if err := sdk.ValidateEdgeExecutionResult(childRequest, childResult); err != nil {
			return sdk.EdgeExecutionResult{}, fmt.Errorf("validate edge destination %q result: %w", destination, err)
		}
		if strings.ContainsAny(childResult.OperationID, "\r\n\x00") {
			return sdk.EdgeExecutionResult{}, fmt.Errorf("edge destination %q returned an unsafe operation identity", destination)
		}
		result.OperationID = joinOperationID(result.OperationID, destination, childResult.OperationID)
		result.ResourceRefs = append(result.ResourceRefs, wrapReferences(destination, childResult.ResourceRefs)...)
		result.ProofRefs = append(result.ProofRefs, wrapReferences(destination, childResult.ProofRefs)...)
		result.Outputs = append(result.Outputs, prefixOutputs(destination, childResult.Outputs)...)
	}
	if result.OperationID == "" {
		result.OperationID = string(request.Action) + ":composite:" + markerDigest(request.OwnershipMarker)
	}
	sort.Strings(result.ResourceRefs)
	sort.Strings(result.ProofRefs)
	sort.Slice(result.Outputs, func(i, j int) bool { return result.Outputs[i].Key < result.Outputs[j].Key })
	return result, nil
}

func validateCompositePlan(adapter *CompositeAdapter, request sdk.EdgeExecutionRequest, plan compositePlan) error {
	descriptor := adapter.EdgeDescriptor()
	if request.Plan.AdapterID != descriptor.ID {
		return fmt.Errorf("composite edge plan adapter %q does not match adapter %q", request.Plan.AdapterID, descriptor.ID)
	}
	if request.Plan.TargetProvider != adapter.provider {
		return fmt.Errorf("composite edge plan provider %q does not match adapter provider %q", request.Plan.TargetProvider, adapter.provider)
	}
	if plan.adapterID == "" || plan.targetProvider == "" || plan.targetRuntime == "" || plan.childRoutingDigest == "" {
		return errors.New("composite edge plan integrity metadata is missing")
	}
	if plan.adapterID != descriptor.ID || request.Plan.AdapterID != plan.adapterID {
		return errors.New("composite edge plan adapter identity was changed after planning")
	}
	if plan.targetProvider != request.Plan.TargetProvider || plan.targetProvider != adapter.provider {
		return errors.New("composite edge plan target provider was changed after planning")
	}
	if plan.targetRuntime != request.Plan.TargetRuntime {
		return errors.New("composite edge plan target runtime was changed after planning")
	}
	if plan.ownershipMarker != request.Plan.OwnershipMarker || plan.ownershipMarker != request.OwnershipMarker {
		return errors.New("composite edge plan ownership was changed after planning")
	}
	if plan.childRoutingDigest != compositeChildRoutingDigest(plan.children) {
		return errors.New("composite edge plan child routing was changed after planning")
	}
	for destination, childPlan := range plan.children {
		child, ok := adapter.clients[destination]
		if !ok {
			return fmt.Errorf("composite edge plan has no adapter for destination %q", destination)
		}
		if isNilAdapter(child) {
			return fmt.Errorf("composite edge adapter for %q is required", destination)
		}
		childDescriptor := child.EdgeDescriptor()
		if err := sdk.ValidateEdgeAdapterDescriptor(childDescriptor); err != nil {
			return fmt.Errorf("validate composite edge child %q: %w", destination, err)
		}
		if childPlan.AdapterID != childDescriptor.ID {
			return fmt.Errorf("composite edge child %q adapter identity was changed after planning", destination)
		}
		if childPlan.TargetProvider != request.Plan.TargetProvider || childPlan.TargetRuntime != request.Plan.TargetRuntime {
			return fmt.Errorf("composite edge child %q target was changed after planning", destination)
		}
		if childPlan.OwnershipMarker != request.Plan.OwnershipMarker {
			return fmt.Errorf("composite edge child %q ownership was changed after planning", destination)
		}
	}
	return nil
}

func childPlanRequest(request sdk.EdgePlanRequest, destination string) (sdk.EdgePlanRequest, bool) {
	intent := request.Intent
	switch {
	case destination == strings.TrimSpace(intent.NativeProvider) && (intent.Mode == "native" || intent.Mode == "both"):
		intent.ExternalProvider = ""
		intent.Lifecycle = ""
		intent.Certification = ""
		intent.CredentialRefs = nil
		intent.Mode = "native"
		return sdk.EdgePlanRequest{TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime, Intent: intent, Configuration: cloneConfiguration(request.Configuration)}, true
	case destination == strings.TrimSpace(intent.ExternalProvider) && (intent.Mode == "external" || intent.Mode == "both" || (intent.Mode == "" && strings.TrimSpace(intent.ExternalProvider) != "")):
		intent.NativeProvider = ""
		intent.Mode = "external"
		return sdk.EdgePlanRequest{TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime, Intent: intent, Configuration: cloneConfiguration(request.Configuration)}, true
	default:
		return sdk.EdgePlanRequest{}, false
	}
}

func (adapter *CompositeAdapter) destinations() []string {
	destinations := make([]string, 0, len(adapter.clients))
	for destination := range adapter.clients {
		destinations = append(destinations, destination)
	}
	sort.Strings(destinations)
	return destinations
}

func compositeChildRoutingDigest(children map[string]sdk.EdgePlan) string {
	destinations := make([]string, 0, len(children))
	for destination := range children {
		destinations = append(destinations, destination)
	}
	sort.Strings(destinations)
	parts := make([]string, 0, len(destinations)*5)
	for _, destination := range destinations {
		child := children[destination]
		parts = append(parts,
			destination,
			child.AdapterID,
			string(child.TargetProvider),
			string(child.TargetRuntime),
			child.OwnershipMarker,
		)
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func cloneConfiguration(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func joinOperationID(current, destination, operation string) string {
	if strings.TrimSpace(operation) == "" {
		return current
	}
	if current == "" {
		return destination + ":" + operation
	}
	return current + "," + destination + ":" + operation
}

func prefixOutputs(destination string, outputs []sdk.AdapterOutput) []sdk.AdapterOutput {
	if len(outputs) == 0 {
		return nil
	}
	prefixed := make([]sdk.AdapterOutput, 0, len(outputs))
	for _, output := range outputs {
		prefixed = append(prefixed, sdk.AdapterOutput{Key: destination + "." + output.Key, Value: output.Value})
	}
	return prefixed
}

const compositeReferencePrefix = "composite:"

func wrapReferences(destination string, references []string) []string {
	wrapped := make([]string, 0, len(references))
	for _, reference := range references {
		wrapped = append(wrapped, compositeReferencePrefix+base64.RawURLEncoding.EncodeToString([]byte(destination))+":"+base64.RawURLEncoding.EncodeToString([]byte(reference)))
	}
	return wrapped
}

func routeReferences(references []string) (map[string][]string, error) {
	routed := make(map[string][]string)
	for _, reference := range references {
		if !strings.HasPrefix(reference, compositeReferencePrefix) {
			return nil, fmt.Errorf("composite edge received an unscoped resource reference %q", reference)
		}
		encodedDestination, encodedReference, ok := strings.Cut(strings.TrimPrefix(reference, compositeReferencePrefix), ":")
		if !ok || encodedDestination == "" || encodedReference == "" {
			return nil, fmt.Errorf("composite edge received an invalid resource reference %q", reference)
		}
		destinationBytes, err := base64.RawURLEncoding.DecodeString(encodedDestination)
		if err != nil || len(destinationBytes) == 0 {
			return nil, fmt.Errorf("composite edge received an invalid destination reference %q", reference)
		}
		resourceBytes, err := base64.RawURLEncoding.DecodeString(encodedReference)
		if err != nil || len(resourceBytes) == 0 {
			return nil, fmt.Errorf("composite edge received an invalid child reference %q", reference)
		}
		destination := string(destinationBytes)
		resource := string(resourceBytes)
		if strings.TrimSpace(destination) == "" || strings.ContainsAny(destination+resource, "\r\n\x00") {
			return nil, fmt.Errorf("composite edge received an unsafe resource reference %q", reference)
		}
		routed[destination] = append(routed[destination], resource)
	}
	return routed, nil
}

func markerDigest(marker string) string {
	digest := sha256.Sum256([]byte(marker))
	return hex.EncodeToString(digest[:8])
}

func isNilAdapter(adapter sdk.EdgeAdapter) bool {
	if adapter == nil {
		return true
	}
	value := reflect.ValueOf(adapter)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func containsAction(actions []sdk.EdgeAction, wanted sdk.EdgeAction) bool {
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}
