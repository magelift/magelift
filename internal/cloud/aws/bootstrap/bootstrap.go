package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

var componentPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)
var accountPattern = regexp.MustCompile(`^[0-9]{12}$`)
var regionPattern = regexp.MustCompile(`^[a-z]{2}(?:-gov)?-[a-z]+-[0-9]+$`)
var bucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

const (
	stateBackupRetentionDays = 90
	stateMultipartAbortDays  = 7
)

type Spec struct {
	Project         string
	Environment     string
	AccountID       string
	Region          string
	AccessLogBucket string
}

type Plan struct {
	StateBucket     string            `json:"stateBucket" yaml:"stateBucket"`
	AccessLogBucket string            `json:"accessLogBucket" yaml:"accessLogBucket"`
	AccessLogPrefix string            `json:"accessLogPrefix" yaml:"accessLogPrefix"`
	KMSAlias        string            `json:"kmsAlias" yaml:"kmsAlias"`
	AccountID       string            `json:"accountId,omitempty" yaml:"accountId,omitempty"`
	Region          string            `json:"region,omitempty" yaml:"region,omitempty"`
	Tags            map[string]string `json:"tags" yaml:"tags"`
}

type Result struct {
	Plan   Plan   `json:"plan" yaml:"plan"`
	KeyARN string `json:"keyArn" yaml:"keyArn"`
}

func BuildPlan(spec Spec) (Plan, error) {
	if !componentPattern.MatchString(spec.Project) {
		return Plan{}, errors.New("bootstrap project must be a lowercase stable name")
	}
	if !componentPattern.MatchString(spec.Environment) {
		return Plan{}, errors.New("bootstrap environment must be a lowercase stable name")
	}
	if !accountPattern.MatchString(spec.AccountID) {
		return Plan{}, errors.New("bootstrap account ID must contain 12 digits")
	}
	if !regionPattern.MatchString(spec.Region) {
		return Plan{}, errors.New("bootstrap region is invalid")
	}
	if !bucketPattern.MatchString(spec.AccessLogBucket) {
		return Plan{}, errors.New("bootstrap access log bucket is invalid")
	}
	bucket := strings.Join([]string{"magelift", spec.AccountID, spec.Region, spec.Project, spec.Environment, "state"}, "-")
	if len(bucket) > 63 {
		return Plan{}, errors.New("generated state bucket name exceeds 63 characters")
	}
	bootstrapID := spec.AccountID + ":" + spec.Region + ":" + spec.Project + ":" + spec.Environment
	return Plan{
		StateBucket:     bucket,
		AccessLogBucket: spec.AccessLogBucket,
		AccessLogPrefix: "pulumi-state/" + bucket + "/",
		KMSAlias:        "alias/magelift/" + spec.Project + "/" + spec.Environment + "/state",
		AccountID:       spec.AccountID,
		Region:          spec.Region,
		Tags: map[string]string{
			"magelift:bootstrap-id": bootstrapID,
			"magelift:environment":  spec.Environment,
			"magelift:managed-by":   "magelift",
			"magelift:project":      spec.Project,
			"magelift:purpose":      "pulumi-state",
		},
	}, nil
}

