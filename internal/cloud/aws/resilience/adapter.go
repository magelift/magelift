// Package resilience exposes the AWS recovery lifecycle boundary.
//
// AWS-specific clients translate these semantic operations to RDS/Aurora,
// S3, Secrets Manager, SQS or the selected self-hosted service APIs. The
// portable ordering, polling, ownership, and evidence rules live in sdk/v1.
package resilience

import (
	"context"

	"github.com/magelift/magelift/internal/cloud/resilience"
	"github.com/magelift/magelift/internal/platform"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

const AdapterID = "aws.resilience"

func Descriptor() sdk.ResilienceAdapterDescriptor {
	return resilience.Descriptor(sdk.ProviderID("aws"), AdapterID, "1.0.0", []resilience.DataClassSpec{
		{Name: "database", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: databaseDestinations(), Strategy: "managed-backup-and-isolated-restore", BackupMechanism: "RDS-or-Aurora-snapshot-and-point-in-time-log", RestoreMechanism: "managed-instance-or-cluster-restore", IntegrityMethod: "fixture-checksum-and-application-read", RetentionPolicy: "RDS-or-Aurora-backup-retention-and-PITR-window", EncryptionBoundary: "RDS-or-Aurora-KMS-encryption", ProtectionMechanism: "RDS-or-Aurora-deletion-protection-and-backup-scope", Reason: resilience.NativeTranslatorReason("aws", "database"), Durable: true},
		{Name: "media", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "versioned-object-backup-and-manifest-check", BackupMechanism: "S3-versioning-and-replication", RestoreMechanism: "isolated-object-prefix-or-bucket-restore", IntegrityMethod: "manifest-and-content-checksum", RetentionPolicy: "S3-lifecycle-retention-or-object-lock", EncryptionBoundary: "S3-SSE-KMS-or-SSE-S3-by-policy", ProtectionMechanism: "S3-versioning-and-object-lock-where-profiled", Reason: resilience.NativeTranslatorReason("aws", "media"), Durable: true},
		{Name: "configuration-secrets", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "versioned-secret-and-config-export", BackupMechanism: "Secrets-Manager-versions-and-encrypted-config-archive", RestoreMechanism: "scoped-secret-version-restore", IntegrityMethod: "reference-and-permission-check", RetentionPolicy: "encrypted-config-archive-retention", EncryptionBoundary: "Secrets-Manager-KMS-and-archive-KMS", ProtectionMechanism: "version-pinning-and-owned-archive-retention", Reason: resilience.NativeTranslatorReason("aws", "configuration-secrets"), Durable: true},
		{Name: "infrastructure-state", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "immutable-state-backup", BackupMechanism: "versioned-encrypted-S3-state-archive", RestoreMechanism: "isolated-state-prefix-restore", IntegrityMethod: "archive-manifest-checksum", RetentionPolicy: "state-archive-retention-or-object-lock", EncryptionBoundary: "S3-state-archive-SSE-KMS", ProtectionMechanism: "versioned-state-archive-and-delete-protection", Reason: resilience.NativeTranslatorReason("aws", "infrastructure-state"), Durable: true},
		{Name: "queue", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: []sdk.RecoveryDestination{sdk.RecoverySameRegionIsolated}, Strategy: "quiesced-bounded-SQS-export-and-isolated-replay", BackupMechanism: "ownership-tagged-SQS-export-queue-with-source-preservation", RestoreMechanism: "ownership-scoped-same-region-isolated-message-replay", IntegrityMethod: "known-message-count-and-application-consumer-read", RetentionPolicy: "SQS-message-retention-up-to-fourteen-days", EncryptionBoundary: "SQS-SSE-SQS-or-SSE-KMS", ProtectionMechanism: "ownership-tagged-export-and-restore-queues-with-inventory-cleanup", Reason: "The AWS SQS operation translator implements a bounded standard-queue export and isolated replay through the official SDK. It requires operator-approved quiescence, rejects in-flight or delayed source messages, preserves the source by resetting visibility, checks every SendMessageBatch partial failure, and refuses FIFO, in-place, and alternate-region paths because SQS has no native snapshot, fencing, or cross-region replication primitive; independent live evidence and HA/DR exercises are still required.", Durable: true, ProtectionOptional: true},
		{Name: "search-index", Status: sdk.ResilienceCapabilityExperimental, Actions: restoreActions(), Destinations: destinations(), Strategy: "rebuild-from-durable-source", RestoreMechanism: "isolated-index-rebuild", IntegrityMethod: "document-count-and-application-query", Reason: resilience.ExperimentalReason("aws", "search-index"), Durable: false},
		{Name: "cache", Status: sdk.ResilienceCapabilityExperimental, Actions: restoreActions(), Destinations: destinations(), Strategy: "reconstruct-from-source-of-truth", RestoreMechanism: "cache-warmup-from-durable-source", IntegrityMethod: "application-read-and-cache-health", Reason: resilience.ExperimentalReason("aws", "cache"), Durable: false},
		{Name: "audit-evidence", Status: sdk.ResilienceCapabilityExperimental, Actions: durableActions(), Destinations: destinations(), Strategy: "append-only-evidence-export", BackupMechanism: "encrypted-versioned-object-export", RestoreMechanism: "isolated-evidence-prefix-restore", IntegrityMethod: "record-digest-chain", RetentionPolicy: "evidence-archive-retention-or-object-lock", EncryptionBoundary: "evidence-archive-SSE-KMS", ProtectionMechanism: "append-only-owned-evidence-prefix", Reason: resilience.NativeTranslatorReason("aws", "audit-evidence"), Durable: true},
	})
}

func New(client sdk.ResilienceOperationClient, policy sdk.ResilienceOperationPolicy) (sdk.ResilienceAdapter, error) {
	return resilience.New(Descriptor(), client, policy)
}

// NewResilienceClient wraps an AWS official-SDK translator at the provider
// boundary. The translator owns RDS/S3/Secrets/SQS API models; the returned
// client exposes only the normalized public SDK port to the shared lifecycle.
func NewResilienceClient(backend provider.NativeOperationBackend) (sdk.ResilienceOperationClient, error) {
	return provider.NewNormalizedOperationClient(backend)
}

// NewNativeResilienceClient adds the AWS operation-name translator in front
// of an API wrapper built on the official AWS SDK. The API wrapper owns RDS,
// S3, Secrets Manager, SQS, and workload-specific request models; this
// package owns only the AWS operation-family mapping.
func NewNativeResilienceClient(api resilience.NativeOperationAPI) (sdk.ResilienceOperationClient, error) {
	backend, err := resilience.NewMappedOperationBackend(sdk.ProviderID("aws"), api, awsOperationName)
	if err != nil {
		return nil, err
	}
	return provider.NewNormalizedOperationClient(backend)
}

func awsOperationName(request sdk.ResilienceOperationRequest) (string, error) {
	prefix := "aws.recovery"
	if len(request.DataClasses) == 1 {
		switch request.DataClasses[0] {
		case "database":
			prefix = "aws.rds"
		case "media", "infrastructure-state", "audit-evidence":
			prefix = "aws.s3"
		case "configuration-secrets":
			prefix = "aws.secrets-manager"
		case "queue":
			prefix = "aws.queue"
		case "search-index":
			prefix = "aws.opensearch"
		case "cache":
			prefix = "aws.cache"
		}
	}
	return resilience.OperationName(prefix, request.Action, request.DataClasses)
}

// NewFromFactory validates the exact AWS target identity before asking a
// provider-owned factory to construct its SDK client. No cloud mutation occurs
// during construction.
func NewFromFactory(ctx context.Context, factory provider.ResilienceClientFactory, request provider.ClientRequest, policy sdk.ResilienceOperationPolicy) (sdk.ResilienceAdapter, error) {
	if err := provider.ValidateClientRequestForProvider(request, sdk.ProviderID("aws")); err != nil {
		return nil, err
	}
	client, err := provider.ConstructResilienceClient(ctx, factory, request)
	if err != nil {
		return nil, err
	}
	return New(client, policy)
}

// NewLifecycleFactories adapts a provider SDK operation client to the
// provider-neutral module seam. The client is intentionally injected by the
// caller so credential and region construction stays outside the core.
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

func databaseDestinations() []sdk.RecoveryDestination {
	return []sdk.RecoveryDestination{sdk.RecoverySameRegion, sdk.RecoverySameRegionIsolated}
}

func durableActions() []sdk.ResilienceAction {
	return []sdk.ResilienceAction{sdk.ResilienceBackup, sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck}
}

func restoreActions() []sdk.ResilienceAction {
	return []sdk.ResilienceAction{sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck}
}
