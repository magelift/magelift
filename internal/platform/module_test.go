package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	sdk "github.com/magelift/magelift/sdk/v1"
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

type fakePlannedStack struct {
	id       sdk.TargetID
	provider sdk.ProviderID
	runtime  sdk.RuntimeID
}

func (p fakePlannedStack) StackName() string        { return "test-stack" }
func (p fakePlannedStack) Provider() sdk.ProviderID { return p.provider }
func (p fakePlannedStack) Runtime() sdk.RuntimeID   { return p.runtime }
func (p fakePlannedStack) Project() string          { return "test-project" }
func (p fakePlannedStack) Environment() string      { return "test" }
func (p fakePlannedStack) Region() string           { return "test-region" }
func (p fakePlannedStack) CertificationTier() CertificationTier {
	return TierExperimental
}
func (p fakePlannedStack) EnvironmentClass() string { return "preview" }
func (p fakePlannedStack) Protected() bool          { return false }
func (p fakePlannedStack) ImageDigest() string      { return "" }
func (p fakePlannedStack) TargetDescriptor() sdk.TargetDescriptor {
	id := p.id
	if id == "" {
		id = "test.target"
	}
	return sdk.TargetDescriptor{ID: id, Provider: p.provider, Runtime: p.runtime}
}
func (p fakePlannedStack) WithImageDigest(string) (PlannedStack, error) {
	return p, nil
}

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

func TestRegisterModuleValidatesOptionalResilienceAdapter(t *testing.T) {
	valid := resilienceModule{
		fakeModule: fakeModule{
			id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierCertified,
			keys: RequiredOutputKeys(),
		},
		adapter: resilienceStub{descriptor: sdk.ResilienceAdapterDescriptor{
			APIVersion:   sdk.ExtensionAPIVersion,
			ID:           "aws.ecs-resilience",
			Provider:     "aws",
			Version:      "1.0.0",
			Capabilities: []sdk.ResilienceAction{sdk.ResilienceBackup, sdk.ResilienceRestore},
			DataClasses: []sdk.ResilienceDataClassCapability{{
				Name:            "database",
				Status:          sdk.ResilienceCapabilityExperimental,
				Actions:         []sdk.ResilienceAction{sdk.ResilienceBackup},
				PollingRequired: true,
			}},
		}},
	}
	if err := NewModuleRegistry().RegisterModule(valid); err != nil {
		t.Fatalf("valid resilience adapter was rejected: %v", err)
	}

	invalid := valid
	invalid.adapter = resilienceStub{descriptor: sdk.ResilienceAdapterDescriptor{
		APIVersion:   sdk.ExtensionAPIVersion,
		ID:           "gcp.resilience",
		Provider:     "gcp",
		Version:      "1.0.0",
		Capabilities: []sdk.ResilienceAction{sdk.ResilienceBackup},
		DataClasses: []sdk.ResilienceDataClassCapability{{
			Name:            "database",
			Status:          sdk.ResilienceCapabilitySupported,
			Actions:         []sdk.ResilienceAction{sdk.ResilienceBackup},
			PollingRequired: true,
		}},
	}}
	err := NewModuleRegistry().RegisterModule(invalid)
	if err == nil || !strings.Contains(err.Error(), "does not match target provider") {
		t.Fatalf("provider mismatch error = %v", err)
	}
}

func TestRegisterModuleValidatesOptionalEdgeAndObservabilityAdapters(t *testing.T) {
	valid := integrationModule{
		fakeModule:    fakeModule{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierCertified, keys: RequiredOutputKeys()},
		edge:          integrationStub{edge: true, edgeDescriptor: sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "aws.edge", Provider: "aws", Version: "1.0.0", Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy}}},
		observability: integrationStub{observability: true, observabilityDescriptor: sdk.ObservabilityAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "aws.observability", Provider: "aws", Version: "1.0.0", Capabilities: []sdk.ObservabilityAction{sdk.ObservabilityApply, sdk.ObservabilityVerify, sdk.ObservabilityDestroy}}},
	}
	if err := NewModuleRegistry().RegisterModule(valid); err != nil {
		t.Fatalf("valid edge and observability adapters were rejected: %v", err)
	}
	invalid := valid
	invalid.edge = integrationStub{edge: true, edgeDescriptor: sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "gcp.edge", Provider: "gcp", Version: "1.0.0", Capabilities: []sdk.EdgeAction{sdk.EdgeApply}}}
	err := NewModuleRegistry().RegisterModule(invalid)
	if err == nil || !strings.Contains(err.Error(), "edge adapter provider") {
		t.Fatalf("edge provider mismatch error = %v", err)
	}
}