type S3API interface {
	HeadBucket(context.Context, *s3.HeadBucketInput, ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	CreateBucket(context.Context, *s3.CreateBucketInput, ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
	PutPublicAccessBlock(context.Context, *s3.PutPublicAccessBlockInput, ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error)
	PutBucketOwnershipControls(context.Context, *s3.PutBucketOwnershipControlsInput, ...func(*s3.Options)) (*s3.PutBucketOwnershipControlsOutput, error)
	PutBucketVersioning(context.Context, *s3.PutBucketVersioningInput, ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error)
	PutBucketLifecycleConfiguration(context.Context, *s3.PutBucketLifecycleConfigurationInput, ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error)
	PutBucketEncryption(context.Context, *s3.PutBucketEncryptionInput, ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error)
	PutBucketLogging(context.Context, *s3.PutBucketLoggingInput, ...func(*s3.Options)) (*s3.PutBucketLoggingOutput, error)
	PutBucketTagging(context.Context, *s3.PutBucketTaggingInput, ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error)
}

type KMSAPI interface {
	DescribeKey(context.Context, *kms.DescribeKeyInput, ...func(*kms.Options)) (*kms.DescribeKeyOutput, error)
	CreateKey(context.Context, *kms.CreateKeyInput, ...func(*kms.Options)) (*kms.CreateKeyOutput, error)
	CreateAlias(context.Context, *kms.CreateAliasInput, ...func(*kms.Options)) (*kms.CreateAliasOutput, error)
	EnableKeyRotation(context.Context, *kms.EnableKeyRotationInput, ...func(*kms.Options)) (*kms.EnableKeyRotationOutput, error)
	TagResource(context.Context, *kms.TagResourceInput, ...func(*kms.Options)) (*kms.TagResourceOutput, error)
	PutKeyPolicy(context.Context, *kms.PutKeyPolicyInput, ...func(*kms.Options)) (*kms.PutKeyPolicyOutput, error)
	ListKeys(context.Context, *kms.ListKeysInput, ...func(*kms.Options)) (*kms.ListKeysOutput, error)
	ListResourceTags(context.Context, *kms.ListResourceTagsInput, ...func(*kms.Options)) (*kms.ListResourceTagsOutput, error)
}

type Bootstrapper struct {
	s3  S3API
	kms KMSAPI
}

type Ensurer interface {
	Ensure(context.Context, Plan, string) (Result, error)
}

func New(s3Client S3API, kmsClient KMSAPI) *Bootstrapper {
	return &Bootstrapper{s3: s3Client, kms: kmsClient}
}

func (b *Bootstrapper) Ensure(ctx context.Context, plan Plan, region string) (Result, error) {
	if b == nil || b.s3 == nil || b.kms == nil {
		return Result{}, errors.New("AWS bootstrap clients are required")
	}
	if err := validatePlan(plan); err != nil {
		return Result{}, err
	}
	keyARN, err := b.ensureKey(ctx, plan)
	if err != nil {
		return Result{}, err
	}
	if err := b.ensureBucket(ctx, plan, region, keyARN); err != nil {
		return Result{}, err
	}
	return Result{Plan: plan, KeyARN: keyARN}, nil
}

func validatePlan(plan Plan) error {
	if !bucketPattern.MatchString(plan.StateBucket) || !bucketPattern.MatchString(plan.AccessLogBucket) {
		return errors.New("bootstrap plan contains an invalid bucket")
	}
	if !strings.HasPrefix(plan.KMSAlias, "alias/magelift/") || plan.AccessLogPrefix == "" {
		return errors.New("bootstrap plan contains invalid state protection settings")
	}
	if plan.Tags["magelift:managed-by"] != "magelift" || plan.Tags["magelift:bootstrap-id"] == "" {
		return errors.New("bootstrap plan is missing stable ownership tags")
	}
	return nil
}

func (b *Bootstrapper) ensureKey(ctx context.Context, plan Plan) (string, error) {
	described, err := b.kms.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: awssdk.String(plan.KMSAlias)})
	var keyID, keyARN string
	if err == nil {
		if described == nil || described.KeyMetadata == nil {
			return "", errors.New("describe bootstrap KMS key returned no metadata")
		}
		keyID, keyARN = awssdk.ToString(described.KeyMetadata.KeyId), awssdk.ToString(described.KeyMetadata.Arn)
		if keyID == "" || keyARN == "" {
			return "", errors.New("bootstrap KMS key metadata is incomplete")
		}
		owned, ownershipErr := b.keyHasBootstrapID(ctx, keyID, plan.Tags["magelift:bootstrap-id"])
		if ownershipErr != nil {
			return "", ownershipErr
		}
		if !owned {
			return "", errors.New("bootstrap KMS alias resolves to a key not owned by this environment")
		}
	} else if isNotFound(err) {
		keyID, keyARN, err = b.findUnaliasedKey(ctx, plan.Tags["magelift:bootstrap-id"])
		if err != nil {
			return "", err
		}
		if keyID == "" {
			created, createErr := b.kms.CreateKey(ctx, &kms.CreateKeyInput{
				Description: awssdk.String("MageLift Pulumi state encryption"),
				KeySpec:     kmstypes.KeySpecSymmetricDefault,
				KeyUsage:    kmstypes.KeyUsageTypeEncryptDecrypt,
				Policy:      awssdk.String(kmsKeyPolicy(plan.AccountID, plan.Region)),
				Tags:        kmsTags(plan.Tags),
			})
			if createErr != nil {
				return "", fmt.Errorf("create bootstrap KMS key: %w", createErr)
			}
			if created == nil || created.KeyMetadata == nil {
				return "", errors.New("create bootstrap KMS key returned no metadata")
			}
			keyID, keyARN = awssdk.ToString(created.KeyMetadata.KeyId), awssdk.ToString(created.KeyMetadata.Arn)
		}
		if _, aliasErr := b.kms.CreateAlias(ctx, &kms.CreateAliasInput{AliasName: awssdk.String(plan.KMSAlias), TargetKeyId: awssdk.String(keyID)}); aliasErr != nil {
			return "", fmt.Errorf("create bootstrap KMS alias: %w", aliasErr)
		}
	} else {
		return "", fmt.Errorf("describe bootstrap KMS key: %w", err)
	}
	if keyID == "" || keyARN == "" {
		return "", errors.New("bootstrap KMS key metadata is incomplete")
	}
	if _, err := b.kms.TagResource(ctx, &kms.TagResourceInput{KeyId: awssdk.String(keyID), Tags: kmsTags(plan.Tags)}); err != nil {
		return "", fmt.Errorf("tag bootstrap KMS key: %w", err)
	}
	if _, err := b.kms.EnableKeyRotation(ctx, &kms.EnableKeyRotationInput{KeyId: awssdk.String(keyID)}); err != nil {
		return "", fmt.Errorf("enable bootstrap KMS key rotation: %w", err)
	}
	if _, err := b.kms.PutKeyPolicy(ctx, &kms.PutKeyPolicyInput{
		KeyId: awssdk.String(keyID), PolicyName: awssdk.String("default"), Policy: awssdk.String(kmsKeyPolicy(plan.AccountID, plan.Region)),
	}); err != nil {
		return "", fmt.Errorf("set bootstrap KMS key policy: %w", err)
	}
	return keyARN, nil
}

