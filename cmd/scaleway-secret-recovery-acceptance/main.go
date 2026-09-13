// Command scaleway-secret-recovery-acceptance runs one disposable Scaleway
// Secret Manager backup, isolated restore, integrity, and cleanup cell.
// Scaleway Secret Manager assigns source IDs during creation, so this command
// creates the source at the provider boundary before handing its opaque ID to
// the provider-neutral certification cell.
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
	secret "github.com/scaleway/scaleway-sdk-go/api/secret/v1beta1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

const (
	defaultTimeout       = 10 * time.Minute
	objectLockAcceptance = 15 * time.Second
)

var safeSecretNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "Scaleway Secret Manager recovery acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("scaleway-secret-recovery-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	profile := flags.String("profile", "default", "Scaleway profile name")
	region := flags.String("region", "", "Scaleway Secret Manager and Object Storage region")
	bucket := flags.String("bucket", "", "uniquely owned disposable Object Storage archive bucket")
	secretName := flags.String("secret-name", "", "uniquely owned disposable source secret name")
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
		{*profile, "profile"}, {*region, "region"}, {*bucket, "bucket"}, {*secretName, "secret-name"}, {*marker, "marker"}, {*fixture, "fixture"},
	} {
		if strings.TrimSpace(part.value) == "" || strings.ContainsAny(part.value, "\r\n\x00") {
			return fmt.Errorf("%s is required and must be single-line", part.name)
		}
	}
	if !safeSecretNamePattern.MatchString(*secretName) {
		return errors.New("secret-name must contain only letters, numbers, dot, underscore, and hyphen and be at most 64 characters")
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	credentialProvider, profileConfig, project, profileEndpoint, err := credentialsFromProfile(*profile, *region)
	if err != nil {
		return err
	}
	endpoint, err := resolveEndpoint(profileEndpoint, *region)
	if err != nil {
		return err
	}
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(*region), awsconfig.WithCredentialsProvider(credentialProvider))
	if err != nil {
		return fmt.Errorf("load Scaleway Object Storage SDK configuration: %w", err)
	}
	objects := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = true
	})
	if err := ensureObjectLock(ctx, objects, *bucket); err != nil {
		return err
	}

	secretClient, err := newSecretClient(profileConfig, *region)
	if err != nil {
		return err
	}
	value := certification.SecretRecoveryFixtureValue(*fixture)
	created, err := secretClient.CreateSecret(&secret.CreateSecretRequest{
		Region: scw.Region(*region), ProjectID: project, Name: *secretName,
		Tags: []string{
			"magelift.io/ownership=" + *marker,
			"magelift.io/data-class=configuration-secrets",
		}, Protected: true,
	}, scw.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("create Scaleway Secret Manager source: %w", err)
	}
	if created == nil || strings.TrimSpace(created.ID) == "" {
		return errors.New("Scaleway Secret Manager source creation returned no ID")
	}
	if _, err := secretClient.CreateSecretVersion(&secret.CreateSecretVersionRequest{Region: scw.Region(*region), SecretID: created.ID, Data: value}, scw.WithContext(ctx)); err != nil {
		return fmt.Errorf("create Scaleway Secret Manager source version: %w", err)
	}
	sourceReference := "scaleway-secret-manager://" + created.ID
	sourceStore := scalewaySecretStore{client: secretClient, region: scw.Region(*region), project: project, marker: *marker}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), 90*time.Second)
		defer cleanupCancel()
		runErr = errors.Join(runErr, sourceStore.Delete(cleanupCtx, sourceReference))
	}()
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), 90*time.Second)
		defer cleanupCancel()
		runErr = errors.Join(runErr, purgeBucketVersions(cleanupCtx, objects, *bucket))
	}()

	native, err := scalewayresilience.NewScalewayNativeAPIWithSecrets(ctx, scalewayresilience.NativeAPIConfig{
		ArchiveBucket: bucketValue(*bucket), RestoreBucket: bucketValue(*bucket), Region: *region, Endpoint: endpoint,
		Credentials: credentialProvider, ArchivePrefix: "magelift/recovery", RestorePrefix: "magelift/recovery/restored",
		RetentionDays: 1, RequireObjectLock: true, ObjectLockRetention: objectLockAcceptance,
	}, scw.WithProfile(profileConfig), scw.WithDefaultProjectID(project), scw.WithDefaultRegion(scw.Region(*region)))
	if err != nil {
		return fmt.Errorf("construct Scaleway Secret Manager recovery translator: %w", err)
	}
	operationClient, err := scalewayresilience.NewNativeResilienceClient(native)
	if err != nil {
		return fmt.Errorf("construct Scaleway Secret Manager operation client: %w", err)
	}
	cell, err := certification.NewSecretRecoveryCell(certification.SecretRecoveryCellConfig{
		Operations: operationClient, Store: sourceStore, ResourceReference: sourceReference,
		Marker: *marker, FixtureID: *fixture,
	})
	if err != nil {
		return fmt.Errorf("construct provider-neutral Secret Manager recovery cell: %w", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), 90*time.Second)
		defer cleanupCancel()
		runErr = errors.Join(runErr, cleanupSecretCell(cleanupCtx, cell))
	}()

	if err := cell.Prepare(ctx); err != nil {
		return err
	}
	result, err := cell.Run(ctx)
	if err != nil {
		return err
	}
	if err := cleanupSecretCell(ctx, cell); err != nil {
		return err
	}
	if err := purgeBucketVersions(ctx, objects, *bucket); err != nil {
		return err
	}
	remaining, err := native.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("verify Scaleway Secret Manager recovery cleanup: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("Scaleway Secret Manager recovery outputs remain after cleanup: %#v", remaining)
	}

	fmt.Fprintf(output, "Scaleway Secret Manager recovery acceptance PASS region=%s bucket=%s source=%s backup=%s restore=%s valueFingerprint=%s sourceDeletion=scheduled-free cleanup=verified\n", *region, *bucket, sourceReference, result.BackupID, result.RestoreID, result.ValueFingerprint)
	return nil
}

