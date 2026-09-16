//go:build floci_gcp

package flocigcp_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	logging "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/apiv2/loggingpb"
	"google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestLogWriteAndListAgainstFlociGCP(t *testing.T) {
	host := requireFlociGCP(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := logging.NewClient(ctx, flociGCPClientOptions(host)...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	const project = "floci-local"
	logName := "projects/" + project + "/logs/magelift-floci-gcp"
	payload := fmt.Sprintf("floci-gcp-log-%d", time.Now().UnixNano())
	if _, err := client.WriteLogEntries(ctx, &loggingpb.WriteLogEntriesRequest{
		Entries: []*loggingpb.LogEntry{{
			LogName:   logName,
			Resource:  &monitoredres.MonitoredResource{Type: "global"},
			Timestamp: timestamppb.Now(),
			Payload:   &loggingpb.LogEntry_TextPayload{TextPayload: payload},
		}},
	}); err != nil {
		t.Fatalf("write log entries: %v", err)
	}

	it := client.ListLogEntries(ctx, &loggingpb.ListLogEntriesRequest{
		ResourceNames: []string{"projects/" + project},
		Filter:        fmt.Sprintf(`logName="%s"`, logName),
	})
	entry, err := it.Next()
	if err != nil {
		t.Fatalf("list log entries: %v", err)
	}
	if entry.GetTextPayload() != payload {
		t.Fatalf("got %q, want %q", entry.GetTextPayload(), payload)
	}
}