func kmsKeyPolicy(accountID, region string) string {
	return `{
  "Version": "2012-10-17",
  "Id": "magelift-bootstrap-key",
  "Statement": [
    {
      "Sid": "EnableIAMUserPermissions",
      "Effect": "Allow",
      "Principal": {"AWS": "arn:aws:iam::` + accountID + `:root"},
      "Action": "kms:*",
      "Resource": "*"
    },
    {
      "Sid": "AllowCloudWatchLogs",
      "Effect": "Allow",
      "Principal": {"Service": "logs.` + region + `.amazonaws.com"},
      "Action": ["kms:Encrypt","kms:Decrypt","kms:ReEncrypt*","kms:GenerateDataKey*","kms:DescribeKey"],
      "Resource": "*",
      "Condition": {
        "ArnLike": {
          "kms:EncryptionContext:aws:logs:arn": "arn:aws:logs:` + region + `:` + accountID + `:*"
        }
      }
    }
  ]
}`
}

func (b *Bootstrapper) findUnaliasedKey(ctx context.Context, bootstrapID string) (string, string, error) {
	var marker *string
	var foundID, foundARN string
	for {
		page, err := b.kms.ListKeys(ctx, &kms.ListKeysInput{Marker: marker})
		if err != nil {
			return "", "", fmt.Errorf("list bootstrap KMS keys: %w", err)
		}
		if page == nil {
			return "", "", errors.New("list bootstrap KMS keys returned no response")
		}
		for _, key := range page.Keys {
			matches, err := b.keyHasBootstrapID(ctx, awssdk.ToString(key.KeyId), bootstrapID)
			if err != nil {
				return "", "", err
			}
			if matches {
				if foundID != "" {
					return "", "", errors.New("multiple unaliased bootstrap KMS keys have the same ownership tag")
				}
				foundID, foundARN = awssdk.ToString(key.KeyId), awssdk.ToString(key.KeyArn)
			}
		}
		if !page.Truncated {
			return foundID, foundARN, nil
		}
		if page.NextMarker == nil {
			return "", "", errors.New("KMS key listing was truncated without a continuation marker")
		}
		marker = page.NextMarker
	}
}

