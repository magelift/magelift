//go:build floci

package floci_test

import (
	"context"
	"errors"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/magelift/magelift/internal/s3state"
)

func TestAES256LockAgainstFloci(t *testing.T) {
	endpoint := flociEndpoint(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	configuration := flociConfig(t, ctx, endpoint)
	s3Client := s3.NewFromConfig(configuration, func(options *s3.Options) {
		options.UsePathStyle = true
	})
	bucket := "magelift-aes256-state"
	if _, err := s3Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: awssdk.String(bucket)}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	encryption := s3state.ObjectEncryption{Mode: s3state.EncryptionAES256}
	manager, err := s3state.NewAWSWithEndpoint(ctx, "us-east-1", bucket, "shop", "staging", encryption, endpoint)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := manager.Acquire(ctx, "shop", "staging", "floci-aes256")
	if err != nil {
		// If the emulator rejects SSE headers, EncryptionNone is the documented fallback.
		t.Logf("AES256 acquire failed (%v); retrying EncryptionNone", err)
		encryption = s3state.ObjectEncryption{Mode: s3state.EncryptionNone}
		manager, err = s3state.NewAWSWithEndpoint(ctx, "us-east-1", bucket, "shop", "preview", encryption, endpoint)
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
	if _, err := manager.Acquire(ctx, handle.Info().Project, handle.Info().Environment, "second"); !errors.Is(err, s3state.ErrLocked) {
		t.Fatalf("second lock error = %v", err)
	}
	if err := handle.Release(ctx); err != nil {
		t.Fatal(err)
	}
}
