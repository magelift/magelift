package fastly

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/sdk"
)

// SDKAdapter exposes the existing Fastly implementation through the public
// provider-neutral edge port. Fastly configuration remains adapter-owned;
// only opaque resource references and semantic proof flags cross the port.
type SDKAdapter struct {
	Adapter   Adapter
	lifecycle Lifecycle
}

func NewSDKAdapter(adapter Adapter) SDKAdapter {
	return SDKAdapter{Adapter: adapter}
}

// Lifecycle is the provider-owned Fastly semantic lifecycle used by the
// public SDK bridge. Both the CLI backend and the official Go SDK backend
// implement this contract; the bridge owns the portable request/result shape
// once, so adding a backend does not duplicate edge validation.
type Lifecycle interface {
	Plan(Request) (Plan, error)
	Apply(context.Context, Request) (Result, error)
	Verify(context.Context, Request, Result) error
	Destroy(context.Context, Request, Result) error
}

// FailoverLifecycle is an optional extension of Lifecycle. The public edge
// descriptor advertises failover and rollback only when the provider-owned
// controller is present and can prove route convergence.
type FailoverLifecycle interface {
	SupportsFailover() bool
	Failover(context.Context, Request, Result) (Result, error)
	Rollback(context.Context, Request, Result) (Result, error)
}

func NewSDKLifecycleAdapter(lifecycle Lifecycle) SDKAdapter {
	return SDKAdapter{lifecycle: lifecycle}
}

func (adapter SDKAdapter) backend() Lifecycle {
	if adapter.lifecycle != nil {
		return adapter.lifecycle
	}
	return adapter.Adapter
}

func (adapter SDKAdapter) EdgeDescriptor() sdk.EdgeAdapterDescriptor {
	capabilities := []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy}
	if failover, ok := adapter.backend().(FailoverLifecycle); ok && failover.SupportsFailover() {
		capabilities = []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeFailover, sdk.EdgeRollback, sdk.EdgeDestroy}
	}
	return sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "fastly.edge", Provider: "fastly", Version: "1.0.0", Capabilities: capabilities}
}

func (adapter SDKAdapter) PlanEdge(_ context.Context, request sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	fastlyRequest, err := requestFromSDK(request)
	if err != nil {
		return sdk.EdgePlan{}, err
	}
	plan, err := adapter.backend().Plan(fastlyRequest)
	if err != nil {
		return sdk.EdgePlan{}, err
	}
	return sdk.EdgePlan{
		AdapterID: adapter.EdgeDescriptor().ID, TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime,
		OwnershipMarker: plan.OwnershipMarker, Outputs: sdkOutputs(plan.OutputKeys), Opaque: plan,
	}, nil
}

func (adapter SDKAdapter) ExecuteEdge(ctx context.Context, request sdk.EdgeExecutionRequest) (sdk.EdgeExecutionResult, error) {
	plan, ok := request.Plan.Opaque.(Plan)
	if !ok {
		return sdk.EdgeExecutionResult{}, errors.New("Fastly SDK edge plan is missing its adapter-owned plan")
	}
	fastlyRequest, err := requestFromPlan(plan)
	if err != nil {
		return sdk.EdgeExecutionResult{}, err
	}
	fastlyResult := resultFromReferences(plan, request.ResourceReferences)
	switch request.Action {
	case sdk.EdgeApply:
		fastlyResult, err = adapter.backend().Apply(ctx, fastlyRequest)
	case sdk.EdgeVerify:
		if fastlyResult.ServiceID == "" {
			return sdk.EdgeExecutionResult{}, errors.New("Fastly verification requires a service resource reference")
		}
		err = adapter.backend().Verify(ctx, fastlyRequest, fastlyResult)
	case sdk.EdgeDestroy:
		if fastlyResult.ServiceID == "" {
			return sdk.EdgeExecutionResult{}, errors.New("Fastly cleanup requires a service resource reference")
		}
		err = adapter.backend().Destroy(ctx, fastlyRequest, fastlyResult)
	case sdk.EdgeFailover, sdk.EdgeRollback:
		failover, supported := adapter.backend().(FailoverLifecycle)
		if !supported || !failover.SupportsFailover() {
			return sdk.EdgeExecutionResult{}, sdk.EdgeCapabilityError{AdapterID: adapter.EdgeDescriptor().ID, Action: request.Action, Status: sdk.EdgeCapabilityUnsupported, Reason: "the Fastly adapter has no provider-owned failover controller"}
		}
		if fastlyResult.ServiceID == "" {
			return sdk.EdgeExecutionResult{}, errors.New("Fastly route mutation requires a service resource reference")
		}
		if request.Action == sdk.EdgeFailover {
			fastlyResult, err = failover.Failover(ctx, fastlyRequest, fastlyResult)
		} else {
			fastlyResult, err = failover.Rollback(ctx, fastlyRequest, fastlyResult)
		}
	default:
		return sdk.EdgeExecutionResult{}, fmt.Errorf("unsupported Fastly SDK edge action %q", request.Action)
	}
	if err != nil {
		return sdk.EdgeExecutionResult{}, err
	}
	resourceReferences := referencesFromResult(fastlyResult)
	proofRefs := []string{"fastly:ownership", "fastly:origin-health", "fastly:routing"}
	if request.Action == sdk.EdgeFailover || request.Action == sdk.EdgeRollback {
		proofRefs = append(proofRefs, "fastly:route-convergence")
	}
	if request.Action == sdk.EdgeDestroy {
		proofRefs = append(proofRefs, "fastly:owning-service-inventory-empty")
	}
	return sdk.EdgeExecutionResult{
		Action: request.Action, OperationID: operationIdentity(request.Action, fastlyResult), ResourceRefs: resourceReferences,
		ProofRefs: proofRefs, Outputs: sdkOutputsFromFastly(fastlyResult), OwnershipMarker: request.OwnershipMarker,
		OwnershipVerified: true, IdempotencyVerified: true,
	}, nil
}

