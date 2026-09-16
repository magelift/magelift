package resilience

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/ovh/go-ovh/ovh"
	"github.com/ovh/okms-sdk-go"

	provider "github.com/magelift/magelift/internal/provider"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	cloudresilience "github.com/magelift/magelift/internal/shared/resilience"
	"github.com/magelift/magelift/sdk"
)

const (
	ovhRecoveryOperationPrefix = "ovh-recovery:v1:"
	defaultOVHArchivePrefix    = "magelift/recovery"
	defaultOVHRetentionDays    = 30
)

type operationState = cloudrecovery.OperationState

// ObjectAPI is the provider-local subset of the official S3 SDK used by
// OVHcloud Object Storage. SDK models remain inside this provider package.
type ObjectAPI interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	CopyObject(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// NativeAPIConfig contains OVHcloud recovery policy and opaque Object Storage
// identities. Credentials are injected as an SDK provider and never enter
// plans, checkpoints, operation IDs, or evidence.
type NativeAPIConfig struct {
	ArchiveBucket              string
	ArchivePrefix              string
	RestoreBucket              string
	RestorePrefix              string
	RestoreSecretPrefix        string
	DatabaseProjectID          string
	DatabaseEngine             string
	DatabaseRegion             string
	DatabasePlan               string
	DatabaseFlavor             string
	DatabaseVersion            string
	DatabaseSourceID           string
	DatabaseDiskGB             int
	DatabaseIPRestrictions     []string
	RestoreDatabaseRegion      string
	RestoreDatabasePlan        string
	RestoreDatabaseFlavor      string
	RestoreDatabaseVersion     string
	RequireDatabaseHA          bool
	MaxDatabaseBackupAge       time.Duration
	DatabaseBackupNotBefore    time.Time
	DatabaseUsePITR            bool
	DatabasePITRSettleDuration time.Duration
	DatabaseDeletePollInterval time.Duration
	SecretOKMSID               string
	SecretEndpoint             string
	SecretCredentialRef        string
	Verifier                   RecoveryVerifier
	Region                     string
	Endpoint                   string
	Credentials                aws.CredentialsProvider
	RequireKMS                 bool
	KMSKeyID                   string
	RequireObjectLock          bool
	// ObjectLockRetention is an acceptance-only positive override for the
	// otherwise day-based RetentionDays policy. Production callers should leave
	// it zero so the configured provider retention is applied.
	ObjectLockRetention time.Duration
	RetentionDays       int
	Projection          *cloudrecovery.ProjectionLifecycle
	Now                 func() time.Time
}

// NativeAPI translates the provider-neutral object recovery lifecycle to the
// OVHcloud S3-compatible Object Storage API. The archive algorithm is shared
// with AWS and other S3-compatible providers through internal/shared/recovery.
type NativeAPI struct {
	objects  ObjectAPI
	database DatabaseAPI
	secrets  SecretAPI
	config   NativeAPIConfig
}

var _ cloudresilience.NativeOperationAPI = (*NativeAPI)(nil)

// NewNativeAPI constructs a translator around an injected official SDK
// client. It is deterministic and does not perform provider mutations.
func NewNativeAPI(objects ObjectAPI, config NativeAPIConfig) (*NativeAPI, error) {
	return NewNativeAPIWithDatabaseAndSecrets(objects, nil, nil, config)
}

// NewNativeAPIWithDatabase composes the S3-compatible Object Storage
// translator with an optional OVHcloud Public Cloud Database API. The API is
// provider-local so neither OVH response models nor SDK credentials cross the
// shared recovery boundary.
func NewNativeAPIWithDatabase(objects ObjectAPI, database DatabaseAPI, config NativeAPIConfig) (*NativeAPI, error) {
	return NewNativeAPIWithDatabaseAndSecrets(objects, database, nil, config)
}

// NewNativeAPIWithDatabaseAndSecrets composes Object Storage, managed
// Database, and Secret Manager translators while keeping every provider SDK
// model at this package boundary.
func NewNativeAPIWithDatabaseAndSecrets(objects ObjectAPI, database DatabaseAPI, secrets SecretAPI, config NativeAPIConfig) (*NativeAPI, error) {
	if objects == nil {
		return nil, errors.New("OVHcloud Object Storage API is required")
	}
	if err := config.validate(); err != nil {
		return nil, err
	}
	database = normalizeOVHDatabaseAPI(database)
	secrets = normalizeOVHSecretAPI(secrets)
	return &NativeAPI{objects: objects, database: database, secrets: secrets, config: normalizeOVHConfig(config)}, nil
}

func normalizeOVHDatabaseAPI(api DatabaseAPI) DatabaseAPI {
	if typed, ok := api.(*ovhDatabaseSDK); ok && typed == nil {
		return nil
	}
	return api
}

func normalizeOVHSecretAPI(api SecretAPI) SecretAPI {
	if typed, ok := api.(*ovhSecretSDK); ok && typed == nil {
		return nil
	}
	return api
}

// NewOVHNativeAPI constructs the official AWS SDK S3 client against the
// documented OVHcloud Object Storage endpoint. It uses the AWS SDK only for
// the published S3-compatible protocol, not for AWS lifecycle semantics.
func NewOVHNativeAPI(ctx context.Context, config NativeAPIConfig) (*NativeAPI, error) {
	return newOVHNativeAPI(ctx, config, nil, nil)
}

// NewOVHNativeAPIWithDatabase constructs the official S3 and OVH API
// translators. The OVH API client is injected after credential resolution so
// secret values remain outside recovery plans and operation identities.
func NewOVHNativeAPIWithDatabase(ctx context.Context, config NativeAPIConfig, database *ovh.Client) (*NativeAPI, error) {
	if database == nil {
		return nil, errors.New("OVHcloud Public Cloud Database API client is required")
	}
	return newOVHNativeAPI(ctx, config, database, nil)
}

// NewOVHDatabaseNativeAPI constructs only the Public Cloud Database
// translator from an injected official OVH API client. Database-only recovery
// cells must not require Object Storage credentials or perform unrelated
// object inventory requests.
func NewOVHDatabaseNativeAPI(ctx context.Context, config NativeAPIConfig, database *ovh.Client) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("OVHcloud recovery context is required")
	}
	if database == nil {
		return nil, errors.New("OVHcloud Public Cloud Database API client is required")
	}
	if strings.TrimSpace(config.DatabaseProjectID) == "" {
		return nil, errors.New("OVHcloud Public Cloud Database project is required")
	}
	if err := config.validatePolicy(); err != nil {
		return nil, err
	}
	return &NativeAPI{
		database: newOVHDatabaseSDK(database, config),
		config:   normalizeOVHConfig(config),
	}, nil
}

