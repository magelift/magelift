// Command gcp-secret-recovery-acceptance runs one disposable Google Cloud
// Secret Manager backup, isolated or approved in-place restore, integrity, and
// cleanup cell. The provider-neutral certification cell owns the operation
// sequence; this command owns only the official Secret Manager value boundary.
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

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/magelift/magelift/internal/certification"
	gcpresilience "github.com/magelift/magelift/internal/cloud/gcp/resilience"
	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
)

const defaultTimeout = 10 * time.Minute

var safeSecretIDPattern = regexp.MustCompile("^[A-Za-z0-9_-]+$")

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "GCP Secret Manager recovery acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("gcp-secret-recovery-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", "", "Google Cloud project ID")
	archiveBucket := flags.String("archive-bucket", "", "uniquely owned disposable Cloud Storage archive bucket")
	secretID := flags.String("secret-id", "", "uniquely owned disposable source secret ID")
	marker := flags.String("marker", "", "single-line MageLift ownership marker")
	fixture := flags.String("fixture", "", "single-line known-content fixture ID")
	destinationName := flags.String("destination", "isolated", "recovery destination: isolated or in-place")
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
		{*project, "project"}, {*archiveBucket, "archive-bucket"}, {*secretID, "secret-id"}, {*marker, "marker"}, {*fixture, "fixture"},
	} {
		if strings.TrimSpace(part.value) == "" || strings.ContainsAny(part.value, "\r\n\x00") {
			return fmt.Errorf("%s is required and must be single-line", part.name)
		}
	}
	if !safeSecretIDPattern.MatchString(*secretID) || len(*secretID) > 255 {
		return errors.New("secret-id must contain only letters, numbers, underscore, or hyphen and be at most 255 characters")
	}
	destination, approval, err := parseRecoveryDestination(*destinationName)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	secrets, err := secretmanager.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create GCP Secret Manager acceptance client: %w", err)
	}
	defer func() { runErr = errors.Join(runErr, secrets.Close()) }()
	native, err := gcpresilience.NewGCPSecretNativeAPI(ctx, gcpresilience.NativeAPIConfig{
		ArchiveBucket: *archiveBucket, RestoreBucket: *archiveBucket, Project: *project,
		ArchivePrefix: "magelift/recovery", RestoreSecretPrefix: "magelift-recovery", RetentionDays: 1,
	})
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, native.Close()) }()
	operations, err := gcpresilience.NewNativeResilienceClient(native)
	if err != nil {
		return fmt.Errorf("construct GCP Secret Manager operation client: %w", err)
	}
	resource := "gcp-secret-manager://projects/" + *project + "/secrets/" + *secretID
	cell, err := certification.NewSecretRecoveryCell(certification.SecretRecoveryCellConfig{
		Operations:        operations,
		Store:             gcpSecretStore{client: secrets, project: *project, marker: *marker, fixture: *fixture},
		ResourceReference: resource, Marker: *marker, FixtureID: *fixture,
		Destination: destination, ApprovalReference: approval,
	})
	if err != nil {
		return fmt.Errorf("construct provider-neutral Secret Manager recovery cell: %w", err)
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
		return fmt.Errorf("verify GCP Secret Manager recovery cleanup: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("GCP Secret Manager recovery outputs remain after cleanup: %#v", remaining)
	}

	fmt.Fprintf(output, "GCP Secret Manager recovery acceptance PASS project=%s source=%s destination=%s backup=%s restore=%s valueFingerprint=%s cleanup=verified\n", *project, resource, destination, result.BackupID, result.RestoreID, result.ValueFingerprint)
	return nil
}

type gcpSecretStore struct {
	client  *secretmanager.Client
	project string
	marker  string
	fixture string
}

