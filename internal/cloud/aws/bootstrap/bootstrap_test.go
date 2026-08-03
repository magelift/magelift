package bootstrap

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

func testPlan(t *testing.T) Plan {
	t.Helper()
	plan, err := BuildPlan(Spec{Project: "shop", Environment: "production", AccountID: "123456789012", Region: "eu-west-3", AccessLogBucket: "magelift-access-logs-123"})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

type fakeKMS struct {
	mu            sync.Mutex
	keyExists     bool
	aliasExists   bool
	createCalls   int
	aliasCalls    int
	rotationCalls int
	tagCalls      int
	tags          []kmstypes.Tag
	failAliasOnce bool
}

func (f *fakeKMS) DescribeKey(context.Context, *kms.DescribeKeyInput, ...func(*kms.Options)) (*kms.DescribeKeyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.aliasExists {
		return nil, &smithy.GenericAPIError{Code: "NotFoundException", Message: "missing"}
	}
	return &kms.DescribeKeyOutput{KeyMetadata: &kmstypes.KeyMetadata{KeyId: awssdk.String("key-1"), Arn: awssdk.String("arn:aws:kms:eu-west-3:123456789012:key/key-1")}}, nil
}

func (f *fakeKMS) CreateKey(_ context.Context, input *kms.CreateKeyInput, _ ...func(*kms.Options)) (*kms.CreateKeyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	f.keyExists = true
	f.tags = append([]kmstypes.Tag(nil), input.Tags...)
	return &kms.CreateKeyOutput{KeyMetadata: &kmstypes.KeyMetadata{KeyId: awssdk.String("key-1"), Arn: awssdk.String("arn:aws:kms:eu-west-3:123456789012:key/key-1")}}, nil
}

func (f *fakeKMS) CreateAlias(context.Context, *kms.CreateAliasInput, ...func(*kms.Options)) (*kms.CreateAliasOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.aliasCalls++
	if f.failAliasOnce {
		f.failAliasOnce = false
		return nil, errors.New("injected alias failure")
	}
	f.aliasExists = true
	return &kms.CreateAliasOutput{}, nil
}

func (f *fakeKMS) ListKeys(context.Context, *kms.ListKeysInput, ...func(*kms.Options)) (*kms.ListKeysOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	output := &kms.ListKeysOutput{}
	if f.keyExists {
		output.Keys = []kmstypes.KeyListEntry{{KeyId: awssdk.String("key-1"), KeyArn: awssdk.String("arn:aws:kms:eu-west-3:123456789012:key/key-1")}}
	}
	return output, nil
}

func (f *fakeKMS) ListResourceTags(context.Context, *kms.ListResourceTagsInput, ...func(*kms.Options)) (*kms.ListResourceTagsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return &kms.ListResourceTagsOutput{Tags: append([]kmstypes.Tag(nil), f.tags...)}, nil
}

func (f *fakeKMS) EnableKeyRotation(context.Context, *kms.EnableKeyRotationInput, ...func(*kms.Options)) (*kms.EnableKeyRotationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rotationCalls++
	return &kms.EnableKeyRotationOutput{}, nil
}

func (f *fakeKMS) TagResource(_ context.Context, input *kms.TagResourceInput, _ ...func(*kms.Options)) (*kms.TagResourceOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tagCalls++
	if len(input.Tags) != 5 {
		return nil, errors.New("stable KMS tags missing")
	}
	return &kms.TagResourceOutput{}, nil
}

func (f *fakeKMS) PutKeyPolicy(_ context.Context, input *kms.PutKeyPolicyInput, _ ...func(*kms.Options)) (*kms.PutKeyPolicyOutput, error) {
	if input == nil || input.Policy == nil || strings.TrimSpace(*input.Policy) == "" {
		return nil, errors.New("kms policy required")
	}
	if !strings.Contains(*input.Policy, "logs.") {
		return nil, errors.New("kms policy must allow CloudWatch Logs")
	}
	return &kms.PutKeyPolicyOutput{}, nil
}

type fakeS3 struct {
	mu                 sync.Mutex
	exists             bool
	createCalls        int
	publicAccess       *s3.PutPublicAccessBlockInput
	ownership          *s3.PutBucketOwnershipControlsInput
	versioning         *s3.PutBucketVersioningInput
	lifecycle          *s3.PutBucketLifecycleConfigurationInput
	encryption         *s3.PutBucketEncryptionInput
	logging            *s3.PutBucketLoggingInput
	tagging            *s3.PutBucketTaggingInput
	failEncryptionOnce bool
}

func (f *fakeS3) HeadBucket(context.Context, *s3.HeadBucketInput, ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.exists {
		return nil, &smithy.GenericAPIError{Code: "NoSuchBucket", Message: "missing"}
	}
	return &s3.HeadBucketOutput{}, nil
}

func (f *fakeS3) CreateBucket(_ context.Context, input *s3.CreateBucketInput, _ ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	f.exists = true
	if input.CreateBucketConfiguration == nil || input.CreateBucketConfiguration.LocationConstraint != s3types.BucketLocationConstraint("eu-west-3") {
		return nil, errors.New("regional bucket configuration missing")
	}
	return &s3.CreateBucketOutput{}, nil
}

func (f *fakeS3) PutPublicAccessBlock(_ context.Context, input *s3.PutPublicAccessBlockInput, _ ...func(*s3.Options)) (*s3.PutPublicAccessBlockOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.publicAccess = input
	return &s3.PutPublicAccessBlockOutput{}, nil
}

func (f *fakeS3) PutBucketOwnershipControls(_ context.Context, input *s3.PutBucketOwnershipControlsInput, _ ...func(*s3.Options)) (*s3.PutBucketOwnershipControlsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ownership = input
	return &s3.PutBucketOwnershipControlsOutput{}, nil
}

func (f *fakeS3) PutBucketVersioning(_ context.Context, input *s3.PutBucketVersioningInput, _ ...func(*s3.Options)) (*s3.PutBucketVersioningOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.versioning = input
	return &s3.PutBucketVersioningOutput{}, nil
}

func (f *fakeS3) PutBucketLifecycleConfiguration(_ context.Context, input *s3.PutBucketLifecycleConfigurationInput, _ ...func(*s3.Options)) (*s3.PutBucketLifecycleConfigurationOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lifecycle = input
	return &s3.PutBucketLifecycleConfigurationOutput{}, nil
}

func (f *fakeS3) PutBucketEncryption(_ context.Context, input *s3.PutBucketEncryptionInput, _ ...func(*s3.Options)) (*s3.PutBucketEncryptionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.encryption = input
	if f.failEncryptionOnce {
		f.failEncryptionOnce = false
		return nil, errors.New("injected encryption failure")
	}
	return &s3.PutBucketEncryptionOutput{}, nil
}

func (f *fakeS3) PutBucketLogging(_ context.Context, input *s3.PutBucketLoggingInput, _ ...func(*s3.Options)) (*s3.PutBucketLoggingOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.logging = input
	return &s3.PutBucketLoggingOutput{}, nil
}

func (f *fakeS3) PutBucketTagging(_ context.Context, input *s3.PutBucketTaggingInput, _ ...func(*s3.Options)) (*s3.PutBucketTaggingOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tagging = input
	return &s3.PutBucketTaggingOutput{}, nil
}

func TestBuildPlanIsDeterministic(t *testing.T) {
	first := testPlan(t)
	second := testPlan(t)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("plans differ: %#v %#v", first, second)
	}
	if first.StateBucket != "magelift-123456789012-eu-west-3-shop-production-state" || first.KMSAlias != "alias/magelift/shop/production/state" {
		t.Fatalf("unexpected plan: %#v", first)
	}
}

