package observability

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	ecsCollectorOwnershipTag         = "magelift/ownership-marker"
	ecsCollectorPreviousTag          = "magelift/previous-task-definition"
	ecsCollectorImageTag             = "magelift/collector-image"
	ecsCollectorConfigTag            = "magelift/collector-config"
	ecsCollectorContainerName        = "magelift-collector-"
	ecsCollectorOwnershipKey         = "magelift.dev/ownership-marker"
	ecsCollectorDefaultCredentialEnv = "NEW_RELIC_LICENSE_KEY"
	ecsCollectorCredentialHeader     = "api-key"
	ecsCollectorOTLPEndpointEnv      = "OTEL_EXPORTER_OTLP_ENDPOINT"
	ecsCollectorReadinessTimeout     = 5 * time.Minute
	ecsCollectorReadinessPoll        = 5 * time.Second
)

// ECSCollectorAPI is the smallest official ECS SDK surface needed to add a
// Contrib sidecar to an existing service task definition and restore that
// service during rollback or cleanup.
type ECSCollectorAPI interface {
	DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	DescribeTaskDefinition(context.Context, *ecs.DescribeTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
	RegisterTaskDefinition(context.Context, *ecs.RegisterTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error)
	UpdateService(context.Context, *ecs.UpdateServiceInput, ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
	ListTasks(context.Context, *ecs.ListTasksInput, ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
	DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
	DeregisterTaskDefinition(context.Context, *ecs.DeregisterTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DeregisterTaskDefinitionOutput, error)
}

// ECSCollectorSignalProbe proves delivery at the selected destination. ECS
// task/service health is necessary but never sufficient telemetry evidence.
type ECSCollectorSignalProbe interface {
	VerifyCollectorSignals(context.Context, sdk.CollectorDeploymentPlan, providerobservability.CollectorResource) (bool, error)
}

// ECSSecretReferenceResolver maps MageLift's provider-neutral secret URI to
// the AWS Secrets Manager/SSM identity accepted by an ECS task definition. It
// must never return or fetch the secret value.
type ECSSecretReferenceResolver interface {
	ResolveECSSecretReference(context.Context, string) (string, error)
}

type ECSSecretReferenceResolverFunc func(context.Context, string) (string, error)

func (resolver ECSSecretReferenceResolverFunc) ResolveECSSecretReference(ctx context.Context, reference string) (string, error) {
	return resolver(ctx, reference)
}

// ECSCollectorConfig contains provider-owned deployment choices. ConfigRef is
// an env: config-provider URI; the backend materializes only the generated
// non-secret collector configuration in that task environment variable.
type ECSCollectorConfig struct {
	ServiceReference      string
	ImageDigest           string
	ConfigReference       string
	CredentialEnvironment string
	CPU                   int32
	MemoryMiB             int32
	Probe                 ECSCollectorSignalProbe
	SecretResolver        ECSSecretReferenceResolver
}

// ECSCollectorBackend is the concrete AWS implementation of the shared
// CollectorBackend port. It mutates only a task-definition revision and the
// selected service; it never deletes or replaces the customer's ECS service.
type ECSCollectorBackend struct {
	api                   ECSCollectorAPI
	cluster               string
	service               string
	serviceReference      string
	image                 string
	configReference       string
	configEnvironment     string
	credentialEnvironment string
	cpu                   int32
	memoryMiB             int32
	probe                 ECSCollectorSignalProbe
	secretResolver        ECSSecretReferenceResolver
}

var _ providerobservability.CollectorBackend = (*ECSCollectorBackend)(nil)

// NewECSCollectorBackend constructs an ECS task-definition translator around
// an injected official SDK client. The service reference is required so direct
// inventory remains scoped after a process restart.
func NewECSCollectorBackend(api ECSCollectorAPI, config ECSCollectorConfig) (*ECSCollectorBackend, error) {
	if api == nil {
		return nil, errors.New("ECS collector API is required")
	}
	cluster, service, err := parseECSServiceReference(config.ServiceReference)
	if err != nil {
		return nil, err
	}
	image := strings.TrimSpace(config.ImageDigest)
	if !isImmutableCollectorImage(image) {
		return nil, errors.New("ECS collector image must use an immutable repository@sha256 digest")
	}
	configReference := strings.TrimSpace(config.ConfigReference)
	configEnvironment, err := parseECSCollectorConfigReference(configReference)
	if err != nil {
		return nil, err
	}
	if config.CPU <= 0 {
		config.CPU = 128
	}
	if config.MemoryMiB <= 0 {
		config.MemoryMiB = 256
	}
	if config.Probe == nil {
		return nil, errors.New("ECS collector signal probe is required")
	}
	if config.SecretResolver == nil {
		return nil, errors.New("ECS collector secret-reference resolver is required")
	}
	credentialEnvironment := strings.TrimSpace(config.CredentialEnvironment)
	if credentialEnvironment == "" {
		credentialEnvironment = ecsCollectorDefaultCredentialEnv
	}
	if !validEnvironmentName(credentialEnvironment) {
		return nil, errors.New("ECS collector credential environment must be a valid environment variable name")
	}
	return &ECSCollectorBackend{api: api, cluster: cluster, service: service, serviceReference: strings.TrimSpace(config.ServiceReference), image: image, configReference: configReference, configEnvironment: configEnvironment, credentialEnvironment: credentialEnvironment, cpu: config.CPU, memoryMiB: config.MemoryMiB, probe: config.Probe, secretResolver: config.SecretResolver}, nil
}

// NewECSCollectorSDKBackend is the provider-owned official SDK constructor.
func NewECSCollectorSDKBackend(config awssdk.Config, collector ECSCollectorConfig) (*ECSCollectorBackend, error) {
	return NewECSCollectorBackend(ecs.NewFromConfig(config), collector)
}

func (backend *ECSCollectorBackend) Find(ctx context.Context, plan sdk.CollectorDeploymentPlan) (providerobservability.CollectorResource, bool, error) {
	inspection, err := backend.inspect(ctx, plan.TargetProvider, plan.TargetRuntime, plan.Workload, plan.Distribution, plan.OwnershipMarker)
	if err != nil {
		return providerobservability.CollectorResource{}, false, err
	}
	if !inspection.hasCollector {
		return providerobservability.CollectorResource{}, false, nil
	}
	return inspection.resource, true, nil
}

func (backend *ECSCollectorBackend) Ensure(ctx context.Context, plan sdk.CollectorDeploymentPlan) (providerobservability.CollectorResource, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.CollectorResource{}, err
	}
	inspection, err := backend.inspect(ctx, plan.TargetProvider, plan.TargetRuntime, plan.Workload, plan.Distribution, plan.OwnershipMarker)
	if err != nil {
		return providerobservability.CollectorResource{}, err
	}
	if inspection.hasCollector {
		if !inspection.resource.Owned || inspection.resource.OwnershipMarker != plan.OwnershipMarker {
			return providerobservability.CollectorResource{}, errors.New("refusing to mutate an unowned ECS collector sidecar")
		}
		return inspection.resource, nil
	}
	if inspection.service == nil || inspection.taskDefinition == nil {
		return providerobservability.CollectorResource{}, errors.New("ECS collector target service or task definition is missing")
	}
	definition := *inspection.taskDefinition
	containers := append([]ecstypes.ContainerDefinition(nil), definition.ContainerDefinitions...)
	collectorContainer, err := backend.collectorContainer(ctx, plan)
	if err != nil {
		return providerobservability.CollectorResource{}, err
	}
	containers = append(containers, collectorContainer)
	previousTaskDefinition := awssdk.ToString(inspection.service.TaskDefinition)
	registered, err := backend.api.RegisterTaskDefinition(ctx, registerCollectorTaskDefinitionInput(definition, containers, plan, inspection.tags, previousTaskDefinition))
	if err != nil {
		return providerobservability.CollectorResource{}, fmt.Errorf("register ECS collector task definition: %w", err)
	}
	if registered == nil || registered.TaskDefinition == nil || registered.TaskDefinition.TaskDefinitionArn == nil {
		return providerobservability.CollectorResource{}, errors.New("ECS collector task-definition registration returned no ARN")
	}
	collectorTaskDefinition := awssdk.ToString(registered.TaskDefinition.TaskDefinitionArn)
	if _, err := backend.api.UpdateService(ctx, &ecs.UpdateServiceInput{Cluster: awssdk.String(backend.cluster), Service: awssdk.String(backend.service), TaskDefinition: awssdk.String(collectorTaskDefinition), ForceNewDeployment: true}); err != nil {
		_, _ = backend.api.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{TaskDefinition: awssdk.String(collectorTaskDefinition)})
		return providerobservability.CollectorResource{}, fmt.Errorf("update ECS service for collector: %w", err)
	}
	return backend.resource(plan.TargetProvider, plan.TargetRuntime, plan.Workload, plan.Distribution, plan.OwnershipMarker, collectorTaskDefinition, previousTaskDefinition, deploymentStatus(inspection.service)), nil
}

func (backend *ECSCollectorBackend) Verify(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) (providerobservability.CollectorVerification, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.CollectorVerification{}, err
	}
	deadline := time.NewTimer(ecsCollectorReadinessTimeout)
	defer deadline.Stop()
	poll := time.NewTicker(ecsCollectorReadinessPoll)
	defer poll.Stop()
	for {
		inspection, err := backend.inspect(ctx, plan.TargetProvider, plan.TargetRuntime, plan.Workload, plan.Distribution, plan.OwnershipMarker)
		if err != nil {
			return providerobservability.CollectorVerification{}, err
		}
		verification, retry, err := backend.verifyRuntime(ctx, plan, resource, inspection)
		if err != nil {
			return providerobservability.CollectorVerification{}, err
		}
		if !retry {
			if !verification.Ready || !verification.Healthy {
				return verification, nil
			}
			break
		}
		select {
		case <-ctx.Done():
			return providerobservability.CollectorVerification{}, ctx.Err()
		case <-deadline.C:
			return verification, nil
		case <-poll.C:
		}
	}
	delivered, err := backend.probe.VerifyCollectorSignals(ctx, plan, resource)
	if err != nil {
		return providerobservability.CollectorVerification{}, fmt.Errorf("verify ECS collector signal delivery: %w", err)
	}
	if !delivered {
		return providerobservability.CollectorVerification{Ready: true, Healthy: true, Reason: "ECS collector signal probe did not verify delivery"}, nil
	}
	return providerobservability.CollectorVerification{Ready: true, Healthy: true, SignalsDelivered: true}, nil
}

