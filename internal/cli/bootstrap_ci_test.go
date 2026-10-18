package cli

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/githubci"
	"github.com/magelift/magelift/internal/platform"
)

type fakeBootstrapGitHub struct {
	resolveCalls int
	ensureCalls  []string
	resolve      githubci.Repository
	resolveErr   error
	ensureErr    map[string]error
}

func (f *fakeBootstrapGitHub) ResolveRepository(context.Context, string, string) (githubci.Repository, error) {
	f.resolveCalls++
	if f.resolveErr != nil {
		return githubci.Repository{}, f.resolveErr
	}
	return f.resolve, nil
}

func (f *fakeBootstrapGitHub) EnsureVariable(_ context.Context, _, _, name, _ string) (githubci.EnsureVariableResult, error) {
	f.ensureCalls = append(f.ensureCalls, name)
	if err, ok := f.ensureErr[name]; ok {
		return githubci.EnsureVariableResult{}, err
	}
	return githubci.EnsureVariableResult{Name: name}, nil
}

func syntheticGCPBootstrapDetails() map[string]any {
	return map[string]any{
		bootstrapDetailCIPermissionsApplied: true,
		bootstrapDetailGitHubRepositoryID:   "1001",
		bootstrapDetailGitHubOwnerID:        "2002",
		bootstrapDetailCIVariables: map[string]any{
			"MAGELIFT_GCP_REGION":     "europe-west1",
			"MAGELIFT_VERSION":        "v1.0.0",
			"MAGELIFT_GCP_PROJECT_ID": "example-gcp",
		},
	}
}

func TestBootstrapCIFlagsRecordApplyCIPermissions(t *testing.T) {
	configPath := writeGCPLifecycleConfig(t, "staging")
	fake := &recordingBootstrap{
		result: platform.BootstrapResult{
			BackendURL: "gs://state",
			Details:    syntheticGCPBootstrapDetails(),
		},
	}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	if err := o.modules.RegisterModule(stubGCPModule{}); err != nil {
		t.Fatal(err)
	}
	o.testBootstrap = fake
	o.newBootstrapGitHub = func(context.Context) (bootstrapGitHubClient, error) {
		return &fakeBootstrapGitHub{resolve: githubci.Repository{ID: 1001, OwnerID: 2002, OwnerLogin: "acme", Name: "shop"}}, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{
		"--config", configPath,
		"--env", "staging",
		"--output", "json",
		"bootstrap",
		"--github-owner", "acme",
		"--github-repo", "shop",
		"--apply-ci-permissions",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !fake.req.ApplyCIPermissions {
		t.Fatalf("ApplyCIPermissions = false, req = %#v", fake.req)
	}
	if fake.req.GitHubOwner != "acme" || fake.req.GitHubRepo != "shop" {
		t.Fatalf("unexpected github identity: %#v", fake.req)
	}
}

func TestBootstrapCIRejectsConfigureWithoutApplyBeforeEnsure(t *testing.T) {
	runBootstrapCIRejectTest(t, writeGCPLifecycleConfig(t, "staging"), true, []string{
		"bootstrap", "--configure-github", "--github-owner", "acme", "--github-repo", "shop",
	}, "--configure-github requires --apply-ci-permissions")
}

func TestBootstrapCIRejectsConfigureWithoutOwnerBeforeEnsure(t *testing.T) {
	runBootstrapCIRejectTest(t, writeGCPLifecycleConfig(t, "staging"), true, []string{
		"bootstrap", "--apply-ci-permissions", "--configure-github", "--github-repo", "shop",
	}, "--github-owner")
}

func TestBootstrapCIRejectsApplyOnAWSBeforeEnsure(t *testing.T) {
	runBootstrapCIRejectTest(t, writeLifecycleConfig(t, "staging", false), false, []string{
		"bootstrap", "--apply-ci-permissions", "--github-owner", "acme", "--github-repo", "shop",
	}, "only supported for GCP")
}

func runBootstrapCIRejectTest(t *testing.T, configPath string, registerGCP bool, commandArgs []string, want string) {
	t.Helper()
	var out bytes.Buffer
	fake := &recordingBootstrap{}
	o := testOptions(&out, &fakeTerminal{interactive: false})
	if registerGCP {
		if err := o.modules.RegisterModule(stubGCPModule{}); err != nil {
			t.Fatal(err)
		}
	}
	o.testBootstrap = fake
	cmd := newCommandWithOptions(o)
	args := []string{"--config", configPath, "--env", "staging"}
	args = append(args, commandArgs...)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
	if fake.ensureCalls != 0 || fake.verifyCalls != 0 {
		t.Fatalf("bootstrap ran verify:%d ensure:%d", fake.verifyCalls, fake.ensureCalls)
	}
}

func TestBootstrapCIFailsWhenProviderDoesNotConfirmApply(t *testing.T) {
	configPath := writeGCPLifecycleConfig(t, "staging")
	fake := &recordingBootstrap{
		result: platform.BootstrapResult{
			BackendURL: "gs://state",
			Details: map[string]any{
				bootstrapDetailCIPermissionsApplied: false,
			},
		},
	}
	o := testOptions(nil, &fakeTerminal{interactive: false})
	if err := o.modules.RegisterModule(stubGCPModule{}); err != nil {
		t.Fatal(err)
	}
	o.testBootstrap = fake
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{
		"--config", configPath, "--env", "staging",
		"bootstrap", "--apply-ci-permissions", "--github-owner", "acme", "--github-repo", "shop",
	})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "did not apply CI permissions") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfigureBootstrapGitHubRejectsTransferBeforeWrites(t *testing.T) {
	client := &fakeBootstrapGitHub{
		resolve: githubci.Repository{ID: 9999, OwnerID: 2002},
	}
	_, err := configureBootstrapGitHub(context.Background(), client, "acme", "shop", syntheticGCPBootstrapDetails())
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.resolveCalls != 1 || len(client.ensureCalls) != 0 {
		t.Fatalf("resolve=%d ensure=%v", client.resolveCalls, client.ensureCalls)
	}
}

func TestConfigureBootstrapGitHubEnsuresSortedVariables(t *testing.T) {
	client := &fakeBootstrapGitHub{
		resolve: githubci.Repository{ID: 1001, OwnerID: 2002, OwnerLogin: "acme", Name: "shop"},
	}
	names, err := configureBootstrapGitHub(context.Background(), client, "acme", "shop", syntheticGCPBootstrapDetails())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"MAGELIFT_GCP_PROJECT_ID", "MAGELIFT_GCP_REGION", "MAGELIFT_VERSION"}
	if !sort.StringsAreSorted(client.ensureCalls) {
		t.Fatalf("ensure order not sorted: %v", client.ensureCalls)
	}
	if strings.Join(client.ensureCalls, ",") != strings.Join(want, ",") {
		t.Fatalf("ensure calls = %v, want %v", client.ensureCalls, want)
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("configured = %v, want %v", names, want)
	}
}

