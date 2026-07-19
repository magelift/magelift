package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	awsbootstrap "github.com/acourtiol/magelift/internal/cloud/aws/bootstrap"
)

type fakeBootstrap struct {
	plan   awsbootstrap.Plan
	region string
	result awsbootstrap.Result
	err    error
}

type fakeIdentity struct {
	plan awsbootstrap.IdentityPlan
	err  error
}

func (f *fakeIdentity) Ensure(_ context.Context, plan awsbootstrap.IdentityPlan) error {
	f.plan = plan
	return f.err
}

func (f *fakeBootstrap) Ensure(_ context.Context, plan awsbootstrap.Plan, region string) (awsbootstrap.Result, error) {
	f.plan = plan
	f.region = region
	if f.result.Plan.StateBucket == "" {
		f.result.Plan = plan
	}
	return f.result, f.err
}

func TestBootstrapCommandReconcilesSelectedEnvironment(t *testing.T) {
	configPath := bootstrapConfig(t)
	fake := &fakeBootstrap{result: awsbootstrap.Result{KeyARN: "arn:aws:kms:eu-west-3:123456789012:key/test"}}
	identity := &fakeIdentity{}
	var output strings.Builder
	o := testOptions(nil, &fakeTerminal{interactive: false})
	o.stdout = &output
	o.stderr = &output
	o.newBootstrap = func(context.Context, string) (awsbootstrap.Ensurer, error) { return fake, nil }
	o.newIdentity = func(context.Context, string) (awsbootstrap.IdentityEnsurer, error) { return identity, nil }
	o.verifyAccount = func(context.Context, string, string) error { return nil }
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
	if fake.region != "eu-west-3" || fake.plan.StateBucket != "magelift-123456789012-eu-west-3-example-shop-staging-state" {
		t.Fatalf("unexpected bootstrap request: %#v region=%q", fake.plan, fake.region)
	}
	if identity.plan.CIRoleARN == "" || identity.plan.StateRoleARN == "" || identity.plan.BuildRoleARN == "" || identity.plan.CIRoleARN == identity.plan.StateRoleARN || identity.plan.BuildRoleARN == identity.plan.CIRoleARN || !strings.Contains(output.String(), "ciRoleArn") || !strings.Contains(output.String(), "buildRoleArn") {
		t.Fatalf("unexpected output: %s", output.String())
	}
}

func TestBootstrapCommandRequiresAccessLogBucket(t *testing.T) {
	o := testOptions(nil, &fakeTerminal{interactive: false})
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"bootstrap"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "access-log-bucket") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBootstrapCommandRedactsBackendFailure(t *testing.T) {
	configPath := bootstrapConfig(t)
	secret := "secret-provider-detail"
	o := testOptions(nil, &fakeTerminal{interactive: false})
	o.newBootstrap = func(context.Context, string) (awsbootstrap.Ensurer, error) {
		return &fakeBootstrap{err: errors.New(secret)}, nil
	}
	o.newIdentity = func(context.Context, string) (awsbootstrap.IdentityEnsurer, error) { return &fakeIdentity{}, nil }
	o.verifyAccount = func(context.Context, string, string) error { return nil }
	cmd := newCommandWithOptions(o)
	o.verifyAccount = func(context.Context, string, string) error { return nil }
	cmd.SetArgs([]string{"--config", configPath, "--env", "staging", "bootstrap", "--access-log-bucket", "logs", "--github-owner", "acourtiol", "--github-repo", "magelift"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), secret) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func bootstrapConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	if err := os.WriteFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
