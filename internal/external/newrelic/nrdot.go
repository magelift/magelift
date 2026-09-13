package newrelic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	sdk "github.com/magelift/magelift/sdk/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

// NRDOTKubernetesChartReference is the chart documented by New Relic for
// Kubernetes monitoring. The similarly named collector image is not a Helm
// chart and must not be accepted as the chart identity.
const NRDOTKubernetesChartReference = "newrelic/nr-k8s-otel-collector"

// NRDOTKubernetesDistribution is the stable provider-owned distribution
// identity carried through the shared collector lifecycle.
const NRDOTKubernetesDistribution = NRDOTKubernetesChartReference

// NRDOTHelmConfig contains the deployment policy that is specific to the
// New Relic Kubernetes chart. The shared Helm lifecycle still owns
// idempotency, ownership, drift, readiness, rollback, and cleanup.
type NRDOTHelmConfig struct {
	Namespace          string
	ReleaseName        string
	ChartVersion       string
	ResolveClusterName func(context.Context, string) (string, error)
	SignalProbe        kube.CollectorSignalProbe
}

// NRDOTKubernetesValues is the secret-free subset of the documented chart
// values that MageLift controls. It is intentionally provider-owned: the
// portable collector contract does not know Helm values, RBAC, or chart
// templates.
type NRDOTKubernetesValues struct {
	Cluster                string                  `yaml:"cluster"`
	CustomSecretName       string                  `yaml:"customSecretName"`
	CustomSecretLicenseKey string                  `yaml:"customSecretLicenseKey"`
	Provider               string                  `yaml:"provider,omitempty"`
	Images                 NRDOTImages             `yaml:"images"`
	Labels                 map[string]string       `yaml:"labels"`
	PodLabels              map[string]string       `yaml:"podLabels"`
	DaemonSet              NRDOTCollectorComponent `yaml:"daemonset"`
	Deployment             NRDOTCollectorComponent `yaml:"deployment"`
	RBAC                   NRDOTRBAC               `yaml:"rbac"`
	ServiceAccount         NRDOTServiceAccount     `yaml:"serviceAccount"`
	Receivers              NRDOTReceivers          `yaml:"receivers"`
}

type NRDOTImages struct {
	Collector NRDOTImage `yaml:"collector"`
}

type NRDOTImage struct {
	Repository string `yaml:"repository"`
}

type NRDOTCollectorComponent struct {
	ExtraArgs []string `yaml:"extraArgs"`
}

type NRDOTRBAC struct {
	Create bool `yaml:"create"`
}

type NRDOTServiceAccount struct {
	Create bool `yaml:"create"`
}

type NRDOTReceivers struct {
	Prometheus NRDOTReceiver `yaml:"prometheus"`
	K8sEvents  NRDOTReceiver `yaml:"k8sEvents"`
	Filelog    NRDOTReceiver `yaml:"filelog"`
}

type NRDOTReceiver struct {
	Enabled bool `yaml:"enabled"`
}

// NRDOTHelmReleaseAPI is the provider-owned Helm SDK port. Its installer
// receives typed chart values, while the shared kube package sees only the
// normalized release lifecycle interface.
type NRDOTHelmReleaseAPI interface {
	GetRelease(context.Context, string, string) (kube.HelmCollectorRelease, bool, error)
	InstallNRDOTRelease(context.Context, kube.HelmCollectorReleaseRequest, NRDOTKubernetesValues) (kube.HelmCollectorRelease, error)
	UninstallRelease(context.Context, string, string) error
}

// NRDOTHelmCollectorAPI translates the semantic Helm request into the
// documented New Relic chart values without moving a secret value across the
// shared collector boundary.
type NRDOTHelmCollectorAPI struct {
	api                NRDOTHelmReleaseAPI
	resolveClusterName func(context.Context, string) (string, error)
}

var _ kube.HelmCollectorAPI = (*NRDOTHelmCollectorAPI)(nil)

