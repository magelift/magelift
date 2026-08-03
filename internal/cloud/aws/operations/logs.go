package operations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
)

const (
	DefaultLogLimit = 100
	MaxLogLimit     = 1000
)

// LogsAPI is the small CloudWatch Logs surface used by the CLI. Keeping the
// SDK boundary here makes account-free tests deterministic.
type LogsAPI interface {
	FilterLogEvents(context.Context, *cloudwatchlogs.FilterLogEventsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error)
}

type Event struct {
	Timestamp  time.Time `json:"timestamp" yaml:"timestamp"`
	IngestedAt time.Time `json:"ingestedAt" yaml:"ingestedAt"`
	Message    string    `json:"message" yaml:"message"`
	LogStream  string    `json:"logStream" yaml:"logStream"`
	EventID    string    `json:"eventId,omitempty" yaml:"eventId,omitempty"`
}

type Store struct {
	client LogsAPI
}

func New(ctx context.Context, region string) (*Store, error) {
	if strings.TrimSpace(region) == "" {
		return nil, errors.New("AWS logs region is required")
	}
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration: %w", err)
	}
	options := []func(*cloudwatchlogs.Options){}
	if endpoint != "" {
		options = append(options, func(options *cloudwatchlogs.Options) {
			options.BaseEndpoint = awssdk.String(endpoint)
		})
	}
	return NewFromClient(cloudwatchlogs.NewFromConfig(cfg, options...))
}

func NewFromClient(client LogsAPI) (*Store, error) {
	if client == nil {
		return nil, errors.New("AWS logs client is required")
	}
	return &Store{client: client}, nil
}

func (s *Store) Tail(ctx context.Context, group string, start time.Time, end *time.Time, filter string, limit int) ([]Event, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("AWS logs client is required")
	}
	group = strings.TrimSpace(group)
	if group == "" {
		return nil, errors.New("log group is required")
	}
	if start.IsZero() {
		return nil, errors.New("log start time is required")
	}
	if limit <= 0 || limit > MaxLogLimit {
		return nil, fmt.Errorf("log limit must be between 1 and %d", MaxLogLimit)
	}
	if end != nil && end.Before(start) {
		return nil, errors.New("log end time must not precede start time")
	}

	input := &cloudwatchlogs.FilterLogEventsInput{
		LogGroupName:  awssdk.String(group),
		StartTime:     awssdk.Int64(start.UnixMilli()),
		StartFromHead: awssdk.Bool(false),
		Limit:         awssdk.Int32(int32(limit)),
	}
	if end != nil {
		input.EndTime = awssdk.Int64(end.UnixMilli())
	}
	if strings.TrimSpace(filter) != "" {
		input.FilterPattern = awssdk.String(filter)
	}

	events := make([]Event, 0, limit)
	var previousToken string
	for len(events) < limit {
		output, err := s.client.FilterLogEvents(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("filter CloudWatch Logs events: %w", err)
		}
		if output == nil {
			return nil, errors.New("filter CloudWatch Logs events returned no output")
		}
		for _, value := range output.Events {
			events = append(events, eventFromSDK(value))
			if len(events) == limit {
				break
			}
		}
		next := strings.TrimSpace(awssdk.ToString(output.NextToken))
		if next == "" || next == previousToken {
			break
		}
		previousToken = next
		input.NextToken = awssdk.String(next)
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].EventID < events[j].EventID
		}
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return events, nil
}

func eventFromSDK(value types.FilteredLogEvent) Event {
	return Event{
		Timestamp:  unixMillis(awssdk.ToInt64(value.Timestamp)),
		IngestedAt: unixMillis(awssdk.ToInt64(value.IngestionTime)),
		Message:    awssdk.ToString(value.Message),
		LogStream:  awssdk.ToString(value.LogStreamName),
		EventID:    awssdk.ToString(value.EventId),
	}
}

func unixMillis(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(value).UTC()
}
