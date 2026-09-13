package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	awsobservability "github.com/magelift/magelift/internal/cloud/aws/observability"
	"github.com/magelift/magelift/internal/external/newrelic"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	queryCredentialReference = "newrelic-acceptance://user-key"
	defaultCollectorSignals  = "logs,metrics,traces"
	defaultCollectorTimeout  = 10 * time.Minute
	collectorCleanupTimeout  = 90 * time.Second
)

type options struct {
	region            string
	cluster           string
	service           string
	secretReference   string
	secretARN         string
	endpoint          string
	nerdGraphEndpoint string
	accountID         int64
	imageDigest       string
	marker            string
	signals           []string
	timeout           time.Duration
	queryKey          []byte
}

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "AWS ECS collector acceptance failed: %v\n", err)
		os.Exit(1)
	}
}

func runMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Stdout)
}

func run(parent context.Context, args []string, output io.Writer) (runErr error) {
	if parent == nil {
		return errors.New("AWS ECS collector acceptance context is required")
	}
	parsed, err := parseOptions(args)
	if err != nil {
		return err
	}
	defer clear(parsed.queryKey)

	ctx, cancel := context.WithTimeout(parent, parsed.timeout)
	defer cancel()
	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(parsed.region))
	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}

	secretResolver := awsobservability.ECSSecretReferenceResolverFunc(func(ctx context.Context, reference string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if reference != parsed.secretReference {
			return "", errors.New("unexpected AWS ECS collector credential reference")
		}
		return parsed.secretARN, nil
	})
	queryResolver := provider.CredentialResolverFunc(func(ctx context.Context, reference string, consume func([]byte) error) error {
		if reference != queryCredentialReference {
			return errors.New("unexpected New Relic query credential reference")
		}
		key := append([]byte(nil), parsed.queryKey...)
		defer clear(key)
		return consume(key)
	})
	queryer, err := newrelic.NewNRQLClientWithQueryCredentialAndRetry(
		&http.Client{Timeout: 30 * time.Second}, queryResolver, parsed.nerdGraphEndpoint, parsed.accountID, queryCredentialReference,
		newrelic.RetryPolicy{MaxAttempts: 10, InitialDelay: 30 * time.Second, MaxDelay: 60 * time.Second},
	)
	if err != nil {
		return fmt.Errorf("construct New Relic query verifier: %w", err)
	}
	probe, err := awsobservability.NewECSNewRelicSignalProbe(queryer, queryCredentialReference)
	if err != nil {
		return fmt.Errorf("construct ECS collector signal probe: %w", err)
	}
	adapter, err := awsobservability.NewECSContribCollectorDeploymentAdapter(ecs.NewFromConfig(awsConfig), awsobservability.ECSCollectorConfig{
		ServiceReference: parsed.cluster + "/" + parsed.service,
		ImageDigest:      parsed.imageDigest,
		ConfigReference:  "env:MAGELIFT_OTEL_CONFIG",
		Probe:            probe,
		SecretResolver:   secretResolver,
	})
	if err != nil {
		return fmt.Errorf("construct ECS collector adapter: %w", err)
	}
	request := collectorRequest(parsed)
	cleanupCollector := true
	defer func() {
		if !cleanupCollector {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), collectorCleanupTimeout)
		defer cleanupCancel()
		_, destroyed, cleanupErr := sdk.RunCollectorDeploymentLifecycle(cleanupCtx, adapter, request, sdk.CollectorDestroy, parsed.marker+":destroy", nil, "")
		if cleanupErr != nil {
			runErr = errors.Join(runErr, cleanupErr)
			return
		}
		if !destroyed.CleanupVerified {
			runErr = errors.Join(runErr, errors.New("ECS collector cleanup did not prove direct inventory convergence"))
		}
	}()

	_, applied, err := sdk.RunCollectorDeploymentLifecycle(ctx, adapter, request, sdk.CollectorApply, parsed.marker+":apply", nil, "")
	if err != nil {
		return fmt.Errorf("apply ECS collector: %w", err)
	}
	if !applied.ReadyVerified || !applied.HealthVerified || !applied.OwnershipVerified {
		return errors.New("ECS collector apply returned incomplete readiness, health, or ownership proof")
	}
	if _, verified, err := sdk.RunCollectorDeploymentLifecycle(ctx, adapter, request, sdk.CollectorVerify, parsed.marker+":verify", applied.ResourceRefs, ""); err != nil {
		return fmt.Errorf("verify ECS collector: %w", err)
	} else if !verified.ReadyVerified || !verified.HealthVerified {
		return errors.New("ECS collector verify returned incomplete readiness or health proof")
	}

	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), collectorCleanupTimeout)
	defer cleanupCancel()
	_, destroyed, err := sdk.RunCollectorDeploymentLifecycle(cleanupCtx, adapter, request, sdk.CollectorDestroy, parsed.marker+":destroy", applied.ResourceRefs, "")
	if err != nil {
		return fmt.Errorf("destroy ECS collector: %w", err)
	}
	if !destroyed.CleanupVerified {
		return errors.New("ECS collector destroy did not prove direct inventory convergence")
	}
	cleanupCollector = false
	fmt.Fprintf(output, "AWS ECS collector acceptance PASS region=%s cluster=%s service=%s runtime=ecs-fargate marker=%s signals=%s cleanup=verified\n", parsed.region, parsed.cluster, parsed.service, parsed.marker, strings.Join(parsed.signals, ","))
	return nil
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("aws-collector-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	parsed := options{}
	rawSignals := ""
	flags.StringVar(&parsed.region, "region", envOr("AWS_REGION", ""), "AWS region")
	flags.StringVar(&parsed.cluster, "cluster", envOr("MAGELIFT_AWS_COLLECTOR_CLUSTER", ""), "ECS cluster name")
	flags.StringVar(&parsed.service, "service", envOr("MAGELIFT_AWS_COLLECTOR_SERVICE", ""), "ECS service name")
	flags.StringVar(&parsed.secretReference, "credential-ref", envOr("MAGELIFT_AWS_COLLECTOR_CREDENTIAL_REF", ""), "opaque AWS secret reference")
	flags.StringVar(&parsed.secretARN, "secret-arn", envOr("MAGELIFT_AWS_COLLECTOR_SECRET_ARN", ""), "AWS Secrets Manager ARN for task injection")
	flags.StringVar(&parsed.endpoint, "endpoint", envOr("MAGELIFT_NEWRELIC_OTLP_ENDPOINT", ""), "New Relic OTLP endpoint")
	flags.StringVar(&parsed.nerdGraphEndpoint, "nerdgraph-endpoint", envOr("MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT", ""), "New Relic NerdGraph endpoint")
	flags.Int64Var(&parsed.accountID, "account-id", envInt64("MAGELIFT_NEWRELIC_ACCOUNT_ID"), "New Relic account ID")
	flags.StringVar(&parsed.imageDigest, "image-digest", envOr("MAGELIFT_AWS_COLLECTOR_IMAGE_DIGEST", ""), "immutable OpenTelemetry Collector Contrib image digest")
	flags.StringVar(&parsed.marker, "marker", envOr("MAGELIFT_AWS_COLLECTOR_MARKER", ""), "single-line MageLift ownership marker")
	flags.StringVar(&rawSignals, "signals", envOr("MAGELIFT_AWS_COLLECTOR_SIGNALS", defaultCollectorSignals), "comma-separated OTLP signals")
	flags.DurationVar(&parsed.timeout, "timeout", envDuration("MAGELIFT_AWS_COLLECTOR_TIMEOUT", defaultCollectorTimeout), "complete collector lifecycle timeout")
	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("unexpected positional arguments")
	}
	if parsed.marker == "" {
		parsed.marker = fmt.Sprintf("magelift/aws/collector/%s-%d", time.Now().UTC().Format("20060102-150405"), os.Getpid())
	}
	var err error
	parsed.signals, err = parseSignals(rawSignals)
	if err != nil {
		return options{}, err
	}
	parsed.queryKey = []byte(strings.TrimSpace(os.Getenv("MAGELIFT_AWS_COLLECTOR_QUERY_KEY")))
	if err := validateOptions(parsed); err != nil {
		clear(parsed.queryKey)
		return options{}, err
	}
	return parsed, nil
}

