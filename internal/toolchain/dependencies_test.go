package toolchain

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type dependencyProbe struct {
	paths  map[string]string
	output map[string][]byte
	err    map[string]error
	args   [][]string
}

func (p *dependencyProbe) LookPath(command string) (string, error) {
	path, ok := p.paths[command]
	if !ok {
		return "", errors.New("not found")
	}
	return path, nil
}

func (p *dependencyProbe) Run(_ context.Context, command string, args ...string) ([]byte, error) {
	p.args = append(p.args, append([]string{command}, args...))
	key := command
	if len(args) > 0 {
		key += " " + args[0]
	}
	return p.output[key], p.err[key]
}

func TestCheckDependenciesPreservesVersionArgvAndReportsAvailable(t *testing.T) {
	probe := &dependencyProbe{
		paths:  map[string]string{"pulumi": "/usr/local/bin/pulumi"},
		output: map[string][]byte{"pulumi version": []byte("v3.200.0\n")},
	}
	report := CheckDependencies(context.Background(), probe, []DependencySpec{{
		ID: "pulumi", Command: "pulumi", Capability: "Automation API", Requirement: DependencyRequired, VersionArgs: []string{"version"}, InstallPackage: "pulumi",
	}})
	if report.Status != "ready" || len(report.Checks) != 1 {
		t.Fatalf("report = %#v", report)
	}
	check := report.Checks[0]
	if check.Status != DependencyAvailable || check.Path != "/usr/local/bin/pulumi" || check.Version != "v3.200.0" || check.InstallHint != "" {
		t.Fatalf("check = %#v", check)
	}
	if want := [][]string{{"pulumi", "version"}}; !reflect.DeepEqual(probe.args, want) {
		t.Fatalf("argv = %#v, want %#v", probe.args, want)
	}
}

func TestCheckDependenciesDistinguishesRequiredAndOptionalFailures(t *testing.T) {
	probe := &dependencyProbe{paths: map[string]string{"brew": "/opt/homebrew/bin/brew"}}
	report := CheckDependencies(context.Background(), probe, []DependencySpec{
		{ID: "docker", Command: "docker", Capability: "local runtime", Requirement: DependencyOptional, InstallPackage: "docker-desktop", HomebrewCask: true},
		{ID: "pulumi", Command: "pulumi", Capability: "cloud planning", Requirement: DependencyRequired, InstallPackage: "pulumi"},
	})
	if report.Status != "unavailable" || len(report.Checks) != 2 {
		t.Fatalf("report = %#v", report)
	}
	if report.Checks[0].Status != DependencyMissing || report.Checks[0].InstallHint != "brew install --cask docker-desktop" {
		t.Fatalf("optional check = %#v", report.Checks[0])
	}
	if report.Checks[1].Status != DependencyMissing || report.Checks[1].InstallHint != "brew install pulumi" {
		t.Fatalf("required check = %#v", report.Checks[1])
	}
	if len(probe.args) != 0 {
		t.Fatalf("missing dependencies were executed: %#v", probe.args)
	}
}

func TestCheckDependenciesReportsVersionProbeFailure(t *testing.T) {
	probe := &dependencyProbe{
		paths: map[string]string{"docker": "/usr/local/bin/docker", "brew": "/opt/homebrew/bin/brew"},
		err:   map[string]error{"docker version": errors.New("daemon unavailable")},
	}
	report := CheckDependencies(context.Background(), probe, []DependencySpec{{
		ID: "docker", Command: "docker", Capability: "local runtime", Requirement: DependencyRequired, VersionArgs: []string{"version"}, HomebrewCask: true, InstallPackage: "docker-desktop",
	}})
	if report.Status != "unavailable" || report.Checks[0].Status != DependencyUnreachable {
		t.Fatalf("report = %#v", report)
	}
	if report.Checks[0].Message != "docker is installed but could not be used" {
		t.Fatalf("message = %q", report.Checks[0].Message)
	}
}

func TestSpecsForLocalRequiresDockerAndCompose(t *testing.T) {
	specs := SpecsForLocal()
	if len(specs) != 2 {
		t.Fatalf("specs = %#v", specs)
	}
	for _, spec := range specs {
		if spec.Command != "docker" || spec.Requirement != DependencyRequired || spec.InstallPackage != "docker-desktop" {
			t.Fatalf("spec = %#v", spec)
		}
	}
}

func TestSpecsForCloudReleaseVerificationDoNotRequireDocker(t *testing.T) {
	specs := SpecsForCosignVerification()
	if len(specs) != 1 || specs[0].ID != "cosign" || specs[0].Requirement != DependencyRequired {
		t.Fatalf("specs = %#v", specs)
	}
}

func TestSpecsForStandaloneLocalImageBuildRequireGitAndDockerOnly(t *testing.T) {
	specs := SpecsForLocalImageBuild()
	if len(specs) != 2 || specs[0].ID != "git" || specs[1].ID != "docker" {
		t.Fatalf("specs = %#v", specs)
	}
	for _, spec := range specs {
		if spec.Requirement != DependencyRequired {
			t.Fatalf("spec = %#v", spec)
		}
	}
}

