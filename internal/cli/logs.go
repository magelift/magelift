package cli

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/spf13/cobra"
)

type logsResult struct {
	Environment string              `json:"environment" yaml:"environment"`
	Group       string              `json:"group" yaml:"group"`
	Events      []platform.LogEvent `json:"events" yaml:"events"`
}

var logServiceName = regexp.MustCompile(`^(web|deploy|cron)$`)

func logsCommand(o *options) *cobra.Command {
	var service, since, filter string
	var limit int
	command := &cobra.Command{
		Use:   "logs",
		Short: "Read recent Magento application logs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !logServiceName.MatchString(service) {
				return invalid(errors.New("--service must be web, deploy, or cron"))
			}
			start, err := parseLogStart(since, time.Now().UTC())
			if err != nil {
				return invalid(err)
			}
			if limit <= 0 || limit > platform.MaxLogLimit {
				return invalid(fmt.Errorf("--limit must be between 1 and %d", platform.MaxLogLimit))
			}
			_, planned, outputs, err := o.plannedOutputs(cmd.Context())
			if err != nil {
				return err
			}
			observe, err := o.runtimeObserve()
			if err != nil {
				return err
			}
			// Kube factory Observe needs outputs before TailLogs (interface has no outputs arg).
			if binder, ok := observe.(interface{ BindOutputs(map[string]any) error }); ok {
				if bindErr := binder.BindOutputs(outputs); bindErr != nil {
					return &exitError{code: 3, err: bindErr}
				}
			}
			events, err := observe.TailLogs(cmd.Context(), planned, platform.LogQuery{
				Workload: sdk.WorkloadID(service),
				Since:    start,
				Filter:   filter,
				Limit:    limit,
			})
			if err != nil {
				if mapped := notSupported(err, planned, "logs"); mapped != err {
					return mapped
				}
				return &exitError{code: 3, err: err}
			}
			group := fmt.Sprintf("/magelift/%s/%s/%s", planned.Project(), planned.Environment(), service)
			return o.write(logsResult{Environment: planned.Environment(), Group: group, Events: events})
		},
	}
	command.Flags().StringVar(&service, "service", "web", "log service: web, deploy, or cron")
	command.Flags().StringVar(&since, "since", "15m", "duration or RFC3339 start time")
	command.Flags().StringVar(&filter, "filter", "", "provider log filter pattern")
	command.Flags().IntVar(&limit, "limit", platform.DefaultLogLimit, "maximum number of events")
	return command
}

func (o *options) runtimeObserve() (platform.RuntimeObserve, error) {
	if o.testRuntimeObserve != nil {
		return o.testRuntimeObserve, nil
	}
	if o.modules == nil {
		return nil, errors.New("stack module registry is required")
	}
	effective, _, err := o.resolveWithEnvironment()
	if err != nil {
		return nil, invalid(err)
	}
	module, found := o.modules.Module(sdk.ProviderID(effective.Config.Target.Provider), sdk.RuntimeID(effective.Config.Target.Runtime))
	if !found {
		return nil, invalid(fmt.Errorf("no stack module for %s/%s", effective.Config.Target.Provider, effective.Config.Target.Runtime))
	}
	observe := platform.ModuleRuntimeObserve(module)
	if observe == nil {
		return nil, invalid(fmt.Errorf("%s", notSupportedForModule(module, "logs")))
	}
	return observe, nil
}

func parseLogStart(value string, now time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("--since is required")
	}
	if duration, err := time.ParseDuration(value); err == nil {
		if duration <= 0 {
			return time.Time{}, errors.New("--since duration must be positive")
		}
		return now.Add(-duration), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New("--since must be a duration or RFC3339 timestamp")
	}
	parsed = parsed.UTC()
	if parsed.After(now) {
		return time.Time{}, errors.New("--since must not be in the future")
	}
	return parsed, nil
}
