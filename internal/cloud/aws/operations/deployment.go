package operations

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
)

var candidateDigest = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)

const (
	databaseClientImage  = "public.ecr.aws/docker/library/mysql:8.4"
	databaseGrantCommand = "set -eu; MYSQL_PWD=\"$MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD\" mysql --protocol=TCP -h \"$MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST\" -P \"$MAGENTO_DC_DB__CONNECTION__DEFAULT__PORT\" -u \"$MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME\" -e \"GRANT ALL PRIVILEGES ON \\`$MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME\\`.* TO '$MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME'@'%';\""
)

// DeploymentAPI is the bounded ECS surface used for pre-traffic Magento
// migrations. The durable service remains owned by Pulumi; this adapter owns
// only the short-lived candidate task definition and task.
type DeploymentAPI interface {
	DescribeTaskDefinition(context.Context, *ecs.DescribeTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
	RegisterTaskDefinition(context.Context, *ecs.RegisterTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error)
	RunTask(context.Context, *ecs.RunTaskInput, ...func(*ecs.Options)) (*ecs.RunTaskOutput, error)
	DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
	StopTask(context.Context, *ecs.StopTaskInput, ...func(*ecs.Options)) (*ecs.StopTaskOutput, error)
	DeregisterTaskDefinition(context.Context, *ecs.DeregisterTaskDefinitionInput, ...func(*ecs.Options)) (*ecs.DeregisterTaskDefinitionOutput, error)
}

type CandidateRequest struct {
	Cluster           string
	TaskDefinitionARN string
	ImageDigest       string
	PrivateSubnetIDs  []string
	SecurityGroupID   string
	StartedBy         string
}

type Candidate struct {
	Cluster                        string
	TaskDefinitionARN              string
	DatabaseGrantTaskDefinitionARN string
	TaskARN                        string
	PrivateSubnetIDs               []string
	SecurityGroupID                string
	StartedBy                      string
}

type DeploymentStore struct {
	client       DeploymentAPI
	waitInterval time.Duration
	waitTimeout  time.Duration
	probeTimeout time.Duration // test override; non-positive means probeWaitTimeout
}

func NewDeployment(ctx context.Context, region string) (*DeploymentStore, error) {
	if strings.TrimSpace(region) == "" {
		return nil, errors.New("AWS ECS region is required")
	}
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	options := []func(*ecs.Options){}
	if endpoint != "" {
		options = append(options, func(options *ecs.Options) { options.BaseEndpoint = awssdk.String(endpoint) })
	}
	return NewDeploymentFromClient(ecs.NewFromConfig(cfg, options...))
}

func NewDeploymentFromClient(client DeploymentAPI) (*DeploymentStore, error) {
	if client == nil {
		return nil, errors.New("AWS ECS deployment client is required")
	}
	return &DeploymentStore{client: client, waitInterval: 2 * time.Second, waitTimeout: 30 * time.Minute}, nil
}

// RegisterCandidate copies the durable deploy task definition, replaces only
// its image, and registers an ephemeral family. This prevents a migration from
// running an older image when the configured release changes between updates.
func (s *DeploymentStore) RegisterCandidate(ctx context.Context, request CandidateRequest) (Candidate, error) {
	if s == nil || s.client == nil {
		return Candidate{}, errors.New("AWS ECS deployment client is required")
	}
	if err := validateCandidateRequest(request); err != nil {
		return Candidate{}, err
	}
	described, err := s.client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: awssdk.String(request.TaskDefinitionARN),
		Include:        []types.TaskDefinitionField{types.TaskDefinitionFieldTags},
	})
	if err != nil {
		return Candidate{}, fmt.Errorf("describe deploy task definition: %w", err)
	}
	if described == nil || described.TaskDefinition == nil || len(described.TaskDefinition.ContainerDefinitions) == 0 {
		return Candidate{}, errors.New("deploy task definition has no container definition")
	}
	definition := *described.TaskDefinition
	containers := append([]types.ContainerDefinition(nil), definition.ContainerDefinitions...)
	tags := append([]types.Tag(nil), described.Tags...)
	replaceApplicationImages(containers, request.ImageDigest)
	baseFamily := awssdk.ToString(definition.Family)
	if baseFamily == "" {
		return Candidate{}, errors.New("deploy task definition has no family")
	}
	family := baseFamily + "-candidate-" + time.Now().UTC().Format("20060102T150405000000000")
	registeredARN, err := registerTaskDefinition(ctx, s.client, definition, family, containers, tags)
	if err != nil {
		return Candidate{}, fmt.Errorf("register deploy candidate: %w", err)
	}
	grantARN := ""
	if grantContainer, ok := databaseGrantContainer(definition); ok {
		grantARN, err = registerTaskDefinition(ctx, s.client, definition, family+"-db-grant", []types.ContainerDefinition{grantContainer}, tags)
		if err != nil {
			_, _ = s.client.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{TaskDefinition: awssdk.String(registeredARN)})
			return Candidate{}, fmt.Errorf("register database privilege grant: %w", err)
		}
	}
	startedBy := request.StartedBy
	if strings.TrimSpace(startedBy) == "" {
		startedBy = "magelift"
	}
	return Candidate{Cluster: request.Cluster, TaskDefinitionARN: registeredARN, DatabaseGrantTaskDefinitionARN: grantARN, PrivateSubnetIDs: append([]string(nil), request.PrivateSubnetIDs...), SecurityGroupID: request.SecurityGroupID, StartedBy: startedBy}, nil
}

