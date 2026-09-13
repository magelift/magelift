package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/localdev"
	"github.com/magelift/magelift/internal/toolchain"
	"github.com/spf13/cobra"
)

const doctorExitUnhealthy = 4

type doctorCheck struct {
	ID          string `json:"id" yaml:"id"`
	Status      string `json:"status" yaml:"status"`
	Message     string `json:"message" yaml:"message"`
	Capability  string `json:"capability,omitempty" yaml:"capability,omitempty"`
	Requirement string `json:"requirement,omitempty" yaml:"requirement,omitempty"`
	Path        string `json:"path,omitempty" yaml:"path,omitempty"`
	Version     string `json:"version,omitempty" yaml:"version,omitempty"`
	InstallHint string `json:"installHint,omitempty" yaml:"installHint,omitempty"`
}

type doctorReport struct {
	Config       string        `json:"config" yaml:"config"`
	Status       string        `json:"status" yaml:"status"`
	Next         string        `json:"next,omitempty" yaml:"next,omitempty"`
	Environments []string      `json:"environments" yaml:"environments"`
	Checks       []doctorCheck `json:"checks" yaml:"checks"`
}

func doctorCommand(o *options) *cobra.Command {
	var installDependencies bool
	command := &cobra.Command{Use: "doctor", Short: "Check this project and print the next Magelift command", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		file, err := o.load()
		if err != nil {
			return invalid(err)
		}
		report := inspectProject(o.configPath, file)
		specs := dependencySpecsForDoctor(file)
		dependencies := toolchain.CheckDependencies(cmd.Context(), dependencyRunner(o), specs)
		if installDependencies {
			if err := installMissingDependencies(cmd.Context(), o, specs, dependencies); err != nil {
				appendDependencyChecks(&report, dependencies)
				setNextFromFirstFailure(&report)
				if writeErr := o.write(report); writeErr != nil {
					return writeErr
				}
				return err
			}
			dependencies = toolchain.CheckDependencies(cmd.Context(), dependencyRunner(o), specs)
		}
		appendDependencyChecks(&report, dependencies)
		setNextFromFirstFailure(&report)
		if err := o.write(report); err != nil {
			return err
		}
		if report.Status != "ok" {
			return &exitError{code: doctorExitUnhealthy, err: errors.New("project readiness checks failed")}
		}
		return nil
	}}
	command.Flags().BoolVar(&installDependencies, "install-dependencies", false, "install missing allowlisted dependencies after confirmation")
	return command
}

func dependencySpecsForDoctor(file *config.File) []toolchain.DependencySpec {
	provider, runtimeID := dependencyTarget(file)
	return toolchain.SpecsForTarget(provider, runtimeID)
}

func inspectProject(path string, file *config.File) doctorReport {
	environments := file.Environments()
	report := doctorReport{Config: path, Status: "ok", Environments: environments, Checks: []doctorCheck{}}
	add := func(id string, err error) {
		check := doctorCheck{ID: id, Status: "ok", Message: "valid"}
		if err != nil {
			check.Status, check.Message, report.Status = "failed", err.Error(), "failed"
		}
		report.Checks = append(report.Checks, check)
	}
	_, err := file.ResolveBuild()
	add("build", err)
	if len(environments) == 0 {
		add("environments", errors.New("at least one environment is required"))
	}
	for _, environment := range environments {
		_, err := file.Resolve(environment, config.ResolveOptions{})
		if err != nil {
			err = fmt.Errorf("environment %s: %w", environment, err)
		}
		add("environment."+environment, err)
	}
	if report.Status == "ok" && len(environments) > 0 {
		env := environments[0]
		report.Next = "magelift bootstrap --env " + env
	}
	report.Checks = append(report.Checks, doctorCheck{
		ID:      "local.edge",
		Status:  "ok",
		Message: localdev.CloudOnlyNote,
	})
	return report
}

// setNextFromFirstFailure points Next at the fix for the first failed
// check. Config families land on validate; missing dependencies land on
// the installer flag. Unknown check IDs default to validate so the map
// cannot go stale silently as checks grow. A healthy report keeps the
// bootstrap Next set by inspectProject.
func setNextFromFirstFailure(report *doctorReport) {
	if report == nil || report.Status == "ok" {
		return
	}
	for _, check := range report.Checks {
		if check.Status == "ok" {
			continue
		}
		if strings.HasPrefix(check.ID, "dependency.") {
			report.Next = "magelift doctor --install-dependencies"
			return
		}
		report.Next = "magelift config validate"
		return
	}
}
