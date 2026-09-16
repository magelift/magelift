package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
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

func TestMapPluginErrorClassifiesByCode(t *testing.T) {
	cases := []struct {
		code     sdk.OperationErrorCode
		exitCode int
		guidance string
	}{
		{sdk.ErrCodeInvalid, 2, "fix the flagged input"},
		{sdk.ErrCodeCredential, 3, "re-authenticate"},
		{sdk.ErrCodeCompatibility, 3, "reinstall the provider"},
		{sdk.ErrCodeConflict, 3, ""},
		{sdk.ErrCodeNotFound, 3, ""},
	}
	for _, tc := range cases {
		err := mapPluginError(&providerhost.PluginError{Operation: sdk.OpApply, Code: tc.code, Message: "boom"})
		var exit *exitError
		if !errors.As(err, &exit) || exit.code != tc.exitCode {
			t.Errorf("%s: err = %#v, want exit %d", tc.code, err, tc.exitCode)
			continue
		}
		if tc.guidance != "" && !strings.Contains(err.Error(), tc.guidance) {
			t.Errorf("%s: err = %q, want guidance %q", tc.code, err.Error(), tc.guidance)
		}
	}
	upstream := mapPluginError(&providerhost.PluginError{Operation: sdk.OpApply, Code: sdk.ErrCodeUpstream, Message: "boom"})
	if _, ok := upstream.(*providerhost.PluginError); !ok {
		t.Errorf("upstream must pass through unmapped, got %#v", upstream)
	}
	coded := mapPluginError(invalid(errors.New("already coded")))
	var exit *exitError
	if !errors.As(coded, &exit) || exit.code != 2 {
		t.Errorf("coded error must pass through, got %#v", coded)
	}
	if err := mapPluginError(errors.New("plain")); err.Error() != "plain" {
		t.Errorf("plain error must pass through, got %#v", err)
	}
}
