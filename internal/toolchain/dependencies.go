package toolchain

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type DependencyStatus string

const (
	DependencyAvailable   DependencyStatus = "available"
	DependencyMissing     DependencyStatus = "missing"
	DependencyUnreachable DependencyStatus = "unreachable"
)

type DependencyRequirement string

const (
	DependencyRequired DependencyRequirement = "required"
	DependencyOptional DependencyRequirement = "optional"
)

// DependencySpec describes one executable needed by a capability. VersionArgs
// is an argv vector, not a shell command.
type DependencySpec struct {
	ID             string
	Command        string
	Capability     string
	Requirement    DependencyRequirement
	VersionArgs    []string
	HomebrewCask   bool
	InstallPackage string
	Installable    bool
}

type DependencyCheck struct {
	ID          string                `json:"id" yaml:"id"`
	Command     string                `json:"command" yaml:"command"`
	Capability  string                `json:"capability" yaml:"capability"`
	Requirement DependencyRequirement `json:"requirement" yaml:"requirement"`
	Status      DependencyStatus      `json:"status" yaml:"status"`
	Path        string                `json:"path,omitempty" yaml:"path,omitempty"`
	Version     string                `json:"version,omitempty" yaml:"version,omitempty"`
	Message     string                `json:"message" yaml:"message"`
	InstallHint string                `json:"installHint,omitempty" yaml:"installHint,omitempty"`
}

type DependencyReport struct {
	Status string            `json:"status" yaml:"status"`
	Checks []DependencyCheck `json:"checks" yaml:"checks"`
}

type DependencyRunner interface {
	LookPath(string) (string, error)
	Run(context.Context, string, ...string) ([]byte, error)
}

// DependencyInstallAction is an argv-only package-manager invocation. The
// executable and arguments are selected by MageLift's allowlist; callers must
// not turn this into a shell string.
type DependencyInstallAction struct {
	DependencyID string
	Manager      string
	Executable   string
	Args         []string
}

// CheckDependencies performs read-only executable and version probes.
func CheckDependencies(ctx context.Context, runner DependencyRunner, specs []DependencySpec) DependencyReport {
	if ctx == nil {
		ctx = context.Background()
	}
	report := DependencyReport{Status: "ready", Checks: make([]DependencyCheck, 0, len(specs))}
	if runner == nil {
		return DependencyReport{Status: "unavailable", Checks: []DependencyCheck{{
			ID: "dependency-runner", Command: "", Capability: "dependency preflight", Requirement: DependencyRequired,
			Status: DependencyUnreachable, Message: "dependency runner is unavailable", InstallHint: "",
		}}}
	}
	for _, spec := range specs {
		check := DependencyCheck{
			ID: spec.ID, Command: spec.Command, Capability: spec.Capability,
			Requirement: spec.Requirement, Status: DependencyAvailable,
		}
		path, err := runner.LookPath(spec.Command)
		if err != nil {
			check.Status = DependencyMissing
			check.Message = fmt.Sprintf("%s is not installed", spec.Command)
			check.InstallHint = installHint(runner, spec)
			report.Checks = append(report.Checks, check)
			if spec.Requirement == DependencyRequired {
				report.Status = "unavailable"
			} else if report.Status == "ready" {
				report.Status = "degraded"
			}
			continue
		}
		check.Path = path
		output, err := runner.Run(ctx, spec.Command, spec.VersionArgs...)
		if err != nil {
			check.Status = DependencyUnreachable
			check.Message = fmt.Sprintf("%s is installed but could not be used", spec.Command)
			if detail := boundedOutput(output, 240); detail != "" {
				check.Message += ": " + detail
			}
			check.InstallHint = installHint(runner, spec)
			if spec.Requirement == DependencyRequired {
				report.Status = "unavailable"
			} else if report.Status == "ready" {
				report.Status = "degraded"
			}
			report.Checks = append(report.Checks, check)
			continue
		}
		check.Version = boundedOutput(output, 240)
		check.Message = "available"
		report.Checks = append(report.Checks, check)
	}
	return report
}

