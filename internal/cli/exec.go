package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osExec "os/exec"
	"strings"

	awsoperations "github.com/acourtiol/magelift/internal/cloud/aws/operations"
	awsstack "github.com/acourtiol/magelift/internal/cloud/aws/stack"
	"github.com/spf13/cobra"
)

type execStore interface {
	SelectTask(context.Context, string, string) (awsoperations.Task, error)
}

type remoteCommandResult struct {
	Environment string `json:"environment" yaml:"environment"`
	Cluster     string `json:"cluster" yaml:"cluster"`
	Service     string `json:"service" yaml:"service"`
	Task        string `json:"task" yaml:"task"`
	Container   string `json:"container" yaml:"container"`
	Command     string `json:"command" yaml:"command"`
}

func execCommand(o *options) *cobra.Command {
	var service, container string
	var sessionOnly bool
	command := &cobra.Command{
		Use:   "exec --service web --container web -- <command>",
		Short: "Run a command through ECS Exec",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, argv, err := o.prepareRemoteCommand(cmd.Context(), service, container, args)
			if err != nil {
				return err
			}
			if sessionOnly {
				return o.write(result)
			}
			if o.runCommand == nil {
				return &exitError{code: 3, err: errors.New("AWS CLI command runner is unavailable")}
			}
			if err := o.runCommand(cmd.Context(), "", argv, o.stdout, o.stderr); err != nil {
				return &exitError{code: 3, err: fmt.Errorf("run ECS Exec command: %w", err)}
			}
			return nil
		},
	}
	command.Flags().StringVar(&service, "service", "web", "logical service: web, deploy, or cron")
	command.Flags().StringVar(&container, "container", "web", "container name")
	command.Flags().BoolVar(&sessionOnly, "session-only", false, "print the resolved session command without starting it")
	return command
}

func magentoOperationCommand(o *options, name string) *cobra.Command {
	commands := map[string]string{
		"cache-flush":  "bin/magento cache:flush",
		"reindex":      "bin/magento indexer:reindex",
		"cron-run":     "bin/magento cron:run",
		"queue-status": "bin/magento queue:consumers:list",
	}
	return &cobra.Command{
		Use:   name,
		Short: "Run the Magento " + name + " operation through ECS Exec",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// An nginx-fpm task keeps PHP in the php-fpm sidecar. FrankenPHP
			// serves the application and runs PHP in the web container.
			result, argv, err := o.prepareRemoteCommand(cmd.Context(), "web", "", strings.Fields(commands[name]))
			if err != nil {
				return err
			}
			if o.runCommand == nil {
				return &exitError{code: 3, err: errors.New("AWS CLI command runner is unavailable")}
			}
			if err := o.runCommand(cmd.Context(), "", argv, o.stdout, o.stderr); err != nil {
				return &exitError{code: 3, err: fmt.Errorf("run Magento operation: %w", err)}
			}
			_ = result
			return nil
		},
	}
}

func sshCommand(o *options) *cobra.Command {
	var service, container, commandText string
	var sessionOnly bool
	command := &cobra.Command{
		Use:   "ssh",
		Short: "Open an SSH-compatible shell through ECS Exec",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, argv, err := o.prepareRemoteCommand(cmd.Context(), service, container, []string{commandText})
			if err != nil {
				return err
			}
			if sessionOnly {
				return o.write(result)
			}
			if o.runCommand == nil {
				return &exitError{code: 3, err: errors.New("AWS CLI command runner is unavailable")}
			}
			if err := o.runCommand(cmd.Context(), "", argv, o.stdout, o.stderr); err != nil {
				return &exitError{code: 3, err: fmt.Errorf("open ECS Exec shell: %w", err)}
			}
			return nil
		},
	}
	command.Flags().StringVar(&service, "service", "web", "logical service: web, deploy, or cron")
	command.Flags().StringVar(&container, "container", "web", "container name")
	command.Flags().StringVar(&commandText, "command", "/bin/sh", "shell command to run")
	command.Flags().BoolVar(&sessionOnly, "session-only", false, "print the resolved session command without starting it")
	return command
}

