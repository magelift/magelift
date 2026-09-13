package cli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/health"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
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
			case health.StatusUnhealthy, health.StatusDegraded:
				return &exitError{code: healthExitUnhealthy, err: errors.New("health checks failed")}
			case health.StatusStale:
				return &exitError{code: healthExitUnavailable, err: errors.New("health evidence is stale")}
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
	observedAt := time.Now().UTC()
	report := health.Report{Environment: environment, Target: target, Mode: mode, Checks: []health.Check{}, ObservedAt: observedAt, Source: mode, Freshness: health.FreshnessCurrent}
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
		if err := requireTargetDependencies(ctx, o, planned); err != nil {
			return health.Report{}, err
		}
		backend, err := o.newBackend(ctx, planned, o.infrastructureBackendURL(planned))
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
	for i := range report.Checks {
		report.Checks[i] = health.AnnotateLayer(report.Checks[i])
		if report.Checks[i].Source == "" {
			report.Checks[i].Source = mode
		}
		if report.Checks[i].ObservedAt.IsZero() {
			report.Checks[i].ObservedAt = observedAt
		}
		if report.Checks[i].Freshness == "" {
			report.Checks[i].Freshness = health.FreshnessCurrent
		}
	}
	report.Status = health.Summarize(report.Checks)
	switch report.Status {
	case health.StatusStale:
		report.Freshness = health.FreshnessStale
	case health.StatusUnavailable:
		report.Freshness = health.FreshnessUnknown
	}
	return report, nil
}

func (o *options) runtimeHealthEvidence(ctx context.Context) []health.Check {
	unavailable := []health.Check{health.AnnotateLayer(health.Check{ID: "runtime.observe", Status: health.StatusUnavailable, Message: "runtime health evidence is unavailable"})}
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
			return []health.Check{health.AnnotateLayer(health.Check{ID: "runtime.observe", Status: health.StatusUnavailable, Message: notSupportedForPlanned(planned, "runtime health")})}
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
		case "degraded":
			status = health.StatusDegraded
		case "stale":
			status = health.StatusStale
		}
		message := item.Detail
		if message == "" {
			message = item.Service + " is " + item.Status
		}
		check := health.Check{ID: id, Layer: health.Layer(item.Layer), Status: status, Message: message}
		checks = append(checks, health.AnnotateLayer(check))
	}
	if len(checks) == 0 {
		return []health.Check{health.AnnotateLayer(health.Check{ID: "runtime.observe", Status: health.StatusUnavailable, Message: "runtime health returned no probes"})}
	}
	return checks
}

func (o *options) configHealthChecks(cfg config.Config) []health.Check {
	checks := []health.Check{health.AnnotateLayer(health.Check{ID: "config.resolved", Status: health.StatusHealthy, Message: "configuration resolved and passed schema and compatibility validation"})}
	status, message := health.StatusHealthy, "registered Magento target is selected"
	if o.modules != nil {
		if _, found := o.modules.Module(sdk.ProviderID(cfg.Target.Provider), sdk.RuntimeID(cfg.Target.Runtime)); !found {
			status, message = health.StatusUnhealthy, "the selected target is not registered in this binary"
		}
	} else if cfg.Target.Provider != "aws" || cfg.Target.Runtime != "ecs-fargate" {
		status, message = health.StatusUnhealthy, "the selected target is not supported by this release"
	}
	return append(checks, health.AnnotateLayer(health.Check{ID: "target.supported", Status: status, Message: message}))
}

func outputHealthChecks(outputs map[string]any) []health.Check {
	checks := make([]health.Check, 0, 2)
	for _, key := range []string{"applicationURL", "mediaURL"} {
		value, ok := outputs[key].(string)
		parsed, err := url.ParseRequestURI(value)
		if !ok || err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			checks = append(checks, health.AnnotateLayer(health.Check{ID: "output." + key, Status: health.StatusUnhealthy, Message: key + " is missing or is not an HTTPS URL"}))
			continue
		}
		checks = append(checks, health.AnnotateLayer(health.Check{ID: "output." + key, Status: health.StatusHealthy, Message: key + " is present and structurally valid"}))
	}
	return checks
}