func registerTaskDefinition(ctx context.Context, client DeploymentAPI, definition types.TaskDefinition, family string, containers []types.ContainerDefinition, tags []types.Tag) (string, error) {
	registered, err := client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  awssdk.String(family),
		ContainerDefinitions:    containers,
		Cpu:                     definition.Cpu,
		EnableFaultInjection:    definition.EnableFaultInjection,
		EphemeralStorage:        definition.EphemeralStorage,
		ExecutionRoleArn:        definition.ExecutionRoleArn,
		InferenceAccelerators:   append([]types.InferenceAccelerator(nil), definition.InferenceAccelerators...),
		IpcMode:                 definition.IpcMode,
		Memory:                  definition.Memory,
		NetworkMode:             definition.NetworkMode,
		PidMode:                 definition.PidMode,
		PlacementConstraints:    append([]types.TaskDefinitionPlacementConstraint(nil), definition.PlacementConstraints...),
		ProxyConfiguration:      definition.ProxyConfiguration,
		RequiresCompatibilities: append([]types.Compatibility(nil), definition.RequiresCompatibilities...),
		RuntimePlatform:         definition.RuntimePlatform,
		Tags:                    tags,
		TaskRoleArn:             definition.TaskRoleArn,
		Volumes:                 append([]types.Volume(nil), definition.Volumes...),
	})
	if err != nil {
		return "", err
	}
	if registered == nil || registered.TaskDefinition == nil || registered.TaskDefinition.TaskDefinitionArn == nil {
		return "", errors.New("registration returned no task definition ARN")
	}
	return awssdk.ToString(registered.TaskDefinition.TaskDefinitionArn), nil
}

func databaseGrantContainer(definition types.TaskDefinition) (types.ContainerDefinition, bool) {
	for _, container := range definition.ContainerDefinitions {
		if awssdk.ToString(container.Name) != "deploy" || !hasDatabaseGrantInputs(container) {
			continue
		}
		container.Name = awssdk.String("database-grant")
		container.Image = awssdk.String(databaseClientImage)
		container.EntryPoint = []string{"/bin/sh", "-ec"}
		container.Command = []string{databaseGrantCommand}
		container.HealthCheck = nil
		container.PortMappings = nil
		container.DependsOn = nil
		return container, true
	}
	return types.ContainerDefinition{}, false
}

func hasDatabaseGrantInputs(container types.ContainerDefinition) bool {
	environment := map[string]bool{}
	for _, value := range container.Environment {
		environment[awssdk.ToString(value.Name)] = true
	}
	secrets := map[string]bool{}
	for _, value := range container.Secrets {
		secrets[awssdk.ToString(value.Name)] = true
	}
	for _, name := range []string{
		"MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME",
		"MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST",
		"MAGENTO_DC_DB__CONNECTION__DEFAULT__PORT",
	} {
		if !environment[name] {
			return false
		}
	}
	for _, name := range []string{
		"MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD",
		"MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME",
	} {
		if !secrets[name] {
			return false
		}
	}
	return true
}

