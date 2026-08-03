//go:build floci

package floci_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/mediasync"
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestMediaSyncListingDiffAgainstFloci(t *testing.T) {
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
	client := s3.NewFromConfig(configuration, func(options *s3.Options) {
		options.BaseEndpoint = awssdk.String(endpoint)
		options.UsePathStyle = true
	})
	bucket := "magelift-floci-media-sync"
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: awssdk.String(bucket)}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		listed, listErr := client.ListObjectsV2(context.Background(), &s3.ListObjectsV2Input{Bucket: awssdk.String(bucket)})
		if listErr != nil {
			return
		}
		for _, object := range listed.Contents {
			_, _ = client.DeleteObject(context.Background(), &s3.DeleteObjectInput{
				Bucket: awssdk.String(bucket),
				Key:    object.Key,
			})
		}
		_, _ = client.DeleteBucket(context.Background(), &s3.DeleteBucketInput{Bucket: awssdk.String(bucket)})
	}()

	source, err := filepath.Abs(filepath.Join("..", "..", "testdata", "fixtures", "migrate", "media"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := mediasync.Sync(ctx, mediasync.Options{
		Source: source,
		Bucket: bucket,
		Client: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Diff.Empty() {
		t.Fatalf("listing diff = %#v, want empty", result.Diff)
	}
	if result.Uploaded < 1 {
		t.Fatalf("uploaded = %d", result.Uploaded)
	}
}
