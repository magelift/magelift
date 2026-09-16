package edge

import (
	"context"
	"reflect"
	"testing"

	"github.com/magelift/magelift/sdk"
)

type recordingEdgeAdapter struct {
	descriptor sdk.EdgeAdapterDescriptor
	requests   []sdk.EdgePlanRequest
	executions []sdk.EdgeExecutionRequest
}

func (adapter *recordingEdgeAdapter) EdgeDescriptor() sdk.EdgeAdapterDescriptor {
	return adapter.descriptor
}

func (adapter *recordingEdgeAdapter) PlanEdge(_ context.Context, request sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	adapter.requests = append(adapter.requests, request)
	return sdk.EdgePlan{
		AdapterID:       adapter.descriptor.ID,
		TargetProvider:  request.TargetProvider,
		TargetRuntime:   request.TargetRuntime,
		OwnershipMarker: request.Intent.OwnershipMarker,
		Outputs:         []sdk.AdapterOutput{{Key: "endpoint", Value: adapter.descriptor.ID}},
		Opaque:          adapter.descriptor.ID,
	}, nil
}

func (adapter *recordingEdgeAdapter) ExecuteEdge(_ context.Context, request sdk.EdgeExecutionRequest) (sdk.EdgeExecutionResult, error) {
	adapter.executions = append(adapter.executions, request)
	return sdk.EdgeExecutionResult{
		Action:              request.Action,
		OperationID:         adapter.descriptor.ID + ":operation",
		ResourceRefs:        []string{adapter.descriptor.ID + ":resource"},
		ProofRefs:           []string{adapter.descriptor.ID + ":" + sdk.EdgeProofOriginHealth},
		Outputs:             []sdk.AdapterOutput{{Key: "endpoint", Value: adapter.descriptor.ID}},
		OwnershipMarker:     request.OwnershipMarker,
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}, nil
}

func newRecordingEdgeAdapter(id, provider string) *recordingEdgeAdapter {
	return &recordingEdgeAdapter{descriptor: sdk.EdgeAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: id, Provider: sdk.ProviderID(provider), Version: "1.0.0",
		Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy},
	}}
}

