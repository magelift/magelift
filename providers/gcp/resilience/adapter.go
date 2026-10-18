// Package resilience exposes the GCP recovery lifecycle boundary.
//
// GCP-specific clients translate semantic operations to Cloud SQL, Cloud
// Storage, Secret Manager, Pub/Sub, and selected self-hosted service APIs.
package resilience

import (
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/internal/shared/resilience"
	"github.com/magelift/magelift/sdk"
)

const AdapterID = "gcp.resilience"

func Descriptor() sdk.ResilienceAdapterDescriptor {
	return resilience.Descriptor(sdk.ProviderID("gcp"), AdapterID, "1.0.0", []resilience.DataClassSpec{
		{Name: "database", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: databaseDestinations(), Strategy: "managed-backup-isolated-or-approved-in-place-restore", BackupMechanism: "Cloud-SQL-on-demand-backup-and-PITR", RestoreMechanism: "Cloud-SQL-restoreBackup-to-new-instance-or-approved-source-instance", IntegrityMethod: "fixture-checksum-and-application-read", RetentionPolicy: "Cloud-SQL-backup-TTL-and-PITR-window", EncryptionBoundary: "Cloud-SQL-Google-managed-or-CMEK-encryption", ProtectionMechanism: "Cloud-SQL-HA-and-deletion-protection-policy", Reason: "The gcp database operation translator is implemented against the official SDK for isolated and alternate-region new-instance restore and same-region in-place restore onto the source instance after an operator approval reference. Independent live evidence and provider certification are still required.", Durable: true},
		{Name: "media", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "versioned-object-backup-isolated-or-approved-in-place-restore", BackupMechanism: "Cloud-Storage-object-copy-and-sealed-manifest", RestoreMechanism: "isolated-prefix-or-approved-source-prefix-restore", IntegrityMethod: "manifest-size-and-object-validator-check", RetentionPolicy: "Cloud-Storage-retention-policy-or-locked-retention", EncryptionBoundary: "Cloud-Storage-Google-managed-or-CMEK-encryption", ProtectionMechanism: "ownership-and-retention-lock-where-profiled", Reason: "The gcp media operation translator is implemented against the official SDK for isolated prefix restore and same-region in-place restore onto the source prefix after an operator approval reference. Independent live evidence and provider certification are still required.", Durable: true},
		{Name: "configuration-secrets", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "versioned-secret-isolated-or-approved-in-place-restore", BackupMechanism: "Secret-Manager-version-and-sealed-encrypted-archive", RestoreMechanism: "isolated-secret-or-approved-source-version-restore", IntegrityMethod: "reference-and-permission-check", RetentionPolicy: "encrypted-config-archive-retention", EncryptionBoundary: "Secret-Manager-CMEK-or-managed-encryption", ProtectionMechanism: "secret-version-pinning-and-owned-archive-retention", Reason: "The gcp configuration-secrets operation translator is implemented against the official SDK for isolated restore-secret creation and same-region in-place restore by adding a version on the source secret after an operator approval reference. Independent live evidence and provider certification are still required.", Durable: true},
		{Name: "infrastructure-state", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "immutable-state-backup-isolated-or-approved-in-place-restore", BackupMechanism: "versioned-encrypted-Cloud-Storage-state-archive", RestoreMechanism: "isolated-state-prefix-or-approved-source-prefix-restore", IntegrityMethod: "archive-manifest-checksum", RetentionPolicy: "state-archive-retention-or-locked-retention", EncryptionBoundary: "Cloud-Storage-state-archive-CMEK-or-managed-encryption", ProtectionMechanism: "versioned-state-archive-and-retention-lock", Reason: "The gcp infrastructure-state operation translator is implemented against the official SDK for isolated prefix restore and same-region in-place restore onto the source prefix after an operator approval reference. Independent live evidence and provider certification are still required.", Durable: true},
		{Name: "queue", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: []sdk.RecoveryDestination{sdk.RecoverySameRegion, sdk.RecoverySameRegionIsolated}, Strategy: "provider-or-broker-specific-message-recovery", BackupMechanism: "Pub-Sub-snapshot-with-ownership-labels-or-self-hosted-broker-export", RestoreMechanism: "same-region-subscription-seek-or-isolated-subscription-seek", IntegrityMethod: "known-message-count-and-consumer-read", RetentionPolicy: "Pub-Sub-snapshot-retention-up-to-seven-days-or-export-retention", EncryptionBoundary: "Pub-Sub-service-managed-encryption-or-broker-encryption-boundary", ProtectionMechanism: "ownership-labels-and-explicit-snapshot-cleanup", Reason: "The operation translator for the official Pub/Sub snapshot/seek API is implemented for owned same-region source-subscription seek and same-region isolated restore onto a new owned subscription. It fails closed for retention beyond seven days, alternate regions, and unverified CMEK; independent live evidence and HA/DR exercises remain open.", Durable: true, ProtectionOptional: true},
		{Name: "search-index", Status: sdk.ResilienceCapabilityExperimental, Actions: restoreActions(), Destinations: destinations(), Strategy: "rebuild-from-durable-source", RestoreMechanism: "isolated-index-rebuild", IntegrityMethod: "document-count-and-application-query", Reason: resilience.ExperimentalReason("gcp", "search-index"), Durable: false},
		{Name: "cache", Status: sdk.ResilienceCapabilityExperimental, Actions: restoreActions(), Destinations: destinations(), Strategy: "reconstruct-from-source-of-truth", RestoreMechanism: "cache-warmup-from-durable-source", IntegrityMethod: "application-read-and-cache-health", Reason: resilience.ExperimentalReason("gcp", "cache"), Durable: false},
		{Name: "audit-evidence", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "append-only-evidence-export-isolated-or-approved-in-place-restore", BackupMechanism: "encrypted-versioned-object-export", RestoreMechanism: "isolated-evidence-prefix-or-approved-source-prefix-restore", IntegrityMethod: "record-digest-chain", RetentionPolicy: "evidence-archive-retention-or-locked-retention", EncryptionBoundary: "evidence-archive-CMEK-or-managed-encryption", ProtectionMechanism: "append-only-owned-evidence-prefix", Reason: "The gcp audit-evidence operation translator is implemented against the official SDK for isolated prefix restore and same-region in-place restore onto the source prefix after an operator approval reference. Independent live evidence and provider certification are still required.", Durable: true},
	})
}

