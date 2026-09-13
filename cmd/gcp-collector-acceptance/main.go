package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	gcpobservability "github.com/magelift/magelift/internal/cloud/gcp/observability"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/external/newrelic"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	queryCredentialReference = "newrelic-acceptance://user-key"
	secretKey                = "license-key"
	secretOwnershipKey       = "magelift.dev/ownership-marker"
	defaultCollectorSignals  = "logs,metrics,traces"
	defaultCollectorTimeout  = 8 * time.Minute
	collectorCleanupTimeout  = 90 * time.Second
)

type options struct {
	project           string
	region            string
	cluster           string
	runtime           sdk.RuntimeID
	namespace         string
	kubeconfig        string
	endpoint          string
	nerdGraphEndpoint string
	accountID         int64
	imageDigest       string
	marker            string
	signals           []string
	timeout           time.Duration
	licenseKey        []byte
	queryKey          []byte
}

func main() {
	if err := runMain(); err != nil {
		fmt.Fprintf(os.Stderr, "GCP collector acceptance failed: %v\n", err)
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
		return errors.New("GCP collector acceptance context is required")
	}
	parsed, err := parseOptions(args)
	if err != nil {
		return err
	}
	defer clear(parsed.licenseKey)
	defer clear(parsed.queryKey)

	ctx, cancel := context.WithTimeout(parent, parsed.timeout)
	defer cancel()
	restConfig, client, err := kubernetesClient(parsed.kubeconfig)
	if err != nil {
		return err
	}
	secretName := collectorSecretName(parsed.marker)
	if err := ensureCollectorSecret(ctx, client, parsed.namespace, secretName, parsed.marker, parsed.licenseKey); err != nil {
		return err
	}
	cleanupRequired := true
	defer func() {
		if !cleanupRequired {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), collectorCleanupTimeout)
		defer cleanupCancel()
		if cleanupErr := deleteCollectorSecret(cleanupCtx, client, parsed.namespace, secretName, parsed.marker); cleanupErr != nil {
			runErr = errors.Join(runErr, cleanupErr)
		}
	}()

	resolver := provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
		if reference != queryCredentialReference {
			return errors.New("unexpected New Relic query credential reference")
		}
		return consume(parsed.queryKey)
	})
	queryer, err := newrelic.NewNRQLClientWithQueryCredentialAndRetry(
		&http.Client{Timeout: 30 * time.Second}, resolver, parsed.nerdGraphEndpoint, parsed.accountID, queryCredentialReference,
		newrelic.RetryPolicy{MaxAttempts: 10, InitialDelay: 5 * time.Second, MaxDelay: 15 * time.Second},
	)
	if err != nil {
		return fmt.Errorf("construct New Relic query verifier: %w", err)
	}
	probe, err := newrelic.NewKubernetesCollectorSignalProbe(newrelic.KubernetesCollectorSignalProbeConfig{
		Client: client, RESTConfig: restConfig, Query: queryer, QueryCredentialRef: queryCredentialReference,
	})
	if err != nil {
		return fmt.Errorf("construct Kubernetes collector signal probe: %w", err)
	}
	adapter, err := gcpobservability.NewGKEContribCollectorDeploymentAdapter(client, kubeCollectorConfig(parsed, probe))
	if err != nil {
		return fmt.Errorf("construct GKE collector adapter: %w", err)
	}
	request := collectorRequest(parsed, secretName)
	cleanupCollector := true
	defer func() {
		if !cleanupCollector {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), collectorCleanupTimeout)
		defer cleanupCancel()
		_, result, cleanupErr := sdk.RunCollectorDeploymentLifecycle(
			cleanupCtx, adapter, request, sdk.CollectorDestroy, parsed.marker+":destroy", nil, "",
		)
		if cleanupErr != nil {
			runErr = errors.Join(runErr, cleanupErr)
			return
		}
		if !result.CleanupVerified {
			runErr = errors.Join(runErr, errors.New("collector cleanup did not prove direct inventory convergence"))
		}
	}()

	_, applied, err := sdk.RunCollectorDeploymentLifecycle(ctx, adapter, request, sdk.CollectorApply, parsed.marker+":apply", nil, "")
	if err != nil {
		return fmt.Errorf("apply GKE collector: %w", err)
	}
	if !applied.ReadyVerified || !applied.HealthVerified || !applied.OwnershipVerified {
		return errors.New("GKE collector apply returned incomplete readiness, health, or ownership proof")
	}
	if _, verified, err := sdk.RunCollectorDeploymentLifecycle(ctx, adapter, request, sdk.CollectorVerify, parsed.marker+":verify", applied.ResourceRefs, ""); err != nil {
		return fmt.Errorf("verify GKE collector: %w", err)
	} else if !verified.ReadyVerified || !verified.HealthVerified {
		return errors.New("GKE collector verify returned incomplete readiness or health proof")
	}

	cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(context.Background()), collectorCleanupTimeout)
	defer cleanupCancel()
	_, destroyed, err := sdk.RunCollectorDeploymentLifecycle(cleanupCtx, adapter, request, sdk.CollectorDestroy, parsed.marker+":destroy", applied.ResourceRefs, "")
	if err != nil {
		return fmt.Errorf("destroy GKE collector: %w", err)
	}
	if !destroyed.CleanupVerified {
		return errors.New("GKE collector destroy did not prove direct inventory convergence")
	}
	cleanupCollector = false
	if err := deleteCollectorSecret(cleanupCtx, client, parsed.namespace, secretName, parsed.marker); err != nil {
		return err
	}
	cleanupRequired = false
	fmt.Fprintf(output, "GCP collector acceptance PASS project=%s cluster=%s runtime=%s marker=%s signals=%s cleanup=verified\n", parsed.project, parsed.cluster, parsed.runtime, parsed.marker, strings.Join(parsed.signals, ","))
	return nil
}

