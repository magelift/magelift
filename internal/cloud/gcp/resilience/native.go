package resilience

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	gcpstorage "cloud.google.com/go/storage"
	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
	"google.golang.org/api/iterator"
)

const (
	defaultArchivePrefix = "magelift/recovery"
	defaultRetentionDays = 30
	operationPrefix      = "gcp-recovery:v1:"
	ownershipMetadataKey = "magelift-ownership"
	classMetadataKey     = "magelift-data-class"
	fixtureMetadataKey   = "magelift-fixture"
	manifestMetadataKey  = "magelift-manifest"
	ownershipLabelKey    = "magelift_ownership"
	classLabelKey        = "magelift_data_class"
	fixtureLabelKey      = "magelift_fixture"
	recoveryRoleLabelKey = "magelift_recovery_role"
	isolatedRestoreRole  = "isolated-restore"
)

type GCSObject struct {
	Key  string
	Size int64
	ETag string
}

type GCSObjectMetadata struct {
	Size          int64
	ETag          string
	Metadata      map[string]string
	KMSKeyName    string
	RetentionMode string
}

type GCSAPI interface {
	List(context.Context, string, string) ([]GCSObject, error)
	Head(context.Context, string, string) (GCSObjectMetadata, error)
	Read(context.Context, string, string) ([]byte, error)
	Put(context.Context, string, string, []byte, map[string]string, string) error
	Copy(context.Context, string, string, string, string, map[string]string, string) error
	Delete(context.Context, string, string) error
}

type GCSRetentionAPI interface {
	PutWithRetention(context.Context, string, string, []byte, map[string]string, string, time.Time) error
	CopyWithRetention(context.Context, string, string, string, string, map[string]string, string, time.Time) error
}

type SecretValue struct {
	Data []byte
}

type SecretMetadata struct {
	Name   string
	Labels map[string]string
}

type SecretAPI interface {
	Get(context.Context, string) (SecretValue, error)
	Describe(context.Context, string) (SecretMetadata, error)
	List(context.Context, string) ([]SecretMetadata, error)
	Create(context.Context, string, string, []byte, map[string]string) error
	AddVersion(context.Context, string, []byte) error
	Delete(context.Context, string) error
}

type RecoveryVerification struct {
	ManifestVerified         bool
	CountsVerified           bool
	ApplicationReadsVerified bool
	PermissionsVerified      bool
	SecretReferencesVerified bool
	ServiceHealthVerified    bool
}

type RecoveryVerifier interface {
	Verify(context.Context, RecoveryVerificationRequest) (RecoveryVerification, error)
}

// RecoveryFixtureReadiness is an optional provider-local precondition for
// recovery operations whose native snapshot API captures an asynchronous
// delivery state. Implementations should observe the known fixture and leave
// it available for the native snapshot operation; the shared recovery
// contract remains expressed by RecoveryVerifier.
type RecoveryFixtureReadiness interface {
	WaitForFixture(context.Context, RecoveryVerificationRequest) error
}

type RecoveryVerificationRequest struct {
	DataClass       string
	Resource        string
	FixtureID       string
	OwnershipMarker string
}

type NativeAPIConfig struct {
	ArchiveBucket          string
	ArchivePrefix          string
	RestoreBucket          string
	RestorePrefix          string
	RestoreSecretPrefix    string
	RestoreProject         string
	RestoreInstancePrefix  string
	RestoreTier            string
	RestoreRegion          string
	RequireRegionalHA      bool
	RequireDeletionProtect bool
	Project                string
	KMSKeyName             string
	RequireCMEK            bool
	RequireLockedRetention bool
	RetentionDays          int
	Verifier               RecoveryVerifier
	Projection             *cloudrecovery.ProjectionLifecycle
	Now                    func() time.Time
}

type NativeAPI struct {
	storage GCSAPI
	secrets SecretAPI
	sql     CloudSQLAPI
	pubsub  PubSubAPI
	config  NativeAPIConfig
	close   func() error
}

var _ cloudresilience.NativeOperationAPI = (*NativeAPI)(nil)

func NewNativeAPI(storage GCSAPI, secrets SecretAPI, config NativeAPIConfig) (*NativeAPI, error) {
	return NewNativeAPIWithSQL(storage, secrets, nil, config)
}

