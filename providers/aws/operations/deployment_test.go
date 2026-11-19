package operations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

type deploymentMock struct {
	definition              *types.TaskDefinition
	tags                    []types.Tag
	described               *ecs.DescribeTaskDefinitionInput
	registered              *ecs.RegisterTaskDefinitionInput
	registrations           []*ecs.RegisterTaskDefinitionInput
	describes               int
	stopped                 bool
	deregistered            string
	deregisteredDefinitions []string
	runTaskDefinitions      []string
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
	m.registrations = append(m.registrations, input)
	arn := fmt.Sprintf("arn:aws:ecs:eu-west-3:123:task-definition/shop-candidate:%d", len(m.registrations))
	return &ecs.RegisterTaskDefinitionOutput{TaskDefinition: &types.TaskDefinition{TaskDefinitionArn: awssdk.String(arn)}}, nil
}
func (m *deploymentMock) RunTask(_ context.Context, input *ecs.RunTaskInput, _ ...func(*ecs.Options)) (*ecs.RunTaskOutput, error) {
	m.runTaskDefinitions = append(m.runTaskDefinitions, awssdk.ToString(input.TaskDefinition))
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
	m.deregisteredDefinitions = append(m.deregisteredDefinitions, awssdk.ToString(input.TaskDefinition))
	return &ecs.DeregisterTaskDefinitionOutput{}, nil
}

func databaseGrantTestContainer() types.ContainerDefinition {
	return types.ContainerDefinition{
		Name:  awssdk.String("deploy"),
		Image: awssdk.String("ghcr.io/example/shop@sha256:" + strings.Repeat("b", 64)),
		Environment: []types.KeyValuePair{
			{Name: awssdk.String("MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME"), Value: awssdk.String("magento")},
			{Name: awssdk.String("MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST"), Value: awssdk.String("db.example")},
			{Name: awssdk.String("MAGENTO_DC_DB__CONNECTION__DEFAULT__PORT"), Value: awssdk.String("3306")},
		},
		Secrets: []types.Secret{
			{Name: awssdk.String("MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD"), ValueFrom: awssdk.String("password-secret")},
			{Name: awssdk.String("MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME"), ValueFrom: awssdk.String("username-secret")},
		},
		HealthCheck:  &types.HealthCheck{Command: []string{"CMD-SHELL", "true"}},
		PortMappings: []types.PortMapping{{ContainerPort: awssdk.Int32(8080)}},
		DependsOn:    []types.ContainerDependency{{ContainerName: awssdk.String("db"), Condition: types.ContainerConditionHealthy}},
	}
}

func TestDatabaseGrantContainerUsesDeployDatabaseInputs(t *testing.T) {
	grant, ok := databaseGrantContainer(types.TaskDefinition{ContainerDefinitions: []types.ContainerDefinition{databaseGrantTestContainer()}})
	if !ok {
		t.Fatal("database grant container was not created")
	}
	if got := awssdk.ToString(grant.Name); got != "database-grant" {
		t.Fatalf("grant container name = %q", got)
	}
	if got := awssdk.ToString(grant.Image); got != databaseClientImage {
		t.Fatalf("grant container image = %q, want %q", got, databaseClientImage)
	}
	if len(grant.EntryPoint) != 2 || grant.EntryPoint[0] != "/bin/sh" || grant.EntryPoint[1] != "-ec" {
		t.Fatalf("grant container entrypoint = %#v", grant.EntryPoint)
	}
	if len(grant.Command) != 1 || !strings.Contains(grant.Command[0], "GRANT ALL PRIVILEGES ON") {
		t.Fatalf("grant container command = %#v", grant.Command)
	}
	if grant.HealthCheck != nil || len(grant.PortMappings) != 0 || len(grant.DependsOn) != 0 {
		t.Fatalf("grant container retained runtime-only settings: %#v", grant)
	}
}

func TestDatabaseGrantContainerRequiresAllDatabaseInputs(t *testing.T) {
	container := databaseGrantTestContainer()
	container.Secrets = container.Secrets[:1]
	if hasDatabaseGrantInputs(container) {
		t.Fatal("database grant inputs accepted without the username secret")
	}
	container = databaseGrantTestContainer()
	container.Environment = container.Environment[:2]
	if hasDatabaseGrantInputs(container) {
		t.Fatal("database grant inputs accepted without the database port")
	}
}

