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
	"sort"
	"strings"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	rdstypes "github.com/aws/aws-sdk-go-v2/service/rds/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretstypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	cloudrecovery "github.com/magelift/magelift/internal/cloud/recovery"
	cloudresilience "github.com/magelift/magelift/internal/cloud/resilience"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	defaultArchivePrefix = "magelift/recovery"
	defaultRetentionDays = 30
	ownershipTagKey      = "magelift.io/ownership"
	classTagKey          = "magelift.io/data-class"
	operationTagKey      = "magelift.io/operation"
	archiveOwnershipKey  = "magelift-ownership"
	archiveClassKey      = "magelift-data-class"
	archiveFixtureKey    = "magelift-fixture"
	archiveManifestKey   = "magelift-manifest"
	operationVersion     = cloudrecovery.OperationVersion
)

// RDSAPI is the provider-local subset of the official AWS SDK used by the
// recovery translator. AWS request and response models stop at this package.
type RDSAPI interface {
	CreateDBSnapshot(context.Context, *rds.CreateDBSnapshotInput, ...func(*rds.Options)) (*rds.CreateDBSnapshotOutput, error)
	DescribeDBSnapshots(context.Context, *rds.DescribeDBSnapshotsInput, ...func(*rds.Options)) (*rds.DescribeDBSnapshotsOutput, error)
	DeleteDBSnapshot(context.Context, *rds.DeleteDBSnapshotInput, ...func(*rds.Options)) (*rds.DeleteDBSnapshotOutput, error)
	CreateDBClusterSnapshot(context.Context, *rds.CreateDBClusterSnapshotInput, ...func(*rds.Options)) (*rds.CreateDBClusterSnapshotOutput, error)
	DescribeDBClusterSnapshots(context.Context, *rds.DescribeDBClusterSnapshotsInput, ...func(*rds.Options)) (*rds.DescribeDBClusterSnapshotsOutput, error)
	DeleteDBClusterSnapshot(context.Context, *rds.DeleteDBClusterSnapshotInput, ...func(*rds.Options)) (*rds.DeleteDBClusterSnapshotOutput, error)
	DescribeDBInstances(context.Context, *rds.DescribeDBInstancesInput, ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
	ModifyDBInstance(context.Context, *rds.ModifyDBInstanceInput, ...func(*rds.Options)) (*rds.ModifyDBInstanceOutput, error)
	DeleteDBInstance(context.Context, *rds.DeleteDBInstanceInput, ...func(*rds.Options)) (*rds.DeleteDBInstanceOutput, error)
	DescribeDBClusters(context.Context, *rds.DescribeDBClustersInput, ...func(*rds.Options)) (*rds.DescribeDBClustersOutput, error)
	ModifyDBCluster(context.Context, *rds.ModifyDBClusterInput, ...func(*rds.Options)) (*rds.ModifyDBClusterOutput, error)
	DeleteDBCluster(context.Context, *rds.DeleteDBClusterInput, ...func(*rds.Options)) (*rds.DeleteDBClusterOutput, error)
	RestoreDBInstanceFromDBSnapshot(context.Context, *rds.RestoreDBInstanceFromDBSnapshotInput, ...func(*rds.Options)) (*rds.RestoreDBInstanceFromDBSnapshotOutput, error)
	RestoreDBClusterFromSnapshot(context.Context, *rds.RestoreDBClusterFromSnapshotInput, ...func(*rds.Options)) (*rds.RestoreDBClusterFromSnapshotOutput, error)
	CreateDBInstance(context.Context, *rds.CreateDBInstanceInput, ...func(*rds.Options)) (*rds.CreateDBInstanceOutput, error)
	ListTagsForResource(context.Context, *rds.ListTagsForResourceInput, ...func(*rds.Options)) (*rds.ListTagsForResourceOutput, error)
}

// S3API is the provider-local subset of the official AWS SDK used for media,
// infrastructure-state, audit, and encrypted secret archives.
type S3API interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	CopyObject(context.Context, *s3.CopyObjectInput, ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
	DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error)
}