func TestPlannedLifecycleFactoriesUseTheProviderNeutralPorts(t *testing.T) {
	module := lifecycleFactoryModule{
		fakeModule: fakeModule{
			id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierCertified,
			keys: RequiredOutputKeys(),
		},
		resilience: resilienceStub{descriptor: validResilienceDescriptor("aws")},
		edge:       integrationStub{edgeDescriptor: validEdgeDescriptor("aws")},
		observability: integrationStub{
			observabilityDescriptor: validObservabilityDescriptor("aws"),
		},
		collector: collectorDeploymentStub{},
	}
	planned := fakePlannedStack{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate"}

	resilience, err := ModuleResilienceFor(context.Background(), &module, planned)
	if err != nil || resilience == nil {
		t.Fatalf("ModuleResilienceFor() = %v, %v", resilience, err)
	}
	edge, err := ModuleEdgeFor(context.Background(), &module, planned)
	if err != nil || edge == nil {
		t.Fatalf("ModuleEdgeFor() = %v, %v", edge, err)
	}
	observability, err := ModuleObservabilityFor(context.Background(), &module, planned)
	if err != nil || observability == nil {
		t.Fatalf("ModuleObservabilityFor() = %v, %v", observability, err)
	}
	collector, err := ModuleCollectorDeploymentFor(context.Background(), &module, planned)
	if err != nil || collector == nil {
		t.Fatalf("ModuleCollectorDeploymentFor() = %v, %v", collector, err)
	}
	if module.resilienceCalls != 1 || module.edgeCalls != 1 || module.observabilityCalls != 1 || module.collectorCalls != 1 {
		t.Fatalf("factory calls = resilience:%d edge:%d observability:%d collector:%d", module.resilienceCalls, module.edgeCalls, module.observabilityCalls, module.collectorCalls)
	}
}

type collectorDeploymentStub struct{}

func (collectorDeploymentStub) PlanCollector(context.Context, sdk.CollectorDeploymentPlanRequest) (sdk.CollectorDeploymentPlan, error) {
	return sdk.CollectorDeploymentPlan{}, nil
}

func (collectorDeploymentStub) ExecuteCollector(context.Context, sdk.CollectorDeploymentExecutionRequest) (sdk.CollectorDeploymentExecutionResult, error) {
	return sdk.CollectorDeploymentExecutionResult{}, nil
}

func TestPlannedLifecycleFactoriesRejectInvalidInputsAndProviderIdentity(t *testing.T) {
	module := lifecycleFactoryModule{fakeModule: fakeModule{
		id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierCertified,
		keys: RequiredOutputKeys(),
	}, resilience: resilienceStub{descriptor: validResilienceDescriptor("aws")}}
	planned := fakePlannedStack{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate"}

	var nilContext context.Context
	if _, err := ModuleResilienceFor(nilContext, &module, planned); err == nil || !strings.Contains(err.Error(), "context") {
		t.Fatalf("nil context error = %v", err)
	}
	if _, err := ModuleResilienceFor(context.Background(), &module, nil); err == nil || !strings.Contains(err.Error(), "planned stack") {
		t.Fatalf("nil planned stack error = %v", err)
	}
	if _, err := ModuleResilienceFor(context.Background(), &module, fakePlannedStack{id: "aws.ecs-fargate", provider: "gcp", runtime: "ecs-fargate"}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("provider mismatch error = %v", err)
	}
	if _, err := ModuleResilienceFor(context.Background(), &module, fakePlannedStack{id: "aws.ecs-fargate", provider: "aws", runtime: "gke"}); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("runtime mismatch error = %v", err)
	}

	module.resilience = resilienceStub{descriptor: validResilienceDescriptor("gcp")}
	if _, err := ModuleResilienceFor(context.Background(), &module, planned); err == nil || !strings.Contains(err.Error(), "adapter provider") {
		t.Fatalf("adapter provider mismatch error = %v", err)
	}
}

func TestModuleRegistryExposesFirstPartyExtensionProvenance(t *testing.T) {
	registry := NewModuleRegistry()
	module := fakeModule{
		id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierCertified,
		keys: RequiredOutputKeys(),
	}
	if err := registry.RegisterModule(module); err != nil {
		t.Fatal(err)
	}
	extensions := registry.Extensions()
	if len(extensions) != 1 {
		t.Fatalf("extension count = %d", len(extensions))
	}
	if extensions[0].ID != "magelift.aws.ecs-fargate" || extensions[0].Source != "magelift" || extensions[0].Build != "first-party" {
		t.Fatalf("extension = %#v", extensions[0])
	}
}

type metadataExtension struct{}

type resilienceModule struct {
	fakeModule
	adapter sdk.ResilienceAdapter
}

func (m resilienceModule) Resilience() sdk.ResilienceAdapter { return m.adapter }

type resilienceStub struct {
	descriptor sdk.ResilienceAdapterDescriptor
}

type integrationModule struct {
	fakeModule
	edge          sdk.EdgeAdapter
	observability sdk.ObservabilityAdapter
}

type lifecycleFactoryModule struct {
	fakeModule
	resilience                                     sdk.ResilienceAdapter
	edge                                           sdk.EdgeAdapter
	observability                                  sdk.ObservabilityAdapter
	collector                                      sdk.CollectorDeploymentAdapter
	resilienceCalls, edgeCalls, observabilityCalls int
	collectorCalls                                 int
}

func (m *lifecycleFactoryModule) NewResilience(context.Context, PlannedStack) (sdk.ResilienceAdapter, error) {
	m.resilienceCalls++
	return m.resilience, nil
}

func (m *lifecycleFactoryModule) NewEdge(context.Context, PlannedStack) (sdk.EdgeAdapter, error) {
	m.edgeCalls++
	return m.edge, nil
}

func (m *lifecycleFactoryModule) NewObservability(context.Context, PlannedStack) (sdk.ObservabilityAdapter, error) {
	m.observabilityCalls++
	return m.observability, nil
}

func (m *lifecycleFactoryModule) NewCollectorDeployment(context.Context, PlannedStack) (sdk.CollectorDeploymentAdapter, error) {
	m.collectorCalls++
	return m.collector, nil
}

func (m integrationModule) Edge() sdk.EdgeAdapter { return m.edge }

func (m integrationModule) Observability() sdk.ObservabilityAdapter { return m.observability }

type integrationStub struct {
	edge                    bool
	observability           bool
	edgeDescriptor          sdk.EdgeAdapterDescriptor
	observabilityDescriptor sdk.ObservabilityAdapterDescriptor
}

func validResilienceDescriptor(provider sdk.ProviderID) sdk.ResilienceAdapterDescriptor {
	return sdk.ResilienceAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: string(provider) + ".resilience", Provider: provider, Version: "1.0.0",
		Capabilities: []sdk.ResilienceAction{sdk.ResilienceBackup},
		DataClasses: []sdk.ResilienceDataClassCapability{{
			Name: "database", Status: sdk.ResilienceCapabilitySupported,
			Actions: []sdk.ResilienceAction{sdk.ResilienceBackup}, PollingRequired: true,
		}},
	}
}