func (backend *ECSCollectorBackend) verifyRuntime(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource, inspection ecsInspection) (providerobservability.CollectorVerification, bool, error) {
	if !inspection.hasCollector || !inspection.resource.Owned || resource.Identity != inspection.resource.Identity {
		return providerobservability.CollectorVerification{Reason: "ECS collector sidecar is missing or ownership changed"}, false, nil
	}
	desired := inspection.service.DesiredCount
	if desired <= 0 {
		return providerobservability.CollectorVerification{Reason: "ECS service has no desired tasks"}, false, nil
	}
	expectedTaskDefinition := awssdk.ToString(inspection.taskDefinition.TaskDefinitionArn)
	if expectedTaskDefinition == "" {
		return providerobservability.CollectorVerification{}, false, errors.New("ECS collector task definition has no ARN")
	}
	tasks, err := backend.api.ListTasks(ctx, &ecs.ListTasksInput{Cluster: awssdk.String(backend.cluster), ServiceName: awssdk.String(backend.service), DesiredStatus: ecstypes.DesiredStatusRunning})
	if err != nil {
		return providerobservability.CollectorVerification{}, false, fmt.Errorf("list running ECS collector tasks: %w", err)
	}
	if tasks == nil || int32(len(tasks.TaskArns)) < desired {
		return providerobservability.CollectorVerification{Reason: "ECS service has fewer running tasks than desired"}, ecsCollectorRolloutPending(inspection.service, expectedTaskDefinition), nil
	}
	described, err := backend.api.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: awssdk.String(backend.cluster), Tasks: append([]string(nil), tasks.TaskArns...)})
	if err != nil {
		return providerobservability.CollectorVerification{}, false, fmt.Errorf("describe running ECS collector tasks: %w", err)
	}
	if described == nil || int32(len(described.Tasks)) < desired {
		return providerobservability.CollectorVerification{Reason: "ECS task descriptions are incomplete"}, ecsCollectorRolloutPending(inspection.service, expectedTaskDefinition), nil
	}
	readyTasks := int32(0)
	sawOldTask := false
	for _, task := range described.Tasks {
		if awssdk.ToString(task.TaskDefinitionArn) != expectedTaskDefinition {
			sawOldTask = true
			continue
		}
		if awssdk.ToString(task.LastStatus) != "RUNNING" {
			return providerobservability.CollectorVerification{Reason: "an ECS task is not RUNNING"}, ecsCollectorRolloutPending(inspection.service, expectedTaskDefinition), nil
		}
		container := findRunningCollectorContainer(task.Containers, plan.OwnershipMarker)
		if container == nil || awssdk.ToString(container.LastStatus) != "RUNNING" {
			return providerobservability.CollectorVerification{Reason: "the ECS collector sidecar is not RUNNING"}, ecsCollectorRolloutPending(inspection.service, expectedTaskDefinition), nil
		}
		if container.HealthStatus == ecstypes.HealthStatusUnhealthy {
			return providerobservability.CollectorVerification{Reason: "the ECS collector sidecar is UNHEALTHY"}, false, nil
		}
		readyTasks++
	}
	if readyTasks < desired {
		reason := "ECS service has fewer ready collector tasks than desired"
		if readyTasks == 0 && sawOldTask {
			reason = "an ECS task is still running the pre-collector task definition"
		}
		return providerobservability.CollectorVerification{Reason: reason}, ecsCollectorRolloutPending(inspection.service, expectedTaskDefinition), nil
	}
	return providerobservability.CollectorVerification{Ready: true, Healthy: true}, false, nil
}