func parseOptions(args []string) (options, error) {
	flags := flag.NewFlagSet("gcp-collector-acceptance", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	parsed := options{}
	var runtimeName string
	var rawSignals string
	flags.StringVar(&parsed.project, "project", envOr("GCP_PROJECT", ""), "GCP project ID")
	flags.StringVar(&parsed.region, "region", envOr("GCP_REGION", ""), "GCP cluster region")
	flags.StringVar(&parsed.cluster, "cluster", envOr("MAGELIFT_GCP_COLLECTOR_CLUSTER", ""), "GKE cluster name")
	flags.StringVar(&runtimeName, "runtime", envOr("MAGELIFT_GCP_COLLECTOR_RUNTIME", "gke-autopilot"), "GKE runtime identity")
	flags.StringVar(&parsed.namespace, "namespace", envOr("MAGELIFT_GCP_COLLECTOR_NAMESPACE", "default"), "Kubernetes namespace for the collector")
	flags.StringVar(&parsed.kubeconfig, "kubeconfig", envOr("KUBECONFIG", ""), "Kubernetes client configuration path")
	flags.StringVar(&parsed.endpoint, "endpoint", envOr("MAGELIFT_NEWRELIC_OTLP_ENDPOINT", ""), "New Relic OTLP endpoint")
	flags.StringVar(&parsed.nerdGraphEndpoint, "nerdgraph-endpoint", envOr("MAGELIFT_NEWRELIC_NERDGRAPH_ENDPOINT", ""), "New Relic NerdGraph endpoint")
	flags.Int64Var(&parsed.accountID, "account-id", envInt64("MAGELIFT_NEWRELIC_ACCOUNT_ID"), "New Relic account ID")
	flags.StringVar(&parsed.imageDigest, "image-digest", envOr("MAGELIFT_GCP_COLLECTOR_IMAGE_DIGEST", ""), "immutable OpenTelemetry Collector Contrib image digest")
	flags.StringVar(&parsed.marker, "marker", envOr("MAGELIFT_GCP_COLLECTOR_MARKER", ""), "single-line MageLift ownership marker")
	flags.StringVar(&rawSignals, "signals", envOr("MAGELIFT_GCP_COLLECTOR_SIGNALS", defaultCollectorSignals), "comma-separated OTLP signals")
	flags.DurationVar(&parsed.timeout, "timeout", envDuration("MAGELIFT_GCP_COLLECTOR_TIMEOUT", defaultCollectorTimeout), "complete collector lifecycle timeout")
	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}
	if flags.NArg() != 0 {
		return options{}, errors.New("unexpected positional arguments")
	}
	parsed.runtime = sdk.RuntimeID(runtimeName)
	if parsed.marker == "" {
		parsed.marker = fmt.Sprintf("magelift/gcp/collector/%s-%d", time.Now().UTC().Format("20060102-150405"), os.Getpid())
	}
	var err error
	parsed.signals, err = parseSignals(rawSignals)
	if err != nil {
		return options{}, err
	}
	parsed.licenseKey = []byte(strings.TrimSpace(os.Getenv("MAGELIFT_GCP_COLLECTOR_LICENSE_KEY")))
	parsed.queryKey = []byte(strings.TrimSpace(os.Getenv("MAGELIFT_GCP_COLLECTOR_QUERY_KEY")))
	if err := validateOptions(parsed); err != nil {
		clear(parsed.licenseKey)
		clear(parsed.queryKey)
		return options{}, err
	}
	return parsed, nil
}

