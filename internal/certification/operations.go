package certification

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type OperationStatus string

const (
	OperationPending   OperationStatus = "pending"
	OperationRunning   OperationStatus = "running"
	OperationSucceeded OperationStatus = "succeeded"
	OperationFailed    OperationStatus = "failed"
)

// OperationObservation is the provider-neutral result of one poll. Provider
// adapters translate AWS, GCP, Scaleway, or OVH operation states into these
// four values and keep native response shapes out of the core.
type OperationObservation struct {
	Status       OperationStatus `json:"status" yaml:"status"`
	OperationID  string          `json:"operationId" yaml:"operationId"`
	ResourceRefs []string        `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	Detail       string          `json:"detail,omitempty" yaml:"detail,omitempty"`
}

type OperationPoller interface {
	Poll(context.Context, string) (OperationObservation, error)
}

type OperationPolicy struct {
	Timeout      time.Duration `json:"timeout" yaml:"timeout"`
	PollInterval time.Duration `json:"pollInterval" yaml:"pollInterval"`
	MaxAttempts  int           `json:"maxAttempts" yaml:"maxAttempts"`
}

// WaitForOperation polls one provider operation with explicit time and retry
// limits. It never treats an unknown or unfinished state as success.
func WaitForOperation(ctx context.Context, poller OperationPoller, operationID string, policy OperationPolicy) (OperationObservation, error) {
	if ctx == nil {
		return OperationObservation{}, errors.New("operation context is required")
	}
	if poller == nil {
		return OperationObservation{}, errors.New("operation poller is required")
	}
	if strings.TrimSpace(operationID) == "" {
		return OperationObservation{}, errors.New("operation ID is required")
	}
	if policy.Timeout <= 0 || policy.PollInterval <= 0 || policy.MaxAttempts <= 0 {
		return OperationObservation{}, errors.New("operation policy requires positive timeout, poll interval, and max attempts")
	}
	operationCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
	defer cancel()
	deadline := time.Now().Add(policy.Timeout)
	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := operationCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return OperationObservation{}, fmt.Errorf("operation %q timed out after %s", operationID, policy.Timeout)
			}
			return OperationObservation{}, err
		}
		if time.Now().After(deadline) {
			return OperationObservation{}, fmt.Errorf("operation %q timed out after %s", operationID, policy.Timeout)
		}
		observation, err := poller.Poll(operationCtx, operationID)
		if err != nil {
			lastErr = err
		} else {
			if strings.TrimSpace(observation.OperationID) == "" {
				return OperationObservation{}, fmt.Errorf("operation %q returned no operation identity", operationID)
			}
			if observation.OperationID != operationID {
				return OperationObservation{}, fmt.Errorf("operation poll returned identity %q, want %q", observation.OperationID, operationID)
			}
			switch observation.Status {
			case OperationSucceeded:
				return observation, nil
			case OperationFailed:
				if observation.Detail == "" {
					observation.Detail = "provider reported operation failure"
				}
				return observation, fmt.Errorf("operation %q failed: %s", operationID, observation.Detail)
			case OperationPending, OperationRunning:
				lastErr = nil
			default:
				return OperationObservation{}, fmt.Errorf("operation %q returned unknown status %q", operationID, observation.Status)
			}
		}
		if attempt == policy.MaxAttempts {
			if lastErr != nil {
				return OperationObservation{}, fmt.Errorf("poll operation %q after %d attempts: %w", operationID, attempt, lastErr)
			}
			return OperationObservation{}, fmt.Errorf("operation %q did not complete after %d attempts", operationID, attempt)
		}
		wait := policy.PollInterval
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return OperationObservation{}, fmt.Errorf("operation %q timed out after %s", operationID, policy.Timeout)
		}
		if wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-operationCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			if errors.Is(operationCtx.Err(), context.DeadlineExceeded) {
				return OperationObservation{}, fmt.Errorf("operation %q timed out after %s", operationID, policy.Timeout)
			}
			return OperationObservation{}, operationCtx.Err()
		case <-timer.C:
		}
	}
	return OperationObservation{}, fmt.Errorf("operation %q did not complete", operationID)
}
