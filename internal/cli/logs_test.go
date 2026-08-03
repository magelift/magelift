package cli

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type recordingObserve struct {
	query platform.LogQuery
}

func (r *recordingObserve) TailLogs(_ context.Context, planned platform.PlannedStack, query platform.LogQuery) ([]platform.LogEvent, error) {
	r.query = query
	_ = planned
	return []platform.LogEvent{{Timestamp: time.Date(2026, time.July, 18, 2, 0, 0, 0, time.UTC), Message: "healthy"}}, nil
}
func (r *recordingObserve) CheckRuntime(context.Context, platform.PlannedStack, map[string]any) ([]platform.RuntimeHealth, error) {
	return nil, platform.ErrNotSupported
}
func (r *recordingObserve) PrepareExec(context.Context, platform.PlannedStack, map[string]any, platform.ExecQuery) (platform.ExecTarget, error) {
	return platform.ExecTarget{}, platform.ErrNotSupported
}

func TestLogsCommandUsesResolvedEnvironmentAndStructuredOutput(t *testing.T) {
	configPath := writeLifecycleConfig(t, "staging", false)
	var output bytes.Buffer
	observe := &recordingObserve{}
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.newBackend = func(_ context.Context, _ platform.PlannedStack, _ string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: map[string]any{"clusterName": "shop-cluster"}}, nil
	}
	o.testRuntimeObserve = observe
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", configPath, "--env", "staging", "--output", "json", "logs", "--service", "deploy", "--since", "30m", "--filter", "ERROR", "--limit", "7"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if observe.query.Workload != sdk.WorkloadID("deploy") || observe.query.Filter != "ERROR" || observe.query.Limit != 7 || observe.query.Since.IsZero() {
		t.Fatalf("query = %#v", observe.query)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"message": "healthy"`)) || !bytes.Contains(output.Bytes(), []byte(`"group": "/magelift/shop/staging/deploy"`)) {
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
