package operations

import (
	"context"
	"errors"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

type fakeLogs struct {
	outputs []*cloudwatchlogs.FilterLogEventsOutput
	inputs  []*cloudwatchlogs.FilterLogEventsInput
	err     error
}

func (f *fakeLogs) FilterLogEvents(_ context.Context, input *cloudwatchlogs.FilterLogEventsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	f.inputs = append(f.inputs, input)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.outputs) == 0 {
		return &cloudwatchlogs.FilterLogEventsOutput{}, nil
	}
	output := f.outputs[0]
	f.outputs = f.outputs[1:]
	return output, nil
}

func TestTailPaginatesAndSortsWithoutExceedingLimit(t *testing.T) {
	client := &fakeLogs{outputs: []*cloudwatchlogs.FilterLogEventsOutput{
		{Events: []types.FilteredLogEvent{
			{Timestamp: awssdk.Int64(2000), IngestionTime: awssdk.Int64(2100), Message: awssdk.String("later"), EventId: awssdk.String("b")},
			{Timestamp: awssdk.Int64(1000), IngestionTime: awssdk.Int64(1100), Message: awssdk.String("first"), EventId: awssdk.String("a")},
		}, NextToken: awssdk.String("next")},
		{Events: []types.FilteredLogEvent{{Timestamp: awssdk.Int64(3000), Message: awssdk.String("ignored")}}},
	}}
	store, err := NewFromClient(client)
	if err != nil {
		t.Fatal(err)
	}
	start := time.UnixMilli(0).UTC()
	events, err := store.Tail(context.Background(), "/magelift/shop/staging/web", start, nil, "ERROR", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Message != "first" || events[1].Message != "later" {
		t.Fatalf("events = %#v", events)
	}
	if len(client.inputs) != 1 || awssdk.ToString(client.inputs[0].LogGroupName) != "/magelift/shop/staging/web" || awssdk.ToString(client.inputs[0].FilterPattern) != "ERROR" {
		t.Fatalf("request = %#v", client.inputs)
	}
}

func TestTailRejectsUnsafeBounds(t *testing.T) {
	store, err := NewFromClient(&fakeLogs{})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(100, 0).UTC()
	for _, test := range []struct {
		name string
		fn   func() error
	}{
		{"group", func() error { _, err := store.Tail(context.Background(), "", start, nil, "", 10); return err }},
		{"limit", func() error {
			_, err := store.Tail(context.Background(), "group", start, nil, "", MaxLogLimit+1)
			return err
		}},
		{"range", func() error {
			end := start.Add(-time.Second)
			_, err := store.Tail(context.Background(), "group", start, &end, "", 10)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.fn(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestTailReturnsClientErrors(t *testing.T) {
	want := errors.New("boom")
	store, err := NewFromClient(&fakeLogs{err: want})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Tail(context.Background(), "group", time.Now().Add(-time.Minute), nil, "", 10)
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}
}
