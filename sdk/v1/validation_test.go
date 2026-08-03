package v1

import (
	"strings"
	"testing"
)

func TestValidatesTargetAndCapabilityDescriptors(t *testing.T) {
	if err := ValidateTargetDescriptor(TargetDescriptor{ID: "aws.ecs", Provider: "aws", Runtime: "ecs-fargate"}); err != nil {
		t.Fatal(err)
	}
	err := ValidateCapabilityDescriptors([]CapabilityDescriptor{
		{ID: "aws.aurora", Capability: "database.mysql", Kind: CapabilityDatabase, Provider: "aws"},
		{ID: "aws.valkey", Capability: "cache.valkey", Kind: CapabilityCache, Provider: "aws"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRejectsUnstableAndDuplicateCapabilityDescriptors(t *testing.T) {
	err := ValidateCapabilityDescriptors([]CapabilityDescriptor{
		{ID: "AWS Aurora", Capability: "database.mysql", Kind: CapabilityDatabase, Provider: "aws"},
		{ID: "AWS Aurora", Capability: "database.mysql", Kind: "compute", Provider: "aws"},
	})
	if err == nil || !strings.Contains(err.Error(), "not a stable ID") || !strings.Contains(err.Error(), "duplicate capability provider ID") || !strings.Contains(err.Error(), "invalid capability kind") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatesArtifactCapabilityRequirements(t *testing.T) {
	available := []CapabilityID{CapabilityDatabaseMySQL, CapabilityCacheValkey, CapabilityQueueDatabase}
	if err := ValidateCapabilityRequirements([]CapabilityID{CapabilityDatabaseMySQL, CapabilityQueueDatabase}, available); err != nil {
		t.Fatal(err)
	}
	err := ValidateCapabilityRequirements([]CapabilityID{CapabilityQueueRabbitMQ, CapabilityQueueRabbitMQ, "Invalid capability"}, available)
	if err == nil || !strings.Contains(err.Error(), "duplicate required capability") || !strings.Contains(err.Error(), "not a stable ID") || !strings.Contains(err.Error(), "queue.rabbitmq") {
		t.Fatalf("unexpected requirement error: %v", err)
	}
	if err := ValidateCapabilityRequirements(nil, available); err != nil {
		t.Fatalf("empty requirements should be valid before a manifest is attached: %v", err)
	}
}

func TestTransformRequiresTypedScope(t *testing.T) {
	err := ValidateTransformDescriptor(TransformDescriptor{
		ID: "tags.production", Provider: "aws", Runtime: "ecs-fargate",
		Components: []ComponentID{"runtime.web", "runtime.web"},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate component ID") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExistingResourceReferenceValidation(t *testing.T) {
	valid := ExistingResourceRef{ID: "database.primary", Provider: "aws", Kind: ExistingDatabase, ExternalID: "arn:example"}
	if err := ValidateExistingResourceRef(valid); err != nil {
		t.Fatal(err)
	}
	valid.ExternalID = " "
	if err := ValidateExistingResourceRef(valid); err == nil || !strings.Contains(err.Error(), "external ID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLifecycleHookDescriptors(t *testing.T) {
	descriptors := []LifecycleHookDescriptor{
		{ID: "vendor.prepare", Phase: PhaseDeploy, Relationship: HookBefore, RelativeTo: "deploy.upgrade", Timeout: 60, MaxAttempts: 1},
		{ID: "vendor.warm", Phase: PhasePostDeploy, Relationship: HookAfter, RelativeTo: "post-deploy.smoke", DependsOn: []HookID{"vendor.prepare"}, Timeout: 30, MaxAttempts: 2, Idempotent: true},
	}
	if err := ValidateLifecycleHooks(descriptors); err != nil {
		t.Fatal(err)
	}
}

func TestLifecycleHookRejectsCyclesAndUnsafeRetries(t *testing.T) {
	err := ValidateLifecycleHooks([]LifecycleHookDescriptor{
		{ID: "vendor.first", Phase: PhaseDeploy, Relationship: HookBefore, RelativeTo: "deploy.upgrade", DependsOn: []HookID{"vendor.second"}, Timeout: 60, MaxAttempts: 2},
		{ID: "vendor.second", Phase: PhaseDeploy, Relationship: HookAfter, RelativeTo: "deploy.upgrade", DependsOn: []HookID{"vendor.first"}, Timeout: 60, MaxAttempts: 1},
	})
	if err == nil || !strings.Contains(err.Error(), "only idempotent") || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("unexpected error: %v", err)
	}
}
