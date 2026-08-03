package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/platform"
)

type recordingBootstrap struct {
	req    platform.BootstrapRequest
	result platform.BootstrapResult
	err    error
}

func (r *recordingBootstrap) VerifyAccount(context.Context, platform.PlannedStack) error {
	return nil
}

func (r *recordingBootstrap) Ensure(_ context.Context, _ platform.PlannedStack, req platform.BootstrapRequest) (platform.BootstrapResult, error) {
	r.req = req
	if r.err != nil {
		return platform.BootstrapResult{}, r.err
	}
	if r.result.BackendURL == "" {
		r.result = platform.BootstrapResult{
			BackendURL: "s3://magelift-123456789012-eu-west-3-example-shop-staging-state",
			KeyRef:     "arn:aws:kms:eu-west-3:123456789012:key/test",
			Details: map[string]any{
				"identity": map[string]string{
					"ciRoleArn":    "arn:aws:iam::123456789012:role/ci",
					"stateRoleArn": "arn:aws:iam::123456789012:role/state",
					"buildRoleArn": "arn:aws:iam::123456789012:role/build",
				},
			},
		}
	}
	return r.result, nil
}

func TestBootstrapCommandReconcilesSelectedEnvironment(t *testing.T) {
	configPath := writeLifecycleConfig(t, "staging", false)
	fake := &recordingBootstrap{}
	var output strings.Builder
	o := testOptions(nil, &fakeTerminal{interactive: false})
	o.stdout = &output
	o.stderr = &output
	o.testBootstrap = fake
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{
		"--config", configPath,
		"--env", "staging",
		"--output", "json",
		"bootstrap",
		"--access-log-bucket", "existing-log-bucket",
		"--github-owner", "acourtiol",
		"--github-repo", "magelift",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if fake.req.AccessLogBucket != "existing-log-bucket" || fake.req.GitHubOwner != "acourtiol" || fake.req.GitHubRepo != "magelift" {
		t.Fatalf("unexpected bootstrap request: %#v", fake.req)
	}
	if !strings.Contains(output.String(), "backendURL") || !strings.Contains(output.String(), "ciRoleArn") {
		t.Fatalf("unexpected output: %s", output.String())
	}
}

func TestBootstrapCommandRequiresAccessLogBucket(t *testing.T) {
	configPath := writeLifecycleConfig(t, "staging", false)
	fake := &recordingBootstrap{err: errors.New("--access-log-bucket is required")}
	o := testOptions(nil, &fakeTerminal{interactive: false})
	o.testBootstrap = fake
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "--env", "staging", "bootstrap"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "access-log-bucket") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapCommandRedactsBackendFailure(t *testing.T) {
	configPath := writeLifecycleConfig(t, "staging", false)
	secret := "secret-provider-detail"
	o := testOptions(nil, &fakeTerminal{interactive: false})
	o.testBootstrap = &recordingBootstrap{err: errors.New(secret)}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "--env", "staging", "bootstrap", "--access-log-bucket", "logs", "--github-owner", "acourtiol", "--github-repo", "magelift"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), secret) {
		t.Fatalf("unexpected error: %v", err)
	}
}
