package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestLoginVerifiesSelectedAWSAccount(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	var output bytes.Buffer
	verified := false
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.output = path, "staging", "json"
	o.verifyAccount = func(_ context.Context, region, account string) error {
		verified = region == "eu-west-3" && account == "123456789012"
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "login"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !verified || !strings.Contains(output.String(), `"authenticated": true`) {
		t.Fatalf("verified=%v output=%s", verified, output.String())
	}
}

func TestLoginSurfacesCredentialFailureWithoutMutation(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"
	o.verifyAccount = func(context.Context, string, string) error { return context.Canceled }
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "login"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "verify AWS credentials") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
}
