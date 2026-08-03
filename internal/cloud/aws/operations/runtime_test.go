package operations

import (
	"context"
	"errors"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type fakeRuntime struct {
	services *ecs.DescribeServicesOutput
	tasks    *ecs.ListTasksOutput
	describe *ecs.DescribeTasksOutput
	err      error
}

func (f *fakeRuntime) DescribeServices(context.Context, *ecs.DescribeServicesInput, ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	return f.services, f.err
}
func (f *fakeRuntime) ListTasks(context.Context, *ecs.ListTasksInput, ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	return f.tasks, f.err
}
func (f *fakeRuntime) DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	return f.describe, f.err
}

func TestRuntimeCheckReportsDeterministicServiceAndTaskHealth(t *testing.T) {
	client := &fakeRuntime{
		services: &ecs.DescribeServicesOutput{Services: []types.Service{{DesiredCount: 2, RunningCount: 2, PendingCount: 0, Deployments: []types.Deployment{{Status: awssdk.String("PRIMARY"), RolloutState: types.DeploymentRolloutStateCompleted}}}}},
		tasks:    &ecs.ListTasksOutput{TaskArns: []string{"task-b", "task-a"}},
		describe: &ecs.DescribeTasksOutput{Tasks: []types.Task{{TaskArn: awssdk.String("task-b"), LastStatus: awssdk.String("RUNNING"), HealthStatus: types.HealthStatusHealthy}, {TaskArn: awssdk.String("task-a"), LastStatus: awssdk.String("RUNNING"), HealthStatus: types.HealthStatusHealthy}}},
	}
	store, err := NewRuntimeFromClient(client)
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.Check(context.Background(), "cluster", "service")
	if err != nil {
		t.Fatal(err)
	}
	if result.DesiredCount != 2 || result.RunningCount != 2 || result.PrimaryRollout != string(types.DeploymentRolloutStateCompleted) || len(result.Tasks) != 2 || result.Tasks[0].ARN != "task-a" {
		t.Fatalf("health = %#v", result)
	}
}

func TestRuntimeCheckSurfacesMissingServiceAndClientErrors(t *testing.T) {
	store, err := NewRuntimeFromClient(&fakeRuntime{services: &ecs.DescribeServicesOutput{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Check(context.Background(), "cluster", "service"); err == nil {
		t.Fatal("expected missing service error")
	}
	store, err = NewRuntimeFromClient(&fakeRuntime{err: errors.New("denied")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Check(context.Background(), "cluster", "service"); err == nil {
		t.Fatal("expected API error")
	}
}
