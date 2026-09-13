package resilience

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type fakeECSProjectionAPI struct {
	executeInput   *ecs.ExecuteCommandInput
	describeInput  *ecs.DescribeTasksInput
	listInput      *ecs.ListTasksInput
	executeOutput  *ecs.ExecuteCommandOutput
	describeOutput *ecs.DescribeTasksOutput
	listOutput     *ecs.ListTasksOutput
	executeErr     error
	describeErr    error
	listErr        error
}

func (api *fakeECSProjectionAPI) ExecuteCommand(_ context.Context, input *ecs.ExecuteCommandInput, _ ...func(*ecs.Options)) (*ecs.ExecuteCommandOutput, error) {
	api.executeInput = input
	return api.executeOutput, api.executeErr
}

func (api *fakeECSProjectionAPI) DescribeTasks(_ context.Context, input *ecs.DescribeTasksInput, _ ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error) {
	api.describeInput = input
	return api.describeOutput, api.describeErr
}

func (api *fakeECSProjectionAPI) ListTasks(_ context.Context, input *ecs.ListTasksInput, _ ...func(*ecs.Options)) (*ecs.ListTasksOutput, error) {
	api.listInput = input
	return api.listOutput, api.listErr
}

type fakeECSProjectionSessionRunner struct {
	session ECSProjectionSession
	result  cloudrecovery.ProjectionCommandResult
	err     error
}

func (runner *fakeECSProjectionSessionRunner) Run(_ context.Context, session ECSProjectionSession) (cloudrecovery.ProjectionCommandResult, error) {
	runner.session = session
	if runner.err != nil {
		return cloudrecovery.ProjectionCommandResult{}, runner.err
	}
	return runner.result, nil
}

func validECSProjectionTarget() ECSProjectionTarget {
	return ECSProjectionTarget{
		Cluster: "shop-cluster", Task: "task-123", Container: "web", Region: "eu-west-3",
		Profile: "default", EndpointURL: "https://ssm.example.invalid",
	}
}

func validECSProjectionRequest(action sdk.ResilienceAction) cloudrecovery.ProjectionRequest {
	return cloudrecovery.ProjectionRequest{
		Action: action, DataClass: cloudrecovery.ProjectionSearchIndex,
		SourceReference: "runtime://source", TargetReference: "runtime://target",
		Destination: sdk.RecoverySameRegionIsolated, FixtureID: "fixture-1",
		OwnershipMarker: "magelift/test/projection", IdempotencyKey: "projection-1",
	}
}

func validECSProjectionVerifier(_ context.Context, request cloudrecovery.ProjectionRequest, _ cloudrecovery.ProjectionCommandRequest, execution cloudrecovery.ProjectionCommandResult) (cloudrecovery.ProjectionResult, error) {
	return cloudrecovery.ProjectionResult{
		Status: execution.Status, OperationID: execution.OperationID, ResourceReference: execution.ResourceReference,
		FixtureID: request.FixtureID, OwnershipMarker: request.OwnershipMarker, IdempotencyVerified: true,
		CountsVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true,
		SecretReferencesVerified: true, ServiceHealthVerified: true,
	}, nil
}

func validECSExecuteOutput() *ecs.ExecuteCommandOutput {
	return &ecs.ExecuteCommandOutput{
		ClusterArn:    awssdk.String("arn:aws:ecs:eu-west-3:123456789012:cluster/shop-cluster"),
		TaskArn:       awssdk.String("arn:aws:ecs:eu-west-3:123456789012:task/shop-cluster/task-123"),
		ContainerName: awssdk.String("web"),
		Interactive:   true,
		Session: &ecstypes.Session{
			SessionId:  awssdk.String("session-123"),
			StreamUrl:  awssdk.String("wss://ssmmessages.example.invalid/session"),
			TokenValue: awssdk.String("token-must-not-escape"),
		},
	}
}

