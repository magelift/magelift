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
)

// ECSAPI is the ECS control-plane surface needed to select a running task for
// ECS Exec. The interactive data channel is deliberately handled by the AWS
// CLI, which is the supported client for that protocol.
type ECSAPI interface {
	ListTasks(context.Context, *ecs.ListTasksInput, ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
}

type Task struct {
	ARN string `json:"arn" yaml:"arn"`
}

type ExecStore struct {
	client ECSAPI
}

func NewExec(ctx context.Context, region string) (*ExecStore, error) {
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
	return NewExecFromClient(ecs.NewFromConfig(cfg, options...))
}

func NewExecFromClient(client ECSAPI) (*ExecStore, error) {
	if client == nil {
		return nil, errors.New("AWS ECS client is required")
	}
	return &ExecStore{client: client}, nil
}

// SelectTask picks a deterministic running task from the requested service.
// ECS does not guarantee task ordering, so sorting is part of the contract.
func (s *ExecStore) SelectTask(ctx context.Context, cluster, service string) (Task, error) {
	if s == nil || s.client == nil {
		return Task{}, errors.New("AWS ECS client is required")
	}
	cluster = strings.TrimSpace(cluster)
	service = strings.TrimSpace(service)
	if cluster == "" || service == "" {
		return Task{}, errors.New("ECS cluster and service are required")
	}
	output, err := s.client.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster: awssdk.String(cluster), ServiceName: awssdk.String(service),
		DesiredStatus: "RUNNING",
	})
	if err != nil {
		return Task{}, fmt.Errorf("list running ECS tasks: %w", err)
	}
	if output == nil || len(output.TaskArns) == 0 {
		return Task{}, fmt.Errorf("no running ECS tasks found for service %q", service)
	}
	arns := append([]string(nil), output.TaskArns...)
	sort.Strings(arns)
	return Task{ARN: arns[0]}, nil
}
