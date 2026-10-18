package certification

import (
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/sdk"
)

func TestCleanupProofFromDeletionObservationRequiresEmptyOwningInventoryForPass(t *testing.T) {
	checkedAt := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	proof, err := CleanupProofFromDeletionObservation("aws/cleanup", "magelift/test/cleanup", sdk.DeletionObservation{
		Status: sdk.ResilienceOperationSucceeded, OperationID: "delete-1",
	}, checkedAt, 1500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Status != StatusPass || proof.LatencySeconds != 2 || proof.CheckedAt != checkedAt.Format(time.RFC3339Nano) || len(proof.Metadata) != 1 {
		t.Fatalf("cleanup proof = %#v", proof)
	}
}

func TestCleanupProofFromDeletionObservationSeparatesDelayedAndProtectedResources(t *testing.T) {
	proof, err := CleanupProofFromDeletionObservation("gcp/cleanup", "magelift/test/cleanup", sdk.DeletionObservation{
		Status: sdk.ResilienceOperationSucceeded, OperationID: "delete-2", ResourceRefs: []string{"live:db"}, DelayedResourceRefs: []string{"tombstone:network"}, ProtectedResourceRefs: []string{"protected:key"}, Detail: "retry cleanup",
	}, time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Status != StatusPending || len(proof.Live) != 1 || len(proof.Delayed) != 1 || len(proof.Protected) != 1 {
		t.Fatalf("cleanup proof = %#v", proof)
	}
	if len(proof.Remaining) != 1 || !strings.HasPrefix(proof.Metadata[1], "detail:") {
		t.Fatalf("cleanup metadata = %#v", proof.Metadata)
	}
}

func TestCleanupProofFromDeletionObservationRejectsAmbiguousInventory(t *testing.T) {
	_, err := CleanupProofFromDeletionObservation("ovh/cleanup", "magelift/test/cleanup", sdk.DeletionObservation{
		Status: sdk.ResilienceOperationPending, ResourceRefs: []string{"resource:1"}, DelayedResourceRefs: []string{"resource:1"},
	}, time.Now().UTC(), time.Second)
	if err == nil || !strings.Contains(err.Error(), "multiple inventory classes") {
		t.Fatalf("ambiguous cleanup observation error = %v", err)
	}
}

func TestCleanupProofFromDeletionObservationRejectsDuplicateInventory(t *testing.T) {
	_, err := CleanupProofFromDeletionObservation("ovh/cleanup", "magelift/test/cleanup", sdk.DeletionObservation{
		Status: sdk.ResilienceOperationPending, ResourceRefs: []string{"resource:1", "resource:1"},
	}, time.Now().UTC(), time.Second)
	if err == nil || !strings.Contains(err.Error(), "is duplicated") {
		t.Fatalf("duplicate cleanup observation error = %v", err)
	}
}

func TestNewCleanupRecordSealsProviderNeutralEvidence(t *testing.T) {
	checkedAt := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	record, err := NewCleanupRecord("run-cleanup", "aws/delete-observation", "aws/cleanup", "magelift/test/cleanup", sdk.DeletionObservation{
		Status:      sdk.ResilienceOperationSucceeded,
		OperationID: "delete-3",
	}, checkedAt, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if record.Type != RecordCleanup || record.Status != StatusPass || record.RecordDigest == "" {
		t.Fatalf("cleanup record = %#v", record)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestNewCleanupRecordRequiresReasonForNonPassObservation(t *testing.T) {
	record, err := NewCleanupRecord("run-cleanup", "gcp/delete-observation", "gcp/cleanup", "magelift/test/cleanup", sdk.DeletionObservation{
		Status: sdk.ResilienceOperationPending,
	}, time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC), 0)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != StatusPending || record.Reason == "" {
		t.Fatalf("pending cleanup record = %#v", record)
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}