func validateOptions(parsed options) error {
	if !validGCPProjectID(parsed.project) {
		return errors.New("project must be a valid GCP project ID")
	}
	if !validRegion(parsed.region) {
		return errors.New("region must be a valid GCP region")
	}
	if parsed.cluster == "" || len(parsed.cluster) > 40 || len(validation.IsDNS1123Subdomain(parsed.cluster)) > 0 {
		return errors.New("cluster must be a valid GKE cluster name of at most 40 characters")
	}
	if parsed.runtime != "gke-autopilot" && parsed.runtime != "gke-standard" {
		return fmt.Errorf("runtime %q is not a supported GKE runtime", parsed.runtime)
	}
	if parsed.namespace == "" || len(validation.IsDNS1123Subdomain(parsed.namespace)) > 0 {
		return errors.New("namespace must be a valid Kubernetes namespace")
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
	if len(parsed.licenseKey) == 0 || len(parsed.queryKey) == 0 {
		return errors.New("MAGELIFT_GCP_COLLECTOR_LICENSE_KEY and MAGELIFT_GCP_COLLECTOR_QUERY_KEY are required")
	}
	if parsed.timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	return nil
}

func collectorRequest(parsed options, secretName string) sdk.CollectorDeploymentPlanRequest {
	return sdk.CollectorDeploymentPlanRequest{
		TargetProvider:  "gcp",
		TargetRuntime:   parsed.runtime,
		Workload:        "kubernetes",
		Distribution:    "opentelemetry-collector-contrib",
		CredentialRef:   fmt.Sprintf("kubernetes-secret://%s/%s#%s", parsed.namespace, secretName, secretKey),
		Endpoint:        parsed.endpoint,
		NativeReference: fmt.Sprintf("projects/%s/locations/%s/clusters/%s", parsed.project, parsed.region, parsed.cluster),
		OwnershipMarker: parsed.marker,
		Signals:         append([]string(nil), parsed.signals...),
	}
}

func kubeCollectorConfig(parsed options, probe *newrelic.KubernetesCollectorSignalProbe) kube.KubernetesCollectorConfig {
	return kube.KubernetesCollectorConfig{Namespace: parsed.namespace, ImageDigest: parsed.imageDigest, Probe: probe}
}

func kubernetesClient(kubeconfig string) (*rest.Config, kubernetes.Interface, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if strings.TrimSpace(kubeconfig) != "" {
		loadingRules.ExplicitPath = kubeconfig
	}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("load Kubernetes client configuration: %w", err)
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("construct Kubernetes client: %w", err)
	}
	return restConfig, client, nil
}

func ensureCollectorSecret(ctx context.Context, client kubernetes.Interface, namespace, name, marker string, licenseKey []byte) error {
	if ctx == nil || client == nil {
		return errors.New("collector Secret setup requires context and Kubernetes client")
	}
	labels := map[string]string{"app.kubernetes.io/managed-by": "magelift", "magelift.dev/component": "newrelic-collector"}
	annotations := map[string]string{secretOwnershipKey: marker}
	data := append([]byte(nil), licenseKey...)
	defer clear(data)
	desired := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels, Annotations: annotations}, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{secretKey: data}}
	existing, err := client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := client.CoreV1().Secrets(namespace).Create(ctx, desired, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create marker-owned collector Secret: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect collector Secret: %w", err)
	}
	if existing.Annotations[secretOwnershipKey] != marker {
		return errors.New("refusing to replace an unowned collector Secret")
	}
	desired.ResourceVersion = existing.ResourceVersion
	if _, err := client.CoreV1().Secrets(namespace).Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update marker-owned collector Secret: %w", err)
	}
	return nil
}

func deleteCollectorSecret(ctx context.Context, client kubernetes.Interface, namespace, name, marker string) error {
	secret, err := client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inventory collector Secret before cleanup: %w", err)
	}
	if secret.Annotations[secretOwnershipKey] != marker {
		return errors.New("refusing to delete an unowned collector Secret")
	}
	if err := client.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete collector Secret: %w", err)
	}
	deadline := time.NewTimer(collectorCleanupTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		secret, err := client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("verify collector Secret cleanup: %w", err)
		}
		if secret.Annotations[secretOwnershipKey] != marker {
			return errors.New("collector Secret ownership changed during cleanup")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("collector Secret cleanup did not converge")
		case <-ticker.C:
		}
	}
}

func collectorSecretName(marker string) string {
	digest := sha256.Sum256([]byte(marker))
	return "magelift-nr-" + hex.EncodeToString(digest[:])[:16]
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

func validateHTTPSURL(raw, name string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an HTTPS URL without credentials, query, or fragment", name)
	}
	return nil
}

func immutableImage(raw string) bool {
	separator := strings.LastIndex(raw, "@sha256:")
	if separator <= 0 || len(raw[separator+len("@sha256:"):]) != 64 {
		return false
	}
	return strings.Trim(raw[:separator], " \t\r\n\x00") == raw[:separator] && strings.Trim(raw[separator+len("@sha256:"):], "0123456789abcdef") == ""
}

func validGCPProjectID(value string) bool {
	if len(value) < 6 || len(value) > 30 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
			if index == len(value)-1 && character == '-' {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func validRegion(value string) bool {
	if value == "" || strings.ContainsAny(value, " \t\r\n\x00") || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
			continue
		}
		return false
	}
	return true
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envInt64(name string) int64 {
	value, _ := strconv.ParseInt(strings.TrimSpace(os.Getenv(name)), 10, 64)
	return value
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name)))
	if err != nil {
		return fallback
	}
	return value
}