// SecretsAPI is the provider-local subset of Secrets Manager needed for
// versioned secret archive and isolated restore.
type SecretsAPI interface {
	GetSecretValue(context.Context, *secretsmanager.GetSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
	DescribeSecret(context.Context, *secretsmanager.DescribeSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error)
	CreateSecret(context.Context, *secretsmanager.CreateSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.CreateSecretOutput, error)
	PutSecretValue(context.Context, *secretsmanager.PutSecretValueInput, ...func(*secretsmanager.Options)) (*secretsmanager.PutSecretValueOutput, error)
	DeleteSecret(context.Context, *secretsmanager.DeleteSecretInput, ...func(*secretsmanager.Options)) (*secretsmanager.DeleteSecretOutput, error)
	ListSecrets(context.Context, *secretsmanager.ListSecretsInput, ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
}

// SQSAPI is the provider-local subset of the official AWS SDK used for the
// bounded, ownership-scoped SQS export/replay path. SQS has no native snapshot
// or writer-fencing API, so this port deliberately exposes only the calls
// required to inspect queues, copy messages without deleting the source, and
// clean up queues created by the recovery operation.
type SQSAPI interface {
	GetQueueUrl(context.Context, *sqs.GetQueueUrlInput, ...func(*sqs.Options)) (*sqs.GetQueueUrlOutput, error)
	GetQueueAttributes(context.Context, *sqs.GetQueueAttributesInput, ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
	ListQueueTags(context.Context, *sqs.ListQueueTagsInput, ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error)
	CreateQueue(context.Context, *sqs.CreateQueueInput, ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error)
	TagQueue(context.Context, *sqs.TagQueueInput, ...func(*sqs.Options)) (*sqs.TagQueueOutput, error)
	ReceiveMessage(context.Context, *sqs.ReceiveMessageInput, ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	SendMessageBatch(context.Context, *sqs.SendMessageBatchInput, ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error)
	ChangeMessageVisibilityBatch(context.Context, *sqs.ChangeMessageVisibilityBatchInput, ...func(*sqs.Options)) (*sqs.ChangeMessageVisibilityBatchOutput, error)
	ListQueues(context.Context, *sqs.ListQueuesInput, ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error)
	DeleteQueue(context.Context, *sqs.DeleteQueueInput, ...func(*sqs.Options)) (*sqs.DeleteQueueOutput, error)
}

// RecoveryVerification is the result of an application-owned known-content
// probe. Provider APIs can establish control-plane health, but only the
// workload can prove that a restored database contains the expected fixture.
type RecoveryVerification struct {
	ManifestVerified         bool
	CountsVerified           bool
	ApplicationReadsVerified bool
	PermissionsVerified      bool
	SecretReferencesVerified bool
	ServiceHealthVerified    bool
}

// RecoveryVerifier is optional for object and secret classes, which can be
// verified directly. It is required for database integrity checks because RDS
// does not expose application record contents through its control plane.
type RecoveryVerifier interface {
	Verify(context.Context, RecoveryVerificationRequest) (RecoveryVerification, error)
}

type RecoveryVerificationRequest struct {
	DataClass       string
	Resource        string
	FixtureID       string
	OwnershipMarker string
}

// NativeAPIConfig contains only provider-local recovery policy. Resource
// references remain explicit opaque identities supplied by the architecture;
// this type does not invent a cloud resource or silently pick an account.
type NativeAPIConfig struct {
	ArchiveBucket              string
	ArchivePrefix              string
	RestoreBucket              string
	RestorePrefix              string
	RestoreDBInstanceClass     string
	RestoreDBSubnetGroup       string
	RestorePubliclyAccessible  bool
	RestoreVPCSecurityGroupIDs []string
	RestoreAuroraInstanceClass string
	RestoreAuroraSubnetGroup   string
	RestoreSecretPrefix        string
	RetentionDays              int
	QueueMaxMessages           int
	QueueVisibilityTimeout     int32
	RequireKMS                 bool
	KMSKeyID                   string
	RequireObjectLock          bool
	Verifier                   RecoveryVerifier
	Projection                 *cloudrecovery.ProjectionLifecycle
	Now                        func() time.Time
}

// NativeAPI translates the provider-neutral recovery port to the official AWS
// SDK. It is safe to construct once and reuse across certification stages; no
// operation state is held in memory, so Poll can resume in a new process.
type NativeAPI struct {
	rds     RDSAPI
	s3      S3API
	secrets SecretsAPI
	sqs     SQSAPI
	config  NativeAPIConfig
}

var _ cloudresilience.NativeOperationAPI = (*NativeAPI)(nil)

// NewNativeAPI constructs an AWS recovery translator around injected official
// SDK clients. Injecting the narrow interfaces keeps tests deterministic and
// allows community AWS-compatible implementations without changing the core.
func NewNativeAPI(rdsClient RDSAPI, s3Client S3API, secretsClient SecretsAPI, config NativeAPIConfig) (*NativeAPI, error) {
	if err := config.validate(true); err != nil {
		return nil, err
	}
	return &NativeAPI{rds: rdsClient, s3: s3Client, secrets: secretsClient, config: normalizeConfig(config)}, nil
}

// NewNativeAPIWithSQS adds the injected SQS port without changing the
// existing constructor used by the other AWS recovery classes. The archive
// bucket remains required because this constructor can still serve all AWS
// classes.
func NewNativeAPIWithSQS(rdsClient RDSAPI, s3Client S3API, secretsClient SecretsAPI, sqsClient SQSAPI, config NativeAPIConfig) (*NativeAPI, error) {
	if err := config.validate(true); err != nil {
		return nil, err
	}
	if sqsClient == nil {
		return nil, errors.New("AWS SQS recovery API is required")
	}
	return &NativeAPI{rds: rdsClient, s3: s3Client, secrets: secretsClient, sqs: sqsClient, config: normalizeConfig(config)}, nil
}

// NewAWSNativeAPI loads the official AWS SDK configuration and constructs the
// provider translator. Credential and region resolution remain at the AWS
// boundary; neither enters the portable lifecycle contracts.
func NewAWSNativeAPI(ctx context.Context, config NativeAPIConfig, opts ...func(*awsconfig.LoadOptions) error) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("AWS recovery context is required")
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK configuration: %w", err)
	}
	return NewNativeAPIWithSQS(
		rds.NewFromConfig(awsConfig),
		s3.NewFromConfig(awsConfig),
		secretsmanager.NewFromConfig(awsConfig),
		sqs.NewFromConfig(awsConfig),
		config,
	)
}

// NewAWSSQSNativeAPI constructs only the SQS translator and its official SDK
// client. It is intended for focused certification cells so queue tests do not
// require an archive bucket or construct unrelated AWS service clients.
func NewAWSSQSNativeAPI(ctx context.Context, config NativeAPIConfig, opts ...func(*awsconfig.LoadOptions) error) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("AWS SQS recovery context is required")
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK configuration for SQS: %w", err)
	}
	if err := config.validate(false); err != nil {
		return nil, err
	}
	return &NativeAPI{sqs: sqs.NewFromConfig(awsConfig), config: normalizeConfig(config)}, nil
}