func credentialsFromProfile(profile, region string) (aws.CredentialsProvider, *scw.Profile, string, string, error) {
	config, err := scw.LoadConfig()
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("load Scaleway profile configuration: %w", err)
	}
	profileConfig, err := config.GetProfile(profile)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("load Scaleway profile %q: %w", profile, err)
	}
	client, err := scw.NewClient(scw.WithProfile(profileConfig), scw.WithDefaultRegion(scw.Region(region)))
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("construct Scaleway profile client: %w", err)
	}
	accessKey, accessOK := client.GetAccessKey()
	secretKey, secretOK := client.GetSecretKey()
	if !accessOK || !secretOK || strings.TrimSpace(accessKey) == "" || strings.TrimSpace(secretKey) == "" {
		return nil, nil, "", "", errors.New("selected Scaleway profile does not contain an access key and secret key")
	}
	project, projectOK := client.GetDefaultProjectID()
	if !projectOK || strings.TrimSpace(project) == "" {
		return nil, nil, "", "", errors.New("selected Scaleway profile does not contain a default project ID")
	}
	endpoint, _ := client.GetS3Endpoint()
	return credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""), profileConfig, project, endpoint, nil
}

func newSecretClient(profile *scw.Profile, region string) (*secret.API, error) {
	client, err := scw.NewClient(scw.WithProfile(profile), scw.WithDefaultRegion(scw.Region(region)))
	if err != nil {
		return nil, fmt.Errorf("construct Scaleway Secret Manager client: %w", err)
	}
	return secret.NewAPI(client), nil
}

func resolveEndpoint(profileEndpoint, region string) (string, error) {
	endpoint := strings.TrimSpace(profileEndpoint)
	if endpoint == "" {
		endpoint = "https://s3." + strings.TrimSpace(region) + ".scw.cloud"
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("Scaleway Object Storage endpoint must be an HTTPS URL without a path, query, or fragment")
	}
	return strings.TrimRight(endpoint, "/"), nil
}