func TestRegisterCandidateRegistersDatabaseGrantDefinition(t *testing.T) {
	mock := &deploymentMock{definition: &types.TaskDefinition{
		Family:               awssdk.String("shop-deploy"),
		Cpu:                  awssdk.String("512"),
		Memory:               awssdk.String("1024"),
		ContainerDefinitions: []types.ContainerDefinition{databaseGrantTestContainer()},
	}}
	store, err := NewDeploymentFromClient(mock)
	if err != nil {
		t.Fatal(err)
	}
	newImage := "ghcr.io/example/shop@sha256:" + strings.Repeat("a", 64)
	candidate, err := store.RegisterCandidate(context.Background(), CandidateRequest{
		Cluster: "shop-cluster", TaskDefinitionARN: "arn:aws:ecs:eu-west-3:123:task-definition/shop-deploy:1",
		ImageDigest: newImage, PrivateSubnetIDs: []string{"subnet-a"}, SecurityGroupID: "sg-web",
	})
	if err != nil {
		t.Fatal(err)
	}
	if candidate.DatabaseGrantTaskDefinitionARN == "" || len(mock.registrations) != 2 {
		t.Fatalf("candidate grant registration = candidate=%#v registrations=%d", candidate, len(mock.registrations))
	}
	if got := awssdk.ToString(mock.registrations[0].ContainerDefinitions[0].Image); got != newImage {
		t.Fatalf("candidate image = %q, want %q", got, newImage)
	}
	grant := mock.registrations[1]
	if got := awssdk.ToString(grant.Family); !strings.HasSuffix(got, "-db-grant") {
		t.Fatalf("grant family = %q", got)
	}
	if len(grant.ContainerDefinitions) != 1 || awssdk.ToString(grant.ContainerDefinitions[0].Name) != "database-grant" {
		t.Fatalf("grant registration containers = %#v", grant.ContainerDefinitions)
	}
}

func probeTestDefinition() *types.TaskDefinition {
	return &types.TaskDefinition{
		Family: awssdk.String("shop-deploy"), Cpu: awssdk.String("512"), Memory: awssdk.String("1024"),
		NetworkMode: types.NetworkModeAwsvpc, RequiresCompatibilities: []types.Compatibility{types.CompatibilityFargate},
		ContainerDefinitions: []types.ContainerDefinition{{
			Name: awssdk.String("deploy"), Image: awssdk.String("ghcr.io/example/shop@sha256:" + strings.Repeat("b", 64)),
		}},
	}
}

func probeTestRequest() CandidateRequest {
	return CandidateRequest{
		Cluster: "shop-cluster", TaskDefinitionARN: "arn:aws:ecs:eu-west-3:123:task-definition/shop-deploy:1",
		ImageDigest:      "ghcr.io/example/shop@sha256:" + strings.Repeat("a", 64),
		PrivateSubnetIDs: []string{"subnet-a"}, SecurityGroupID: "sg-web", StartedBy: "magelift",
	}
}

func TestRunProbeOverridesDeployCommand(t *testing.T) {
	mock := &deploymentMock{definition: probeTestDefinition()}
	store, err := NewDeploymentFromClient(mock)
	if err != nil {
		t.Fatal(err)
	}
	store.waitInterval = time.Millisecond
	command := []string{"/bin/sh", "-ec", "bin/magento setup:db:status"}
	if err := store.RunProbe(context.Background(), probeTestRequest(), command); err != nil {
		t.Fatalf("RunProbe: %v", err)
	}
	if len(mock.registrations) != 1 {
		t.Fatalf("registrations = %d, want 1 probe task definition", len(mock.registrations))
	}
	registered := mock.registrations[0]
	if got := awssdk.ToString(registered.Family); !strings.Contains(got, "-probe-") {
		t.Fatalf("probe family = %q", got)
	}
	if got := registered.ContainerDefinitions[0].Command; strings.Join(got, " ") != strings.Join(command, " ") {
		t.Fatalf("probe command = %q", got)
	}
	if len(mock.runTaskDefinitions) != 1 || len(mock.deregisteredDefinitions) != 1 {
		t.Fatalf("run=%v deregistered=%v", mock.runTaskDefinitions, mock.deregisteredDefinitions)
	}
}

