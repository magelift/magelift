// Command scaleway-recovery-acceptance runs one disposable Scaleway Object
// Storage backup, isolated-restore, integrity, and ownership-cleanup cell.
//
// The command deliberately keeps provider credentials at the adapter edge:
// the selected Scaleway profile is resolved into an AWS SDK credential
// provider, while no credential value enters an operation request, an
// operation ID, evidence, or process output.
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
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/magelift/magelift/internal/certification"
	scalewayresilience "github.com/magelift/magelift/internal/cloud/scaleway/resilience"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

const defaultTimeout = 10 * time.Minute

var safeBucketPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{2,62}$`)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "Scaleway recovery acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("scaleway-recovery-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "default", "Scaleway profile name")
	region := flags.String("region", "", "Scaleway Object Storage region")
	bucket := flags.String("bucket", "", "uniquely owned disposable Object Storage bucket")
	marker := flags.String("marker", "", "single-line MageLift ownership marker")
	fixture := flags.String("fixture", "", "single-line known-content fixture ID")
	endpoint := flags.String("endpoint", "", "optional HTTPS S3 endpoint; otherwise use the profile or regional default")
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
		{*profile, "profile"}, {*region, "region"}, {*bucket, "bucket"}, {*marker, "marker"}, {*fixture, "fixture"},
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

	credentialProvider, profileEndpoint, err := credentialsFromProfile(*profile)
	if err != nil {
		return err
	}
	resolvedEndpoint, err := resolveEndpoint(*endpoint, profileEndpoint, *region)
	if err != nil {
		return err
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(*region), awsconfig.WithCredentialsProvider(credentialProvider))
	if err != nil {
		return fmt.Errorf("load Scaleway Object Storage SDK configuration: %w", err)
	}
	objects := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(resolvedEndpoint)
		options.UsePathStyle = true
	})
	native, err := scalewayresilience.NewScalewayNativeAPI(ctx, scalewayresilience.NativeAPIConfig{
		ArchiveBucket:     *bucket,
		RestoreBucket:     *bucket,
		Region:            *region,
		Endpoint:          resolvedEndpoint,
		Credentials:       credentialProvider,
		ArchivePrefix:     "magelift/recovery",
		RestorePrefix:     "magelift/recovery/restored",
		RetentionDays:     1,
		RequireObjectLock: false,
	})
	if err != nil {
		return fmt.Errorf("construct Scaleway recovery translator: %w", err)
	}
	operationClient, err := scalewayresilience.NewNativeResilienceClient(native)
	if err != nil {
		return fmt.Errorf("construct Scaleway recovery operation client: %w", err)
	}

	sourcePrefix := "fixture/" + *fixture
	resourceReference := "scaleway-object://" + *bucket + "/" + sourcePrefix
	cell, err := certification.NewObjectRecoveryCell(certification.ObjectRecoveryCellConfig{
		Operations:        operationClient,
		Store:             scalewayObjectStore{client: objects},
		Bucket:            *bucket,
		ResourceReference: resourceReference,
		Marker:            *marker,
		FixtureID:         *fixture,
		SourcePrefix:      sourcePrefix,
		ParseReference:    parseObjectReference,
	})
	if err != nil {
		return fmt.Errorf("construct provider-neutral Object Storage recovery cell: %w", err)
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
		return fmt.Errorf("verify Object Storage recovery cleanup: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("Object Storage recovery outputs remain after cleanup: %#v", remaining)
	}

	fmt.Fprintf(output, "Scaleway Object Storage recovery acceptance PASS region=%s bucket=%s objects=%d backup=%s restore=%s fixtureFingerprint=%s cleanup=verified\n", *region, *bucket, result.ObjectCount, result.BackupID, result.RestoreID, result.FixtureFingerprint)
	return nil
}

func credentialsFromProfile(profile string) (aws.CredentialsProvider, string, error) {
	config, err := scw.LoadConfig()
	if err != nil {
		return nil, "", fmt.Errorf("load Scaleway profile configuration: %w", err)
	}
	profileConfig, err := config.GetProfile(profile)
	if err != nil {
		return nil, "", fmt.Errorf("load Scaleway profile %q: %w", profile, err)
	}
	client, err := scw.NewClient(scw.WithProfile(profileConfig))
	if err != nil {
		return nil, "", fmt.Errorf("construct Scaleway profile client: %w", err)
	}
	accessKey, accessOK := client.GetAccessKey()
	secretKey, secretOK := client.GetSecretKey()
	if !accessOK || !secretOK || strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return nil, "", errors.New("selected Scaleway profile does not contain an access key and secret key")
	}
	profileEndpoint, _ := client.GetS3Endpoint()
	return credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""), profileEndpoint, nil
}

func resolveEndpoint(explicit, profileEndpoint, region string) (string, error) {
	endpoint := strings.TrimSpace(explicit)
	if endpoint == "" {
		endpoint = strings.TrimSpace(profileEndpoint)
	}
	if endpoint == "" {
		endpoint = "https://s3." + strings.TrimSpace(region) + ".scw.cloud"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Scaleway Object Storage endpoint must be an HTTPS URL without a path, query, or fragment")
	}
	return strings.TrimRight(endpoint, "/"), nil
}

type scalewayObjectStore struct {
	client *s3.Client
}

func (store scalewayObjectStore) Put(ctx context.Context, bucket string, object certification.ObjectRecoveryFixture) error {
	_, err := store.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket), Key: aws.String(object.Key), Body: bytes.NewReader(object.Body),
		ContentLength: aws.Int64(int64(len(object.Body))), ServerSideEncryption: s3types.ServerSideEncryptionAes256,
	})
	if err != nil {
		return fmt.Errorf("put source fixture object %q: %w", object.Key, err)
	}
	return nil
}

func (store scalewayObjectStore) Delete(ctx context.Context, bucket, key string) error {
	_, err := store.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	return err
}

func (store scalewayObjectStore) ReadPrefix(ctx context.Context, bucket, prefix string) (map[string][]byte, error) {
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
				return nil, fmt.Errorf("read Object Storage object %q: %w", key, err)
			}
			if object == nil || object.Body == nil {
				return nil, fmt.Errorf("Object Storage object %q returned no body", key)
			}
			body, readErr := io.ReadAll(object.Body)
			closeErr := object.Body.Close()
			if readErr != nil {
				return nil, fmt.Errorf("read Object Storage object %q: %w", key, readErr)
			}
			if closeErr != nil {
				return nil, fmt.Errorf("close Object Storage object %q: %w", key, closeErr)
			}
			result[key] = body
		}
	}
	return result, nil
}

func parseObjectReference(reference string) (bucket, prefix string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || parsed.Scheme != "scaleway-object" || strings.TrimSpace(parsed.Host) == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("Object Storage reference %q is invalid", reference)
	}
	prefix, err = url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || strings.TrimSpace(prefix) == "" || strings.ContainsAny(prefix, "\r\n\x00") {
		return "", "", fmt.Errorf("Object Storage reference %q does not contain a safe prefix", reference)
	}
	return parsed.Host, prefix, nil
}

func validatePart(value, name string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	return nil
}