// SpecsForLocal returns the host requirements for any operation that starts
// or changes local containers. Both probes are required because Compose is a
// Docker subcommand, but a separate check makes the user-facing capability
// and remediation explicit.
func SpecsForLocal() []DependencySpec {
	return dockerDependencySpecs(DependencyRequired)
}

// SpecsForBuild returns the host requirements for a local build. Composer and
// Magento run inside the pinned builder image; Git, Docker, and (for pushed
// releases) cosign are host launchers.
func SpecsForBuild(push bool) []DependencySpec {
	specs := dockerDependencySpecs(DependencyRequired)
	specs = append(specs, DependencySpec{ID: "git", Command: "git", Capability: "source revision and provenance", Requirement: DependencyRequired, VersionArgs: []string{"--version"}, InstallPackage: "git", Installable: true})
	if push {
		specs = append(specs, SpecsForCosignVerification()...)
	}
	return specs
}

// SpecsForCosignVerification is intentionally separate from the local build
// contract because release verification is also used by cloud-only commands.
func SpecsForCosignVerification() []DependencySpec {
	return []DependencySpec{{ID: "cosign", Command: "cosign", Capability: "signed release verification", Requirement: DependencyRequired, VersionArgs: []string{"version"}, InstallPackage: "cosign", Installable: true}}
}

// SpecsForLocalImageBuild covers the standalone image-builder binary. Compose
// is not part of that pipeline, so it must not inherit the broader build list.
func SpecsForLocalImageBuild() []DependencySpec {
	return []DependencySpec{
		{ID: "git", Command: "git", Capability: "source revision and provenance", Requirement: DependencyRequired, VersionArgs: []string{"--version"}, InstallPackage: "git", Installable: true},
		{ID: "docker", Command: "docker", Capability: "local Docker image build", Requirement: DependencyRequired, VersionArgs: []string{"version", "--format", "{{.Server.Version}}"}, HomebrewCask: true, InstallPackage: "docker-desktop", Installable: true},
	}
}

// SpecsForSkillsCLI describes the optional Node-backed user skill installer.
// The CLI deliberately does not install Node or npx automatically because the
// package manager and version policy belong to the user's workstation.
func SpecsForSkillsCLI() []DependencySpec {
	return []DependencySpec{{ID: "npx", Command: "npx", Capability: "user skill installation backend", Requirement: DependencyRequired, VersionArgs: []string{"--version"}, InstallPackage: "node"}}
}

// SpecsForFastly describes the external Fastly CLI used by the edge adapter.
func SpecsForFastly() []DependencySpec {
	return []DependencySpec{{ID: "fastly", Command: "fastly", Capability: "Fastly edge lifecycle", Requirement: DependencyRequired, VersionArgs: []string{"version"}, InstallPackage: "fastly"}}
}

// SpecsForDumpImport selects the actual local dump-import transport. A host
// mysql client takes precedence over the Docker Compose fallback, matching the
// importer; Kubernetes mode always uses kubectl. A compressed dump also needs
// the host gunzip executable because decompression happens before the SQL
// stream reaches the selected transport.
func SpecsForDumpImport(runner DependencyRunner, transport string, compressed bool) []DependencySpec {
	specs := make([]DependencySpec, 0, 3)
	if compressed {
		specs = append(specs, DependencySpec{
			ID: "gunzip", Command: "gunzip", Capability: "compressed dump decompression",
			Requirement: DependencyRequired, VersionArgs: []string{"--version"}, InstallPackage: "gzip", Installable: true,
		})
	}
	if strings.EqualFold(strings.TrimSpace(transport), "kube") {
		return append(specs, DependencySpec{
			ID: "kubectl", Command: "kubectl", Capability: "Kubernetes dump import transport",
			Requirement: DependencyRequired, VersionArgs: []string{"version", "--client=true", "--output=yaml"}, InstallPackage: "kubectl", Installable: true,
		})
	}
	if runner != nil {
		if _, err := runner.LookPath("mysql"); err == nil {
			return append(specs, DependencySpec{
				ID: "mysql", Command: "mysql", Capability: "host database dump import transport",
				Requirement: DependencyRequired, VersionArgs: []string{"--version"}, InstallPackage: "mysql-client",
			})
		}
	}
	return append(specs, dockerDependencySpecs(DependencyRequired)...)
}