func validEdgeDescriptor(provider sdk.ProviderID) sdk.EdgeAdapterDescriptor {
	return sdk.EdgeAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: string(provider) + ".edge", Provider: provider, Version: "1.0.0",
		Capabilities: []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeDestroy},
	}
}

func validObservabilityDescriptor(provider sdk.ProviderID) sdk.ObservabilityAdapterDescriptor {
	return sdk.ObservabilityAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: string(provider) + ".observability", Provider: provider, Version: "1.0.0",
		Capabilities: []sdk.ObservabilityAction{sdk.ObservabilityApply, sdk.ObservabilityVerify, sdk.ObservabilityDestroy},
	}
}

func (s integrationStub) EdgeDescriptor() sdk.EdgeAdapterDescriptor {
	return s.edgeDescriptor
}

func (s integrationStub) PlanEdge(context.Context, sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	return sdk.EdgePlan{}, nil
}

func (s integrationStub) ExecuteEdge(context.Context, sdk.EdgeExecutionRequest) (sdk.EdgeExecutionResult, error) {
	return sdk.EdgeExecutionResult{}, nil
}

func (s integrationStub) ObservabilityDescriptor() sdk.ObservabilityAdapterDescriptor {
	return s.observabilityDescriptor
}

func (s integrationStub) PlanObservability(context.Context, sdk.ObservabilityPlanRequest) (sdk.ObservabilityPlan, error) {
	return sdk.ObservabilityPlan{}, nil
}

func (s integrationStub) ExecuteObservability(context.Context, sdk.ObservabilityExecutionRequest) (sdk.ObservabilityExecutionResult, error) {
	return sdk.ObservabilityExecutionResult{}, nil
}

