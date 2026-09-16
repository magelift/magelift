package providerhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

// ShimEdgeAdapter is a plugin-backed sdk.EdgeAdapter for native purge flows.
type ShimEdgeAdapter struct {
	client     *Client
	envelope   sdk.Envelope
	stored     sdk.StoredPlan
	descriptor sdk.EdgeAdapterDescriptor
}

var _ sdk.EdgeAdapter = (*ShimEdgeAdapter)(nil)

// NewShimEdgeAdapter builds the edge proxy for a shim plan. The descriptor
// comes from Describe negotiation, never from a hardcoded copy.
func NewShimEdgeAdapter(client *Client, planned platform.PlannedStack) (sdk.EdgeAdapter, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return nil, err
	}
	described := client.Describe()
	if described == nil || described.Edge == nil {
		return nil, errors.New("provider did not advertise a native edge adapter")
	}
	return &ShimEdgeAdapter{client: client, envelope: shim.inputs.envelope, stored: shim.stored, descriptor: *described.Edge}, nil
}

func (a *ShimEdgeAdapter) EdgeDescriptor() sdk.EdgeAdapterDescriptor {
	if a == nil {
		return sdk.EdgeAdapterDescriptor{}
	}
	return a.descriptor
}

func (a *ShimEdgeAdapter) PlanEdge(ctx context.Context, req sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	if a == nil || a.client == nil {
		return sdk.EdgePlan{}, errors.New("provider client is required")
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return sdk.EdgePlan{}, err
	}
	resp, err := Call[sdk.EdgePlanCall, sdk.EdgePlanResult](ctx, a.client, sdk.OpEdgePlan, &sdk.EdgePlanCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: a.envelope, Plan: a.stored, RequestJSON: encoded,
	})
	if err != nil {
		return sdk.EdgePlan{}, err
	}
	if resp.Error != nil {
		return sdk.EdgePlan{}, AsPluginError(sdk.OpEdgePlan, resp.Error)
	}
	var plan sdk.EdgePlan
	if err := json.Unmarshal(resp.PlanJSON, &plan); err != nil {
		return sdk.EdgePlan{}, fmt.Errorf("decode edge plan: %w", err)
	}
	return plan, nil
}

func (a *ShimEdgeAdapter) ExecuteEdge(ctx context.Context, req sdk.EdgeExecutionRequest) (sdk.EdgeExecutionResult, error) {
	if a == nil || a.client == nil {
		return sdk.EdgeExecutionResult{}, errors.New("provider client is required")
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return sdk.EdgeExecutionResult{}, err
	}
	resp, err := Call[sdk.EdgeExecuteCall, sdk.EdgeExecuteResult](ctx, a.client, sdk.OpEdgeExecute, &sdk.EdgeExecuteCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: a.envelope, Plan: a.stored, RequestJSON: encoded,
	})
	if err != nil {
		return sdk.EdgeExecutionResult{}, err
	}
	if resp.Error != nil {
		return sdk.EdgeExecutionResult{}, AsPluginError(sdk.OpEdgeExecute, resp.Error)
	}
	var result sdk.EdgeExecutionResult
	if err := json.Unmarshal(resp.ResultJSON, &result); err != nil {
		return sdk.EdgeExecutionResult{}, fmt.Errorf("decode edge execution result: %w", err)
	}
	return result, nil
}

// ShimResilienceAdapter is a plugin-backed sdk.ResilienceAdapter.
type ShimResilienceAdapter struct {
	client     *Client
	envelope   sdk.Envelope
	stored     sdk.StoredPlan
	descriptor sdk.ResilienceAdapterDescriptor
}

var _ sdk.ResilienceAdapter = (*ShimResilienceAdapter)(nil)

// NewShimResilienceAdapter builds the resilience proxy for a shim plan.
func NewShimResilienceAdapter(client *Client, planned platform.PlannedStack) (sdk.ResilienceAdapter, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return nil, err
	}
	described := client.Describe()
	if described == nil || described.Resilience == nil {
		return nil, errors.New("provider did not advertise a native resilience adapter")
	}
	return &ShimResilienceAdapter{client: client, envelope: shim.inputs.envelope, stored: shim.stored, descriptor: *described.Resilience}, nil
}

func (a *ShimResilienceAdapter) ResilienceDescriptor() sdk.ResilienceAdapterDescriptor {
	if a == nil {
		return sdk.ResilienceAdapterDescriptor{}
	}
	return a.descriptor
}

