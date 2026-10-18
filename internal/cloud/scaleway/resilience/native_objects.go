package resilience

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	cloudresilience "github.com/magelift/magelift/internal/shared/resilience"
	"github.com/magelift/magelift/sdk"
)

const (
	scalewayRecoveryOperationPrefix = "scaleway-recovery:v1:"
	defaultScalewayArchivePrefix    = "magelift/recovery"
	defaultScalewayRetentionDays    = 30
)

type operationState = cloudrecovery.OperationState

// ObjectAPI is the provider-local subset of the official S3 SDK used by
// Scaleway Object Storage. The SDK models do not cross this package boundary.
type ObjectAPI interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	CopyObject(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// NativeAPIConfig contains only Scaleway recovery policy and opaque storage
// identities. Credentials stay in the provider constructor and are never
// persisted in an operation ID or returned observation.
type NativeAPIConfig struct {
	ArchiveBucket           string
	ArchivePrefix           string
	RestoreBucket           string
	RestorePrefix           string
	DatabaseProjectID       string
	DatabaseRegion          string
	RestoreDatabaseNodeType string
	RestoreSecretPrefix     string
	Region                  string
	Endpoint                string
	Credentials             aws.CredentialsProvider
	RequireKMS              bool
	KMSKeyID                string
	RequireObjectLock       bool
	// ObjectLockRetention overrides the day-based retention only for bounded
	// acceptance cells. Production callers should leave it zero and use
	// RetentionDays; a non-zero value must still be a positive duration.
	ObjectLockRetention time.Duration
	RequireDatabaseHA   bool
	Verifier            RecoveryVerifier
	Projection          *cloudrecovery.ProjectionLifecycle
	RetentionDays       int
	Now                 func() time.Time
}

// NativeAPI translates the provider-neutral object recovery lifecycle to
// Scaleway's official S3-compatible Object Storage API. The archive algorithm
// and manifest protocol live in internal/shared/recovery and are reused by all
// providers that expose the S3 protocol.
type NativeAPI struct {
	objects  ObjectAPI
	database DatabaseAPI
	secrets  SecretAPI
	config   NativeAPIConfig
}

var _ cloudresilience.NativeOperationAPI = (*NativeAPI)(nil)

// NewNativeAPI constructs a deterministic translator around an injected
// official SDK client. It is the constructor used by fake-client tests and by
// community implementations that already own credential resolution.
func NewNativeAPI(objects ObjectAPI, config NativeAPIConfig) (*NativeAPI, error) {
	return NewNativeAPIWithServices(objects, nil, nil, config)
}

// NewNativeAPIWithDatabase constructs the same object translator with an
// optional provider-local Managed Database API. Keeping the API optional
// preserves the object-only constructor while allowing one lifecycle client
// to compose multiple Scaleway service boundaries.
func NewNativeAPIWithDatabase(objects ObjectAPI, database DatabaseAPI, config NativeAPIConfig) (*NativeAPI, error) {
	return NewNativeAPIWithServices(objects, database, nil, config)
}

// NewNativeAPIWithServices composes the provider-local service ports into one
// resumable recovery client. Each port is optional so a community provider can
// ship a subset without pretending that an absent native boundary exists.
func NewNativeAPIWithServices(objects ObjectAPI, database DatabaseAPI, secrets SecretAPI, config NativeAPIConfig) (*NativeAPI, error) {
	if objects == nil {
		return nil, errors.New("Scaleway Object Storage API is required")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	return &NativeAPI{objects: objects, database: database, secrets: secrets, config: normalizeScalewayConfig(config)}, nil
}

// NewScalewayNativeAPI constructs the official AWS SDK S3 client against the
// documented Scaleway Object Storage endpoint. The AWS SDK is used only as a
// protocol client; all Scaleway lifecycle semantics remain in this package.
func NewScalewayNativeAPI(ctx context.Context, config NativeAPIConfig) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("Scaleway recovery context is required")
	}
	if strings.TrimSpace(config.Region) == "" {
		return nil, errors.New("Scaleway Object Storage region is required")
	}
	if config.Credentials == nil {
		return nil, errors.New("Scaleway Object Storage credentials are required")
	}
	endpoint, err := scalewayObjectEndpoint(config)
	if err != nil {
		return nil, err
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(config.Region), awsconfig.WithCredentialsProvider(config.Credentials))
	if err != nil {
		return nil, fmt.Errorf("load Scaleway Object Storage SDK configuration: %w", err)
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	return NewNativeAPI(client, config)
}

func (config NativeAPIConfig) validate() error {
	if strings.TrimSpace(config.ArchiveBucket) == "" {
		return errors.New("Scaleway recovery archive bucket is required")
	}
	return config.validatePolicy()
}

func (config NativeAPIConfig) validatePolicy() error {
	if config.RequireKMS && strings.TrimSpace(config.KMSKeyID) == "" {
		return errors.New("Scaleway recovery KMS key is required when KMS encryption is required")
	}
	if config.ObjectLockRetention < 0 {
		return errors.New("Scaleway recovery Object Lock retention cannot be negative")
	}
	if config.Now != nil && config.Now().IsZero() {
		return errors.New("Scaleway recovery clock must return a non-zero time")
	}
	return nil
}

func normalizeScalewayConfig(config NativeAPIConfig) NativeAPIConfig {
	if strings.TrimSpace(config.ArchivePrefix) == "" {
		config.ArchivePrefix = defaultScalewayArchivePrefix
	}
	if strings.TrimSpace(config.RestoreBucket) == "" {
		config.RestoreBucket = config.ArchiveBucket
	}
	if strings.TrimSpace(config.RestorePrefix) == "" {
		config.RestorePrefix = strings.Trim(config.ArchivePrefix, "/") + "/restored"
	}
	if config.RetentionDays <= 0 {
		config.RetentionDays = defaultScalewayRetentionDays
	}
	return config
}

func (api *NativeAPI) Start(ctx context.Context, request cloudresilience.NativeOperationRequest) (provider.NativeOperationObservation, error) {
	if err := validateScalewayContext(ctx); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if request.Provider != sdk.ProviderID("scaleway") {
		return provider.NativeOperationObservation{}, fmt.Errorf("Scaleway recovery received provider %q", request.Provider)
	}
	if len(request.Request.DataClasses) != 1 {
		return provider.NativeOperationObservation{}, errors.New("Scaleway recovery requires exactly one data class per operation")
	}
	expectedOperation, err := scalewayOperationName(request.Request)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if request.Operation != expectedOperation {
		return provider.NativeOperationObservation{}, fmt.Errorf("Scaleway recovery operation %q does not match %q", request.Operation, expectedOperation)
	}
	if err := validateScalewayRequest(request.Request); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	state := operationState{
		Version:           cloudrecovery.OperationVersion,
		Action:            request.Request.Action,
		DataClass:         request.Request.DataClasses[0],
		Resource:          request.Request.ResourceReferences[request.Request.DataClasses[0]],
		Backup:            request.Request.BackupReferences[request.Request.DataClasses[0]],
		Destination:       request.Request.Destination,
		FixtureID:         request.Request.FixtureID,
		OwnershipMarker:   request.Request.OwnershipMarker,
		IdempotencyKey:    request.Request.IdempotencyKey,
		ApprovalReference: request.Request.ApprovalReference,
	}
	if strings.TrimSpace(state.Resource) == "" && state.Action != sdk.ResilienceCleanup {
		return provider.NativeOperationObservation{}, fmt.Errorf("Scaleway recovery resource reference for %q is required", state.DataClass)
	}
	operationID, err := cloudrecovery.EncodeOperationID(scalewayRecoveryOperationPrefix, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.DataClass {
	case "database":
		return api.startDatabase(ctx, state, operationID)
	case "configuration-secrets":
		return api.startSecret(ctx, state, operationID)
	case "search-index", "cache":
		return api.startProjectionClass(ctx, state, operationID)
	case "media", "infrastructure-state", "audit-evidence":
		return api.startObjects(ctx, state, operationID)
	default:
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway native recovery implements Managed Database snapshots, Secret Manager archives, Object Storage durable classes, and cache reconstruction when a Kapsule/workload projection adapter is configured")
	}
}

func (api *NativeAPI) Poll(ctx context.Context, operationID string) (provider.NativeOperationObservation, error) {
	if err := validateScalewayContext(ctx); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	state, err := cloudrecovery.DecodeOperationID(scalewayRecoveryOperationPrefix, operationID)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.DataClass {
	case "database":
		return api.pollDatabase(ctx, state, operationID)
	case "configuration-secrets":
		return api.pollSecret(ctx, state, operationID)
	case "search-index", "cache":
		return api.pollProjectionClass(ctx, state, operationID)
	case "media", "infrastructure-state", "audit-evidence":
		return api.pollObjects(ctx, state, operationID)
	default:
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway native recovery implements Managed Database snapshots, Secret Manager archives, Object Storage durable classes, and cache reconstruction when a Kapsule/workload projection adapter is configured")
	}
}

func (api *NativeAPI) Inventory(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	if err := validateScalewayContext(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("Scaleway recovery inventory ownership marker is required")
	}
	resources := make([]provider.InventoryResource, 0)
	if api.objects != nil {
		store, err := api.objectStore()
		if err != nil {
			return nil, err
		}
		objects, err := store.List(ctx, api.config.ArchiveBucket, api.archivePrefixForMarker(marker))
		if err != nil {
			return nil, fmt.Errorf("inventory Scaleway recovery archive: %w", err)
		}
		resources = make([]provider.InventoryResource, 0, len(objects))
		for _, object := range objects {
			metadata, headErr := store.Head(ctx, api.config.ArchiveBucket, object.Key)
			if headErr != nil {
				if isScalewayNotFound(headErr) {
					continue
				}
				return nil, fmt.Errorf("inspect Scaleway recovery inventory object: %w", headErr)
			}
			if metadata.Metadata[cloudrecovery.OwnershipMetadataKey] != marker || !api.policyVerified(metadata) {
				continue
			}
			resources = append(resources, provider.InventoryResource{Identity: "scaleway-object://" + api.config.ArchiveBucket + "/" + object.Key, Owned: true, Live: true})
		}
	}
	if api.database != nil {
		databaseResources, err := api.inventoryDatabase(ctx, marker)
		if err != nil {
			return nil, err
		}
		resources = append(resources, databaseResources...)
	}
	if api.secrets != nil {
		secretResources, err := api.inventorySecrets(ctx, marker)
		if err != nil {
			return nil, err
		}
		resources = append(resources, secretResources...)
	}
	return resources, nil
}

func (api *NativeAPI) startObjects(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupObjectsForState(ctx, state, operationID)
	}
	if _, _, err := parseScalewayObjectReference(state.Resource); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	engine, err := api.objectEngine()
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	scope, err := scalewayObjectScope(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		manifest, backupErr := engine.Backup(ctx, scope)
		if backupErr != nil {
			return provider.NativeOperationObservation{}, backupErr
		}
		return scalewayObjectBackupObservation(api, state, operationID, manifest), nil
	case sdk.ResilienceRestore:
		return api.restoreObjects(ctx, state, operationID, engine, scope)
	case sdk.ResilienceIntegrityCheck:
		return api.integrityObjects(ctx, state, operationID, engine, scope)
	default:
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway Object Storage recovery does not implement this action")
	}
}

func (api *NativeAPI) pollObjects(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupObjectsForState(ctx, state, operationID)
	}
	engine, err := api.objectEngine()
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	scope, err := scalewayObjectScope(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	manifestBucket, manifestKey, parseErr := parseScalewayObjectReference(state.Backup)
	if state.Action == sdk.ResilienceBackup {
		manifestBucket = api.config.ArchiveBucket
		manifestKey = api.objectPrefixForOperation(state) + "/manifest.json"
	}
	if parseErr != nil && state.Action != sdk.ResilienceBackup {
		return provider.NativeOperationObservation{}, parseErr
	}
	manifest, err := engine.ReadManifest(ctx, scope, manifestBucket, manifestKey)
	if err != nil {
		if isScalewayNotFound(err) {
			return scalewayPending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return scalewayObjectBackupObservation(api, state, operationID, manifest), nil
	case sdk.ResilienceRestore:
		targetPrefix, verifyErr := engine.VerifyRestore(ctx, scope, manifest)
		if errors.Is(verifyErr, cloudrecovery.ErrObjectNotReady) {
			return scalewayPending(state, operationID), nil
		}
		if verifyErr != nil {
			return provider.NativeOperationObservation{}, verifyErr
		}
		return scalewayObjectRestoreObservation(api, state, operationID, manifest, targetPrefix), nil
	case sdk.ResilienceIntegrityCheck:
		if err := engine.Integrity(ctx, scope, manifest); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return scalewayObjectIntegrityObservation(api, state, operationID, manifest), nil
	default:
		return provider.NativeOperationObservation{}, scalewayCapabilityError(state, "Scaleway Object Storage recovery does not implement this action")
	}
}

func (api *NativeAPI) restoreObjects(ctx context.Context, state operationState, operationID string, engine *cloudrecovery.ObjectArchiveEngine, scope cloudrecovery.ObjectArchiveScope) (provider.NativeOperationObservation, error) {
	bucket, key, err := parseScalewayObjectReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	manifest, err := engine.ReadManifest(ctx, scope, bucket, key)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	targetPrefix, err := engine.Restore(ctx, scope, manifest)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return scalewayObjectRestoreObservation(api, state, operationID, manifest, targetPrefix), nil
}

func (api *NativeAPI) integrityObjects(ctx context.Context, state operationState, operationID string, engine *cloudrecovery.ObjectArchiveEngine, scope cloudrecovery.ObjectArchiveScope) (provider.NativeOperationObservation, error) {
	bucket, key, err := parseScalewayObjectReference(state.Backup)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	manifest, err := engine.ReadManifest(ctx, scope, bucket, key)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if err := engine.Integrity(ctx, scope, manifest); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	return scalewayObjectIntegrityObservation(api, state, operationID, manifest), nil
}

func (api *NativeAPI) objectEngine() (*cloudrecovery.ObjectArchiveEngine, error) {
	store, err := api.objectStore()
	if err != nil {
		return nil, err
	}
	return cloudrecovery.NewObjectArchiveEngine(store, cloudrecovery.ObjectArchiveConfig{
		ArchiveBucket: api.config.ArchiveBucket,
		ArchivePrefix: api.config.ArchivePrefix,
		RestoreBucket: api.config.RestoreBucket,
		RestorePrefix: api.config.RestorePrefix,
		IsNotFound:    isScalewayNotFound,
		EncryptionVerified: func(metadata cloudrecovery.ObjectMetadata) bool {
			return api.encryptionVerified(metadata)
		},
		ProtectionVerified: func(metadata cloudrecovery.ObjectMetadata) bool {
			return api.policyVerified(metadata)
		},
	})
}

func (api *NativeAPI) objectStore() (cloudrecovery.ObjectStore, error) {
	options := cloudrecovery.S3ObjectWriteOptions{Encryption: string(s3types.ServerSideEncryptionAes256)}
	if api.config.RequireKMS {
		options.Encryption = string(s3types.ServerSideEncryptionAwsKms)
		options.EncryptionKey = api.config.KMSKeyID
	}
	if api.config.RequireObjectLock {
		options.ObjectLockMode = string(s3types.ObjectLockModeCompliance)
		options.RetainUntil = api.now().Add(api.objectLockRetention())
	}
	return cloudrecovery.NewS3ObjectStore(scalewayS3API{client: api.objects, now: api.now, requireObjectLock: api.config.RequireObjectLock}, options)
}

func (api *NativeAPI) encryptionVerified(metadata cloudrecovery.ObjectMetadata) bool {
	if api.config.RequireKMS {
		return metadata.Encryption == string(s3types.ServerSideEncryptionAwsKms) && metadata.EncryptionKey == api.config.KMSKeyID
	}
	return metadata.Encryption != ""
}

func (api *NativeAPI) policyVerified(metadata cloudrecovery.ObjectMetadata) bool {
	return api.encryptionVerified(metadata) && (!api.config.RequireObjectLock || metadata.Protected)
}

func (api *NativeAPI) now() time.Time {
	if api.config.Now != nil {
		return api.config.Now()
	}
	return time.Now()
}

func (api *NativeAPI) retentionDays() int {
	if api.config.RetentionDays > 0 {
		return api.config.RetentionDays
	}
	return defaultScalewayRetentionDays
}

func (api *NativeAPI) objectLockRetention() time.Duration {
	if api.config.ObjectLockRetention > 0 {
		return api.config.ObjectLockRetention
	}
	return time.Duration(api.retentionDays()) * 24 * time.Hour
}

type scalewayS3API struct {
	client            ObjectAPI
	now               func() time.Time
	requireObjectLock bool
}

func (api scalewayS3API) List(ctx context.Context, bucket, prefix, token string) (cloudrecovery.S3ObjectPage, error) {
	input := &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)}
	if token != "" {
		input.ContinuationToken = aws.String(token)
	}
	output, err := api.client.ListObjectsV2(ctx, input)
	if err != nil {
		return cloudrecovery.S3ObjectPage{}, err
	}
	if output == nil {
		return cloudrecovery.S3ObjectPage{}, errors.New("Scaleway Object Storage listing returned an empty response")
	}
	objects := make([]cloudrecovery.ObjectInfo, 0, len(output.Contents))
	for _, object := range output.Contents {
		objects = append(objects, cloudrecovery.ObjectInfo{Key: aws.ToString(object.Key), Size: aws.ToInt64(object.Size), Validator: aws.ToString(object.ETag)})
	}
	return cloudrecovery.S3ObjectPage{Objects: objects, NextContinuationToken: aws.ToString(output.NextContinuationToken), Truncated: aws.ToBool(output.IsTruncated)}, nil
}

