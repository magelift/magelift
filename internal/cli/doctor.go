package cli

import (
	"errors"
	"fmt"

	"github.com/acourtiol/magelift/internal/config"
	"github.com/spf13/cobra"
)

const doctorExitUnhealthy = 4

type doctorCheck struct {
	ID      string `json:"id" yaml:"id"`
	Status  string `json:"status" yaml:"status"`
	Message string `json:"message" yaml:"message"`
}

type doctorReport struct {
	Config       string        `json:"config" yaml:"config"`
	Status       string        `json:"status" yaml:"status"`
	Environments []string      `json:"environments" yaml:"environments"`
	Checks       []doctorCheck `json:"checks" yaml:"checks"`
}

func doctorCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Check local project readiness", Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error {
		file, err := o.load()
		if err != nil {
			return invalid(err)
		}
		report := inspectProject(o.configPath, file)
		if err := o.write(report); err != nil {
			return err
		}
		if report.Status != "ok" {
			return &exitError{code: doctorExitUnhealthy, err: errors.New("project readiness checks failed")}
		}
		return nil
	}}
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
	return report
}