// NewAWSObjectNativeAPI constructs only the S3 translator and official S3
// client for focused object-recovery cells. It avoids constructing unrelated
// RDS, Secrets Manager, and SQS clients while preserving the same normalized
// provider boundary used by the full AWS recovery client.
func NewAWSObjectNativeAPI(ctx context.Context, config NativeAPIConfig, opts ...func(*awsconfig.LoadOptions) error) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("AWS object recovery context is required")
	}
	if err := config.validate(true); err != nil {
		return nil, err
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK configuration for S3: %w", err)
	}
	return &NativeAPI{s3: s3.NewFromConfig(awsConfig), config: normalizeConfig(config)}, nil
}

// NewAWSDatabaseNativeAPI constructs only the RDS/Aurora translator needed by
// focused database recovery cells. It does not require an archive bucket or
// construct unrelated S3, Secrets Manager, or SQS clients.
func NewAWSDatabaseNativeAPI(ctx context.Context, config NativeAPIConfig, opts ...func(*awsconfig.LoadOptions) error) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("AWS database recovery context is required")
	}
	if err := config.validate(false); err != nil {
		return nil, err
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK configuration for RDS: %w", err)
	}
	return &NativeAPI{rds: rds.NewFromConfig(awsConfig), config: normalizeConfig(config)}, nil
}