func requestFromSDK(request sdk.EdgePlanRequest) (Request, error) {
	if request.Intent.ExternalProvider != "fastly" {
		return Request{}, fmt.Errorf("Fastly SDK edge adapter requires external provider fastly, got %q", request.Intent.ExternalProvider)
	}
	providerConfig, err := configFromIntent(request.Intent, request.Configuration)
	if err != nil {
		return Request{}, err
	}
	return Request{
		ServiceID: request.Intent.ServiceReference, ServiceName: stringOption(request.Configuration, "serviceName"),
		CredentialRef: firstReference(request.Intent.CredentialRefs), Domains: append([]string(nil), request.Intent.Domains...),
		OwnershipMarker: request.Intent.OwnershipMarker, PurgeOnDeploy: request.Intent.PurgeOnDeploy,
		CreateService: request.Intent.ServiceReference == "", ProviderConfig: providerConfig,
	}, nil
}

func requestFromPlan(plan Plan) (Request, error) {
	return Request{
		ServiceID: plan.ServiceID, ServiceName: plan.ServiceName, CredentialRef: plan.CredentialRef, Domains: append([]string(nil), plan.Domains...),
		OwnershipMarker: plan.OwnershipMarker, PurgeOnDeploy: plan.PurgeOnDeploy, CreateService: plan.CreateService, ProviderConfig: plan.ProviderConfig,
	}, nil
}

func configFromIntent(intent sdk.EdgeIntent, configuration map[string]any) (Config, error) {
	base := Config{
		Version: stringOption(configuration, "version"), ActivateVersion: boolOption(configuration, "activateVersion"), TLS: intent.TLS, TLSMode: intent.TLSMode, DNSMode: intent.DNSMode,
		TLSSubscriptionID: stringOption(configuration, "tlsSubscriptionId"), VCLName: stringOption(configuration, "vclName"),
		VCLContent: stringOption(configuration, "vclContent"), VCLRef: stringOption(configuration, "vclRef"),
		OriginHealthRef: intent.OriginHealthRef, OriginAddress: stringOption(configuration, "originAddress"),
		OriginHost: stringOption(configuration, "originHost"), OriginPort: intOption(configuration, "originPort"),
		OriginUseTLS: boolOption(configuration, "originUseTls"), BackendName: stringOption(configuration, "backendName"), RouteSnippetName: stringOption(configuration, "routeSnippetName"),
		CachePolicyRef: intent.CachePolicyRef, PurgePolicyRef: intent.PurgePolicyRef,
		WAFPolicyRef: intent.WAFPolicyRef, FailoverPolicyRef: intent.FailoverPolicyRef,
		HealthOriginURL: intent.Health.OriginURL, HealthOriginHost: intent.Health.OriginHost, HealthExpectedCNAME: intent.Health.ExpectedRouteTarget, HealthRoutePath: intent.Health.RoutePath,
		HealthExpectedStatus: intent.Health.ExpectedStatus, HealthRouteTimeoutSeconds: intent.Health.RouteTimeoutSeconds, HealthRoutePollSeconds: intent.Health.RoutePollSeconds,
	}
	if advanced, ok := fastlyExtensionConfig(configuration); ok {
		return applyConfigOverlay(base, advanced)
	}
	return base, nil
}

