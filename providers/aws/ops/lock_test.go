package ops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"github.com/magelift/magelift/internal/s3state"
	awsstack "github.com/magelift/magelift/providers/aws/stack"
)

func TestAcquireDeploymentLockFailsClosedOnS3ClientError(t *testing.T) {
	t.Cleanup(func() { newAWSState = s3state.NewAWS })
	newAWSState = func(context.Context, string, string, string, string, s3state.ObjectEncryption) (*s3state.Manager, error) {
		return nil, errors.New("failed to load AWS config")
	}

	release, err := acquireDeploymentLock(context.Background(), awsLockTestSpec())
	if err == nil {
		t.Fatal("expected S3 client construction to fail closed")
	}
	if release != nil {
		t.Fatal("S3 construct failure must not return a no-op unlock")
	}
	if !strings.Contains(err.Error(), "failed to load AWS config") {
		t.Fatalf("error = %v", err)
	}
}

func TestAcquireDeploymentLockMissingBucketIsBootstrapError(t *testing.T) {
	t.Cleanup(func() { newAWSState = s3state.NewAWS })
	newAWSState = func(_ context.Context, _, bucket, project, environment string, encryption s3state.ObjectEncryption) (*s3state.Manager, error) {
		return s3state.NewManager(&missingBucketS3{}, bucket, project, environment, encryption)
	}

	release, err := acquireDeploymentLock(context.Background(), awsLockTestSpec())
	if release != nil {
		t.Fatal("missing bootstrap bucket must not return a no-op unlock")
	}
	if !errors.Is(err, ErrStateBucketMissing) {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "magelift bootstrap") {
		t.Fatalf("bootstrap guidance missing: %v", err)
	}
}

func awsLockTestSpec() awsstack.Spec {
	return awsstack.Spec{
		Identity: awsstack.Identity{
			Project: "shop", Environment: "staging",
			AccountID: "123456789012", Region: "eu-west-3",
		},
		Dependencies: awsstack.Dependencies{
			KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000",
		},
	}
}

type missingBucketS3 struct{}

func missingBucketErr() error {
	return &smithy.GenericAPIError{Code: "NoSuchBucket", Message: "The specified bucket does not exist"}
}

func (missingBucketS3) HeadObject(context.Context, *s3.HeadObjectInput, ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	return nil, missingBucketErr()
}
func (missingBucketS3) GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	return nil, missingBucketErr()
}
func (missingBucketS3) PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return nil, missingBucketErr()
}
func (missingBucketS3) DeleteObject(context.Context, *s3.DeleteObjectInput, ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	return nil, missingBucketErr()
}
