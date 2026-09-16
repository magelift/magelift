// Command aws-sqs-acceptance runs one disposable, provider-backed AWS SQS
// bounded export/replay cell. The shell wrapper owns the source queue; this
// command owns the MageLift export/restore queues and their verifier evidence.
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

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	awsresilience "github.com/magelift/magelift/internal/cloud/aws/resilience"
	"github.com/magelift/magelift/sdk"
)

const defaultTimeout = 5 * time.Minute

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "AWS SQS acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	flags := flag.NewFlagSet("aws-sqs-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	region := flags.String("region", "", "AWS region")
	sourceURL := flags.String("source-url", "", "source SQS queue URL")
	marker := flags.String("marker", "", "single-line ownership marker")
	fixture := flags.String("fixture", "", "single-line known-message fixture ID")
	retentionDays := flags.Int("retention-days", 1, "export and restore queue retention in days")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	for value, name := range map[string]string{*region: "region", *sourceURL: "source-url", *marker: "marker", *fixture: "fixture"} {
		if err := validatePart(value, name); err != nil {
			return err
		}
	}
	if *retentionDays <= 0 || *retentionDays > 14 {
		return errors.New("retention-days must be between one and fourteen")
	}
	if !strings.HasPrefix(*sourceURL, "https://") {
		return errors.New("source-url must use https")
	}

	ctx, cancel := context.WithTimeout(parent, defaultTimeout)
	defer cancel()
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(*region))
	if err != nil {
		return fmt.Errorf("load AWS SDK configuration: %w", err)
	}
	verificationClient := sqs.NewFromConfig(awsConfig)
	verifier := queueVerifier{client: verificationClient}
	native, err := awsresilience.NewAWSSQSNativeAPI(ctx, awsresilience.NativeAPIConfig{
		RetentionDays: *retentionDays, QueueMaxMessages: 10, QueueVisibilityTimeout: 30, Verifier: verifier,
	}, awsconfig.WithRegion(*region))
	if err != nil {
		return err
	}
	cleanupDone := false
	cleanup := func() error {
		if cleanupDone {
			return nil
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(parent), 45*time.Second)
		defer cleanupCancel()
		client, clientErr := awsresilience.NewNativeResilienceClient(native)
		if clientErr != nil {
			return clientErr
		}
		_, cleanupErr := client.Start(cleanupCtx, sdk.ResilienceOperationRequest{
			Action: sdk.ResilienceCleanup, DataClasses: []string{"queue"}, Destination: sdk.RecoverySameRegionIsolated,
			FixtureID: *fixture, OwnershipMarker: *marker, IdempotencyKey: "live/queue/cleanup", ResourceReferences: map[string]string{"queue": "aws-sqs://" + *sourceURL},
		})
		if cleanupErr == nil {
			cleanupDone = true
		}
		return cleanupErr
	}
	defer func() { runErr = errors.Join(runErr, cleanup()) }()

	operationClient, err := awsresilience.NewNativeResilienceClient(native)
	if err != nil {
		return err
	}
	resource := "aws-sqs://" + *sourceURL
	base := sdk.ResilienceOperationRequest{
		DataClasses: []string{"queue"}, Destination: sdk.RecoverySameRegionIsolated,
		FixtureID: *fixture, OwnershipMarker: *marker,
		ResourceReferences: map[string]string{"queue": resource},
	}
	backupRequest := base
	backupRequest.Action = sdk.ResilienceBackup
	backupRequest.IdempotencyKey = "live/queue/backup"
	backupRequest.ApprovalReference = "live-quiescence-approved"
	backup, err := operationClient.Start(ctx, backupRequest)
	if err != nil {
		return fmt.Errorf("start SQS backup: %w", err)
	}
	if backup.Status != sdk.ResilienceOperationSucceeded || len(backup.Evidence) != 1 || backup.Evidence[0].BackupID == "" {
		return fmt.Errorf("SQS backup did not produce complete success evidence: status=%s evidence=%d", backup.Status, len(backup.Evidence))
	}
	backupID := backup.Evidence[0].BackupID

	integrityRequest := base
	integrityRequest.Action = sdk.ResilienceIntegrityCheck
	integrityRequest.IdempotencyKey = "live/queue/integrity"
	integrityRequest.BackupReferences = map[string]string{"queue": backupID}
	integrity, err := operationClient.Start(ctx, integrityRequest)
	if err != nil {
		return fmt.Errorf("verify SQS export integrity: %w", err)
	}
	if integrity.Status != sdk.ResilienceOperationSucceeded || len(integrity.Evidence) != 1 || !integrity.Evidence[0].CountsVerified || !integrity.Evidence[0].ApplicationReadsVerified || !integrity.Evidence[0].PermissionsVerified || !integrity.Evidence[0].ServiceHealthVerified {
		return fmt.Errorf("SQS integrity evidence is incomplete: status=%s evidence=%#v", integrity.Status, integrity.Evidence)
	}

	restoreRequest := base
	restoreRequest.Action = sdk.ResilienceRestore
	restoreRequest.IdempotencyKey = "live/queue/restore"
	restoreRequest.BackupReferences = map[string]string{"queue": backupID}
	restored, err := operationClient.Start(ctx, restoreRequest)
	if err != nil {
		return fmt.Errorf("restore SQS queue into isolated destination: %w", err)
	}
	if restored.Status != sdk.ResilienceOperationSucceeded || len(restored.Evidence) != 1 || !restored.Evidence[0].CountsVerified || !restored.Evidence[0].ApplicationReadsVerified {
		return fmt.Errorf("SQS restore evidence is incomplete: status=%s evidence=%#v", restored.Status, restored.Evidence)
	}

	inventory, err := native.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("inventory SQS acceptance resources: %w", err)
	}
	if len(inventory) != 2 {
		return fmt.Errorf("unexpected SQS acceptance inventory before cleanup: %#v", inventory)
	}
	if err := cleanup(); err != nil {
		return fmt.Errorf("cleanup SQS recovery queues: %w", err)
	}
	remaining, err := native.Inventory(ctx, *marker)
	if err != nil {
		return fmt.Errorf("verify SQS acceptance cleanup: %w", err)
	}
	if len(remaining) != 0 {
		return fmt.Errorf("SQS recovery queues remain after cleanup: %#v", remaining)
	}

	fmt.Fprintf(output, "AWS SQS acceptance PASS region=%s source=%s backup=%s restore=%s retentionDays=%d cleanup=verified\n", *region, *sourceURL, backupID, restored.Evidence[0].RestoreID, *retentionDays)
	return nil
}

