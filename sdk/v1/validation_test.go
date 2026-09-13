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

func TestValidateExternalIntentRequiresReferenceShapeAndLifecycle(t *testing.T) {
	if err := ValidateExternalIntent("fastly", ExternalLifecycleExtension, ExternalExperimental, []string{"aws-secrets-manager://magelift/fastly-token"}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExternalIntent("fastly", "unknown", ExternalExperimental, nil); err == nil || !strings.Contains(err.Error(), "lifecycle") {
		t.Fatalf("invalid lifecycle was accepted: %v", err)
	}
	if err := ValidateExternalIntent("fastly", ExternalLifecycleExtension, ExternalExperimental, []string{"raw-token"}); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("plaintext credential was accepted: %v", err)
	}
	for _, reference := range []string{"https://example.invalid/token", "token://embedded/value", "secret://user:password@example.invalid/name", "secret://example.invalid/name#value"} {
		if err := ValidateCredentialReference(reference); err == nil {
			t.Fatalf("unsafe credential reference %q was accepted", reference)
		}
	}
	for _, reference := range []string{"aws-secrets-manager://magelift/fastly-token", "gcp-secret-manager://projects/demo/secrets/newrelic", "vault://magelift/observability/newrelic"} {
		if err := ValidateCredentialReference(reference); err != nil {
			t.Fatalf("valid credential reference %q rejected: %v", reference, err)
		}
	}
	if err := ValidateCredentialReference("kubernetes-secret://magelift/new-relic#license-key"); err != nil {
		t.Fatalf("Kubernetes Secret key selector was rejected: %v", err)
	}
	if err := ValidateExternalIntent("", ExternalLifecycleExtension, "", nil); err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("metadata without provider was accepted: %v", err)
	}
}

func TestValidatesExtensionDescriptorAndCopiesMetadata(t *testing.T) {
	descriptor := ExtensionDescriptor{
		APIVersion: ExtensionAPIVersion,
		ID:         "vendor.kubernetes",
		Version:    "1.0.0",
		Source:     "github.com/vendor/magelift-kubernetes",
		Digest:     "sha256:0123456789abcdef",
		Tier:       ExtensionTierExperimental,
		Targets: []TargetDescriptor{{
			ID: "vendor.kubernetes", Provider: "vendor", Runtime: "kubernetes",
		}},
		Capabilities: []CapabilityDescriptor{{
			ID: "vendor.mysql", Capability: CapabilityDatabaseMySQL,
			Kind: CapabilityDatabase, Provider: "vendor",
		}},
		OutputKeys: CoreOutputKeys(),
	}
	if err := ValidateExtensionDescriptor(descriptor); err != nil {
		t.Fatal(err)
	}
	descriptor.OutputKeys[0] = "changed"
	if CoreOutputKeys()[0] != "applicationURL" {
		t.Fatal("core output contract is mutable")
	}
}

func TestRejectsExtensionWithUnsupportedAPIOrMissingOutput(t *testing.T) {
	err := ValidateExtensionDescriptor(ExtensionDescriptor{
		APIVersion: "v2",
		ID:         "vendor.kubernetes",
		Version:    "1.0.0",
		Source:     "vendor",
		Tier:       ExtensionTierExperimental,
		Targets:    []TargetDescriptor{{ID: "vendor.kubernetes", Provider: "vendor", Runtime: "kubernetes"}},
		OutputKeys: []string{"applicationURL"},
	})
	if err == nil || !strings.Contains(err.Error(), "API version") || !strings.Contains(err.Error(), "omits required output key") {
		t.Fatalf("unexpected extension validation error: %v", err)
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