// NewOVHNativeAPIWithSecrets constructs the official OVHcloud Object Storage
// and Secret Manager translators. The managed database translator remains
// optional; callers that need both use NewOVHNativeAPIWithDatabaseAndSecrets.
func NewOVHNativeAPIWithSecrets(ctx context.Context, config NativeAPIConfig, secrets *okms.Client) (*NativeAPI, error) {
	if secrets == nil {
		return nil, errors.New("OVHcloud Secret Manager API client is required")
	}
	return newOVHNativeAPI(ctx, config, nil, secrets)
}

// NewOVHNativeAPIWithDatabaseAndSecrets constructs the official OVHcloud
// Object Storage, Public Cloud Database, and Secret Manager translators.
// Secret Manager authentication is configured on the injected OKMS client so
// no token or certificate material enters MageLift lifecycle state.
func NewOVHNativeAPIWithDatabaseAndSecrets(ctx context.Context, config NativeAPIConfig, database *ovh.Client, secrets *okms.Client) (*NativeAPI, error) {
	if database == nil {
		return nil, errors.New("OVHcloud Public Cloud Database API client is required")
	}
	if secrets == nil {
		return nil, errors.New("OVHcloud Secret Manager API client is required")
	}
	return newOVHNativeAPI(ctx, config, database, secrets)
}

func newOVHNativeAPI(ctx context.Context, config NativeAPIConfig, database *ovh.Client, secrets *okms.Client) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("OVHcloud recovery context is required")
	}
	if strings.TrimSpace(config.Region) == "" {
		return nil, errors.New("OVHcloud Object Storage region is required")
	}
	if config.Credentials == nil {
		return nil, errors.New("OVHcloud Object Storage credentials are required")
	}
	if secrets != nil {
		if err := validateOVHSecretConfig(config); err != nil {
			return nil, err
		}
	}
	endpoint, err := ovhObjectEndpoint(config)
	if err != nil {
		return nil, err
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(config.Region), awsconfig.WithCredentialsProvider(config.Credentials))
	if err != nil {
		return nil, fmt.Errorf("load OVHcloud Object Storage SDK configuration: %w", err)
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	var databaseAPI DatabaseAPI
	if database != nil {
		databaseAPI = newOVHDatabaseSDK(database, config)
	}
	var secretAPI SecretAPI
	if secrets != nil {
		secretAPI = newOVHSecretSDK(secrets, config)
	}
	return NewNativeAPIWithDatabaseAndSecrets(client, databaseAPI, secretAPI, config)
}