func NewNRDOTHelmCollectorAPI(api NRDOTHelmReleaseAPI, resolveClusterName func(context.Context, string) (string, error)) (*NRDOTHelmCollectorAPI, error) {
	if api == nil {
		return nil, errors.New("New Relic NRDOT Helm release API is required")
	}
	if resolveClusterName == nil {
		return nil, errors.New("New Relic NRDOT cluster-name resolver is required")
	}
	return &NRDOTHelmCollectorAPI{api: api, resolveClusterName: resolveClusterName}, nil
}

// NewNRDOTHelmCollectorBackend creates the shared Helm lifecycle backend with
// New Relic's provider-owned values translator. Cloud-provider packages only
// bind this backend to their target identity; chart values and lifecycle
// semantics remain in this one New Relic implementation.
func NewNRDOTHelmCollectorBackend(api NRDOTHelmReleaseAPI, config NRDOTHelmConfig) (*kube.HelmCollectorBackend, error) {
	nrdotAPI, err := NewNRDOTHelmCollectorAPI(api, config.ResolveClusterName)
	if err != nil {
		return nil, err
	}
	return kube.NewHelmCollectorBackend(kube.HelmCollectorConfig{
		Namespace: config.Namespace, ReleaseName: config.ReleaseName, ChartReference: NRDOTKubernetesChartReference,
		ChartVersion: config.ChartVersion, API: nrdotAPI, Probe: config.SignalProbe,
	})
}

func (api *NRDOTHelmCollectorAPI) GetRelease(ctx context.Context, namespace, name string) (kube.HelmCollectorRelease, bool, error) {
	if api == nil || api.api == nil {
		return kube.HelmCollectorRelease{}, false, errors.New("New Relic NRDOT Helm release API is required")
	}
	if err := validateNRDOTReleaseIdentity(namespace, name); err != nil {
		return kube.HelmCollectorRelease{}, false, err
	}
	return api.api.GetRelease(ctx, namespace, name)
}

func (api *NRDOTHelmCollectorAPI) InstallRelease(ctx context.Context, request kube.HelmCollectorReleaseRequest) (kube.HelmCollectorRelease, error) {
	if api == nil || api.api == nil {
		return kube.HelmCollectorRelease{}, errors.New("New Relic NRDOT Helm release API is required")
	}
	if ctx == nil {
		return kube.HelmCollectorRelease{}, errors.New("New Relic NRDOT Helm context is required")
	}
	if err := ctx.Err(); err != nil {
		return kube.HelmCollectorRelease{}, err
	}
	if err := validateNRDOTHelmRequest(request); err != nil {
		return kube.HelmCollectorRelease{}, err
	}
	if err := validateNRDOTReleaseIdentity(request.Namespace, request.Name); err != nil {
		return kube.HelmCollectorRelease{}, err
	}
	clusterName, err := api.resolveClusterName(ctx, request.NativeReference)
	if err != nil {
		return kube.HelmCollectorRelease{}, fmt.Errorf("resolve New Relic NRDOT cluster name: %w", err)
	}
	values, err := BuildNRDOTKubernetesValues(request, clusterName)
	if err != nil {
		return kube.HelmCollectorRelease{}, err
	}
	return api.api.InstallNRDOTRelease(ctx, request, values)
}

func (api *NRDOTHelmCollectorAPI) UninstallRelease(ctx context.Context, namespace, name string) error {
	if api == nil || api.api == nil {
		return errors.New("New Relic NRDOT Helm release API is required")
	}
	if err := validateNRDOTReleaseIdentity(namespace, name); err != nil {
		return err
	}
	return api.api.UninstallRelease(ctx, namespace, name)
}

