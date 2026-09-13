package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

type recordingTunnel struct {
	query  platform.TunnelQuery
	target platform.ExecTarget
	err    error
}

func (r *recordingTunnel) PrepareTunnel(_ context.Context, _ platform.PlannedStack, _ map[string]any, query platform.TunnelQuery) (platform.ExecTarget, error) {
	r.query = query
	return r.target, r.err
}

func TestTunnelSessionOnlyReturnsRedactedLauncherShape(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	var output bytes.Buffer
	recording := &recordingTunnel{target: platform.ExecTarget{
		Launcher: "kubectl",
		Args:     []string{"--kubeconfig", "/tmp/magelift-kubeconfig-secret.yaml", "port-forward", "service/shop-queue", "18080:15672"},
	}}
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.output = path, "staging", "json"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"serviceName": "shop-web"}}, nil
	}
	o.testRuntimeTunnel = recording
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "tunnel", "--target", "queue-ui", "--local-port", "18080", "--session-only"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var got tunnelCommandResult
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v; output=%q", err, output.String())
	}
	if got.Target != platform.TunnelTargetQueueUI || got.LocalPort != 18080 || got.RemotePort != 15672 || got.Launcher != "kubectl" {
		t.Fatalf("result = %#v", got)
	}
	if !reflect.DeepEqual(got.Args, []string{"--kubeconfig", "<temporary-kubeconfig>", "port-forward", "service/shop-queue", "18080:15672"}) {
		t.Fatalf("preview args = %#v", got.Args)
	}
	if recording.query.Target != platform.TunnelTargetQueueUI || recording.query.LocalPort != 18080 || recording.query.RemotePort != 15672 {
		t.Fatalf("query = %#v", recording.query)
	}
}

func TestTunnelRunsLauncherAndCleansTemporaryFiles(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	cleanup, err := os.CreateTemp("", "magelift-tunnel-test-")
	if err != nil {
		t.Fatal(err)
	}
	cleanupPath := cleanup.Name()
	if err := cleanup.Close(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(cleanupPath) }()
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"serviceName": "shop-web"}}, nil
	}
	o.testRuntimeTunnel = &recordingTunnel{target: platform.ExecTarget{
		Launcher: "kubectl", Args: []string{"port-forward", "service/shop-web", "8080:80"}, CleanupPaths: []string{cleanupPath},
	}}
	var gotBinary string
	o.runCommand = func(_ context.Context, binary string, _ []string, _, _ io.Writer) error {
		gotBinary = binary
		if _, err := os.Stat(cleanupPath); err != nil {
			t.Fatalf("cleanup file was removed before launcher ran: %v", err)
		}
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "tunnel", "--target", "app", "--local-port", "8080"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if gotBinary != "kubectl" {
		t.Fatalf("binary = %q, want kubectl", gotBinary)
	}
	if _, err := os.Stat(cleanupPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleanup file still exists: %v", err)
	}
}

func TestTunnelUnsupportedAdapterDoesNotLaunch(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	launched := false
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"serviceName": "shop-web"}}, nil
	}
	o.testRuntimeTunnel = &recordingTunnel{err: errors.Join(platform.ErrNotSupported, errors.New("search dashboard is not provisioned"))}
	o.runCommand = func(context.Context, string, []string, io.Writer, io.Writer) error {
		launched = true
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "tunnel", "--target", "search-ui"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "not provisioned") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
	if launched {
		t.Fatal("unsupported tunnel launched a process")
	}
}
