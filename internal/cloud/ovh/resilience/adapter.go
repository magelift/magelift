// Package resilience exposes the OVHcloud recovery lifecycle boundary.
//
// OVHcloud-specific clients translate supported operations to Managed
// Databases, Object Storage, Secret Manager or the selected self-hosted
// service APIs. Provider gaps are explicit and stop unsupported recovery
// plans before mutation.
package resilience

import (
	"context"

	"github.com/magelift/magelift/internal/platform"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/internal/shared/resilience"
	"github.com/magelift/magelift/sdk"
)

const AdapterID = "ovh.resilience"

func Descriptor() sdk.ResilienceAdapterDescriptor {
	return resilience.Descriptor(sdk.ProviderID("ovh"), AdapterID, "1.0.0", []resilience.DataClassSpec{
		{Name: "database", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "managed-backup-and-isolated-restore", BackupMechanism: "Managed-Database-backup", RestoreMechanism: "managed-instance-restore", IntegrityMethod: "fixture-checksum-and-application-read", RetentionPolicy: "managed-database-backup-retention", EncryptionBoundary: "managed-database-encryption-boundary", ProtectionMechanism: "managed-database-deletion-protection-where-available", Reason: resilience.NativeTranslatorReason("ovh", "database"), Durable: true, ProtectionOptional: true},
		{Name: "media", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "versioned-object-backup-and-manifest-check", BackupMechanism: "Object-Storage-versioning-and-replication", RestoreMechanism: "isolated-bucket-or-prefix-restore", IntegrityMethod: "manifest-and-content-checksum", RetentionPolicy: "Object-Storage-retention-policy-or-version-retention", EncryptionBoundary: "Object-Storage-encryption-boundary", ProtectionMechanism: "object-versioning-and-owned-prefix-protection", Reason: resilience.NativeTranslatorReason("ovh", "media"), Durable: true},
		{Name: "configuration-secrets", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "versioned-secret-and-config-export", BackupMechanism: "encrypted-provider-object-config-archive", RestoreMechanism: "scoped-secret-reference-restore", IntegrityMethod: "reference-and-permission-check", RetentionPolicy: "encrypted-config-archive-retention", EncryptionBoundary: "provider-or-archive-encryption-boundary", ProtectionMechanism: "secret-reference-pinning-and-owned-archive-retention", Reason: resilience.NativeTranslatorReason("ovh", "configuration-secrets"), Durable: true},
		{Name: "infrastructure-state", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "immutable-state-backup", BackupMechanism: "versioned-encrypted-object-state-archive", RestoreMechanism: "isolated-state-prefix-restore", IntegrityMethod: "archive-manifest-checksum", RetentionPolicy: "state-archive-retention", EncryptionBoundary: "encrypted-object-state-archive", ProtectionMechanism: "versioned-state-archive-and-owned-prefix-protection", Reason: resilience.NativeTranslatorReason("ovh", "infrastructure-state"), Durable: true},
		{Name: "queue", Status: sdk.ResilienceCapabilityUnavailable, Strategy: "unsupported-provider-boundary", Reason: "No current OVHcloud managed queue or MageLift queue recovery adapter is declared; use a self-hosted broker profile only after its own adapter is implemented."},
		{Name: "search-index", Status: sdk.ResilienceCapabilityExperimental, Actions: restoreActions(), Destinations: destinations(), Strategy: "rebuild-from-self-hosted-workload", RestoreMechanism: "MKS-workload-index-rebuild", IntegrityMethod: "document-count-and-application-query", Reason: "OVHcloud has no managed search product in this catalog; the MageLift operation translator requires an injected MKS/workload projection adapter for self-hosted search rebuilds, and live certification remains open.", Durable: false},
		{Name: "cache", Status: sdk.ResilienceCapabilityExperimental, Actions: restoreActions(), Destinations: destinations(), Strategy: "reconstruct-from-source-of-truth", RestoreMechanism: "cache-warmup-from-durable-source", IntegrityMethod: "application-read-and-cache-health", Reason: resilience.ExperimentalReason("ovh", "cache"), Durable: false},
		{Name: "audit-evidence", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "append-only-evidence-export", BackupMechanism: "encrypted-versioned-object-export", RestoreMechanism: "isolated-evidence-prefix-restore", IntegrityMethod: "record-digest-chain", RetentionPolicy: "evidence-archive-retention", EncryptionBoundary: "encrypted-evidence-archive", ProtectionMechanism: "append-only-owned-evidence-prefix", Reason: resilience.NativeTranslatorReason("ovh", "audit-evidence"), Durable: true},
	})
}