func ecsCollectorRolloutPending(service *ecstypes.Service, taskDefinition string) bool {
	if service == nil || len(service.Deployments) == 0 {
		return false
	}
	for _, deployment := range service.Deployments {
		if awssdk.ToString(deployment.TaskDefinition) == taskDefinition && awssdk.ToString(deployment.Status) == "PRIMARY" {
			return deployment.RolloutState != ecstypes.DeploymentRolloutStateFailed
		}
	}
	return false
}

func (backend *ECSCollectorBackend) Rollback(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) error {
	return backend.restore(ctx, plan, resource)
}

func (backend *ECSCollectorBackend) Destroy(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) error {
	return backend.restore(ctx, plan, resource)
}

func (backend *ECSCollectorBackend) Inventory(ctx context.Context, provider sdk.ProviderID, runtime sdk.RuntimeID, marker string) ([]providerobservability.CollectorResource, error) {
	inspection, err := backend.inspect(ctx, provider, runtime, "ecs", "opentelemetry-collector-contrib", marker)
	if err != nil {
		return nil, err
	}
	if !inspection.hasCollector {
		return nil, nil
	}
	return []providerobservability.CollectorResource{inspection.resource}, nil
}

func (backend *ECSCollectorBackend) restore(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) error {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return err
	}
	if !resource.Owned || resource.OwnershipMarker != plan.OwnershipMarker {
		return errors.New("refusing ECS collector restore without exact ownership")
	}
	collectorTaskDefinition, previousTaskDefinition, ok := parseECSCollectorIdentity(resource.Identity)
	if !ok || previousTaskDefinition == "" {
		return errors.New("ECS collector resource does not contain a safe previous task-definition identity")
	}
	inspection, err := backend.inspect(ctx, plan.TargetProvider, plan.TargetRuntime, plan.Workload, plan.Distribution, plan.OwnershipMarker)
	if err != nil {
		return err
	}
	if !inspection.hasCollector {
		return nil
	}
	if !inspection.resource.Owned || inspection.resource.Identity != resource.Identity {
		return errors.New("ECS collector ownership or identity changed before restore")
	}
	if _, err := backend.api.UpdateService(ctx, &ecs.UpdateServiceInput{Cluster: awssdk.String(backend.cluster), Service: awssdk.String(backend.service), TaskDefinition: awssdk.String(previousTaskDefinition), ForceNewDeployment: true}); err != nil {
		return fmt.Errorf("restore ECS service task definition: %w", err)
	}
	restored, err := backend.api.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: awssdk.String(backend.cluster), Services: []string{backend.service}})
	if err != nil {
		return fmt.Errorf("verify ECS service task-definition restore: %w", err)
	}
	if restored == nil || len(restored.Services) != 1 || awssdk.ToString(restored.Services[0].TaskDefinition) != previousTaskDefinition {
		return errors.New("ECS service did not report the previous task definition after restore")
	}
	if _, err := backend.api.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{TaskDefinition: awssdk.String(collectorTaskDefinition)}); err != nil {
		return fmt.Errorf("deregister ECS collector task definition: %w", err)
	}
	return nil
}

