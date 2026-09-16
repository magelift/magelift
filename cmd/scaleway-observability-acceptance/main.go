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

	scalewayobservability "github.com/magelift/magelift/internal/cloud/scaleway/observability"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
)

const defaultVerifyBudget = 4 * time.Minute

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "Scaleway observability acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("scaleway-observability-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	project := flags.String("project", "", "Scaleway project ID")
	region := flags.String("region", "", "Scaleway Cockpit region")
	marker := flags.String("marker", "", "single-line MageLift ownership marker")
	verifyBudget := flags.Duration("verify-budget", defaultVerifyBudget, "maximum time to wait for Cockpit delivery")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	for name, value := range map[string]string{"project": *project, "region": *region, "marker": *marker} {
		if err := requiredPart(value, name); err != nil {
			return err
		}
	}
	if *verifyBudget <= 0 {
		return errors.New("verify-budget must be positive")
	}

	ctx, cancel := context.WithTimeout(parent, 12*time.Minute)
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
			fmt.Fprintf(os.Stderr, "Scaleway observability cleanup failed marker=%s: %v\n", *marker, err)
		}
	}()

	var err error
	client, err = scalewayobservability.NewScalewayCockpitSDKClient(ctx, *project, *region)
	if err != nil {
		return fmt.Errorf("construct Scaleway Cockpit lifecycle client: %w", err)
	}
	applied, err = client.Apply(ctx, plan)
	if err != nil {
		return fmt.Errorf("apply Scaleway Cockpit acceptance plan: %w", err)
	}
	if err := verifyDelivery(ctx, client, plan, *verifyBudget); err != nil {
		return err
	}

	cleanup, err := client.Destroy(ctx, plan, applied.ResourceRefs)
	if err != nil {
		return fmt.Errorf("destroy Scaleway Cockpit acceptance plan: %w", err)
	}
	cleanupDone = true
	if !cleanup.Complete || !cleanup.UnownedPreserved {
		return fmt.Errorf("Scaleway Cockpit cleanup incomplete: %#v", cleanup)
	}
	fmt.Fprintf(output, "Scaleway Cockpit observability acceptance PASS region=%s marker=%s resources=%d retentionDays=1 cleanup=verified\n", *region, *marker, len(applied.ResourceRefs))
	return nil
}

func acceptancePlan(marker string) providerobservability.Plan {
	return providerobservability.Plan{
		TargetProvider:  "scaleway",
		TargetRuntime:   "scaleway-cockpit-live-cell",
		OwnershipMarker: marker,
		Bindings: []providerobservability.SignalBinding{
			{Signal: "logs", Destination: "scaleway-cockpit", Mode: "native", OwnershipMarker: marker, RetentionDays: 1},
			{Signal: "metrics", Destination: "scaleway-cockpit", Mode: "native", OwnershipMarker: marker, RetentionDays: 1},
		},
		Alerts:     []sdk.AlertIntent{},
		Dashboards: []sdk.DashboardIntent{},
	}
}

func verifyDelivery(ctx context.Context, client providerobservability.LifecycleClient, plan providerobservability.Plan, budget time.Duration) error {
	deadline := time.NewTimer(budget)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	var lastReason string
	for {
		all := true
		for _, binding := range plan.Bindings {
			observation, err := client.VerifySignal(ctx, binding)
			if err != nil {
				all = false
				lastReason = err.Error()
				continue
			}
			if !observation.Delivered || !observation.LabelsVerified || !observation.RetentionVerified || !observation.RedactionVerified {
				all = false
				lastReason = fmt.Sprintf("signal=%s delivered=%t labels=%t retention=%t redaction=%t reason=%s", observation.Signal, observation.Delivered, observation.LabelsVerified, observation.RetentionVerified, observation.RedactionVerified, observation.Reason)
			}
		}
		operations, err := client.VerifyOperations(ctx, plan)
		if all && err == nil && operations.AlertsVerified && operations.DashboardsVerified && operations.SLOsVerified {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("verify Scaleway Cockpit observability delivery: %w", ctx.Err())
		case <-deadline.C:
			return fmt.Errorf("verify Scaleway Cockpit observability delivery timed out: %s", lastReason)
		case <-ticker.C:
		}
	}
}

func requiredPart(value, name string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	return nil
}
