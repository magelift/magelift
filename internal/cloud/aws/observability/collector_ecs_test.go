package observability

import (
	"context"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestECSCollectorBackendAddsOwnedSecretReferenceSidecarAndRestoresService(t *testing.T) {
	api := newFakeECSCollectorAPI()
	adapter, err := NewECSContribCollectorDeploymentAdapter(api, ECSCollectorConfig{
		ServiceReference:      "cluster/web",
		ImageDigest:           "public.ecr.aws/example/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ConfigReference:       "env:MAGELIFT_OTEL_CONFIG",
		CredentialEnvironment: "OTEL_EXPORTER_API_KEY",
		Probe:                 fakeECSCollectorProbe{delivered: true},
		SecretResolver:        fakeECSSecretResolver{},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := ecsCollectorPlanRequest()
	plan, err := adapter.PlanCollector(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	execution := sdk.CollectorDeploymentExecutionRequest{Plan: plan, Action: sdk.CollectorApply, IdempotencyKey: "collector/apply/1", OwnershipMarker: plan.OwnershipMarker}
	result, err := adapter.ExecuteCollector(context.Background(), execution)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !result.OwnershipVerified || !result.ReadyVerified || !result.HealthVerified || api.registerCalls != 1 || api.updateCalls != 1 {
		t.Fatalf("apply result = %#v api = %#v", result, api)
	}
	registered := api.lastRegister
	if registered == nil || len(registered.ContainerDefinitions) != 2 {
		t.Fatalf("registered task definition = %#v", registered)
	}
	collector := registered.ContainerDefinitions[1]
	if len(collector.Secrets) != 1 || awssdk.ToString(collector.Secrets[0].ValueFrom) != "arn:aws:secretsmanager:eu-west-3:123456789012:secret:newrelic" {
		t.Fatalf("collector secrets = %#v", collector.Secrets)
	}
	if awssdk.ToString(collector.Secrets[0].Name) != "OTEL_EXPORTER_API_KEY" {
		t.Fatalf("collector credential environment = %q", awssdk.ToString(collector.Secrets[0].Name))
	}
	if collector.Environment[0].Value == nil || awssdk.ToString(collector.Environment[0].Value) != request.Endpoint {
		t.Fatalf("collector endpoint environment = %#v", collector.Environment)
	}
	configValue := ""
	for _, environment := range collector.Environment {
		if awssdk.ToString(environment.Name) == "MAGELIFT_OTEL_CONFIG" {
			configValue = awssdk.ToString(environment.Value)
		}
	}
	if !strings.Contains(configValue, "logs:\n") || !strings.Contains(configValue, "metrics:\n") || !strings.Contains(configValue, "traces:\n") {
		t.Fatalf("collector config environment = %q", configValue)
	}
	if strings.Contains(configValue, "secret-value") || strings.Contains(configValue, "api-key: ") && !strings.Contains(configValue, "api-key: ${OTEL_EXPORTER_API_KEY}") {
		t.Fatalf("collector config contains a credential value: %q", configValue)
	}
	if strings.Contains(awssdk.ToString(collector.Secrets[0].ValueFrom), "secret-value") {
		t.Fatal("secret value appeared in ECS task-definition input")
	}
	if tagValue(api.lastRegister.Tags, ecsCollectorPreviousTag) != api.initialTaskDefinition {
		t.Fatalf("previous task-definition tag = %q", tagValue(api.lastRegister.Tags, ecsCollectorPreviousTag))
	}

	repeated, err := adapter.ExecuteCollector(context.Background(), execution)
	if err != nil {
		t.Fatalf("repeat apply: %v", err)
	}
	if repeated.OperationID != result.OperationID || api.registerCalls != 1 {
		t.Fatalf("repeat result = %#v api = %#v", repeated, api)
	}

	destroy := execution
	destroy.Action = sdk.CollectorDestroy
	destroy.IdempotencyKey = "collector/destroy/1"
	destroyed, err := adapter.ExecuteCollector(context.Background(), destroy)
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if !destroyed.CleanupVerified || api.deregisterCalls != 1 || api.taskDefinition != api.initialTaskDefinition {
		t.Fatalf("destroyed = %#v api = %#v", destroyed, api)
	}
}

func TestECSCollectorBackendRollsBackAfterSignalProofFailure(t *testing.T) {
	api := newFakeECSCollectorAPI()
	adapter, err := NewECSContribCollectorDeploymentAdapter(api, ECSCollectorConfig{
		ServiceReference: "cluster/web", ImageDigest: "public.ecr.aws/example/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ConfigReference: "env:MAGELIFT_OTEL_CONFIG", Probe: fakeECSCollectorProbe{delivered: false},
		SecretResolver: fakeECSSecretResolver{},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.PlanCollector(context.Background(), ecsCollectorPlanRequest())
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.ExecuteCollector(context.Background(), sdk.CollectorDeploymentExecutionRequest{Plan: plan, Action: sdk.CollectorApply, IdempotencyKey: "collector/apply/fails", OwnershipMarker: plan.OwnershipMarker})
	if err == nil || !strings.Contains(err.Error(), "signal delivery") {
		t.Fatalf("error = %v, want signal-delivery failure", err)
	}
	if api.taskDefinition != api.initialTaskDefinition || api.deregisterCalls != 1 || api.updateCalls != 2 {
		t.Fatalf("rollback state api = %#v", api)
	}
}

func TestECSCollectorBackendRefusesForeignSidecarBeforeRegister(t *testing.T) {
	api := newFakeECSCollectorAPI()
	foreign := api.definitions[api.initialTaskDefinition].TaskDefinition
	foreign.ContainerDefinitions = append(foreign.ContainerDefinitions, ecstypes.ContainerDefinition{Name: awssdk.String(ecsCollectorContainerName + "foreign"), DockerLabels: map[string]string{ecsCollectorOwnershipKey: "other-marker"}})
	api.definitions[api.initialTaskDefinition].TaskDefinition = foreign
	adapter, err := NewECSContribCollectorDeploymentAdapter(api, ECSCollectorConfig{
		ServiceReference: "cluster/web", ImageDigest: "public.ecr.aws/example/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ConfigReference: "env:MAGELIFT_OTEL_CONFIG", Probe: fakeECSCollectorProbe{delivered: true},
		SecretResolver: fakeECSSecretResolver{},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.PlanCollector(context.Background(), ecsCollectorPlanRequest())
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.ExecuteCollector(context.Background(), sdk.CollectorDeploymentExecutionRequest{Plan: plan, Action: sdk.CollectorApply, IdempotencyKey: "collector/apply/foreign", OwnershipMarker: plan.OwnershipMarker})
	if err == nil || !strings.Contains(err.Error(), "unowned") {
		t.Fatalf("error = %v, want unowned refusal", err)
	}
	if api.registerCalls != 0 || api.updateCalls != 0 {
		t.Fatalf("foreign sidecar triggered mutation: %#v", api)
	}
}

func TestECSCollectorBackendRefusesMixedOwnedAndForeignSidecars(t *testing.T) {
	api := newFakeECSCollectorAPI()
	definition := api.definitions[api.initialTaskDefinition].TaskDefinition
	definition.ContainerDefinitions = append(definition.ContainerDefinitions,
		ecstypes.ContainerDefinition{Name: awssdk.String(ecsCollectorContainerName + "owned"), DockerLabels: map[string]string{ecsCollectorOwnershipKey: "magelift/test/ecs-collector"}},
		ecstypes.ContainerDefinition{Name: awssdk.String(ecsCollectorContainerName + "foreign"), DockerLabels: map[string]string{ecsCollectorOwnershipKey: "other-marker"}},
	)
	api.definitions[api.initialTaskDefinition].TaskDefinition = definition
	adapter, err := NewECSContribCollectorDeploymentAdapter(api, ECSCollectorConfig{
		ServiceReference: "cluster/web", ImageDigest: "public.ecr.aws/example/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ConfigReference: "env:MAGELIFT_OTEL_CONFIG", Probe: fakeECSCollectorProbe{delivered: true}, SecretResolver: fakeECSSecretResolver{},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.PlanCollector(context.Background(), ecsCollectorPlanRequest())
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.ExecuteCollector(context.Background(), sdk.CollectorDeploymentExecutionRequest{Plan: plan, Action: sdk.CollectorApply, IdempotencyKey: "collector/apply/mixed", OwnershipMarker: plan.OwnershipMarker})
	if err == nil || !strings.Contains(err.Error(), "unowned") {
		t.Fatalf("error = %v, want mixed-ownership refusal", err)
	}
	if api.registerCalls != 0 || api.updateCalls != 0 {
		t.Fatalf("mixed sidecars triggered mutation: %#v", api)
	}
}

func TestECSCollectorBackendRejectsStaleRunningTaskDefinition(t *testing.T) {
	api := newFakeECSCollectorAPI()
	backend, err := NewECSCollectorBackend(api, ECSCollectorConfig{
		ServiceReference: "cluster/web", ImageDigest: "public.ecr.aws/example/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ConfigReference: "env:MAGELIFT_OTEL_CONFIG", Probe: fakeECSCollectorProbe{delivered: true}, SecretResolver: fakeECSSecretResolver{},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := ecsCollectorPlanRequest()
	resource, err := backend.Ensure(context.Background(), sdk.CollectorDeploymentPlan{
		TargetProvider: plan.TargetProvider, TargetRuntime: plan.TargetRuntime, Workload: plan.Workload, Distribution: plan.Distribution,
		CredentialRef: plan.CredentialRef, Endpoint: plan.Endpoint, NativeReference: plan.NativeReference, OwnershipMarker: plan.OwnershipMarker, Signals: plan.Signals,
	})
	if err != nil {
		t.Fatal(err)
	}
	api.reportedTaskDefinition = api.initialTaskDefinition
	verification, err := backend.Verify(context.Background(), sdk.CollectorDeploymentPlan{
		TargetProvider: plan.TargetProvider, TargetRuntime: plan.TargetRuntime, Workload: plan.Workload, Distribution: plan.Distribution,
		CredentialRef: plan.CredentialRef, Endpoint: plan.Endpoint, NativeReference: plan.NativeReference, OwnershipMarker: plan.OwnershipMarker, Signals: plan.Signals,
	}, resource)
	if err != nil {
		t.Fatal(err)
	}
	if verification.Ready || verification.Healthy || verification.SignalsDelivered || !strings.Contains(verification.Reason, "pre-collector") {
		t.Fatalf("verification = %#v, want stale-task rejection", verification)
	}
}

func TestFindRunningCollectorContainerRequiresExactMarkerIdentity(t *testing.T) {
	marker := "magelift/test/ecs-collector"
	expected := ecsCollectorContainerName + markerDigest(marker)
	containers := []ecstypes.Container{
		{Name: awssdk.String(ecsCollectorContainerName + "foreign"), LastStatus: awssdk.String("RUNNING")},
		{Name: awssdk.String(expected), LastStatus: awssdk.String("RUNNING")},
	}
	if got := findRunningCollectorContainer(containers, marker); got == nil || awssdk.ToString(got.Name) != expected {
		t.Fatalf("collector container = %#v, want exact marker-derived name %q", got, expected)
	}
}

func TestParseECSCollectorConfigReference(t *testing.T) {
	tests := []struct {
		name      string
		reference string
		want      string
		wantError bool
	}{
		{name: "valid", reference: "env:MAGELIFT_OTEL_CONFIG", want: "MAGELIFT_OTEL_CONFIG"},
		{name: "file provider", reference: "file:/etc/otel/config.yaml", wantError: true},
		{name: "invalid environment", reference: "env:MAGELIFT-OTEL-CONFIG", wantError: true},
		{name: "empty", reference: "env:", wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseECSCollectorConfigReference(test.reference)
			if test.wantError {
				if err == nil {
					t.Fatalf("parseECSCollectorConfigReference(%q) succeeded", test.reference)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("parseECSCollectorConfigReference(%q) = %q, %v; want %q", test.reference, got, err, test.want)
			}
		})
	}
}

func ecsCollectorPlanRequest() sdk.CollectorDeploymentPlanRequest {
	return sdk.CollectorDeploymentPlanRequest{TargetProvider: "aws", TargetRuntime: "ecs-fargate", Workload: "ecs", Distribution: "opentelemetry-collector-contrib", CredentialRef: "aws-secrets-manager://magelift/newrelic", Endpoint: "https://otlp.eu01.nr-data.net", NativeReference: "cluster/web", OwnershipMarker: "magelift/test/ecs-collector", Signals: []string{"logs", "metrics", "traces"}}
}

type fakeECSCollectorProbe struct{ delivered bool }

func (probe fakeECSCollectorProbe) VerifyCollectorSignals(context.Context, sdk.CollectorDeploymentPlan, providerobservability.CollectorResource) (bool, error) {
	return probe.delivered, nil
}

type fakeECSSecretResolver struct{}

func (fakeECSSecretResolver) ResolveECSSecretReference(context.Context, string) (string, error) {
	return "arn:aws:secretsmanager:eu-west-3:123456789012:secret:newrelic", nil
}

type fakeECSCollectorAPI struct {
	initialTaskDefinition  string
	taskDefinition         string
	reportedTaskDefinition string
	service                ecstypes.Service
	definitions            map[string]*ecs.DescribeTaskDefinitionOutput
	lastRegister           *ecs.RegisterTaskDefinitionInput
	registerCalls          int
	updateCalls            int
	deregisterCalls        int
}

func newFakeECSCollectorAPI() *fakeECSCollectorAPI {
	initial := "arn:aws:ecs:eu-west-3:123456789012:task-definition/web:1"
	return &fakeECSCollectorAPI{
		initialTaskDefinition: initial, taskDefinition: initial,
		service:     ecstypes.Service{TaskDefinition: awssdk.String(initial), DesiredCount: 1, RunningCount: 1},
		definitions: map[string]*ecs.DescribeTaskDefinitionOutput{initial: {TaskDefinition: &ecstypes.TaskDefinition{TaskDefinitionArn: awssdk.String(initial), Family: awssdk.String("web"), Cpu: awssdk.String("1024"), Memory: awssdk.String("2048"), NetworkMode: ecstypes.NetworkModeAwsvpc, RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate}, ContainerDefinitions: []ecstypes.ContainerDefinition{{Name: awssdk.String("web"), Image: awssdk.String("example/web@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), Memory: awssdk.Int32(1024)}}}}},
	}
}

func (api *fakeECSCollectorAPI) DescribeServices(_ context.Context, input *ecs.DescribeServicesInput, _ ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	service := api.service
	if input == nil || len(input.Services) != 1 || input.Services[0] != "web" {
		return &ecs.DescribeServicesOutput{}, nil
	}
	return &ecs.DescribeServicesOutput{Services: []ecstypes.Service{service}}, nil
}

func (api *fakeECSCollectorAPI) DescribeTaskDefinition(_ context.Context, input *ecs.DescribeTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error) {
	if input == nil || input.TaskDefinition == nil {
		return nil, nil
	}
	definition := api.definitions[awssdk.ToString(input.TaskDefinition)]
	if definition == nil {
		return nil, nil
	}
	copy := *definition
	copy.Tags = append([]ecstypes.Tag(nil), definition.Tags...)
	return &copy, nil
}

func (api *fakeECSCollectorAPI) RegisterTaskDefinition(_ context.Context, input *ecs.RegisterTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error) {
	api.registerCalls++
	api.lastRegister = input
	arn := "arn:aws:ecs:eu-west-3:123456789012:task-definition/web-mage:2"
	api.definitions[arn] = &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &ecstypes.TaskDefinition{TaskDefinitionArn: awssdk.String(arn), Family: input.Family, Cpu: input.Cpu, Memory: input.Memory, NetworkMode: input.NetworkMode, RequiresCompatibilities: input.RequiresCompatibilities, ContainerDefinitions: input.ContainerDefinitions}, Tags: append([]ecstypes.Tag(nil), input.Tags...)}
	return &ecs.RegisterTaskDefinitionOutput{TaskDefinition: api.definitions[arn].TaskDefinition}, nil
}

func (api *fakeECSCollectorAPI) UpdateService(_ context.Context, input *ecs.UpdateServiceInput, _ ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error) {
	api.updateCalls++
	api.taskDefinition = awssdk.ToString(input.TaskDefinition)
	api.service.TaskDefinition = input.TaskDefinition
	return &ecs.UpdateServiceOutput{Service: &api.service}, nil
}

func (api *fakeECSCollectorAPI) ListTasks(_ context.Context, _ *ecs.ListTasksInput, _ ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	return &ecs.ListTasksOutput{TaskArns: []string{"arn:aws:ecs:eu-west-3:123456789012:task/web/1"}}, nil
}

func (api *fakeECSCollectorAPI) DescribeTasks(_ context.Context, _ *ecs.DescribeTasksInput, _ ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	reported := api.reportedTaskDefinition
	if reported == "" {
		reported = api.taskDefinition
	}
	return &ecs.DescribeTasksOutput{Tasks: []ecstypes.Task{{TaskDefinitionArn: awssdk.String(reported), LastStatus: awssdk.String("RUNNING"), Containers: []ecstypes.Container{{Name: awssdk.String(ecsCollectorContainerName + markerDigest("magelift/test/ecs-collector")), LastStatus: awssdk.String("RUNNING"), HealthStatus: ecstypes.HealthStatusUnknown}}}}}, nil
}

func (api *fakeECSCollectorAPI) DeregisterTaskDefinition(_ context.Context, input *ecs.DeregisterTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.DeregisterTaskDefinitionOutput, error) {
	api.deregisterCalls++
	delete(api.definitions, awssdk.ToString(input.TaskDefinition))
	return &ecs.DeregisterTaskDefinitionOutput{}, nil
}