type ecsInspection struct {
	service        *ecstypes.Service
	taskDefinition *ecstypes.TaskDefinition
	tags           []ecstypes.Tag
	hasCollector   bool
	resource       providerobservability.CollectorResource
}

func (backend *ECSCollectorBackend) inspect(ctx context.Context, provider sdk.ProviderID, runtime sdk.RuntimeID, workload, distribution, marker string) (ecsInspection, error) {
	if backend == nil || backend.api == nil {
		return ecsInspection{}, errors.New("ECS collector backend is required")
	}
	if ctx == nil {
		return ecsInspection{}, errors.New("ECS collector context is required")
	}
	services, err := backend.api.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: awssdk.String(backend.cluster), Services: []string{backend.service}})
	if err != nil {
		return ecsInspection{}, fmt.Errorf("describe ECS collector service: %w", err)
	}
	if services == nil || len(services.Services) != 1 || services.Services[0].TaskDefinition == nil {
		return ecsInspection{}, fmt.Errorf("ECS collector service %q was not found", backend.serviceReference)
	}
	service := &services.Services[0]
	described, err := backend.api.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{TaskDefinition: service.TaskDefinition, Include: []ecstypes.TaskDefinitionField{ecstypes.TaskDefinitionFieldTags}})
	if err != nil {
		return ecsInspection{}, fmt.Errorf("describe ECS collector task definition: %w", err)
	}
	if described == nil || described.TaskDefinition == nil {
		return ecsInspection{}, errors.New("ECS collector task definition was not returned")
	}
	container := findCollectorContainer(described.TaskDefinition.ContainerDefinitions, marker)
	foreign := false
	for _, candidate := range described.TaskDefinition.ContainerDefinitions {
		if !isCollectorContainer(candidate) {
			continue
		}
		if candidate.DockerLabels[ecsCollectorOwnershipKey] != marker {
			foreign = true
		}
	}
	if container == nil && !foreign {
		return ecsInspection{service: service, taskDefinition: described.TaskDefinition, tags: append([]ecstypes.Tag(nil), described.Tags...)}, nil
	}
	owned := container != nil && !foreign && container.DockerLabels[ecsCollectorOwnershipKey] == marker
	previous := tagValue(described.Tags, ecsCollectorPreviousTag)
	current := awssdk.ToString(described.TaskDefinition.TaskDefinitionArn)
	if current == "" {
		return ecsInspection{}, errors.New("ECS collector task definition has no ARN")
	}
	resource := backend.resource(provider, runtime, workload, distribution, marker, current, previous, deploymentStatus(service))
	resource.Owned = owned
	if container != nil {
		resource.OwnershipMarker = container.DockerLabels[ecsCollectorOwnershipKey]
	}
	return ecsInspection{service: service, taskDefinition: described.TaskDefinition, tags: append([]ecstypes.Tag(nil), described.Tags...), hasCollector: true, resource: resource}, nil
}