func validateOptions(parsed options) error {
	for name, value := range map[string]string{
		"region": parsed.region, "cluster": parsed.cluster, "service": parsed.service,
		"credential-ref": parsed.secretReference, "secret-arn": parsed.secretARN,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("%s is required and must be single-line", name)
		}
	}
	if err := sdk.ValidateCredentialReference(parsed.secretReference); err != nil {
		return fmt.Errorf("credential-ref: %w", err)
	}
	if !strings.HasPrefix(parsed.secretARN, "arn:aws:secretsmanager:") {
		return errors.New("secret-arn must be an AWS Secrets Manager ARN")
	}
	if err := validateHTTPSURL(parsed.endpoint, "OTLP endpoint"); err != nil {
		return err
	}
	if err := validateHTTPSURL(parsed.nerdGraphEndpoint, "NerdGraph endpoint"); err != nil {
		return err
	}
	if parsed.accountID <= 0 {
		return errors.New("account-id must be positive")
	}
	if !immutableImage(parsed.imageDigest) {
		return errors.New("image-digest must use an immutable repository@sha256 digest")
	}
	if parsed.marker == "" || strings.ContainsAny(parsed.marker, "\r\n\x00") {
		return errors.New("marker must be non-empty and single-line")
	}
	if len(parsed.signals) == 0 {
		return errors.New("signals must contain at least one of logs, metrics, or traces")
	}
	if len(parsed.queryKey) == 0 {
		return errors.New("MAGELIFT_AWS_COLLECTOR_QUERY_KEY is required")
	}
	if parsed.timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	return nil
}

