// Command aws-recovery-acceptance runs one disposable, provider-backed AWS S3
// backup, isolated-restore, integrity, and ownership-cleanup cell.
//
// The command owns the provider SDK boundary and the normalized recovery
// client. The provider-neutral certification cell owns the operation order
// and exact source fixture cleanup; the shell wrapper owns the bucket.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/magelift/magelift/internal/certification"
	awsresilience "github.com/magelift/magelift/internal/cloud/aws/resilience"
)

const defaultTimeout = 10 * time.Minute

var safeBucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{2,62}$`)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "AWS S3 recovery acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("aws-recovery-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	region := flags.String("region", "", "AWS region")
	bucket := flags.String("bucket", "", "uniquely owned disposable S3 bucket")
	marker := flags.String("marker", "", "single-line MageLift ownership marker")
	fixture := flags.String("fixture", "", "single-line known-content fixture ID")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	for _, part := range []struct {
		value string
		name  string
	}{
		{*region, "region"}, {*bucket, "bucket"}, {*marker, "marker"}, {*fixture, "fixture"},
	} {
		if err := validatePart(part.value, part.name); err != nil {
			return err
		}
	}
	if !safeBucketPattern.MatchString(*bucket) {
		return errors.New("bucket must be a lowercase DNS-compatible name between 3 and 63 characters")
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(*region))
	if err != nil {
		return fmt.Errorf("load AWS SDK configuration: %w", err)
	}
	objects := s3.NewFromConfig(awsConfig)
	native, err := awsresilience.NewAWSObjectNativeAPI(ctx, awsresilience.NativeAPIConfig{
		ArchiveBucket: *bucket,
		RestoreBucket: *bucket,
		ArchivePrefix: "magelift/recovery",
		RestorePrefix: "magelift/recovery/restored",
		RetentionDays: 1,
	}, awsconfig.WithRegion(*region))
	if err != nil {
		return fmt.Errorf("construct AWS S3 recovery translator: %w", err)
	}
	operationClient, err := awsresilience.NewNativeResilienceClient(native)
	if err != nil {
		return fmt.Errorf("construct AWS recovery operation client: %w", err)
	}

	sourcePrefix := "fixture/" + *fixture
	cell, err := certification.NewObjectRecoveryCell(certification.ObjectRecoveryCellConfig{
		Operations:        operationClient,
		Store:             awsObjectStore{client: objects},
		Bucket:            *bucket,
		ResourceReference: "s3://" + *bucket + "/" + sourcePrefix,
		Marker:            *marker,
		FixtureID:         *fixture,
		SourcePrefix:      sourcePrefix,
		ParseReference:    parseObjectReference,
	})
	if err != nil {
		return fmt.Errorf("construct provider-neutral S3 recovery cell: %w", err)
	}

	cleanupDone := false
	cleanup := func() error {
		if cleanupDone {
			return nil
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), 90*time.Second)
		defer cleanupCancel()
		if err := cell.Cleanup(cleanupCtx); err != nil {
			return err
		}
		cleanupDone = true
		return nil
	}
	defer func() {
		runErr = errors.Join(runErr, cleanup())
	}()

	if err := cell.Prepare(ctx); err != nil {
		return err
	}
	result, err := cell.Run(ctx)
	if err != nil {
		return err
	}
	if err := cleanup(); err != nil {
		return err
	}
	remaining, err := native.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("verify S3 recovery cleanup: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("S3 recovery outputs remain after cleanup: %#v", remaining)
	}

	fmt.Fprintf(output, "AWS S3 recovery acceptance PASS region=%s bucket=%s objects=%d backup=%s restore=%s fixtureFingerprint=%s cleanup=verified\n", *region, *bucket, result.ObjectCount, result.BackupID, result.RestoreID, result.FixtureFingerprint)
	return nil
}

type awsObjectStore struct {
	client *s3.Client
}

func (store awsObjectStore) Put(ctx context.Context, bucket string, object certification.ObjectRecoveryFixture) error {
	_, err := store.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(object.Key), Body: bytes.NewReader(object.Body),
		ContentLength: aws.Int64(int64(len(object.Body))), ServerSideEncryption: s3types.ServerSideEncryptionAes256,
	})
	if err != nil {
		return fmt.Errorf("put source fixture object %q: %w", object.Key, err)
	}
	return nil
}

func (store awsObjectStore) Delete(ctx context.Context, bucket, key string) error {
	_, err := store.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	return err
}

func (store awsObjectStore) ReadPrefix(ctx context.Context, bucket, prefix string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	paginator := s3.NewListObjectsV2Paginator(store.client, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix)})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, entry := range page.Contents {
			key := aws.ToString(entry.Key)
			if key == "" {
				continue
			}
			object, err := store.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
			if err != nil {
				return nil, fmt.Errorf("read S3 object %q: %w", key, err)
			}
			if object == nil || object.Body == nil {
				return nil, fmt.Errorf("S3 object %q returned no body", key)
			}
			body, readErr := io.ReadAll(object.Body)
			closeErr := object.Body.Close()
			if readErr != nil {
				return nil, fmt.Errorf("read S3 object %q: %w", key, readErr)
			}
			if closeErr != nil {
				return nil, fmt.Errorf("close S3 object %q: %w", key, closeErr)
			}
			result[key] = body
		}
	}
	return result, nil
}

func parseObjectReference(reference string) (bucket, prefix string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || (parsed.Scheme != "s3" && parsed.Scheme != "aws-s3") || strings.TrimSpace(parsed.Host) == "" {
		return "", "", fmt.Errorf("S3 object reference %q must use s3://bucket/prefix", reference)
	}
	prefix, err = url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || strings.TrimSpace(prefix) == "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(prefix, "\r\n\x00") {
		return "", "", fmt.Errorf("S3 object reference %q must include a safe non-empty prefix", reference)
	}
	return parsed.Host, prefix, nil
}

func validatePart(value, name string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	return nil
}