func (api scalewayS3API) Head(ctx context.Context, bucket, key string) (cloudrecovery.ObjectMetadata, error) {
	output, err := api.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return cloudrecovery.ObjectMetadata{}, err
	}
	if output == nil {
		return cloudrecovery.ObjectMetadata{}, errors.New("Scaleway Object Storage inspection returned an empty response")
	}
	protected := !api.requireObjectLock || (output.ObjectLockMode == s3types.ObjectLockModeCompliance && output.ObjectLockRetainUntilDate != nil && !output.ObjectLockRetainUntilDate.Before(api.now()))
	return cloudrecovery.ObjectMetadata{Size: aws.ToInt64(output.ContentLength), Validator: aws.ToString(output.ETag), Metadata: cloneScalewayMetadata(output.Metadata), Encryption: string(output.ServerSideEncryption), EncryptionKey: aws.ToString(output.SSEKMSKeyId), Protected: protected}, nil
}

func (api scalewayS3API) Read(ctx context.Context, bucket, key string) ([]byte, error) {
	output, err := api.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Body == nil {
		return nil, errors.New("Scaleway Object Storage response did not contain a body")
	}
	defer output.Body.Close()
	return io.ReadAll(output.Body)
}

func (api scalewayS3API) Put(ctx context.Context, bucket, key string, body []byte, metadata map[string]string, options cloudrecovery.S3ObjectWriteOptions) error {
	input := &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body), Metadata: cloneScalewayMetadata(metadata)}
	applyScalewayWriteOptions(input, options)
	_, err := api.client.PutObject(ctx, input)
	return err
}

