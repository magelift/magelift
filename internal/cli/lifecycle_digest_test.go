package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	freshDeployDigest = "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	staleDeployDigest = "ghcr.io/magelift/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

var deployDigestPattern = regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`)

type digestProbeKey struct{}

// digestProbe refuses an empty, stale, or malformed digest on Plan, which is
// the provider boundary that runs before WithImageDigest.
type digestProbe struct {
	stale          string
	refuse         bool
	plans          []string
	allowExpired   []bool
	runtimeObserve []bool
	contexts       []context.Context
	backendOpened  bool
	backendDigest  string
}

func (p *digestProbe) Descriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "gcp.gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot"}
}

func (*digestProbe) CertificationTier() platform.CertificationTier {
	return platform.TierCertified
}

func (*digestProbe) OutputKeys() []string { return platform.RequiredOutputKeys() }

func (*digestProbe) Program(platform.PlannedStack) (pulumi.RunFunc, error) { return nil, nil }

func (p *digestProbe) Plan(cfg config.Config, environment string, opts platform.PlanOptions) (platform.PlannedStack, error) {
	if p.backendOpened {
		return nil, fmt.Errorf("provider plan ran after infrastructure backend creation")
	}
	if strings.TrimSpace(environment) == "" {
		return nil, fmt.Errorf("environment is required")
	}
	digest := ""
	if cfg.Target.GCP != nil {
		digest = cfg.Target.GCP.ImageDigest
	}
	p.plans = append(p.plans, digest)
	p.allowExpired = append(p.allowExpired, opts.AllowExpiredPreview)
	p.runtimeObserve = append(p.runtimeObserve, opts.RuntimeObserve)
	p.contexts = append(p.contexts, opts.Context)
	if p.refuse && !opts.RuntimeObserve && (digest == "" || digest == p.stale || !deployDigestPattern.MatchString(digest)) {
		return nil, fmt.Errorf("refusing image digest %q", digest)
	}
	return stubPlanned{
		stackName: cfg.Project.Name + "-" + environment, provider: "gcp", runtime: "gke-autopilot",
		project: cfg.Project.Name, environment: environment, region: cfg.Defaults.Region,
		envClass: cfg.Class, protected: cfg.Protection, digest: digest, tier: platform.TierCertified,
	}, nil
}

func writeFreshGCPConfig(t *testing.T, imageDigest string) (string, []byte) {
	t.Helper()
	body := fmt.Sprintf(minimalGCPStarterTemplate, strconv.Quote("shop"), "open-source", "2.4.8-p5")
	body = strings.Replace(body, "php: \"8.5\"", "php: \"8.4\"", 1)
	if imageDigest != "" {
		needle := "    project: example-gcp-project\n"
		body = strings.Replace(body, needle, needle+"    imageDigest: "+imageDigest+"\n", 1)
	}
	data := []byte(body)
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data
}

func digestDeployOptions(t *testing.T, path string, probe *digestProbe) *options {
	t.Helper()
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "test"
	if err := o.modules.RegisterModule(probe); err != nil {
		t.Fatal(err)
	}
	o.newBackend = func(_ context.Context, planned platform.PlannedStack, _ string) (infrastructureBackend, error) {
		probe.backendOpened = true
		probe.backendDigest = planned.ImageDigest()
		return &fakeInfrastructureBackend{}, nil
	}
	o.newLock = func(context.Context, platform.PlannedStack) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	return o
}

func assertConfigUnchanged(t *testing.T, path string, original []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("source config changed:\n%s", got)
	}
}

func TestDeployDigestReachesFirstPlanBeforeBackend(t *testing.T) {
	for _, test := range []struct {
		name       string
		yamlDigest string
		flag       string
		wantErr    bool
	}{
		{name: "fresh config", flag: freshDeployDigest},
		{name: "replaces stale yaml", yamlDigest: staleDeployDigest, flag: freshDeployDigest},
		{name: "rejects stale flag", flag: staleDeployDigest, wantErr: true},
		{name: "rejects invalid flag", flag: "ghcr.io/magelift/magento:latest", wantErr: true},
		{name: "rejects missing flag", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, original := writeFreshGCPConfig(t, test.yamlDigest)
			probe := &digestProbe{stale: staleDeployDigest, refuse: true}
			o := digestDeployOptions(t, path, probe)
			cmd := newCommandWithOptions(o)
			args := []string{"--config", path, "--env", "test", "--output", "json", "deploy"}
			if test.flag != "" {
				args = append(args, "--digest", test.flag)
			}
			cmd.SetArgs(args)
			err := cmd.Execute()
			assertConfigUnchanged(t, path, original)
			if test.wantErr {
				if err == nil || ExitCode(err) != 2 || probe.backendOpened {
					t.Fatalf("err=%v backend=%v plans=%v", err, probe.backendOpened, probe.plans)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(probe.plans) == 0 || probe.plans[0] != freshDeployDigest || !probe.backendOpened || probe.backendDigest != freshDeployDigest {
				t.Fatalf("plans=%v backend=%v digest=%q", probe.plans, probe.backendOpened, probe.backendDigest)
			}
			for _, seen := range probe.plans {
				if seen != freshDeployDigest {
					t.Fatalf("plan digest = %q", seen)
				}
			}
			effective, _, resolveErr := o.resolveWithEnvironment()
			if resolveErr != nil {
				t.Fatal(resolveErr)
			}
			if effective.Config.Target.GCP == nil || effective.Config.Target.GCP.ImageDigest != test.yamlDigest {
				t.Fatalf("resolved digest = %#v", effective.Config.Target.GCP)
			}
		})
	}
}

func TestDestroyPlanKeepsResolvedDigestAndExpiredPreview(t *testing.T) {
	path, original := writeFreshGCPConfig(t, "")
	probe := &digestProbe{}
	o := digestDeployOptions(t, path, probe)
	var plannedDigest string
	o.newBackend = func(_ context.Context, planned platform.PlannedStack, _ string) (infrastructureBackend, error) {
		probe.backendOpened = true
		plannedDigest = planned.ImageDigest()
		return &fakeInfrastructureBackend{}, nil
	}
	_, err := o.executeInfrastructure(context.Background(), "destroy", func(context.Context, infrastructureBackend, automation.Request) (infrastructureResult, error) {
		return infrastructureResult{}, nil
	}, freshDeployDigest)
	if err != nil {
		t.Fatal(err)
	}
	assertConfigUnchanged(t, path, original)
	if len(probe.plans) == 0 || probe.plans[0] != "" || !probe.allowExpired[0] {
		t.Fatalf("destroy plan = %#v expired=%v", probe.plans, probe.allowExpired)
	}
	if plannedDigest != freshDeployDigest {
		t.Fatalf("destroy WithImageDigest = %q", plannedDigest)
	}
}

func TestPlanStackWithPreservesContextAndEmptyDigest(t *testing.T) {
	path, _ := writeFreshGCPConfig(t, "")
	probe := &digestProbe{}
	o := digestDeployOptions(t, path, probe)
	fallback := context.WithValue(context.Background(), digestProbeKey{}, "fallback")
	explicit := context.WithValue(context.Background(), digestProbeKey{}, "explicit")
	o.planContext = fallback
	if _, _, err := o.planStackWith(platform.PlanOptions{Context: explicit}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.planStack(true); err != nil {
		t.Fatal(err)
	}
	if len(probe.contexts) != 2 || probe.contexts[0].Value(digestProbeKey{}) != "explicit" || probe.contexts[1].Value(digestProbeKey{}) != "fallback" {
		t.Fatalf("contexts = %#v", probe.contexts)
	}
	if probe.plans[0] != "" || probe.plans[1] != "" || !probe.allowExpired[1] || probe.allowExpired[0] {
		t.Fatalf("plans=%v expired=%v", probe.plans, probe.allowExpired)
	}
}

func TestPlannedOutputsUsesRuntimeObservePlan(t *testing.T) {
	path, _ := writeFreshGCPConfig(t, "")
	probe := &digestProbe{refuse: true}
	o := digestDeployOptions(t, path, probe)
	if _, _, _, err := o.plannedOutputs(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(probe.runtimeObserve) != 1 || !probe.runtimeObserve[0] {
		t.Fatalf("runtimeObserve = %v", probe.runtimeObserve)
	}
}

func TestOutputCommandsUseRuntimeObservePlan(t *testing.T) {
	for _, operation := range []string{"outputs", "health-outputs"} {
		t.Run(operation, func(t *testing.T) {
			path, original := writeFreshGCPConfig(t, "")
			probe := &digestProbe{refuse: true}
			o := digestDeployOptions(t, path, probe)
			if operation == "outputs" {
				cmd := newCommandWithOptions(o)
				cmd.SetArgs([]string{"--config", path, "--env", "test", "--output", "json", "outputs"})
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := o.runHealth(context.Background(), "outputs"); err != nil {
					t.Fatal(err)
				}
			}
			assertConfigUnchanged(t, path, original)
			if len(probe.runtimeObserve) != 1 || !probe.runtimeObserve[0] || !probe.backendOpened {
				t.Fatalf("runtimeObserve=%v backend=%v", probe.runtimeObserve, probe.backendOpened)
			}
		})
	}
}

func TestDeployPlanNeverSetsRuntimeObserve(t *testing.T) {
	path, original := writeFreshGCPConfig(t, "")
	probe := &digestProbe{}
	o := digestDeployOptions(t, path, probe)
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "test", "--output", "json", "deploy", "--digest", freshDeployDigest})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	assertConfigUnchanged(t, path, original)
	for _, observe := range probe.runtimeObserve {
		if observe {
			t.Fatal("deploy planning set RuntimeObserve")
		}
	}
}

func TestConfigWithImageDigestCopiesEveryTarget(t *testing.T) {
	aws := &config.AWSTarget{ImageDigest: "aws-old", VPCCIDR: "10.0.0.0/16", AvailabilityZones: []string{"eu-west-3a"}}
	gcp := &config.GCPTarget{Project: "example-gcp-project", ImageDigest: "gcp-old", Labels: map[string]string{"k": "v"}}
	ovh := &config.OVHTarget{ServiceName: "project", ImageDigest: "ovh-old"}
	scw := &config.ScalewayTarget{ProjectID: "project", ImageDigest: "scw-old"}
	for _, test := range []struct {
		name   string
		cfg    config.Config
		mutate func(before, after config.Config)
	}{
		{
			name: "aws",
			cfg:  config.Config{Target: config.Target{Provider: "aws", AWS: aws}},
			mutate: func(before, after config.Config) {
				if before.Target.AWS != aws || aws.ImageDigest != "aws-old" || after.Target.AWS == aws {
					t.Fatalf("aws alias: before=%p after=%p digest=%q", before.Target.AWS, after.Target.AWS, aws.ImageDigest)
				}
				if after.Target.AWS.ImageDigest != freshDeployDigest || after.Target.AWS.VPCCIDR != "10.0.0.0/16" || len(after.Target.AWS.AvailabilityZones) != 1 {
					t.Fatalf("aws copy = %#v", after.Target.AWS)
				}
			},
		},
		{
			name: "gcp",
			cfg:  config.Config{Target: config.Target{Provider: "gcp", AWS: aws, GCP: gcp}},
			mutate: func(before, after config.Config) {
				if before.Target.GCP != gcp || gcp.ImageDigest != "gcp-old" || after.Target.GCP == gcp || after.Target.AWS != aws || aws.ImageDigest != "aws-old" {
					t.Fatalf("gcp alias or sibling mutated")
				}
				if after.Target.GCP.ImageDigest != freshDeployDigest || after.Target.GCP.Project != "example-gcp-project" || after.Target.GCP.Labels["k"] != "v" {
					t.Fatalf("gcp copy = %#v", after.Target.GCP)
				}
			},
		},
		{
			name: "ovh",
			cfg:  config.Config{Target: config.Target{Provider: "ovh", OVH: ovh}},
			mutate: func(before, after config.Config) {
				if before.Target.OVH != ovh || ovh.ImageDigest != "ovh-old" || after.Target.OVH == ovh {
					t.Fatalf("ovh alias")
				}
				if after.Target.OVH.ImageDigest != freshDeployDigest || after.Target.OVH.ServiceName != "project" {
					t.Fatalf("ovh copy = %#v", after.Target.OVH)
				}
			},
		},
		{
			name: "scaleway",
			cfg:  config.Config{Target: config.Target{Provider: "scaleway", Scaleway: scw}},
			mutate: func(before, after config.Config) {
				if before.Target.Scaleway != scw || scw.ImageDigest != "scw-old" || after.Target.Scaleway == scw {
					t.Fatalf("scaleway alias")
				}
				if after.Target.Scaleway.ImageDigest != freshDeployDigest || after.Target.Scaleway.ProjectID != "project" {
					t.Fatalf("scaleway copy = %#v", after.Target.Scaleway)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := test.cfg
			after, err := configWithImageDigest(before, freshDeployDigest)
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(before, after)
		})
	}
	if _, err := configWithImageDigest(config.Config{Target: config.Target{Provider: "gcp"}}, freshDeployDigest); err == nil {
		t.Fatal("nil gcp target was accepted")
	}
	external := config.Config{Target: config.Target{Provider: "other", GCP: gcp}}
	untouched, err := configWithImageDigest(external, freshDeployDigest)
	if err != nil || untouched.Target.GCP != gcp || gcp.ImageDigest != "gcp-old" {
		t.Fatal("external module configuration changed before its typed artifact override")
	}
}
