// Command aws-secret-recovery-acceptance runs one disposable AWS Secrets
// Manager backup, isolated restore, integrity, and ownership-cleanup cell.
// The operation sequence and proof checks live in the provider-neutral
// certification cell; this command owns only the AWS SDK secret-value edge.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretstypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"

	"github.com/magelift/magelift/internal/certification"
	awsresilience "github.com/magelift/magelift/internal/cloud/aws/resilience"
)

const defaultTimeout = 10 * time.Minute

var safeSecretNamePattern = regexp.MustCompile(`^[A-Za-z0-9/_+=.@-]{1,512}$`)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "AWS Secrets Manager recovery acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("aws-secret-recovery-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	region := flags.String("region", "", "AWS region")
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
		{*region, "region"}, {*secretName, "secret-name"}, {*marker, "marker"}, {*fixture, "fixture"},
	} {
		if err := validatePart(part.value, part.name); err != nil {
			return err
		}
	}
	if !safeSecretNamePattern.MatchString(*secretName) {
		return errors.New("secret-name must contain only AWS Secrets Manager name characters")
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(*region))
	if err != nil {
		return fmt.Errorf("load AWS SDK configuration: %w", err)
	}
	secrets := secretsmanager.NewFromConfig(awsConfig)
	// Secret Manager archives are kept in the disposable S3 bucket supplied by
	// the wrapper through MAGELIFT_AWS_SECRET_ARCHIVE_BUCKET.
	archiveBucket := strings.TrimSpace(os.Getenv("MAGELIFT_AWS_SECRET_ARCHIVE_BUCKET"))
	if archiveBucket == "" {
		return errors.New("MAGELIFT_AWS_SECRET_ARCHIVE_BUCKET is required")
	}
	native, err := awsresilience.NewAWSSecretNativeAPI(ctx, awsresilience.NativeAPIConfig{
		ArchiveBucket: archiveBucket,
		RestoreBucket: archiveBucket,
		ArchivePrefix: "magelift/recovery",
		RestorePrefix: "magelift/recovery",
		RetentionDays: 1,
	}, awsconfig.WithRegion(*region))
	if err != nil {
		return fmt.Errorf("construct AWS Secrets Manager recovery translator: %w", err)
	}
	operations, err := awsresilience.NewNativeResilienceClient(native)
	if err != nil {
		return fmt.Errorf("construct AWS Secrets Manager operation client: %w", err)
	}
	resource := "aws-secretsmanager://" + *secretName
	cell, err := certification.NewSecretRecoveryCell(certification.SecretRecoveryCellConfig{
		Operations:        operations,
		Store:             awsSecretStore{client: secrets, marker: *marker},
		ResourceReference: resource,
		Marker:            *marker,
		FixtureID:         *fixture,
	})
	if err != nil {
		return fmt.Errorf("construct provider-neutral Secrets Manager recovery cell: %w", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), 90*time.Second)
		defer cleanupCancel()
		runErr = errors.Join(runErr, cell.Cleanup(cleanupCtx))
	}()

	if err := cell.Prepare(ctx); err != nil {
		return err
	}
	result, err := cell.Run(ctx)
	if err != nil {
		return err
	}
	if err := cell.Cleanup(ctx); err != nil {
		return err
	}
	remaining, err := native.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("verify AWS Secrets Manager recovery cleanup: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("AWS Secrets Manager recovery outputs remain after cleanup: %#v", remaining)
	}

	fmt.Fprintf(output, "AWS Secrets Manager recovery acceptance PASS region=%s source=%s backup=%s restore=%s valueFingerprint=%s cleanup=verified\n", *region, resource, result.BackupID, result.RestoreID, result.ValueFingerprint)
	return nil
}

type awsSecretStore struct {
	client *secretsmanager.Client
	marker string
}

func (store awsSecretStore) Put(ctx context.Context, reference string, value []byte) error {
	name, err := parseSecretReference(reference)
	if err != nil {
		return err
	}
	if _, err := store.client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)}); err == nil {
		return fmt.Errorf("refusing to adopt existing AWS secret %q", name)
	} else if !isSecretNotFound(err) {
		return fmt.Errorf("probe AWS source secret %q: %w", name, err)
	}
	_, err = store.client.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name: aws.String(name), SecretString: aws.String(string(value)),
		Description: aws.String("MageLift disposable recovery fixture"),
		Tags: []secretstypes.Tag{
			{Key: aws.String("magelift.io/ownership"), Value: aws.String(store.marker)},
			{Key: aws.String("magelift.io/data-class"), Value: aws.String("configuration-secrets")},
		},
	})
	if err != nil {
		return fmt.Errorf("create AWS source secret: %w", err)
	}
	return nil
}

func (store awsSecretStore) Read(ctx context.Context, reference string) ([]byte, error) {
	name, err := parseSecretReference(reference)
	if err != nil {
		return nil, err
	}
	value, err := store.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: aws.String(name)})
	if err != nil {
		return nil, fmt.Errorf("read AWS secret %q: %w", name, err)
	}
	if value == nil {
		return nil, errors.New("AWS secret read returned an empty response")
	}
	if value.SecretString != nil {
		return []byte(*value.SecretString), nil
	}
	if len(value.SecretBinary) != 0 {
		return append([]byte(nil), value.SecretBinary...), nil
	}
	return nil, errors.New("AWS secret read returned no value")
}

func (store awsSecretStore) Delete(ctx context.Context, reference string) error {
	name, err := parseSecretReference(reference)
	if err != nil {
		return err
	}
	if _, err := store.client.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{SecretId: aws.String(name), ForceDeleteWithoutRecovery: aws.Bool(true)}); err != nil && !isSecretNotFound(err) {
		return fmt.Errorf("delete AWS source secret: %w", err)
	}
	for {
		_, err := store.client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{SecretId: aws.String(name)})
		if isSecretNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("verify AWS source secret deletion: %w", err)
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

func parseSecretReference(reference string) (string, error) {
	reference = strings.TrimSpace(reference)
	for _, scheme := range []string{"aws-secretsmanager://", "secretsmanager://"} {
		if strings.HasPrefix(strings.ToLower(reference), scheme) {
			name := strings.TrimSpace(reference[len(scheme):])
			if name == "" || !safeSecretNamePattern.MatchString(name) {
				return "", fmt.Errorf("AWS secret reference %q has an invalid secret name", reference)
			}
			return name, nil
		}
	}
	return "", fmt.Errorf("AWS secret reference %q must use aws-secretsmanager://", reference)
}

func isSecretNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "resource not found") || strings.Contains(message, "resourcenotfound") || strings.Contains(message, "not found") || strings.Contains(message, "404")
}

func validatePart(value, name string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	return nil
}
