package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/external/newrelic"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	provider "github.com/magelift/magelift/internal/provider"
)

const (
	credentialReference      = "newrelic-acceptance://license"
	queryCredentialReference = "newrelic-acceptance://user-key"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "newrelic OTLP acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	apiKey := []byte(strings.TrimSpace(os.Getenv("MAGELIFT_NEWRELIC_OTLP_API_KEY")))
	if len(apiKey) == 0 {
		return errors.New("MAGELIFT_NEWRELIC_OTLP_API_KEY is required")
	}
	defer clear(apiKey)
	queryKey := []byte(strings.TrimSpace(os.Getenv("MAGELIFT_NEWRELIC_NERDGRAPH_API_KEY")))
	if len(queryKey) == 0 {
		return errors.New("MAGELIFT_NEWRELIC_NERDGRAPH_API_KEY is required")
	}
	defer clear(queryKey)
	accountID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("MAGELIFT_NEWRELIC_ACCOUNT_ID")), 10, 64)
	if err != nil || accountID <= 0 {
		return errors.New("MAGELIFT_NEWRELIC_ACCOUNT_ID must be a positive integer")
	}
	endpoint := strings.TrimSpace(os.Getenv("MAGELIFT_NEWRELIC_OTLP_ENDPOINT"))
	if endpoint == "" {
		return errors.New("MAGELIFT_NEWRELIC_OTLP_ENDPOINT is required so the data region is explicit")
	}
	nerdGraphEndpoint := strings.TrimSpace(os.Getenv("MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT"))
	if nerdGraphEndpoint == "" {
		return errors.New("MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT is required so the API data region is explicit")
	}
	marker := strings.TrimSpace(os.Getenv("MAGELIFT_NEWRELIC_OTLP_MARKER"))
	if marker == "" {
		marker = "magelift/acceptance/newrelic-otlp/" + time.Now().UTC().Format("20060102-150405")
	}
	signals, err := selectedSignals(os.Getenv("MAGELIFT_NEWRELIC_OTLP_SIGNALS"))
	if err != nil {
		return err
	}
	resolver := provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
		switch reference {
		case credentialReference:
			return consume(apiKey)
		case queryCredentialReference:
			return consume(queryKey)
		default:
			return errors.New("unexpected New Relic acceptance credential reference")
		}
	})
	exporter, err := newrelic.NewClient(nil, resolver)
	if err != nil {
		return err
	}
	queryer, err := newrelic.NewNRQLClientWithQueryCredential(nil, resolver, nerdGraphEndpoint, accountID, queryCredentialReference)
	if err != nil {
		return err
	}
	lifecycle, err := newrelic.NewOTLPLifecycleClientWithQuery(exporter, queryer)
	if err != nil {
		return err
	}
	bindings := make([]providerobservability.SignalBinding, 0, len(signals))
	for _, signal := range signals {
		bindings = append(bindings, newrelicBinding(signal, endpoint, marker))
	}
	plan := providerobservability.Plan{
		TargetProvider:  "newrelic",
		TargetRuntime:   "otlp-live-acceptance",
		OwnershipMarker: marker,
		Bindings:        bindings,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	result, err := lifecycle.Apply(ctx, plan)
	if err != nil {
		return err
	}
	for _, binding := range plan.Bindings {
		observation, err := lifecycle.VerifySignal(ctx, binding)
		if err != nil {
			return err
		}
		if !observation.Delivered || !observation.LabelsVerified {
			return fmt.Errorf("signal %s was not queryably delivered: %s", binding.Signal, observation.Reason)
		}
		fmt.Printf("signal=%s delivered=true labels=true\n", binding.Signal)
	}
	cleanup, err := lifecycle.Destroy(ctx, plan, result.ResourceRefs)
	if err != nil {
		return err
	}
	if !cleanup.Complete || !cleanup.UnownedPreserved {
		return fmt.Errorf("provider resource cleanup proof was incomplete: %#v", cleanup)
	}
	fmt.Printf("newrelic OTLP acceptance PASS endpoint=%s marker=%s resourceCleanup=none providerRetention=managed\n", endpoint, marker)
	return nil
}

func selectedSignals(raw string) ([]newrelic.Signal, error) {
	if strings.TrimSpace(raw) == "" {
		return []newrelic.Signal{newrelic.SignalLogs, newrelic.SignalMetrics, newrelic.SignalTraces}, nil
	}
	var signals []newrelic.Signal
	seen := make(map[newrelic.Signal]struct{})
	for _, value := range strings.Split(raw, ",") {
		signal := newrelic.Signal(strings.TrimSpace(value))
		switch signal {
		case newrelic.SignalLogs, newrelic.SignalMetrics, newrelic.SignalTraces:
		default:
			return nil, fmt.Errorf("unsupported New Relic acceptance signal %q", value)
		}
		if _, exists := seen[signal]; exists {
			return nil, fmt.Errorf("duplicate New Relic acceptance signal %q", signal)
		}
		seen[signal] = struct{}{}
		signals = append(signals, signal)
	}
	if len(signals) == 0 {
		return nil, errors.New("at least one New Relic acceptance signal is required")
	}
	return signals, nil
}

func newrelicBinding(signal newrelic.Signal, endpoint, marker string) providerobservability.SignalBinding {
	return providerobservability.SignalBinding{
		Signal:          string(signal),
		Destination:     "newrelic",
		Mode:            "otlp",
		CredentialRef:   credentialReference,
		Endpoint:        endpoint,
		OwnershipMarker: marker,
	}
}
