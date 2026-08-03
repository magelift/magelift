package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/health"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/spf13/cobra"
)

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
		report.Checks = o.configHealthChecks(effective.Config)
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
		report.Checks = o.runtimeHealthEvidence(ctx)
	default:
		return health.Report{}, invalid(fmt.Errorf("unsupported health mode %q", mode))
	}
	report.Status = health.Summarize(report.Checks)
	return report, nil
}

func (o *options) runtimeHealthEvidence(ctx context.Context) []health.Check {
	unavailable := []health.Check{{ID: "runtime.observe", Status: health.StatusUnavailable, Message: "runtime health evidence is unavailable"}}
	observe, err := o.runtimeObserve()
	if err != nil {
		return unavailable
	}
	if o.newBackend == nil {
		return unavailable
	}
	_, planned, outputs, err := o.plannedOutputs(ctx)
	if err != nil {
		return unavailable
	}
	items, err := observe.CheckRuntime(ctx, planned, outputs)
	if err != nil {
		if errors.Is(err, platform.ErrNotSupported) {
			return []health.Check{{ID: "runtime.observe", Status: health.StatusUnavailable, Message: notSupportedForPlanned(planned, "runtime health")}}
		}
		return unavailable
	}
	return runtimeHealthChecks(items)
}

func runtimeHealthChecks(items []platform.RuntimeHealth) []health.Check {
	checks := make([]health.Check, 0, len(items))
	for _, item := range items {
		id := item.ID
		if id == "" {
			id = "runtime." + item.Service
		}
		status := health.StatusUnavailable
		switch strings.ToLower(item.Status) {
		case "healthy":
			status = health.StatusHealthy
		case "unhealthy":
			status = health.StatusUnhealthy
		}
		message := item.Detail
		if message == "" {
			message = item.Service + " is " + item.Status
		}
		checks = append(checks, health.Check{ID: id, Status: status, Message: message})
	}
	if len(checks) == 0 {
		return []health.Check{{ID: "runtime.observe", Status: health.StatusUnavailable, Message: "runtime health returned no probes"}}
	}
	return checks
}

func (o *options) configHealthChecks(cfg config.Config) []health.Check {
	checks := []health.Check{{ID: "config.resolved", Status: health.StatusHealthy, Message: "configuration resolved and passed schema and compatibility validation"}}
	status, message := health.StatusHealthy, "registered Magento target is selected"
	if o.modules != nil {
		if _, found := o.modules.Module(sdk.ProviderID(cfg.Target.Provider), sdk.RuntimeID(cfg.Target.Runtime)); !found {
			status, message = health.StatusUnhealthy, "the selected target is not registered in this binary"
		}
	} else if cfg.Target.Provider != "aws" || cfg.Target.Runtime != "ecs-fargate" {
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
