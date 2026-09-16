package resilience

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"sort"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

// ECSProjectionAPI is the smallest AWS ECS control-plane surface required by
// ECS Exec. The DescribeTasks call is not optional: the AWS-supported
// session-manager-plugin invocation needs the container runtime ID in its
// Target parameter.
type ECSProjectionAPI interface {
	ExecuteCommand(context.Context, *ecs.ExecuteCommandInput, ...func(*ecs.Options)) (*ecs.ExecuteCommandOutput, error)
	DescribeTasks(context.Context, *ecs.DescribeTasksInput, ...func(*ecs.Options)) (*ecs.DescribeTasksOutput, error)
}

// ECSProjectionTaskLister is an optional extension used when a projection
// target names a service instead of an ephemeral task. Pinned-task callers
// can continue implementing only ECSProjectionAPI.
type ECSProjectionTaskLister interface {
	ListTasks(context.Context, *ecs.ListTasksInput, ...func(*ecs.Options)) (*ecs.ListTasksOutput, error)
}

// ECSProjectionTarget is provider-local workload identity. Service is the
// normal durable target and resolves to one deterministic running task at
// execution time; Task is available for an explicitly pinned acceptance or
// failure-drill target. None of these fields enters the provider-neutral
// projection request.
type ECSProjectionTarget struct {
	Cluster     string
	Service     string
	Task        string
	Container   string
	Region      string
	Profile     string
	EndpointURL string
}

func (target ECSProjectionTarget) validate() error {
	for name, value := range map[string]string{
		"cluster": target.Cluster, "container": target.Container, "region": target.Region,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("AWS ECS projection %s is required and must be single-line", name)
		}
	}
	if (strings.TrimSpace(target.Service) == "") == (strings.TrimSpace(target.Task) == "") {
		return errors.New("AWS ECS projection requires exactly one of service or task")
	}
	for name, value := range map[string]string{"service": target.Service, "task": target.Task} {
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("AWS ECS projection %s must be single-line", name)
		}
	}
	if strings.ContainsAny(target.Profile, "\r\n\x00") {
		return errors.New("AWS ECS projection profile must be single-line")
	}
	_, err := validateEndpoint(target.EndpointURL)
	return err
}

// ECSProjectionConfig injects the AWS API, optional session transport, and
// application verifier. A nil SessionRunner selects the local
// session-manager-plugin implementation.
type ECSProjectionConfig struct {
	Target        ECSProjectionTarget
	API           ECSProjectionAPI
	SessionRunner ECSProjectionSessionRunner
	Plugin        SessionManagerPluginConfig
	Verifier      cloudrecovery.ProjectionVerifier
}

// ECSProjectionBackend shares lifecycle and proof semantics with Kubernetes
// and future runtimes. It contains no AWS SDK response in the portable core.
type ECSProjectionBackend struct {
	*cloudrecovery.CommandProjectionBackend
}

var _ cloudrecovery.ProjectionBackend = (*ECSProjectionBackend)(nil)

// NewECSProjectionBackend constructs the ECS adapter around injected ports.
// The separate constructor keeps fake-client certification deterministic and
// lets community implementations provide an ECS-compatible API.
func NewECSProjectionBackend(config ECSProjectionConfig) (*ECSProjectionBackend, error) {
	if err := config.Target.validate(); err != nil {
		return nil, err
	}
	if config.API == nil {
		return nil, errors.New("AWS ECS projection API is required")
	}
	sessionRunner := config.SessionRunner
	if sessionRunner == nil {
		var err error
		sessionRunner, err = NewSessionManagerPluginRunner(config.Plugin)
		if err != nil {
			return nil, err
		}
	}
	runner := &ecsProjectionCommandRunner{target: config.Target, api: config.API, sessionRunner: sessionRunner}
	backend, err := cloudrecovery.NewCommandProjectionBackend(runner, config.Verifier)
	if err != nil {
		return nil, err
	}
	return &ECSProjectionBackend{CommandProjectionBackend: backend}, nil
}