func (store gcpSecretStore) Put(ctx context.Context, reference string, value []byte) error {
	name, err := parseSecretReference(reference)
	if err != nil {
		return err
	}
	if _, err := store.client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{Name: name}); err == nil {
		return fmt.Errorf("refusing to adopt existing GCP secret %q", name)
	} else if !isNotFound(err) {
		return fmt.Errorf("probe GCP source secret %q: %w", name, err)
	}
	secretID := name[strings.LastIndex(name, "/")+1:]
	created, err := store.client.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{
		Parent: "projects/" + store.project, SecretId: secretID,
		Secret: &secretmanagerpb.Secret{Labels: map[string]string{"magelift_ownership": "m" + cloudrecovery.Digest([]byte(store.marker))[:24], "magelift_data_class": "configuration-secrets", "magelift_fixture": "m" + cloudrecovery.Digest([]byte(store.fixture))[:24]}, Replication: &secretmanagerpb.Replication{Replication: &secretmanagerpb.Replication_Automatic_{Automatic: &secretmanagerpb.Replication_Automatic{}}}},
	})
	if err != nil {
		return fmt.Errorf("create GCP source secret: %w", err)
	}
	if _, err := store.client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{Parent: created.Name, Payload: &secretmanagerpb.SecretPayload{Data: value}}); err != nil {
		return fmt.Errorf("add GCP source secret version: %w", err)
	}
	return nil
}

func (store gcpSecretStore) Overwrite(ctx context.Context, reference string, value []byte) error {
	name, err := parseSecretReference(reference)
	if err != nil {
		return err
	}
	if _, err := store.client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{Name: name}); err != nil {
		return fmt.Errorf("probe GCP source secret for overwrite %q: %w", name, err)
	}
	if _, err := store.client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{Parent: name, Payload: &secretmanagerpb.SecretPayload{Data: value}}); err != nil {
		return fmt.Errorf("overwrite GCP source secret version: %w", err)
	}
	return nil
}

func (store gcpSecretStore) Read(ctx context.Context, reference string) ([]byte, error) {
	name, err := parseSecretReference(reference)
	if err != nil {
		return nil, err
	}
	response, err := store.client.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{Name: name + "/versions/latest"})
	if err != nil {
		return nil, fmt.Errorf("read GCP secret %q: %w", name, err)
	}
	if response == nil || response.Payload == nil || len(response.Payload.Data) == 0 {
		return nil, errors.New("GCP secret read returned no value")
	}
	return append([]byte(nil), response.Payload.Data...), nil
}

func (store gcpSecretStore) Delete(ctx context.Context, reference string) error {
	name, err := parseSecretReference(reference)
	if err != nil {
		return err
	}
	if err := store.client.DeleteSecret(ctx, &secretmanagerpb.DeleteSecretRequest{Name: name}); err != nil && !isNotFound(err) {
		return fmt.Errorf("delete GCP source secret: %w", err)
	}
	for {
		_, err := store.client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{Name: name})
		if isNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("verify GCP source secret deletion: %w", err)
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
	for _, scheme := range []string{"gcp-secret-manager://", "secretmanager://"} {
		if strings.HasPrefix(strings.ToLower(reference), scheme) {
			name := strings.TrimSpace(reference[len(scheme):])
			name = strings.TrimPrefix(name, "//")
			parts := strings.Split(name, "/")
			if len(parts) != 4 || parts[0] != "projects" || parts[2] != "secrets" || parts[1] == "" || parts[3] == "" {
				return "", fmt.Errorf("GCP secret reference %q must use projects/PROJECT/secrets/NAME", reference)
			}
			return name, nil
		}
	}
	return "", fmt.Errorf("GCP secret reference %q must use gcp-secret-manager://", reference)
}

func parseRecoveryDestination(value string) (sdk.RecoveryDestination, string, error) {
	switch strings.TrimSpace(value) {
	case "", "isolated":
		return sdk.RecoverySameRegionIsolated, "", nil
	case "in-place":
		return sdk.RecoverySameRegion, "gcp-secret-recovery-acceptance-in-place", nil
	default:
		return "", "", fmt.Errorf("destination must be isolated or in-place, got %q", value)
	}
}

func isNotFound(err error) bool { return status.Code(err) == codes.NotFound }
