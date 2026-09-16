package sdk

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DeletionObservation is the normalized result of an owning-service
// inventory poll. Delayed and protected identities are retained explicitly;
// their presence can never be treated as successful cleanup.
type DeletionObservation struct {
	Status                ResilienceOperationStatus `json:"status" yaml:"status"`
	OperationID           string                    `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	ResourceRefs          []string                  `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	DelayedResourceRefs   []string                  `json:"delayedResourceRefs,omitempty" yaml:"delayedResourceRefs,omitempty"`
	ProtectedResourceRefs []string                  `json:"protectedResourceRefs,omitempty" yaml:"protectedResourceRefs,omitempty"`
	Detail                string                    `json:"detail,omitempty" yaml:"detail,omitempty"`
}

// DeletionClient is implemented by a cloud or SaaS adapter. PollDeletion must
// query the service that owns the resource, not a local state cache or an
// eventually consistent secondary index.
type DeletionClient interface {
	PollDeletion(context.Context, string) (DeletionObservation, error)
}

// WaitForDeletion bounds asynchronous cleanup and requires the owning service
// inventory to be empty. It is safe to call again after interruption because
// the ownership marker, not a guessed resource list, defines the scope.
func WaitForDeletion(ctx context.Context, client DeletionClient, ownershipMarker string, policy ResilienceOperationPolicy) (DeletionObservation, error) {
	if ctx == nil {
		return DeletionObservation{}, errors.New("deletion context is required")
	}
	if client == nil {
		return DeletionObservation{}, errors.New("deletion client is required")
	}
	if strings.TrimSpace(ownershipMarker) == "" || strings.ContainsAny(ownershipMarker, "\r\n\x00") {
		return DeletionObservation{}, errors.New("deletion ownership marker is required and must be single-line")
	}
	if err := validateResilienceOperationPolicy(policy); err != nil {
		return DeletionObservation{}, err
	}
	deletionCtx, cancel := context.WithTimeout(ctx, policy.Timeout)
	defer cancel()
	var last DeletionObservation
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := deletionCtx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return last, fmt.Errorf("deletion timed out after %s", policy.Timeout)
			}
			return last, err
		}
		observation, err := client.PollDeletion(deletionCtx, ownershipMarker)
		if err != nil {
			if attempt == policy.MaxAttempts {
				return last, fmt.Errorf("poll deletion after %d attempts: %w", attempt, err)
			}
		} else {
			last = observation
			switch observation.Status {
			case ResilienceOperationSucceeded:
				if len(observation.ResourceRefs) == 0 && len(observation.DelayedResourceRefs) == 0 && len(observation.ProtectedResourceRefs) == 0 {
					return observation, nil
				}
			case ResilienceOperationPending, ResilienceOperationRunning:
			case ResilienceOperationFailed:
				if observation.Detail == "" {
					observation.Detail = "owning service reported deletion failure"
				}
				return observation, fmt.Errorf("deletion failed: %s", observation.Detail)
			default:
				return DeletionObservation{}, fmt.Errorf("deletion returned unknown status %q", observation.Status)
			}
		}
		if attempt == policy.MaxAttempts {
			return last, fmt.Errorf("deletion did not complete after %d attempts", attempt)
		}
		timer := time.NewTimer(policy.PollInterval)
		select {
		case <-deletionCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			if errors.Is(deletionCtx.Err(), context.DeadlineExceeded) {
				return last, fmt.Errorf("deletion timed out after %s", policy.Timeout)
			}
			return last, deletionCtx.Err()
		case <-timer.C:
		}
	}
	return last, errors.New("deletion did not complete")
}