// NewAWSECSProjectionBackend loads the official AWS SDK and retains the
// session-manager-plugin boundary for the interactive data channel.
func NewAWSECSProjectionBackend(ctx context.Context, config ECSProjectionConfig, opts ...func(*awsconfig.LoadOptions) error) (*ECSProjectionBackend, error) {
	if ctx == nil {
		return nil, errors.New("AWS ECS projection context is required")
	}
	if err := config.Target.validate(); err != nil {
		return nil, err
	}
	loadOptions := append([]func(*awsconfig.LoadOptions) error(nil), opts...)
	loadOptions = append(loadOptions, awsconfig.WithRegion(config.Target.Region))
	if strings.TrimSpace(config.Target.Profile) != "" {
		loadOptions = append(loadOptions, awsconfig.WithSharedConfigProfile(config.Target.Profile))
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load AWS ECS projection configuration: %w", err)
	}
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	ecsOptions := make([]func(*ecs.Options), 0, 1)
	if endpoint != "" {
		ecsOptions = append(ecsOptions, func(options *ecs.Options) {
			options.BaseEndpoint = awssdk.String(endpoint)
		})
		if strings.TrimSpace(config.Target.EndpointURL) == "" {
			config.Target.EndpointURL = endpoint
		}
	}
	config.API = ecs.NewFromConfig(awsConfig, ecsOptions...)
	return NewECSProjectionBackend(config)
}

type ecsProjectionCommandRunner struct {
	target        ECSProjectionTarget
	api           ECSProjectionAPI
	sessionRunner ECSProjectionSessionRunner
}

func (runner *ecsProjectionCommandRunner) Run(ctx context.Context, request cloudrecovery.ProjectionCommandRequest) (cloudrecovery.ProjectionCommandResult, error) {
	if runner == nil || runner.api == nil || runner.sessionRunner == nil {
		return cloudrecovery.ProjectionCommandResult{}, errors.New("AWS ECS projection command runner is not configured")
	}
	if ctx == nil {
		return cloudrecovery.ProjectionCommandResult{}, errors.New("AWS ECS projection command context is required")
	}
	if err := ctx.Err(); err != nil {
		return cloudrecovery.ProjectionCommandResult{}, err
	}
	command, err := projectionCommandText(request.Command)
	if err != nil {
		return cloudrecovery.ProjectionCommandResult{}, err
	}
	task, err := runner.taskForExecution(ctx)
	if err != nil {
		return cloudrecovery.ProjectionCommandResult{}, err
	}
	output, err := runner.api.ExecuteCommand(ctx, &ecs.ExecuteCommandInput{
		Cluster:     awssdk.String(runner.target.Cluster),
		Task:        awssdk.String(task),
		Container:   awssdk.String(runner.target.Container),
		Command:     awssdk.String(command),
		Interactive: true,
	})
	if err != nil {
		return cloudrecovery.ProjectionCommandResult{}, fmt.Errorf("execute AWS ECS command: %w", err)
	}
	session, err := runner.sessionFromOutput(output)
	if err != nil {
		return cloudrecovery.ProjectionCommandResult{}, err
	}
	describe, err := runner.api.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: awssdk.String(session.Cluster), Tasks: []string{session.Task},
	})
	if err != nil {
		return cloudrecovery.ProjectionCommandResult{}, fmt.Errorf("describe AWS ECS task for exec transport: %w", err)
	}
	session.ContainerRuntimeID, err = runtimeID(describe, session.Container)
	if err != nil {
		return cloudrecovery.ProjectionCommandResult{}, err
	}
	result, err := runner.sessionRunner.Run(ctx, session)
	if err != nil {
		return cloudrecovery.ProjectionCommandResult{}, fmt.Errorf("run AWS session-manager-plugin: %w", err)
	}
	if result.Status != sdk.ResilienceOperationSucceeded {
		return cloudrecovery.ProjectionCommandResult{}, fmt.Errorf("AWS session-manager-plugin returned unsupported status %q", result.Status)
	}
	result.OperationID = "aws-ecs-exec-session://" + session.SessionID
	result.ResourceReference = "aws-ecs-task://" + session.Task
	result.ProofReferences = append(result.ProofReferences, "aws.ecs.execute-command")
	result.Reason = "AWS ECS Exec projection command completed"
	return result, nil
}