func (backend *ECSCollectorBackend) validatePlan(ctx context.Context, plan sdk.CollectorDeploymentPlan) error {
	if backend == nil || backend.api == nil {
		return errors.New("ECS collector backend is required")
	}
	if ctx == nil {
		return errors.New("ECS collector context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if plan.Distribution != "opentelemetry-collector-contrib" {
		return sdk.CollectorCapabilityError{Action: "backend", Reason: "ECS task-definition backend supports only the OpenTelemetry Collector Contrib distribution"}
	}
	if plan.NativeReference != backend.serviceReference {
		return errors.New("ECS collector native reference does not match the backend service scope")
	}
	if strings.TrimSpace(plan.OwnershipMarker) == "" || strings.ContainsAny(plan.OwnershipMarker, "\r\n\x00") {
		return errors.New("ECS collector ownership marker is required")
	}
	return nil
}

func (backend *ECSCollectorBackend) collectorContainer(ctx context.Context, plan sdk.CollectorDeploymentPlan) (ecstypes.ContainerDefinition, error) {
	secretIdentity, err := backend.secretResolver.ResolveECSSecretReference(ctx, plan.CredentialRef)
	if err != nil {
		return ecstypes.ContainerDefinition{}, fmt.Errorf("resolve ECS collector credential reference: %w", err)
	}
	if err := validateECSCredentialReference(secretIdentity); err != nil {
		return ecstypes.ContainerDefinition{}, err
	}
	return ecstypes.ContainerDefinition{
		Name:      awssdk.String(ecsCollectorContainerName + markerDigest(plan.OwnershipMarker)),
		Image:     awssdk.String(backend.image),
		Cpu:       backend.cpu,
		Memory:    awssdk.Int32(backend.memoryMiB),
		Essential: awssdk.Bool(false),
		Command:   []string{"--config=" + backend.configReference},
		Environment: []ecstypes.KeyValuePair{
			{Name: awssdk.String(ecsCollectorOTLPEndpointEnv), Value: awssdk.String(plan.Endpoint)},
			{Name: awssdk.String("MAGELIFT_OWNERSHIP_MARKER"), Value: awssdk.String(plan.OwnershipMarker)},
			{Name: awssdk.String(backend.configEnvironment), Value: awssdk.String(ecsCollectorConfig(plan, backend.credentialEnvironment))},
		},
		Secrets:      []ecstypes.Secret{{Name: awssdk.String(backend.credentialEnvironment), ValueFrom: awssdk.String(secretIdentity)}},
		DockerLabels: map[string]string{ecsCollectorOwnershipKey: plan.OwnershipMarker},
	}, nil
}

func ecsCollectorConfig(plan sdk.CollectorDeploymentPlan, credentialEnvironment string) string {
	var builder strings.Builder
	builder.WriteString("receivers:\n  otlp:\n    protocols:\n      grpc:\n        endpoint: 0.0.0.0:4317\n      http:\n        endpoint: 0.0.0.0:4318\nexporters:\n  otlphttp/destination:\n    endpoint: ${")
	builder.WriteString(ecsCollectorOTLPEndpointEnv)
	builder.WriteString("}\n    headers:\n      ")
	builder.WriteString(ecsCollectorCredentialHeader)
	builder.WriteString(": ${")
	builder.WriteString(credentialEnvironment)
	builder.WriteString("}\nservice:\n  pipelines:\n")
	for _, signal := range plan.Signals {
		builder.WriteString("    ")
		builder.WriteString(signal)
		builder.WriteString(":\n      receivers: [otlp]\n      exporters: [otlphttp/destination]\n")
	}
	return builder.String()
}

func registerCollectorTaskDefinitionInput(definition ecstypes.TaskDefinition, containers []ecstypes.ContainerDefinition, plan sdk.CollectorDeploymentPlan, existingTags []ecstypes.Tag, previousTaskDefinition string) *ecs.RegisterTaskDefinitionInput {
	family := awssdk.ToString(definition.Family) + "-magelift-" + markerDigest(plan.OwnershipMarker)
	if len(family) > 255 {
		family = family[:255]
	}
	tags := setTag(append([]ecstypes.Tag(nil), existingTags...), ecsCollectorOwnershipTag, plan.OwnershipMarker)
	tags = setTag(tags, ecsCollectorPreviousTag, previousTaskDefinition)
	tags = setTag(tags, ecsCollectorImageTag, awssdk.ToString(containers[len(containers)-1].Image))
	tags = setTag(tags, ecsCollectorConfigTag, "configured")
	return &ecs.RegisterTaskDefinitionInput{Family: awssdk.String(family), ContainerDefinitions: containers, Cpu: definition.Cpu, EnableFaultInjection: definition.EnableFaultInjection, EphemeralStorage: definition.EphemeralStorage, ExecutionRoleArn: definition.ExecutionRoleArn, InferenceAccelerators: append([]ecstypes.InferenceAccelerator(nil), definition.InferenceAccelerators...), IpcMode: definition.IpcMode, Memory: definition.Memory, NetworkMode: definition.NetworkMode, PidMode: definition.PidMode, PlacementConstraints: append([]ecstypes.TaskDefinitionPlacementConstraint(nil), definition.PlacementConstraints...), ProxyConfiguration: definition.ProxyConfiguration, RequiresCompatibilities: append([]ecstypes.Compatibility(nil), definition.RequiresCompatibilities...), RuntimePlatform: definition.RuntimePlatform, Tags: tags, TaskRoleArn: definition.TaskRoleArn, Volumes: append([]ecstypes.Volume(nil), definition.Volumes...)}
}

func findCollectorContainer(containers []ecstypes.ContainerDefinition, marker string) *ecstypes.ContainerDefinition {
	for i := range containers {
		if isCollectorContainer(containers[i]) && containers[i].DockerLabels[ecsCollectorOwnershipKey] == marker {
			return &containers[i]
		}
	}
	return nil
}

func findRunningCollectorContainer(containers []ecstypes.Container, marker string) *ecstypes.Container {
	expectedName := ecsCollectorContainerName + markerDigest(marker)
	for i := range containers {
		if marker != "" && awssdk.ToString(containers[i].Name) == expectedName {
			return &containers[i]
		}
	}
	return nil
}

func isCollectorContainer(container ecstypes.ContainerDefinition) bool {
	return strings.HasPrefix(awssdk.ToString(container.Name), ecsCollectorContainerName) || container.DockerLabels[ecsCollectorOwnershipKey] != ""
}

func (backend *ECSCollectorBackend) resource(provider sdk.ProviderID, runtime sdk.RuntimeID, workload, distribution, marker, current, previous, status string) providerobservability.CollectorResource {
	return providerobservability.CollectorResource{Identity: ecsCollectorIdentity(backend.cluster, backend.service, current, previous), TargetProvider: provider, TargetRuntime: runtime, Workload: workload, Distribution: distribution, OwnershipMarker: marker, Status: status, Owned: true}
}

func ecsCollectorIdentity(cluster, service, current, previous string) string {
	return "service:" + cluster + "/" + service + "|task:" + current + "|previous:" + previous
}

func parseECSCollectorIdentity(identity string) (string, string, bool) {
	parts := strings.Split(identity, "|")
	if len(parts) != 3 || !strings.HasPrefix(parts[1], "task:") || !strings.HasPrefix(parts[2], "previous:") {
		return "", "", false
	}
	collector := strings.TrimPrefix(parts[1], "task:")
	previous := strings.TrimPrefix(parts[2], "previous:")
	return collector, previous, collector != "" && strings.HasPrefix(parts[0], "service:")
}

func parseECSServiceReference(reference string) (string, string, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" || strings.ContainsAny(reference, "\r\n\x00|") {
		return "", "", errors.New("ECS service reference is required and must be single-line")
	}
	if strings.HasPrefix(reference, "arn:") {
		parts := strings.SplitN(reference, ":", 6)
		if len(parts) != 6 || parts[2] != "ecs" || !strings.HasPrefix(parts[5], "service/") {
			return "", "", errors.New("ECS service ARN is invalid")
		}
		serviceParts := strings.Split(strings.TrimPrefix(parts[5], "service/"), "/")
		if len(serviceParts) != 2 || serviceParts[0] == "" || serviceParts[1] == "" {
			return "", "", errors.New("ECS service ARN must contain service/cluster/service")
		}
		return serviceParts[0], serviceParts[1], nil
	}
	parts := strings.Split(reference, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", errors.New("ECS service reference must be cluster/service or a full ECS service ARN")
	}
	return parts[0], parts[1], nil
}