func NewNativeAPIWithSQL(storage GCSAPI, secrets SecretAPI, sql CloudSQLAPI, config NativeAPIConfig) (*NativeAPI, error) {
	return NewNativeAPIWithSQLAndProjectionAndPubSub(storage, secrets, sql, nil, config, config.Projection)
}

// NewNativeAPIWithSQLAndProjection constructs the GCP recovery translator
// with an optional workload projection adapter for search rebuilds and cache
// reconstruction. The adapter is injected because ECS/Kubernetes execution
// identities are runtime-specific and must not enter the portable core.
func NewNativeAPIWithSQLAndProjection(storage GCSAPI, secrets SecretAPI, sql CloudSQLAPI, config NativeAPIConfig, projection *cloudrecovery.ProjectionLifecycle) (*NativeAPI, error) {
	return NewNativeAPIWithSQLAndProjectionAndPubSub(storage, secrets, sql, nil, config, projection)
}

// NewNativeAPIWithSQLAndProjectionAndPubSub constructs the GCP recovery
// translator with the optional official Pub/Sub snapshot/seek boundary.
// Existing object, secret, SQL, and projection callers remain valid through
// the narrower constructors above.
func NewNativeAPIWithSQLAndProjectionAndPubSub(storage GCSAPI, secrets SecretAPI, sql CloudSQLAPI, pubsub PubSubAPI, config NativeAPIConfig, projection *cloudrecovery.ProjectionLifecycle) (*NativeAPI, error) {
	if storage != nil && strings.TrimSpace(config.ArchiveBucket) == "" {
		return nil, errors.New("GCP recovery archive bucket is required")
	}
	if config.RequireCMEK && strings.TrimSpace(config.KMSKeyName) == "" {
		return nil, errors.New("GCP recovery KMS key is required when CMEK is required")
	}
	if config.Now != nil && config.Now().IsZero() {
		return nil, errors.New("GCP recovery clock must return a non-zero time")
	}
	if strings.TrimSpace(config.RestoreBucket) == "" {
		config.RestoreBucket = config.ArchiveBucket
	}
	if strings.TrimSpace(config.ArchivePrefix) == "" {
		config.ArchivePrefix = defaultArchivePrefix
	}
	if strings.TrimSpace(config.RestorePrefix) == "" {
		config.RestorePrefix = strings.Trim(config.ArchivePrefix, "/") + "/restored"
	}
	if strings.TrimSpace(config.RestoreSecretPrefix) == "" {
		config.RestoreSecretPrefix = "magelift-recovery"
	}
	if config.RetentionDays <= 0 {
		config.RetentionDays = defaultRetentionDays
	}
	config.Projection = projection
	return &NativeAPI{storage: storage, secrets: secrets, sql: sql, pubsub: pubsub, config: config}, nil
}

func NewGCPNativeAPI(ctx context.Context, config NativeAPIConfig) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("GCP recovery context is required")
	}
	storageClient, err := gcpstorage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCP Cloud Storage client: %w", err)
	}
	secretClient, err := secretmanager.NewClient(ctx)
	if err != nil {
		_ = storageClient.Close()
		return nil, fmt.Errorf("create GCP Secret Manager client: %w", err)
	}
	sqlClient, err := newCloudSQLSDK(ctx)
	if err != nil {
		_ = storageClient.Close()
		_ = secretClient.Close()
		return nil, fmt.Errorf("create GCP Cloud SQL client: %w", err)
	}
	pubsubClient, err := newPubSubSDK(ctx)
	if err != nil {
		_ = storageClient.Close()
		_ = secretClient.Close()
		return nil, fmt.Errorf("create GCP Pub/Sub client: %w", err)
	}
	api, err := NewNativeAPIWithSQLAndProjectionAndPubSub(gcsSDK{client: storageClient}, gcpSecretSDK{client: secretClient}, sqlClient, pubsubClient, config, config.Projection)
	if err != nil {
		_ = storageClient.Close()
		_ = secretClient.Close()
		_ = pubsubClient.Close()
		return nil, err
	}
	api.close = func() error {
		return errors.Join(storageClient.Close(), secretClient.Close(), pubsubClient.Close())
	}
	return api, nil
}