func (runner *ecsProjectionCommandRunner) taskForExecution(ctx context.Context) (string, error) {
	if task := strings.TrimSpace(runner.target.Task); task != "" {
		return task, nil
	}
	lister, ok := runner.api.(ECSProjectionTaskLister)
	if !ok {
		return "", errors.New("AWS ECS projection service target requires an ECS ListTasks API")
	}
	output, err := lister.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster: awssdk.String(runner.target.Cluster), ServiceName: awssdk.String(runner.target.Service),
		DesiredStatus: ecstypes.DesiredStatusRunning, MaxResults: awssdk.Int32(100),
	})
	if err != nil {
		return "", fmt.Errorf("list AWS ECS running projection tasks: %w", err)
	}
	if output == nil || len(output.TaskArns) == 0 {
		return "", fmt.Errorf("AWS ECS projection service %q has no running tasks", runner.target.Service)
	}
	tasks := make([]string, 0, len(output.TaskArns))
	for _, arn := range output.TaskArns {
		value := strings.TrimSpace(arn)
		if value == "" || strings.ContainsAny(value, "\r\n\x00") {
			continue
		}
		tasks = append(tasks, value)
	}
	if len(tasks) == 0 {
		return "", fmt.Errorf("AWS ECS projection service %q returned no valid running task identities", runner.target.Service)
	}
	sort.Strings(tasks)
	return tasks[0], nil
}

func (runner *ecsProjectionCommandRunner) sessionFromOutput(output *ecs.ExecuteCommandOutput) (ECSProjectionSession, error) {
	if output == nil || output.Session == nil {
		return ECSProjectionSession{}, errors.New("AWS ECS ExecuteCommand returned no session")
	}
	cluster, err := requiredECSIdentifier(output.ClusterArn, runner.target.Cluster, "cluster")
	if err != nil {
		return ECSProjectionSession{}, err
	}
	task, err := requiredECSIdentifier(output.TaskArn, runner.target.Task, "task")
	if err != nil {
		return ECSProjectionSession{}, err
	}
	container := strings.TrimSpace(awssdk.ToString(output.ContainerName))
	if container == "" {
		container = runner.target.Container
	}
	if container == "" || strings.ContainsAny(container, "\r\n\x00") {
		return ECSProjectionSession{}, errors.New("AWS ECS ExecuteCommand returned no valid container name")
	}
	session := ECSProjectionSession{
		SessionID:   requiredSessionField(output.Session.SessionId, "session ID"),
		StreamURL:   requiredSessionField(output.Session.StreamUrl, "stream URL"),
		TokenValue:  requiredSessionField(output.Session.TokenValue, "session token"),
		Region:      runner.target.Region,
		Profile:     runner.target.Profile,
		EndpointURL: runner.target.EndpointURL,
		Cluster:     cluster,
		Task:        task,
		Container:   container,
	}
	if session.SessionID == "" || session.StreamURL == "" || session.TokenValue == "" {
		return ECSProjectionSession{}, errors.New("AWS ECS ExecuteCommand returned an incomplete session")
	}
	return session, nil
}

func requiredECSIdentifier(value *string, fallback, name string) (string, error) {
	identifier := strings.TrimSpace(awssdk.ToString(value))
	if identifier == "" {
		identifier = strings.TrimSpace(fallback)
	}
	if identifier == "" || strings.ContainsAny(identifier, "\r\n\x00") {
		return "", fmt.Errorf("AWS ECS ExecuteCommand returned no valid %s", name)
	}
	if index := strings.LastIndexByte(identifier, '/'); index >= 0 {
		identifier = identifier[index+1:]
	}
	return identifier, nil
}