func collectorRequest(parsed options) sdk.CollectorDeploymentPlanRequest {
	return sdk.CollectorDeploymentPlanRequest{
		TargetProvider:  "aws",
		TargetRuntime:   "ecs-fargate",
		Workload:        "ecs",
		Distribution:    "opentelemetry-collector-contrib",
		CredentialRef:   parsed.secretReference,
		Endpoint:        parsed.endpoint,
		NativeReference: parsed.cluster + "/" + parsed.service,
		OwnershipMarker: parsed.marker,
		Signals:         append([]string(nil), parsed.signals...),
	}
}

func parseSignals(raw string) ([]string, error) {
	seen := make(map[string]struct{})
	var signals []string
	for _, value := range strings.Split(raw, ",") {
		signal := strings.TrimSpace(value)
		if signal != "logs" && signal != "metrics" && signal != "traces" {
			return nil, fmt.Errorf("unsupported collector signal %q", signal)
		}
		if _, exists := seen[signal]; exists {
			return nil, fmt.Errorf("duplicate collector signal %q", signal)
		}
		seen[signal] = struct{}{}
		signals = append(signals, signal)
	}
	if len(signals) == 0 {
		return nil, errors.New("at least one collector signal is required")
	}
	return signals, nil
}

func immutableImage(value string) bool {
	const suffix = "@sha256:"
	index := strings.LastIndex(value, suffix)
	if index <= 0 || len(value[index+len(suffix):]) != 64 {
		return false
	}
	for _, character := range value[index+len(suffix):] {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func validateHTTPSURL(value, name string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || strings.ContainsAny(value, " \t\r\n\x00") {
		return fmt.Errorf("%s must be an HTTPS URL", name)
	}
	return nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt64(name string) int64 {
	var value int64
	if _, err := fmt.Sscan(os.Getenv(name), &value); err != nil {
		return 0
	}
	return value
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0
	}
	return parsed
}