func ensureObjectLock(ctx context.Context, objects *s3.Client, bucket string) error {
	if _, err := objects.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucket), VersioningConfiguration: &s3types.VersioningConfiguration{Status: s3types.BucketVersioningStatusEnabled},
	}); err != nil {
		return fmt.Errorf("enable Scaleway Object Storage bucket versioning: %w", err)
	}
	if _, err := objects.PutObjectLockConfiguration(ctx, &s3.PutObjectLockConfigurationInput{
		Bucket: aws.String(bucket), ObjectLockConfiguration: &s3types.ObjectLockConfiguration{ObjectLockEnabled: s3types.ObjectLockEnabledEnabled},
	}); err != nil {
		return fmt.Errorf("enable Scaleway Object Storage Object Lock: %w", err)
	}
	configuration, err := objects.GetObjectLockConfiguration(ctx, &s3.GetObjectLockConfigurationInput{Bucket: aws.String(bucket)})
	if err != nil {
		return fmt.Errorf("verify Scaleway Object Storage Object Lock: %w", err)
	}
	if configuration == nil || configuration.ObjectLockConfiguration == nil || configuration.ObjectLockConfiguration.ObjectLockEnabled != s3types.ObjectLockEnabledEnabled {
		return errors.New("Scaleway Object Storage Object Lock verification returned disabled")
	}
	return nil
}

// purgeBucketVersions is an acceptance-only finalizer. Object Lock requires
// versioning, and deleting an object without its version ID leaves the old
// version behind, which prevents exact bucket deletion. The lifecycle core
// never calls this broad operation; the bucket is uniquely generated by this
// command and the retention wait has already completed before this runs.
func purgeBucketVersions(ctx context.Context, objects *s3.Client, bucket string) error {
	keyMarker := ""
	versionMarker := ""
	for {
		input := &s3.ListObjectVersionsInput{Bucket: aws.String(bucket)}
		if keyMarker != "" {
			input.KeyMarker = aws.String(keyMarker)
		}
		if versionMarker != "" {
			input.VersionIdMarker = aws.String(versionMarker)
		}
		page, err := objects.ListObjectVersions(ctx, input)
		if err != nil {
			if isNotFound(err) {
				return nil
			}
			return fmt.Errorf("list Scaleway archive object versions for cleanup: %w", err)
		}
		if page == nil {
			return errors.New("Scaleway archive version listing returned an empty response")
		}
		for _, version := range page.Versions {
			if err := deleteBucketVersion(ctx, objects, bucket, aws.ToString(version.Key), aws.ToString(version.VersionId)); err != nil {
				return err
			}
		}
		for _, marker := range page.DeleteMarkers {
			if err := deleteBucketVersion(ctx, objects, bucket, aws.ToString(marker.Key), aws.ToString(marker.VersionId)); err != nil {
				return err
			}
		}
		if !aws.ToBool(page.IsTruncated) {
			break
		}
		keyMarker = aws.ToString(page.NextKeyMarker)
		versionMarker = aws.ToString(page.NextVersionIdMarker)
		if keyMarker == "" && versionMarker == "" {
			return errors.New("Scaleway archive version listing was truncated without a continuation marker")
		}
	}
	return nil
}

func deleteBucketVersion(ctx context.Context, objects *s3.Client, bucket, key, version string) error {
	if strings.TrimSpace(key) == "" || strings.TrimSpace(version) == "" {
		return errors.New("Scaleway archive version cleanup returned an incomplete object identity")
	}
	if _, err := objects.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), VersionId: aws.String(version)}); err != nil && !isNotFound(err) {
		return fmt.Errorf("delete Scaleway archive object version %q: %w", key, err)
	}
	return nil
}

