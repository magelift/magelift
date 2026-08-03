package infra

import (
	"context"
	"testing"

	sdk "github.com/acourtiol/magelift/sdk/v1"
)

type testTarget struct{ descriptor sdk.TargetDescriptor }

func (t testTarget) Descriptor() sdk.TargetDescriptor                { return t.descriptor }
func (testTarget) Validate(context.Context, sdk.TargetRequest) error { return nil }

type testCapability struct{ descriptor sdk.CapabilityDescriptor }

func (c testCapability) Descriptor() sdk.CapabilityDescriptor                { return c.descriptor }
func (testCapability) Validate(context.Context, sdk.CapabilityRequest) error { return nil }

type testOptions struct{ Value string }

func (testOptions) MageLiftResourceOptions() {}

type testTransform struct{ descriptor sdk.TransformDescriptor }

func (t testTransform) Descriptor() sdk.TransformDescriptor { return t.descriptor }
func (testTransform) Apply(_ context.Context, input sdk.TransformInput[testOptions]) (testOptions, error) {
	input.Options.Value += ":applied"
	return input.Options, nil
}

type testHook struct{ descriptor sdk.LifecycleHookDescriptor }

func (h testHook) Descriptor() sdk.LifecycleHookDescriptor                { return h.descriptor }
func (testHook) Validate(context.Context, sdk.LifecycleHookRequest) error { return nil }
func (testHook) Run(context.Context, sdk.LifecycleHookRequest) error      { return nil }

func TestRegistrySelectsTargetByProviderAndRuntime(t *testing.T) {
	registry := NewRegistry()
	target := testTarget{descriptor: sdk.TargetDescriptor{ID: "aws.ecs-fargate", Provider: "aws", Runtime: "ecs-fargate"}}
	if err := registry.RegisterTarget(target); err != nil {
		t.Fatal(err)
	}
	got, found := registry.Target("aws", "ecs-fargate")
	if !found || got.Descriptor() != target.Descriptor() {
		t.Fatalf("target = %#v, found = %v", got, found)
	}
	if _, found := registry.Target("gcp", "ecs-fargate"); found {
		t.Fatal("target leaked across providers")
	}
}

func TestRegistryRejectsDuplicateTargetPlatform(t *testing.T) {
	registry := NewRegistry()
	first := testTarget{descriptor: sdk.TargetDescriptor{ID: "aws.ecs-fargate", Provider: "aws", Runtime: "ecs-fargate"}}
	second := testTarget{descriptor: sdk.TargetDescriptor{ID: "aws.ecs-fargate-alt", Provider: "aws", Runtime: "ecs-fargate"}}
	if err := registry.RegisterTarget(first); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterTarget(second); err == nil {
		t.Fatal("duplicate platform was accepted")
	}
}

func TestCapabilitiesAreProviderScopedAndStable(t *testing.T) {
	registry := NewRegistry()
	providers := []testCapability{
		{descriptor: sdk.CapabilityDescriptor{ID: "aws.search", Capability: "search", Kind: sdk.CapabilitySearch, Provider: "aws"}},
		{descriptor: sdk.CapabilityDescriptor{ID: "aws.database", Capability: "database", Kind: sdk.CapabilityDatabase, Provider: "aws"}},
		{descriptor: sdk.CapabilityDescriptor{ID: "gcp.database", Capability: "database", Kind: sdk.CapabilityDatabase, Provider: "gcp"}},
	}
	for _, provider := range providers {
		if err := registry.RegisterCapability(provider); err != nil {
			t.Fatal(err)
		}
	}
	got := registry.Capabilities("aws")
	if len(got) != 2 || got[0].Descriptor().ID != "aws.database" || got[1].Descriptor().ID != "aws.search" {
		t.Fatalf("capabilities = %#v", got)
	}
}

func TestLifecycleOwnershipBoundary(t *testing.T) {
	tests := map[LifecycleOperation]LifecycleOwner{
		OperationDurableInfrastructure: OwnerTarget,
		OperationCandidateResource:     OwnerOrchestrator,
		OperationDeploymentLock:        OwnerOrchestrator,
		OperationReleaseOrchestration:  OwnerOrchestrator,
	}
	for operation, want := range tests {
		got, found := OwnerOf(operation)
		if !found || got != want {
			t.Fatalf("OwnerOf(%q) = %q, %v", operation, got, found)
		}
	}
	if _, found := OwnerOf("unknown"); found {
		t.Fatal("unknown operation has an owner")
	}
}

func TestNilRegistryAndInvalidDescriptorsAreRejected(t *testing.T) {
	var registry *Registry
	if err := registry.RegisterTarget(testTarget{}); err == nil {
		t.Fatal("nil registry accepted a target")
	}
	registry = NewRegistry()
	if err := registry.RegisterTarget(nil); err == nil {
		t.Fatal("nil target was accepted")
	}
	if err := registry.RegisterTarget(testTarget{}); err == nil {
		t.Fatal("invalid target descriptor was accepted")
	}
}

func TestRegistryDispatchesTypedTransforms(t *testing.T) {
	registry := NewRegistry()
	transform := testTransform{descriptor: sdk.TransformDescriptor{ID: "tags.production", Provider: "aws", Runtime: "ecs-fargate", Components: []sdk.ComponentID{"runtime.web"}}}
	if err := RegisterTransform(registry, transform); err != nil {
		t.Fatal(err)
	}
	got, err := ApplyTransform(context.Background(), registry, transform.descriptor.ID, "runtime.web", "service.web", testOptions{Value: "base"})
	if err != nil || got.Value != "base:applied" {
		t.Fatalf("transform result = %#v, error = %v", got, err)
	}
	if _, err := ApplyTransform(context.Background(), registry, transform.descriptor.ID, "runtime.worker", "service.worker", testOptions{}); err == nil {
		t.Fatal("transform applied to an undeclared component")
	}
	if err := RegisterTransform(registry, transform); err == nil {
		t.Fatal("duplicate transform accepted")
	}
}

func TestRegistryReturnsStableLifecycleHooks(t *testing.T) {
	registry := NewRegistry()
	for _, id := range []sdk.HookID{"vendor.z-last", "vendor.a-first"} {
		if err := registry.RegisterLifecycleHook(testHook{descriptor: sdk.LifecycleHookDescriptor{ID: id, Phase: sdk.PhasePostDeploy, Relationship: sdk.HookAfter, RelativeTo: "post-deploy.smoke", Timeout: 30, MaxAttempts: 1}}); err != nil {
			t.Fatal(err)
		}
	}
	hooks := registry.LifecycleHooks(sdk.PhasePostDeploy)
	if len(hooks) != 2 || hooks[0].Descriptor().ID != "vendor.a-first" || hooks[1].Descriptor().ID != "vendor.z-last" {
		t.Fatalf("hooks = %#v", hooks)
	}
}
