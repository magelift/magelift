package automation

import (
	"context"
	"io"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type capturePulumiCommand struct{}

func (capturePulumiCommand) Version() semver.Version {
	return semver.Version{Major: 3, Minor: 262, Patch: 0}
}

func (capturePulumiCommand) Run(
	context.Context,
	string,
	io.Reader,
	[]io.Writer,
	[]io.Writer,
	[]string,
	...string,
) (string, string, int, error) {
	return "", "", 0, nil
}

func TestMergeInlineStackEnvBackendURLAuthoritative(t *testing.T) {
	t.Parallel()
	caller := map[string]string{
		"PULUMI_BACKEND_URL":       "gs://caller",
		"PULUMI_CONFIG_PASSPHRASE": "local",
	}
	merged := mergeInlineStackEnv("gs://authoritative", caller)
	if merged["PULUMI_BACKEND_URL"] != "gs://authoritative" {
		t.Fatalf("backend URL=%q", merged["PULUMI_BACKEND_URL"])
	}
	if merged["PULUMI_CONFIG_PASSPHRASE"] != "local" {
		t.Fatalf("passphrase=%q", merged["PULUMI_CONFIG_PASSPHRASE"])
	}
	if caller["PULUMI_BACKEND_URL"] != "gs://caller" {
		t.Fatalf("caller map mutated: %#v", caller)
	}
}

func TestInlineStackWorkspaceOptionsSingleEnvVars(t *testing.T) {
	t.Parallel()
	opts := inlineStackWorkspaceOptions("gs://state", map[string]string{"CUSTOM": "1"})
	if len(opts) != 1 {
		t.Fatalf("options len=%d, want one EnvVars option", len(opts))
	}
	ctx := context.Background()
	ws, err := auto.NewLocalWorkspace(ctx, append(opts, auto.Pulumi(capturePulumiCommand{}))...)
	if err != nil {
		t.Fatal(err)
	}
	got := ws.GetEnvVars()
	if got["PULUMI_BACKEND_URL"] != "gs://state" || got["CUSTOM"] != "1" {
		t.Fatalf("env=%#v", got)
	}
}

func TestNewInlineStackWithEnvironmentUsesMergedWorkspaceEnv(t *testing.T) {
	t.Parallel()
	var captured []auto.LocalWorkspaceOption
	upsert := func(
		ctx context.Context,
		stackName string,
		projectName string,
		program pulumi.RunFunc,
		opts ...auto.LocalWorkspaceOption,
	) (auto.Stack, error) {
		captured = append([]auto.LocalWorkspaceOption(nil), opts...)
		return auto.Stack{}, nil
	}

	_, err := newInlineStackWithEnvironment(
		context.Background(),
		"shop-preview-gcp",
		func(*pulumi.Context) error { return nil },
		"gs://bucket",
		map[string]string{"PULUMI_CONFIG_PASSPHRASE": "managed"},
		upsert,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(captured) != 1 {
		t.Fatalf("captured options=%d", len(captured))
	}
	ws, err := auto.NewLocalWorkspace(context.Background(), append(captured, auto.Pulumi(capturePulumiCommand{}))...)
	if err != nil {
		t.Fatal(err)
	}
	env := ws.GetEnvVars()
	if env["PULUMI_BACKEND_URL"] != "gs://bucket" || env["PULUMI_CONFIG_PASSPHRASE"] != "managed" {
		t.Fatalf("env=%#v", env)
	}
}