func (config NativeAPIConfig) validate() error {
	if strings.TrimSpace(config.ArchiveBucket) == "" {
		return errors.New("OVHcloud recovery archive bucket is required")
	}
	return config.validatePolicy()
}

func (config NativeAPIConfig) validatePolicy() error {
	if config.RequireKMS && strings.TrimSpace(config.KMSKeyID) == "" {
		return errors.New("OVHcloud recovery KMS key is required when KMS encryption is required")
	}
	if config.Now != nil && config.Now().IsZero() {
		return errors.New("OVHcloud recovery clock must return a non-zero time")
	}
	if config.MaxDatabaseBackupAge < 0 {
		return errors.New("OVHcloud database backup age cannot be negative")
	}
	if config.DatabaseDeletePollInterval < 0 {
		return errors.New("OVHcloud database delete poll interval cannot be negative")
	}
	for _, restriction := range config.DatabaseIPRestrictions {
		if _, _, err := net.ParseCIDR(strings.TrimSpace(restriction)); err != nil {
			return fmt.Errorf("OVHcloud database IP restriction %q is not a valid CIDR: %w", restriction, err)
		}
	}
	if config.ObjectLockRetention < 0 {
		return errors.New("OVHcloud Object Lock retention cannot be negative")
	}
	if strings.TrimSpace(config.SecretOKMSID) != "" && strings.TrimSpace(config.SecretEndpoint) == "" {
		return errors.New("OVHcloud Secret Manager endpoint is required when an OKMS identity is configured")
	}
	return nil
}

func normalizeOVHConfig(config NativeAPIConfig) NativeAPIConfig {
	if strings.TrimSpace(config.ArchivePrefix) == "" {
		config.ArchivePrefix = defaultOVHArchivePrefix
	}
	if strings.TrimSpace(config.RestoreBucket) == "" {
		config.RestoreBucket = config.ArchiveBucket
	}
	if strings.TrimSpace(config.RestorePrefix) == "" {
		config.RestorePrefix = strings.Trim(config.ArchivePrefix, "/") + "/restored"
	}
	if config.RetentionDays <= 0 {
		config.RetentionDays = defaultOVHRetentionDays
	}
	if strings.TrimSpace(config.DatabaseRegion) == "" {
		config.DatabaseRegion = strings.TrimSpace(config.Region)
	}
	if strings.TrimSpace(config.RestoreDatabaseRegion) == "" {
		config.RestoreDatabaseRegion = config.DatabaseRegion
	}
	if config.MaxDatabaseBackupAge == 0 {
		config.MaxDatabaseBackupAge = 24 * time.Hour
	}
	if config.DatabaseDeletePollInterval == 0 {
		config.DatabaseDeletePollInterval = time.Second
	}
	config.DatabaseIPRestrictions = append([]string(nil), config.DatabaseIPRestrictions...)
	config.SecretEndpoint = strings.TrimRight(strings.TrimSpace(config.SecretEndpoint), "/")
	return config
}