// BuildNRDOTKubernetesValues maps only values documented by New Relic's
// nr-k8s-otel-collector chart. The license value is deliberately impossible
// to represent: the chart receives a same-namespace Secret name and key.
func BuildNRDOTKubernetesValues(request kube.HelmCollectorReleaseRequest, clusterName string) (NRDOTKubernetesValues, error) {
	if err := validateNRDOTHelmRequest(request); err != nil {
		return NRDOTKubernetesValues{}, err
	}
	clusterName = strings.TrimSpace(clusterName)
	if clusterName == "" || strings.ContainsAny(clusterName, "\r\n\x00") {
		return NRDOTKubernetesValues{}, errors.New("New Relic NRDOT Kubernetes cluster name is required and must be single-line")
	}
	secret, err := parseNRDOTSecretReference(request.CredentialRef, request.Namespace)
	if err != nil {
		return NRDOTKubernetesValues{}, err
	}
	if err := validateNRDOTEndpoint(request.Endpoint); err != nil {
		return NRDOTKubernetesValues{}, err
	}

	values := NRDOTKubernetesValues{
		Cluster:                clusterName,
		CustomSecretName:       secret.Name,
		CustomSecretLicenseKey: secret.Key,
		Images:                 NRDOTImages{Collector: NRDOTImage{Repository: "newrelic/nrdot-collector"}},
		Labels:                 nrdotOwnershipLabels(request),
		PodLabels:              nrdotOwnershipLabels(request),
		DaemonSet:              NRDOTCollectorComponent{ExtraArgs: []string{nrDotEndpointArg(request.Endpoint)}},
		Deployment:             NRDOTCollectorComponent{ExtraArgs: []string{nrDotEndpointArg(request.Endpoint)}},
		RBAC:                   NRDOTRBAC{Create: true},
		ServiceAccount:         NRDOTServiceAccount{Create: true},
		Receivers:              NRDOTReceivers{},
	}
	for _, signal := range request.Signals {
		switch signal {
		case "logs":
			values.Receivers.Filelog.Enabled = true
		case "metrics":
			values.Receivers.Prometheus.Enabled = true
		case "events":
			values.Receivers.K8sEvents.Enabled = true
		default:
			return NRDOTKubernetesValues{}, fmt.Errorf("New Relic NRDOT Kubernetes chart does not document collector signal %q", signal)
		}
	}
	if len(request.Signals) == 0 {
		return NRDOTKubernetesValues{}, errors.New("New Relic NRDOT Kubernetes chart requires at least one signal")
	}
	if request.TargetProvider == "gcp" && request.TargetRuntime == "gke-autopilot" {
		values.Provider = "GKE_AUTOPILOT"
	}
	return values, nil
}

type nrdotSecretReference struct {
	Name string
	Key  string
}