func (s resilienceStub) ResilienceDescriptor() sdk.ResilienceAdapterDescriptor {
	return s.descriptor
}

func (s resilienceStub) PlanResilience(context.Context, sdk.ResiliencePlanRequest) (sdk.ResiliencePlan, error) {
	return sdk.ResiliencePlan{}, nil
}

func (s resilienceStub) ExecuteResilience(context.Context, sdk.ResilienceExecutionRequest) (sdk.ResilienceExecutionResult, error) {
	return sdk.ResilienceExecutionResult{}, nil
}

func (metadataExtension) Descriptor() sdk.ExtensionDescriptor {
	return sdk.ExtensionDescriptor{
		APIVersion: sdk.ExtensionAPIVersion,
		ID:         "vendor.kubernetes",
		Version:    "1.0.0",
		Source:     "github.com/vendor/magelift-kubernetes",
		Tier:       sdk.ExtensionTierExperimental,
		Targets: []sdk.TargetDescriptor{{
			ID: "vendor.kubernetes", Provider: "vendor", Runtime: "kubernetes",
		}},
		OutputKeys: sdk.CoreOutputKeys(),
	}
}

func TestModuleRegistryRequiresExplicitExtensionRegistration(t *testing.T) {
	registry := NewModuleRegistry()
	if err := registry.RegisterExtension(metadataExtension{}); err != nil {
		t.Fatal(err)
	}
	if len(registry.Extensions()) != 1 {
		t.Fatal("extension metadata was not registered")
	}
	if _, found := registry.Module("vendor", "kubernetes"); found {
		t.Fatal("metadata registration unexpectedly created a deployable module")
	}
}