func stringOption(configuration map[string]any, key string) string {
	value, _ := configuration[key].(string)
	return value
}

func intOption(configuration map[string]any, key string) int {
	switch value := configuration[key].(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func boolOption(configuration map[string]any, key string) bool {
	value, _ := configuration[key].(bool)
	return value
}

func firstReference(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func sdkOutputs(keys []string) []sdk.AdapterOutput {
	outputs := make([]sdk.AdapterOutput, 0, len(keys))
	for _, key := range keys {
		outputs = append(outputs, sdk.AdapterOutput{Key: key})
	}
	return outputs
}

func sdkOutputsFromFastly(result Result) []sdk.AdapterOutput {
	outputs := make([]sdk.AdapterOutput, 0, len(result.Outputs))
	for _, output := range result.Outputs {
		outputs = append(outputs, sdk.AdapterOutput{Key: output.Key, Value: output.Value})
	}
	return outputs
}

func referencesFromResult(result Result) []string {
	refs := []string{"service:" + result.ServiceID}
	if result.CreatedService {
		refs = append(refs, "created-service:"+result.ServiceID)
	}
	for _, domain := range result.CreatedDomains {
		refs = append(refs, "domain:"+domain)
	}
	if result.CreatedBackend && result.BackendName != "" {
		refs = append(refs, "backend:"+result.BackendName)
	}
	if result.CreatedRouteSnippet && result.RouteSnippetName != "" {
		refs = append(refs, "route-snippet:"+result.RouteSnippetName)
	}
	if result.VersionActivated {
		refs = append(refs, "version:activated")
	}
	if result.CreatedTLS && result.TLSSubscriptionID != "" {
		refs = append(refs, "tls:"+result.TLSSubscriptionID)
	}
	if result.VCLApplied {
		refs = append(refs, "vcl:applied")
	}
	if result.FailoverApplied {
		refs = append(refs, "failover:applied")
	}
	if result.RollbackApplied {
		refs = append(refs, "rollback:applied")
	}
	return refs
}

func resultFromReferences(plan Plan, refs []string) Result {
	result := Result{ServiceID: plan.ServiceID, CreatedService: plan.CreateService, OwnershipMarker: plan.OwnershipMarker, Version: plan.ProviderConfig.Version, PurgeRequested: plan.PurgeOnDeploy, VCLApplied: plan.ProviderConfig.VCLName != ""}
	for _, ref := range refs {
		switch {
		case strings.HasPrefix(ref, "service:"):
			result.ServiceID = strings.TrimPrefix(ref, "service:")
		case strings.HasPrefix(ref, "created-service:"):
			result.ServiceID = strings.TrimPrefix(ref, "created-service:")
			result.CreatedService = true
		case strings.HasPrefix(ref, "domain:"):
			result.CreatedDomains = append(result.CreatedDomains, strings.TrimPrefix(ref, "domain:"))
		case strings.HasPrefix(ref, "backend:"):
			result.BackendName = strings.TrimPrefix(ref, "backend:")
			result.CreatedBackend = true
		case strings.HasPrefix(ref, "route-snippet:"):
			result.RouteSnippetName = strings.TrimPrefix(ref, "route-snippet:")
			result.CreatedRouteSnippet = true
		case ref == "version:activated":
			result.VersionActivated = true
		case strings.HasPrefix(ref, "tls:"):
			result.TLSSubscriptionID = strings.TrimPrefix(ref, "tls:")
			result.CreatedTLS = true
		case ref == "vcl:applied":
			result.VCLApplied = true
		case ref == "failover:applied":
			result.FailoverApplied = true
		case ref == "rollback:applied":
			result.RollbackApplied = true
		}
	}
	return result
}

func operationIdentity(action sdk.EdgeAction, result Result) string {
	if result.ServiceID == "" {
		return string(action)
	}
	return string(action) + ":" + result.ServiceID
}
