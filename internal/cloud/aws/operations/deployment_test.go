package operations

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type deploymentMock struct {
	definition   *types.TaskDefinition
	tags         []types.Tag
	described    *ecs.DescribeTaskDefinitionInput
	registered   *ecs.RegisterTaskDefinitionInput
	describes    int
	stopped      bool
	deregistered string
}

type runFailureMock struct{ *deploymentMock }

func (m runFailureMock) RunTask(context.Context, *ecs.RunTaskInput, ...func(*ecs.Options)) (*ecs.RunTaskOutput, error) {
	return nil, errors.New("injected ECS capacity failure")
}

type failedTaskMock struct{ *deploymentMock }

func (m failedTaskMock) DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	return &ecs.DescribeTasksOutput{Tasks: []types.Task{{TaskArn: awssdk.String("arn:aws:ecs:eu-west-3:123:task/shop-migration"), LastStatus: awssdk.String("STOPPED"), StopCode: types.TaskStopCodeEssentialContainerExited, StoppedReason: awssdk.String("exit 1"), Containers: []types.Container{{ExitCode: awssdk.Int32(1), Reason: awssdk.String("magento setup:upgrade failed")}}}}}, nil
}

type hangingTaskMock struct{ *deploymentMock }

func (m hangingTaskMock) DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	return &ecs.DescribeTasksOutput{Tasks: []types.Task{{TaskArn: awssdk.String("arn:aws:ecs:eu-west-3:123:task/shop-migration"), LastStatus: awssdk.String("RUNNING")}}}, nil
}

func (m *deploymentMock) DescribeTaskDefinition(_ context.Context, input *ecs.DescribeTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error) {
	m.described = input
	return &ecs.DescribeTaskDefinitionOutput{TaskDefinition: m.definition, Tags: m.tags}, nil
}
func (m *deploymentMock) RegisterTaskDefinition(_ context.Context, input *ecs.RegisterTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error) {
	m.registered = input
	return &ecs.RegisterTaskDefinitionOutput{TaskDefinition: &types.TaskDefinition{TaskDefinitionArn: awssdk.String("arn:aws:ecs:eu-west-3:123:task-definition/shop-candidate:1")}}, nil
}
func (*deploymentMock) RunTask(context.Context, *ecs.RunTaskInput, ...func(*ecs.Options)) (*ecs.RunTaskOutput, error) {
	return &ecs.RunTaskOutput{Tasks: []types.Task{{TaskArn: awssdk.String("arn:aws:ecs:eu-west-3:123:task/shop-migration")}}}, nil
}
func (m *deploymentMock) DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	m.describes++
	if m.describes == 1 {
		return &ecs.DescribeTasksOutput{Tasks: []types.Task{{TaskArn: awssdk.String("arn:aws:ecs:eu-west-3:123:task/shop-migration"), LastStatus: awssdk.String("RUNNING")}}}, nil
	}
	return &ecs.DescribeTasksOutput{Tasks: []types.Task{{TaskArn: awssdk.String("arn:aws:ecs:eu-west-3:123:task/shop-migration"), LastStatus: awssdk.String("STOPPED"), StopCode: types.TaskStopCodeEssentialContainerExited, Containers: []types.Container{{ExitCode: awssdk.Int32(0)}}}}}, nil
}
func (m *deploymentMock) StopTask(context.Context, *ecs.StopTaskInput, ...func(*ecs.Options)) (*ecs.StopTaskOutput, error) {
	m.stopped = true
	return &ecs.StopTaskOutput{}, nil
}
func (m *deploymentMock) DeregisterTaskDefinition(_ context.Context, input *ecs.DeregisterTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.DeregisterTaskDefinitionOutput, error) {
	m.deregistered = awssdk.ToString(input.TaskDefinition)
	return &ecs.DeregisterTaskDefinitionOutput{}, nil
}

