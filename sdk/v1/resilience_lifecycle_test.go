package v1

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type lifecycleClient struct {
	start ResilienceOperationObservation
	polls []ResilienceOperationObservation
	index int
}

func (c *lifecycleClient) Start(context.Context, ResilienceOperationRequest) (ResilienceOperationObservation, error) {
	return c.start, nil
}

func (c *lifecycleClient) Poll(context.Context, string) (ResilienceOperationObservation, error) {
	if c.index >= len(c.polls) {
		return ResilienceOperationObservation{Status: ResilienceOperationRunning, OperationID: "operation-1"}, nil
	}
	result := c.polls[c.index]
	c.index++
	return result, nil
}

func (c *lifecycleClient) Inventory(context.Context, string) ([]ResilienceInventoryResource, error) {
	return nil, nil
}

func TestWaitForResilienceOperationPollsAndPropagatesProofs(t *testing.T) {
	client := &lifecycleClient{
		polls: []ResilienceOperationObservation{
			{Status: ResilienceOperationRunning, OperationID: "operation-1"},
			{Status: ResilienceOperationSucceeded, OperationID: "operation-1", OwnershipMarker: "magelift/test", OwnershipVerified: true, IdempotencyVerified: true, ProofRefs: []string{"proof/1"}},
		},
	}
	observation, err := WaitForResilienceOperation(context.Background(), client, "operation-1", ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 3})
	if err != nil {
		t.Fatalf("WaitForResilienceOperation() error = %v", err)
	}
	if observation.Status != ResilienceOperationSucceeded || len(observation.ProofRefs) != 1 || client.index != 2 {
		t.Fatalf("observation = %#v, polls = %d", observation, client.index)
	}
}

func TestWaitForResilienceOperationRejectsFailureIdentityAndCancellation(t *testing.T) {
	tests := []struct {
		name   string
		client *lifecycleClient
		want   string
	}{
		{name: "failure", client: &lifecycleClient{polls: []ResilienceOperationObservation{{Status: ResilienceOperationFailed, OperationID: "operation-1", Detail: "denied"}}}, want: "denied"},
		{name: "identity", client: &lifecycleClient{polls: []ResilienceOperationObservation{{Status: ResilienceOperationSucceeded, OperationID: "operation-2"}}}, want: "identity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := WaitForResilienceOperation(context.Background(), tt.client, "operation-1", ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 1})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := WaitForResilienceOperation(ctx, &lifecycleClient{}, "operation-1", ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 1})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v, want context.Canceled", err)
	}
}

func TestOperationBackedResilienceAdapterRequiresScopedEvidenceInputs(t *testing.T) {
	descriptor := ResilienceAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "test.lifecycle", Provider: "aws", Version: "1.0.0",
		Capabilities: []ResilienceAction{ResilienceBackup, ResilienceRestore, ResilienceIntegrityCheck, ResilienceCleanup},
		DataClasses: []ResilienceDataClassCapability{{
			Name: "database", Status: ResilienceCapabilitySupported,
			Actions:         []ResilienceAction{ResilienceBackup, ResilienceRestore, ResilienceIntegrityCheck},
			Destinations:    []RecoveryDestination{RecoverySameRegionIsolated},
			PollingRequired: true,
		}},
	}
	client := &lifecycleClient{start: ResilienceOperationObservation{
		Status: ResilienceOperationSucceeded, Action: ResilienceBackup, OperationID: "operation-1",
		OwnershipMarker: "magelift/test", OwnershipVerified: true, IdempotencyVerified: true,
	}}
	adapter, err := NewOperationBackedResilienceAdapter(descriptor, client, ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 1})
	if err != nil {
		t.Fatalf("NewOperationBackedResilienceAdapter() error = %v", err)
	}
	plan := ResiliencePlan{AdapterID: descriptor.ID, FixtureID: "fixture", OwnershipMarker: "magelift/test", Stages: []ResilienceStage{{ID: "backup-database", Action: ResilienceBackup, DataClasses: []string{"database"}, Destination: RecoverySameRegionIsolated, Idempotent: true}}, RequiredProofs: []string{"backup:database"}}
	_, err = adapter.ExecuteResilience(context.Background(), ResilienceExecutionRequest{Plan: plan, StageID: "backup-database", IdempotencyKey: "key", OwnershipMarker: "magelift/test"})
	if err == nil || !strings.Contains(err.Error(), "fixture ID") {
		t.Fatalf("missing fixture error = %v", err)
	}
	_, err = adapter.ExecuteResilience(context.Background(), ResilienceExecutionRequest{Plan: plan, StageID: "backup-database", FixtureID: "fixture", IdempotencyKey: "key", OwnershipMarker: "magelift/test", ResourceReferences: map[string]string{"database": "bad\nref"}})
	if err == nil || !strings.Contains(err.Error(), "single-line") {
		t.Fatalf("unsafe resource reference error = %v", err)
	}
}

