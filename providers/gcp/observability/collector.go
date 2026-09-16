package observability

import (
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/providers/gcp/nrdot"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
	"k8s.io/client-go/kubernetes"
)

// NewGKECollectorDeploymentAdapter binds the shared lifecycle to an injected
// Kubernetes Helm/client-go backend for GKE Autopilot or Standard.
func NewGKECollectorDeploymentAdapter(backend providerobservability.CollectorBackend) (sdk.CollectorDeploymentAdapter, error) {
	return providerobservability.NewCollectorSDKAdapterWithDistribution(
		"gcp.gke.collector", "gcp", "kubernetes", func(runtime sdk.RuntimeID) bool {
			return string(runtime) == "gke" || strings.HasPrefix(string(runtime), "gke-")
		},
		func(distribution string) bool {
			return distribution == nrdot.NRDOTKubernetesDistribution || distribution == "opentelemetry-collector-contrib"
		}, backend,
	)
}

// NewGKEContribCollectorDeploymentAdapter wires the concrete client-go
// Contrib backend to the shared GKE lifecycle. NRDOT remains an injected Helm
// backend because its chart owns distribution-specific RBAC and values.
func NewGKEContribCollectorDeploymentAdapter(client kubernetes.Interface, config kube.KubernetesCollectorConfig) (sdk.CollectorDeploymentAdapter, error) {
	backend, err := kube.NewKubernetesCollectorBackend(client, config)
	if err != nil {
		return nil, err
	}
	return NewGKECollectorDeploymentAdapter(backend)
}

// NewGKENRDOTCollectorDeploymentAdapter binds the documented NRDOT Helm path
// to the shared GKE lifecycle. The New Relic adapter owns chart values, RBAC,
// and Secret projection; the injected release API owns Helm SDK operations.
func NewGKENRDOTCollectorDeploymentAdapter(api nrdot.NRDOTHelmReleaseAPI, config nrdot.NRDOTHelmConfig) (sdk.CollectorDeploymentAdapter, error) {
	backend, err := nrdot.NewNRDOTHelmCollectorBackend(api, config)
	if err != nil {
		return nil, err
	}
	return providerobservability.NewCollectorSDKAdapterWithDistribution(
		"gcp.gke.nrdot.collector", "gcp", "kubernetes", func(runtime sdk.RuntimeID) bool {
			return string(runtime) == "gke" || strings.HasPrefix(string(runtime), "gke-")
		}, func(distribution string) bool { return distribution == nrdot.NRDOTKubernetesDistribution }, backend,
	)
}
