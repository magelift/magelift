package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/platform"
)

func TestNotSupportedNamesSurfaceTargetAndTier(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		runtime  string
		tier     platform.CertificationTier
		surface  string
	}{
		{
			name:     "certified",
			provider: "aws",
			runtime:  "ecs-fargate",
			tier:     platform.TierCertified,
			surface:  "cost estimation",
		},
		{
			name:     "experimental",
			provider: "ovh",
			runtime:  "mks",
			tier:     platform.TierExperimental,
			surface:  "cost estimation",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			planned := stubPlanned{
				provider: tt.provider,
				runtime:  tt.runtime,
				tier:     tt.tier,
			}
			err := notSupported(platform.ErrNotSupported, planned, tt.surface)
			if err == nil || ExitCode(err) != 2 {
				t.Fatalf("error/code = %v/%d", err, ExitCode(err))
			}
			want := tt.surface + " is not supported for target " + tt.provider + "/" + tt.runtime + " yet (" + string(tt.tier) + ")"
			if err.Error() != want {
				t.Fatalf("message = %q, want %q", err.Error(), want)
			}
			other := platform.TierCertified
			if tt.tier == platform.TierCertified {
				other = platform.TierExperimental
			}
			if strings.Contains(err.Error(), string(other)) {
				t.Fatalf("message must not contain the other tier %q: %v", other, err)
			}
		})
	}
}

func TestPortHelpersNameTargetAndTier(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"

	tests := []struct {
		name    string
		surface string
		call    func(*options) error
	}{
		{
			name:    "bootstrap",
			surface: "bootstrap",
			call: func(o *options) error {
				_, err := o.bootstrapPort()
				return err
			},
		},
		{
			name:    "state",
			surface: "state operations",
			call: func(o *options) error {
				_, err := o.statePort()
				return err
			},
		},
		{
			name:    "secrets",
			surface: "secrets",
			call: func(o *options) error {
				_, err := o.secretsPort()
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call(o)
			if err == nil || ExitCode(err) != 2 {
				t.Fatalf("error/code = %v/%d", err, ExitCode(err))
			}
			want := tt.surface + " is not supported for target aws/ecs-fargate yet (certified)"
			if err.Error() != want {
				t.Fatalf("message = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestPortHelpersNameExperimentalTier(t *testing.T) {
	path := writeExperimentalOVHConfig(t)
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath, o.environment = path, "staging"

	err := func() error {
		_, err := o.bootstrapPort()
		return err
	}()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
	want := "bootstrap is not supported for target ovh/mks yet (experimental)"
	if err.Error() != want {
		t.Fatalf("message = %q, want %q", err.Error(), want)
	}
	if strings.Contains(err.Error(), string(platform.TierCertified)) {
		t.Fatalf("experimental message must not contain certified: %v", err)
	}
}

func TestLogsErrNotSupportedNamesTier(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster"}}, nil
	}
	o.testRuntimeObserve = fakeRuntimeObserve{err: platform.ErrNotSupported}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "logs"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
	wantPrefix := "logs is not supported for target aws/ecs-fargate yet"
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("leading portion must survive; got: %v", err)
	}
	if !strings.Contains(err.Error(), "("+string(platform.TierCertified)+")") {
		t.Fatalf("message must name certified tier: %v", err)
	}
}

func TestCostErrNotSupportedNamesTierBothDirections(t *testing.T) {
	t.Run("certified", func(t *testing.T) {
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
		if !strings.HasPrefix(err.Error(), costNotSupportedPrefix) {
			t.Fatalf("01-03 prefix must survive byte-for-byte; got: %v", err)
		}
		if !strings.Contains(err.Error(), "("+string(platform.TierCertified)+")") {
			t.Fatalf("message must name certified tier: %v", err)
		}
		if strings.Contains(err.Error(), string(platform.TierExperimental)) {
			t.Fatalf("certified message must not contain experimental: %v", err)
		}
	})
	t.Run("experimental", func(t *testing.T) {
		path := writeExperimentalOVHConfig(t)
		var out bytes.Buffer
		o := testOptions(&out, nil)
		o.testCostEstimator = &recordingCostEstimator{err: platform.ErrNotSupported}
		cmd := newCommandWithOptions(o)
		cmd.SetArgs([]string{"--config", path, "--env", "staging", "cost"})
		err := cmd.Execute()
		if err == nil || ExitCode(err) != 2 {
			t.Fatalf("error/code = %v/%d", err, ExitCode(err))
		}
		prefix := "cost estimation is not supported for target ovh/mks yet"
		if !strings.HasPrefix(err.Error(), prefix) {
			t.Fatalf("leading portion must survive; got: %v", err)
		}
		if !strings.Contains(err.Error(), "("+string(platform.TierExperimental)+")") {
			t.Fatalf("message must name experimental tier: %v", err)
		}
		if strings.Contains(err.Error(), string(platform.TierCertified)) {
			t.Fatalf("experimental message must not contain certified: %v", err)
		}
	})
}

func TestHealthRuntimeErrNotSupportedNamesTier(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	var out bytes.Buffer
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath, o.environment, o.output = path, "staging", "json"
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster"}}, nil
	}
	o.testRuntimeObserve = fakeRuntimeObserve{err: platform.ErrNotSupported}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "health", "--mode", "runtime"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != healthExitUnavailable {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
	want := "runtime health is not supported for target aws/ecs-fargate yet (certified)"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("health check message must name surface/target/tier; got: %s", out.String())
	}
}

func writeExperimentalOVHConfig(t *testing.T) string {
	t.Helper()
	contents := `schemaVersion: 1
project:
  name: shop
application:
  edition: open-source
  version: "2.4.8"
  mode: integrated
build:
  php: "8.3"
target:
  provider: ovh
  runtime: mks
  ovh:
    serviceName: pc-example
defaults:
  region: GRA9
  preset: preview
environments:
  staging:
    account: "pc-example"
    class: staging
    domain: shop.example
    monthlyBudgetCents: 250000
extensions: {}
`
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