func TestEnsureCreatesAndReconcilesStateProtection(t *testing.T) {
	s3Client, kmsClient := &fakeS3{}, &fakeKMS{}
	plan := testPlan(t)
	result, err := New(s3Client, kmsClient).Ensure(context.Background(), plan, "eu-west-3")
	if err != nil {
		t.Fatal(err)
	}
	if result.KeyARN == "" || s3Client.createCalls != 1 || kmsClient.createCalls != 1 || kmsClient.aliasCalls != 1 || kmsClient.rotationCalls != 1 {
		t.Fatalf("unexpected result or calls: %#v s3=%#v kms=%#v", result, s3Client, kmsClient)
	}
	public := s3Client.publicAccess.PublicAccessBlockConfiguration
	if !awssdk.ToBool(public.BlockPublicAcls) || !awssdk.ToBool(public.BlockPublicPolicy) || !awssdk.ToBool(public.IgnorePublicAcls) || !awssdk.ToBool(public.RestrictPublicBuckets) {
		t.Fatal("public access is not fully blocked")
	}
	if got := s3Client.ownership.OwnershipControls.Rules[0].ObjectOwnership; got != s3types.ObjectOwnershipBucketOwnerEnforced {
		t.Fatalf("ownership = %q", got)
	}
	if got := s3Client.versioning.VersioningConfiguration.Status; got != s3types.BucketVersioningStatusEnabled {
		t.Fatalf("versioning = %q", got)
	}
	if s3Client.lifecycle == nil || s3Client.lifecycle.LifecycleConfiguration == nil || len(s3Client.lifecycle.LifecycleConfiguration.Rules) != 2 {
		t.Fatalf("state lifecycle policy is incomplete: %#v", s3Client.lifecycle)
	}
	backupRule := s3Client.lifecycle.LifecycleConfiguration.Rules[0]
	if awssdk.ToString(backupRule.ID) != "magelift-backup-retention" || backupRule.Status != s3types.ExpirationStatusEnabled || awssdk.ToString(backupRule.Filter.Prefix) != "backups/" || awssdk.ToInt32(backupRule.Expiration.Days) != stateBackupRetentionDays || awssdk.ToInt32(backupRule.NoncurrentVersionExpiration.NoncurrentDays) != stateBackupRetentionDays || awssdk.ToInt32(backupRule.AbortIncompleteMultipartUpload.DaysAfterInitiation) != stateMultipartAbortDays {
		t.Fatalf("backup lifecycle rule = %#v", backupRule)
	}
	uploadRule := s3Client.lifecycle.LifecycleConfiguration.Rules[1]
	if awssdk.ToString(uploadRule.ID) != "magelift-abort-incomplete-uploads" || awssdk.ToString(uploadRule.Filter.Prefix) != "" || awssdk.ToInt32(uploadRule.AbortIncompleteMultipartUpload.DaysAfterInitiation) != stateMultipartAbortDays {
		t.Fatalf("multipart lifecycle rule = %#v", uploadRule)
	}
	rule := s3Client.encryption.ServerSideEncryptionConfiguration.Rules[0]
	if rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm != s3types.ServerSideEncryptionAwsKms || awssdk.ToString(rule.ApplyServerSideEncryptionByDefault.KMSMasterKeyID) != result.KeyARN || !awssdk.ToBool(rule.BucketKeyEnabled) {
		t.Fatal("SSE-KMS configuration is incomplete")
	}
	logging := s3Client.logging.BucketLoggingStatus.LoggingEnabled
	if awssdk.ToString(logging.TargetBucket) != plan.AccessLogBucket || awssdk.ToString(logging.TargetPrefix) != plan.AccessLogPrefix {
		t.Fatal("access logging target is incorrect")
	}
	if len(s3Client.tagging.Tagging.TagSet) != len(plan.Tags) {
		t.Fatal("stable bucket tags are incomplete")
	}
}