// SpecsForDumpExport selects the live dump-retrieve transport. Kubernetes mode
// uses kubectl (mysqldump runs in the VPC-adjacent pod). Host mysqldump takes
// precedence over the Docker Compose fallback, matching Export. Gzip wrapping
// is done in Go and does not require a host gzip binary.
func SpecsForDumpExport(runner DependencyRunner, transport string) []DependencySpec {
	if strings.EqualFold(strings.TrimSpace(transport), "kube") {
		return []DependencySpec{{
			ID: "kubectl", Command: "kubectl", Capability: "Kubernetes dump retrieve transport",
			Requirement: DependencyRequired, VersionArgs: []string{"version", "--client=true", "--output=yaml"}, InstallPackage: "kubectl", Installable: true,
		}}
	}
	if runner != nil {
		if _, err := runner.LookPath("mysqldump"); err == nil {
			return []DependencySpec{{
				ID: "mysqldump", Command: "mysqldump", Capability: "host database dump retrieve transport",
				Requirement: DependencyRequired, VersionArgs: []string{"--version"}, InstallPackage: "mysql-client",
			}}
		}
	}
	return dockerDependencySpecs(DependencyRequired)
}

// PlanDependencyInstalls creates explicit package-manager actions for missing
// dependencies. Installed-but-unusable executables are intentionally excluded:
// replacing a broken installation needs a separate operator decision.
func PlanDependencyInstalls(runner DependencyRunner, specs []DependencySpec, report DependencyReport) ([]DependencyInstallAction, error) {
	if runner == nil {
		return nil, fmt.Errorf("dependency runner is unavailable")
	}
	missing := make(map[string]struct{})
	for _, check := range report.Checks {
		if check.Status == DependencyMissing {
			missing[check.ID] = struct{}{}
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	manager, ok := packageManager(runner)
	if !ok {
		return nil, errors.New("no supported package manager was found; install dependencies from their official distributions")
	}
	actions := make([]DependencyInstallAction, 0, len(missing))
	seen := make(map[string]struct{})
	for _, spec := range specs {
		if _, needed := missing[spec.ID]; !needed {
			continue
		}
		action, ok := dependencyInstallAction(manager, spec)
		if !ok {
			return nil, fmt.Errorf("no allowlisted %s installation is available for dependency %q; use the hint in doctor output", manager, spec.ID)
		}
		key := action.Executable + "\x00" + strings.Join(action.Args, "\x00")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		actions = append(actions, action)
	}
	return actions, nil
}

// InstallDependencies executes only actions returned by
// PlanDependencyInstalls. It never invokes a shell.
func InstallDependencies(ctx context.Context, runner DependencyRunner, actions []DependencyInstallAction) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if runner == nil {
		return errors.New("dependency runner is unavailable")
	}
	for _, action := range actions {
		if action.Executable == "" || len(action.Args) == 0 {
			return fmt.Errorf("invalid install action for dependency %q", action.DependencyID)
		}
		if _, err := runner.Run(ctx, action.Executable, action.Args...); err != nil {
			return fmt.Errorf("install dependency %q with %s: %w", action.DependencyID, action.Manager, err)
		}
	}
	return nil
}

// SpecsForTarget returns dependencies for cloud operations plus optional local
// and day-2 launchers. The Go SDKs remain the provider API boundary; provider
// CLIs are listed only when a returned ExecTarget launches them.
func SpecsForTarget(provider, runtimeID string) []DependencySpec {
	specs := []DependencySpec{
		{ID: "pulumi", Command: "pulumi", Capability: "cloud infrastructure Automation API", Requirement: DependencyRequired, VersionArgs: []string{"version"}, InstallPackage: "pulumi", Installable: true},
	}
	specs = append(specs, dockerDependencySpecs(DependencyOptional)...)

	switch runtimeID {
	case "eks", "gke-autopilot", "gke-standard", "kapsule", "mks":
		specs = append(specs, DependencySpec{ID: "kubectl", Command: "kubectl", Capability: "Kubernetes exec and private tunnels", Requirement: DependencyOptional, VersionArgs: []string{"version", "--client=true", "--output=yaml"}, InstallPackage: "kubectl", Installable: true})
	}
	if provider == "gcp" {
		specs = append(specs, DependencySpec{ID: "cloud-sql-proxy", Command: "cloud-sql-proxy", Capability: "GCP private database tunnels", Requirement: DependencyOptional, VersionArgs: []string{"--version"}, InstallPackage: "cloud-sql-proxy", Installable: true})
	}
	if provider == "aws" && runtimeID == "ecs-fargate" {
		specs = append(specs,
			DependencySpec{ID: "aws", Command: "aws", Capability: "AWS ECS exec and ssh", Requirement: DependencyOptional, VersionArgs: []string{"--version"}, InstallPackage: "awscli", Installable: true},
			DependencySpec{ID: "session-manager-plugin", Command: "session-manager-plugin", Capability: "AWS ECS interactive sessions", Requirement: DependencyOptional, VersionArgs: []string{"--version"}, InstallPackage: "session-manager-plugin", Installable: true},
		)
	}
	return specs
}

// SpecsForLauncher describes tools needed only when an operator starts a
// remote session. Planning and session-only output can work without these
// launchers being installed.
func SpecsForLauncher(launcher string) []DependencySpec {
	switch strings.TrimSpace(launcher) {
	case "", "aws":
		return []DependencySpec{
			{ID: "aws", Command: "aws", Capability: "AWS remote sessions", Requirement: DependencyRequired, VersionArgs: []string{"--version"}, InstallPackage: "awscli", Installable: true},
			{ID: "session-manager-plugin", Command: "session-manager-plugin", Capability: "AWS ECS interactive sessions", Requirement: DependencyRequired, VersionArgs: []string{"--version"}, InstallPackage: "session-manager-plugin", Installable: true},
		}
	case "kubectl":
		return []DependencySpec{{ID: "kubectl", Command: "kubectl", Capability: "Kubernetes exec and private tunnels", Requirement: DependencyRequired, VersionArgs: []string{"version", "--client=true", "--output=yaml"}, InstallPackage: "kubectl", Installable: true}}
	case "cloud-sql-proxy":
		return []DependencySpec{{ID: "cloud-sql-proxy", Command: "cloud-sql-proxy", Capability: "GCP private database tunnels", Requirement: DependencyRequired, VersionArgs: []string{"--version"}, InstallPackage: "cloud-sql-proxy", Installable: true}}
	default:
		command := strings.TrimSpace(launcher)
		if command == "" || strings.ContainsAny(command, "\x00\r\n /\\") {
			return nil
		}
		return []DependencySpec{{ID: "launcher." + command, Command: command, Capability: "remote session launcher", Requirement: DependencyRequired, VersionArgs: []string{"--version"}, InstallPackage: command}}
	}
}

func dockerDependencySpecs(requirement DependencyRequirement) []DependencySpec {
	return []DependencySpec{
		{ID: "docker", Command: "docker", Capability: "local Docker runtime and image builds", Requirement: requirement, VersionArgs: []string{"version", "--format", "{{.Server.Version}}"}, HomebrewCask: true, InstallPackage: "docker-desktop", Installable: true},
		{ID: "docker-compose", Command: "docker", Capability: "local Docker Compose", Requirement: requirement, VersionArgs: []string{"compose", "version"}, HomebrewCask: true, InstallPackage: "docker-desktop", Installable: true},
	}
}

func installHint(runner DependencyRunner, spec DependencySpec) string {
	packageName := strings.TrimSpace(spec.InstallPackage)
	if packageName == "" {
		packageName = spec.Command
	}
	if brew, err := runner.LookPath("brew"); err == nil && brew != "" {
		if spec.HomebrewCask {
			return "brew install --cask " + packageName
		}
		return "brew install " + packageName
	}
	if scoop, err := runner.LookPath("scoop"); err == nil && scoop != "" {
		if spec.HomebrewCask {
			return "install " + spec.Command + " from its official distribution"
		}
		return "scoop install " + packageName
	}
	if runtime.GOOS == "linux" {
		if spec.HomebrewCask {
			return "install " + spec.Command + " from its official distribution"
		}
		if apt, err := runner.LookPath("apt-get"); err == nil && apt != "" {
			return "sudo apt-get install -y " + packageName
		}
		if dnf, err := runner.LookPath("dnf"); err == nil && dnf != "" {
			return "sudo dnf install -y " + packageName
		}
	}
	return "install " + spec.Command + " from its official distribution"
}

func packageManager(runner DependencyRunner) (string, bool) {
	if path, err := runner.LookPath("brew"); err == nil && path != "" {
		return "brew", true
	}
	if path, err := runner.LookPath("scoop"); err == nil && path != "" {
		return "scoop", true
	}
	if runtime.GOOS == "linux" {
		if path, err := runner.LookPath("apt-get"); err == nil && path != "" {
			if sudo, err := runner.LookPath("sudo"); err == nil && sudo != "" {
				return "apt-get", true
			}
		}
		if path, err := runner.LookPath("dnf"); err == nil && path != "" {
			if sudo, err := runner.LookPath("sudo"); err == nil && sudo != "" {
				return "dnf", true
			}
		}
	}
	return "", false
}

func dependencyInstallAction(manager string, spec DependencySpec) (DependencyInstallAction, bool) {
	if !spec.Installable {
		return DependencyInstallAction{}, false
	}
	packageName := strings.TrimSpace(spec.InstallPackage)
	if packageName == "" || strings.ContainsAny(packageName, "\x00\r\n/\\;|&`$()") {
		return DependencyInstallAction{}, false
	}
	action := DependencyInstallAction{DependencyID: spec.ID, Manager: manager}
	switch manager {
	case "brew":
		action.Executable = "brew"
		action.Args = []string{"install"}
		if spec.HomebrewCask {
			action.Args = append(action.Args, "--cask")
		}
		action.Args = append(action.Args, packageName)
	case "scoop":
		if spec.HomebrewCask {
			return DependencyInstallAction{}, false
		}
		action.Executable = "scoop"
		action.Args = []string{"install", packageName}
	case "apt-get", "dnf":
		if spec.HomebrewCask {
			return DependencyInstallAction{}, false
		}
		action.Executable = "sudo"
		action.Args = []string{manager, "install", "-y", packageName}
	default:
		return DependencyInstallAction{}, false
	}
	return action, true
}

func boundedOutput(output []byte, limit int) string {
	value := strings.TrimSpace(string(output))
	value = strings.Join(strings.Fields(value), " ")
	if len(value) > limit {
		return value[:limit] + "..."
	}
	return value
}

type systemDependencyRunner struct{}

// SystemDependencyRunner probes the host without changing it. Installation
// remains an explicit caller action; this runner never invokes a package
// manager.
func SystemDependencyRunner() DependencyRunner { return systemDependencyRunner{} }

func (systemDependencyRunner) LookPath(command string) (string, error) { return exec.LookPath(command) }

func (systemDependencyRunner) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, command, args...).CombinedOutput()
}