func parseNRDOTSecretReference(raw, namespace string) (nrdotSecretReference, error) {
	if err := sdk.ValidateCredentialReference(raw); err != nil {
		return nrdotSecretReference{}, fmt.Errorf("New Relic NRDOT credential reference: %w", err)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "kubernetes-secret" || parsed.Host == "" || parsed.Path == "" || parsed.RawQuery != "" || parsed.Fragment == "" {
		return nrdotSecretReference{}, errors.New("New Relic NRDOT credential reference must be kubernetes-secret://namespace/name#key")
	}
	name := strings.TrimPrefix(parsed.Path, "/")
	if parsed.Host != namespace || strings.Contains(name, "/") || len(validation.IsDNS1123Subdomain(parsed.Host)) > 0 || len(validation.IsDNS1123Subdomain(name)) > 0 || len(validation.IsConfigMapKey(parsed.Fragment)) > 0 {
		return nrdotSecretReference{}, errors.New("New Relic NRDOT credential reference must select a valid Secret in the release namespace")
	}
	return nrdotSecretReference{Name: name, Key: parsed.Fragment}, nil
}

func validateNRDOTEndpoint(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || strings.ContainsAny(raw, " \t\r\n\x00") {
		return errors.New("New Relic NRDOT endpoint must be an HTTPS URL without credentials or a fragment")
	}
	return nil
}

func nrDotEndpointArg(endpoint string) string {
	return "--config=yaml:exporters::otlp_http/newrelic::endpoint: " + strings.TrimSpace(endpoint)
}

func isImmutableChartVersion(version string) bool {
	version = strings.TrimSpace(version)
	return version != "" && !strings.EqualFold(version, "latest") && !strings.ContainsAny(version, " \t\r\n\x00")
}

func nrdotOwnershipLabels(request kube.HelmCollectorReleaseRequest) map[string]string {
	digest := sha256.Sum256([]byte(request.OwnershipMarker))
	return map[string]string{
		"magelift.dev/managed-by":       "magelift",
		"magelift.dev/ownership-digest": hex.EncodeToString(digest[:16]),
		"magelift.dev/target-provider":  string(request.TargetProvider),
		"magelift.dev/target-runtime":   string(request.TargetRuntime),
	}
}

func isNRDOTKubernetesRuntime(provider sdk.ProviderID, runtime sdk.RuntimeID) bool {
	return isNRDOTKubernetesTarget(string(provider), string(runtime))
}

func validateNRDOTHelmRequest(request kube.HelmCollectorReleaseRequest) error {
	if request.ChartReference != NRDOTKubernetesChartReference {
		return fmt.Errorf("New Relic NRDOT chart reference must be %q", NRDOTKubernetesChartReference)
	}
	if request.Distribution != NRDOTKubernetesDistribution {
		return fmt.Errorf("New Relic NRDOT distribution must be %q", NRDOTKubernetesDistribution)
	}
	if !isImmutableChartVersion(request.ChartVersion) {
		return errors.New("New Relic NRDOT chart version must be immutable and single-line")
	}
	if err := validateNewRelicTarget(string(request.TargetProvider), string(request.TargetRuntime)); err != nil {
		return err
	}
	if !isNRDOTKubernetesRuntime(request.TargetProvider, request.TargetRuntime) {
		return fmt.Errorf("New Relic NRDOT Kubernetes runtime %q is unsupported for provider %q", request.TargetRuntime, request.TargetProvider)
	}
	if strings.TrimSpace(request.NativeReference) == "" || strings.ContainsAny(request.NativeReference, "\r\n\x00") {
		return errors.New("New Relic NRDOT native reference is required and must be single-line")
	}
	if err := validateOwnershipMarker(request.OwnershipMarker); err != nil {
		return fmt.Errorf("New Relic NRDOT ownership marker: %w", err)
	}
	if _, err := parseNRDOTSecretReference(request.CredentialRef, request.Namespace); err != nil {
		return err
	}
	if err := validateNRDOTEndpoint(request.Endpoint); err != nil {
		return err
	}
	if len(request.Signals) == 0 {
		return errors.New("New Relic NRDOT Kubernetes chart requires at least one signal")
	}
	for _, signal := range request.Signals {
		switch signal {
		case "logs", "metrics", "events":
		default:
			return fmt.Errorf("New Relic NRDOT Kubernetes chart does not document collector signal %q", signal)
		}
	}
	return validateNRDOTReleaseNamespace(request.Namespace)
}

func validateNRDOTReleaseIdentity(namespace, name string) error {
	if err := validateNRDOTReleaseNamespace(namespace); err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" || len(validation.IsDNS1123Subdomain(name)) > 0 || strings.ContainsAny(name, "/\r\n\x00") {
		return errors.New("New Relic NRDOT Helm release name must be a valid DNS name")
	}
	return nil
}

func validateNRDOTReleaseNamespace(namespace string) error {
	if strings.TrimSpace(namespace) == "" || len(validation.IsDNS1123Subdomain(namespace)) > 0 || strings.ContainsAny(namespace, "/\r\n\x00") {
		return errors.New("New Relic NRDOT Helm namespace must be a valid DNS name")
	}
	return nil
}