func requiredSessionField(value *string, name string) string {
	field := strings.TrimSpace(awssdk.ToString(value))
	if field == "" || strings.ContainsAny(field, "\r\n\x00") {
		return ""
	}
	return field
}

func runtimeID(output *ecs.DescribeTasksOutput, containerName string) (string, error) {
	if output == nil || len(output.Tasks) != 1 {
		return "", errors.New("AWS ECS task description did not return exactly one task")
	}
	for _, container := range output.Tasks[0].Containers {
		if strings.TrimSpace(awssdk.ToString(container.Name)) != containerName {
			continue
		}
		runtime := strings.TrimSpace(awssdk.ToString(container.RuntimeId))
		if runtime == "" || strings.ContainsAny(runtime, "\r\n\x00") {
			return "", errors.New("AWS ECS task container has no valid runtime ID")
		}
		return runtime, nil
	}
	return "", fmt.Errorf("AWS ECS task does not contain exec container %q", containerName)
}

func projectionCommandText(command []string) (string, error) {
	if len(command) == 0 {
		return "", errors.New("AWS ECS projection command is required")
	}
	for _, part := range command {
		if strings.ContainsAny(part, "\r\n\x00") {
			return "", errors.New("AWS ECS projection command arguments must not contain newline or NUL characters")
		}
	}
	return strings.TrimSpace(strings.Join(command, " ")), nil
}

// ECSProjectionSession is the short-lived native session hand-off. TokenValue
// is never returned in a result, error, log, or evidence object.
type ECSProjectionSession struct {
	SessionID          string
	StreamURL          string
	TokenValue         string
	Region             string
	Profile            string
	EndpointURL        string
	Cluster            string
	Task               string
	Container          string
	ContainerRuntimeID string
}

// ECSProjectionSessionRunner owns the interactive transport after AWS
// ExecuteCommand has created a session.
type ECSProjectionSessionRunner interface {
	Run(context.Context, ECSProjectionSession) (cloudrecovery.ProjectionCommandResult, error)
}

