package platform

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/sdk"
)

const (
	DefaultLogLimit = 100
	MaxLogLimit     = 10000
)

// LogEvent is a provider-neutral log line for Magento workloads.
type LogEvent struct {
	Timestamp  time.Time `json:"timestamp" yaml:"timestamp"`
	Message    string    `json:"message" yaml:"message"`
	Workload   string    `json:"workload,omitempty" yaml:"workload,omitempty"`
	Source     string    `json:"source,omitempty" yaml:"source,omitempty"`
	IngestedAt time.Time `json:"ingestedAt,omitempty" yaml:"ingestedAt,omitempty"`
	EventID    string    `json:"eventId,omitempty" yaml:"eventId,omitempty"`
}

// LogReadFailure identifies a source that could not be read during a
// multi-source log query.
type LogReadFailure struct {
	Source string
	Err    error
}

// PartialLogError preserves successfully read events while making an
// incomplete multi-source query visible to callers.
type PartialLogError struct {
	Failures []LogReadFailure
}

func (e *PartialLogError) Error() string {
	if e == nil || len(e.Failures) == 0 {
		return "log query returned a partial result"
	}
	parts := make([]string, 0, len(e.Failures))
	for _, failure := range e.Failures {
		if failure.Err == nil {
			parts = append(parts, failure.Source)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %v", failure.Source, failure.Err))
	}
	return "log query returned a partial result (" + strings.Join(parts, "; ") + ")"
}

func (e *PartialLogError) Unwrap() error {
	if e == nil || len(e.Failures) == 0 {
		return nil
	}
	return e.Failures[0].Err
}

var (
	logSensitiveValue = regexp.MustCompile(`(?i)(password|passwd|secret|token|authorization|api[_-]?key|private[_-]?key)(\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;}\]]+)`)
	logBearerValue    = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
)

// RedactLogMessage removes common credential forms before a provider log line
// crosses the platform boundary. It is intentionally conservative: unknown
// application data is preserved rather than treated as a credential.
func RedactLogMessage(message string) string {
	message = logBearerValue.ReplaceAllString(message, "Bearer [redacted]")
	return logSensitiveValue.ReplaceAllString(message, "$1$2[redacted]")
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
	Layer   string `json:"layer,omitempty" yaml:"layer,omitempty"`
	Service string `json:"service" yaml:"service"`
	Status  string `json:"status" yaml:"status"`
	Detail  string `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// ExecTarget is an opaque handle the CLI uses to launch a remote Magento shell.
// Launcher and Args are the portable contract; Cluster/Task/Container are
// optional display labels for session-only output.
// CleanupPaths are temp files the CLI must remove after the remote command ends
// (success or failure); e.g. kubeconfig written for kubectl --kubeconfig.
type ExecTarget struct {
	Launcher     string
	Args         []string
	Cluster      string   // optional display
	Task         string   // optional display
	Container    string   // optional display
	CleanupPaths []string // optional temp files owned by PrepareExec
}

// ExecQuery selects which Magento workload to exec into.
type ExecQuery struct {
	Workload     sdk.WorkloadID
	Container    string
	Command      []string
	WaitForReady bool
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

// ValidateFirstPartyObservability prevents a first-party stack from silently
// ignoring a telemetry provider that it cannot configure. Native destinations
// remain provider-specific, while New Relic and other external destinations
// belong in an extension that consumes the same SDK intent.
func ValidateFirstPartyObservability(cfg config.Config) error {
	observability := cfg.Observability
	provider := strings.TrimSpace(observability.NativeProvider)
	if provider == "" || provider == "none" {
		return nil
	}
	switch provider {
	case "cloudwatch":
		if cfg.Target.Provider != "aws" {
			return fmt.Errorf("observability provider %q requires target.provider aws", provider)
		}
	case "google-cloud-operations":
		if cfg.Target.Provider != "gcp" {
			return fmt.Errorf("observability provider %q requires target.provider gcp", provider)
		}
	case "scaleway-cockpit":
		if cfg.Target.Provider != "scaleway" {
			return fmt.Errorf("observability provider %q requires target.provider scaleway", provider)
		}
	case "ovh-logs-data-platform":
		if cfg.Target.Provider != "ovh" {
			return fmt.Errorf("observability provider %q requires target.provider ovh", provider)
		}
	case "xray", "aws-xray":
		return sdk.ObservabilityCapabilityError{
			Signal: "traces",
			Reason: "X-Ray Magento tracing requires an ObservabilityAdapter plugin; the EKS IAM policy snippet is not a Magento cell",
		}
	default:
		return fmt.Errorf("native observability provider %q is not registered for the first-party stack", provider)
	}
	return nil
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
