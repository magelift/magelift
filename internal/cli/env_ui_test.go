package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/dumpimport"
	"github.com/magelift/magelift/internal/platform"
)

func TestEnvUISessionOnlyReturnsTunnelShape(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	var output bytes.Buffer
	recording := &recordingTunnel{target: platform.ExecTarget{
		Launcher: "kubectl",
		Args:     []string{"--kubeconfig", "/tmp/magelift-kubeconfig-secret.yaml", "port-forward", "service/shop-db-ui", "8080:80"},
	}}
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"serviceName": "shop-web"}}, nil
	}
	o.testRuntimeTunnel = recording
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--output", "json", "env", "ui", "staging", "--target", "db-ui"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v\n%s", err, output.String())
	}
	if got["target"] != platform.TunnelTargetDatabaseUI || got["sessionOnly"] != true {
		t.Fatalf("result = %#v", got)
	}
	if recording.query.Target != platform.TunnelTargetDatabaseUI {
		t.Fatalf("query = %#v", recording.query)
	}
}

func TestEnvUIUnsupportedDoesNotBlockDump(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.output = path, "json"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"serviceName": "shop-web"}}, nil
	}
	o.testRuntimeTunnel = &recordingTunnel{err: errors.Join(platform.ErrNotSupported, errors.New("database console is not provisioned"))}
	dumped := false
	o.exportDump = func(_ context.Context, opts dumpimport.Options) error {
		dumped = true
		return os.WriteFile(opts.OutputPath, []byte("-- dump\n"), 0o600)
	}
	ui := newCommandWithOptions(o)
	ui.SetArgs([]string{"--config", path, "env", "ui", "staging", "--target", "db-ui"})
	err := ui.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "not provisioned") {
		t.Fatalf("ui error=%v code=%d", err, ExitCode(err))
	}

	dest := filepath.Join(t.TempDir(), "staging.sql")
	dump := newCommandWithOptions(o)
	dump.SetArgs([]string{"--config", path, "--output", "json", "env", "dump", "staging", "--to", dest})
	if err := dump.Execute(); err != nil {
		t.Fatal(err)
	}
	if !dumped {
		t.Fatal("dump was not invoked after unsupported UI")
	}
}