func TestOperationBackedResilienceAdapterRejectsIncompleteClassEvidence(t *testing.T) {
	descriptor := ResilienceAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "test.evidence", Provider: "aws", Version: "1.0.0",
		Capabilities: []ResilienceAction{ResilienceBackup, ResilienceCleanup},
		DataClasses: []ResilienceDataClassCapability{{
			Name: "database", Status: ResilienceCapabilitySupported, PollingRequired: true,
			Actions: []ResilienceAction{ResilienceBackup}, Destinations: []RecoveryDestination{RecoverySameRegionIsolated},
		}},
	}
	client := &lifecycleClient{start: ResilienceOperationObservation{
		Status: ResilienceOperationSucceeded, Action: ResilienceBackup, OperationID: "operation-1",
		OwnershipMarker: "magelift/test", OwnershipVerified: true, IdempotencyVerified: true,
	}}
	adapter, err := NewOperationBackedResilienceAdapter(descriptor, client, ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 1})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	plan := ResiliencePlan{AdapterID: descriptor.ID, FixtureID: "fixture", OwnershipMarker: "magelift/test", Stages: []ResilienceStage{{ID: "backup-database", Action: ResilienceBackup, DataClasses: []string{"database"}, Destination: RecoverySameRegionIsolated, Idempotent: true}}, RequiredProofs: []string{"backup:database"}}
	_, err = adapter.ExecuteResilience(context.Background(), ResilienceExecutionRequest{
		Plan: plan, StageID: "backup-database", FixtureID: "fixture", IdempotencyKey: "key", OwnershipMarker: "magelift/test",
	})
	if err == nil || !strings.Contains(err.Error(), "independent evidence") {
		t.Fatalf("incomplete evidence error = %v", err)
	}

	client.start.Evidence = []ResilienceProofEvidence{{
		DataClass: "database", Destination: string(RecoverySameRegionIsolated), FixtureID: "fixture", BackupID: "backup-1",
	}}
	result, err := adapter.ExecuteResilience(context.Background(), ResilienceExecutionRequest{
		Plan: plan, StageID: "backup-database", FixtureID: "fixture", IdempotencyKey: "key-2", OwnershipMarker: "magelift/test",
	})
	if err != nil || result.Action != ResilienceBackup {
		t.Fatalf("complete evidence result = %#v, error = %v", result, err)
	}
}

