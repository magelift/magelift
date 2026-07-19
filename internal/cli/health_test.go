package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	awsoperations "github.com/acourtiol/magelift/internal/cloud/aws/operations"
	awsstack "github.com/acourtiol/magelift/internal/cloud/aws/stack"
	"github.com/acourtiol/magelift/internal/health"
)

type fakeRuntimeStore struct {
	result awsoperations.ServiceHealth
}

func (f fakeRuntimeStore) Check(context.Context, string, string) (awsoperations.ServiceHealth, error) {
	return f.result, nil
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
	if !strings.Contains(out.String(), `"status": "unavailable"`) || !strings.Contains(out.String(), `"id": "runtime.ecs"`) {
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
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}, nil
	}
	o.newRuntime = func(context.Context, string) (runtimeStore, error) {
		return fakeRuntimeStore{result: awsoperations.ServiceHealth{DesiredCount: 2, RunningCount: 2, PrimaryRollout: "COMPLETED", Tasks: []awsoperations.TaskHealth{{ARN: "task-a", LastStatus: "RUNNING", HealthStatus: "HEALTHY"}}}}, nil
	}
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
	checks := runtimeHealthChecks(awsoperations.ServiceHealth{DesiredCount: 2, RunningCount: 1, PrimaryRollout: "IN_PROGRESS", Tasks: []awsoperations.TaskHealth{{ARN: "task-a", LastStatus: "STOPPED"}}})
	if health.Summarize(checks) != health.StatusUnhealthy {
		t.Fatalf("unhealthy runtime evidence was accepted: %#v", checks)
	}
}
