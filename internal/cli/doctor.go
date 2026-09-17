package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		for _, check := range credentialChecks(cmd.Context(), file, dependencyRunner(o), o.getenv) {
			report.Checks = append(report.Checks, check)
			if check.Status == "failed" {
				report.Status = "failed"
			}
		}
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
// the installer flag; credential failures name the login command (the one
// Next that is not a magelift invocation). Unknown check IDs default to
// validate so the map cannot go stale silently as checks grow. A healthy
// report keeps the bootstrap Next set by inspectProject.
func setNextFromFirstFailure(report *doctorReport) {
	if report == nil || report.Status == "ok" {
		return
	}
	for _, check := range report.Checks {
		if check.Status == "ok" || check.Status == "skipped" {
			continue
		}
		if strings.HasPrefix(check.ID, "dependency.") {
			report.Next = "magelift doctor --install-dependencies"
			return
		}
		if strings.HasPrefix(check.ID, "credentials.") {
			report.Next = "gcloud auth application-default login"
			return
		}
		report.Next = "magelift config validate"
		return
	}
}

// credentialChecks verifies the cloud credentials deploys actually use.
// GCP deploys consume Application Default Credentials, which `gcloud auth
// login` alone does not provide; the check mints a token when gcloud is
// present so the failure names the real fix. CI runners (workload
// identity, no ADC file) skip explicitly instead of failing spuriously.
func credentialChecks(ctx context.Context, file *config.File, runner toolchain.DependencyRunner, getenv func(string) string) []doctorCheck {
	provider, _ := dependencyTarget(file)
	if provider != "gcp" {
		return nil
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	if strings.TrimSpace(getenv("CI")) != "" {
		return []doctorCheck{{ID: "credentials.gcp", Status: "skipped", Message: "CI runner: workload identity supplies credentials"}}
	}
	if runner == nil {
		return []doctorCheck{{ID: "credentials.gcp", Status: "failed", Message: "credential probe is not configured"}}
	}
	if _, err := runner.LookPath("gcloud"); err == nil {
		probe, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if _, err := runner.Run(probe, "gcloud", "auth", "application-default", "print-access-token"); err != nil {
			return []doctorCheck{{
				ID: "credentials.gcp", Status: "failed",
				Message:     "ADC cannot mint a token; gcloud auth login alone is not enough",
				InstallHint: "gcloud auth application-default login",
			}}
		}
		return []doctorCheck{{ID: "credentials.gcp", Status: "ok", Message: "ADC mints tokens"}}
	}
	if path := adcFilePresent(getenv); path != "" {
		return []doctorCheck{{ID: "credentials.gcp", Status: "ok", Message: "ADC file present (gcloud unavailable to verify)", Path: path}}
	}
	return []doctorCheck{{
		ID: "credentials.gcp", Status: "failed",
		Message:     "no gcloud and no ADC file; deploys cannot authenticate",
		InstallHint: "install gcloud, then run: gcloud auth application-default login",
	}}
}

// adcFilePresent returns the ADC file path when one exists, else "".
func adcFilePresent(getenv func(string) string) string {
	if explicit := strings.TrimSpace(getenv("GOOGLE_APPLICATION_CREDENTIALS")); explicit != "" {
		if info, err := os.Stat(explicit); err == nil && !info.IsDir() {
			return explicit
		}
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	candidate := filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
	if info, err := os.Stat(candidate); err != nil || info.IsDir() {
		return ""
	}
	return candidate
}
