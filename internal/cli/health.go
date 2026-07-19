package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	awsoperations "github.com/acourtiol/magelift/internal/cloud/aws/operations"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/health"
	"github.com/spf13/cobra"
)

type runtimeStore interface {
	Check(context.Context, string, string) (awsoperations.ServiceHealth, error)
}

const (
	healthExitUnhealthy   = 4
	healthExitUnavailable = 3
)

func healthCommand(o *options) *cobra.Command {
	mode := "config"
	command := &cobra.Command{
		Use:   "health",
		Short: "Check configuration or deployed stack health evidence",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report, err := o.runHealth(cmd.Context(), mode)
			if err != nil {
				return err
			}
			if err := o.write(report); err != nil {
				return err
			}
			switch report.Status {
			case health.StatusHealthy:
				return nil
			case health.StatusUnhealthy:
				return &exitError{code: healthExitUnhealthy, err: errors.New("health checks failed")}
			default:
				return &exitError{code: healthExitUnavailable, err: errors.New("runtime health evidence is unavailable")}
			}
		},
	}
	command.Flags().StringVar(&mode, "mode", "config", "health evidence source: config, outputs, or runtime")
	return command
}

func (o *options) runHealth(ctx context.Context, mode string) (health.Report, error) {
	effective, environment, err := o.resolveWithEnvironment()
	if err != nil {
		return health.Report{}, invalid(err)
	}
	target := effective.Config.Target.Provider + "." + effective.Config.Target.Runtime
	report := health.Report{Environment: environment, Target: target, Mode: mode, Checks: []health.Check{}}
	switch mode {
	case "config":
		report.Checks = configHealthChecks(effective.Config)
	case "outputs":
		_, planned, err := o.planStack(false)
		if err != nil {
			return health.Report{}, invalid(err)
		}
		if o.newBackend == nil {
			return health.Report{}, errors.New("infrastructure backend factory is required")
		}
		backend, err := o.newBackend(ctx, planned, strings.TrimSpace(o.getenv("PULUMI_BACKEND_URL")))
		if err != nil {
			return health.Report{}, fmt.Errorf("create infrastructure backend: %w", err)
		}
		outputs, err := backend.Outputs(ctx)
		if err != nil {
			return health.Report{}, fmt.Errorf("read infrastructure outputs: %w", err)
		}
		report.Checks = outputHealthChecks(outputs)
	case "runtime":
		if o.newRuntime == nil {
			report.Checks = []health.Check{{ID: "runtime.ecs", Status: health.StatusUnavailable, Message: "ECS runtime health adapter is not configured"}}
			break
		}
		_, spec, outputs, err := o.runtimeOutputs(ctx)
		if err != nil {
			return health.Report{}, err
		}
		cluster := outputString(outputs, "clusterName")
		service := outputString(outputs, "serviceName")
		if cluster == "" || service == "" {
			report.Checks = []health.Check{{ID: "runtime.ecs", Status: health.StatusUnavailable, Message: "stack outputs do not contain ECS cluster and service identifiers"}}
			break
		}
		store, err := o.newRuntime(ctx, spec.Identity.Region)
		if err != nil {
			return health.Report{}, &exitError{code: healthExitUnavailable, err: fmt.Errorf("create ECS runtime health client: %w", err)}
		}
		healthResult, err := store.Check(ctx, cluster, service)
		if err != nil {
			return health.Report{}, &exitError{code: healthExitUnavailable, err: err}
		}
		report.Checks = runtimeHealthChecks(healthResult)
	default:
		return health.Report{}, invalid(fmt.Errorf("unsupported health mode %q", mode))
	}
	report.Status = health.Summarize(report.Checks)
	return report, nil
}

func runtimeHealthChecks(result awsoperations.ServiceHealth) []health.Check {
	checks := make([]health.Check, 0, 3+len(result.Tasks))
	serviceStatus := health.StatusHealthy
	serviceMessage := fmt.Sprintf("ECS service has %d running of %d desired tasks", result.RunningCount, result.DesiredCount)
	if result.DesiredCount <= 0 || result.RunningCount < result.DesiredCount {
		serviceStatus = health.StatusUnhealthy
		serviceMessage = fmt.Sprintf("ECS service has %d running of %d desired tasks", result.RunningCount, result.DesiredCount)
	}
	checks = append(checks, health.Check{ID: "runtime.ecs.service", Status: serviceStatus, Message: serviceMessage})
	rolloutStatus := health.StatusHealthy
	rolloutMessage := "the primary ECS deployment rollout completed"
	if result.PrimaryRollout != "COMPLETED" {
		rolloutStatus = health.StatusUnhealthy
		rolloutMessage = "the primary ECS deployment rollout has not completed"
	}
	checks = append(checks, health.Check{ID: "runtime.ecs.rollout", Status: rolloutStatus, Message: rolloutMessage})
	for _, task := range result.Tasks {
		status := health.StatusHealthy
		message := "ECS task is running and healthy"
		// ECS reports UNKNOWN when the task definition has no container health
		// check; that is not a failure. Only UNHEALTHY (or non-RUNNING) is.
		switch {
		case task.LastStatus != "RUNNING" || task.HealthStatus == "UNHEALTHY":
			status = health.StatusUnhealthy
			message = fmt.Sprintf("ECS task status is %s/%s", task.LastStatus, task.HealthStatus)
		case task.HealthStatus == "" || task.HealthStatus == "UNKNOWN":
			message = "ECS task is running (no container health check configured)"
		}
		checks = append(checks, health.Check{ID: "runtime.ecs.task." + task.ARN, Status: status, Message: message})
	}
	return checks
}

func configHealthChecks(cfg config.Config) []health.Check {
	checks := []health.Check{{ID: "config.resolved", Status: health.StatusHealthy, Message: "configuration resolved and passed schema and compatibility validation"}}
	status, message := health.StatusHealthy, "AWS ECS Fargate target is selected"
	if cfg.Target.Provider != "aws" || cfg.Target.Runtime != "ecs-fargate" {
		status, message = health.StatusUnhealthy, "the selected target is not supported by this release"
	}
	return append(checks, health.Check{ID: "target.supported", Status: status, Message: message})
}

func outputHealthChecks(outputs map[string]any) []health.Check {
	checks := make([]health.Check, 0, 2)
	for _, key := range []string{"applicationURL", "mediaURL"} {
		value, ok := outputs[key].(string)
		parsed, err := url.ParseRequestURI(value)
		if !ok || err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			checks = append(checks, health.Check{ID: "output." + key, Status: health.StatusUnhealthy, Message: key + " is missing or is not an HTTPS URL"})
			continue
		}
		checks = append(checks, health.Check{ID: "output." + key, Status: health.StatusHealthy, Message: key + " is present and structurally valid"})
	}
	return checks
}
