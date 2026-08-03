package operations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// RuntimeAPI is the read-only ECS surface used by runtime health checks.
type RuntimeAPI interface {
	DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	ListTasks(context.Context, *ecs.ListTasksInput, ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
	DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
}

type TaskHealth struct {
	ARN          string `json:"arn" yaml:"arn"`
	LastStatus   string `json:"lastStatus" yaml:"lastStatus"`
	HealthStatus string `json:"healthStatus,omitempty" yaml:"healthStatus,omitempty"`
}

type ServiceHealth struct {
	Cluster        string       `json:"cluster" yaml:"cluster"`
	Service        string       `json:"service" yaml:"service"`
	DesiredCount   int32        `json:"desiredCount" yaml:"desiredCount"`
	RunningCount   int32        `json:"runningCount" yaml:"runningCount"`
	PendingCount   int32        `json:"pendingCount" yaml:"pendingCount"`
	Deployments    int          `json:"deployments" yaml:"deployments"`
	PrimaryRollout string       `json:"primaryRollout,omitempty" yaml:"primaryRollout,omitempty"`
	Tasks          []TaskHealth `json:"tasks" yaml:"tasks"`
}

type RuntimeStore struct {
	client RuntimeAPI
}

func NewRuntime(ctx context.Context, region string) (*RuntimeStore, error) {
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
		options = append(options, func(options *ecs.Options) {
			options.BaseEndpoint = awssdk.String(endpoint)
		})
	}
	return NewRuntimeFromClient(ecs.NewFromConfig(cfg, options...))
}

func NewRuntimeFromClient(client RuntimeAPI) (*RuntimeStore, error) {
	if client == nil {
		return nil, errors.New("AWS ECS runtime client is required")
	}
	return &RuntimeStore{client: client}, nil
}

func (s *RuntimeStore) Check(ctx context.Context, cluster, service string) (ServiceHealth, error) {
	if s == nil || s.client == nil {
		return ServiceHealth{}, errors.New("AWS ECS runtime client is required")
	}
	cluster = strings.TrimSpace(cluster)
	service = strings.TrimSpace(service)
	if cluster == "" || service == "" {
		return ServiceHealth{}, errors.New("ECS cluster and service are required")
	}
	services, err := s.client.DescribeServices(ctx, &ecs.DescribeServicesInput{Cluster: awssdk.String(cluster), Services: []string{service}})
	if err != nil {
		return ServiceHealth{}, fmt.Errorf("describe ECS service: %w", err)
	}
	if services == nil || len(services.Services) != 1 {
		return ServiceHealth{}, fmt.Errorf("ECS service %q was not found", service)
	}
	value := services.Services[0]
	result := ServiceHealth{
		Cluster:      cluster,
		Service:      service,
		DesiredCount: value.DesiredCount,
		RunningCount: value.RunningCount,
		PendingCount: value.PendingCount,
		Deployments:  len(value.Deployments),
		Tasks:        []TaskHealth{},
	}
	for _, deployment := range value.Deployments {
		if awssdk.ToString(deployment.Status) == "PRIMARY" {
			result.PrimaryRollout = string(deployment.RolloutState)
			break
		}
	}
	tasks, err := s.client.ListTasks(ctx, &ecs.ListTasksInput{Cluster: awssdk.String(cluster), ServiceName: awssdk.String(service), DesiredStatus: types.DesiredStatusRunning})
	if err != nil {
		return ServiceHealth{}, fmt.Errorf("list running ECS tasks: %w", err)
	}
	if tasks == nil || len(tasks.TaskArns) == 0 {
		return result, nil
	}
	arns := append([]string(nil), tasks.TaskArns...)
	sort.Strings(arns)
	described, err := s.client.DescribeTasks(ctx, &ecs.DescribeTasksInput{Cluster: awssdk.String(cluster), Tasks: arns})
	if err != nil {
		return ServiceHealth{}, fmt.Errorf("describe ECS tasks: %w", err)
	}
	if described == nil {
		return result, nil
	}
	for _, task := range described.Tasks {
		result.Tasks = append(result.Tasks, TaskHealth{ARN: awssdk.ToString(task.TaskArn), LastStatus: awssdk.ToString(task.LastStatus), HealthStatus: string(task.HealthStatus)})
	}
	sort.Slice(result.Tasks, func(i, j int) bool { return result.Tasks[i].ARN < result.Tasks[j].ARN })
	return result, nil
}
