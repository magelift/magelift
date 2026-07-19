package platform

import (
	"context"
	"time"

	sdk "github.com/acourtiol/magelift/sdk/v1"
)

const (
	DefaultLogLimit = 100
	MaxLogLimit     = 10000
)

// LogEvent is a provider-neutral log line for Magento workloads.
type LogEvent struct {
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`
	Message   string    `json:"message" yaml:"message"`
}

// LogQuery selects Magento workload logs.
type LogQuery struct {
	Workload sdk.WorkloadID
	Since    time.Time
	Until    *time.Time
	Filter   string
	Limit    int
}

// RuntimeHealth summarizes one Magento runtime health probe.
// ID is a stable check identifier for CLI health reports (for example
// runtime.service). Status is healthy, unhealthy, or unavailable.
type RuntimeHealth struct {
	ID      string `json:"id,omitempty" yaml:"id,omitempty"`
	Service string `json:"service" yaml:"service"`
	Status  string `json:"status" yaml:"status"`
	Detail  string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// ExecTarget is an opaque handle the CLI uses to launch a remote Magento shell.
// Launcher and Args are the portable contract; Cluster/Task/Container are
// optional display labels for session-only output.
type ExecTarget struct {
	Launcher  string
	Args      []string
	Cluster   string // optional display
	Task      string // optional display
	Container string // optional display
}

// ExecQuery selects which Magento workload to exec into.
type ExecQuery struct {
	Workload  sdk.WorkloadID
	Container string
	Command   []string
}

// RuntimeObserve is the day-2 Magento runtime port (logs, exec, health).
// Experimental adapters return ErrNotSupported until implemented.
type RuntimeObserve interface {
	TailLogs(ctx context.Context, planned PlannedStack, query LogQuery) ([]LogEvent, error)
	CheckRuntime(ctx context.Context, planned PlannedStack, outputs map[string]any) ([]RuntimeHealth, error)
	PrepareExec(ctx context.Context, planned PlannedStack, outputs map[string]any, query ExecQuery) (ExecTarget, error)
}

// HasRuntimeObserve is implemented by StackModules that expose day-2 runtime ops.
type HasRuntimeObserve interface {
	RuntimeObserve() RuntimeObserve
}

// ModuleRuntimeObserve returns RuntimeObserve when the module implements it.
func ModuleRuntimeObserve(module StackModule) RuntimeObserve {
	if module == nil {
		return nil
	}
	if provider, ok := module.(HasRuntimeObserve); ok {
		return provider.RuntimeObserve()
	}
	return nil
}