func TestConfigureBootstrapGitHubPropagatesConflictWithoutWrites(t *testing.T) {
	client := &fakeBootstrapGitHub{
		resolve: githubci.Repository{ID: 1001, OwnerID: 2002, OwnerLogin: "acme", Name: "shop"},
		ensureErr: map[string]error{
			"MAGELIFT_GCP_PROJECT_ID": &githubci.ErrVariableConflict{Name: "MAGELIFT_GCP_PROJECT_ID"},
		},
	}
	_, err := configureBootstrapGitHub(context.Background(), client, "acme", "shop", syntheticGCPBootstrapDetails())
	var conflict *githubci.ErrVariableConflict
	if !errors.As(err, &conflict) || conflict.Name != "MAGELIFT_GCP_PROJECT_ID" {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "example-gcp") {
		t.Fatalf("error leaked value: %v", err)
	}
	if len(client.ensureCalls) != 1 {
		t.Fatalf("ensure calls = %v", client.ensureCalls)
	}
}

func TestConfigureBootstrapGitHubSafeRerunAfterNoop(t *testing.T) {
	client := &fakeBootstrapGitHub{
		resolve: githubci.Repository{ID: 1001, OwnerID: 2002, OwnerLogin: "acme", Name: "shop"},
	}
	for i := 0; i < 2; i++ {
		names, err := configureBootstrapGitHub(context.Background(), client, "acme", "shop", syntheticGCPBootstrapDetails())
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 3 {
			t.Fatalf("names = %v", names)
		}
	}
	if client.resolveCalls != 2 {
		t.Fatalf("resolve calls = %d", client.resolveCalls)
	}
}