func TestDeploymentCandidateRunsMigrationsAndCleansUp(t *testing.T) {
	mock := &deploymentMock{definition: &types.TaskDefinition{
		Family: awssdk.String("shop-deploy"), Cpu: awssdk.String("512"), Memory: awssdk.String("1024"),
		EnableFaultInjection: awssdk.Bool(true),
		EphemeralStorage:     &types.EphemeralStorage{SizeInGiB: 50},
		ExecutionRoleArn:     awssdk.String("arn:aws:iam::123:role/execution"), TaskRoleArn: awssdk.String("arn:aws:iam::123:role/task"),
		InferenceAccelerators: []types.InferenceAccelerator{{DeviceName: awssdk.String("device"), DeviceType: awssdk.String("device-type")}},
		IpcMode:               types.IpcModeNone,
		NetworkMode:           types.NetworkModeAwsvpc, RequiresCompatibilities: []types.Compatibility{types.CompatibilityFargate},
		PidMode:              types.PidModeTask,
		PlacementConstraints: []types.TaskDefinitionPlacementConstraint{{Type: types.TaskDefinitionPlacementConstraintTypeMemberOf, Expression: awssdk.String("attribute:ecs.instance-type =~ t3.*")}},
		ProxyConfiguration:   &types.ProxyConfiguration{ContainerName: awssdk.String("proxy")},
		ContainerDefinitions: []types.ContainerDefinition{{Name: awssdk.String("deploy"), Image: awssdk.String("ghcr.io/example/shop@sha256:" + strings.Repeat("b", 64))}},
	}, tags: []types.Tag{{Key: awssdk.String("owner"), Value: awssdk.String("magelift")}}}
	store, err := NewDeploymentFromClient(mock)
	if err != nil {
		t.Fatal(err)
	}
	store.waitInterval = time.Millisecond
	store.waitTimeout = time.Second
	candidate, err := store.RegisterCandidate(context.Background(), CandidateRequest{
		Cluster: "shop-cluster", TaskDefinitionARN: "arn:aws:ecs:eu-west-3:123:task-definition/shop-deploy:1",
		ImageDigest: "ghcr.io/example/shop@sha256:" + strings.Repeat("a", 64), PrivateSubnetIDs: []string{"subnet-a", "subnet-b"}, SecurityGroupID: "sg-web",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mock.described == nil || len(mock.described.Include) != 1 || mock.described.Include[0] != types.TaskDefinitionFieldTags {
		t.Fatalf("candidate description did not request tags: %#v", mock.described)
	}
	if mock.registered == nil || awssdk.ToString(mock.registered.ContainerDefinitions[0].Image) != "ghcr.io/example/shop@sha256:"+strings.Repeat("a", 64) || strings.Contains(awssdk.ToString(mock.registered.Family), ".") {
		t.Fatalf("candidate registration = %#v", mock.registered)
	}
	if !awssdk.ToBool(mock.registered.EnableFaultInjection) || mock.registered.EphemeralStorage.SizeInGiB != 50 || len(mock.registered.Tags) != 1 || len(mock.registered.PlacementConstraints) != 1 || mock.registered.ProxyConfiguration == nil {
		t.Fatalf("candidate task settings were not preserved: %#v", mock.registered)
	}
	if err := store.RunMigrations(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if err := store.Cleanup(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if mock.deregistered != candidate.TaskDefinitionARN || mock.stopped {
		t.Fatalf("candidate cleanup = deregistered=%q stopped=%v", mock.deregistered, mock.stopped)
	}
}

func TestRegisterCandidateUpdatesAllApplicationContainers(t *testing.T) {
	oldImage := "ghcr.io/example/shop@sha256:" + strings.Repeat("b", 64)
	newImage := "ghcr.io/example/shop@sha256:" + strings.Repeat("a", 64)
	mock := &deploymentMock{definition: &types.TaskDefinition{
		Family:               awssdk.String("shop-deploy"),
		EnableFaultInjection: awssdk.Bool(true), EphemeralStorage: &types.EphemeralStorage{SizeInGiB: 30},
		IpcMode: types.IpcModeNone, PidMode: types.PidModeTask,
		ContainerDefinitions: []types.ContainerDefinition{
			{Name: awssdk.String("php-fpm"), Image: awssdk.String(oldImage)},
			{Name: awssdk.String("web"), Image: awssdk.String(oldImage)},
			{Name: awssdk.String("varnish"), Image: awssdk.String("public.ecr.aws/varnish:7")},
		},
	}, tags: []types.Tag{{Key: awssdk.String("magelift:role"), Value: awssdk.String("deploy")}}}
	store, err := NewDeploymentFromClient(mock)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.RegisterCandidate(context.Background(), CandidateRequest{
		Cluster: "shop-cluster", TaskDefinitionARN: "arn:aws:ecs:eu-west-3:123:task-definition/shop-deploy:1",
		ImageDigest: newImage, PrivateSubnetIDs: []string{"subnet-a"}, SecurityGroupID: "sg-web",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mock.registered == nil || len(mock.registered.ContainerDefinitions) != 3 {
		t.Fatalf("registered containers = %#v", mock.registered)
	}
	if mock.registered.EnableFaultInjection == nil || !awssdk.ToBool(mock.registered.EnableFaultInjection) || mock.registered.EphemeralStorage == nil || mock.registered.EphemeralStorage.SizeInGiB != 30 || mock.registered.IpcMode != types.IpcModeNone || mock.registered.PidMode != types.PidModeTask || len(mock.registered.Tags) != 1 {
		t.Fatalf("candidate task settings were not preserved: %#v", mock.registered)
	}
	if got := awssdk.ToString(mock.registered.ContainerDefinitions[0].Image); got != newImage {
		t.Fatalf("php-fpm image = %q, want %q", got, newImage)
	}
	if got := awssdk.ToString(mock.registered.ContainerDefinitions[1].Image); got != newImage {
		t.Fatalf("web image = %q, want %q", got, newImage)
	}
	if got := awssdk.ToString(mock.registered.ContainerDefinitions[2].Image); got != "public.ecr.aws/varnish:7" {
		t.Fatalf("varnish image = %q, sidecar was unexpectedly changed", got)
	}
}

func TestReplaceApplicationImagesPreservesUnknownFirstSidecar(t *testing.T) {
	oldImage := "ghcr.io/example/shop@sha256:" + strings.Repeat("b", 64)
	newImage := "ghcr.io/example/shop@sha256:" + strings.Repeat("a", 64)
	containers := []types.ContainerDefinition{
		{Name: awssdk.String("metrics"), Image: awssdk.String("public.ecr.aws/aws-observability/aws-otel-collector:latest")},
		{Name: awssdk.String("php-fpm"), Image: awssdk.String(oldImage)},
		{Name: awssdk.String("web"), Image: awssdk.String(oldImage)},
	}
	replaceApplicationImages(containers, newImage)
	if got := awssdk.ToString(containers[0].Image); got != "public.ecr.aws/aws-observability/aws-otel-collector:latest" {
		t.Fatalf("metrics sidecar image = %q, sidecar was unexpectedly changed", got)
	}
	for _, index := range []int{1, 2} {
		if got := awssdk.ToString(containers[index].Image); got != newImage {
			t.Fatalf("container %d image = %q, want %q", index, got, newImage)
		}
	}
}

func TestDeploymentRejectsUnpinnedCandidateImage(t *testing.T) {
	store, err := NewDeploymentFromClient(&deploymentMock{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.RegisterCandidate(context.Background(), CandidateRequest{Cluster: "cluster", TaskDefinitionARN: "definition", ImageDigest: "latest", PrivateSubnetIDs: []string{"subnet"}, SecurityGroupID: "sg"})
	if err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeploymentReportsRunTaskFailure(t *testing.T) {
	base := &deploymentMock{definition: &types.TaskDefinition{Family: awssdk.String("shop-deploy")}}
	store, err := NewDeploymentFromClient(runFailureMock{deploymentMock: base})
	if err != nil {
		t.Fatal(err)
	}
	err = store.RunMigrations(context.Background(), Candidate{Cluster: "shop-cluster", TaskDefinitionARN: "candidate", PrivateSubnetIDs: []string{"subnet-a"}, SecurityGroupID: "sg-web", StartedBy: "magelift"})
	if err == nil || !strings.Contains(err.Error(), "capacity failure") {
		t.Fatalf("run task failure = %v", err)
	}
}

func TestDeploymentReportsFailedMigrationContainer(t *testing.T) {
	base := &deploymentMock{}
	store, err := NewDeploymentFromClient(failedTaskMock{deploymentMock: base})
	if err != nil {
		t.Fatal(err)
	}
	err = store.RunMigrations(context.Background(), Candidate{Cluster: "shop-cluster", TaskDefinitionARN: "candidate", TaskARN: "migration", PrivateSubnetIDs: []string{"subnet-a"}, SecurityGroupID: "sg-web", StartedBy: "magelift"})
	if err == nil || !strings.Contains(err.Error(), "setup:upgrade failed") {
		t.Fatalf("failed migration = %v", err)
	}
}

func TestDeploymentStopsTimedOutMigration(t *testing.T) {
	base := &deploymentMock{}
	store, err := NewDeploymentFromClient(hangingTaskMock{deploymentMock: base})
	if err != nil {
		t.Fatal(err)
	}
	store.waitInterval = time.Millisecond
	store.waitTimeout = 5 * time.Millisecond
	err = store.RunMigrations(context.Background(), Candidate{Cluster: "shop-cluster", TaskDefinitionARN: "candidate", TaskARN: "migration", PrivateSubnetIDs: []string{"subnet-a"}, SecurityGroupID: "sg-web", StartedBy: "magelift"})
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("migration timeout = %v", err)
	}
	if !base.stopped {
		t.Fatal("timed-out migration was not stopped")
	}
}