func tunnelCommand() *cobra.Command {
	return &cobra.Command{Use: "tunnel", Short: "Explain why port forwarding is unavailable", Args: cobra.ArbitraryArgs, RunE: func(*cobra.Command, []string) error {
		return &exitError{code: 3, err: errors.New("tunnel is unavailable for the Fargate-only runtime: use local development or an approved private access adapter")}
	}}
}

func (o *options) prepareRemoteCommand(ctx context.Context, service, container string, command []string) (remoteCommandResult, []string, error) {
	environment, spec, outputs, err := o.runtimeOutputs(ctx)
	if err != nil {
		return remoteCommandResult{}, nil, err
	}
	service = strings.TrimSpace(service)
	container = strings.TrimSpace(container)
	if container == "" && service == "web" {
		if spec.Application.WebRuntime == "nginx-fpm" {
			container = "php-fpm"
		} else {
			container = "web"
		}
	}
	if service != "web" && service != "deploy" && service != "cron" {
		return remoteCommandResult{}, nil, invalid(fmt.Errorf("unsupported service %q", service))
	}
	if container == "" {
		return remoteCommandResult{}, nil, invalid(errors.New("container is required"))
	}
	if len(command) == 0 {
		return remoteCommandResult{}, nil, invalid(errors.New("a command is required"))
	}
	for _, part := range command {
		if strings.ContainsRune(part, '\x00') || strings.ContainsAny(part, "\r\n") {
			return remoteCommandResult{}, nil, invalid(errors.New("command arguments must not contain NUL or newline characters"))
		}
	}
	cluster := outputString(outputs, "clusterName")
	serviceName := outputString(outputs, serviceNameOutput(service))
	if cluster == "" || serviceName == "" {
		return remoteCommandResult{}, nil, &exitError{code: 3, err: fmt.Errorf("stack outputs do not contain ECS %s runtime identifiers", service)}
	}
	if o.newExec == nil {
		return remoteCommandResult{}, nil, errors.New("AWS ECS operation factory is required")
	}
	store, err := o.newExec(ctx, spec.Identity.Region)
	if err != nil {
		return remoteCommandResult{}, nil, fmt.Errorf("create AWS ECS operation client: %w", err)
	}
	task, err := store.SelectTask(ctx, cluster, serviceName)
	if err != nil {
		return remoteCommandResult{}, nil, &exitError{code: 3, err: err}
	}
	commandText := strings.Join(command, " ")
	result := remoteCommandResult{Environment: environment, Cluster: cluster, Service: serviceName, Task: task.ARN, Container: container, Command: commandText}
	argv := []string{"ecs", "execute-command", "--cluster", cluster, "--task", task.ARN, "--container", container, "--command", commandText, "--interactive"}
	return result, argv, nil
}

func (o *options) runtimeOutputs(ctx context.Context) (string, awsstack.Spec, map[string]any, error) {
	environment, spec, err := o.infrastructureSpec()
	if err != nil {
		return "", awsstack.Spec{}, nil, invalid(err)
	}
	if o.newBackend == nil {
		return "", awsstack.Spec{}, nil, errors.New("infrastructure backend factory is required")
	}
	planned := awsstack.Planned{Spec: spec}
	backend, err := o.newBackend(ctx, planned, strings.TrimSpace(o.getenv("PULUMI_BACKEND_URL")))
	if err != nil {
		return "", awsstack.Spec{}, nil, fmt.Errorf("create infrastructure backend: %w", err)
	}
	outputs, err := backend.Outputs(ctx)
	if err != nil {
		return "", awsstack.Spec{}, nil, fmt.Errorf("read infrastructure outputs: %w", err)
	}
	return environment, spec, outputs, nil
}

func serviceNameOutput(service string) string {
	switch service {
	case "web":
		return "serviceName"
	case "deploy":
		return "deployServiceName"
	case "cron":
		return "cronServiceName"
	default:
		return ""
	}
}

func outputString(outputs map[string]any, key string) string {
	value, ok := outputs[key]
	if !ok {
		return ""
	}
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value)
	case fmt.Stringer:
		return strings.TrimSpace(value.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func runAWSCommand(ctx context.Context, directory string, args []string, stdout, stderr io.Writer) error {
	command := osExec.CommandContext(ctx, "aws", args...)
	command.Dir = directory
	command.Stdin = os.Stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}