func TestConfigureBootstrapGitHubRejectsUnknownVariableBeforeWrites(t *testing.T) {
	details := syntheticGCPBootstrapDetails()
	vars := details[bootstrapDetailCIVariables].(map[string]any)
	vars["GITHUB_TOKEN"] = "nope"
	client := &fakeBootstrapGitHub{resolve: githubci.Repository{ID: 1001, OwnerID: 2002, OwnerLogin: "acme", Name: "shop"}}
	_, err := configureBootstrapGitHub(context.Background(), client, "acme", "shop", details)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.ensureCalls) != 0 {
		t.Fatalf("ensure calls = %v", client.ensureCalls)
	}
}

func TestConfigureBootstrapGitHubRejectsNonStringVariableBeforeWrites(t *testing.T) {
	details := syntheticGCPBootstrapDetails()
	vars := details[bootstrapDetailCIVariables].(map[string]any)
	vars["MAGELIFT_VERSION"] = 42
	client := &fakeBootstrapGitHub{resolve: githubci.Repository{ID: 1001, OwnerID: 2002, OwnerLogin: "acme", Name: "shop"}}
	_, err := configureBootstrapGitHub(context.Background(), client, "acme", "shop", details)
	if err == nil || !strings.Contains(err.Error(), "not a string") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.ensureCalls) != 0 {
		t.Fatalf("ensure calls = %v", client.ensureCalls)
	}
}

func TestBootstrapConfigureGitHubWritesVariablesAndNextCI(t *testing.T) {
	configPath := writeGCPLifecycleConfig(t, "staging")
	fake := &recordingBootstrap{
		result: platform.BootstrapResult{
			BackendURL: "gs://state",
			Details:    syntheticGCPBootstrapDetails(),
		},
	}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	if err := o.modules.RegisterModule(stubGCPModule{}); err != nil {
		t.Fatal(err)
	}
	o.configPath = configPath
	o.testBootstrap = fake
	gh := &fakeBootstrapGitHub{resolve: githubci.Repository{ID: 1001, OwnerID: 2002, OwnerLogin: "acme", Name: "shop"}}
	o.newBootstrapGitHub = func(context.Context) (bootstrapGitHubClient, error) { return gh, nil }
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{
		"--config", configPath,
		"--env", "staging",
		"--output", "json",
		"bootstrap",
		"--github-owner", "acme",
		"--github-repo", "shop",
		"--apply-ci-permissions",
		"--configure-github",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"next": "magelift --config `) || !strings.Contains(output.String(), "ci generate") {
		t.Fatalf("output missing ci generate next: %s", output.String())
	}
	if !strings.Contains(output.String(), `"githubRepositoryVariables"`) {
		t.Fatalf("output missing configured variable names: %s", output.String())
	}
	if !strings.Contains(output.String(), "MAGELIFT_GCP_REGION") {
		t.Fatalf("output missing variable names: %s", output.String())
	}
}

func TestBootstrapApplyCINextSuggestsGitHubReviewCommand(t *testing.T) {
	configPath := writeGCPLifecycleConfig(t, "staging")
	fake := &recordingBootstrap{
		result: platform.BootstrapResult{
			BackendURL: "gs://state",
			Details: map[string]any{
				bootstrapDetailCIPermissionsApplied: true,
			},
		},
	}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	if err := o.modules.RegisterModule(stubGCPModule{}); err != nil {
		t.Fatal(err)
	}
	o.configPath = configPath
	o.testBootstrap = fake
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{
		"--config", configPath,
		"--env", "staging",
		"--output", "json",
		"bootstrap",
		"--github-owner", "acme",
		"--github-repo", "shop",
		"--apply-ci-permissions",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	next := output.String()
	for _, fragment := range []string{
		"--apply-ci-permissions",
		"--configure-github",
		"--github-owner",
		"--github-repo",
		"shop",
	} {
		if !strings.Contains(next, fragment) {
			t.Fatalf("next missing %q: %s", fragment, next)
		}
	}
}

func TestBootstrapRejectsInvalidCISwitchesBeforeReadingConfiguration(t *testing.T) {
	var out bytes.Buffer
	command := newCommand(&out, &out, nil)
	command.SetArgs([]string{"--config", t.TempDir() + "/absent.yaml", "bootstrap", "--configure-github"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "requires --apply-ci-permissions") {
		t.Fatalf("configuration lookup preceded switch validation: %v", err)
	}
}