// NewAWSSecretNativeAPI constructs only the Secrets Manager and archive S3
// clients required by focused secret-recovery cells. It avoids constructing
// unrelated RDS and SQS clients while preserving the same provider-neutral
// operation boundary as the full AWS translator.
func NewAWSSecretNativeAPI(ctx context.Context, config NativeAPIConfig, opts ...func(*awsconfig.LoadOptions) error) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("AWS secret recovery context is required")
	}
	if err := config.validate(true); err != nil {
		return nil, err
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK configuration for Secrets Manager: %w", err)
	}
	return &NativeAPI{s3: s3.NewFromConfig(awsConfig), secrets: secretsmanager.NewFromConfig(awsConfig), config: normalizeConfig(config)}, nil
}

func (config NativeAPIConfig) validate(archiveRequired bool) error {
	if archiveRequired && strings.TrimSpace(config.ArchiveBucket) == "" {
		return errors.New("AWS recovery archive bucket is required")
	}
	if config.RequireKMS && strings.TrimSpace(config.KMSKeyID) == "" {
		return errors.New("AWS recovery KMS key is required when KMS encryption is required")
	}
	if config.Now != nil && config.Now().IsZero() {
		return errors.New("AWS recovery clock must return a non-zero time")
	}
	return nil
}

func normalizeConfig(config NativeAPIConfig) NativeAPIConfig {
	config.RestoreVPCSecurityGroupIDs = append([]string(nil), config.RestoreVPCSecurityGroupIDs...)
	if strings.TrimSpace(config.RestoreBucket) == "" {
		config.RestoreBucket = config.ArchiveBucket
	}
	if strings.TrimSpace(config.ArchivePrefix) == "" {
		config.ArchivePrefix = defaultArchivePrefix
	}
	if strings.TrimSpace(config.RestorePrefix) == "" {
		config.RestorePrefix = strings.Trim(config.ArchivePrefix, "/") + "/restored"
	}
	if config.RetentionDays <= 0 {
		config.RetentionDays = defaultRetentionDays
	}
	if config.QueueMaxMessages <= 0 {
		config.QueueMaxMessages = 1000
	}
	if config.QueueVisibilityTimeout <= 0 {
		config.QueueVisibilityTimeout = 30
	}
	return config
}