func (a *ShimResilienceAdapter) PlanResilience(ctx context.Context, req sdk.ResiliencePlanRequest) (sdk.ResiliencePlan, error) {
	if a == nil || a.client == nil {
		return sdk.ResiliencePlan{}, errors.New("provider client is required")
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return sdk.ResiliencePlan{}, err
	}
	resp, err := Call[sdk.ResiliencePlanCall, sdk.ResiliencePlanResult](ctx, a.client, sdk.OpResiliencePlan, &sdk.ResiliencePlanCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: a.envelope, Plan: a.stored, RequestJSON: encoded,
	})
	if err != nil {
		return sdk.ResiliencePlan{}, err
	}
	if resp.Error != nil {
		return sdk.ResiliencePlan{}, AsPluginError(sdk.OpResiliencePlan, resp.Error)
	}
	var plan sdk.ResiliencePlan
	if err := json.Unmarshal(resp.PlanJSON, &plan); err != nil {
		return sdk.ResiliencePlan{}, fmt.Errorf("decode resilience plan: %w", err)
	}
	return plan, nil
}

func (a *ShimResilienceAdapter) ExecuteResilience(ctx context.Context, req sdk.ResilienceExecutionRequest) (sdk.ResilienceExecutionResult, error) {
	if a == nil || a.client == nil {
		return sdk.ResilienceExecutionResult{}, errors.New("provider client is required")
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return sdk.ResilienceExecutionResult{}, err
	}
	resp, err := Call[sdk.ResilienceExecuteCall, sdk.ResilienceExecuteResult](ctx, a.client, sdk.OpResilienceExecute, &sdk.ResilienceExecuteCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: a.envelope, Plan: a.stored, RequestJSON: encoded,
	})
	if err != nil {
		return sdk.ResilienceExecutionResult{}, err
	}
	if resp.Error != nil {
		return sdk.ResilienceExecutionResult{}, AsPluginError(sdk.OpResilienceExecute, resp.Error)
	}
	var result sdk.ResilienceExecutionResult
	if err := json.Unmarshal(resp.ResultJSON, &result); err != nil {
		return sdk.ResilienceExecutionResult{}, fmt.Errorf("decode resilience execution result: %w", err)
	}
	return result, nil
}

// CleanupInventory runs ledger inventory over the client. The CLI wraps it
// as a CleanupProvider (the interface lives in cli to avoid an import
// cycle).
func CleanupInventory(ctx context.Context, client *Client, req sdk.CleanupInventoryRequest) ([]sdk.CleanupInventoryResource, error) {
	if client == nil {
		return nil, errors.New("provider client is required")
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := Call[sdk.InventoryCall, sdk.InventoryResult](ctx, client, sdk.OpInventory, &sdk.InventoryCall{
		ProtocolVersion: sdk.ProtocolV1, RequestJSON: encoded,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpInventory, resp.Error)
	}
	var resources []sdk.CleanupInventoryResource
	if err := json.Unmarshal(resp.ResourcesJSON, &resources); err != nil {
		return nil, fmt.Errorf("decode cleanup inventory: %w", err)
	}
	return resources, nil
}

// CleanupDelete runs ledger deletion over the client.
func CleanupDelete(ctx context.Context, client *Client, project, marker string, resource sdk.CleanupResource) error {
	if client == nil {
		return errors.New("provider client is required")
	}
	encoded, err := json.Marshal(resource)
	if err != nil {
		return err
	}
	resp, err := Call[sdk.DeleteCall, sdk.DeleteResult](ctx, client, sdk.OpDelete, &sdk.DeleteCall{
		ProtocolVersion: sdk.ProtocolV1, Project: project, Marker: marker, ResourceJSON: encoded,
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return AsPluginError(sdk.OpDelete, resp.Error)
	}
	return nil
}

// DestroyLeftoverBackups deletes provider backups left after teardown.
func DestroyLeftoverBackups(ctx context.Context, client *Client, planned platform.PlannedStack) ([]string, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return nil, err
	}
	resp, err := Call[sdk.DestroyLeftoverBackupsCall, sdk.DestroyLeftoverBackupsResult](ctx, shimClientOf(shim), sdk.OpDestroyLeftoverBackups, &sdk.DestroyLeftoverBackupsCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpDestroyLeftoverBackups, resp.Error)
	}
	return resp.Destroyed, nil
}

// SecretStore is a plugin-backed secretref.GCPSecretManagerProvider.
type SecretStore struct {
	client   *Client
	envelope sdk.Envelope
}

// NewSecretStore builds the secretref provider over a connected client. The
// envelope carries request context (project scoping comes from the full
// resource names).
func NewSecretStore(client *Client, envelope sdk.Envelope) (*SecretStore, error) {
	if client == nil {
		return nil, errors.New("provider client is required")
	}
	return &SecretStore{client: client, envelope: envelope}, nil
}