func parseECSCollectorConfigReference(reference string) (string, error) {
	const prefix = "env:"
	if !strings.HasPrefix(reference, prefix) {
		return "", errors.New("ECS collector config reference must use the env: provider")
	}
	name := strings.TrimPrefix(reference, prefix)
	if !validEnvironmentName(name) {
		return "", errors.New("ECS collector config environment must be a valid environment variable name")
	}
	return name, nil
}

func validateECSCredentialReference(reference string) error {
	if strings.TrimSpace(reference) == "" || strings.ContainsAny(reference, "\r\n\x00") {
		return errors.New("ECS collector credential reference is required")
	}
	if !(strings.HasPrefix(reference, "arn:aws:secretsmanager:") || strings.HasPrefix(reference, "arn:aws:ssm:") || strings.HasPrefix(reference, "arn:aws-us-gov:secretsmanager:") || strings.HasPrefix(reference, "arn:aws-us-gov:ssm:") || strings.HasPrefix(reference, "arn:aws-cn:secretsmanager:") || strings.HasPrefix(reference, "arn:aws-cn:ssm:")) {
		return errors.New("ECS collector credential reference must be an AWS Secrets Manager or SSM identity")
	}
	return nil
}

func tagValue(tags []ecstypes.Tag, key string) string {
	for _, tag := range tags {
		if awssdk.ToString(tag.Key) == key {
			return awssdk.ToString(tag.Value)
		}
	}
	return ""
}

func setTag(tags []ecstypes.Tag, key, value string) []ecstypes.Tag {
	for i := range tags {
		if awssdk.ToString(tags[i].Key) == key {
			tags[i].Value = awssdk.String(value)
			return tags
		}
	}
	return append(tags, ecstypes.Tag{Key: awssdk.String(key), Value: awssdk.String(value)})
}

func deploymentStatus(service *ecstypes.Service) string {
	if service == nil {
		return "missing"
	}
	if service.DesiredCount > 0 && service.RunningCount >= service.DesiredCount {
		return "ready"
	}
	return "pending"
}

func isImmutableCollectorImage(image string) bool {
	separator := strings.LastIndex(image, "@sha256:")
	return separator > 0 && len(image[separator+len("@sha256:"):]) == 64 && strings.Trim(image[separator+len("@sha256:"):], "0123456789abcdef") == ""
}

func validEnvironmentName(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '_' || (index > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}
