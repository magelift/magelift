package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

type recordingBootstrap struct {
	req         platform.BootstrapRequest
	result      platform.BootstrapResult
	err         error
	verifyCalls int
	ensureCalls int
}

func (r *recordingBootstrap) VerifyAccount(context.Context, platform.PlannedStack) error {
	r.verifyCalls++
	return nil
}

func (r *recordingBootstrap) Ensure(_ context.Context, _ platform.PlannedStack, req platform.BootstrapRequest) (platform.BootstrapResult, error) {
	r.ensureCalls++
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

type rejectingBootstrapPlanAdmission struct {
	err   error
	calls int
}

func (a *rejectingBootstrapPlanAdmission) Admit(context.Context, platform.PlannedStack) (platform.PlannedStack, error) {
	a.calls++
	return nil, a.err
}

type bootstrapAdmissionModule struct {
	stubAWSModule
	admission platform.PlanAdmission
}

func (m bootstrapAdmissionModule) PlanAdmission() platform.PlanAdmission {
	return m.admission
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
	if !strings.Contains(output.String(), `"next": "magelift deploy --env staging --yes"`) {
		t.Fatalf("bootstrap did not print next Magelift command: %s", output.String())
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
	if !strings.Contains(err.Error(), "existing log bucket") || !strings.Contains(err.Error(), "Next:") {
		t.Fatalf("bootstrap error was not Magelift-guided: %v", err)
	}
}

func TestBootstrapCommandBlocksBeforeBootstrapMutation(t *testing.T) {
	configPath := writeLifecycleConfig(t, "staging", false)
	fake := &recordingBootstrap{}
	admission := &rejectingBootstrapPlanAdmission{err: errors.New("quota admission failed")}
	modules := platform.NewModuleRegistry()
	if err := modules.RegisterModule(bootstrapAdmissionModule{admission: admission}); err != nil {
		t.Fatal(err)
	}
	o := testOptions(nil, &fakeTerminal{interactive: false})
	o.modules = modules
	o.testBootstrap = fake
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", configPath, "--env", "staging", "bootstrap"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "provider plan admission") || !strings.Contains(err.Error(), "quota admission failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if admission.calls != 1 {
		t.Fatalf("plan admission calls = %d, want 1", admission.calls)
	}
	if fake.verifyCalls != 0 || fake.ensureCalls != 0 {
		t.Fatalf("bootstrap mutation calls = verify:%d ensure:%d, want zero", fake.verifyCalls, fake.ensureCalls)
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