func (b *Bootstrapper) keyHasBootstrapID(ctx context.Context, keyID, bootstrapID string) (bool, error) {
	var marker *string
	for {
		page, err := b.kms.ListResourceTags(ctx, &kms.ListResourceTagsInput{KeyId: awssdk.String(keyID), Marker: marker})
		if err != nil {
			return false, fmt.Errorf("inspect bootstrap KMS key tags: %w", err)
		}
		if page == nil {
			return false, errors.New("inspect bootstrap KMS key tags returned no response")
		}
		for _, tag := range page.Tags {
			if awssdk.ToString(tag.TagKey) == "magelift:bootstrap-id" && awssdk.ToString(tag.TagValue) == bootstrapID {
				return true, nil
			}
		}
		if !page.Truncated {
			return false, nil
		}
		if page.NextMarker == nil {
			return false, errors.New("KMS tag listing was truncated without a continuation marker")
		}
		marker = page.NextMarker
	}
}

func (b *Bootstrapper) ensureBucket(ctx context.Context, plan Plan, region, keyARN string) error {
	_, err := b.s3.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: awssdk.String(plan.StateBucket)})
	if err != nil {
		if !isNotFound(err) {
			return fmt.Errorf("inspect bootstrap state bucket: %w", err)
		}
		input := &s3.CreateBucketInput{Bucket: awssdk.String(plan.StateBucket)}
		if region != "us-east-1" {
			input.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{LocationConstraint: s3types.BucketLocationConstraint(region)}
		}
		if _, err = b.s3.CreateBucket(ctx, input); err != nil {
			return fmt.Errorf("create bootstrap state bucket: %w", err)
		}
	}
	bucket := awssdk.String(plan.StateBucket)
	if _, err = b.s3.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{Bucket: bucket, PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{BlockPublicAcls: awssdk.Bool(true), BlockPublicPolicy: awssdk.Bool(true), IgnorePublicAcls: awssdk.Bool(true), RestrictPublicBuckets: awssdk.Bool(true)}}); err != nil {
		return fmt.Errorf("protect bootstrap state bucket public access: %w", err)
	}
	if _, err = b.s3.PutBucketOwnershipControls(ctx, &s3.PutBucketOwnershipControlsInput{Bucket: bucket, OwnershipControls: &s3types.OwnershipControls{Rules: []s3types.OwnershipControlsRule{{ObjectOwnership: s3types.ObjectOwnershipBucketOwnerEnforced}}}}); err != nil {
		return fmt.Errorf("set bootstrap state bucket ownership: %w", err)
	}
	if _, err = b.s3.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{Bucket: bucket, VersioningConfiguration: &s3types.VersioningConfiguration{Status: s3types.BucketVersioningStatusEnabled}}); err != nil {
		return fmt.Errorf("enable bootstrap state versioning: %w", err)
	}
	if _, err = b.s3.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{Bucket: bucket, LifecycleConfiguration: &s3types.BucketLifecycleConfiguration{Rules: stateLifecycleRules()}}); err != nil {
		return fmt.Errorf("configure bootstrap state retention: %w", err)
	}
	if _, err = b.s3.PutBucketEncryption(ctx, &s3.PutBucketEncryptionInput{Bucket: bucket, ServerSideEncryptionConfiguration: &s3types.ServerSideEncryptionConfiguration{Rules: []s3types.ServerSideEncryptionRule{{ApplyServerSideEncryptionByDefault: &s3types.ServerSideEncryptionByDefault{SSEAlgorithm: s3types.ServerSideEncryptionAwsKms, KMSMasterKeyID: awssdk.String(keyARN)}, BucketKeyEnabled: awssdk.Bool(true)}}}}); err != nil {
		return fmt.Errorf("encrypt bootstrap state bucket: %w", err)
	}
	if _, err = b.s3.PutBucketLogging(ctx, &s3.PutBucketLoggingInput{Bucket: bucket, BucketLoggingStatus: &s3types.BucketLoggingStatus{LoggingEnabled: &s3types.LoggingEnabled{TargetBucket: awssdk.String(plan.AccessLogBucket), TargetPrefix: awssdk.String(plan.AccessLogPrefix)}}}); err != nil {
		return fmt.Errorf("enable bootstrap state access logging: %w", err)
	}
	if _, err = b.s3.PutBucketTagging(ctx, &s3.PutBucketTaggingInput{Bucket: bucket, Tagging: &s3types.Tagging{TagSet: s3Tags(plan.Tags)}}); err != nil {
		return fmt.Errorf("tag bootstrap state bucket: %w", err)
	}
	return nil
}