func TestSpecsForDumpImportSelectsHostMySQLBeforeCompose(t *testing.T) {
	probe := &dependencyProbe{paths: map[string]string{"mysql": "/usr/local/bin/mysql"}}
	specs := SpecsForDumpImport(probe, "", true)
	if len(specs) != 2 || specs[0].ID != "gunzip" || specs[1].ID != "mysql" {
		t.Fatalf("specs = %#v", specs)
	}
}

func TestSpecsForDumpImportUsesKubernetesTransport(t *testing.T) {
	probe := &dependencyProbe{}
	specs := SpecsForDumpImport(probe, "kube", false)
	if len(specs) != 1 || specs[0].ID != "kubectl" || specs[0].Requirement != DependencyRequired {
		t.Fatalf("specs = %#v", specs)
	}
}

func TestSpecsForDumpImportFallsBackToDockerCompose(t *testing.T) {
	probe := &dependencyProbe{}
	specs := SpecsForDumpImport(probe, "", false)
	if len(specs) != 2 || specs[0].ID != "docker" || specs[1].ID != "docker-compose" {
		t.Fatalf("specs = %#v", specs)
	}
}

func TestSpecsForDumpExportSelectsHostMysqldumpBeforeCompose(t *testing.T) {
	probe := &dependencyProbe{paths: map[string]string{"mysqldump": "/usr/local/bin/mysqldump"}}
	specs := SpecsForDumpExport(probe, "")
	if len(specs) != 1 || specs[0].ID != "mysqldump" {
		t.Fatalf("specs = %#v", specs)
	}
}

func TestSpecsForDumpExportUsesKubernetesTransport(t *testing.T) {
	probe := &dependencyProbe{}
	specs := SpecsForDumpExport(probe, "kube")
	if len(specs) != 1 || specs[0].ID != "kubectl" || specs[0].Requirement != DependencyRequired {
		t.Fatalf("specs = %#v", specs)
	}
}

func TestPlanAndInstallDependenciesUseAllowlistedArgv(t *testing.T) {
	probe := &dependencyProbe{paths: map[string]string{"brew": "/opt/homebrew/bin/brew"}}
	specs := []DependencySpec{
		{ID: "docker", Command: "docker", HomebrewCask: true, InstallPackage: "docker-desktop", Installable: true},
		{ID: "pulumi", Command: "pulumi", InstallPackage: "pulumi", Installable: true},
	}
	report := DependencyReport{Status: "unavailable", Checks: []DependencyCheck{
		{ID: "docker", Status: DependencyMissing, Requirement: DependencyRequired},
		{ID: "pulumi", Status: DependencyMissing, Requirement: DependencyRequired},
	}}
	actions, err := PlanDependencyInstalls(probe, specs, report)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 || actions[0].Executable != "brew" || actions[0].Args[0] != "install" {
		t.Fatalf("actions = %#v", actions)
	}
	for _, action := range actions {
		if action.Executable == "sh" || contains(action.Args, "-c") {
			t.Fatalf("shell action = %#v", action)
		}
	}
	if err := InstallDependencies(context.Background(), probe, actions); err != nil {
		t.Fatal(err)
	}
	if got := probe.args; len(got) != 2 || got[0][0] != "brew" || got[1][0] != "brew" {
		t.Fatalf("install calls = %#v", got)
	}
}

func TestPlanDependencyInstallsRejectsUnsupportedDockerManager(t *testing.T) {
	probe := &dependencyProbe{paths: map[string]string{"scoop": "C:\\scoop\\shims\\scoop.exe"}}
	specs := []DependencySpec{{ID: "docker", Command: "docker", HomebrewCask: true, InstallPackage: "docker-desktop", Installable: true}}
	report := DependencyReport{Status: "unavailable", Checks: []DependencyCheck{{ID: "docker", Status: DependencyMissing, Requirement: DependencyRequired}}}
	if _, err := PlanDependencyInstalls(probe, specs, report); err == nil {
		t.Fatal("expected unsupported Docker package-manager error")
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestSpecsForTargetGuidesGcloudForGCP(t *testing.T) {
	var gcloud *DependencySpec
	for _, spec := range SpecsForTarget("gcp", "gke-autopilot") {
		if spec.ID == "gcloud" {
			gcloud = &spec
		}
	}
	if gcloud == nil {
		t.Fatal("gcp specs lack gcloud guidance")
	}
	if gcloud.Requirement != DependencyOptional || gcloud.Installable {
		t.Fatalf("gcloud spec = %#v, want guided-optional without auto-install", gcloud)
	}
	for _, spec := range SpecsForTarget("aws", "ecs-fargate") {
		if spec.ID == "gcloud" {
			t.Fatal("aws specs must not include gcloud")
		}
	}
}
