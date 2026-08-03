package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osExec "os/exec"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/spf13/cobra"
)

type remoteCommandResult struct {
	Environment string `json:"environment" yaml:"environment"`
	Cluster     string `json:"cluster,omitempty" yaml:"cluster,omitempty"`
	Service     string `json:"service,omitempty" yaml:"service,omitempty"`
	Task        string `json:"task,omitempty" yaml:"task,omitempty"`
	Container   string `json:"container,omitempty" yaml:"container,omitempty"`
	Command     string `json:"command" yaml:"command"`
	Launcher    string `json:"launcher,omitempty" yaml:"launcher,omitempty"`
}

func execCommand(o *options) *cobra.Command {
	var service, container string
	var sessionOnly bool
	command := &cobra.Command{
		Use:   "exec --service web --container web -- <command>",
		Short: "Run a command on a Magento workload",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, target, err := o.prepareRemoteCommand(cmd.Context(), service, container, args)
			if err != nil {
				return err
			}
			if sessionOnly {
				return o.write(result)
			}
			return o.runExecTarget(cmd.Context(), target)
		},
	}
	command.Flags().StringVar(&service, "service", "web", "logical service: web or cron")
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
		Short: "Run the Magento " + name + " operation on the web workload",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Empty container lets the adapter pick php-fpm vs web from WebRuntime.
			_, target, err := o.prepareRemoteCommand(cmd.Context(), "web", "", strings.Fields(commands[name]))
			if err != nil {
				return err
			}
			return o.runExecTarget(cmd.Context(), target)
		},
	}
}

func sshCommand(o *options) *cobra.Command {
	var service, container, commandText string
	var sessionOnly bool
	command := &cobra.Command{
		Use:   "ssh",
		Short: "Open a shell on a Magento workload",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, target, err := o.prepareRemoteCommand(cmd.Context(), service, container, []string{commandText})
			if err != nil {
				return err
			}
			if sessionOnly {
				return o.write(result)
			}
			return o.runExecTarget(cmd.Context(), target)
		},
	}
	command.Flags().StringVar(&service, "service", "web", "logical service: web or cron")
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

func (o *options) runExecTarget(ctx context.Context, target platform.ExecTarget) error {
	if o.runCommand == nil {
		return &exitError{code: 3, err: errors.New("remote command runner is unavailable")}
	}
	defer cleanupExecTempFiles(target.CleanupPaths)
	launcher := target.Launcher
	if launcher == "" {
		launcher = "aws"
	}
	args := append([]string{}, target.Args...)
	// Args are the argv after the launcher binary (AWS CLI and kubectl share this shape).
	if err := o.runCommand(ctx, launcher, args, o.stdout, o.stderr); err != nil {
		return &exitError{code: 3, err: fmt.Errorf("run remote command (%s): %w", launcher, err)}
	}
	return nil
}

func cleanupExecTempFiles(paths []string) {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		_ = os.Remove(path)
	}
}

func (o *options) prepareRemoteCommand(ctx context.Context, service, container string, command []string) (remoteCommandResult, platform.ExecTarget, error) {
	environment, planned, outputs, err := o.plannedOutputs(ctx)
	if err != nil {
		return remoteCommandResult{}, platform.ExecTarget{}, err
	}
	service = strings.TrimSpace(service)
	container = strings.TrimSpace(container)
	if service == "deploy" {
		return remoteCommandResult{}, platform.ExecTarget{}, invalid(errors.New("exec does not support --service deploy; migrate runs as a one-off candidate task. Use logs --service deploy, or exec against web/cron"))
	}
	if service != "web" && service != "cron" {
		return remoteCommandResult{}, platform.ExecTarget{}, invalid(fmt.Errorf("unsupported service %q (want web or cron)", service))
	}
	if len(command) == 0 {
		return remoteCommandResult{}, platform.ExecTarget{}, invalid(errors.New("a command is required"))
	}
	for _, part := range command {
		if strings.ContainsRune(part, '\x00') || strings.ContainsAny(part, "\r\n") {
			return remoteCommandResult{}, platform.ExecTarget{}, invalid(errors.New("command arguments must not contain NUL or newline characters"))
		}
	}
	observe, err := o.runtimeObserve()
	if err != nil {
		return remoteCommandResult{}, platform.ExecTarget{}, err
	}
	target, err := observe.PrepareExec(ctx, planned, outputs, platform.ExecQuery{
		Workload:  sdk.WorkloadID(service),
		Container: container,
		Command:   command,
	})
	if err != nil {
		if mapped := notSupported(err, planned, "exec"); mapped != err {
			return remoteCommandResult{}, platform.ExecTarget{}, mapped
		}
		return remoteCommandResult{}, platform.ExecTarget{}, &exitError{code: 3, err: err}
	}
	result := remoteCommandResult{
		Environment: environment,
		Cluster:     target.Cluster,
		Task:        target.Task,
		Container:   target.Container,
		Command:     strings.Join(command, " "),
		Launcher:    target.Launcher,
	}
	return result, target, nil
}

func (o *options) plannedOutputs(ctx context.Context) (string, platform.PlannedStack, map[string]any, error) {
	environment, planned, err := o.planStack(false)
	if err != nil {
		return "", nil, nil, invalid(err)
	}
	if o.newBackend == nil {
		return "", nil, nil, errors.New("infrastructure backend factory is required")
	}
	backend, err := o.newBackend(ctx, planned, strings.TrimSpace(o.getenv("PULUMI_BACKEND_URL")))
	if err != nil {
		return "", nil, nil, fmt.Errorf("create infrastructure backend: %w", err)
	}
	outputs, err := backend.Outputs(ctx)
	if err != nil {
		return "", nil, nil, fmt.Errorf("read infrastructure outputs: %w", err)
	}
	return environment, planned, outputs, nil
}

func runRemoteCommand(ctx context.Context, binary string, args []string, stdout, stderr io.Writer) error {
	if binary == "" {
		binary = "aws"
	}
	command := osExec.CommandContext(ctx, binary, args...)
	command.Stdin = os.Stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}
