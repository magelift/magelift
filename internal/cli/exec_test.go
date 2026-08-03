package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type recordingExecObserve struct {
	query  platform.ExecQuery
	target platform.ExecTarget
}

func (r *recordingExecObserve) TailLogs(context.Context, platform.PlannedStack, platform.LogQuery) ([]platform.LogEvent, error) {
	return nil, platform.ErrNotSupported
}
func (r *recordingExecObserve) CheckRuntime(context.Context, platform.PlannedStack, map[string]any) ([]platform.RuntimeHealth, error) {
	return nil, platform.ErrNotSupported
}
func (r *recordingExecObserve) PrepareExec(_ context.Context, _ platform.PlannedStack, outputs map[string]any, query platform.ExecQuery) (platform.ExecTarget, error) {
	r.query = query
	if _, ok := outputs["serviceName"]; !ok && (query.Workload == "" || query.Workload == "web") {
		return platform.ExecTarget{}, errors.New("stack outputs do not contain ECS web runtime identifiers")
	}
	if r.target.Launcher != "" {
		return r.target, nil
	}
	container := query.Container
	if container == "" {
		container = "php-fpm"
	}
	return platform.ExecTarget{
		Launcher: "aws",
		Args: []string{
			"ecs", "execute-command",
			"--cluster", "shop-cluster",
			"--task", "task-a",
			"--container", container,
			"--command", strings.Join(query.Command, " "),
			"--interactive",
		},
		Cluster: "shop-cluster", Task: "task-a", Container: container,
	}, nil
}

func TestExecBuildsPinnedAWSCLICommandFromStackOutputs(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}
	var output bytes.Buffer
	var captured []string
	observe := &recordingExecObserve{target: platform.ExecTarget{
		Launcher: "aws",
		Args:     []string{"ecs", "execute-command", "--cluster", "shop-cluster", "--task", "arn:aws:ecs:eu-west-3:123456789012:task/shop/task-a", "--container", "web", "--command", "bin/magento cache:flush", "--interactive"},
		Cluster:  "shop-cluster", Task: "arn:aws:ecs:eu-west-3:123456789012:task/shop/task-a", Container: "web",
	}}
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.output = path, "staging", "json"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.testRuntimeObserve = observe
	o.runCommand = func(_ context.Context, _ string, args []string, _, _ io.Writer) error {
		captured = append([]string(nil), args...)
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "exec", "--service", "web", "--container", "web", "--", "bin/magento", "cache:flush"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []string{"ecs", "execute-command", "--cluster", "shop-cluster", "--task", "arn:aws:ecs:eu-west-3:123456789012:task/shop/task-a", "--container", "web", "--command", "bin/magento cache:flush", "--interactive"}
	if !reflect.DeepEqual(captured, want) {
		t.Fatalf("args = %#v, want %#v", captured, want)
	}
	if observe.query.Workload != sdk.WorkloadID("web") {
		t.Fatalf("workload = %q", observe.query.Workload)
	}
}

func TestExecRejectsMissingRuntimeOutputAndUnsafeCommand(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster"}}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.testRuntimeObserve = &recordingExecObserve{}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "exec", "--", "echo", "ok"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "runtime identifiers") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}

	o = testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}, nil
	}
	o.testRuntimeObserve = &recordingExecObserve{}
	command = newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "exec", "--", "echo\nno"})
	err = command.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "newline") {
		t.Fatalf("unsafe command error=%v code=%d", err, ExitCode(err))
	}
}

func TestMagentoOperationTargetsPHPContainer(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}
	var captured []string
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.testRuntimeObserve = &recordingExecObserve{}
	o.runCommand = func(_ context.Context, _ string, args []string, _, _ io.Writer) error {
		captured = append([]string(nil), args...)
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "cache-flush"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []string{"ecs", "execute-command", "--cluster", "shop-cluster", "--task", "task-a", "--container", "php-fpm", "--command", "bin/magento cache:flush", "--interactive"}
	if !reflect.DeepEqual(captured, want) {
		t.Fatalf("Magento operation args = %#v, want %#v", captured, want)
	}
}

