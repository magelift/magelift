package cli

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	"github.com/spf13/cobra"
)

const logReadTimeout = 2 * time.Minute

type logsResult struct {
	Environment string              `json:"environment" yaml:"environment"`
	Group       string              `json:"group" yaml:"group"`
	Status      string              `json:"status" yaml:"status"`
	ObservedAt  time.Time           `json:"observedAt" yaml:"observedAt"`
	Failures    []string            `json:"failures,omitempty" yaml:"failures,omitempty"`
	Events      []platform.LogEvent `json:"events" yaml:"events"`
}

var logServiceName = regexp.MustCompile(`^(web|deploy|cron)$`)

func logsCommand(o *options) *cobra.Command {
	var service, since, until, filter string
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
			var end *time.Time
			if strings.TrimSpace(until) != "" {
				parsedEnd, parseErr := parseLogEnd(until, time.Now().UTC())
				if parseErr != nil {
					return invalid(parseErr)
				}
				end = &parsedEnd
				if end.Before(start) {
					return invalid(errors.New("--until must not precede --since"))
				}
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
			logContext, cancel := context.WithTimeout(cmd.Context(), logReadTimeout)
			defer cancel()
			events, err := observe.TailLogs(logContext, planned, platform.LogQuery{
				Workload: sdk.WorkloadID(service),
				Since:    start,
				Until:    end,
				Filter:   filter,
				Limit:    limit,
			})
			if err != nil {
				if mapped := notSupported(err, planned, "logs"); mapped != err {
					return mapped
				}
				var partial *platform.PartialLogError
				if errors.As(err, &partial) {
					failures := make([]string, 0, len(partial.Failures))
					for _, failure := range partial.Failures {
						failureText := failure.Source
						if failure.Err != nil {
							failureText += ": " + platform.RedactLogMessage(failure.Err.Error())
						}
						failures = append(failures, failureText)
					}
					if writeErr := o.write(logsResult{Environment: planned.Environment(), Group: fmt.Sprintf("/magelift/%s/%s/%s", planned.Project(), planned.Environment(), service), Status: "partial", ObservedAt: time.Now().UTC(), Failures: failures, Events: redactEvents(events)}); writeErr != nil {
						return writeErr
					}
					return &exitError{code: 3, err: errors.New(platform.RedactLogMessage(err.Error()))}
				}
				return &exitError{code: 3, err: errors.New(platform.RedactLogMessage(err.Error()))}
			}
			group := fmt.Sprintf("/magelift/%s/%s/%s", planned.Project(), planned.Environment(), service)
			return o.write(logsResult{Environment: planned.Environment(), Group: group, Status: "complete", ObservedAt: time.Now().UTC(), Events: redactEvents(events)})
		},
	}
	command.Flags().StringVar(&service, "service", "web", "log service: web, deploy, or cron")
	command.Flags().StringVar(&since, "since", "15m", "duration or RFC3339 start time")
	command.Flags().StringVar(&until, "until", "", "optional duration or RFC3339 end time")
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

func parseLogEnd(value string, now time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("--until must not be empty")
	}
	if duration, err := time.ParseDuration(value); err == nil {
		if duration <= 0 {
			return time.Time{}, errors.New("--until duration must be positive")
		}
		return now.Add(-duration), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, errors.New("--until must be a duration or RFC3339 timestamp")
	}
	parsed = parsed.UTC()
	if parsed.After(now) {
		return time.Time{}, errors.New("--until must not be in the future")
	}
	return parsed, nil
}

func redactEvents(events []platform.LogEvent) []platform.LogEvent {
	redacted := make([]platform.LogEvent, len(events))
	copy(redacted, events)
	for i := range redacted {
		redacted[i].Message = platform.RedactLogMessage(redacted[i].Message)
	}
	return redacted
}