// NewGCPStorageNativeAPI constructs only the official Cloud Storage client
// needed by object recovery. Focused acceptance cells and community adapters
// use this narrow constructor so they do not initialize unrelated Secret
// Manager, Cloud SQL, or Pub/Sub clients.
func NewGCPStorageNativeAPI(ctx context.Context, config NativeAPIConfig) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("GCP recovery context is required")
	}
	storageClient, err := gcpstorage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCP Cloud Storage client: %w", err)
	}
	api, err := NewNativeAPI(gcsSDK{client: storageClient}, nil, config)
	if err != nil {
		_ = storageClient.Close()
		return nil, err
	}
	api.close = storageClient.Close
	return api, nil
}

// NewGCPPubSubNativeAPI constructs only the Pub/Sub client needed by queue
// recovery. It avoids creating unrelated Cloud Storage, Secret Manager, and
// Cloud SQL clients for a focused acceptance cell or community adapter.
func NewGCPPubSubNativeAPI(ctx context.Context, config NativeAPIConfig) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("GCP recovery context is required")
	}
	pubsubClient, err := newPubSubSDK(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCP Pub/Sub client: %w", err)
	}
	api, err := NewNativeAPIWithSQLAndProjectionAndPubSub(nil, nil, nil, pubsubClient, config, config.Projection)
	if err != nil {
		_ = pubsubClient.Close()
		return nil, err
	}
	api.close = pubsubClient.Close
	return api, nil
}

// NewGCPCloudSQLNativeAPI constructs only the official Cloud SQL client needed
// by a focused backup/restore acceptance cell. It avoids creating unrelated
// Cloud Storage, Secret Manager, and Pub/Sub clients, which keeps the test
// boundary explicit and reduces credential scopes and setup time.
func NewGCPCloudSQLNativeAPI(ctx context.Context, config NativeAPIConfig) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("GCP recovery context is required")
	}
	sqlClient, err := newCloudSQLSDK(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCP Cloud SQL client: %w", err)
	}
	return NewNativeAPIWithSQLAndProjectionAndPubSub(nil, nil, sqlClient, nil, config, config.Projection)
}

// NewGCPSecretNativeAPI constructs only the official Cloud Storage and Secret
// Manager clients required by a focused secret-recovery cell. It avoids
// initializing Cloud SQL and Pub/Sub clients that the cell cannot use.
func NewGCPSecretNativeAPI(ctx context.Context, config NativeAPIConfig) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("GCP secret recovery context is required")
	}
	storageClient, err := gcpstorage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCP Cloud Storage client for secrets: %w", err)
	}
	secretClient, err := secretmanager.NewClient(ctx)
	if err != nil {
		_ = storageClient.Close()
		return nil, fmt.Errorf("create GCP Secret Manager client: %w", err)
	}
	api, err := NewNativeAPI(gcsSDK{client: storageClient}, gcpSecretSDK{client: secretClient}, config)
	if err != nil {
		_ = storageClient.Close()
		_ = secretClient.Close()
		return nil, err
	}
	api.close = func() error { return errors.Join(storageClient.Close(), secretClient.Close()) }
	return api, nil
}

// Close releases provider SDK clients owned by a native API constructor.
func (api *NativeAPI) Close() error {
	if api == nil || api.close == nil {
		return nil
	}
	return api.close()
}

