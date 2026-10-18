package certification

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/magelift/magelift/sdk"
)

// CleanupProofFromDeletionObservation converts an owning-service inventory
// observation into the evidence shape. The conversion is deliberately pure:
// polling, retries, and provider API translation stay in sdk or the
// provider adapter, while every provider gets the same cleanup truth rules.
func CleanupProofFromDeletionObservation(prefix, ownershipMarker string, observation sdk.DeletionObservation, checkedAt time.Time, latency time.Duration) (CleanupProof, error) {
	if strings.TrimSpace(prefix) == "" || strings.ContainsAny(prefix, "\r\n\x00") {
		return CleanupProof{}, errors.New("cleanup proof prefix is required and must be single-line")
	}
	if strings.TrimSpace(ownershipMarker) == "" || strings.ContainsAny(ownershipMarker, "\r\n\x00") {
		return CleanupProof{}, errors.New("cleanup proof ownership marker is required and must be single-line")
	}
	if checkedAt.IsZero() {
		return CleanupProof{}, errors.New("cleanup proof checked-at time is required")
	}
	if latency < 0 {
		return CleanupProof{}, errors.New("cleanup proof latency cannot be negative")
	}
	if err := validateCleanupReferences(observation); err != nil {
		return CleanupProof{}, err
	}
	proof := CleanupProof{
		Prefix:          prefix,
		OwnershipMarker: ownershipMarker,
		Remaining:       sortedUnique(observation.ResourceRefs),
		Live:            sortedUnique(observation.ResourceRefs),
		Delayed:         sortedUnique(observation.DelayedResourceRefs),
		Protected:       sortedUnique(observation.ProtectedResourceRefs),
		CheckedAt:       checkedAt.UTC().Format(time.RFC3339Nano),
		LatencySeconds:  latencySeconds(latency),
	}
	if observation.OperationID != "" {
		proof.Metadata = append(proof.Metadata, "operation:"+observation.OperationID)
	}
	if observation.Detail != "" {
		proof.Metadata = append(proof.Metadata, "detail:"+observation.Detail)
	}
	switch observation.Status {
	case sdk.ResilienceOperationSucceeded:
		if len(proof.Remaining) == 0 && len(proof.Delayed) == 0 && len(proof.Protected) == 0 {
			proof.Status = StatusPass
		} else {
			proof.Status = StatusPending
		}
	case sdk.ResilienceOperationPending, sdk.ResilienceOperationRunning:
		proof.Status = StatusPending
	case sdk.ResilienceOperationFailed:
		proof.Status = StatusFail
	default:
		return CleanupProof{}, fmt.Errorf("unsupported deletion observation status %q", observation.Status)
	}
	return proof, nil
}

// NewCleanupRecord converts an owning-service deletion observation into a
// sealed append-only evidence record. Provider adapters supply only the
// normalized observation; status, reason, provenance, and digest rules stay
// in this provider-neutral package.
func NewCleanupRecord(runID, source, prefix, ownershipMarker string, observation sdk.DeletionObservation, checkedAt time.Time, latency time.Duration) (Record, error) {
	if !runIDPattern.MatchString(runID) {
		return Record{}, errors.New("cleanup evidence run ID is invalid")
	}
	if strings.TrimSpace(source) == "" || strings.ContainsAny(source, "\r\n\x00") {
		return Record{}, errors.New("cleanup evidence source is required and must be single-line")
	}
	proof, err := CleanupProofFromDeletionObservation(prefix, ownershipMarker, observation, checkedAt, latency)
	if err != nil {
		return Record{}, err
	}
	record := Record{
		Version: EvidenceVersion,
		Type:    RecordCleanup,
		RunID:   runID,
		Status:  proof.Status,
		Cleanup: proof,
		Provenance: Provenance{
			GeneratedBy: EvidenceGenerator,
			GeneratedAt: checkedAt.UTC().Format(time.RFC3339),
			Source:      source,
			RunID:       runID,
		},
	}
	if proof.Status != StatusPass {
		record.Reason = cleanupRecordReason(proof, observation)
	}
	if err := record.Seal(); err != nil {
		return Record{}, fmt.Errorf("seal cleanup evidence: %w", err)
	}
	return record, nil
}

func cleanupRecordReason(proof CleanupProof, observation sdk.DeletionObservation) string {
	if strings.TrimSpace(observation.Detail) != "" {
		return observation.Detail
	}
	switch proof.Status {
	case StatusFail:
		return "owning service reported deletion failure"
	case StatusPending:
		return "owning service has not confirmed complete deletion"
	default:
		return "cleanup was not verified"
	}
}

func validateCleanupReferences(observation sdk.DeletionObservation) error {
	seen := make(map[string]string, len(observation.ResourceRefs)+len(observation.DelayedResourceRefs)+len(observation.ProtectedResourceRefs))
	for name, values := range map[string][]string{
		"remaining": observation.ResourceRefs,
		"delayed":   observation.DelayedResourceRefs,
		"protected": observation.ProtectedResourceRefs,
	} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("cleanup proof %s resource reference is invalid", name)
			}
			if previous, exists := seen[value]; exists {
				if previous == name {
					return fmt.Errorf("cleanup proof %s resource %q is duplicated", name, value)
				}
				return fmt.Errorf("cleanup proof resource %q appears in multiple inventory classes", value)
			}
			seen[value] = name
		}
	}
	if strings.ContainsAny(observation.OperationID+observation.Detail, "\r\n\x00") {
		return errors.New("cleanup proof operation metadata must be single-line")
	}
	return nil
}

func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func latencySeconds(value time.Duration) int64 {
	if value == 0 {
		return 0
	}
	seconds := int64(value / time.Second)
	if value%time.Second != 0 {
		seconds++
	}
	return seconds
}
