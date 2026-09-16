// Command gcp-pubsub-acceptance runs one disposable, provider-backed Pub/Sub
// snapshot/seek acceptance cell. The shell wrapper owns topic/subscription
// creation and cleanup; this command owns the MageLift snapshot lifecycle.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	pubsub "cloud.google.com/go/pubsub/v2"
	gcpresilience "github.com/magelift/magelift/internal/cloud/gcp/resilience"
	"github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

const (
	defaultRetentionDays = 7
	defaultTimeout       = 2 * time.Minute
)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "gcp Pub/Sub acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("gcp-pubsub-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", "", "GCP project ID")
	subscription := flags.String("subscription", "", "GCP Pub/Sub subscription ID")
	marker := flags.String("marker", "", "single-line ownership marker")
	fixture := flags.String("fixture", "", "single-line queue fixture ID")
	retentionDays := flags.Int("retention-days", defaultRetentionDays, "requested snapshot retention in days")
	destinationName := flags.String("destination", "same-region", "recovery destination: same-region or isolated")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := validatePart(*project, "project", false); err != nil {
		return err
	}
	if err := validatePart(*subscription, "subscription", true); err != nil {
		return err
	}
	if err := validatePart(*marker, "marker", false); err != nil {
		return err
	}
	if err := validatePart(*fixture, "fixture", false); err != nil {
		return err
	}
	if *retentionDays <= 0 {
		return errors.New("retention-days must be positive")
	}
	destination, err := parsePubSubRecoveryDestination(*destinationName)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	client, err := pubsub.NewClient(ctx, *project)
	if err != nil {
		return fmt.Errorf("create Pub/Sub verification client: %w", err)
	}
	defer func() { runErr = errors.Join(runErr, client.Close()) }()
	verifier := queueVerifier{client: client}

	native, err := gcpresilience.NewGCPPubSubNativeAPI(ctx, gcpresilience.NativeAPIConfig{
		Project: *project, RetentionDays: *retentionDays, Verifier: verifier,
	})
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, native.Close()) }()
	defer func() { runErr = errors.Join(runErr, native.DeleteOwnedPubSubSnapshots(ctx, *marker)) }()

	operationClient, err := gcpresilience.NewNativeResilienceClient(native)
	if err != nil {
		return err
	}
	resource := "gcp-pubsub://projects/" + *project + "/subscriptions/" + *subscription
	base := sdk.ResilienceOperationRequest{
		DataClasses: []string{"queue"}, Destination: destination,
		FixtureID: *fixture, OwnershipMarker: *marker,
		ResourceReferences: map[string]string{"queue": resource},
	}

	backupRequest := base
	backupRequest.Action = sdk.ResilienceBackup
	backupRequest.IdempotencyKey = "live/queue/backup"
	backup, err := operationClient.Start(ctx, backupRequest)
	if err != nil {
		return fmt.Errorf("start Pub/Sub backup: %w", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || len(backup.Evidence) != 1 || backup.Evidence[0].BackupID == "" {
		return fmt.Errorf("Pub/Sub backup did not produce complete success evidence: status=%s evidence=%d", backup.Status, len(backup.Evidence))
	}
	backupID := backup.Evidence[0].BackupID

	integrityRequest := base
	integrityRequest.Action = sdk.ResilienceIntegrityCheck
	integrityRequest.IdempotencyKey = "live/queue/integrity"
	integrityRequest.BackupReferences = map[string]string{"queue": backupID}
	if integrity, err := operationClient.Start(ctx, integrityRequest); err != nil {
		return fmt.Errorf("verify Pub/Sub queue integrity: %w", err)
	} else if integrity.Status != sdk.ResilienceOperationSucceeded || len(integrity.Evidence) != 1 || !integrity.Evidence[0].CountsVerified || !integrity.Evidence[0].ApplicationReadsVerified || !integrity.Evidence[0].PermissionsVerified || !integrity.Evidence[0].ServiceHealthVerified {
		return fmt.Errorf("Pub/Sub integrity evidence is incomplete: status=%s evidence=%#v", integrity.Status, integrity.Evidence)
	}

	restoreRequest := base
	restoreRequest.Action = sdk.ResilienceRestore
	restoreRequest.IdempotencyKey = "live/queue/restore"
	restoreRequest.BackupReferences = map[string]string{"queue": backupID}
	restored, err := operationClient.Start(ctx, restoreRequest)
	if err != nil {
		return fmt.Errorf("restore Pub/Sub queue by snapshot seek: %w", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || len(restored.Evidence) != 1 || !restored.Evidence[0].ApplicationReadsVerified || restored.Evidence[0].RestoreID == "" {
		return fmt.Errorf("Pub/Sub restore evidence is incomplete: status=%s evidence=%#v", restored.Status, restored.Evidence)
	}
	restoreID := restored.Evidence[0].RestoreID
	if destination == sdk.RecoverySameRegion && restoreID != "projects/"+*project+"/subscriptions/"+*subscription {
		return fmt.Errorf("same-region Pub/Sub restore targeted %q, want the source subscription", restoreID)
	}
	if destination == sdk.RecoverySameRegionIsolated && (restoreID == "projects/"+*project+"/subscriptions/"+*subscription || !strings.Contains(restoreID, "/subscriptions/magelift-restore-")) {
		return fmt.Errorf("isolated Pub/Sub restore targeted %q, want an owned magelift-restore subscription", restoreID)
	}

	inventory, err := native.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("inventory Pub/Sub acceptance resources: %w", err)
	}
	if err := verifyPubSubInventory(inventory, backupID, restoreID, destination); err != nil {
		return err
	}
	if err := native.DeleteOwnedPubSubSnapshots(ctx, *marker); err != nil {
		return fmt.Errorf("delete Pub/Sub acceptance snapshot: %w", err)
	}
	remaining, err := native.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("verify Pub/Sub acceptance cleanup: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("Pub/Sub acceptance snapshot remains after cleanup: %#v", remaining)
	}

	fmt.Fprintf(output, "gcp Pub/Sub acceptance PASS project=%s subscription=%s snapshot=%s restore=%s destination=%s retentionDays=%d cleanup=verified\n", *project, *subscription, backupID, restoreID, destination, *retentionDays)
	return nil
}

func parsePubSubRecoveryDestination(value string) (sdk.RecoveryDestination, error) {
	switch strings.TrimSpace(value) {
	case "", "same-region":
		return sdk.RecoverySameRegion, nil
	case "isolated":
		return sdk.RecoverySameRegionIsolated, nil
	default:
		return "", fmt.Errorf("destination must be same-region or isolated, got %q", value)
	}
}

func verifyPubSubInventory(inventory []provider.InventoryResource, backupID, restoreID string, destination sdk.RecoveryDestination) error {
	want := map[string]struct{}{"gcp-pubsub-snapshot://" + backupID: {}}
	if destination == sdk.RecoverySameRegionIsolated {
		want["gcp-pubsub://"+restoreID] = struct{}{}
	}
	if len(inventory) != len(want) {
		return fmt.Errorf("unexpected Pub/Sub acceptance inventory: %#v", inventory)
	}
	for _, resource := range inventory {
		if _, ok := want[resource.Identity]; !ok || !resource.Owned || !resource.Live {
			return fmt.Errorf("unexpected Pub/Sub acceptance inventory: %#v", inventory)
		}
	}
	return nil
}

func validatePart(value, name string, rejectSlash bool) error {
	if strings.ContainsAny(value, "\r\n\x00?#") || (rejectSlash && strings.Contains(value, "/")) {
		return fmt.Errorf("%s contains unsupported characters", name)
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

type queueVerifier struct {
	client *pubsub.Client
}

func (v queueVerifier) Verify(ctx context.Context, request gcpresilience.RecoveryVerificationRequest) (gcpresilience.RecoveryVerification, error) {
	if err := v.receiveFixture(ctx, request, true); err != nil {
		return gcpresilience.RecoveryVerification{}, fmt.Errorf("receive known Pub/Sub fixture: %w", err)
	}
	return gcpresilience.RecoveryVerification{CountsVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true}, nil
}

func (v queueVerifier) WaitForFixture(ctx context.Context, request gcpresilience.RecoveryVerificationRequest) error {
	return v.receiveFixture(ctx, request, false)
}

func (v queueVerifier) receiveFixture(ctx context.Context, request gcpresilience.RecoveryVerificationRequest, acknowledge bool) error {
	if v.client == nil {
		return errors.New("Pub/Sub verifier client is required")
	}
	expected := "magelift-pubsub:" + request.FixtureID + ":" + request.OwnershipMarker
	receiveContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	received := make(chan struct{}, 1)
	subscriber := v.client.Subscriber(request.Resource)
	subscriber.ReceiveSettings = pubsub.ReceiveSettings{NumGoroutines: 1, MaxOutstandingMessages: 1, MaxExtension: 30 * time.Second}
	err := subscriber.Receive(receiveContext, func(_ context.Context, message *pubsub.Message) {
		if string(message.Data) != expected {
			message.Nack()
			return
		}
		if acknowledge {
			message.Ack()
		} else {
			message.Nack()
		}
		select {
		case received <- struct{}{}:
		default:
		}
		cancel()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("receive fixture from subscription: %w", err)
	}
	select {
	case <-received:
		return nil
	default:
		return errors.New("known Pub/Sub fixture was not received")
	}
}