func New(client sdk.ResilienceOperationClient, policy sdk.ResilienceOperationPolicy) (sdk.ResilienceAdapter, error) {
	return resilience.New(Descriptor(), client, policy)
}

// NewResilienceClient wraps a GCP official-API translator without leaking
// Cloud SQL, Cloud Storage, Secret Manager, or Pub/Sub models into the SDK.
func NewResilienceClient(backend provider.NativeOperationBackend) (sdk.ResilienceOperationClient, error) {
	return provider.NewNormalizedOperationClient(backend)
}

// NewNativeResilienceClient adds the GCP operation-family translator in
// front of an official-SDK wrapper. Cloud SQL, Cloud Storage, Secret Manager,
// Pub/Sub, and workload APIs remain outside this package and return only the
// normalized observation accepted by the shared lifecycle.
func NewNativeResilienceClient(api resilience.NativeOperationAPI) (sdk.ResilienceOperationClient, error) {
	backend, err := resilience.NewMappedOperationBackend(sdk.ProviderID("gcp"), api, gcpOperationName)
	if err != nil {
		return nil, err
	}
	return provider.NewNormalizedOperationClient(backend)
}

func gcpOperationName(request sdk.ResilienceOperationRequest) (string, error) {
	prefix := "gcp.recovery"
	if len(request.DataClasses) == 1 {
		switch request.DataClasses[0] {
		case "database":
			prefix = "gcp.cloud-sql"
		case "media", "infrastructure-state", "audit-evidence":
			prefix = "gcp.cloud-storage"
		case "configuration-secrets":
			prefix = "gcp.secret-manager"
		case "queue":
			prefix = "gcp.pubsub"
		case "search-index":
			prefix = "gcp.opensearch"
		case "cache":
			prefix = "gcp.memorystore"
		}
	}
	return resilience.OperationName(prefix, request.Action, request.DataClasses)
}

func destinations() []sdk.RecoveryDestination {
	return []sdk.RecoveryDestination{sdk.RecoverySameRegion, sdk.RecoverySameRegionIsolated, sdk.RecoveryAlternateRegion}
}

func databaseDestinations() []sdk.RecoveryDestination {
	return []sdk.RecoveryDestination{sdk.RecoverySameRegion, sdk.RecoverySameRegionIsolated, sdk.RecoveryAlternateRegion}
}

func durableActions() []sdk.ResilienceAction {
	return []sdk.ResilienceAction{sdk.ResilienceBackup, sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck}
}

func restoreActions() []sdk.ResilienceAction {
	return []sdk.ResilienceAction{sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck}
}