func (api *NativeAPI) Start(ctx context.Context, request cloudresilience.NativeOperationRequest) (provider.NativeOperationObservation, error) {
	if err := validateNativeContext(ctx); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if request.Provider != sdk.ProviderID("aws") {
		return provider.NativeOperationObservation{}, fmt.Errorf("AWS recovery received provider %q", request.Provider)
	}
	if len(request.Request.DataClasses) != 1 {
		return provider.NativeOperationObservation{}, errors.New("AWS recovery requires exactly one data class per operation")
	}
	expectedOperation, err := awsOperationName(request.Request)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	if request.Operation != expectedOperation {
		return provider.NativeOperationObservation{}, fmt.Errorf("AWS recovery operation %q does not match %q", request.Operation, expectedOperation)
	}
	if err := validateNativeRequest(request.Request); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	state := operationState{
		Version:           operationVersion,
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
		return provider.NativeOperationObservation{}, fmt.Errorf("AWS recovery resource reference for %q is required", state.DataClass)
	}
	operationID, err := encodeOperationState(state)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.DataClass {
	case "database":
		return api.startDatabase(ctx, state, operationID)
	case "media", "infrastructure-state", "audit-evidence":
		return api.startObjectClass(ctx, state, operationID)
	case "configuration-secrets":
		return api.startSecretClass(ctx, state, operationID)
	case "search-index", "cache":
		return api.startProjectionClass(ctx, state, operationID)
	case "queue":
		return api.startQueue(ctx, state, operationID)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "AWS recovery data class is not registered")
	}
}

func (api *NativeAPI) Poll(ctx context.Context, operationID string) (provider.NativeOperationObservation, error) {
	if err := validateNativeContext(ctx); err != nil {
		return provider.NativeOperationObservation{}, err
	}
	state, err := decodeOperationState(operationID)
	if err != nil {
		return provider.NativeOperationObservation{}, err
	}
	switch state.DataClass {
	case "database":
		return api.pollDatabase(ctx, state, operationID)
	case "media", "infrastructure-state", "audit-evidence":
		return api.pollObjectClass(ctx, state, operationID)
	case "configuration-secrets":
		return api.pollSecretClass(ctx, state, operationID)
	case "search-index", "cache":
		return api.pollProjectionClass(ctx, state, operationID)
	case "queue":
		return api.pollQueue(ctx, state, operationID)
	default:
		return provider.NativeOperationObservation{}, capabilityError(state, "AWS recovery data class is not registered")
	}
}

func (api *NativeAPI) Inventory(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	if err := validateNativeContext(ctx); err != nil {
		return nil, err
	}
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("AWS recovery inventory ownership marker is required")
	}
	if api.rds == nil && api.s3 == nil && api.sqs == nil && api.secrets == nil {
		return nil, errors.New("AWS recovery inventory requires an RDS, S3, SQS, or Secrets Manager API")
	}
	resources := make([]provider.InventoryResource, 0)
	if api.rds != nil {
		rdsResources, err := api.listOwnedRDS(ctx, marker)
		if err != nil {
			return nil, fmt.Errorf("inventory AWS RDS recovery resources: %w", err)
		}
		resources = append(resources, rdsResources...)
	}
	if api.s3 != nil {
		objects, err := api.listObjects(ctx, api.archiveBucket(), api.archivePrefixForMarker(marker))
		if err != nil {
			return nil, fmt.Errorf("inventory AWS recovery archive: %w", err)
		}
		for _, object := range objects {
			key := aws.ToString(object.Key)
			if key == "" {
				continue
			}
			resources = append(resources, provider.InventoryResource{Identity: "aws-s3://" + api.archiveBucket() + "/" + key, Owned: true, Live: true})
		}
	}
	if api.sqs != nil {
		queues, err := api.listOwnedSQSQueues(ctx, marker)
		if err != nil {
			return nil, fmt.Errorf("inventory AWS SQS recovery queues: %w", err)
		}
		resources = append(resources, queues...)
	}
	if api.secrets != nil {
		secrets, err := api.listOwnedSecrets(ctx, marker)
		if err != nil {
			return nil, fmt.Errorf("inventory AWS Secrets Manager recovery secrets: %w", err)
		}
		resources = append(resources, secrets...)
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

type operationState = cloudrecovery.OperationState

func encodeOperationState(state operationState) (string, error) {
	return cloudrecovery.EncodeOperationID("aws-recovery:v1:", state)
}

func decodeOperationState(operationID string) (operationState, error) {
	return cloudrecovery.DecodeOperationID("aws-recovery:v1:", operationID)
}

func validateNativeContext(ctx context.Context) error {
	if ctx == nil {
		return errors.New("AWS recovery context is required")
	}
	return ctx.Err()
}

func validateNativeRequest(request sdk.ResilienceOperationRequest) error {
	if request.Action == "" || strings.TrimSpace(request.FixtureID) == "" || strings.TrimSpace(request.OwnershipMarker) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		return errors.New("AWS recovery action, fixture, ownership marker, and idempotency key are required")
	}
	if strings.ContainsAny(request.FixtureID+request.OwnershipMarker+request.IdempotencyKey, "\r\n\x00") {
		return errors.New("AWS recovery identities must be single-line")
	}
	return nil
}

