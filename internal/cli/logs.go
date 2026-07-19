package cli

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	awsoperations "github.com/acourtiol/magelift/internal/cloud/aws/operations"
	"github.com/spf13/cobra"
)

type logsStore interface {
	Tail(context.Context, string, time.Time, *time.Time, string, int) ([]awsoperations.Event, error)
}

type logsResult struct {
	Environment string                `json:"environment" yaml:"environment"`
	Group       string                `json:"group" yaml:"group"`
	Events      []awsoperations.Event `json:"events" yaml:"events"`
}

var logServiceName = regexp.MustCompile(`^(web|deploy|cron)$`)

func logsCommand(o *options) *cobra.Command {
	var service, since, filter string
	var limit int
	command := &cobra.Command{
		Use:   "logs",
		Short: "Read recent ECS application logs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			effective, environment, err := o.resolveWithEnvironment()
			if err != nil {
				return invalid(err)
			}
			if effective.Config.Target.Provider != "aws" {
				return invalid(errors.New("logs currently supports the AWS target only"))
			}
			if !logServiceName.MatchString(service) {
				return invalid(errors.New("--service must be web, deploy, or cron"))
			}
			start, err := parseLogStart(since, time.Now().UTC())
			if err != nil {
				return invalid(err)
			}
			if limit <= 0 || limit > awsoperations.MaxLogLimit {
				return invalid(fmt.Errorf("--limit must be between 1 and %d", awsoperations.MaxLogLimit))
			}
			group := fmt.Sprintf("/magelift/%s/%s/%s", effective.Config.Project.Name, environment, service)
			factory := o.newLogs
			if factory == nil {
				return &exitError{code: 3, err: errors.New("AWS logs client factory is required")}
			}
			store, err := factory(cmd.Context(), effective.Config.Defaults.Region)
			if err != nil {
				return &exitError{code: 3, err: fmt.Errorf("initialize AWS logs client: %w", err)}
			}
			events, err := store.Tail(cmd.Context(), group, start, nil, filter, limit)
			if err != nil {
				return &exitError{code: 3, err: err}
			}
			return o.write(logsResult{Environment: environment, Group: group, Events: events})
		},
	}
	command.Flags().StringVar(&service, "service", "web", "log service: web, deploy, or cron")
	command.Flags().StringVar(&since, "since", "15m", "duration or RFC3339 start time")
	command.Flags().StringVar(&filter, "filter", "", "CloudWatch Logs filter pattern")
	command.Flags().IntVar(&limit, "limit", awsoperations.DefaultLogLimit, "maximum number of events")
	return command
}

func parseLogStart(value string, now time.Time) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, errors.New("--since is required")
	}
	if duration, err := time.ParseDuration(value); err == nil {
		if duration <= 0 {
			return time.Time{}, errors.New("--since duration must be greater than zero")
		}
		return now.Add(-duration), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("--since must be a positive duration or RFC3339 timestamp: %w", err)
	}
	if parsed.After(now) {
		return time.Time{}, errors.New("--since timestamp must not be in the future")
	}
	return parsed.UTC(), nil
}
