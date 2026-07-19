package platform

import (
	"testing"

	"github.com/acourtiol/magelift/internal/config"
	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type fakeModule struct {
	id       sdk.TargetID
	provider sdk.ProviderID
	runtime  sdk.RuntimeID
	tier     CertificationTier
	keys     []string
}

func (m fakeModule) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: m.id, Provider: m.provider, Runtime: m.runtime}
}
func (m fakeModule) CertificationTier() CertificationTier { return m.tier }
func (m fakeModule) Plan(config.Config, string, PlanOptions) (PlannedStack, error) {
	return nil, nil
}
func (m fakeModule) Program(PlannedStack) (pulumi.RunFunc, error) { return nil, nil }
func (m fakeModule) OutputKeys() []string                         { return m.keys }

func TestRegisterModuleRequiresOutputKeys(t *testing.T) {
	registry := NewModuleRegistry()
	err := registry.RegisterModule(fakeModule{
		id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierCertified,
		keys: []string{OutputApplicationURL},
	})
	if err == nil {
		t.Fatal("expected missing output keys to fail registration")
	}
}

func TestRegisterModuleRejectsDuplicates(t *testing.T) {
	registry := NewModuleRegistry()
	module := fakeModule{
		id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierCertified,
		keys: RequiredOutputKeys(),
	}
	if err := registry.RegisterModule(module); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterModule(module); err == nil {
		t.Fatal("expected duplicate registration to fail")
	}
}

func TestCoreEnvBindingsIncludeMagentoContract(t *testing.T) {
	bindings := CoreEnvBindings(CapabilityEndpoints{
		ApplicationMode: "integrated", WebRuntime: "nginx-fpm",
		DatabaseWriter: "db.internal", DatabaseName: "magento",
		CacheEndpoint: "cache.internal", SessionEndpoint: "session.internal",
	})
	found := map[string]string{}
	for _, binding := range bindings {
		found[binding.Name] = binding.Value
	}
	if found[EnvMagentoDBHost] != "db.internal" || found[EnvMagentoSessionHost] != "session.internal" {
		t.Fatalf("unexpected Magento bindings: %#v", found)
	}
}

func TestCoreEnvBindingsFallsBackSessionToCache(t *testing.T) {
	bindings := CoreEnvBindings(CapabilityEndpoints{
		ApplicationMode: "integrated", WebRuntime: "nginx-fpm",
		DatabaseWriter: "db.internal", DatabaseName: "magento",
		CacheEndpoint: "cache.internal",
	})
	found := map[string]string{}
	for _, binding := range bindings {
		found[binding.Name] = binding.Value
	}
	if found[EnvSessionEndpoint] != "cache.internal" || found[EnvMagentoSessionHost] != "cache.internal" {
		t.Fatalf("session endpoint should fall back to cache: %#v", found)
	}
}
