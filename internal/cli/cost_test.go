package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/usererr"
)

// Stable leading portion of the not-supported message. Plan 01-08 appends the
// certification tier after this prefix; keep assertions on the prefix only.
const costNotSupportedPrefix = "cost estimation is not supported for target aws/ecs-fargate yet"

type recordingCostEstimator struct {
	lastLive *bool
	calls    int
	report   platform.CostReport
	err      error
}

func (e *recordingCostEstimator) Estimate(_ context.Context, _ platform.PlannedStack, _ config.Config, opts platform.CostOptions) (platform.CostReport, error) {
	e.calls++
	live := opts.Live
	e.lastLive = &live
	return e.report, e.err
}

func sampleCostReport() platform.CostReport {
	return platform.CostReport{
		Environment: "staging",
		Provider:    "aws",
		Region:      "eu-west-3",
		Preset:      "preview",
		Mode:        "account-free",
		Currency:    "USD",
		Notice:      "capacity only",
	}
}

func writeCostConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCostRejectsPositionalArgs(t *testing.T) {
	path := writeCostConfig(t, starterConfig)
	var out bytes.Buffer
	o := testOptions(&out, nil)
	o.testCostEstimator = &recordingCostEstimator{report: sampleCostReport()}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "cost", "extra"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected positional argument rejection")
	}
	if ExitCode(err) != 1 && !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts no") {
		// cobra.NoArgs typically surfaces as "accepts no args" with exit 1 from cobra
		// when SilenceErrors is set; accept either message shape.
		if !strings.Contains(strings.ToLower(err.Error()), "arg") {
			t.Fatalf("unexpected error: %v (code %d)", err, ExitCode(err))
		}
	}
}

func TestCostLiveFlag(t *testing.T) {
	path := writeCostConfig(t, starterConfig)
	tests := []struct {
		name string
		args []string
		live bool
	}{
		{name: "default account-free", args: []string{"cost"}, live: false},
		{name: "live", args: []string{"cost", "--live"}, live: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			estimator := &recordingCostEstimator{report: sampleCostReport()}
			o := testOptions(&out, nil)
			o.testCostEstimator = estimator
			cmd := newCommandWithOptions(o)
			args := append([]string{"--config", path, "--env", "staging", "-o", "json"}, tt.args...)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if estimator.calls != 1 || estimator.lastLive == nil || *estimator.lastLive != tt.live {
				t.Fatalf("calls=%d live=%v want live=%v", estimator.calls, estimator.lastLive, tt.live)
			}
		})
	}
}

func TestCostOutputFormats(t *testing.T) {
	path := writeCostConfig(t, starterConfig)
	tests := []struct {
		name       string
		format     string
		wantSubstr string
		wantCode   int
	}{
		{name: "json", format: "json", wantSubstr: `"environment": "staging"`},
		{name: "yaml", format: "yaml", wantSubstr: "environment: staging"},
		{name: "table", format: "table", wantSubstr: "environment: staging"},
		{name: "unsupported", format: "xml", wantCode: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			o := testOptions(&out, nil)
			o.testCostEstimator = &recordingCostEstimator{report: sampleCostReport()}
			cmd := newCommandWithOptions(o)
			cmd.SetArgs([]string{"--config", path, "--env", "staging", "-o", tt.format, "cost"})
			err := cmd.Execute()
			if tt.wantCode != 0 {
				if err == nil || ExitCode(err) != tt.wantCode {
					t.Fatalf("error/code = %v/%d", err, ExitCode(err))
				}
				if !strings.Contains(err.Error(), "unsupported output format") {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tt.wantSubstr) {
				t.Fatalf("output missing %q: %s", tt.wantSubstr, out.String())
			}
		})
	}
}

func TestCostErrNotSupportedExits2(t *testing.T) {
	path := writeCostConfig(t, starterConfig)
	var out bytes.Buffer
	o := testOptions(&out, nil)
	o.testCostEstimator = &recordingCostEstimator{err: platform.ErrNotSupported}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "cost"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.HasPrefix(err.Error(), costNotSupportedPrefix) && !strings.Contains(err.Error(), costNotSupportedPrefix) {
		t.Fatalf("message must identify target; got: %v", err)
	}
	if !strings.Contains(err.Error(), "aws") || !strings.Contains(err.Error(), "ecs-fargate") {
		t.Fatalf("message must name provider/runtime: %v", err)
	}
}

func TestCostOtherEstimatorErrorExits3(t *testing.T) {
	path := writeCostConfig(t, starterConfig)
	var out bytes.Buffer
	o := testOptions(&out, nil)
	o.testCostEstimator = &recordingCostEstimator{err: errors.New("pricing catalog unavailable")}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "cost"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 3 {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
	ue, ok := usererr.As(err)
	if !ok {
		t.Fatalf("expected usererr chain, got %T: %v", err, err)
	}
	if ue.Cause != "cost estimation failed" {
		t.Fatalf("cause = %q", ue.Cause)
	}
	if !strings.Contains(ue.Next, "catalog") && !strings.Contains(ue.Next, "credentials") {
		t.Fatalf("next step = %q", ue.Next)
	}
}

func TestCostModuleWithoutEstimatorExits2(t *testing.T) {
	path := writeCostConfig(t, starterConfig)
	var out bytes.Buffer
	o := testOptions(&out, nil)
	// testCostEstimator left nil; certified stubAWSModule implements no CostEstimator.
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "cost"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.Contains(err.Error(), costNotSupportedPrefix) {
		t.Fatalf("expected not-supported shape naming aws/ecs-fargate: %v", err)
	}
}

func TestCostUnregisteredTargetFailsBeforeEstimator(t *testing.T) {
	// gcp/gke-autopilot validates in config but is not registered by registerTestModules
	// (aws/eks-autopilot is registered for tier-keyed experimental warning tests).
	contents := `schemaVersion: 1
project:
  name: example-shop
application:
  edition: open-source
  version: 2.4.9
  mode: integrated
build:
  php: "8.5"
target:
  provider: gcp
  runtime: gke-autopilot
  gcp:
    project: example-shop
defaults:
  region: europe-west1
  preset: preview
environments:
  staging:
    account: "example-shop"
extensions: {}
`
	path := writeCostConfig(t, contents)
	var out bytes.Buffer
	estimator := &recordingCostEstimator{report: sampleCostReport()}
	o := testOptions(&out, nil)
	o.testCostEstimator = estimator
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "cost"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
	if !strings.Contains(err.Error(), `no stack module registered for target "gcp"/"gke-autopilot"`) {
		t.Fatalf("expected planning gate error, got: %v", err)
	}
	if estimator.calls != 0 {
		t.Fatalf("estimator consulted %d times; planning gate must run first", estimator.calls)
	}
}
