package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/external/newrelic"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

const operationsCredentialReference = "newrelic-acceptance://user-key"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "newrelic operations acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	apiKey := []byte(strings.TrimSpace(os.Getenv("MAGELIFT_NEWRELIC_OPERATIONS_API_KEY")))
	if len(apiKey) == 0 {
		return errors.New("MAGELIFT_NEWRELIC_OPERATIONS_API_KEY is required")
	}
	defer clear(apiKey)
	accountID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("MAGELIFT_NEWRELIC_ACCOUNT_ID")), 10, 64)
	if err != nil || accountID <= 0 {
		return errors.New("MAGELIFT_NEWRELIC_ACCOUNT_ID must be a positive integer")
	}
	endpoint := strings.TrimSpace(os.Getenv("MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT"))
	if endpoint == "" {
		return errors.New("MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT is required so the API region is explicit")
	}
	marker := strings.TrimSpace(os.Getenv("MAGELIFT_NEWRELIC_OPERATIONS_MARKER"))
	if marker == "" {
		return errors.New("MAGELIFT_NEWRELIC_OPERATIONS_MARKER is required")
	}
	entityGUID := strings.TrimSpace(os.Getenv("MAGELIFT_NEWRELIC_OPERATIONS_SLO_ENTITY_GUID"))
	if entityGUID == "" {
		return errors.New("MAGELIFT_NEWRELIC_OPERATIONS_SLO_ENTITY_GUID is required")
	}

	resolver := provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
		if reference != operationsCredentialReference {
			return errors.New("unexpected New Relic operations credential reference")
		}
		return consume(apiKey)
	})
	operations, err := newrelic.NewNerdGraphOperations(http.DefaultClient, resolver, endpoint, accountID, operationsCredentialReference)
	if err != nil {
		return err
	}
	lifecycle, err := newrelic.NewNerdGraphLifecycleClient(operations)
	if err != nil {
		return err
	}
	plan := providerobservability.Plan{
		TargetProvider:  "aws",
		TargetRuntime:   "ecs-fargate",
		OwnershipMarker: marker,
		NativeReference: entityGUID,
		Alerts: []sdk.AlertIntent{{
			ID: "telemetry-loss", Signal: "metrics", Severity: "warning", Operator: "gt", Threshold: 0,
			WindowSeconds: 60, Owner: "platform-oncall", RunbookURL: "https://github.com/magelift/magelift/blob/main/docs/operations/ownership-and-escalation.md", DeduplicationKey: "newrelic-telemetry-loss",
		}},
		Dashboards: []sdk.DashboardIntent{{ID: "operations", Signals: []string{"metrics"}, Owner: "platform-oncall"}},
		SLOs: []sdk.SLOIntent{{
			ID: "availability", Signal: "application-health", Target: 0.999, WindowSeconds: 24 * 60 * 60,
			Owner: "platform-oncall", RunbookURL: "https://github.com/magelift/magelift/blob/main/docs/operations/ownership-and-escalation.md", ErrorBudgetPolicy: "page",
		}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	result, err := lifecycle.Apply(ctx, plan)
	if err != nil {
		return err
	}
	cleaned := false
	defer func() {
		if !cleaned {
			_, _ = lifecycle.Destroy(context.Background(), plan, result.ResourceRefs)
		}
	}()
	observation, err := lifecycle.VerifyOperations(ctx, plan)
	if err != nil {
		return err
	}
	if !observation.AlertsVerified || !observation.DashboardsVerified || !observation.SLOsVerified {
		return fmt.Errorf("New Relic operational verification incomplete: %#v", observation)
	}
	cleanup, err := lifecycle.Destroy(ctx, plan, result.ResourceRefs)
	if err != nil {
		return err
	}
	cleaned = true
	if !cleanup.Complete || !cleanup.UnownedPreserved {
		return fmt.Errorf("New Relic operational cleanup incomplete: %#v", cleanup)
	}
	fmt.Printf("newrelic operations acceptance PASS account=%d marker=%s alerts=true dashboards=true slos=true cleanup=verified\n", accountID, marker)
	return nil
}