// replaceApplicationImages keeps every container built from the application
// image on the same promoted digest while leaving unrelated sidecars intact.
// The runtime uses this for the nginx and PHP-FPM pair; the deploy task has a
// single container, so the same rule also covers that topology.
func replaceApplicationImages(containers []types.ContainerDefinition, image string) {
	if len(containers) == 0 {
		return
	}
	previous := awssdk.ToString(containers[0].Image)
	firstIsApplication := applicationContainerName(awssdk.ToString(containers[0].Name))
	for i := range containers {
		if applicationContainerName(awssdk.ToString(containers[i].Name)) || (firstIsApplication && awssdk.ToString(containers[i].Image) == previous) {
			containers[i].Image = awssdk.String(image)
		}
	}
}

func applicationContainerName(name string) bool {
	switch name {
	case "app", "deploy", "php-fpm", "web", "cron", "queue":
		return true
	default:
		return false
	}
}

// oneOffTaskResult treats Magento migrate as successful when application
// containers exit 0. Sidecars such as search-proxy are essential on copied
// deploy task defs and receive SIGTERM after setup:upgrade, which ECS reports
// as a failed essential container even though Magento finished.
func oneOffTaskResult(label string, task types.Task) error {
	application, others := splitTaskContainers(task.Containers)
	check := others
	if len(application) > 0 {
		check = application
	}
	for _, container := range check {
		if container.ExitCode != nil && awssdk.ToInt32(container.ExitCode) == 0 {
			continue
		}
		name := awssdk.ToString(container.Name)
		if name == "" {
			name = "unnamed"
		}
		exitCode := "unknown"
		if container.ExitCode != nil {
			exitCode = fmt.Sprintf("%d", awssdk.ToInt32(container.ExitCode))
		}
		reason := strings.TrimSpace(awssdk.ToString(container.Reason))
		if reason == "" {
			reason = "not reported by ECS"
		}
		stoppedReason := strings.TrimSpace(awssdk.ToString(task.StoppedReason))
		if stoppedReason == "" {
			stoppedReason = "not reported by ECS"
		}
		return fmt.Errorf("%s container failed: name=%s exit_code=%s reason=%s task_reason=%s", label, name, exitCode, reason, stoppedReason)
	}
	return nil
}

func splitTaskContainers(containers []types.Container) (application, others []types.Container) {
	for _, container := range containers {
		if applicationContainerName(awssdk.ToString(container.Name)) {
			application = append(application, container)
			continue
		}
		others = append(others, container)
	}
	return application, others
}

func (s *DeploymentStore) RunMigrations(ctx context.Context, candidate Candidate) error {
	if s == nil || s.client == nil {
		return errors.New("AWS ECS deployment client is required")
	}
	if strings.TrimSpace(candidate.Cluster) == "" || strings.TrimSpace(candidate.TaskDefinitionARN) == "" || len(candidate.PrivateSubnetIDs) == 0 || strings.TrimSpace(candidate.SecurityGroupID) == "" {
		return errors.New("candidate cluster, task definition, private subnets, and security group are required")
	}
	if candidate.DatabaseGrantTaskDefinitionARN != "" {
		if err := s.runTask(ctx, candidate, candidate.DatabaseGrantTaskDefinitionARN, "database privilege grant"); err != nil {
			return err
		}
	}
	return s.runTask(ctx, candidate, candidate.TaskDefinitionARN, "migration")
}

func (s *DeploymentStore) runTask(ctx context.Context, candidate Candidate, taskDefinitionARN, label string) error {
	run, err := s.client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        awssdk.String(candidate.Cluster),
		TaskDefinition: awssdk.String(taskDefinitionARN),
		StartedBy:      awssdk.String(candidate.StartedBy),
		LaunchType:     types.LaunchTypeFargate,
		Count:          awssdk.Int32(1),
		NetworkConfiguration: &types.NetworkConfiguration{AwsvpcConfiguration: &types.AwsVpcConfiguration{
			AssignPublicIp: types.AssignPublicIpDisabled,
			Subnets:        append([]string(nil), candidate.PrivateSubnetIDs...),
			SecurityGroups: []string{candidate.SecurityGroupID},
		}},
	})
	if err != nil {
		return fmt.Errorf("run %s task: %w", label, err)
	}
	if run == nil {
		return fmt.Errorf("run %s task returned no response", label)
	}
	if len(run.Failures) > 0 {
		return fmt.Errorf("run %s task failed: %s", label, failureMessage(run.Failures))
	}
	if len(run.Tasks) != 1 || run.Tasks[0].TaskArn == nil {
		return fmt.Errorf("run %s task returned no task ARN", label)
	}
	candidate.TaskARN = awssdk.ToString(run.Tasks[0].TaskArn)
	return s.waitForTask(ctx, candidate, label)
}

