package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	awsobservability "github.com/magelift/magelift/internal/cloud/aws/observability"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk/v1"
)

const (
	defaultRetentionDays = 1
	defaultVerifyBudget  = 4 * time.Minute
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("aws-cloudwatch-acceptance", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profile := flags.String("profile", "default", "AWS shared-config profile")
	region := flags.String("region", "", "AWS region; required when the profile has no default")
	marker := flags.String("marker", "", "unique MageLift ownership marker")
	retention := flags.Int("retention-days", defaultRetentionDays, "CloudWatch log retention in days")
	verifyBudget := flags.Duration("verify-budget", defaultVerifyBudget, "maximum time to wait for CloudWatch delivery")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*profile) == "" || strings.ContainsAny(*profile, "\r\n\x00") {
		return errors.New("AWS profile must be a non-empty single-line value")
	}
	if strings.TrimSpace(*region) == "" || strings.ContainsAny(*region, "\r\n\x00") {
		return errors.New("AWS region is required")
	}
	if strings.TrimSpace(*marker) == "" || strings.ContainsAny(*marker, "\r\n\x00") {
		return errors.New("ownership marker is required")
	}
	if *retention <= 0 {
		return errors.New("retention-days must be positive")
	}
	if *verifyBudget <= 0 {
		return errors.New("verify-budget must be positive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()

	plan := acceptancePlan(*marker, *retention)
	var client providerobservability.LifecycleClient
	var applied providerobservability.LifecycleResult
	cleanupDone := false
	defer func() {
		if cleanupDone {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), 90*time.Second)
		defer cleanupCancel()
		if client == nil {
			return
		}
		if _, err := client.Destroy(cleanupCtx, plan, applied.ResourceRefs); err != nil {
			fmt.Fprintf(os.Stderr, "AWS CloudWatch acceptance cleanup failed marker=%s: %v\n", *marker, err)
		}
	}()

	awsConfig, err := config.LoadDefaultConfig(ctx, config.WithRegion(*region))
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}
	client, err = awsobservability.NewCloudWatchSDKClient(awsConfig)
	if err != nil {
		return fmt.Errorf("construct CloudWatch lifecycle client: %w", err)
	}

	applied, err = client.Apply(ctx, plan)
	if err != nil {
		return fmt.Errorf("apply CloudWatch acceptance plan: %w", err)
	}
	if err := verifyDelivery(ctx, client, plan, *verifyBudget); err != nil {
		return err
	}

	cleanup, err := client.Destroy(ctx, plan, applied.ResourceRefs)
	if err != nil {
		return fmt.Errorf("destroy CloudWatch acceptance plan: %w", err)
	}
	cleanupDone = true
	if !cleanup.Complete || !cleanup.UnownedPreserved {
		return fmt.Errorf("CloudWatch cleanup incomplete: %#v", cleanup)
	}

	fmt.Printf("AWS CloudWatch acceptance PASS region=%s marker=%s resources=%d retentionDays=%d cleanup=verified\n", *region, *marker, len(applied.ResourceRefs), *retention)
	return nil
}

func acceptancePlan(marker string, retention int) providerobservability.Plan {
	bindings := []providerobservability.SignalBinding{
		{Signal: "logs", Destination: "cloudwatch", Mode: "native", OwnershipMarker: marker, RetentionDays: retention},
		{Signal: "metrics", Destination: "cloudwatch", Mode: "native", OwnershipMarker: marker},
	}
	return providerobservability.Plan{
		TargetProvider:  "aws",
		TargetRuntime:   "cloudwatch-live-cell",
		OwnershipMarker: marker,
		Bindings:        bindings,
		Alerts: []v1.AlertIntent{{
			ID: "delivery", Signal: "metrics", Severity: "warning", Operator: "gt", Threshold: 0, WindowSeconds: 60,
			Owner: "platform-oncall", RunbookURL: "https://github.com/magelift/magelift/blob/main/docs/operations/ownership-and-escalation.md", DeduplicationKey: "aws-cloudwatch-live/delivery",
		}},
		Dashboards: []v1.DashboardIntent{{ID: "delivery", Signals: []string{"logs", "metrics"}, Owner: "platform-oncall"}},
	}
}

func verifyDelivery(ctx context.Context, client providerobservability.LifecycleClient, plan providerobservability.Plan, budget time.Duration) error {
	deadline := time.NewTimer(budget)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		allSignals, signalFailure := verifySignals(ctx, client, plan.Bindings)
		operations, operationErr := client.VerifyOperations(ctx, plan)
		if allSignals && operationErr == nil && operations.AlertsVerified && operations.DashboardsVerified {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("verify CloudWatch delivery: %w", ctx.Err())
		case <-deadline.C:
			if operationErr != nil {
				return fmt.Errorf("verify CloudWatch operations: %w", operationErr)
			}
			return fmt.Errorf("verify CloudWatch delivery timed out: %s", signalFailure)
		case <-ticker.C:
		}
	}
}

func verifySignals(ctx context.Context, client providerobservability.LifecycleClient, bindings []providerobservability.SignalBinding) (bool, string) {
	all := true
	var failure string
	for _, binding := range bindings {
		observation, err := client.VerifySignal(ctx, binding)
		if err != nil {
			all = false
			failure = err.Error()
			continue
		}
		if !observation.Delivered || !observation.LabelsVerified || !observation.RetentionVerified || !observation.RedactionVerified {
			all = false
			failure = fmt.Sprintf("signal=%s delivered=%t labels=%t retention=%t redaction=%t reason=%s", observation.Signal, observation.Delivered, observation.LabelsVerified, observation.RetentionVerified, observation.RedactionVerified, observation.Reason)
		}
	}
	return all, failure
}