func New(client sdk.ResilienceOperationClient, policy sdk.ResilienceOperationPolicy) (sdk.ResilienceAdapter, error) {
	return resilience.New(Descriptor(), client, policy)
}

// NewResilienceClient wraps an OVHcloud API translator without exposing OVH
// response models to the provider-neutral recovery lifecycle.
func NewResilienceClient(backend provider.NativeOperationBackend) (sdk.ResilienceOperationClient, error) {
	return provider.NewNormalizedOperationClient(backend)
}

// NewNativeResilienceClient adds the OVHcloud operation-family translator in
// front of an official API wrapper. Managed Database, Valkey, Object Storage,
// Logs Data Platform, and MKS/workload request models stay at that wrapper's
// boundary.
func NewNativeResilienceClient(api resilience.NativeOperationAPI) (sdk.ResilienceOperationClient, error) {
	backend, err := resilience.NewMappedOperationBackend(sdk.ProviderID("ovh"), api, ovhOperationName)
	if err != nil {
		return nil, err
	}
	return provider.NewNormalizedOperationClient(backend)
}

func ovhOperationName(request sdk.ResilienceOperationRequest) (string, error) {
	prefix := "ovh.recovery"
	if len(request.DataClasses) == 1 {
		switch request.DataClasses[0] {
		case "database":
			prefix = "ovh.managed-database"
		case "media", "infrastructure-state", "audit-evidence":
			prefix = "ovh.object-storage"
		case "configuration-secrets":
			prefix = "ovh.secret-reference"
		case "queue":
			prefix = "ovh.queue"
		case "search-index":
			prefix = "ovh.search"
		case "cache":
			prefix = "ovh.valkey"
		}
	}
	return resilience.OperationName(prefix, request.Action, request.DataClasses)
}

// NewFromFactory validates the exact OVHcloud target identity before
// constructing a provider-owned SDK client.
func NewFromFactory(ctx context.Context, factory provider.ResilienceClientFactory, request provider.ClientRequest, policy sdk.ResilienceOperationPolicy) (sdk.ResilienceAdapter, error) {
	if err := provider.ValidateClientRequestForProvider(request, sdk.ProviderID("ovh")); err != nil {
		return nil, err
	}
	client, err := provider.ConstructResilienceClient(ctx, factory, request)
	if err != nil {
		return nil, err
	}
	return New(client, policy)
}

// NewLifecycleFactories adapts an injected OVHcloud SDK operation client to
// the planned provider-neutral module seam.
func NewLifecycleFactories(client sdk.ResilienceOperationClient, policy sdk.ResilienceOperationPolicy) platform.LifecycleFactories {
	return platform.LifecycleFactories{
		Resilience: func(context.Context, platform.PlannedStack) (sdk.ResilienceAdapter, error) {
			return New(client, policy)
		},
	}
}

func destinations() []sdk.RecoveryDestination {
	return []sdk.RecoveryDestination{sdk.RecoverySameRegion, sdk.RecoverySameRegionIsolated, sdk.RecoveryAlternateRegion}
}

func durableActions() []sdk.ResilienceAction {
	return []sdk.ResilienceAction{sdk.ResilienceBackup, sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck}
}

func restoreActions() []sdk.ResilienceAction {
	return []sdk.ResilienceAction{sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck}
}
