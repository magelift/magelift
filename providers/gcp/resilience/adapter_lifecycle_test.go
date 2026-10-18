package resilience

import (
	"context"
	"strings"
	"testing"
	"time"

	provider "github.com/magelift/magelift/internal/provider"
	cloudresilience "github.com/magelift/magelift/internal/shared/resilience"
	"github.com/magelift/magelift/sdk"
)

type scriptedOperationClient struct {
	started []sdk.ResilienceOperationRequest
	stages  map[string]sdk.ResilienceOperationRequest
	polls   map[string]int
}

func newScriptedOperationClient() *scriptedOperationClient {
	return &scriptedOperationClient{stages: make(map[string]sdk.ResilienceOperationRequest), polls: make(map[string]int)}
}

func (c *scriptedOperationClient) ResilienceCapabilities() []sdk.ResilienceAction {
	return []sdk.ResilienceAction{sdk.ResilienceBackup, sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck, sdk.ResilienceFence, sdk.ResilienceFailover, sdk.ResilienceCleanup}
}

func (c *scriptedOperationClient) Start(_ context.Context, request sdk.ResilienceOperationRequest) (sdk.ResilienceOperationObservation, error) {
	c.started = append(c.started, request)
	operationID := "operation-" + string(request.Action)
	c.stages[operationID] = request
	return sdk.ResilienceOperationObservation{
		Status: sdk.ResilienceOperationPending, Action: request.Action,
		OperationID: operationID, OwnershipMarker: request.OwnershipMarker,
		OwnershipVerified: true, IdempotencyVerified: true,
	}, nil
}

func (c *scriptedOperationClient) Poll(_ context.Context, operationID string) (sdk.ResilienceOperationObservation, error) {
	request := c.stages[operationID]
	c.polls[operationID]++
	return sdk.ResilienceOperationObservation{
		Status: sdk.ResilienceOperationSucceeded, Action: request.Action,
		OperationID: operationID, ProofRefs: []string{"proof/" + operationID},
		OwnershipMarker: request.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true,
		Evidence: []sdk.ResilienceProofEvidence{{
			DataClass: request.DataClasses[0], Destination: string(request.Destination), FixtureID: request.FixtureID,
			BackupID: "backup/fixture", RestoreID: "restore/fixture", RetentionDays: 30,
			EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true,
			CountsVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true,
			SecretReferencesVerified: true, ServiceHealthVerified: true, MeasuredDurationSeconds: 1,
		}},
	}, nil
}

func (c *scriptedOperationClient) Inventory(context.Context, string) ([]sdk.ResilienceInventoryResource, error) {
	return []sdk.ResilienceInventoryResource{{Identity: "owned-resource", Owned: true, Live: false}}, nil
}

func gcpLifecycleIntent() sdk.ArchitectureIntent {
	owner := "magelift/test/gcp"
	return sdk.ArchitectureIntent{
		ProfileID: "test.gcp", Provider: "gcp", Runtime: "gke",
		AccountOrProjectRef: "opaque/account", Region: "region-1", Regions: []string{"region-1"}, Zones: []string{"zone-a", "zone-b"},
		ComputeMode: "managed", KubernetesMode: "standard", NetworkMode: "private", IngressMode: "load-balancer",
		Boundaries: []sdk.ServiceBoundaryIntent{
			{Role: "database", Family: "mysql", Major: "8.4", Ownership: sdk.ServiceManaged, ResourceReference: "opaque/database", BackupProfile: "snapshot", RecoveryProfile: "isolated"},
			{Role: "media", Family: "object", Major: "current", Ownership: sdk.ServiceManaged, ResourceReference: "opaque/media", BackupProfile: "versioned", RecoveryProfile: "isolated"},
			{Role: "cache", Family: "valkey", Major: "9", Ownership: sdk.ServiceManaged, ResourceReference: "opaque/cache", BackupProfile: "rebuild", RecoveryProfile: "isolated"},
		},
		Edge: sdk.EdgeIntent{Mode: "none"}, Observability: sdk.ObservabilityIntent{NativeProvider: "none", ExternalProvider: "none", OwnershipMarker: owner},
		Resilience: sdk.ResilienceIntent{
			ProfileID: "test.gcp", AvailabilityTarget: "99.9", RPOSeconds: 300, RTOSeconds: 1800, RetentionDays: 30,
			RecoveryScope: "application-and-durable-data", FailoverOwner: "operator", FencingPolicy: "single-writer", RecoveryDestinations: []sdk.RecoveryDestination{sdk.RecoverySameRegionIsolated},
			DataClasses: []sdk.DataClassIntent{
				{Name: "database", SourceOfTruth: "database", FailureDomains: []string{"zone"}, BackupMethod: "snapshot", RestoreMethod: "isolated", IntegrityMethod: "checksum", LossSemantics: "rpo-bound", RetentionDays: 30, Encrypted: true, Immutable: true, DeletionProtection: true, OwnershipMarker: owner + "/database"},
				{Name: "media", SourceOfTruth: "object", FailureDomains: []string{"region"}, BackupMethod: "versioned", RestoreMethod: "isolated", IntegrityMethod: "manifest", LossSemantics: "rpo-bound", RetentionDays: 30, Encrypted: true, Immutable: true, DeletionProtection: true, OwnershipMarker: owner + "/media"},
				{Name: "cache", SourceOfTruth: "database", FailureDomains: []string{"zone"}, BackupMethod: "rebuild", RestoreMethod: "warmup", IntegrityMethod: "health", LossSemantics: "reconstructible", RetentionDays: 1, OwnershipMarker: owner + "/cache"},
			},
		},
		ArtifactDigest: "registry.example.invalid/magelift@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SchemaFingerprint: "schema", MigrationFingerprint: "migration", OwnershipMarker: owner,
	}
}

