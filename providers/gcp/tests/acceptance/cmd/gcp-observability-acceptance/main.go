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

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	gcpobservability "github.com/magelift/magelift/providers/gcp/observability"
	"github.com/magelift/magelift/sdk"
)

const defaultVerifyBudget = 4 * time.Minute

const (
	maxVerifyBudget       = 2 * time.Hour
	lifecycleGracePeriod  = 5 * time.Minute
	defaultLifecycleLimit = 12 * time.Minute
)

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "GCP observability acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("gcp-observability-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", "", "GCP project ID")
	marker := flags.String("marker", "", "single-line MageLift ownership marker")
	verifyBudget := flags.Duration("verify-budget", defaultVerifyBudget, "maximum time to wait for Google Cloud delivery")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	if err := requiredPart(*project, "project"); err != nil {
		return err
	}
	if err := requiredPart(*marker, "marker"); err != nil {
		return err
	}
	if *verifyBudget <= 0 {
		return errors.New("verify-budget must be positive")
	}
	if *verifyBudget > maxVerifyBudget {
		return fmt.Errorf("verify-budget must not exceed %s", maxVerifyBudget)
	}

	ctx, cancel := context.WithTimeout(parent, lifecycleTimeout(*verifyBudget))
	defer cancel()
	plan := acceptancePlan(*marker)
	var client providerobservability.LifecycleClient
	var applied providerobservability.LifecycleResult
	cleanupDone := false
	defer func() {
		if cleanupDone || client == nil {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), 90*time.Second)
		defer cleanupCancel()
		if _, err := client.Destroy(cleanupCtx, plan, applied.ResourceRefs); err != nil {
			fmt.Fprintf(os.Stderr, "GCP observability cleanup failed marker=%s: %v\n", *marker, err)
		}
	}()

	var err error
	client, err = gcpobservability.NewGoogleCloudOperationsSDKClient(ctx, *project)
	if err != nil {
		return fmt.Errorf("construct Google Cloud observability lifecycle client: %w", err)
	}
	applied, err = client.Apply(ctx, plan)
	if err != nil {
		return fmt.Errorf("apply Google Cloud observability acceptance plan: %w", err)
	}
	if err := verifyDelivery(ctx, client, plan, *verifyBudget); err != nil {
		return err
	}

	cleanup, err := client.Destroy(ctx, plan, applied.ResourceRefs)
	if err != nil {
		return fmt.Errorf("destroy Google Cloud observability acceptance plan: %w", err)
	}
	cleanupDone = true
	if !cleanup.Complete || !cleanup.UnownedPreserved {
		return fmt.Errorf("Google Cloud cleanup incomplete: %#v", cleanup)
	}

	fmt.Fprintf(output, "GCP observability acceptance PASS project=%s marker=%s resources=%d cleanup=verified\n", *project, *marker, len(applied.ResourceRefs))
	return nil
}

func lifecycleTimeout(verifyBudget time.Duration) time.Duration {
	minimum := verifyBudget + lifecycleGracePeriod
	if minimum > defaultLifecycleLimit {
		return minimum
	}
	return defaultLifecycleLimit
}

func acceptancePlan(marker string) providerobservability.Plan {
	return providerobservability.Plan{
		TargetProvider:  "gcp",
		TargetRuntime:   "google-cloud-operations-live-cell",
		OwnershipMarker: marker,
		Bindings: []providerobservability.SignalBinding{
			{Signal: "logs", Destination: "google-cloud-operations", Mode: "native", OwnershipMarker: marker, RetentionDays: 1},
			{Signal: "metrics", Destination: "google-cloud-operations", Mode: "native", OwnershipMarker: marker},
		},
		Alerts: []sdk.AlertIntent{{
			ID: "delivery", Signal: "metrics", Severity: "warning", Operator: "gt", Threshold: 0, WindowSeconds: 60,
			Owner: "platform-oncall", RunbookURL: "https://github.com/magelift/magelift/blob/main/docs/operations/ownership-and-escalation.md", DeduplicationKey: "gcp-observability-live/delivery",
		}},
		Dashboards: []sdk.DashboardIntent{{ID: "delivery", Signals: []string{"logs", "metrics"}, Owner: "platform-oncall"}},
	}
}

func verifyDelivery(ctx context.Context, client providerobservability.LifecycleClient, plan providerobservability.Plan, budget time.Duration) error {
	deadline := time.NewTimer(budget)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	publisher, ok := client.(interface {
		PublishSignalProbe(context.Context, providerobservability.SignalBinding) error
	})
	if !ok {
		return errors.New("Google Cloud observability client does not support data-plane probe publication")
	}

	for {
		for _, binding := range plan.Bindings {
			if err := publisher.PublishSignalProbe(ctx, binding); err != nil {
				return fmt.Errorf("republish Google Cloud observability probe for %s: %w", binding.Signal, err)
			}
		}
		allSignals, signalFailure := verifySignals(ctx, client, plan.Bindings)
		operations, operationErr := client.VerifyOperations(ctx, plan)
		if allSignals && operationErr == nil && operations.AlertsVerified && operations.DashboardsVerified {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("verify Google Cloud observability delivery: %w", ctx.Err())
		case <-deadline.C:
			if operationErr != nil {
				return fmt.Errorf("verify Google Cloud observability operations: %w", operationErr)
			}
			return fmt.Errorf("verify Google Cloud observability delivery timed out: %s", signalFailure)
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

func requiredPart(value, name string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	return nil
}