func (api scalewayS3API) Copy(ctx context.Context, sourceBucket, sourceKey, targetBucket, targetKey string, metadata map[string]string, options cloudrecovery.S3ObjectWriteOptions) error {
	input := &s3.CopyObjectInput{Bucket: aws.String(targetBucket), Key: aws.String(targetKey), CopySource: aws.String(url.PathEscape(sourceBucket + "/" + sourceKey)), MetadataDirective: s3types.MetadataDirectiveReplace, Metadata: cloneScalewayMetadata(metadata)}
	applyScalewayCopyOptions(input, options)
	_, err := api.client.CopyObject(ctx, input)
	return err
}

func applyScalewayWriteOptions(input *s3.PutObjectInput, options cloudrecovery.S3ObjectWriteOptions) {
	if options.Encryption != "" {
		input.ServerSideEncryption = s3types.ServerSideEncryption(options.Encryption)
	}
	if options.EncryptionKey != "" {
		input.SSEKMSKeyId = aws.String(options.EncryptionKey)
	}
	if options.ObjectLockMode != "" {
		input.ObjectLockMode = s3types.ObjectLockMode(options.ObjectLockMode)
		retainUntil := options.RetainUntil
		input.ObjectLockRetainUntilDate = &retainUntil
	}
}

func applyScalewayCopyOptions(input *s3.CopyObjectInput, options cloudrecovery.S3ObjectWriteOptions) {
	if options.Encryption != "" {
		input.ServerSideEncryption = s3types.ServerSideEncryption(options.Encryption)
	}
	if options.EncryptionKey != "" {
		input.SSEKMSKeyId = aws.String(options.EncryptionKey)
	}
	if options.ObjectLockMode != "" {
		input.ObjectLockMode = s3types.ObjectLockMode(options.ObjectLockMode)
		retainUntil := options.RetainUntil
		input.ObjectLockRetainUntilDate = &retainUntil
	}
}

func cloneScalewayMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = value
	}
	return result
}

func scalewayObjectScope(state operationState) (cloudrecovery.ObjectArchiveScope, error) {
	bucket, prefix, err := parseScalewayObjectReference(state.Resource)
	if err != nil {
		return cloudrecovery.ObjectArchiveScope{}, err
	}
	return cloudrecovery.ObjectArchiveScope{DataClass: state.DataClass, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker, IdempotencyKey: state.IdempotencyKey, SourceBucket: bucket, SourcePrefix: prefix}, nil
}

func scalewayObjectBackupObservation(api *NativeAPI, state operationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest) provider.NativeOperationObservation {
	backupID := "scaleway-object://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json"
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: fmt.Sprintf("archived %d objects into an ownership-scoped sealed manifest", len(manifest.Entries))}
	return scalewayObservation(state, operationID, []string{backupID}, []string{"scaleway.object-storage.manifest", "scaleway.object-storage.encryption", "scaleway.object-storage.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func scalewayObjectRestoreObservation(api *NativeAPI, state operationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest, targetPrefix string) provider.NativeOperationObservation {
	restoreID := "scaleway-object://" + api.config.RestoreBucket + "/" + targetPrefix
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: "scaleway-object://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json", RestoreID: restoreID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: fmt.Sprintf("restored %d ownership-scoped objects into %s", len(manifest.Entries), targetPrefix)}
	return scalewayObservation(state, operationID, []string{restoreID}, []string{"scaleway.object-storage.restore-manifest", "scaleway.object-storage.restore-ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func scalewayObjectIntegrityObservation(api *NativeAPI, state operationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest) provider.NativeOperationObservation {
	backupID := "scaleway-object://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json"
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: fmt.Sprintf("verified %d archived object identities, sizes, ownership metadata, encryption boundary, and protection policy", len(manifest.Entries))}
	return scalewayObservation(state, operationID, []string{"scaleway-object://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix}, []string{"scaleway.object-storage.integrity-manifest", "scaleway.object-storage.integrity-ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func scalewayObservation(state operationState, operationID string, refs, proofRefs []string, evidence []sdk.ResilienceProofEvidence, status string) provider.NativeOperationObservation {
	return provider.NativeOperationObservation{Status: status, Action: state.Action, OperationID: operationID, ResourceRefs: refs, ProofRefs: proofRefs, OwnershipMarker: state.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true, Evidence: evidence}
}

func scalewayPending(state operationState, operationID string) provider.NativeOperationObservation {
	return scalewayObservation(state, operationID, nil, []string{"scaleway.object-storage.pending"}, nil, string(sdk.ResilienceOperationPending))
}

func parseScalewayObjectReference(reference string) (bucket, key string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || (parsed.Scheme != "scaleway-object" && parsed.Scheme != "scaleway-s3" && parsed.Scheme != "scw-s3") || strings.TrimSpace(parsed.Host) == "" {
		return "", "", fmt.Errorf("Scaleway Object Storage reference %q must use scaleway-object://bucket/prefix", reference)
	}
	key, err = url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || strings.TrimSpace(key) == "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(key, "\r\n\x00") {
		return "", "", fmt.Errorf("Scaleway Object Storage reference %q must include a safe non-empty prefix", reference)
	}
	return parsed.Host, key, nil
}

func (api *NativeAPI) archivePrefixForMarker(marker string) string {
	return api.archivePrefix() + "/" + shortDigest(marker)
}

func (api *NativeAPI) archivePrefix() string {
	return strings.Trim(strings.TrimSpace(api.config.ArchivePrefix), "/")
}

func (api *NativeAPI) objectPrefixForOperation(state operationState) string {
	return api.archivePrefixForMarker(state.OwnershipMarker) + "/" + state.DataClass + "/" + shortDigest(state.IdempotencyKey)
}

func validateScalewayContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("Scaleway recovery context is required")
	}
	return ctx.Err()
}

func validateScalewayRequest(request sdk.ResilienceOperationRequest) error {
	if request.Action == "" || strings.TrimSpace(request.FixtureID) == "" || strings.TrimSpace(request.OwnershipMarker) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		return errors.New("Scaleway recovery action, fixture, ownership marker, and idempotency key are required")
	}
	if strings.ContainsAny(request.FixtureID+request.OwnershipMarker+request.IdempotencyKey, "\r\n\x00") {
		return errors.New("Scaleway recovery identities must be single-line")
	}
	return nil
}

func scalewayCapabilityError(state operationState, reason string) error {
	return sdk.ResilienceCapabilityError{AdapterID: AdapterID, Operation: state.Action, DataClass: state.DataClass, Status: sdk.ResilienceCapabilityUnsupported, Reason: reason}
}

func scalewayObjectEndpoint(config NativeAPIConfig) (string, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = "https://s3." + strings.TrimSpace(config.Region) + ".scw.cloud"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Scaleway Object Storage endpoint must be an HTTPS URL without query or fragment")
	}
	return endpoint, nil
}

func isScalewayNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "notfound") || strings.Contains(message, "notexist") || strings.Contains(message, "nosuchkey") || strings.Contains(message, "404")
}

func isScalewayTransient(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "transient state")
}

func shortDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:24]
}
