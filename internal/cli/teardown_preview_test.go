package cli

import (
	"bytes"
	"context"
	"reflect"
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

func TestDestroyPreviewRequestKeepsDestroyFlag(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.stderr = &bytes.Buffer{}
	o.configPath = path
	o.environment = "staging"
	o.yes = true
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newLock = func(context.Context, platform.PlannedStack) (func(context.Context) error, error) {
		t.Fatal("destroy preview regression must not acquire the provider lock")
		return nil, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--yes", "destroy", "--skip-lock"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"preview", "destroy"}) {
		t.Fatalf("backend calls = %v", backend.calls)
	}
	if len(backend.requests) != 2 || !backend.requests[0].Destroy || !backend.requests[1].Destroy {
		t.Fatalf("requests = %#v, want Destroy on the preview that precedes destroy", backend.requests)
	}
}