func (api *NativeAPI) Start(ctx context.Context, request cloudresilience.NativeOperationRequest) (provider.NativeOperationObservation, error) {
	if err := validateOVHContext(ctx); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if request.Provider != sdk.ProviderID("ovh") {
		return provider.NativeOperationObservation{}, fmt.Errorf("OVHcloud recovery received provider %q", request.Provider)
	}
	if len(request.Request.DataClasses) != 1 {
		return provider.NativeOperationObservation{}, errors.New("OVHcloud recovery requires exactly one data class per operation")
	}
	expectedOperation, err := ovhOperationName(request.Request)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if request.Operation != expectedOperation {
		return provider.NativeOperationObservation{}, fmt.Errorf("OVHcloud recovery operation %q does not match %q", request.Operation, expectedOperation)
	}
	if err := validateOVHRequest(request.Request); err != nil {
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
	if strings.TrimSpace(state.Resource) == "" {
		return provider.NativeOperationObservation{}, fmt.Errorf("OVHcloud recovery resource reference for %q is required", state.DataClass)
	}
	operationID, err := cloudrecovery.EncodeOperationID(ovhRecoveryOperationPrefix, state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if state.DataClass != "database" && state.DataClass != "cache" && state.DataClass != "search-index" && state.DataClass != "media" && state.DataClass != "configuration-secrets" && state.DataClass != "infrastructure-state" && state.DataClass != "audit-evidence" {
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud native recovery currently implements Object Storage durable classes only")
	}
	if state.DataClass == "cache" {
		switch state.Action {
		case sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck:
			return api.startProjectionClass(ctx, state, operationID)
		case sdk.ResilienceBackup:
			return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud cache recovery is reconstructible from the source of truth and does not support durable backup")
		}
	}
	if state.DataClass == "search-index" {
		return api.startProjectionClass(ctx, state, operationID)
	}
	if state.DataClass == "database" || state.DataClass == "cache" {
		return api.startDatabase(ctx, state, operationID)
	}
	if state.DataClass == "configuration-secrets" {
		return api.startSecret(ctx, state, operationID)
	}
	return api.startObjects(ctx, state, operationID)
}

func (api *NativeAPI) Poll(ctx context.Context, operationID string) (provider.NativeOperationObservation, error) {
	if err := validateOVHContext(ctx); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	state, err := cloudrecovery.DecodeOperationID(ovhRecoveryOperationPrefix, operationID)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if state.DataClass != "database" && state.DataClass != "cache" && state.DataClass != "search-index" && state.DataClass != "media" && state.DataClass != "configuration-secrets" && state.DataClass != "infrastructure-state" && state.DataClass != "audit-evidence" {
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud native recovery currently implements Object Storage durable classes only")
	}
	if state.DataClass == "cache" {
		switch state.Action {
		case sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck:
			return api.pollProjectionClass(ctx, state, operationID)
		case sdk.ResilienceBackup:
			return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud cache recovery is reconstructible from the source of truth and does not support durable backup")
		}
	}
	if state.DataClass == "search-index" {
		return api.pollProjectionClass(ctx, state, operationID)
	}
	if state.DataClass == "database" || state.DataClass == "cache" {
		return api.pollDatabase(ctx, state, operationID)
	}
	if state.DataClass == "configuration-secrets" {
		return api.pollSecret(ctx, state, operationID)
	}
	return api.pollObjects(ctx, state, operationID)
}

func (api *NativeAPI) Inventory(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	if err := validateOVHContext(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("OVHcloud recovery inventory ownership marker is required")
	}
	resources := make([]provider.InventoryResource, 0)
	if api.objects != nil {
		store, err := api.objectStore()
		if err != nil {
			return nil, err
		}
		objects, err := store.List(ctx, api.config.ArchiveBucket, api.archivePrefixForMarker(marker))
		if err != nil {
			return nil, fmt.Errorf("inventory OVHcloud recovery archive: %w", err)
		}
		resources = make([]provider.InventoryResource, 0, len(objects))
		for _, object := range objects {
			metadata, headErr := store.Head(ctx, api.config.ArchiveBucket, object.Key)
			if headErr != nil {
				if isOVHNotFound(headErr) {
					continue
				}
				return nil, fmt.Errorf("inspect OVHcloud recovery inventory object: %w", headErr)
			}
			if metadata.Metadata[cloudrecovery.OwnershipMetadataKey] != marker || !api.policyVerified(metadata) {
				continue
			}
			resources = append(resources, provider.InventoryResource{Identity: "ovh-object://" + api.config.ArchiveBucket + "/" + object.Key, Owned: true, Live: true})
		}
	}
	if api.database != nil {
		databaseResources, databaseErr := api.inventoryDatabase(ctx, marker)
		if databaseErr != nil {
			return nil, databaseErr
		}
		resources = append(resources, databaseResources...)
	}
	if api.secrets != nil {
		secretResources, secretErr := api.inventorySecrets(ctx, marker)
		if secretErr != nil {
			return nil, secretErr
		}
		resources = append(resources, secretResources...)
	}
	return resources, nil
}

func (api *NativeAPI) startObjects(ctx context.Context, state operationState, operationID string) (provider.NativeOperationObservation, error) {
	if state.Action == sdk.ResilienceCleanup {
		return api.cleanupObjectsForState(ctx, state, operationID)
	}
	if _, _, err := parseOVHObjectReference(state.Resource); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	engine, err := api.objectEngine()
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	scope, err := ovhObjectScope(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		manifest, backupErr := engine.Backup(ctx, scope)
		if backupErr != nil {
			return provider.NativeOperationObservation{}, backupErr
		}
		return ovhObjectBackupObservation(api, state, operationID, manifest), nil
	case sdk.ResilienceRestore:
		return api.restoreObjects(ctx, state, operationID, engine, scope)
	case sdk.ResilienceIntegrityCheck:
		return api.integrityObjects(ctx, state, operationID, engine, scope)
	default:
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud Object Storage recovery does not implement this action")
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
	scope, err := ovhObjectScope(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	manifestBucket, manifestKey, parseErr := parseOVHObjectReference(state.Backup)
	if state.Action == sdk.ResilienceBackup {
		manifestBucket = api.config.ArchiveBucket
		manifestKey = api.objectPrefixForOperation(state) + "/manifest.json"
	}
	if parseErr != nil && state.Action != sdk.ResilienceBackup {
		return provider.NativeOperationObservation{}, parseErr
	}
	manifest, err := engine.ReadManifest(ctx, scope, manifestBucket, manifestKey)
	if err != nil {
		if isOVHNotFound(err) {
			return ovhPending(state, operationID), nil
		}
		return provider.NativeOperationObservation{}, err
	}
	switch state.Action {
	case sdk.ResilienceBackup:
		return ovhObjectBackupObservation(api, state, operationID, manifest), nil
	case sdk.ResilienceRestore:
		targetPrefix, verifyErr := engine.VerifyRestore(ctx, scope, manifest)
		if errors.Is(verifyErr, cloudrecovery.ErrObjectNotReady) {
			return ovhPending(state, operationID), nil
		}
		if verifyErr != nil {
			return provider.NativeOperationObservation{}, verifyErr
		}
		return ovhObjectRestoreObservation(api, state, operationID, manifest, targetPrefix), nil
	case sdk.ResilienceIntegrityCheck:
		if err := engine.Integrity(ctx, scope, manifest); err != nil {
			return provider.NativeOperationObservation{}, err
		}
		return ovhObjectIntegrityObservation(api, state, operationID, manifest), nil
	default:
		return provider.NativeOperationObservation{}, ovhCapabilityError(state, "OVHcloud Object Storage recovery does not implement this action")
	}
}

func (api *NativeAPI) restoreObjects(ctx context.Context, state operationState, operationID string, engine *cloudrecovery.ObjectArchiveEngine, scope cloudrecovery.ObjectArchiveScope) (provider.NativeOperationObservation, error) {
	bucket, key, err := parseOVHObjectReference(state.Backup)
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
	return ovhObjectRestoreObservation(api, state, operationID, manifest, targetPrefix), nil
}

func (api *NativeAPI) integrityObjects(ctx context.Context, state operationState, operationID string, engine *cloudrecovery.ObjectArchiveEngine, scope cloudrecovery.ObjectArchiveScope) (provider.NativeOperationObservation, error) {
	bucket, key, err := parseOVHObjectReference(state.Backup)
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
	return ovhObjectIntegrityObservation(api, state, operationID, manifest), nil
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
		IsNotFound:    isOVHNotFound,
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
	return cloudrecovery.NewS3ObjectStore(ovhS3API{client: api.objects, now: api.now, requireObjectLock: api.config.RequireObjectLock}, options)
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
	return defaultOVHRetentionDays
}

func (api *NativeAPI) objectLockRetention() time.Duration {
	if api.config.ObjectLockRetention > 0 {
		return api.config.ObjectLockRetention
	}
	return time.Duration(api.retentionDays()) * 24 * time.Hour
}

type ovhS3API struct {
	client            ObjectAPI
	now               func() time.Time
	requireObjectLock bool
}

func (api ovhS3API) List(ctx context.Context, bucket, prefix, token string) (cloudrecovery.S3ObjectPage, error) {
	input := &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)}
	if token != "" {
		input.ContinuationToken = aws.String(token)
	}
	output, err := api.client.ListObjectsV2(ctx, input)
	if err != nil {
		return cloudrecovery.S3ObjectPage{}, err
	}
	if output == nil {
		return cloudrecovery.S3ObjectPage{}, errors.New("OVHcloud Object Storage listing returned an empty response")
	}
	objects := make([]cloudrecovery.ObjectInfo, 0, len(output.Contents))
	for _, object := range output.Contents {
		objects = append(objects, cloudrecovery.ObjectInfo{Key: aws.ToString(object.Key), Size: aws.ToInt64(object.Size), Validator: aws.ToString(object.ETag)})
	}
	return cloudrecovery.S3ObjectPage{Objects: objects, NextContinuationToken: aws.ToString(output.NextContinuationToken), Truncated: aws.ToBool(output.IsTruncated)}, nil
}

func (api ovhS3API) Head(ctx context.Context, bucket, key string) (cloudrecovery.ObjectMetadata, error) {
	output, err := api.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return cloudrecovery.ObjectMetadata{}, err
	}
	if output == nil {
		return cloudrecovery.ObjectMetadata{}, errors.New("OVHcloud Object Storage inspection returned an empty response")
	}
	protected := !api.requireObjectLock || (output.ObjectLockMode == s3types.ObjectLockModeCompliance && output.ObjectLockRetainUntilDate != nil && !output.ObjectLockRetainUntilDate.Before(api.now()))
	return cloudrecovery.ObjectMetadata{Size: aws.ToInt64(output.ContentLength), Validator: aws.ToString(output.ETag), Metadata: cloneOVHMetadata(output.Metadata), Encryption: string(output.ServerSideEncryption), EncryptionKey: aws.ToString(output.SSEKMSKeyId), Protected: protected}, nil
}

func (api ovhS3API) Read(ctx context.Context, bucket, key string) ([]byte, error) {
	output, err := api.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Body == nil {
		return nil, errors.New("OVHcloud Object Storage response did not contain a body")
	}
	defer output.Body.Close()
	return io.ReadAll(output.Body)
}

func (api ovhS3API) Put(ctx context.Context, bucket, key string, body []byte, metadata map[string]string, options cloudrecovery.S3ObjectWriteOptions) error {
	input := &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body), Metadata: cloneOVHMetadata(metadata)}
	applyOVHWriteOptions(input, options)
	_, err := api.client.PutObject(ctx, input)
	return err
}