func TestOperationBackedResilienceAdapterRequiresFencingSafetyProof(t *testing.T) {
	descriptor := ResilienceAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "test.fencing", Provider: "aws", Version: "1.0.0",
		Capabilities: []ResilienceAction{ResilienceFence, ResilienceCleanup},
		DataClasses: []ResilienceDataClassCapability{{
			Name: "database", Status: ResilienceCapabilitySupported,
			Actions: []ResilienceAction{ResilienceIntegrityCheck}, PollingRequired: true,
		}},
	}
	client := &lifecycleClient{start: ResilienceOperationObservation{
		Status: ResilienceOperationSucceeded, Action: ResilienceFence, OperationID: "operation-fence",
		OwnershipMarker: "magelift/test", OwnershipVerified: true, IdempotencyVerified: true,
	}}
	adapter, err := NewOperationBackedResilienceAdapter(descriptor, client, ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 1})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	plan := ResiliencePlan{AdapterID: descriptor.ID, FixtureID: "fixture", OwnershipMarker: "magelift/test", Stages: []ResilienceStage{{
		ID: "fence-runtime", Action: ResilienceFence, DataClasses: []string{"database"},
		Destination: RecoveryAlternateRegion, Idempotent: true, RequiresApproval: true,
		RequiresSingleWriter: true, RequiresSplitBrainCheck: true,
	}}, RequiredProofs: []string{"fencing", "single-writer", "split-brain"}}
	request := ResilienceExecutionRequest{Plan: plan, StageID: "fence-runtime", FixtureID: "fixture", IdempotencyKey: "fence/1", OwnershipMarker: "magelift/test", ApprovalReference: "approval/fence"}
	client.start.Evidence = []ResilienceProofEvidence{{
		DataClass: "database", Destination: string(RecoveryAlternateRegion), FixtureID: "fixture",
	}}
	if _, err := adapter.ExecuteResilience(context.Background(), request); err == nil || !strings.Contains(err.Error(), "writer identity") {
		t.Fatalf("incomplete fencing safety proof error = %v", err)
	}
	client.start.Evidence = []ResilienceProofEvidence{{
		DataClass: "database", Destination: string(RecoveryAlternateRegion), FixtureID: "fixture",
		WriterIdentityRef: "writer/primary", WriterEpoch: "epoch/7", SingleWriterVerified: true,
		SplitBrainAbsent: true, ApprovalVerified: true,
	}}
	result, err := adapter.ExecuteResilience(context.Background(), request)
	if err != nil || result.Action != ResilienceFence {
		t.Fatalf("complete fencing safety proof result = %#v, error = %v", result, err)
	}
}

func TestOperationBackedResilienceAdapterRequiresApprovalSafetyProof(t *testing.T) {
	descriptor := ResilienceAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "test.approval", Provider: "aws", Version: "1.0.0",
		Capabilities: []ResilienceAction{ResilienceRestore, ResilienceCleanup},
		DataClasses: []ResilienceDataClassCapability{{
			Name: "database", Status: ResilienceCapabilitySupported,
			Actions: []ResilienceAction{ResilienceRestore}, Destinations: []RecoveryDestination{RecoverySameRegion}, PollingRequired: true,
		}},
	}
	client := &lifecycleClient{start: ResilienceOperationObservation{
		Status: ResilienceOperationSucceeded, Action: ResilienceRestore, OperationID: "operation-restore",
		OwnershipMarker: "magelift/test", OwnershipVerified: true, IdempotencyVerified: true,
	}}
	adapter, err := NewOperationBackedResilienceAdapter(descriptor, client, ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 1})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	plan := ResiliencePlan{AdapterID: descriptor.ID, FixtureID: "fixture", OwnershipMarker: "magelift/test", Stages: []ResilienceStage{{
		ID: "restore-database", Action: ResilienceRestore, DataClasses: []string{"database"},
		Destination: RecoverySameRegion, Idempotent: true, RequiresApproval: true,
	}}, RequiredProofs: []string{"restore:database"}}
	request := ResilienceExecutionRequest{
		Plan: plan, StageID: "restore-database", FixtureID: "fixture", IdempotencyKey: "restore/1",
		OwnershipMarker: "magelift/test", ApprovalReference: "approval/restore",
	}
	client.start.Evidence = []ResilienceProofEvidence{{
		DataClass: "database", Destination: string(RecoverySameRegion), FixtureID: "fixture",
		RestoreID: "restore-1", ServiceHealthVerified: true,
	}}
	if _, err := adapter.ExecuteResilience(context.Background(), request); err == nil || !strings.Contains(err.Error(), "writer identity") {
		t.Fatalf("incomplete approval safety proof error = %v", err)
	}

	client.start.Evidence = []ResilienceProofEvidence{{
		DataClass: "database", Destination: string(RecoverySameRegion), FixtureID: "fixture",
		RestoreID: "restore-1", ServiceHealthVerified: true,
		WriterIdentityRef: "writer/primary", WriterEpoch: "epoch/7", ApprovalVerified: true,
	}}
	result, err := adapter.ExecuteResilience(context.Background(), request)
	if err != nil || result.Action != ResilienceRestore {
		t.Fatalf("complete approval safety proof result = %#v, error = %v", result, err)
	}
}
