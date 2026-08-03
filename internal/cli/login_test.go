package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

type recordingLoginBootstrap struct {
	verified bool
	err      error
}

func (r *recordingLoginBootstrap) VerifyAccount(_ context.Context, planned platform.PlannedStack) error {
	r.verified = planned.Region() == "eu-west-3"
	return r.err
}

func (r *recordingLoginBootstrap) Ensure(context.Context, platform.PlannedStack, platform.BootstrapRequest) (platform.BootstrapResult, error) {
	return platform.BootstrapResult{}, platform.ErrNotSupported
}

func TestLoginVerifiesSelectedAWSAccount(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	var output bytes.Buffer
	fake := &recordingLoginBootstrap{}
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.output = path, "staging", "json"
	o.testBootstrap = fake
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "login"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !fake.verified || !strings.Contains(output.String(), `"authenticated": true`) {
		t.Fatalf("verified=%v output=%s", fake.verified, output.String())
	}
}

func TestLoginSurfacesCredentialFailureWithoutMutation(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	o.testBootstrap = &recordingLoginBootstrap{err: context.Canceled}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "login"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "verify credentials") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
}
