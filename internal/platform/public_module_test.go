package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type publicModuleStub struct{}

func (publicModuleStub) Descriptor() sdk.ExtensionDescriptor {
	return sdk.ExtensionDescriptor{
		APIVersion: sdk.ExtensionAPIVersion,
		ID:         "vendor.example",
		Version:    "1.2.3",
		Source:     "example.com/vendor/magelift-provider",
		Build:      "test",
		Tier:       sdk.ExtensionTierExperimental,
		Targets: []sdk.TargetDescriptor{{
			ID: "vendor.example", Provider: "vendor", Runtime: "example",
		}},
		OutputKeys: sdk.CoreOutputKeys(),
	}
}

func (publicModuleStub) Plan(_ context.Context, request sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
	target := publicModuleStub{}.Descriptor().Targets[0]
	return sdk.ModulePlan{
		StackName:        request.Project + "-" + request.Environment,
		Provider:         target.Provider,
		Runtime:          target.Runtime,
		Project:          request.Project,
		Environment:      request.Environment,
		Region:           request.Region,
		EnvironmentClass: request.EnvironmentClass,
		Protected:        request.Protected,
		ImageDigest:      request.Artifact.ImageDigest,
		Target:           target,
		Tier:             publicModuleStub{}.Descriptor().Tier,
		Opaque:           request.Configuration,
	}, nil
}

func (publicModuleStub) Program(sdk.ModulePlan) (any, error) {
	return pulumi.RunFunc(func(*pulumi.Context) error { return nil }), nil
}

func TestRegisterPublicModuleAdaptsPlanAndProgram(t *testing.T) {
	registry := NewModuleRegistry()
	if err := registry.RegisterPublicModule(publicModuleStub{}); err != nil {
		t.Fatal(err)
	}
	module, ok := registry.Module("vendor", "example")
	if !ok {
		t.Fatal("public module was not registered")
	}
	planned, err := module.Plan(config.Config{
		Project:     config.Project{Name: "shop"},
		Application: config.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated"},
		Defaults:    config.Defaults{Region: "eu-west-3"},
		Target:      config.Target{Provider: "vendor", Runtime: "example"},
		Class:       "preview",
	}, "staging", PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if planned.StackName() != "shop-staging" || planned.Region() != "eu-west-3" || planned.CertificationTier() != TierExperimental {
		t.Fatalf("planned stack = %#v", planned)
	}
	planned, err = planned.WithImageDigest("registry.example/shop@sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if planned.ImageDigest() == "" {
		t.Fatal("digest override was not passed through the public plan")
	}
	program, err := module.Program(planned)
	if err != nil || program == nil {
		t.Fatalf("program = %v, %v", program, err)
	}
}

type publicModuleWrongRegion struct{ publicModuleStub }

func (publicModuleWrongRegion) Plan(_ context.Context, request sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
	plan, err := publicModuleStub{}.Plan(context.Background(), request)
	if err != nil {
		return sdk.ModulePlan{}, err
	}
	plan.Region = "us-east-1"
	return plan, nil
}

func TestPublicPlanRejectsIdentityMismatch(t *testing.T) {
	registry := NewModuleRegistry()
	if err := registry.RegisterPublicModule(publicModuleWrongRegion{}); err != nil {
		t.Fatal(err)
	}
	module, ok := registry.Module("vendor", "example")
	if !ok {
		t.Fatal("public module was not registered")
	}
	_, err := module.Plan(config.Config{
		Project:     config.Project{Name: "shop"},
		Application: config.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated"},
		Defaults:    config.Defaults{Region: "eu-west-3"},
		Target:      config.Target{Provider: "vendor", Runtime: "example"},
		Class:       "preview",
	}, "staging", PlanOptions{})
	if err == nil || !strings.Contains(err.Error(), "region") {
		t.Fatalf("error = %v, want region mismatch", err)
	}
}

func TestArchitectureNetworkProfileCapturesResolvedAWSEgressBoundary(t *testing.T) {
	tests := []struct {
		name string
		cfg  config.Config
		want string
	}{
		{
			name: "preview default",
			cfg:  config.Config{Defaults: config.Defaults{Preset: "preview"}, Target: config.Target{Provider: "aws", AWS: &config.AWSTarget{}}},
			want: "private-nat-gateway-single-az-none",
		},
		{
			name: "fck nat multi az replacement",
			cfg:  config.Config{Defaults: config.Defaults{Preset: "standard"}, Target: config.Target{Provider: "aws", AWS: &config.AWSTarget{NatMode: "fck-nat", NatTopology: "multi-az", NatReplacementMode: "auto-scaling"}}},
			want: "private-fck-nat-multi-az-auto-scaling-arm64-t4g-nano",
		},
		{
			name: "existing network",
			cfg:  config.Config{Target: config.Target{Provider: "aws", AWS: &config.AWSTarget{Existing: config.AWSExistingResources{Network: &config.AWSExistingResource{}}}}},
			want: "private-existing-network",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := architectureNetworkProfile(test.cfg); got != test.want {
				t.Fatalf("network profile = %q, want %q", got, test.want)
			}
		})
	}
}

