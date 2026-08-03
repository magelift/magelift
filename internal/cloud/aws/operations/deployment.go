package operations

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

var candidateDigest = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)

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
	Cluster           string
	TaskDefinitionARN string
	TaskARN           string
	PrivateSubnetIDs  []string
	SecurityGroupID   string
	StartedBy         string
}

type DeploymentStore struct {
	client       DeploymentAPI
	waitInterval time.Duration
	waitTimeout  time.Duration
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
	registered, err := s.client.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
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
		return Candidate{}, fmt.Errorf("register deploy candidate: %w", err)
	}
	if registered == nil || registered.TaskDefinition == nil || registered.TaskDefinition.TaskDefinitionArn == nil {
		return Candidate{}, errors.New("register deploy candidate returned no task definition ARN")
	}
	startedBy := request.StartedBy
	if strings.TrimSpace(startedBy) == "" {
		startedBy = "magelift"
	}
	return Candidate{Cluster: request.Cluster, TaskDefinitionARN: awssdk.ToString(registered.TaskDefinition.TaskDefinitionArn), PrivateSubnetIDs: append([]string(nil), request.PrivateSubnetIDs...), SecurityGroupID: request.SecurityGroupID, StartedBy: startedBy}, nil
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

func (s *DeploymentStore) RunMigrations(ctx context.Context, candidate Candidate) error {
	if s == nil || s.client == nil {
		return errors.New("AWS ECS deployment client is required")
	}
	if strings.TrimSpace(candidate.Cluster) == "" || strings.TrimSpace(candidate.TaskDefinitionARN) == "" || len(candidate.PrivateSubnetIDs) == 0 || strings.TrimSpace(candidate.SecurityGroupID) == "" {
		return errors.New("candidate cluster, task definition, private subnets, and security group are required")
	}
	run, err := s.client.RunTask(ctx, &ecs.RunTaskInput{
		Cluster:        awssdk.String(candidate.Cluster),
		TaskDefinition: awssdk.String(candidate.TaskDefinitionARN),
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
		return fmt.Errorf("run migration task: %w", err)
	}
	if run == nil {
		return errors.New("run migration task returned no response")
	}
	if len(run.Failures) > 0 {
		return fmt.Errorf("run migration task failed: %s", failureMessage(run.Failures))
	}
	if len(run.Tasks) != 1 || run.Tasks[0].TaskArn == nil {
		return errors.New("run migration task returned no task ARN")
	}
	candidate.TaskARN = awssdk.ToString(run.Tasks[0].TaskArn)
	return s.waitForTask(ctx, candidate)
}

func (s *DeploymentStore) Cleanup(ctx context.Context, candidate Candidate) error {
	if s == nil || s.client == nil {
		return errors.New("AWS ECS deployment client is required")
	}
	if strings.TrimSpace(candidate.TaskDefinitionARN) == "" {
		return nil
	}
	if _, err := s.client.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{TaskDefinition: awssdk.String(candidate.TaskDefinitionARN)}); err != nil {
		return fmt.Errorf("deregister deploy candidate: %w", err)
	}
	return nil
}

func (s *DeploymentStore) waitForTask(ctx context.Context, candidate Candidate) error {
	interval, timeout := s.waitInterval, s.waitTimeout
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
			return fmt.Errorf("describe migration task: %w", err)
		}
		if output == nil || len(output.Tasks) != 1 {
			return errors.New("migration task was not found")
		}
		task := output.Tasks[0]
		if awssdk.ToString(task.LastStatus) == "STOPPED" {
			if task.StopCode != "" && task.StopCode != types.TaskStopCodeEssentialContainerExited {
				return fmt.Errorf("migration task stopped: %s", awssdk.ToString(task.StoppedReason))
			}
			for _, container := range task.Containers {
				if container.ExitCode == nil || awssdk.ToInt32(container.ExitCode) != 0 {
					return fmt.Errorf("migration container failed: %s", awssdk.ToString(container.Reason))
				}
			}
			return nil
		}
		select {
		case <-waitContext.Done():
			_, _ = s.client.StopTask(context.Background(), &ecs.StopTaskInput{Cluster: awssdk.String(candidate.Cluster), Task: awssdk.String(candidate.TaskARN), Reason: awssdk.String("MageLift migration timeout")})
			return fmt.Errorf("wait for migration task: %w", waitContext.Err())
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