func (api *NativeAPI) Start(ctx context.Context, request cloudresilience.NativeOperationRequest) (provider.NativeOperationObservation, error) {
	if err := validateGCPContext(ctx); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if request.Provider != sdk.ProviderID("gcp") {
		return provider.NativeOperationObservation{}, fmt.Errorf("GCP recovery received provider %q", request.Provider)
	}
	if err := refuseUnimplementedRuntimeActions(request.Request); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if len(request.Request.DataClasses) != 1 {
		return provider.NativeOperationObservation{}, errors.New("GCP recovery requires exactly one data class per operation")
	}
	expected, err := gcpOperationName(request.Request)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if request.Operation != expected {
		return provider.NativeOperationObservation{}, fmt.Errorf("GCP recovery operation %q does not match %q", request.Operation, expected)
	}
	if err := validateGCPRequest(request.Request); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	state := cloudrecovery.OperationState{
		Version: cloudrecovery.OperationVersion, Action: request.Request.Action, DataClass: request.Request.DataClasses[0],
		Resource: request.Request.ResourceReferences[request.Request.DataClasses[0]], Backup: request.Request.BackupReferences[request.Request.DataClasses[0]],
		Destination: request.Request.Destination, FixtureID: request.Request.FixtureID, OwnershipMarker: request.Request.OwnershipMarker, IdempotencyKey: request.Request.IdempotencyKey, ApprovalReference: request.Request.ApprovalReference,
	}
	if strings.TrimSpace(state.Resource) == "" {
		return provider.NativeOperationObservation{}, fmt.Errorf("GCP recovery resource reference for %q is required", state.DataClass)
	}
	operationID, err := cloudrecovery.EncodeOperationID(operationPrefix, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.DataClass {
	case "media", "infrastructure-state", "audit-evidence":
		return api.startObjects(ctx, state, operationID)
	case "configuration-secrets":
		return api.startSecret(ctx, state, operationID)
	case "database":
		return api.startDatabase(ctx, state, operationID)
	case "search-index", "cache":
		return api.startProjectionClass(ctx, state, operationID)
	case "queue":
		return api.startQueue(ctx, state, operationID)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP recovery data class is not registered")
	}
}

func (api *NativeAPI) Poll(ctx context.Context, operationID string) (provider.NativeOperationObservation, error) {
	if err := validateGCPContext(ctx); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	state, err := cloudrecovery.DecodeOperationID(operationPrefix, operationID)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.DataClass {
	case "media", "infrastructure-state", "audit-evidence":
		return api.pollObjects(ctx, state, operationID)
	case "configuration-secrets":
		return api.pollSecret(ctx, state, operationID)
	case "database":
		return api.pollDatabase(ctx, state, operationID)
	case "search-index", "cache":
		return api.pollProjectionClass(ctx, state, operationID)
	case "queue":
		return api.pollQueue(ctx, state, operationID)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "GCP recovery data class is not registered")
	}
}

func (api *NativeAPI) Inventory(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	if err := validateGCPContext(ctx); err != nil {
		return nil, err
	}
	resources := make([]provider.InventoryResource, 0)
	if api.storage != nil {
		if strings.TrimSpace(api.config.ArchiveBucket) == "" {
			return nil, errors.New("GCP Cloud Storage archive bucket is required for inventory")
		}
		objects, err := api.storage.List(ctx, api.config.ArchiveBucket, api.archivePrefixForMarker(marker))
		if err != nil {
			return nil, fmt.Errorf("inventory GCP recovery archive: %w", err)
		}
		for _, object := range objects {
			resources = append(resources, provider.InventoryResource{Identity: "gcp-storage://" + api.config.ArchiveBucket + "/" + object.Key, Owned: true, Live: true})
		}
	}
	if api.sql != nil {
		project := firstNonEmpty(api.config.Project, api.config.RestoreProject)
		if project != "" {
			backups, err := api.sql.ListBackups(ctx, project)
			if err != nil {
				return nil, fmt.Errorf("inventory GCP Cloud SQL backups: %w", err)
			}
			markerPrefix := "magelift-recovery:" + shortDigest(marker) + ":"
			for _, backup := range backups {
				if strings.HasPrefix(backup.Description, markerPrefix) {
					resources = append(resources, provider.InventoryResource{Identity: "gcp-cloud-sql-backup://" + backup.Name, Owned: true, Live: true})
				}
			}
			instances, err := api.sql.ListInstances(ctx, project)
			if err != nil {
				return nil, fmt.Errorf("inventory GCP Cloud SQL instances: %w", err)
			}
			for _, instance := range instances {
				if instance.UserLabels[ownershipLabelKey] == marker {
					resources = append(resources, provider.InventoryResource{Identity: "gcp-cloud-sql://projects/" + instance.Project + "/instances/" + instance.Name, Owned: true, Live: true})
				}
			}
		}
	}
	if api.pubsub != nil && strings.TrimSpace(api.config.Project) != "" {
		snapshots, err := api.pubsub.ListSnapshots(ctx, api.config.Project)
		if err != nil {
			return nil, fmt.Errorf("inventory GCP Pub/Sub snapshots: %w", err)
		}
		for _, snapshot := range sortedSnapshots(snapshots) {
			if snapshot.Labels[ownershipLabelKey] == marker && snapshot.Labels[classLabelKey] == "queue" {
				resources = append(resources, provider.InventoryResource{Identity: "gcp-pubsub-snapshot://" + snapshot.Name, Owned: true, Live: true})
			}
		}
		subscriptions, err := api.pubsub.ListSubscriptions(ctx, api.config.Project)
		if err != nil {
			return nil, fmt.Errorf("inventory GCP Pub/Sub subscriptions: %w", err)
		}
		for _, subscription := range subscriptions {
			if isolatedRestoreSubscriptionOwned(subscription.Labels, marker) {
				resources = append(resources, provider.InventoryResource{Identity: "gcp-pubsub://" + subscription.Name, Owned: true, Live: true})
			}
		}
	}
	if api.secrets != nil {
		project := firstNonEmpty(api.config.Project, api.config.RestoreProject)
		if project != "" {
			secrets, err := api.secrets.List(ctx, project)
			if err != nil {
				return nil, fmt.Errorf("inventory GCP Secret Manager recovery secrets: %w", err)
			}
			prefix := strings.TrimSpace(api.config.RestoreSecretPrefix)
			for _, secret := range secrets {
				if strings.HasPrefix(secret.Name, "projects/"+project+"/secrets/") && strings.HasPrefix(strings.TrimPrefix(secret.Name, "projects/"+project+"/secrets/"), prefix) && secretManagerLabelsMatch(secret.Labels, marker, "configuration-secrets") {
					resources = append(resources, provider.InventoryResource{Identity: "gcp-secret-manager://" + secret.Name, Owned: true, Live: true})
				}
			}
		}
	}
	if api.storage == nil && api.sql == nil && api.pubsub == nil && api.secrets == nil {
		return nil, errors.New("GCP recovery API is required for inventory")
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func validateGCPContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("GCP recovery context is required")
	}
	return ctx.Err()
}