func capabilityError(state operationState, reason string) error {
	return sdk.ResilienceCapabilityError{
		AdapterID: AdapterID,
		Operation: state.Action,
		DataClass: state.DataClass,
		Status:    sdk.ResilienceCapabilityUnsupported,
		Reason:    reason,
	}
}

func (api *NativeAPI) archiveBucket() string {
	return api.config.ArchiveBucket
}

func (api *NativeAPI) restoreBucket() string {
	if strings.TrimSpace(api.config.RestoreBucket) != "" {
		return api.config.RestoreBucket
	}
	return api.config.ArchiveBucket
}

func (api *NativeAPI) archivePrefix() string {
	prefix := strings.Trim(strings.TrimSpace(api.config.ArchivePrefix), "/")
	if prefix == "" {
		return defaultArchivePrefix
	}
	return prefix
}

func (api *NativeAPI) archivePrefixForMarker(marker string) string {
	return api.archivePrefix() + "/" + shortDigest(marker)
}

func (api *NativeAPI) objectPrefixForOperation(state operationState) string {
	return api.archivePrefixForMarker(state.OwnershipMarker) + "/" + state.DataClass + "/" + shortDigest(state.IdempotencyKey)
}

func shortDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:24]
}

func (api *NativeAPI) listObjects(ctx context.Context, bucket, prefix string) ([]s3types.Object, error) {
	objects := make([]s3types.Object, 0)
	var token *string
	for {
		output, err := api.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix), ContinuationToken: token})
		if err != nil {
			return nil, err
		}
		if output == nil {
			return nil, errors.New("AWS S3 object listing returned an empty response")
		}
		objects = append(objects, output.Contents...)
		if !aws.ToBool(output.IsTruncated) {
			return objects, nil
		}
		if strings.TrimSpace(aws.ToString(output.NextContinuationToken)) == "" {
			return nil, errors.New("AWS S3 object listing was truncated without a continuation token")
		}
		token = output.NextContinuationToken
	}
}

func readObject(ctx context.Context, api S3API, bucket, key string) ([]byte, error) {
	output, err := api.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return nil, err
	}
	if output == nil || output.Body == nil {
		return nil, errors.New("AWS S3 object response did not contain a body")
	}
	defer output.Body.Close()
	return io.ReadAll(output.Body)
}

func putArchiveObject(ctx context.Context, api S3API, bucket, key string, body []byte, metadata map[string]string, config NativeAPIConfig) error {
	input := &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(body), Metadata: metadata}
	applyS3Encryption(input, config)
	if _, err := api.PutObject(ctx, input); err != nil {
		return err
	}
	return nil
}

func (api *NativeAPI) verifyArchiveObject(ctx context.Context, bucket, key string, state operationState) error {
	output, err := api.s3.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return err
	}
	if output == nil || !metadataMatches(output.Metadata, state) || !api.archiveEncryptionVerified(output) {
		return errors.New("AWS recovery archive object failed ownership or encryption verification")
	}
	return nil
}

func (api *NativeAPI) archiveEncryptionVerified(output *s3.HeadObjectOutput) bool {
	if output == nil || output.ServerSideEncryption == "" {
		return false
	}
	return !api.config.RequireKMS || output.ServerSideEncryption == s3types.ServerSideEncryptionAwsKms
}