// GetSecretValue resolves one full version resource name.
func (s *SecretStore) GetSecretValue(ctx context.Context, name string) ([]byte, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("provider client is required")
	}
	resp, err := Call[sdk.SecretReadRequest, sdk.SecretReadResult](ctx, s.client, sdk.OpSecretRead, &sdk.SecretReadRequest{
		ProtocolVersion: sdk.ProtocolV1, Envelope: s.envelope, Name: name,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpSecretRead, resp.Error)
	}
	return resp.Value, nil
}

// PluginBackend is a plugin-backed automation backend for deploy flows.
type PluginBackend struct {
	client   *Client
	envelope sdk.Envelope
	stored   sdk.StoredPlan
}

var _ automation.Backend = (*PluginBackend)(nil)

// NewPluginBackend binds a client plus plan to the automation backend
// interface. Preview metadata travels per call from the automation request.
func NewPluginBackend(client *Client, envelope sdk.Envelope, stored sdk.StoredPlan) (*PluginBackend, error) {
	if client == nil {
		return nil, errors.New("provider client is required")
	}
	if len(stored.Opaque) == 0 {
		return nil, errors.New("stored plan is required")
	}
	return &PluginBackend{client: client, envelope: envelope, stored: stored}, nil
}

func (b *PluginBackend) stackCall(request automation.Request) (*sdk.StackCall, error) {
	if b == nil || b.client == nil {
		return nil, errors.New("provider client is required")
	}
	call := &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: b.envelope, Plan: b.stored}
	if request.Preview != nil {
		encoded, err := json.Marshal(request.Preview)
		if err != nil {
			return nil, err
		}
		call.PreviewMetadataJSON = encoded
	}
	return call, nil
}

func (b *PluginBackend) runLifecycle(ctx context.Context, operation sdk.Operation, request automation.Request, diagnostics io.Writer) (map[string]int, error) {
	call, err := b.stackCall(request)
	if err != nil {
		return nil, err
	}
	resp, err := Call[sdk.StackCall, sdk.LifecycleResult](ctx, b.client, operation, call)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(operation, resp.Error)
	}
	writePluginDiagnostics(diagnostics, resp.Diagnostics)
	return map[string]int{
		"create": resp.Summary.Create, "update": resp.Summary.Update,
		"delete": resp.Summary.Delete, "replace": resp.Summary.Replace,
		"same": resp.Summary.Same,
	}, nil
}

// Preview runs a provider-side preview.
func (b *PluginBackend) Preview(ctx context.Context, request automation.Request, diagnostics io.Writer) (map[string]int, error) {
	return b.runLifecycle(ctx, sdk.OpPreview, request, diagnostics)
}

// Update runs a provider-side update.
func (b *PluginBackend) Update(ctx context.Context, request automation.Request, diagnostics io.Writer) (map[string]int, error) {
	return b.runLifecycle(ctx, sdk.OpApply, request, diagnostics)
}

// Destroy runs a provider-side destroy.
func (b *PluginBackend) Destroy(ctx context.Context, request automation.Request, diagnostics io.Writer) (map[string]int, error) {
	_ = request.Destroy
	return b.runLifecycle(ctx, sdk.OpDestroy, request, diagnostics)
}

// Outputs reads raw stack outputs for machinery (deploy steps, exec).
func (b *PluginBackend) Outputs(ctx context.Context) (map[string]any, error) {
	if b == nil || b.client == nil {
		return nil, errors.New("provider client is required")
	}
	resp, err := Call[sdk.StackCall, sdk.OutputsResult](ctx, b.client, sdk.OpOutputs, &sdk.StackCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: b.envelope, Plan: b.stored,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpOutputs, resp.Error)
	}
	var values map[string]any
	if err := json.Unmarshal(resp.ValuesJSON, &values); err != nil {
		return nil, fmt.Errorf("decode stack outputs: %w", err)
	}
	return values, nil
}

// RedactedOutputs reads stack outputs with secret values masked for
// display. Secret flags come from the provider, never from key-name
// guessing.
func (b *PluginBackend) RedactedOutputs(ctx context.Context) (map[string]any, error) {
	if b == nil || b.client == nil {
		return nil, errors.New("provider client is required")
	}
	resp, err := Call[sdk.StackCall, sdk.OutputsResult](ctx, b.client, sdk.OpOutputs, &sdk.StackCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: b.envelope, Plan: b.stored,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpOutputs, resp.Error)
	}
	var values map[string]any
	if err := json.Unmarshal(resp.ValuesJSON, &values); err != nil {
		return nil, fmt.Errorf("decode stack outputs: %w", err)
	}
	for _, key := range resp.SecretKeys {
		if _, ok := values[key]; ok {
			values[key] = map[string]any{"secret": true}
		}
	}
	return values, nil
}

func writePluginDiagnostics(diagnostics io.Writer, lines []string) {
	if diagnostics == nil {
		return
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		_, _ = io.WriteString(diagnostics, line+"\n")
	}
}