func validateGCPRequest(request sdk.ResilienceOperationRequest) error {
	if request.Action == "" || strings.TrimSpace(request.FixtureID) == "" || strings.TrimSpace(request.OwnershipMarker) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		return errors.New("GCP recovery action, fixture, ownership marker, and idempotency key are required")
	}
	if strings.ContainsAny(request.FixtureID+request.OwnershipMarker+request.IdempotencyKey, "\r\n\x00") {
		return errors.New("GCP recovery identities must be single-line")
	}
	return nil
}

func capabilityError(state cloudrecovery.OperationState, reason string) error {
	return sdk.ResilienceCapabilityError{AdapterID: AdapterID, Operation: state.Action, DataClass: state.DataClass, Status: sdk.ResilienceCapabilityUnsupported, Reason: reason}
}

func refuseUnimplementedRuntimeActions(request sdk.ResilienceOperationRequest) error {
	switch request.Action {
	case sdk.ResilienceFence, sdk.ResilienceFailover, sdk.ResilienceFailback:
		dataClass := ""
		if len(request.DataClasses) == 1 {
			dataClass = request.DataClasses[0]
		}
		return sdk.ResilienceCapabilityError{
			AdapterID: AdapterID, Operation: request.Action, DataClass: dataClass,
			Status: sdk.ResilienceCapabilityUnsupported,
			Reason: "GCP adapter has not opted into writer fencing, traffic failover, or controlled failback; isolated same-region restore remains available without a second writer",
		}
	default:
		return nil
	}
}

func (api *NativeAPI) archivePrefixForMarker(marker string) string {
	return strings.Trim(api.config.ArchivePrefix, "/") + "/" + shortDigest(marker)
}

func (api *NativeAPI) objectPrefixForOperation(state cloudrecovery.OperationState) string {
	return api.archivePrefixForMarker(state.OwnershipMarker) + "/" + state.DataClass + "/" + shortDigest(state.IdempotencyKey)
}

func shortDigest(value string) string {
	return cloudrecovery.Digest([]byte(value))[:24]
}