type publicLifecycleModule struct{ publicModuleStub }

type publicPlanAdmissionModule struct{ publicModuleStub }

type testPublicPlanAdmission struct{}

func (publicPlanAdmissionModule) NewPlanAdmission(context.Context, sdk.ModulePlan) (sdk.PlanAdmission, error) {
	return testPublicPlanAdmission{}, nil
}

func (testPublicPlanAdmission) Admit(_ context.Context, plan sdk.ModulePlan) (sdk.ModulePlan, error) {
	plan.Opaque = "admitted"
	return plan, nil
}

func TestPublicModuleBridgesPlanAdmission(t *testing.T) {
	module, err := newPublicModuleAdapter(publicPlanAdmissionModule{})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := module.Plan(config.Config{
		Project:     config.Project{Name: "shop"},
		Application: config.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated"},
		Defaults:    config.Defaults{Region: "eu-west-3"},
		Target:      config.Target{Provider: "vendor", Runtime: "example"},
		Class:       "preview",
	}, "staging", PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := AdmitPlan(context.Background(), module, planned)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := admitted.(publicPlannedStack)
	if !ok || value.plan.Opaque != "admitted" {
		t.Fatalf("admitted public plan = %#v", admitted)
	}
}

func (publicLifecycleModule) NewResilience(context.Context, sdk.ModulePlan) (sdk.ResilienceAdapter, error) {
	return resilienceStub{descriptor: validResilienceDescriptor("vendor")}, nil
}

func (publicLifecycleModule) NewEdge(context.Context, sdk.ModulePlan) (sdk.EdgeAdapter, error) {
	return integrationStub{edgeDescriptor: validEdgeDescriptor("vendor")}, nil
}

func (publicLifecycleModule) NewObservability(context.Context, sdk.ModulePlan) (sdk.ObservabilityAdapter, error) {
	return integrationStub{observabilityDescriptor: validObservabilityDescriptor("vendor")}, nil
}

func (publicLifecycleModule) NewCollectorDeployment(context.Context, sdk.ModulePlan) (sdk.CollectorDeploymentAdapter, error) {
	return collectorDeploymentStub{}, nil
}

func TestPublicModuleBridgesPlannedLifecycleFactories(t *testing.T) {
	module, err := newPublicModuleAdapter(publicLifecycleModule{})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := module.Plan(config.Config{
		Project:     config.Project{Name: "shop"},
		Application: config.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated"},
		Defaults:    config.Defaults{Region: "eu-west-3"},
		Target:      config.Target{Provider: "vendor", Runtime: "example"},
		Class:       "preview",
	}, "staging", PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if adapter, err := ModuleCollectorDeploymentFor(context.Background(), module, planned); err != nil || adapter == nil {
		t.Fatalf("bridged collector deployment adapter = %v, %v", adapter, err)
	}
	if adapter, err := ModuleResilienceFor(context.Background(), module, planned); err != nil || adapter == nil {
		t.Fatalf("bridged resilience adapter = %v, %v", adapter, err)
	}
	if adapter, err := ModuleEdgeFor(context.Background(), module, planned); err != nil || adapter == nil {
		t.Fatalf("bridged edge adapter = %v, %v", adapter, err)
	}
	if adapter, err := ModuleObservabilityFor(context.Background(), module, planned); err != nil || adapter == nil {
		t.Fatalf("bridged observability adapter = %v, %v", adapter, err)
	}
}

func TestPublicPlanRequestCarriesCrossCuttingIntent(t *testing.T) {
	request, err := publicPlanRequest(config.Config{
		Project:     config.Project{Name: "shop"},
		Application: config.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated"},
		Defaults:    config.Defaults{Region: "eu-west-3"},
		Edge: config.EdgeConfig{
			ExternalProvider: "fastly",
			ServiceID:        "service-123",
			TokenSecret:      "aws-secrets-manager://magelift/fastly-token",
			Domains:          []string{"shop.example.com"},
			TLS:              true,
			TLSMode:          "managed",
			DNSMode:          "external",
			OriginHealthRef:  "health/magento",
			PurgeOnDeploy:    true,
			VCLRef:           "artifacts/edge.vcl",
			Health:           &config.EdgeHealthConfig{OriginURL: "https://origin.example.com/health", ExpectedCNAME: "dualstack.e.sni.global.fastly.net", RoutePath: "/health", ExpectedStatus: 200, RouteTimeoutSeconds: 90, RoutePollSeconds: 2},
		},
		Observability: config.ObservabilityConfig{
			ExternalProvider: "datadog",
			NativeReference:  "native-destination-123",
			Endpoint:         "https://otlp.example.com",
			ServiceName:      "magento",
			Environment:      "staging",
			Logs:             true,
			Metrics:          true,
			Signals:          []string{"logs", "metrics"},
			Labels:           map[string]string{"team": "commerce"},
		},
		Resilience: config.ResilienceConfig{
			Projection: &config.ResilienceProjection{Runtime: "ecs", Cluster: "shop", Service: "web", Container: "php"},
		},
	}, "staging", PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if request.Edge.ExternalProvider != "fastly" || request.Edge.Lifecycle != sdk.ExternalLifecycleExtension || request.Edge.Certification != sdk.ExternalExperimental || len(request.Edge.CredentialRefs) != 1 || request.Edge.ServiceReference != "service-123" || !request.Edge.TLS || request.Edge.PolicyReference != "artifacts/edge.vcl" {
		t.Fatalf("edge intent = %#v", request.Edge)
	}
	if request.Edge.Health.OriginURL != "https://origin.example.com/health" || request.Edge.Health.ExpectedRouteTarget != "dualstack.e.sni.global.fastly.net" || request.Edge.Health.RouteTimeoutSeconds != 90 {
		t.Fatalf("edge health intent = %#v", request.Edge.Health)
	}
	if request.Observability.ExternalProvider != "datadog" || request.Observability.NativeReference != "native-destination-123" || request.Observability.Lifecycle != sdk.ExternalLifecycleExtension || request.Observability.Certification != sdk.ExternalExperimental || !request.Observability.Logs || request.Observability.Labels["team"] != "commerce" {
		t.Fatalf("observability intent = %#v", request.Observability)
	}
	if request.Resilience.Projection == nil || request.Resilience.Projection.Runtime != "ecs" || request.Resilience.Projection.Cluster != "shop" || request.Resilience.Projection.Service != "web" || request.Resilience.Projection.Container != "php" {
		t.Fatalf("projection intent = %#v", request.Resilience.Projection)
	}
}

func TestPublicPlanRequestPreservesAWSComputeMode(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
		mode    string
		want    string
	}{
		{name: "ecs default", runtime: "ecs-fargate", want: "fargate"},
		{name: "ecs fargate", runtime: "ecs-fargate", mode: "fargate", want: "fargate"},
		{name: "ecs fargate spot", runtime: "ecs-fargate", mode: "fargate-spot", want: "fargate-spot"},
		{name: "ecs ec2 asg", runtime: "ecs-fargate", mode: "ec2-asg", want: "ec2-asg"},
		{name: "ecs managed instances", runtime: "ecs-fargate", mode: "managed-instances", want: "managed-instances"},
		{name: "eks default", runtime: "eks", want: "auto-mode"},
		{name: "eks auto mode", runtime: "eks", mode: "auto-mode", want: "auto-mode"},
		{name: "eks managed node groups", runtime: "eks", mode: "managed-node-groups", want: "managed-node-groups"},
		{name: "eks self managed", runtime: "eks", mode: "self-managed", want: "self-managed"},
		{name: "eks fargate", runtime: "eks", mode: "fargate", want: "fargate"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := config.AWSCatalog{}
			switch test.runtime {
			case "ecs-fargate":
				catalog.Fargate.ComputeMode = test.mode
			case "eks":
				catalog.EKS.ComputeMode = test.mode
			default:
				t.Fatalf("unsupported test runtime %q", test.runtime)
			}

			request, err := publicPlanRequest(config.Config{
				Project:     config.Project{Name: "shop"},
				Application: config.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated"},
				Defaults:    config.Defaults{Region: "eu-west-3"},
				Target: config.Target{
					Provider: "aws",
					Runtime:  test.runtime,
					AWS:      &config.AWSTarget{Catalog: catalog},
				},
			}, "staging", PlanOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got := request.Architecture.ComputeMode; got != test.want {
				t.Fatalf("architecture compute mode = %q, want %q", got, test.want)
			}
		})
	}
}

type invalidProgramPublicModule struct{ publicModuleStub }

func (invalidProgramPublicModule) Program(sdk.ModulePlan) (any, error) {
	return "not a Pulumi program", nil
}

func TestPublicModuleRejectsNonPulumiProgram(t *testing.T) {
	registry := NewModuleRegistry()
	if err := registry.RegisterPublicModule(invalidProgramPublicModule{}); err != nil {
		t.Fatal(err)
	}
	module, ok := registry.Module("vendor", "example")
	if !ok {
		t.Fatal("public module was not registered")
	}
	planned, err := module.Plan(config.Config{
		Project:     config.Project{Name: "shop"},
		Application: config.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated"},
		Defaults:    config.Defaults{Region: "eu-west-3"},
		Target:      config.Target{Provider: "vendor", Runtime: "example"},
		Class:       "preview",
	}, "staging", PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.Program(planned); err == nil || !strings.Contains(err.Error(), "pulumi.RunFunc") {
		t.Fatalf("invalid program was accepted: %v", err)
	}
}

func TestRegisterPublicModuleRejectsMultipleTargets(t *testing.T) {
	module := publicModuleStub{}
	descriptor := module.Descriptor()
	descriptor.Targets = append(descriptor.Targets, descriptor.Targets[0])
	if err := (&multiTargetPublicModule{descriptor: descriptor}).validate(); err == nil {
		t.Fatal("expected multiple target module to be rejected")
	}
}

type multiTargetPublicModule struct {
	descriptor sdk.ExtensionDescriptor
}

func (m multiTargetPublicModule) Descriptor() sdk.ExtensionDescriptor { return m.descriptor }
func (m multiTargetPublicModule) Plan(context.Context, sdk.ModulePlanRequest) (sdk.ModulePlan, error) {
	return sdk.ModulePlan{}, nil
}
func (m multiTargetPublicModule) Program(sdk.ModulePlan) (any, error) {
	return nil, nil
}
func (m multiTargetPublicModule) validate() error {
	_, err := newPublicModuleAdapter(m)
	return err
}