func TestCompositeAdapterSplitsIntentAndRoutesOpaqueReferences(t *testing.T) {
	native := newRecordingEdgeAdapter("aws.cloudfront", "aws")
	external := newRecordingEdgeAdapter("fastly.edge", "fastly")
	adapter, err := NewCompositeAdapter("aws", map[string]sdk.EdgeAdapter{"cloudfront": native, "fastly": external})
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.EdgePlanRequest{
		TargetProvider: "aws", TargetRuntime: "ecs-fargate",
		Intent: sdk.EdgeIntent{
			Mode: "both", NativeProvider: "cloudfront", ExternalProvider: "fastly",
			Lifecycle: sdk.ExternalLifecycleExtension, Certification: sdk.ExternalExperimental,
			CredentialRefs: []string{"aws-secrets-manager://magelift/fastly"}, Domains: []string{"shop.example.com"},
			OriginHealthRef: "health:origin", OwnershipMarker: "magelift/test/edge-composite",
		},
	}
	plan, err := adapter.PlanEdge(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(native.requests) != 1 || len(external.requests) != 1 {
		t.Fatalf("child plan calls = native=%d external=%d", len(native.requests), len(external.requests))
	}
	if native.requests[0].Intent.ExternalProvider != "" || len(native.requests[0].Intent.CredentialRefs) != 0 || native.requests[0].Intent.Mode != "native" {
		t.Fatalf("native child intent = %#v", native.requests[0].Intent)
	}
	if external.requests[0].Intent.NativeProvider != "" || external.requests[0].Intent.Mode != "external" {
		t.Fatalf("external child intent = %#v", external.requests[0].Intent)
	}
	if len(plan.Outputs) != 2 || plan.Opaque == nil {
		t.Fatalf("composite plan = %#v", plan)
	}

	applied, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{
		Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "magelift/test/edge-composite/apply", OwnershipMarker: request.Intent.OwnershipMarker,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantRefs := []string{wrapReferences("cloudfront", []string{"aws.cloudfront:resource"})[0], wrapReferences("fastly", []string{"fastly.edge:resource"})[0]}
	if !reflect.DeepEqual(applied.ResourceRefs, wantRefs) {
		t.Fatalf("composite resource refs = %v, want %v", applied.ResourceRefs, wantRefs)
	}

	destroyed, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{
		Plan: plan, Action: sdk.EdgeDestroy, IdempotencyKey: "magelift/test/edge-composite/destroy", OwnershipMarker: request.Intent.OwnershipMarker,
		ResourceReferences: applied.ResourceRefs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !destroyed.OwnershipVerified || !destroyed.IdempotencyVerified || len(native.executions) != 2 || len(external.executions) != 2 {
		t.Fatalf("destroy result/calls = %#v, native=%d external=%d", destroyed, len(native.executions), len(external.executions))
	}
	if !reflect.DeepEqual(native.executions[1].ResourceReferences, []string{"aws.cloudfront:resource"}) || !reflect.DeepEqual(external.executions[1].ResourceReferences, []string{"fastly.edge:resource"}) {
		t.Fatalf("destroy reference routing = native=%v external=%v", native.executions[1].ResourceReferences, external.executions[1].ResourceReferences)
	}
}

func TestCompositeAdapterPreservesEdgeSafetyIntentAcrossNativeAndExternalChildren(t *testing.T) {
	native := newRecordingEdgeAdapter("aws.cloudfront", "aws")
	external := newRecordingEdgeAdapter("fastly.edge", "fastly")
	adapter, err := NewCompositeAdapter("aws", map[string]sdk.EdgeAdapter{"cloudfront": native, "fastly": external})
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.EdgePlanRequest{
		TargetProvider: "aws", TargetRuntime: "ecs-fargate",
		Intent: sdk.EdgeIntent{
			Mode: "both", NativeProvider: "cloudfront", ExternalProvider: "fastly",
			Lifecycle: sdk.ExternalLifecycleExtension, Certification: sdk.ExternalExperimental,
			CredentialRefs: []string{"aws-secrets-manager://magelift/fastly"},
			Domains:        []string{"shop.example.com"}, TLS: true, TLSMode: "managed", DNSMode: "external",
			PurgeOnDeploy: true, PolicyReference: "policy/edge", OriginHealthRef: "health/origin",
			CachePolicyRef: "cache/shop", PurgePolicyRef: "purge/all", WAFPolicyRef: "waf/shop",
			FailoverPolicyRef: "failover/shop", OwnershipMarker: "magelift/test/edge-safety",
			Health: sdk.EdgeHealthIntent{OriginURL: "https://origin.example.com/health", OriginHost: "origin.example.com", ExpectedRouteTarget: "dualstack.fastly.net", RoutePath: "/health", ExpectedStatus: 200, RouteTimeoutSeconds: 30, RoutePollSeconds: 2},
		},
	}
	if _, err := adapter.PlanEdge(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(native.requests) != 1 || len(external.requests) != 1 {
		t.Fatalf("child plan calls = native=%d external=%d", len(native.requests), len(external.requests))
	}
	nativeIntent := native.requests[0].Intent
	externalIntent := external.requests[0].Intent
	for _, child := range []struct {
		name   string
		intent sdk.EdgeIntent
	}{
		{name: "native", intent: nativeIntent},
		{name: "external", intent: externalIntent},
	} {
		if child.intent.Domains[0] != "shop.example.com" || !child.intent.TLS || child.intent.TLSMode != "managed" || child.intent.DNSMode != "external" || !child.intent.PurgeOnDeploy || child.intent.CachePolicyRef != "cache/shop" || child.intent.PurgePolicyRef != "purge/all" || child.intent.WAFPolicyRef != "waf/shop" || child.intent.FailoverPolicyRef != "failover/shop" || child.intent.OriginHealthRef != "health/origin" || child.intent.Health.RoutePath != "/health" {
			t.Errorf("%s child lost edge safety intent: %#v", child.name, child.intent)
		}
	}
	if nativeIntent.ExternalProvider != "" || len(nativeIntent.CredentialRefs) != 0 || externalIntent.NativeProvider != "" {
		t.Fatalf("provider-specific intent leaked across children: native=%#v external=%#v", nativeIntent, externalIntent)
	}
}

func TestCompositeAdapterRejectsUnscopedReferenceAndTypedNil(t *testing.T) {
	var typedNil *recordingEdgeAdapter
	if _, err := NewCompositeAdapter("aws", map[string]sdk.EdgeAdapter{"cloudfront": typedNil}); err == nil {
		t.Fatal("typed nil edge adapter was accepted")
	}
	native := newRecordingEdgeAdapter("aws.cloudfront", "aws")
	adapter, err := NewCompositeAdapter("aws", map[string]sdk.EdgeAdapter{"cloudfront": native})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.PlanEdge(context.Background(), sdk.EdgePlanRequest{
		TargetProvider: "aws", TargetRuntime: "ecs", Intent: sdk.EdgeIntent{Mode: "native", NativeProvider: "cloudfront", OriginHealthRef: "health:origin", OwnershipMarker: "magelift/test/edge"},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeDestroy, IdempotencyKey: "destroy", OwnershipMarker: "magelift/test/edge", ResourceReferences: []string{"aws.cloudfront:resource"}})
	if err == nil {
		t.Fatal("unscoped edge reference was accepted")
	}
}

func TestCompositeAdapterRejectsTamperedPlanBeforeChildMutation(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(*testing.T, *sdk.EdgeExecutionRequest)
	}{
		{
			name: "adapter identity",
			tamper: func(_ *testing.T, request *sdk.EdgeExecutionRequest) {
				request.Plan.AdapterID = "aws.edge.composite.tampered"
			},
		},
		{
			name: "target provider",
			tamper: func(_ *testing.T, request *sdk.EdgeExecutionRequest) {
				request.Plan.TargetProvider = "gcp"
			},
		},
		{
			name: "target runtime",
			tamper: func(_ *testing.T, request *sdk.EdgeExecutionRequest) {
				request.Plan.TargetRuntime = "gke"
			},
		},
		{
			name: "ownership",
			tamper: func(_ *testing.T, request *sdk.EdgeExecutionRequest) {
				request.Plan.OwnershipMarker = "magelift/test/tampered-ownership"
				request.OwnershipMarker = request.Plan.OwnershipMarker
			},
		},
		{
			name: "child routing",
			tamper: func(t *testing.T, request *sdk.EdgeExecutionRequest) {
				plan, ok := request.Plan.Opaque.(compositePlan)
				if !ok {
					t.Fatal("composite plan opaque value has unexpected type")
				}
				delete(plan.children, "cloudfront")
				request.Plan.Opaque = plan
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, plan, native, external := newPlannedCompositeEdge(t)
			request := sdk.EdgeExecutionRequest{
				Plan:            plan,
				Action:          sdk.EdgeDestroy,
				IdempotencyKey:  "magelift/test/edge-composite/tampered",
				OwnershipMarker: plan.OwnershipMarker,
			}
			tt.tamper(t, &request)

			if _, err := adapter.ExecuteEdge(context.Background(), request); err == nil {
				t.Fatal("tampered composite plan was accepted")
			}
			if len(native.executions) != 0 || len(external.executions) != 0 {
				t.Fatalf("child mutation occurred before tamper rejection: native=%d external=%d", len(native.executions), len(external.executions))
			}
		})
	}
}

func newPlannedCompositeEdge(t *testing.T) (sdk.EdgeAdapter, sdk.EdgePlan, *recordingEdgeAdapter, *recordingEdgeAdapter) {
	t.Helper()
	native := newRecordingEdgeAdapter("aws.cloudfront", "aws")
	external := newRecordingEdgeAdapter("fastly.edge", "fastly")
	adapter, err := NewCompositeAdapter("aws", map[string]sdk.EdgeAdapter{"cloudfront": native, "fastly": external})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.PlanEdge(context.Background(), sdk.EdgePlanRequest{
		TargetProvider: "aws",
		TargetRuntime:  "ecs",
		Intent: sdk.EdgeIntent{
			Mode:             "both",
			NativeProvider:   "cloudfront",
			ExternalProvider: "fastly",
			Lifecycle:        sdk.ExternalLifecycleExtension,
			Certification:    sdk.ExternalExperimental,
			CredentialRefs:   []string{"aws-secrets-manager://magelift/fastly"},
			OriginHealthRef:  "health:origin",
			OwnershipMarker:  "magelift/test/edge-tamper",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter, plan, native, external
}