func metadataFor(state cloudrecovery.OperationState, manifest bool) map[string]string {
	metadata := map[string]string{ownershipMetadataKey: state.OwnershipMarker, classMetadataKey: state.DataClass, fixtureMetadataKey: state.FixtureID}
	if manifest {
		metadata[manifestMetadataKey] = "true"
	}
	return metadata
}

func metadataMatches(metadata map[string]string, state cloudrecovery.OperationState) bool {
	return metadata[ownershipMetadataKey] == state.OwnershipMarker && metadata[classMetadataKey] == state.DataClass && metadata[fixtureMetadataKey] == state.FixtureID
}

func (api *NativeAPI) metadataMatches(metadata GCSObjectMetadata, state cloudrecovery.OperationState) bool {
	return metadataMatches(metadata.Metadata, state)
}

func (api *NativeAPI) encrypted(metadata GCSObjectMetadata) bool {
	if !api.config.RequireCMEK {
		return true
	}
	return metadata.KMSKeyName == api.config.KMSKeyName
}

func (api *NativeAPI) retentionDays() int {
	if api.config.RetentionDays > 0 {
		return api.config.RetentionDays
	}
	return defaultRetentionDays
}

func observation(state cloudrecovery.OperationState, operationID, status string, refs, proofRefs []string, evidence []sdk.ResilienceProofEvidence) provider.NativeOperationObservation {
	return provider.NativeOperationObservation{Status: status, Action: state.Action, OperationID: operationID, ResourceRefs: append([]string(nil), refs...), ProofRefs: append([]string(nil), proofRefs...), OwnershipMarker: state.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true, Evidence: append([]sdk.ResilienceProofEvidence(nil), evidence...)}
}

func pending(state cloudrecovery.OperationState, operationID string) provider.NativeOperationObservation {
	return observation(state, operationID, string(sdk.ResilienceOperationPending), nil, nil, nil)
}

type gcsSDK struct{ client *gcpstorage.Client }

func (s gcsSDK) List(ctx context.Context, bucket, prefix string) ([]GCSObject, error) {
	it := s.client.Bucket(bucket).Objects(ctx, &gcpstorage.Query{Prefix: prefix})
	objects := make([]GCSObject, 0)
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}
		if attrs != nil && attrs.Name != "" {
			objects = append(objects, GCSObject{Key: attrs.Name, Size: attrs.Size, ETag: attrs.Etag})
		}
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	return objects, nil
}

func (s gcsSDK) Head(ctx context.Context, bucket, key string) (GCSObjectMetadata, error) {
	attrs, err := s.client.Bucket(bucket).Object(key).Attrs(ctx)
	if err != nil {
		return GCSObjectMetadata{}, err
	}
	retentionMode := ""
	if attrs.Retention != nil {
		retentionMode = attrs.Retention.Mode
	}
	return GCSObjectMetadata{Size: attrs.Size, ETag: attrs.Etag, Metadata: cloneMetadata(attrs.Metadata), KMSKeyName: attrs.KMSKeyName, RetentionMode: retentionMode}, nil
}