func TestECSProjectionBackendUsesExecuteCommandAndRuntimeID(t *testing.T) {
	t.Parallel()
	api := &fakeECSProjectionAPI{
		executeOutput: validECSExecuteOutput(),
		describeOutput: &ecs.DescribeTasksOutput{Tasks: []ecstypes.Task{{Containers: []ecstypes.Container{{
			Name: awssdk.String("web"), RuntimeId: awssdk.String("runtime-456"),
		}}}}},
	}
	sessionRunner := &fakeECSProjectionSessionRunner{result: cloudrecovery.ProjectionCommandResult{
		Status: sdk.ResilienceOperationSucceeded, Stdout: []byte("known-content"), RestoreDurationSeconds: 2,
	}}
	backend, err := NewECSProjectionBackend(ECSProjectionConfig{
		Target: validECSProjectionTarget(), API: api, SessionRunner: sessionRunner, Verifier: validECSProjectionVerifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := cloudrecovery.NewProjectionLifecycle(backend)
	if err != nil {
		t.Fatal(err)
	}
	result, err := lifecycle.Rebuild(context.Background(), validECSProjectionRequest(sdk.ResilienceRestore))
	if err != nil {
		t.Fatal(err)
	}
	if api.executeInput == nil || !api.executeInput.Interactive || awssdk.ToString(api.executeInput.Command) != "bin/magento indexer:reindex" {
		t.Fatalf("ExecuteCommand input = %#v", api.executeInput)
	}
	if awssdk.ToString(api.executeInput.Cluster) != "shop-cluster" || awssdk.ToString(api.executeInput.Task) != "task-123" || awssdk.ToString(api.executeInput.Container) != "web" {
		t.Fatalf("ExecuteCommand target = %#v", api.executeInput)
	}
	if api.describeInput == nil || awssdk.ToString(api.describeInput.Cluster) != "shop-cluster" || !reflect.DeepEqual(api.describeInput.Tasks, []string{"task-123"}) {
		t.Fatalf("DescribeTasks input = %#v", api.describeInput)
	}
	if sessionRunner.session.ContainerRuntimeID != "runtime-456" || sessionRunner.session.TokenValue != "token-must-not-escape" {
		t.Fatalf("session hand-off = %#v", sessionRunner.session)
	}
	if result.OperationID != "aws-ecs-exec-session://session-123" || result.ResourceReference != "aws-ecs-task://task-123" || result.RestoreDurationSeconds != 2 {
		t.Fatalf("projection result = %#v", result)
	}
	if !containsString(result.ProofReferences, "aws.ecs.execute-command") {
		t.Fatalf("proof references = %#v", result.ProofReferences)
	}
}

func TestECSProjectionBackendResolvesServiceToDeterministicRunningTask(t *testing.T) {
	t.Parallel()
	executeOutput := validECSExecuteOutput()
	executeOutput.TaskArn = awssdk.String("arn:aws:ecs:eu-west-3:123456789012:task/shop-cluster/task-a")
	api := &fakeECSProjectionAPI{
		executeOutput: executeOutput,
		listOutput:    &ecs.ListTasksOutput{TaskArns: []string{"arn:aws:ecs:eu-west-3:123456789012:task/shop-cluster/task-b", "arn:aws:ecs:eu-west-3:123456789012:task/shop-cluster/task-a"}},
		describeOutput: &ecs.DescribeTasksOutput{Tasks: []ecstypes.Task{{Containers: []ecstypes.Container{{
			Name: awssdk.String("web"), RuntimeId: awssdk.String("runtime-a"),
		}}}}},
	}
	target := validECSProjectionTarget()
	target.Task = ""
	target.Service = "web-service"
	backend, err := NewECSProjectionBackend(ECSProjectionConfig{
		Target: target, API: api, SessionRunner: &fakeECSProjectionSessionRunner{result: cloudrecovery.ProjectionCommandResult{
			Status: sdk.ResilienceOperationSucceeded, RestoreDurationSeconds: 1,
		}}, Verifier: validECSProjectionVerifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := cloudrecovery.NewProjectionLifecycle(backend)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.Rebuild(context.Background(), validECSProjectionRequest(sdk.ResilienceRestore)); err != nil {
		t.Fatal(err)
	}
	if api.listInput == nil || awssdk.ToString(api.listInput.ServiceName) != "web-service" || api.listInput.DesiredStatus != ecstypes.DesiredStatusRunning {
		t.Fatalf("ListTasks input = %#v", api.listInput)
	}
	if api.executeInput == nil || awssdk.ToString(api.executeInput.Task) != "arn:aws:ecs:eu-west-3:123456789012:task/shop-cluster/task-a" {
		t.Fatalf("ExecuteCommand task = %#v", api.executeInput)
	}
}

func TestECSProjectionBackendRejectsIncompleteSessionBeforeDescribe(t *testing.T) {
	t.Parallel()
	api := &fakeECSProjectionAPI{executeOutput: validECSExecuteOutput()}
	api.executeOutput.Session.TokenValue = nil
	backend, err := NewECSProjectionBackend(ECSProjectionConfig{
		Target: validECSProjectionTarget(), API: api, SessionRunner: &fakeECSProjectionSessionRunner{}, Verifier: validECSProjectionVerifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = backend.Rebuild(context.Background(), validECSProjectionRequest(sdk.ResilienceRestore))
	if err == nil || !strings.Contains(err.Error(), "incomplete session") {
		t.Fatalf("error = %v", err)
	}
	if api.describeInput != nil {
		t.Fatal("DescribeTasks was called for an incomplete session")
	}
}

func TestECSProjectionBackendDoesNotExposeSessionTokenOnTransportFailure(t *testing.T) {
	t.Parallel()
	api := &fakeECSProjectionAPI{
		executeOutput:  validECSExecuteOutput(),
		describeOutput: &ecs.DescribeTasksOutput{Tasks: []ecstypes.Task{{Containers: []ecstypes.Container{{Name: awssdk.String("web"), RuntimeId: awssdk.String("runtime-456")}}}}},
	}
	backend, err := NewECSProjectionBackend(ECSProjectionConfig{
		Target: validECSProjectionTarget(), API: api,
		SessionRunner: &fakeECSProjectionSessionRunner{err: errors.New("transport unavailable")},
		Verifier:      validECSProjectionVerifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = backend.Rebuild(context.Background(), validECSProjectionRequest(sdk.ResilienceRestore))
	if err == nil || strings.Contains(err.Error(), "token-must-not-escape") {
		t.Fatalf("error = %v", err)
	}
}

type fakeSessionManagerPluginCommand struct {
	binary string
	args   []string
}

func (command *fakeSessionManagerPluginCommand) Run(_ context.Context, binary string, args []string, _ io.Reader, stdout, stderr io.Writer) error {
	command.binary = binary
	command.args = append([]string(nil), args...)
	_, _ = io.WriteString(stdout, "plugin-output")
	_, _ = io.WriteString(stderr, "plugin-diagnostic")
	return nil
}

func TestSessionManagerPluginRunnerUsesOfficialECSArguments(t *testing.T) {
	t.Parallel()
	command := &fakeSessionManagerPluginCommand{}
	runner, err := NewSessionManagerPluginRunner(SessionManagerPluginConfig{
		Binary: "plugin-test", EndpointURL: "https://ssm.example.invalid", Command: command,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), ECSProjectionSession{
		SessionID: "session-123", StreamURL: "wss://stream.example.invalid", TokenValue: "token-123",
		Region: "eu-west-3", Profile: "default", Cluster: "shop-cluster", Task: "task-123",
		Container: "web", ContainerRuntimeID: "runtime-456",
	})
	if err != nil {
		t.Fatal(err)
	}
	if command.binary != "plugin-test" || len(command.args) != 6 {
		t.Fatalf("plugin invocation = binary=%q args=%#v", command.binary, command.args)
	}
	var payload struct {
		SessionID  string `json:"SessionId"`
		StreamURL  string `json:"StreamUrl"`
		TokenValue string `json:"TokenValue"`
	}
	if err := json.Unmarshal([]byte(command.args[0]), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SessionID != "session-123" || payload.StreamURL != "wss://stream.example.invalid" || payload.TokenValue != "token-123" {
		t.Fatalf("session payload = %#v", payload)
	}
	var target struct {
		Target string `json:"Target"`
	}
	if err := json.Unmarshal([]byte(command.args[4]), &target); err != nil {
		t.Fatal(err)
	}
	if target.Target != "ecs:shop-cluster_task-123_runtime-456" || command.args[1] != "eu-west-3" || command.args[2] != "StartSession" || command.args[3] != "default" || command.args[5] != "https://ssm.example.invalid" {
		t.Fatalf("plugin arguments = %#v", command.args)
	}
	if result.Status != sdk.ResilienceOperationSucceeded || result.RestoreDurationSeconds <= 0 || string(result.Stdout) != "plugin-output" || string(result.Stderr) != "plugin-diagnostic" {
		t.Fatalf("plugin result = %#v", result)
	}
}

func TestSessionManagerPluginRunnerHonorsCancellation(t *testing.T) {
	t.Parallel()
	command := &fakeSessionManagerPluginCommand{}
	runner, err := NewSessionManagerPluginRunner(SessionManagerPluginConfig{Command: command})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = runner.Run(ctx, ECSProjectionSession{SessionID: "session", StreamURL: "stream", TokenValue: "token", Region: "eu-west-3", Cluster: "cluster", Task: "task", Container: "web", ContainerRuntimeID: "runtime"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if command.binary != "" {
		t.Fatal("plugin command ran after cancellation")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