func (api *NativeAPI) retentionDays() int {
	if api.config.RetentionDays > 0 {
		return api.config.RetentionDays
	}
	return defaultRetentionDays
}

func applyS3Encryption(input *s3.PutObjectInput, config NativeAPIConfig) {
	if strings.TrimSpace(config.KMSKeyID) != "" {
		input.ServerSideEncryption = s3types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = aws.String(config.KMSKeyID)
		return
	}
	input.ServerSideEncryption = s3types.ServerSideEncryptionAes256
}

func applyS3CopyEncryption(input *s3.CopyObjectInput, config NativeAPIConfig) {
	if strings.TrimSpace(config.KMSKeyID) != "" {
		input.ServerSideEncryption = s3types.ServerSideEncryptionAwsKms
		input.SSEKMSKeyId = aws.String(config.KMSKeyID)
		return
	}
	input.ServerSideEncryption = s3types.ServerSideEncryptionAes256
}

func metadataFor(state operationState, manifest bool) map[string]string {
	metadata := map[string]string{
		archiveOwnershipKey: state.OwnershipMarker,
		archiveClassKey:     state.DataClass,
		archiveFixtureKey:   state.FixtureID,
	}
	if manifest {
		metadata[archiveManifestKey] = "true"
	}
	return metadata
}

func metadataMatches(metadata map[string]string, state operationState) bool {
	return metadata[archiveOwnershipKey] == state.OwnershipMarker && metadata[archiveClassKey] == state.DataClass && metadata[archiveFixtureKey] == state.FixtureID
}

func observationWithOperation(state operationState, operationID string, refs, proofRefs []string, evidence []sdk.ResilienceProofEvidence, status string) provider.NativeOperationObservation {
	return provider.NativeOperationObservation{Status: status, Action: state.Action, OperationID: operationID, ResourceRefs: append([]string(nil), refs...), ProofRefs: append([]string(nil), proofRefs...), OwnershipMarker: state.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true, Evidence: append([]sdk.ResilienceProofEvidence(nil), evidence...)}
}

func parseS3Reference(reference string) (bucket, key string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || (parsed.Scheme != "s3" && parsed.Scheme != "aws-s3") || strings.TrimSpace(parsed.Host) == "" {
		return "", "", fmt.Errorf("S3 resource reference %q must use s3://bucket/prefix", reference)
	}
	key = strings.TrimPrefix(parsed.EscapedPath(), "/")
	key, err = url.PathUnescape(key)
	if err != nil || strings.TrimSpace(key) == "" {
		return "", "", fmt.Errorf("S3 resource reference %q must include a non-empty prefix", reference)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("S3 resource reference %q must not contain query or fragment data", reference)
	}
	return parsed.Host, key, nil
}

func parseArchiveReference(reference string) (bucket, key string, err error) {
	return parseS3Reference(reference)
}

func parseSecretReference(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	for _, scheme := range []string{"aws-secretsmanager://", "secretsmanager://"} {
		if strings.HasPrefix(strings.ToLower(reference), scheme) {
			id := strings.TrimSpace(reference[len(scheme):])
			if id == "" || strings.ContainsAny(id, "\r\n\x00") {
				return "", fmt.Errorf("Secrets Manager resource reference %q has no secret identity", reference)
			}
			return id, nil
		}
	}
	return "", fmt.Errorf("Secrets Manager resource reference %q must use aws-secretsmanager://", reference)
}

type rdsResource struct {
	Kind string
	ID   string
}