func (s *DeploymentStore) runTaskTimeout(ctx context.Context, candidate Candidate, taskDefinitionARN, label string, timeout time.Duration) error {
	run, err := s.client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        awssdk.String(candidate.Cluster),
		TaskDefinition: awssdk.String(taskDefinitionARN),
		StartedBy:      awssdk.String(candidate.StartedBy),
		LaunchType:     types.LaunchTypeFargate,
		Count:          awssdk.Int32(1),
		NetworkConfiguration: &types.NetworkConfiguration{AwsvpcConfiguration: &types.AwsVpcConfiguration{
			AssignPublicIp: types.AssignPublicIpDisabled,
			Subnets:        append([]string(nil), candidate.PrivateSubnetIDs...),
			SecurityGroups: []string{candidate.SecurityGroupID},
		}},
	})
	if err != nil {
		return fmt.Errorf("run %s task: %w", label, err)
	}
	if run == nil {
		return fmt.Errorf("run %s task returned no response", label)
	}
	if len(run.Failures) > 0 {
		return fmt.Errorf("run %s task failed: %s", label, failureMessage(run.Failures))
	}
	if len(run.Tasks) != 1 || run.Tasks[0].TaskArn == nil {
		return fmt.Errorf("run %s task returned no task ARN", label)
	}
	candidate.TaskARN = awssdk.ToString(run.Tasks[0].TaskArn)
	return s.waitForTaskTimeout(ctx, candidate, label, timeout)
}

// probeWaitTimeout bounds the Magento readiness probe. Probes fail fast by
// design: unlike migrations, a probe must never run for half an hour.
const probeWaitTimeout = 5 * time.Minute

// RunProbe executes command as a one-shot probe task and deregisters the
// ephemeral task definition afterwards. It mirrors the migration candidate
// flow (deploy taskdef copy, release image, private subnets) with the probe
// command overriding the deploy container. Probe failure output is attached
// to the returned error.
func (s *DeploymentStore) RunProbe(ctx context.Context, request CandidateRequest, command []string) error {
	if s == nil || s.client == nil {
		return errors.New("AWS ECS deployment client is required")
	}
	if len(command) == 0 {
		return errors.New("probe command is required")
	}
	if err := validateCandidateRequest(request); err != nil {
		return err
	}
	described, err := s.client.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: awssdk.String(request.TaskDefinitionARN),
		Include:        []types.TaskDefinitionField{types.TaskDefinitionFieldTags},
	})
	if err != nil {
		return fmt.Errorf("describe deploy task definition: %w", err)
	}
	if described == nil || described.TaskDefinition == nil || len(described.TaskDefinition.ContainerDefinitions) == 0 {
		return errors.New("deploy task definition has no container definition")
	}
	definition := *described.TaskDefinition
	containers := append([]types.ContainerDefinition(nil), definition.ContainerDefinitions...)
	tags := append([]types.Tag(nil), described.Tags...)
	replaceApplicationImages(containers, request.ImageDigest)
	if err := overrideDeployCommand(containers, command); err != nil {
		return err
	}
	baseFamily := awssdk.ToString(definition.Family)
	if baseFamily == "" {
		return errors.New("deploy task definition has no family")
	}
	family := baseFamily + "-probe-" + time.Now().UTC().Format("20060102T150405000000000")
	probeARN, err := registerTaskDefinition(ctx, s.client, definition, family, containers, tags)
	if err != nil {
		return fmt.Errorf("register probe task definition: %w", err)
	}
	probe := Candidate{Cluster: request.Cluster, TaskDefinitionARN: probeARN, PrivateSubnetIDs: append([]string(nil), request.PrivateSubnetIDs...), SecurityGroupID: request.SecurityGroupID, StartedBy: request.StartedBy}
	if strings.TrimSpace(probe.StartedBy) == "" {
		probe.StartedBy = "magelift"
	}
	timeout := s.probeTimeout
	if timeout <= 0 {
		timeout = probeWaitTimeout
	}
	runErr := s.runTaskTimeout(ctx, probe, probeARN, "readiness probe", timeout)
	_, deregErr := s.client.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{TaskDefinition: awssdk.String(probeARN)})
	if runErr != nil {
		return fmt.Errorf("magento readiness probe: %w", runErr)
	}
	if deregErr != nil {
		return fmt.Errorf("deregister probe task definition: %w", deregErr)
	}
	return nil
}