func TestGCPAdapterUsesProviderNeutralLifecycle(t *testing.T) {
	adapter, err := New(newScriptedOperationClient(), sdk.ResilienceOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 3})
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	descriptor := adapter.ResilienceDescriptor()
	if descriptor.Provider != sdk.ProviderID("gcp") {
		t.Fatalf("provider = %q", descriptor.Provider)
	}
	if err := sdk.ValidateResilienceAdapterDescriptor(descriptor); err != nil {
		t.Fatalf("validate descriptor: %v", err)
	}
	intent := gcpLifecycleIntent()
	plan, err := adapter.PlanResilience(context.Background(), sdk.ResiliencePlanRequest{
		Architecture: intent, FixtureID: "fixture/known-content", OwnershipMarker: intent.OwnershipMarker,
	})
	if err != nil {
		t.Fatalf("plan resilience: %v", err)
	}
	if len(plan.Stages) == 0 || len(plan.RequiredProofs) == 0 {
		t.Fatal("plan must include stages and required proofs")
	}
	for _, stage := range plan.Stages {
		result, err := adapter.ExecuteResilience(context.Background(), sdk.ResilienceExecutionRequest{
			Plan: plan, StageID: stage.ID, FixtureID: "fixture/known-content", IdempotencyKey: "key/" + stage.ID,
			OwnershipMarker: intent.OwnershipMarker, ResourceReferences: map[string]string{"database": "opaque/database"},
		})
		if err != nil {
			t.Fatalf("execute %s: %v", stage.ID, err)
		}
		if result.Action != stage.Action || !result.OwnershipVerified || !result.IdempotencyVerified || len(result.Evidence) == 0 {
			t.Fatalf("stage %s returned incomplete proof: %#v", stage.ID, result)
		}
	}
	inventory, ok := adapter.(interface {
		Inventory(context.Context, string) ([]sdk.ResilienceInventoryResource, error)
	})
	if !ok {
		t.Fatal("operation-backed adapter does not expose owning-service inventory")
	}
	resources, err := inventory.Inventory(context.Background(), intent.OwnershipMarker)
	if err != nil || len(resources) != 1 || resources[0].Live {
		t.Fatalf("inventory = %#v, err = %v", resources, err)
	}
}

func TestGCPAdapterRowsDoNotClaimUnimplementedClients(t *testing.T) {
	adapter, err := New(newScriptedOperationClient(), sdk.DefaultResilienceOperationPolicy())
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	for _, capability := range adapter.ResilienceDescriptor().DataClasses {
		if capability.Status == sdk.ResilienceCapabilitySupported || capability.Status == sdk.ResilienceCapabilityCertified {
			t.Fatalf("data class %q claims status %q without a concrete provider client", capability.Name, capability.Status)
		}
		if capability.Status == sdk.ResilienceCapabilityExperimental && !strings.Contains(capability.Reason, "operation translator") {
			t.Fatalf("experimental data class %q has incomplete reason: %q", capability.Name, capability.Reason)
		}
	}
}

type stubNativeOperationAPI struct {
	started []cloudresilience.NativeOperationRequest
}

func (api *stubNativeOperationAPI) Start(_ context.Context, request cloudresilience.NativeOperationRequest) (provider.NativeOperationObservation, error) {
	api.started = append(api.started, request)
	return provider.NativeOperationObservation{
		Status: "accepted", Action: request.Request.Action, OperationID: "operation-1",
		OwnershipMarker: request.Request.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true,
	}, nil
}

func (api *stubNativeOperationAPI) Poll(_ context.Context, operationID string) (provider.NativeOperationObservation, error) {
	return provider.NativeOperationObservation{Status: "completed", Action: sdk.ResilienceBackup, OperationID: operationID, OwnershipMarker: "magelift/test/operations", OwnershipVerified: true, IdempotencyVerified: true}, nil
}

func (api *stubNativeOperationAPI) Inventory(context.Context, string) ([]provider.InventoryResource, error) {
	return []provider.InventoryResource{{Identity: "owned/resource", Owned: true, Live: false}}, nil
}

func TestGCPNativeClientKeepsOperationMapping(t *testing.T) {
	api := &stubNativeOperationAPI{}
	client, err := NewNativeResilienceClient(api)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.ResilienceOperationRequest{Action: sdk.ResilienceBackup, DataClasses: []string{"database"}, FixtureID: "fixture/known-content", OwnershipMarker: "magelift/test/operations", IdempotencyKey: "operation/1"}
	observation, err := client.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(api.started) != 1 || api.started[0].Operation != "gcp.cloud-sql.backup.database" {
		t.Fatalf("mapped operation = %#v", api.started)
	}
	if observation.Status != sdk.ResilienceOperationPending || observation.OperationID == "" {
		t.Fatalf("observation = %#v", observation)
	}
}