func parseRDSReference(reference string) (rdsResource, error) {
	reference = strings.TrimSpace(reference)
	for _, scheme := range []string{"aws-rds://", "rds://"} {
		if strings.HasPrefix(strings.ToLower(reference), scheme) {
			value := strings.TrimPrefix(reference, scheme)
			parts := strings.SplitN(value, "/", 2)
			if len(parts) != 2 || (parts[0] != "instance" && parts[0] != "cluster") || strings.TrimSpace(parts[1]) == "" {
				return rdsResource{}, fmt.Errorf("RDS resource reference %q must use aws-rds://instance/id or aws-rds://cluster/id", reference)
			}
			return rdsResource{Kind: parts[0], ID: parts[1]}, nil
		}
	}
	if strings.HasPrefix(reference, "arn:") {
		parts := strings.SplitN(reference, ":", 6)
		if len(parts) == 6 && parts[2] == "rds" {
			resource := strings.SplitN(parts[5], ":", 2)
			if len(resource) == 2 && (resource[0] == "db" || resource[0] == "cluster") && strings.TrimSpace(resource[1]) != "" {
				kind := "instance"
				if resource[0] == "cluster" {
					kind = "cluster"
				}
				return rdsResource{Kind: kind, ID: resource[1]}, nil
			}
		}
	}
	return rdsResource{}, fmt.Errorf("RDS resource reference %q must be an AWS RDS ARN or aws-rds:// identity", reference)
}

func rdsReference(resource rdsResource) string {
	return "aws-rds://" + resource.Kind + "/" + resource.ID
}

func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "notfound") || strings.Contains(message, "not found") || strings.Contains(message, "nosuchkey") || strings.Contains(message, "404")
}

func operationProof(state operationState, refs, proofRefs []string, evidence []sdk.ResilienceProofEvidence, status string) provider.NativeOperationObservation {
	return provider.NativeOperationObservation{
		Status:              status,
		Action:              state.Action,
		OperationID:         mustEncodeOperationState(state),
		ResourceRefs:        append([]string(nil), refs...),
		ProofRefs:           append([]string(nil), proofRefs...),
		OwnershipMarker:     state.OwnershipMarker,
		OwnershipVerified:   true,
		IdempotencyVerified: true,
		Evidence:            append([]sdk.ResilienceProofEvidence(nil), evidence...),
	}
}

func mustEncodeOperationState(state operationState) string {
	operationID, err := encodeOperationState(state)
	if err != nil {
		return "aws-recovery:v1:invalid"
	}
	return operationID
}

func pendingOperation(state operationState, operationID string) provider.NativeOperationObservation {
	observation := operationProof(state, nil, nil, nil, string(sdk.ResilienceOperationPending))
	observation.OperationID = operationID
	return observation
}

func failedOperation(state operationState, operationID, detail string) provider.NativeOperationObservation {
	observation := operationProof(state, nil, nil, nil, string(sdk.ResilienceOperationFailed))
	observation.OperationID = operationID
	observation.Detail = detail
	return observation
}

func tagValue(tags []rdstypes.Tag, key string) string {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == key {
			return aws.ToString(tag.Value)
		}
	}
	return ""
}

func secretTagValue(tags []secretstypes.Tag, key string) string {
	for _, tag := range tags {
		if aws.ToString(tag.Key) == key {
			return aws.ToString(tag.Value)
		}
	}
	return ""
}

func tagsFor(state operationState) []rdstypes.Tag {
	return []rdstypes.Tag{
		{Key: aws.String(ownershipTagKey), Value: aws.String(state.OwnershipMarker)},
		{Key: aws.String(classTagKey), Value: aws.String(state.DataClass)},
		{Key: aws.String(operationTagKey), Value: aws.String(shortDigest(state.IdempotencyKey))},
	}
}

func secretTagsFor(state operationState) []secretstypes.Tag {
	return []secretstypes.Tag{
		{Key: aws.String(ownershipTagKey), Value: aws.String(state.OwnershipMarker)},
		{Key: aws.String(classTagKey), Value: aws.String(state.DataClass)},
		{Key: aws.String(operationTagKey), Value: aws.String(shortDigest(state.IdempotencyKey))},
	}
}