// overrideDeployCommand replaces the deploy container command with the probe
// command. A deploy task definition without a deploy container fails loudly:
// the probe must never run an unintended entrypoint.
func overrideDeployCommand(containers []types.ContainerDefinition, command []string) error {
	for i := range containers {
		if awssdk.ToString(containers[i].Name) == "deploy" {
			containers[i].Command = append([]string(nil), command...)
			return nil
		}
	}
	return errors.New("deploy task definition has no deploy container for the probe command")
}

func (s *DeploymentStore) Cleanup(ctx context.Context, candidate Candidate) error {
	if s == nil || s.client == nil {
		return errors.New("AWS ECS deployment client is required")
	}
	definitions := []struct {
		arn   string
		label string
	}{
		{arn: candidate.TaskDefinitionARN, label: "deploy candidate"},
		{arn: candidate.DatabaseGrantTaskDefinitionARN, label: "database privilege grant"},
	}
	seen := map[string]bool{}
	for _, definition := range definitions {
		if strings.TrimSpace(definition.arn) == "" || seen[definition.arn] {
			continue
		}
		seen[definition.arn] = true
		if _, err := s.client.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{TaskDefinition: awssdk.String(definition.arn)}); err != nil {
			return fmt.Errorf("deregister %s: %w", definition.label, err)
		}
	}
	return nil
}

func (s *DeploymentStore) waitForTask(ctx context.Context, candidate Candidate, label string) error {
	timeout := s.waitTimeout
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	return s.waitForTaskTimeout(ctx, candidate, label, timeout)
}

func (s *DeploymentStore) waitForTaskTimeout(ctx context.Context, candidate Candidate, label string, timeout time.Duration) error {
	interval := s.waitInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	waitContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		output, err := s.client.DescribeTasks(waitContext, &ecs.DescribeTasksInput{Cluster: awssdk.String(candidate.Cluster), Tasks: []string{candidate.TaskARN}})
		if err != nil {
			return fmt.Errorf("describe %s task: %w", label, err)
		}
		if output == nil || len(output.Tasks) != 1 {
			return fmt.Errorf("%s task was not found", label)
		}
		task := output.Tasks[0]
		if awssdk.ToString(task.LastStatus) == "STOPPED" {
			if task.StopCode != "" && task.StopCode != types.TaskStopCodeEssentialContainerExited {
				return fmt.Errorf("%s task stopped: %s", label, awssdk.ToString(task.StoppedReason))
			}
			if err := oneOffTaskResult(label, task); err != nil {
				return err
			}
			return nil
		}
		select {
		case <-waitContext.Done():
			_, _ = s.client.StopTask(context.Background(), &ecs.StopTaskInput{Cluster: awssdk.String(candidate.Cluster), Task: awssdk.String(candidate.TaskARN), Reason: awssdk.String("MageLift " + label + " timeout")})
			return fmt.Errorf("wait for %s task: %w", label, waitContext.Err())
		case <-ticker.C:
		}
	}
}

func validateCandidateRequest(request CandidateRequest) error {
	if strings.TrimSpace(request.Cluster) == "" || strings.TrimSpace(request.TaskDefinitionARN) == "" || strings.TrimSpace(request.ImageDigest) == "" || len(request.PrivateSubnetIDs) == 0 || strings.TrimSpace(request.SecurityGroupID) == "" {
		return errors.New("candidate cluster, task definition, image digest, private subnets, and security group are required")
	}
	if !candidateDigest.MatchString(request.ImageDigest) {
		return errors.New("candidate image must be pinned by a digest")
	}
	return nil
}

func failureMessage(failures []types.Failure) string {
	parts := make([]string, 0, len(failures))
	for _, failure := range failures {
		part := awssdk.ToString(failure.Reason)
		if detail := awssdk.ToString(failure.Detail); detail != "" {
			part += ": " + detail
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, "; ")
}
