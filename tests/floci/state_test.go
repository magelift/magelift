//go:build floci

package floci_test

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/acourtiol/magelift/internal/cloud/aws/bootstrap"
	"github.com/acourtiol/magelift/internal/cloud/aws/state"
)

func TestBootstrapStateAndLockAgainstFloci(t *testing.T) {
	if os.Getenv("MAGELIFT_FLOCI") != "1" {
		t.Skip("set MAGELIFT_FLOCI=1 to run the Floci integration test")
	}
	endpoint := os.Getenv("MAGELIFT_FLOCI_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:4566"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	configuration, err := awscfg.LoadDefaultConfig(ctx,
		awscfg.WithRegion("us-east-1"),
		awscfg.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	s3Client := s3.NewFromConfig(configuration, func(options *s3.Options) {
		options.BaseEndpoint = awssdk.String(endpoint)
		options.UsePathStyle = true
	})
	kmsClient := kms.NewFromConfig(configuration, func(options *kms.Options) {
		options.BaseEndpoint = awssdk.String(endpoint)
	})

	accessLogBucket := "magelift-access-logs"
	if _, err := s3Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: awssdk.String(accessLogBucket)}); err != nil {
		t.Fatal(err)
	}
	plan, err := bootstrap.BuildPlan(bootstrap.Spec{
		Project:         "shop",
		Environment:     "staging",
		AccountID:       "000000000000",
		Region:          "us-east-1",
		AccessLogBucket: accessLogBucket,
	})
	if err != nil {
		t.Fatal(err)
	}
	backend := bootstrap.New(s3Client, kmsClient)
	result, err := backend.Ensure(ctx, plan, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Ensure(ctx, plan, "us-east-1"); err != nil {
		t.Fatalf("second bootstrap was not idempotent: %v", err)
	}

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("MAGELIFT_AWS_ENDPOINT_URL", endpoint)
	manager, err := state.NewAWS(ctx, "us-east-1", plan.StateBucket, "shop", "staging", result.KeyARN)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := manager.Acquire(ctx, "shop", "staging", "floci-test")
	if err != nil {
		t.Fatal(err)
	}
	if got := handle.Info().Owner; got != "floci-test" {
		t.Fatalf("lock owner = %q", got)
	}
	if _, err := manager.Acquire(ctx, "shop", "staging", "second-owner"); !errors.Is(err, state.ErrLocked) {
		t.Fatalf("second lock error = %v", err)
	}
	if err := handle.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Status(ctx); !errors.Is(err, state.ErrNotLocked) {
		t.Fatalf("lock status after release = %v", err)
	}

	if _, err := s3Client.PutObject(ctx, &s3.PutObjectInput{Bucket: awssdk.String(plan.StateBucket), Key: awssdk.String("stacks/shop.json"), Body: strings.NewReader("state-v1")}); err != nil {
		t.Fatal(err)
	}
	archive, err := state.NewAWSArchive(ctx, "us-east-1", plan.StateBucket, result.KeyARN)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := archive.Backup(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s3Client.PutObject(ctx, &s3.PutObjectInput{Bucket: awssdk.String(plan.StateBucket), Key: awssdk.String("stacks/shop.json"), Body: strings.NewReader("state-v2")}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Restore(ctx, backup.ID); err != nil {
		t.Fatal(err)
	}
	object, err := s3Client.GetObject(ctx, &s3.GetObjectInput{Bucket: awssdk.String(plan.StateBucket), Key: awssdk.String("stacks/shop.json")})
	if err != nil {
		t.Fatal(err)
	}
	defer object.Body.Close()
	content, err := io.ReadAll(object.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "state-v1" {
		t.Fatalf("restored state = %q", content)
	}
}