func stateLifecycleRules() []s3types.LifecycleRule {
	return []s3types.LifecycleRule{
		{
			ID:                             awssdk.String("magelift-backup-retention"),
			Status:                         s3types.ExpirationStatusEnabled,
			Filter:                         &s3types.LifecycleRuleFilter{Prefix: awssdk.String("backups/")},
			Expiration:                     &s3types.LifecycleExpiration{Days: awssdk.Int32(stateBackupRetentionDays)},
			NoncurrentVersionExpiration:    &s3types.NoncurrentVersionExpiration{NoncurrentDays: awssdk.Int32(stateBackupRetentionDays)},
			AbortIncompleteMultipartUpload: &s3types.AbortIncompleteMultipartUpload{DaysAfterInitiation: awssdk.Int32(stateMultipartAbortDays)},
		},
		{
			ID:                             awssdk.String("magelift-abort-incomplete-uploads"),
			Status:                         s3types.ExpirationStatusEnabled,
			Filter:                         &s3types.LifecycleRuleFilter{Prefix: awssdk.String("")},
			AbortIncompleteMultipartUpload: &s3types.AbortIncompleteMultipartUpload{DaysAfterInitiation: awssdk.Int32(stateMultipartAbortDays)},
		},
	}
}

func kmsTags(tags map[string]string) []kmstypes.Tag {
	keys := sortedKeys(tags)
	result := make([]kmstypes.Tag, 0, len(keys))
	for _, key := range keys {
		result = append(result, kmstypes.Tag{TagKey: awssdk.String(key), TagValue: awssdk.String(tags[key])})
	}
	return result
}

func s3Tags(tags map[string]string) []s3types.Tag {
	keys := sortedKeys(tags)
	result := make([]s3types.Tag, 0, len(keys))
	for _, key := range keys {
		result = append(result, s3types.Tag{Key: awssdk.String(key), Value: awssdk.String(tags[key])})
	}
	return result
}

func sortedKeys(tags map[string]string) []string {
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func isNotFound(err error) bool {
	var apiError smithy.APIError
	if errors.As(err, &apiError) && (apiError.ErrorCode() == "NotFound" || apiError.ErrorCode() == "NotFoundException" || apiError.ErrorCode() == "NoSuchBucket" || apiError.ErrorCode() == "NoSuchEntity" || apiError.ErrorCode() == "NoSuchEntityException" || apiError.ErrorCode() == "ResourceNotFoundException" || apiError.ErrorCode() == "ParameterNotFound") {
		return true
	}
	var responseError interface{ HTTPStatusCode() int }
	return errors.As(err, &responseError) && responseError.HTTPStatusCode() == 404
}