func TestMagentoOperationTargetsFrankenPHPWebContainer(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("webRuntime: nginx-fpm"), []byte("webRuntime: frankenphp-classic"), 1)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}
	var captured []string
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.testRuntimeObserve = frankenObserve{}
	o.runCommand = func(_ context.Context, _ string, args []string, _, _ io.Writer) error {
		captured = append([]string(nil), args...)
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "cache-flush"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := captured[7]; got != "web" {
		t.Fatalf("FrankenPHP Magento operation container = %q, want web; args = %#v", got, captured)
	}
}

type frankenObserve struct{}

func (frankenObserve) TailLogs(context.Context, platform.PlannedStack, platform.LogQuery) ([]platform.LogEvent, error) {
	return nil, platform.ErrNotSupported
}
func (frankenObserve) CheckRuntime(context.Context, platform.PlannedStack, map[string]any) ([]platform.RuntimeHealth, error) {
	return nil, platform.ErrNotSupported
}
func (frankenObserve) PrepareExec(_ context.Context, _ platform.PlannedStack, _ map[string]any, query platform.ExecQuery) (platform.ExecTarget, error) {
	container := query.Container
	if container == "" {
		container = "web"
	}
	return platform.ExecTarget{
		Launcher: "aws",
		Args:     []string{"ecs", "execute-command", "--cluster", "shop-cluster", "--task", "task-a", "--container", container, "--command", strings.Join(query.Command, " "), "--interactive"},
		Cluster:  "shop-cluster", Task: "task-a", Container: container,
	}, nil
}

func TestSSHUsesECSExecAsTheSupportedPath(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}
	var output bytes.Buffer
	var captured []string
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.output = path, "staging", "json"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.testRuntimeObserve = &recordingExecObserve{target: platform.ExecTarget{
		Launcher: "aws",
		Args:     []string{"ecs", "execute-command", "--cluster", "shop-cluster", "--task", "task-a", "--container", "web", "--command", "/bin/bash", "--interactive"},
		Cluster:  "shop-cluster", Task: "task-a", Container: "web",
	}}
	o.runCommand = func(_ context.Context, _ string, args []string, _, _ io.Writer) error { captured = args; return nil }
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "ssh", "--command", "/bin/bash"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(captured, []string{"ecs", "execute-command", "--cluster", "shop-cluster", "--task", "task-a", "--container", "web", "--command", "/bin/bash", "--interactive"}) {
		t.Fatalf("ssh args = %#v", captured)
	}
}

func TestExecRunsKubectlLauncherBinary(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{outputs: map[string]any{
		"clusterName": "shop-cluster",
		"serviceName": "shop-web",
		"kubeconfig":  "apiVersion: v1\nkind: Config\n",
	}}
	var capturedBinary string
	var capturedArgs []string
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.testRuntimeObserve = &recordingExecObserve{target: platform.ExecTarget{
		Launcher: "kubectl",
		Args:     []string{"exec", "-n", "default", "-it", "deploy/shop-web", "--", "/bin/sh"},
		Cluster:  "shop-cluster", Task: "shop-web",
	}}
	o.runCommand = func(_ context.Context, binary string, args []string, _, _ io.Writer) error {
		capturedBinary = binary
		capturedArgs = append([]string(nil), args...)
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "exec", "--service", "web", "--", "/bin/sh"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if capturedBinary != "kubectl" {
		t.Fatalf("binary = %q, want kubectl (empty-binary path breaks kubectl)", capturedBinary)
	}
	want := []string{"exec", "-n", "default", "-it", "deploy/shop-web", "--", "/bin/sh"}
	if !reflect.DeepEqual(capturedArgs, want) {
		t.Fatalf("args = %#v, want %#v", capturedArgs, want)
	}
}

func TestTunnelExplainsTheFargateBoundary(t *testing.T) {
	err := tunnelCommand().Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "Fargate-only") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
}