func TestCoreEnvBindingsWriteMagentoRuntimeOverlays(t *testing.T) {
	bindings := CoreEnvBindings(CapabilityEndpoints{
		ApplicationMode: "headless", WebRuntime: "nginx-fpm",
		DatabaseWriter: "db.internal", DatabaseName: "magento",
		CacheEndpoint: "cache.internal",
		Magento: MagentoOverlays{
			FrontName:        "backend",
			CookieDomain:     ".shop.test",
			UnsecureBaseURL:  "http://api.shop.test/",
			SecureBaseURL:    "https://api.shop.test/",
			CORSOrigins:      []string{"https://storefront.example"},
			StorefrontOrigin: "https://www.shop.test",
			ConsumersMode:    "processes",
			ConsumerNames:    []string{"product_action_attribute.update"},
			Variables:        map[string]string{"CONFIG__DEFAULT__GENERAL__STORE_INFORMATION__NAME": "Shop"},
		},
	})
	found := map[string]string{}
	for _, binding := range bindings {
		found[binding.Name] = binding.Value
	}
	if found[EnvMagentoFrontName] != "backend" || found[EnvMagentoCookieDomain] != ".shop.test" {
		t.Fatalf("scalar Magento overlays = %#v", found)
	}
	if found[EnvMagentoCORSOrigins] != "https://storefront.example,https://www.shop.test" {
		t.Fatalf("CORS origins = %q", found[EnvMagentoCORSOrigins])
	}
	if found[EnvMagentoCronConsumersRun] != "0" || found[EnvMagentoConsumerNameList] != "product_action_attribute.update" {
		t.Fatalf("consumer overlays = %#v", found)
	}
	if found["CONFIG__DEFAULT__GENERAL__STORE_INFORMATION__NAME"] != "Shop" {
		t.Fatalf("CONFIG overlay missing: %#v", found)
	}
	if !strings.Contains(found[EnvMagentoDCOverride], `"frontName":"backend"`) {
		t.Fatalf("override missing frontName: %s", found[EnvMagentoDCOverride])
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
	if found[EnvMagentoInstallDate] != DefaultMagentoInstallDate {
		t.Fatalf("install date binding = %q, want %q", found[EnvMagentoInstallDate], DefaultMagentoInstallDate)
	}
	if found[EnvMagentoQueueDefault] != "db" {
		t.Fatalf("database queue connection = %q, want db", found[EnvMagentoQueueDefault])
	}
}

func TestCoreEnvBindingsSelectsAMQPForRabbitMQ(t *testing.T) {
	bindings := CoreEnvBindings(CapabilityEndpoints{
		ApplicationMode: "integrated", WebRuntime: "nginx-fpm",
		DatabaseWriter: "db.internal", DatabaseName: "magento",
		CacheEndpoint: "cache.internal", QueueMode: "rabbitmq", QueueHost: "rabbitmq.internal",
	})
	found := map[string]string{}
	for _, binding := range bindings {
		found[binding.Name] = binding.Value
	}
	if found[EnvMagentoQueueDefault] != "amqp" {
		t.Fatalf("RabbitMQ queue connection = %q, want amqp", found[EnvMagentoQueueDefault])
	}
}

func TestCoreEnvBindingsConfigureElasticsuite(t *testing.T) {
	bindings := CoreEnvBindings(CapabilityEndpoints{
		ApplicationMode: "integrated", WebRuntime: "nginx-fpm",
		ApplicationVersion: "2.4.9", SearchEndpoint: "search.internal",
	})
	found := map[string]string{}
	for _, binding := range bindings {
		found[binding.Name] = binding.Value
	}
	if found[EnvMagentoSearchHost] != "search.internal" || found[EnvMagentoSearchPort] != "9200" {
		t.Fatalf("Magento search host/port = %q:%q", found[EnvMagentoSearchHost], found[EnvMagentoSearchPort])
	}
	if found[EnvMagentoElasticsuiteServers] != "search.internal:9200" {
		t.Fatalf("Elasticsuite servers = %q", found[EnvMagentoElasticsuiteServers])
	}
	wantOverride := `{"system":{"default":{"smile_elasticsuite_core_base_settings":{"es_client":{"servers":"search.internal:9200","enable_https_mode":0,"enable_http_auth":0}}}}}`
	if found[EnvMagentoDCOverride] != wantOverride {
		t.Fatalf("Magento deployment override = %q, want %q", found[EnvMagentoDCOverride], wantOverride)
	}
}

func TestCoreEnvBindingsAWSOpenSearchUsesHTTPS443(t *testing.T) {
	bindings := CoreEnvBindings(CapabilityEndpoints{
		ApplicationMode: "integrated", WebRuntime: "nginx-fpm",
		SearchEndpoint: "https://vpc-shop.eu-west-3.es.amazonaws.com",
	})
	found := map[string]string{}
	for _, binding := range bindings {
		found[binding.Name] = binding.Value
	}
	if found[EnvMagentoSearchHost] != "vpc-shop.eu-west-3.es.amazonaws.com" || found[EnvMagentoSearchPort] != "443" {
		t.Fatalf("AWS OpenSearch host/port = %q:%q", found[EnvMagentoSearchHost], found[EnvMagentoSearchPort])
	}
	if found[EnvMagentoSearchEnableAuth] != "0" || found[EnvMagentoElasticsuiteHTTPS] != "1" {
		t.Fatalf("AWS OpenSearch auth/https = %q/%q", found[EnvMagentoSearchEnableAuth], found[EnvMagentoElasticsuiteHTTPS])
	}
	if found[EnvMagentoElasticsuiteServers] != "vpc-shop.eu-west-3.es.amazonaws.com:443" {
		t.Fatalf("Elasticsuite servers = %q", found[EnvMagentoElasticsuiteServers])
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

func TestCoreEnvBindingsUsesValkeyForAdobeLatestPatches(t *testing.T) {
	for _, version := range []string{"2.4.6-p15", "2.4.7-p10", "2.4.8-p5", "2.4.9"} {
		bindings := CoreEnvBindings(CapabilityEndpoints{
			ApplicationVersion: version,
			CacheEndpoint:      "cache.internal",
		})
		found := map[string]string{}
		for _, binding := range bindings {
			found[binding.Name] = binding.Value
		}
		if found[EnvMagentoCacheBackend] != ValkeyCacheBackend || found[EnvMagentoPageCacheBackend] != ValkeyCacheBackend || found[EnvMagentoSessionSave] != "valkey" {
			t.Fatalf("%s cache bindings = %#v", version, found)
		}
	}
}

func TestCoreEnvBindingsKeepsLegacyRedisBeforeValkeyCutover(t *testing.T) {
	for _, version := range []string{"2.4.6-p14", "2.4.7-p9", "2.4.8-p4", "2.4.8"} {
		bindings := CoreEnvBindings(CapabilityEndpoints{
			ApplicationVersion: version,
			CacheEndpoint:      "cache.internal",
		})
		found := map[string]string{}
		for _, binding := range bindings {
			found[binding.Name] = binding.Value
		}
		if found[EnvMagentoCacheBackend] != RedisCacheBackend || found[EnvMagentoPageCacheBackend] != RedisCacheBackend || found[EnvMagentoSessionSave] != "redis" {
			t.Fatalf("%s cache bindings = %#v", version, found)
		}
	}
}