func TestRunProbeFailurePropagates(t *testing.T) {
	mock := &deploymentMock{definition: probeTestDefinition()}
	store, err := NewDeploymentFromClient(failedTaskMock{mock})
	if err != nil {
		t.Fatal(err)
	}
	store.waitInterval = time.Millisecond
	err = store.RunProbe(context.Background(), probeTestRequest(), []string{"/bin/sh", "-ec", "bin/magento setup:db:status"})
	if err == nil || !strings.Contains(err.Error(), "magento readiness probe") {
		t.Fatalf("RunProbe error = %v, want probe failure attached", err)
	}
	if len(mock.deregisteredDefinitions) != 1 {
		t.Fatalf("failed probe task definition was not deregistered: %v", mock.deregisteredDefinitions)
	}
}

func TestRunProbeRequiresDeployContainer(t *testing.T) {
	definition := probeTestDefinition()
	definition.ContainerDefinitions[0].Name = awssdk.String("web")
	mock := &deploymentMock{definition: definition}
	store, err := NewDeploymentFromClient(mock)
	if err != nil {
		t.Fatal(err)
	}
	err = store.RunProbe(context.Background(), probeTestRequest(), []string{"/bin/sh", "-ec", "true"})
	if err == nil || !strings.Contains(err.Error(), "no deploy container") {
		t.Fatalf("RunProbe error = %v, want missing-deploy-container refusal", err)
	}
}

func TestRunProbeTimeoutStopsAndCleansUp(t *testing.T) {
	if probeWaitTimeout != 5*time.Minute {
		t.Fatalf("probeWaitTimeout = %s, want 5m", probeWaitTimeout)
	}
	mock := &deploymentMock{definition: probeTestDefinition()}
	store, err := NewDeploymentFromClient(hangingTaskMock{mock})
	if err != nil {
		t.Fatal(err)
	}
	store.waitInterval = time.Millisecond
	store.probeTimeout = 20 * time.Millisecond
	err = store.RunProbe(context.Background(), probeTestRequest(), []string{"/bin/sh", "-ec", "sleep 3600"})
	if err == nil || !strings.Contains(err.Error(), "magento readiness probe") {
		t.Fatalf("RunProbe error = %v, want bounded probe timeout", err)
	}
	if !mock.stopped {
		t.Error("hung probe task was not stopped on timeout")
	}
	if len(mock.deregisteredDefinitions) != 1 {
		t.Errorf("timed-out probe task definition was not deregistered: %v", mock.deregisteredDefinitions)
	}
}

func TestRunMigrationsRunsDatabaseGrantBeforeMigration(t *testing.T) {
	mock := &deploymentMock{}
	store, err := NewDeploymentFromClient(mock)
	if err != nil {
		t.Fatal(err)
	}
	store.waitInterval = time.Millisecond
	store.waitTimeout = time.Second
	candidate := Candidate{Cluster: "shop-cluster", TaskDefinitionARN: "migration-definition", DatabaseGrantTaskDefinitionARN: "grant-definition", PrivateSubnetIDs: []string{"subnet-a"}, SecurityGroupID: "sg-web", StartedBy: "magelift"}
	if err := store.RunMigrations(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(mock.runTaskDefinitions, ","), "grant-definition,migration-definition"; got != want {
		t.Fatalf("task definition execution order = %q, want %q", got, want)
	}
	if err := store.Cleanup(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(mock.deregisteredDefinitions, ","), "migration-definition,grant-definition"; got != want {
		t.Fatalf("task definition cleanup order = %q, want %q", got, want)
	}
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

func TestOneOffTaskResultIgnoresSidecarExitAfterDeploySuccess(t *testing.T) {
	t.Parallel()
	task := types.Task{
		StoppedReason: awssdk.String("Essential container in task exited"),
		Containers: []types.Container{
			{Name: awssdk.String("deploy"), ExitCode: awssdk.Int32(0)},
			{Name: awssdk.String("search-proxy"), ExitCode: awssdk.Int32(2)},
		},
	}
	if err := oneOffTaskResult("migration", task); err != nil {
		t.Fatalf("deploy exit 0 should succeed despite sidecar: %v", err)
	}
}

func TestOneOffTaskResultFailsWhenDeployExitsNonZero(t *testing.T) {
	t.Parallel()
	task := types.Task{
		StoppedReason: awssdk.String("Essential container in task exited"),
		Containers: []types.Container{
			{Name: awssdk.String("deploy"), ExitCode: awssdk.Int32(1), Reason: awssdk.String("magento setup:upgrade failed")},
			{Name: awssdk.String("search-proxy"), ExitCode: awssdk.Int32(0)},
		},
	}
	err := oneOffTaskResult("migration", task)
	if err == nil || !strings.Contains(err.Error(), "setup:upgrade failed") {
		t.Fatalf("deploy exit 1 = %v", err)
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
