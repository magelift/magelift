//go:build floci

package floci_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/magelift/magelift/internal/mediasync"
)

func TestMediaSyncListingDiffAgainstFloci(t *testing.T) {
	endpoint := flociEndpoint(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	configuration := flociConfig(t, ctx, endpoint)
	client := s3.NewFromConfig(configuration, func(options *s3.Options) {
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

	source, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "fixtures", "migrate", "media"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := mediasync.Sync(ctx, mediasync.Options{
		Source: source,
		Bucket: bucket,
		Client: client,
		Prefix: "media/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Diff.Empty() {
		t.Fatalf("listing diff = %#v, want empty", result.Diff)
	}
	for _, key := range result.Keys {
		if len(key) < 6 || key[:6] != "media/" {
			t.Fatalf("unexpected media key %q", key)
		}
	}
	if result.Uploaded < 1 {
		t.Fatalf("uploaded = %d", result.Uploaded)
	}
}
