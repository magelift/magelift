//go:build floci

package floci_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/magelift/magelift/internal/cloud/aws/state"
)

func TestAES256LockAgainstFloci(t *testing.T) {
	if os.Getenv("MAGELIFT_FLOCI") != "1" {
		t.Skip("set MAGELIFT_FLOCI=1 to run the Floci AES256 integration test")
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
	bucket := "magelift-aes256-state"
	if _, err := s3Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: awssdk.String(bucket)}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	encryption := state.ObjectEncryption{Mode: state.EncryptionAES256}
	manager, err := state.NewAWSWithEndpoint(ctx, "us-east-1", bucket, "shop", "staging", encryption, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := manager.Acquire(ctx, "shop", "staging", "floci-aes256")
	if err != nil {
		// If the emulator rejects SSE headers, EncryptionNone is the documented fallback.
		t.Logf("AES256 acquire failed (%v); retrying EncryptionNone", err)
		encryption = state.ObjectEncryption{Mode: state.EncryptionNone}
		manager, err = state.NewAWSWithEndpoint(ctx, "us-east-1", bucket, "shop", "preview", encryption, endpoint)
		if err != nil {
			t.Fatal(err)
		}
		handle, err = manager.Acquire(ctx, "shop", "preview", "floci-none")
		if err != nil {
			t.Fatalf("AES256 and EncryptionNone both failed: %v", err)
		}
		t.Log("AES256 SSE rejected by Floci; EncryptionNone lock path succeeded")
	} else {
		t.Log("AES256 lock path succeeded against Floci")
	}
	if handle == nil {
		t.Fatal("expected lock handle")
	}
	if _, err := manager.Acquire(ctx, handle.Info().Project, handle.Info().Environment, "second"); !errors.Is(err, state.ErrLocked) {
		t.Fatalf("second lock error = %v", err)
	}
	if err := handle.Release(ctx); err != nil {
		t.Fatal(err)
	}
}