func (api ovhS3API) Copy(ctx context.Context, sourceBucket, sourceKey, targetBucket, targetKey string, metadata map[string]string, options cloudrecovery.S3ObjectWriteOptions) error {
	input := &s3.CopyObjectInput{Bucket: aws.String(targetBucket), Key: aws.String(targetKey), CopySource: aws.String(url.PathEscape(sourceBucket + "/" + sourceKey)), MetadataDirective: s3types.MetadataDirectiveReplace, Metadata: cloneOVHMetadata(metadata)}
	applyOVHCopyOptions(input, options)
	_, err := api.client.CopyObject(ctx, input)
	return err
}

func applyOVHWriteOptions(input *s3.PutObjectInput, options cloudrecovery.S3ObjectWriteOptions) {
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

func applyOVHCopyOptions(input *s3.CopyObjectInput, options cloudrecovery.S3ObjectWriteOptions) {
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

func cloneOVHMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	result := make(map[string]string, len(metadata))
	for key, value := range metadata {
		result[key] = value
	}
	return result
}

func ovhObjectScope(state operationState) (cloudrecovery.ObjectArchiveScope, error) {
	bucket, prefix, err := parseOVHObjectReference(state.Resource)
	if err != nil {
		return cloudrecovery.ObjectArchiveScope{}, err
	}
	return cloudrecovery.ObjectArchiveScope{DataClass: state.DataClass, FixtureID: state.FixtureID, OwnershipMarker: state.OwnershipMarker, IdempotencyKey: state.IdempotencyKey, SourceBucket: bucket, SourcePrefix: prefix}, nil
}

func ovhObjectBackupObservation(api *NativeAPI, state operationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest) provider.NativeOperationObservation {
	backupID := "ovh-object://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json"
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: fmt.Sprintf("archived %d objects into an ownership-scoped sealed manifest", len(manifest.Entries))}
	return ovhObservation(state, operationID, []string{backupID}, []string{"ovh.object-storage.manifest", "ovh.object-storage.encryption", "ovh.object-storage.ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func ovhObjectRestoreObservation(api *NativeAPI, state operationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest, targetPrefix string) provider.NativeOperationObservation {
	restoreID := "ovh-object://" + api.config.RestoreBucket + "/" + targetPrefix
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: "ovh-object://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json", RestoreID: restoreID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: fmt.Sprintf("restored %d ownership-scoped objects into %s", len(manifest.Entries), targetPrefix)}
	return ovhObservation(state, operationID, []string{restoreID}, []string{"ovh.object-storage.restore-manifest", "ovh.object-storage.restore-ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func ovhObjectIntegrityObservation(api *NativeAPI, state operationState, operationID string, manifest cloudrecovery.ObjectArchiveManifest) provider.NativeOperationObservation {
	backupID := "ovh-object://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix + "/manifest.json"
	evidence := sdk.ResilienceProofEvidence{DataClass: state.DataClass, Destination: string(state.Destination), BackupID: backupID, FixtureID: state.FixtureID, RetentionDays: api.retentionDays(), EncryptionVerified: true, ProtectionVerified: true, ManifestVerified: true, CountsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true, Reason: fmt.Sprintf("verified %d archived object identities, sizes, ownership metadata, encryption boundary, and protection policy", len(manifest.Entries))}
	return ovhObservation(state, operationID, []string{"ovh-object://" + manifest.ArchiveBucket + "/" + manifest.ArchivePrefix}, []string{"ovh.object-storage.integrity-manifest", "ovh.object-storage.integrity-ownership"}, []sdk.ResilienceProofEvidence{evidence}, string(sdk.ResilienceOperationSucceeded))
}

func ovhObservation(state operationState, operationID string, refs, proofRefs []string, evidence []sdk.ResilienceProofEvidence, status string) provider.NativeOperationObservation {
	return provider.NativeOperationObservation{Status: status, Action: state.Action, OperationID: operationID, ResourceRefs: refs, ProofRefs: proofRefs, OwnershipMarker: state.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true, Evidence: evidence}
}

func ovhPending(state operationState, operationID string) provider.NativeOperationObservation {
	return ovhObservation(state, operationID, nil, []string{"ovh.object-storage.pending"}, nil, string(sdk.ResilienceOperationPending))
}

func parseOVHObjectReference(reference string) (bucket, key string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || (parsed.Scheme != "ovh-object" && parsed.Scheme != "ovh-s3" && parsed.Scheme != "ovhcloud-s3") || strings.TrimSpace(parsed.Host) == "" {
		return "", "", fmt.Errorf("OVHcloud Object Storage reference %q must use ovh-object://bucket/prefix", reference)
	}
	key, err = url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || strings.TrimSpace(key) == "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(key, "\r\n\x00") {
		return "", "", fmt.Errorf("OVHcloud Object Storage reference %q must include a safe non-empty prefix", reference)
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

func validateOVHContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("OVHcloud recovery context is required")
	}
	return ctx.Err()
}

func validateOVHRequest(request sdk.ResilienceOperationRequest) error {
	if request.Action == "" || strings.TrimSpace(request.FixtureID) == "" || strings.TrimSpace(request.OwnershipMarker) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		return errors.New("OVHcloud recovery action, fixture, ownership marker, and idempotency key are required")
	}
	if strings.ContainsAny(request.FixtureID+request.OwnershipMarker+request.IdempotencyKey, "\r\n\x00") {
		return errors.New("OVHcloud recovery identities must be single-line")
	}
	return nil
}

func ovhCapabilityError(state operationState, reason string) error {
	return sdk.ResilienceCapabilityError{AdapterID: AdapterID, Operation: state.Action, DataClass: state.DataClass, Status: sdk.ResilienceCapabilityUnsupported, Reason: reason}
}

func ovhObjectEndpoint(config NativeAPIConfig) (string, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		endpoint = "https://s3." + strings.ToLower(strings.TrimSpace(config.Region)) + ".io.cloud.ovh.net"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("OVHcloud Object Storage endpoint must be an HTTPS URL without query or fragment")
	}
	return endpoint, nil
}

func isOVHNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "notfound") || strings.Contains(message, "notexist") || strings.Contains(message, "nosuchkey") || strings.Contains(message, "404")
}

func shortDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:24]
}
