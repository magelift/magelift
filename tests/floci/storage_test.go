//go:build floci

package floci_test

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestVersionedMediaRestoreAgainstFloci(t *testing.T) {
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
	bucket := "magelift-floci-media"
	key := "media/catalog.json"
	if _, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: awssdk.String(bucket)}); err != nil {
		t.Fatal(err)
	}
	defer func() {
		versions, listErr := client.ListObjectVersions(context.Background(), &s3.ListObjectVersionsInput{Bucket: awssdk.String(bucket)})
		if listErr != nil {
			return
		}
		for _, version := range versions.Versions {
			_, _ = client.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: awssdk.String(bucket), Key: version.Key, VersionId: version.VersionId})
		}
		for _, marker := range versions.DeleteMarkers {
			_, _ = client.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: awssdk.String(bucket), Key: marker.Key, VersionId: marker.VersionId})
		}
	}()
	if _, err := client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket:                  awssdk.String(bucket),
		VersioningConfiguration: &types.VersioningConfiguration{Status: types.BucketVersioningStatusEnabled},
	}); err != nil {
		t.Fatal(err)
	}
	first, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: awssdk.String(bucket), Key: awssdk.String(key), Body: strings.NewReader("media-v1")})
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionId == nil || awssdk.ToString(first.VersionId) == "" {
		t.Fatal("versioned S3 write did not return a version ID")
	}
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: awssdk.String(bucket), Key: awssdk.String(key), Body: strings.NewReader("media-v2")}); err != nil {
		t.Fatal(err)
	}
	restored, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: awssdk.String(bucket), Key: awssdk.String(key), VersionId: first.VersionId})
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(restored.Body)
	_ = restored.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "media-v1" {
		t.Fatalf("versioned media snapshot = %q", content)
	}
	if _, err := client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: awssdk.String(bucket), Key: awssdk.String(key)}); err != nil {
		t.Fatal(err)
	}
	versions, err := client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{Bucket: awssdk.String(bucket), Prefix: awssdk.String(key)})
	if err != nil {
		t.Fatal(err)
	}
	if len(versions.DeleteMarkers) != 1 {
		t.Fatalf("delete markers = %d, want one", len(versions.DeleteMarkers))
	}
	if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: awssdk.String(bucket), Key: awssdk.String(key), Body: strings.NewReader(string(content))}); err != nil {
		t.Fatal(err)
	}
	current, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: awssdk.String(bucket), Key: awssdk.String(key)})
	if err != nil {
		t.Fatal(err)
	}
	defer current.Body.Close()
	content, err = io.ReadAll(current.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "media-v1" {
		t.Fatalf("restored current media = %q", content)
	}
}