// SessionManagerPluginCommand is the narrow process boundary used by the
// concrete session-manager-plugin runner. Tests can replace it without a
// shell, network, or real session.
type SessionManagerPluginCommand interface {
	Run(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error
}

// SessionManagerPluginConfig configures the locally installed AWS-supported
// transport. It contains no credentials or session values.
type SessionManagerPluginConfig struct {
	Binary      string
	EndpointURL string
	Command     SessionManagerPluginCommand
	Stdin       io.Reader
}

// SessionManagerPluginRunner invokes the official ECS Exec hand-off shape
// used by the AWS CLI: session JSON, region, StartSession, profile, ECS target
// parameters, and the SSM endpoint.
type SessionManagerPluginRunner struct {
	binary      string
	endpointURL string
	command     SessionManagerPluginCommand
	stdin       io.Reader
}

func NewSessionManagerPluginRunner(config SessionManagerPluginConfig) (*SessionManagerPluginRunner, error) {
	binary := strings.TrimSpace(config.Binary)
	if binary == "" {
		binary = "session-manager-plugin"
	}
	if strings.ContainsAny(binary, "\r\n\x00") {
		return nil, errors.New("session-manager-plugin binary must be a single-line path")
	}
	endpoint, err := validateEndpoint(config.EndpointURL)
	if err != nil {
		return nil, err
	}
	command := config.Command
	if command == nil {
		command = osSessionManagerPluginCommand{}
	}
	return &SessionManagerPluginRunner{binary: binary, endpointURL: endpoint, command: command, stdin: config.Stdin}, nil
}

func (runner *SessionManagerPluginRunner) Run(ctx context.Context, session ECSProjectionSession) (cloudrecovery.ProjectionCommandResult, error) {
	if runner == nil || runner.command == nil {
		return cloudrecovery.ProjectionCommandResult{}, errors.New("session-manager-plugin runner is not configured")
	}
	if ctx == nil {
		return cloudrecovery.ProjectionCommandResult{}, errors.New("session-manager-plugin context is required")
	}
	if err := ctx.Err(); err != nil {
		return cloudrecovery.ProjectionCommandResult{}, err
	}
	if err := session.validate(); err != nil {
		return cloudrecovery.ProjectionCommandResult{}, err
	}
	endpoint := runner.endpointURL
	if endpoint == "" {
		endpoint = strings.TrimSpace(session.EndpointURL)
	}
	if endpoint == "" {
		var err error
		endpoint, err = defaultSSMEndpoint(ctx, session.Region)
		if err != nil {
			return cloudrecovery.ProjectionCommandResult{}, err
		}
	}
	payload, err := json.Marshal(struct {
		SessionID  string `json:"SessionId"`
		StreamURL  string `json:"StreamUrl"`
		TokenValue string `json:"TokenValue"`
	}{SessionID: session.SessionID, StreamURL: session.StreamURL, TokenValue: session.TokenValue})
	if err != nil {
		return cloudrecovery.ProjectionCommandResult{}, fmt.Errorf("encode ECS Exec session: %w", err)
	}
	targetPayload, err := json.Marshal(struct {
		Target string `json:"Target"`
	}{Target: "ecs:" + session.Cluster + "_" + session.Task + "_" + session.ContainerRuntimeID})
	if err != nil {
		return cloudrecovery.ProjectionCommandResult{}, fmt.Errorf("encode ECS Exec target: %w", err)
	}
	stdin := runner.stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	var stdout, stderr bytes.Buffer
	started := time.Now()
	if err := runner.command.Run(ctx, runner.binary, []string{string(payload), session.Region, "StartSession", session.Profile, string(targetPayload), endpoint}, stdin, &stdout, &stderr); err != nil {
		clear(stdout.Bytes())
		clear(stderr.Bytes())
		return cloudrecovery.ProjectionCommandResult{}, fmt.Errorf("session-manager-plugin process failed: %w", err)
	}
	elapsed := time.Since(started)
	duration := int64(elapsed / time.Second)
	if elapsed%time.Second != 0 {
		duration++
	}
	return cloudrecovery.ProjectionCommandResult{
		Status:                 sdk.ResilienceOperationSucceeded,
		Stdout:                 append([]byte(nil), stdout.Bytes()...),
		Stderr:                 append([]byte(nil), stderr.Bytes()...),
		RestoreDurationSeconds: duration,
		Reason:                 "AWS session-manager-plugin completed the ECS Exec command",
	}, nil
}

func (session ECSProjectionSession) validate() error {
	for name, value := range map[string]string{
		"session ID": session.SessionID, "stream URL": session.StreamURL, "session token": session.TokenValue,
		"region": session.Region, "cluster": session.Cluster, "task": session.Task,
		"container": session.Container, "container runtime ID": session.ContainerRuntimeID,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			if name == "session token" {
				return errors.New("AWS ECS Exec session token is missing or invalid")
			}
			return fmt.Errorf("AWS ECS Exec %s is missing or invalid", name)
		}
	}
	if strings.ContainsAny(session.Profile, "\r\n\x00") {
		return errors.New("AWS ECS profile must be single-line")
	}
	return nil
}

func validateEndpoint(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return "", errors.New("AWS SSM endpoint must be an HTTP(S) URL")
	}
	return value, nil
}

func defaultSSMEndpoint(ctx context.Context, region string) (string, error) {
	endpoint, err := ssm.NewDefaultEndpointResolverV2().ResolveEndpoint(ctx, ssm.EndpointParameters{Region: awssdk.String(region)})
	if err != nil {
		return "", fmt.Errorf("resolve AWS SSM endpoint: %w", err)
	}
	return endpoint.URI.String(), nil
}

type osSessionManagerPluginCommand struct{}

func (osSessionManagerPluginCommand) Run(ctx context.Context, binary string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	command := exec.CommandContext(ctx, binary, args...)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

var _ ECSProjectionSessionRunner = (*SessionManagerPluginRunner)(nil)