func (s gcsSDK) Read(ctx context.Context, bucket, key string) ([]byte, error) {
	reader, err := s.client.Bucket(bucket).Object(key).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (s gcsSDK) Delete(ctx context.Context, bucket, key string) error {
	return s.client.Bucket(bucket).Object(key).Delete(ctx)
}

func (s gcsSDK) Put(ctx context.Context, bucket, key string, body []byte, metadata map[string]string, kmsKey string) error {
	return s.put(ctx, bucket, key, body, metadata, kmsKey, time.Time{})
}

func (s gcsSDK) PutWithRetention(ctx context.Context, bucket, key string, body []byte, metadata map[string]string, kmsKey string, retainUntil time.Time) error {
	return s.put(ctx, bucket, key, body, metadata, kmsKey, retainUntil)
}

func (s gcsSDK) put(ctx context.Context, bucket, key string, body []byte, metadata map[string]string, kmsKey string, retainUntil time.Time) error {
	writer := s.client.Bucket(bucket).Object(key).NewWriter(ctx)
	writer.Metadata = cloneMetadata(metadata)
	writer.KMSKeyName = kmsKey
	if !retainUntil.IsZero() {
		writer.Retention = &gcpstorage.ObjectRetention{Mode: "Locked", RetainUntil: retainUntil}
	}
	if _, err := io.Copy(writer, bytes.NewReader(body)); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

func (s gcsSDK) Copy(ctx context.Context, sourceBucket, sourceKey, targetBucket, targetKey string, metadata map[string]string, kmsKey string) error {
	return s.copy(ctx, sourceBucket, sourceKey, targetBucket, targetKey, metadata, kmsKey, time.Time{})
}

func (s gcsSDK) CopyWithRetention(ctx context.Context, sourceBucket, sourceKey, targetBucket, targetKey string, metadata map[string]string, kmsKey string, retainUntil time.Time) error {
	return s.copy(ctx, sourceBucket, sourceKey, targetBucket, targetKey, metadata, kmsKey, retainUntil)
}

func (s gcsSDK) copy(ctx context.Context, sourceBucket, sourceKey, targetBucket, targetKey string, metadata map[string]string, kmsKey string, retainUntil time.Time) error {
	destination := s.client.Bucket(targetBucket).Object(targetKey)
	copier := destination.CopierFrom(s.client.Bucket(sourceBucket).Object(sourceKey))
	copier.Metadata = cloneMetadata(metadata)
	copier.DestinationKMSKeyName = kmsKey
	if !retainUntil.IsZero() {
		copier.Retention = &gcpstorage.ObjectRetention{Mode: "Locked", RetainUntil: retainUntil}
	}
	_, err := copier.Run(ctx)
	return err
}

type gcpSecretSDK struct{ client *secretmanager.Client }

func (s gcpSecretSDK) Get(ctx context.Context, name string) (SecretValue, error) {
	versionName := name
	if !strings.Contains(name, "/versions/") {
		versionName += "/versions/latest"
	}
	response, err := s.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{Name: versionName})
	if err != nil {
		return SecretValue{}, err
	}
	if response == nil || response.Payload == nil || len(response.Payload.Data) == 0 {
		return SecretValue{}, errors.New("GCP secret has no value")
	}
	return SecretValue{Data: append([]byte(nil), response.Payload.Data...)}, nil
}

func (s gcpSecretSDK) Describe(ctx context.Context, name string) (SecretMetadata, error) {
	response, err := s.client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{Name: name})
	if err != nil {
		return SecretMetadata{}, err
	}
	if response == nil {
		return SecretMetadata{}, errors.New("GCP secret response is empty")
	}
	return SecretMetadata{Name: response.Name, Labels: cloneMetadata(response.Labels)}, nil
}

func (s gcpSecretSDK) List(ctx context.Context, project string) ([]SecretMetadata, error) {
	it := s.client.ListSecrets(ctx, &secretmanagerpb.ListSecretsRequest{Parent: "projects/" + project})
	secrets := make([]SecretMetadata, 0)
	for {
		response, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return secrets, nil
		}
		if err != nil {
			return nil, err
		}
		if response != nil {
			secrets = append(secrets, SecretMetadata{Name: response.Name, Labels: cloneMetadata(response.Labels)})
		}
	}
}

func (s gcpSecretSDK) Create(ctx context.Context, project, secretID string, value []byte, labels map[string]string) error {
	parent := "projects/" + project
	secret, err := s.client.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{Parent: parent, SecretId: secretID, Secret: &secretmanagerpb.Secret{Labels: cloneMetadata(labels), Replication: &secretmanagerpb.Replication{Replication: &secretmanagerpb.Replication_Automatic_{Automatic: &secretmanagerpb.Replication_Automatic{}}}}})
	if err != nil {
		return err
	}
	return s.addVersion(ctx, secret.Name, value)
}

func (s gcpSecretSDK) AddVersion(ctx context.Context, name string, value []byte) error {
	return s.addVersion(ctx, name, value)
}

func (s gcpSecretSDK) Delete(ctx context.Context, name string) error {
	return s.client.DeleteSecret(ctx, &secretmanagerpb.DeleteSecretRequest{Name: name})
}

func (s gcpSecretSDK) addVersion(ctx context.Context, name string, value []byte) error {
	_, err := s.client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{Parent: name, Payload: &secretmanagerpb.SecretPayload{Data: value}})
	return err
}

func cloneMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