type queueVerifier struct {
	client *sqs.Client
}

func (v queueVerifier) Verify(ctx context.Context, request awsresilience.RecoveryVerificationRequest) (awsresilience.RecoveryVerification, error) {
	if v.client == nil {
		return awsresilience.RecoveryVerification{}, errors.New("SQS verification client is required")
	}
	expected := "magelift-sqs:" + request.FixtureID + ":" + request.OwnershipMarker
	received, err := v.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{QueueUrl: aws.String(request.Resource), MaxNumberOfMessages: 10, MessageAttributeNames: []string{"All"}, VisibilityTimeout: 30})
	if err != nil {
		return awsresilience.RecoveryVerification{}, fmt.Errorf("receive known SQS fixture: %w", err)
	}
	if received == nil {
		return awsresilience.RecoveryVerification{}, errors.New("SQS verifier received no response")
	}
	found := false
	entries := make([]sqstypes.ChangeMessageVisibilityBatchRequestEntry, 0, len(received.Messages))
	for index, message := range received.Messages {
		if aws.ToString(message.Body) == expected {
			found = true
		}
		if strings.TrimSpace(aws.ToString(message.ReceiptHandle)) == "" {
			return awsresilience.RecoveryVerification{}, errors.New("SQS verifier received a message without a receipt handle")
		}
		entries = append(entries, sqstypes.ChangeMessageVisibilityBatchRequestEntry{Id: aws.String(fmt.Sprintf("verify-%d", index)), ReceiptHandle: message.ReceiptHandle, VisibilityTimeout: 0})
	}
	if len(entries) > 0 {
		visibility, err := v.client.ChangeMessageVisibilityBatch(ctx, &sqs.ChangeMessageVisibilityBatchInput{QueueUrl: aws.String(request.Resource), Entries: entries})
		if err != nil {
			return awsresilience.RecoveryVerification{}, fmt.Errorf("reset SQS verifier visibility: %w", err)
		}
		if visibility == nil || len(visibility.Failed) > 0 {
			return awsresilience.RecoveryVerification{}, errors.New("SQS verifier could not reset every message visibility")
		}
	}
	if !found {
		return awsresilience.RecoveryVerification{}, errors.New("known SQS fixture message was not observed")
	}
	return awsresilience.RecoveryVerification{CountsVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true, ServiceHealthVerified: true}, nil
}

func validatePart(value, name string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	return nil
}
