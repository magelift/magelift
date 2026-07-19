package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	awsoperations "github.com/acourtiol/magelift/internal/cloud/aws/operations"
)

type recordingLogsStore struct {
	group  string
	start  time.Time
	filter string
	limit  int
}

func (s *recordingLogsStore) Tail(_ context.Context, group string, start time.Time, _ *time.Time, filter string, limit int) ([]awsoperations.Event, error) {
	s.group, s.start, s.filter, s.limit = group, start, filter, limit
	return []awsoperations.Event{{Timestamp: time.Date(2026, time.July, 18, 2, 0, 0, 0, time.UTC), Message: "healthy"}}, nil
}

func TestLogsCommandUsesResolvedEnvironmentAndStructuredOutput(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	store := &recordingLogsStore{}
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.newLogs = func(context.Context, string) (logsStore, error) { return store, nil }
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", configPath, "--env", "staging", "--output", "json", "logs", "--service", "deploy", "--since", "30m", "--filter", "ERROR", "--limit", "7"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if store.group != "/magelift/example-shop/staging/deploy" || store.filter != "ERROR" || store.limit != 7 || store.start.IsZero() {
		t.Fatalf("request = %#v", store)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"message": "healthy"`)) || !bytes.Contains(output.Bytes(), []byte(`"group": "/magelift/example-shop/staging/deploy"`)) {
		t.Fatalf("output = %s", output.Bytes())
	}
}

func TestParseLogStartRejectsFutureAndInvalidValues(t *testing.T) {
	now := time.Date(2026, time.July, 18, 2, 0, 0, 0, time.UTC)
	for _, value := range []string{"0s", "tomorrow", "2026-07-18T03:00:00Z"} {
		if _, err := parseLogStart(value, now); err == nil {
			t.Fatalf("%q was accepted", value)
		}
	}
	start, err := parseLogStart("30m", now)
	if err != nil || !start.Equal(now.Add(-30*time.Minute)) {
		t.Fatalf("start = %v, err = %v", start, err)
	}
}
