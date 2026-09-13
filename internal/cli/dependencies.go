package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/toolchain"
)

func dependencyRunner(o *options) toolchain.DependencyRunner {
	if o != nil && o.dependencyRunner != nil {
		return o.dependencyRunner
	}
	return toolchain.SystemDependencyRunner()
}

func dependencyTarget(file *config.File) (string, string) {
	if file == nil {
		return "", ""
	}
	for _, environment := range file.Environments() {
		effective, err := file.Resolve(environment, config.ResolveOptions{})
		if err == nil {
			return effective.Config.Target.Provider, effective.Config.Target.Runtime
		}
	}
	return "", ""
}

func appendDependencyChecks(report *doctorReport, dependencies toolchain.DependencyReport) {
	for _, dependency := range dependencies.Checks {
		check := doctorCheck{
			ID:          "dependency." + dependency.ID,
			Status:      string(dependency.Status),
			Message:     dependency.Message,
			Capability:  dependency.Capability,
			Requirement: string(dependency.Requirement),
			Path:        dependency.Path,
			Version:     dependency.Version,
			InstallHint: dependency.InstallHint,
		}
		if dependency.Status == toolchain.DependencyAvailable {
			check.Status = "ok"
		}
		if dependency.Requirement == toolchain.DependencyRequired && dependency.Status != toolchain.DependencyAvailable {
			report.Status = "failed"
		}
		report.Checks = append(report.Checks, check)
	}
}

func requireDependencies(ctx context.Context, o *options, specs []toolchain.DependencySpec, capability string) error {
	if len(specs) == 0 {
		return &exitError{code: 3, err: fmt.Errorf("%s has no valid dependency specification", capability)}
	}
	requiredSpecs := make([]toolchain.DependencySpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Requirement == toolchain.DependencyRequired {
			requiredSpecs = append(requiredSpecs, spec)
		}
	}
	if len(requiredSpecs) == 0 {
		return nil
	}
	report := toolchain.CheckDependencies(ctx, dependencyRunner(o), requiredSpecs)
	missing := make([]string, 0)
	for _, check := range report.Checks {
		if check.Requirement != toolchain.DependencyRequired || check.Status == toolchain.DependencyAvailable {
			continue
		}
		message := check.ID + ": " + check.Message
		if check.InstallHint != "" {
			message += " (install: " + check.InstallHint + ")"
		}
		missing = append(missing, message)
	}
	if len(missing) == 0 {
		return nil
	}
	return &exitError{code: 3, err: fmt.Errorf("%s is unavailable: %s", capability, strings.Join(missing, "; "))}
}

func requireLocalDependencies(ctx context.Context, o *options) error {
	return requireDependencies(ctx, o, toolchain.SpecsForLocal(), "local Docker runtime")
}

func requireTargetDependencies(ctx context.Context, o *options, planned platform.PlannedStack) error {
	return requireDependencies(ctx, o, toolchain.SpecsForTarget(string(planned.Provider()), string(planned.Runtime())), "cloud operation")
}

func requireCosignVerificationDependencies(ctx context.Context, o *options) error {
	return requireDependencies(ctx, o, toolchain.SpecsForCosignVerification(), "signed release verification")
}

func (o *options) confirm(prompt string) (bool, error) {
	if o != nil && o.yes {
		return true, nil
	}
	if o != nil && o.noInteraction {
		return false, errors.New("dependency installation requires --yes when --no-interaction is set")
	}
	if o != nil {
		if terminal, ok := o.terminal.(interface{ Confirm(string) (bool, error) }); ok {
			return terminal.Confirm(prompt)
		}
	}
	return false, errors.New("dependency installation requires --yes outside an interactive terminal")
}

func installMissingDependencies(ctx context.Context, o *options, specs []toolchain.DependencySpec, report toolchain.DependencyReport) error {
	actions, err := toolchain.PlanDependencyInstalls(dependencyRunner(o), specs, report)
	if err != nil {
		return invalid(err)
	}
	if len(actions) == 0 {
		return nil
	}
	confirmed, err := o.confirm(fmt.Sprintf("Install %d missing dependency package(s)?", len(actions)))
	if err != nil {
		return invalid(err)
	}
	if !confirmed {
		return invalid(errors.New("dependency installation cancelled"))
	}
	if err := toolchain.InstallDependencies(ctx, dependencyRunner(o), actions); err != nil {
		return &exitError{code: 3, err: err}
	}
	return nil
}