func cleanupSecretCell(ctx context.Context, cell *certification.SecretRecoveryCell) error {
	for {
		err := cell.Cleanup(ctx)
		if err == nil {
			return nil
		}
		if !strings.Contains(strings.ToLower(err.Error()), "pending") {
			return err
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

type scalewaySecretStore struct {
	client  *secret.API
	region  scw.Region
	project string
	marker  string
}

func (store scalewaySecretStore) Put(ctx context.Context, reference string, expected []byte) error {
	id, err := parseSecretReference(reference)
	if err != nil {
		return err
	}
	current, err := store.client.GetSecret(&secret.GetSecretRequest{Region: store.region, SecretID: id}, scw.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("inspect pre-created Scaleway source secret: %w", err)
	}
	if current == nil || !current.Protected || !hasTag(current.Tags, "magelift.io/ownership="+store.marker) || !hasTag(current.Tags, "magelift.io/data-class=configuration-secrets") {
		return errors.New("pre-created Scaleway source secret is not protected and ownership-tagged")
	}
	version, err := store.client.AccessSecretVersion(&secret.AccessSecretVersionRequest{Region: store.region, SecretID: id, Revision: "latest_enabled"}, scw.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("read pre-created Scaleway source secret: %w", err)
	}
	if version == nil || !bytes.Equal(version.Data, expected) {
		return errors.New("pre-created Scaleway source secret does not match the known fixture")
	}
	return nil
}

func (store scalewaySecretStore) Read(ctx context.Context, reference string) ([]byte, error) {
	id, err := parseSecretReference(reference)
	if err != nil {
		return nil, err
	}
	version, err := store.client.AccessSecretVersion(&secret.AccessSecretVersionRequest{Region: store.region, SecretID: id, Revision: "latest_enabled"}, scw.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("read Scaleway secret %q: %w", id, err)
	}
	if version == nil || len(version.Data) == 0 {
		return nil, errors.New("Scaleway Secret Manager read returned no value")
	}
	return append([]byte(nil), version.Data...), nil
}

func (store scalewaySecretStore) Delete(ctx context.Context, reference string) error {
	id, err := parseSecretReference(reference)
	if err != nil {
		return err
	}
	active, err := store.active(ctx, id)
	if err != nil || !active {
		return err
	}
	current, err := store.client.GetSecret(&secret.GetSecretRequest{Region: store.region, SecretID: id}, scw.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("inspect Scaleway source secret for deletion: %w", err)
	}
	if current.Protected {
		if _, err := store.client.UnprotectSecret(&secret.UnprotectSecretRequest{Region: store.region, SecretID: id}, scw.WithContext(ctx)); err != nil {
			return fmt.Errorf("unprotect Scaleway source secret: %w", err)
		}
	}
	if err := store.client.DeleteSecret(&secret.DeleteSecretRequest{Region: store.region, SecretID: id}, scw.WithContext(ctx)); err != nil && !isNotFound(err) {
		return fmt.Errorf("schedule Scaleway source secret deletion: %w", err)
	}
	for {
		active, err := store.active(ctx, id)
		if err != nil {
			return err
		}
		if !active {
			return nil
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (store scalewaySecretStore) active(ctx context.Context, id string) (bool, error) {
	response, err := store.client.ListSecrets(&secret.ListSecretsRequest{Region: store.region, ProjectID: &store.project}, scw.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("list active Scaleway source secrets: %w", err)
	}
	if response == nil {
		return false, errors.New("Scaleway Secret Manager active-secret listing returned an empty response")
	}
	for _, item := range response.Secrets {
		if item.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func parseSecretReference(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	for _, scheme := range []string{"scaleway-secret-manager://", "scaleway-secret://"} {
		if strings.HasPrefix(strings.ToLower(reference), scheme) {
			id := strings.TrimSpace(reference[len(scheme):])
			if id == "" || strings.ContainsAny(id, "/?#\r\n\x00") {
				return "", fmt.Errorf("Scaleway secret reference %q must identify one secret", reference)
			}
			return id, nil
		}
	}
	return "", fmt.Errorf("Scaleway secret reference %q must use scaleway-secret-manager://", reference)
}

func hasTag(tags []string, expected string) bool {
	for _, tag := range tags {
		if tag == expected {
			return true
		}
	}
	return false
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "notfound") || strings.Contains(message, "404")
}

func bucketValue(value string) string { return strings.TrimSpace(value) }
