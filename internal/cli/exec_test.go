package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	awsoperations "github.com/acourtiol/magelift/internal/cloud/aws/operations"
	awsstack "github.com/acourtiol/magelift/internal/cloud/aws/stack"
)

type fakeExecStore struct {
	task awsoperations.Task
}

func (f fakeExecStore) SelectTask(context.Context, string, string) (awsoperations.Task, error) {
	return f.task, nil
}

func TestExecBuildsPinnedAWSCLICommandFromStackOutputs(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}
	var output bytes.Buffer
	var captured []string
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.output = path, "staging", "json"
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newExec = func(context.Context, string) (execStore, error) {
		return fakeExecStore{task: awsoperations.Task{ARN: "arn:aws:ecs:eu-west-3:123456789012:task/shop/task-a"}}, nil
	}
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
}

func TestExecRejectsMissingRuntimeOutputAndUnsafeCommand(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster"}}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newExec = func(context.Context, string) (execStore, error) {
		return fakeExecStore{task: awsoperations.Task{ARN: "task"}}, nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "exec", "--", "echo", "ok"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "runtime identifiers") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}

	o = testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}, nil
	}
	o.newExec = func(context.Context, string) (execStore, error) {
		return fakeExecStore{task: awsoperations.Task{ARN: "task"}}, nil
	}
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
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newExec = func(context.Context, string) (execStore, error) {
		return fakeExecStore{task: awsoperations.Task{ARN: "task-a"}}, nil
	}
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
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newExec = func(context.Context, string) (execStore, error) {
		return fakeExecStore{task: awsoperations.Task{ARN: "task-a"}}, nil
	}
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

func TestSSHUsesECSExecAsTheSupportedPath(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}
	var output bytes.Buffer
	var captured []string
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.output = path, "staging", "json"
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newExec = func(context.Context, string) (execStore, error) {
		return fakeExecStore{task: awsoperations.Task{ARN: "task-a"}}, nil
	}
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

func TestTunnelExplainsTheFargateBoundary(t *testing.T) {
	err := tunnelCommand().Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "Fargate-only") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
}
