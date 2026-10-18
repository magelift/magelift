// Package resilience exposes the Scaleway recovery lifecycle boundary.
//
// Scaleway-specific clients translate supported operations to Managed
// Database, Object Storage, Secret Manager, and selected self-hosted service
// APIs. Missing managed boundaries remain unavailable instead of being
// silently substituted with another provider.
package resilience

import (
	"context"

	"github.com/magelift/magelift/internal/platform"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/internal/shared/resilience"
	"github.com/magelift/magelift/sdk"
)

const AdapterID = "scaleway.resilience"

func Descriptor() sdk.ResilienceAdapterDescriptor {
	return resilience.Descriptor(sdk.ProviderID("scaleway"), AdapterID, "1.0.0", []resilience.DataClassSpec{
		{Name: "database", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "managed-block-snapshot-and-isolated-restore", BackupMechanism: "Managed-Database-encrypted-block-snapshot", RestoreMechanism: "managed-instance-restore", IntegrityMethod: "fixture-checksum-and-application-read", RetentionPolicy: "managed-database-snapshot-expiration", EncryptionBoundary: "managed-database-encryption-at-rest-and-block-snapshot", ProtectionMechanism: "managed-database-snapshot-expiration-and-owned-source", Reason: resilience.NativeTranslatorReason("scaleway", "database"), Durable: true, ProtectionOptional: true},
		{Name: "media", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "versioned-object-backup-and-manifest-check", BackupMechanism: "Object-Storage-versioning-and-replication", RestoreMechanism: "isolated-bucket-or-prefix-restore", IntegrityMethod: "manifest-and-content-checksum", RetentionPolicy: "Object-Storage-retention-policy-or-version-retention", EncryptionBoundary: "Object-Storage-encryption-boundary", ProtectionMechanism: "object-versioning-and-owned-prefix-protection", Reason: resilience.NativeTranslatorReason("scaleway", "media"), Durable: true},
		{Name: "configuration-secrets", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "versioned-secret-and-config-export", BackupMechanism: "Secret-Manager-versions-and-encrypted-Object-Storage-archive", RestoreMechanism: "scoped-protected-secret-version-restore", IntegrityMethod: "reference-and-permission-check", RetentionPolicy: "Object-Lock-archive-retention", EncryptionBoundary: "Secret-Manager-and-Object-Storage-encryption-boundary", ProtectionMechanism: "Secret-Manager-protection-and-Object-Lock", Reason: resilience.NativeTranslatorReason("scaleway", "configuration-secrets"), Durable: true},
		{Name: "infrastructure-state", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "immutable-state-backup", BackupMechanism: "versioned-encrypted-object-state-archive", RestoreMechanism: "isolated-state-prefix-restore", IntegrityMethod: "archive-manifest-checksum", RetentionPolicy: "state-archive-retention", EncryptionBoundary: "encrypted-object-state-archive", ProtectionMechanism: "versioned-state-archive-and-owned-prefix-protection", Reason: resilience.NativeTranslatorReason("scaleway", "infrastructure-state"), Durable: true},
		{Name: "queue", Status: sdk.ResilienceCapabilityUnavailable, Strategy: "unsupported-provider-boundary", Reason: "No current Scaleway managed queue or MageLift queue recovery adapter is declared; use a self-hosted broker profile only after its own adapter is implemented."},
		{Name: "search-index", Status: sdk.ResilienceCapabilityExperimental, Actions: restoreActions(), Destinations: destinations(), Strategy: "rebuild-from-self-hosted-workload", RestoreMechanism: "Kapsule-workload-index-rebuild", IntegrityMethod: "document-count-and-application-query", Reason: "Scaleway has no managed search product in this catalog; the MageLift operation translator requires an injected Kapsule/workload projection adapter for self-hosted search rebuilds, and live certification remains open.", Durable: false},
		{Name: "cache", Status: sdk.ResilienceCapabilityExperimental, Actions: restoreActions(), Destinations: destinations(), Strategy: "reconstruct-from-source-of-truth", RestoreMechanism: "cache-warmup-from-durable-source", IntegrityMethod: "application-read-and-cache-health", Reason: resilience.ExperimentalReason("scaleway", "cache"), Durable: false},
		{Name: "audit-evidence", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "append-only-evidence-export", BackupMechanism: "encrypted-versioned-object-export", RestoreMechanism: "isolated-evidence-prefix-restore", IntegrityMethod: "record-digest-chain", RetentionPolicy: "evidence-archive-retention", EncryptionBoundary: "encrypted-evidence-archive", ProtectionMechanism: "append-only-owned-evidence-prefix", Reason: resilience.NativeTranslatorReason("scaleway", "audit-evidence"), Durable: true},
	})
}

func New(client sdk.ResilienceOperationClient, policy sdk.ResilienceOperationPolicy) (sdk.ResilienceAdapter, error) {
	return resilience.New(Descriptor(), client, policy)
}

// NewResilienceClient wraps a Scaleway SDK/API translator at the provider
// edge. Managed Database, Object Storage, and Secret Manager types remain
// owned by the provider implementation.
func NewResilienceClient(backend provider.NativeOperationBackend) (sdk.ResilienceOperationClient, error) {
	return provider.NewNormalizedOperationClient(backend)
}

// NewNativeResilienceClient adds the Scaleway operation-family translator in
// front of an official-SDK wrapper. Managed Database, Object Storage,
// Secret Manager, and Kapsule/workload APIs remain provider-owned.
func NewNativeResilienceClient(api resilience.NativeOperationAPI) (sdk.ResilienceOperationClient, error) {
	backend, err := resilience.NewMappedOperationBackend(sdk.ProviderID("scaleway"), api, scalewayOperationName)
	if err != nil {
		return nil, err
	}
	return provider.NewNormalizedOperationClient(backend)
}

func scalewayOperationName(request sdk.ResilienceOperationRequest) (string, error) {
	prefix := "scaleway.recovery"
	if len(request.DataClasses) == 1 {
		switch request.DataClasses[0] {
		case "database":
			prefix = "scaleway.managed-database"
		case "media", "infrastructure-state", "audit-evidence":
			prefix = "scaleway.object-storage"
		case "configuration-secrets":
			prefix = "scaleway.secret-manager"
		case "queue":
			prefix = "scaleway.queue"
		case "search-index":
			prefix = "scaleway.search"
		case "cache":
			prefix = "scaleway.redis"
		}
	}
	return resilience.OperationName(prefix, request.Action, request.DataClasses)
}

// NewFromFactory validates the exact Scaleway target identity before
// constructing a provider-owned SDK client.
func NewFromFactory(ctx context.Context, factory provider.ResilienceClientFactory, request provider.ClientRequest, policy sdk.ResilienceOperationPolicy) (sdk.ResilienceAdapter, error) {
	if err := provider.ValidateClientRequestForProvider(request, sdk.ProviderID("scaleway")); err != nil {
		return nil, err
	}
	client, err := provider.ConstructResilienceClient(ctx, factory, request)
	if err != nil {
		return nil, err
	}
	return New(client, policy)
}

// NewLifecycleFactories adapts an injected Scaleway SDK operation client to
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