func TestEnsureIsIdempotent(t *testing.T) {
	s3Client, kmsClient := &fakeS3{}, &fakeKMS{}
	bootstrapper := New(s3Client, kmsClient)
	plan := testPlan(t)
	for range 2 {
		if _, err := bootstrapper.Ensure(context.Background(), plan, "eu-west-3"); err != nil {
			t.Fatal(err)
		}
	}
	if s3Client.createCalls != 1 || kmsClient.createCalls != 1 || kmsClient.aliasCalls != 1 {
		t.Fatalf("resources recreated: s3=%d kms=%d aliases=%d", s3Client.createCalls, kmsClient.createCalls, kmsClient.aliasCalls)
	}
	if kmsClient.rotationCalls != 2 || kmsClient.tagCalls != 2 {
		t.Fatal("existing KMS settings were not reconciled")
	}
}

func TestEnsureRejectsExistingKMSAliasWithoutEnvironmentOwnership(t *testing.T) {
	s3Client := &fakeS3{exists: true}
	kmsClient := &fakeKMS{
		keyExists:   true,
		aliasExists: true,
		tags: []kmstypes.Tag{{
			TagKey:   awssdk.String("magelift:bootstrap-id"),
			TagValue: awssdk.String("123456789012:eu-west-3:another-project:production"),
		}},
	}

	_, err := New(s3Client, kmsClient).Ensure(context.Background(), testPlan(t), "eu-west-3")
	if err == nil || !strings.Contains(err.Error(), "not owned by this environment") {
		t.Fatalf("error = %v", err)
	}
	if kmsClient.tagCalls != 0 || kmsClient.rotationCalls != 0 || s3Client.publicAccess != nil {
		t.Fatal("bootstrap mutated resources before verifying KMS ownership")
	}
}

func TestEnsureRecoversAfterPartialBucketFailure(t *testing.T) {
	s3Client, kmsClient := &fakeS3{failEncryptionOnce: true}, &fakeKMS{}
	bootstrapper := New(s3Client, kmsClient)
	plan := testPlan(t)
	if _, err := bootstrapper.Ensure(context.Background(), plan, "eu-west-3"); err == nil {
		t.Fatal("injected failure was ignored")
	}
	if _, err := bootstrapper.Ensure(context.Background(), plan, "eu-west-3"); err != nil {
		t.Fatal(err)
	}
	if s3Client.createCalls != 1 || kmsClient.createCalls != 1 || kmsClient.aliasCalls != 1 || s3Client.logging == nil || s3Client.tagging == nil {
		t.Fatal("retry recreated resources or did not finish reconciliation")
	}
}

func TestEnsureRecoversKeyCreatedBeforeAliasFailure(t *testing.T) {
	s3Client, kmsClient := &fakeS3{}, &fakeKMS{failAliasOnce: true}
	bootstrapper := New(s3Client, kmsClient)
	plan := testPlan(t)
	if _, err := bootstrapper.Ensure(context.Background(), plan, "eu-west-3"); err == nil {
		t.Fatal("injected alias failure was ignored")
	}
	if _, err := bootstrapper.Ensure(context.Background(), plan, "eu-west-3"); err != nil {
		t.Fatal(err)
	}
	if kmsClient.createCalls != 1 || kmsClient.aliasCalls != 2 {
		t.Fatalf("orphan recovery calls: keys=%d aliases=%d", kmsClient.createCalls, kmsClient.aliasCalls)
	}
}
