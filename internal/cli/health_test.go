package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/health"
	"github.com/acourtiol/magelift/internal/platform"
)

type fakeRuntimeObserve struct {
	items []platform.RuntimeHealth
	err   error
}

func (f fakeRuntimeObserve) TailLogs(context.Context, platform.PlannedStack, platform.LogQuery) ([]platform.LogEvent, error) {
	return nil, platform.ErrNotSupported
}
func (f fakeRuntimeObserve) CheckRuntime(context.Context, platform.PlannedStack, map[string]any) ([]platform.RuntimeHealth, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}
func (f fakeRuntimeObserve) PrepareExec(context.Context, platform.PlannedStack, map[string]any, platform.ExecQuery) (platform.ExecTarget, error) {
	return platform.ExecTarget{}, platform.ErrNotSupported
}

func TestHealthConfigModeProducesJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "health"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status": "healthy"`) || !strings.Contains(out.String(), `"id": "config.resolved"`) {
		t.Fatalf("unexpected health report: %s", out.String())
	}
}

func TestHealthRuntimeModeDoesNotFabricateSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "health", "--mode", "runtime"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != healthExitUnavailable {
		t.Fatalf("runtime health error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.Contains(out.String(), `"status": "unavailable"`) || !strings.Contains(out.String(), `"id": "runtime.observe"`) {
		t.Fatalf("unexpected health report: %s", out.String())
	}
}

func TestOutputHealthChecksRejectMissingEvidence(t *testing.T) {
	checks := outputHealthChecks(map[string]any{"applicationURL": "https://shop.example"})
	if health.Summarize(checks) != health.StatusUnhealthy || checks[1].ID != "output.mediaURL" {
		t.Fatalf("missing output was not unhealthy: %#v", checks)
	}
}

func TestOutputHealthChecksAcceptHTTPSURLs(t *testing.T) {
	checks := outputHealthChecks(map[string]any{"applicationURL": "https://shop.example", "mediaURL": "https://media.example"})
	if health.Summarize(checks) != health.StatusHealthy {
		t.Fatalf("valid outputs were not healthy: %#v", checks)
	}
}

func TestHealthRuntimeModeUsesECSServiceAndTaskEvidence(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.output = path, "staging", "json"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}, nil
	}
	o.testRuntimeObserve = fakeRuntimeObserve{items: []platform.RuntimeHealth{
		{ID: "runtime.ecs.service", Service: "shop-web-service", Status: "healthy", Detail: "ECS service has 2 running of 2 desired tasks"},
		{ID: "runtime.ecs.rollout", Service: "shop-web-service", Status: "healthy", Detail: "the primary ECS deployment rollout completed"},
		{ID: "runtime.ecs.task.task-a", Service: "shop-web-service", Status: "healthy", Detail: "ECS task is running and healthy"},
	}}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "health", "--mode", "runtime"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status": "healthy"`) || !strings.Contains(out.String(), `"id": "runtime.ecs.service"`) {
		t.Fatalf("unexpected runtime health: %s", out.String())
	}
}

func TestRuntimeHealthChecksMarkUnhealthyRollouts(t *testing.T) {
	checks := runtimeHealthChecks([]platform.RuntimeHealth{
		{ID: "runtime.ecs.service", Status: "unhealthy", Detail: "ECS service has 1 running of 2 desired tasks"},
		{ID: "runtime.ecs.rollout", Status: "unhealthy", Detail: "the primary ECS deployment rollout has not completed"},
		{ID: "runtime.ecs.task.task-a", Status: "unhealthy", Detail: "ECS task status is STOPPED/"},
	})
	if health.Summarize(checks) != health.StatusUnhealthy {
		t.Fatalf("unhealthy runtime evidence was accepted: %#v", checks)
	}
}

func TestRuntimeHealthChecksAcceptUnknownWhenRunning(t *testing.T) {
	checks := runtimeHealthChecks([]platform.RuntimeHealth{
		{ID: "runtime.ecs.service", Status: "healthy"},
		{ID: "runtime.ecs.rollout", Status: "healthy"},
		{ID: "runtime.ecs.task.task-a", Status: "healthy", Detail: "ECS task is running (no container health check configured)"},
	})
	if health.Summarize(checks) != health.StatusHealthy {
		t.Fatalf("RUNNING/UNKNOWN must be healthy when no container health check is set: %#v", checks)
	}
}

func TestRuntimeHealthChecksRejectUnhealthyTask(t *testing.T) {
	checks := runtimeHealthChecks([]platform.RuntimeHealth{
		{ID: "runtime.ecs.service", Status: "healthy"},
		{ID: "runtime.ecs.rollout", Status: "healthy"},
		{ID: "runtime.ecs.task.task-a", Status: "unhealthy", Detail: "ECS task status is RUNNING/UNHEALTHY"},
	})
	if health.Summarize(checks) != health.StatusUnhealthy {
		t.Fatalf("RUNNING/UNHEALTHY must fail: %#v", checks)
	}
}
