package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

func TestPreviewCollisionExitsDedicatedCode(t *testing.T) {
	path := writeLifecycleConfigWithExistingNetwork(t)
	backend := &fakeInfrastructureBackend{previewErr: errors.New("update failed: the stack is currently locked by ci")}
	var stdout, stderr bytes.Buffer
	o := testOptions(&stdout, &fakeTerminal{interactive: false})
	o.stderr = &stderr
	o.configPath = path
	o.environment = "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "preview"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected a collision error")
	}
	var exit *exitError
	if !errors.As(err, &exit) || exit.code != concurrentUpdateExit {
		t.Fatalf("err = %#v, want exit code %d", err, concurrentUpdateExit)
	}
	if !strings.Contains(err.Error(), "wait for it to finish, then retry") {
		t.Fatalf("err = %v, want the retry sentence", err)
	}
	if got := ExitCode(err); got != concurrentUpdateExit {
		t.Fatalf("ExitCode = %d, want %d", got, concurrentUpdateExit)
	}
}

func TestPreviewGenericFailureKeepsGenericCode(t *testing.T) {
	path := writeLifecycleConfigWithExistingNetwork(t)
	backend := &fakeInfrastructureBackend{previewErr: errors.New("engine exploded on resource x")}
	var stdout, stderr bytes.Buffer
	o := testOptions(&stdout, &fakeTerminal{interactive: false})
	o.stderr = &stderr
	o.configPath = path
	o.environment = "staging"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "preview"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected a failure")
	}
	var exit *exitError
	if errors.As(err, &exit) && exit.code == concurrentUpdateExit {
		t.Fatalf("generic failure mapped to the collision code: %v", err)
	}
	if !strings.Contains(err.Error(), "engine exploded") {
		t.Fatalf("err = %v, want the backend cause visible", err)
	}
}
