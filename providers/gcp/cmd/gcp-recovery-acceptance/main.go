// Command gcp-recovery-acceptance runs one disposable Google Cloud Storage
// backup, isolated-restore, integrity, and ownership-cleanup cell.
//
// The provider-specific Google Cloud client and gs:// reference parser stay
// in this command. The operation sequence, known-content fixture, independent
// byte verification, and cleanup contract are shared by the certification
// core with every object-storage adapter.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	gcpstorage "cloud.google.com/go/storage"
	"google.golang.org/api/iterator"

	"github.com/magelift/magelift/internal/certification"
	gcprecovery "github.com/magelift/magelift/providers/gcp/resilience"
	"github.com/magelift/magelift/sdk"
)

const defaultTimeout = 10 * time.Minute

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "GCP recovery acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("gcp-recovery-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", "", "Google Cloud project ID")
	bucket := flags.String("bucket", "", "uniquely owned disposable Cloud Storage bucket")
	marker := flags.String("marker", "", "single-line MageLift ownership marker")
	fixture := flags.String("fixture", "", "single-line known-content fixture ID")
	destinationName := flags.String("destination", "isolated", "recovery destination: isolated or in-place")
	dataClass := flags.String("data-class", "media", "durable object class: media, infrastructure-state, or audit-evidence")
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
		{*project, "project"}, {*bucket, "bucket"}, {*marker, "marker"}, {*fixture, "fixture"},
	} {
		if err := validatePart(part.value, part.name); err != nil {
			return err
		}
	}
	class, err := parseObjectDataClass(*dataClass)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	objects, err := gcpstorage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("create GCP Cloud Storage acceptance client: %w", err)
	}
	defer func() { runErr = errors.Join(runErr, objects.Close()) }()

	native, err := gcprecovery.NewGCPStorageNativeAPI(ctx, gcprecovery.NativeAPIConfig{
		ArchiveBucket: *bucket,
		RestoreBucket: *bucket,
		Project:       *project,
		ArchivePrefix: "magelift/recovery",
		RestorePrefix: "magelift/recovery/restored",
		RetentionDays: 1,
	})
	if err != nil {
		return fmt.Errorf("construct GCP Cloud Storage recovery translator: %w", err)
	}
	defer func() { runErr = errors.Join(runErr, native.Close()) }()
	operationClient, err := gcprecovery.NewNativeResilienceClient(native)
	if err != nil {
		return fmt.Errorf("construct GCP recovery operation client: %w", err)
	}

	sourcePrefix := objectSourcePrefix(class, *fixture)
	destination, approval, err := parseRecoveryDestination(*destinationName)
	if err != nil {
		return err
	}
	cell, err := certification.NewObjectRecoveryCell(certification.ObjectRecoveryCellConfig{
		Operations:        operationClient,
		Store:             gcpObjectStore{client: objects},
		Bucket:            *bucket,
		ResourceReference: "gs://" + *bucket + "/" + sourcePrefix,
		DataClass:         class,
		Marker:            *marker,
		FixtureID:         *fixture,
		SourcePrefix:      sourcePrefix,
		Destination:       destination,
		ApprovalReference: approval,
		ParseReference:    parseObjectReference,
	})
	if err != nil {
		return fmt.Errorf("construct provider-neutral Cloud Storage recovery cell: %w", err)
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
	defer func() { runErr = errors.Join(runErr, cleanup()) }()

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
		return fmt.Errorf("verify Cloud Storage recovery cleanup: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("Cloud Storage recovery outputs remain after cleanup: %#v", remaining)
	}

	fmt.Fprintf(output, "GCP Cloud Storage recovery acceptance PASS project=%s bucket=%s objects=%d class=%s destination=%s backup=%s restore=%s fixtureFingerprint=%s cleanup=verified\n", *project, *bucket, result.ObjectCount, class, destination, result.BackupID, result.RestoreID, result.FixtureFingerprint)
	return nil
}

func parseRecoveryDestination(value string) (sdk.RecoveryDestination, string, error) {
	switch strings.TrimSpace(value) {
	case "", "isolated":
		return sdk.RecoverySameRegionIsolated, "", nil
	case "in-place":
		return sdk.RecoverySameRegion, "gcp-recovery-acceptance-in-place", nil
	default:
		return "", "", fmt.Errorf("destination must be isolated or in-place, got %q", value)
	}
}

func parseObjectDataClass(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "", "media":
		return "media", nil
	case "infrastructure-state", "audit-evidence":
		return strings.TrimSpace(value), nil
	default:
		return "", fmt.Errorf("data-class must be media, infrastructure-state, or audit-evidence, got %q", value)
	}
}

func objectSourcePrefix(dataClass, fixture string) string {
	switch dataClass {
	case "infrastructure-state":
		return "state/" + fixture
	case "audit-evidence":
		return "audit/" + fixture
	default:
		return "fixture/" + fixture
	}
}

type gcpObjectStore struct {
	client *gcpstorage.Client
}

func (store gcpObjectStore) Put(ctx context.Context, bucket string, object certification.ObjectRecoveryFixture) error {
	writer := store.client.Bucket(bucket).Object(object.Key).NewWriter(ctx)
	writer.ContentType = "application/octet-stream"
	if _, err := writer.Write(object.Body); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write source fixture object %q: %w", object.Key, err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("close source fixture object %q: %w", object.Key, err)
	}
	return nil
}

func (store gcpObjectStore) Delete(ctx context.Context, bucket, key string) error {
	err := store.client.Bucket(bucket).Object(key).Delete(ctx)
	if err != nil && isObjectNotFound(err) {
		return nil
	}
	return err
}

func (store gcpObjectStore) ReadPrefix(ctx context.Context, bucket, prefix string) (map[string][]byte, error) {
	result := make(map[string][]byte)
	objectsIterator := store.client.Bucket(bucket).Objects(ctx, &gcpstorage.Query{Prefix: prefix})
	for {
		attrs, err := objectsIterator.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list Cloud Storage objects: %w", err)
		}
		if attrs == nil || attrs.Name == "" {
			continue
		}
		reader, err := store.client.Bucket(bucket).Object(attrs.Name).NewReader(ctx)
		if err != nil {
			return nil, fmt.Errorf("read Cloud Storage object %q: %w", attrs.Name, err)
		}
		body, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read Cloud Storage object %q: %w", attrs.Name, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close Cloud Storage object %q: %w", attrs.Name, closeErr)
		}
		result[attrs.Name] = body
	}
	return result, nil
}

func parseObjectReference(reference string) (bucket, prefix string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(reference))
	if err != nil || (parsed.Scheme != "gs" && parsed.Scheme != "gcs" && parsed.Scheme != "gcp-storage") || strings.TrimSpace(parsed.Host) == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", fmt.Errorf("Cloud Storage reference %q is invalid", reference)
	}
	prefix, err = url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || strings.TrimSpace(prefix) == "" || strings.ContainsAny(prefix, "\r\n\x00") {
		return "", "", fmt.Errorf("Cloud Storage reference %q does not contain a safe prefix", reference)
	}
	return parsed.Host, prefix, nil
}

func validatePart(value, name string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	return nil
}

func isObjectNotFound(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "not found") || strings.Contains(message, "notfound") || strings.Contains(message, "404")
}
