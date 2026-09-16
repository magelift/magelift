package observability

import (
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/external/newrelic"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
	"k8s.io/client-go/kubernetes"
)

// NewMKSCollectorDeploymentAdapter binds the shared lifecycle to an injected
// Kubernetes Helm/client-go backend. OTLP is the documented default for MKS;
// the native Logs Data Platform audit subscription remains a separate adapter.
func NewMKSCollectorDeploymentAdapter(backend providerobservability.CollectorBackend) (sdk.CollectorDeploymentAdapter, error) {
	return providerobservability.NewCollectorSDKAdapterWithDistribution(
		"ovh.mks.collector", "ovh", "kubernetes", func(runtime sdk.RuntimeID) bool {
			return string(runtime) == "mks" || strings.HasPrefix(string(runtime), "mks-")
		},
		func(distribution string) bool { return distribution == "opentelemetry-collector-contrib" }, backend,
	)
}

// NewMKSContribCollectorDeploymentAdapter wires the concrete client-go
// Contrib backend to the shared MKS lifecycle.
func NewMKSContribCollectorDeploymentAdapter(client kubernetes.Interface, config kube.KubernetesCollectorConfig) (sdk.CollectorDeploymentAdapter, error) {
	backend, err := kube.NewKubernetesCollectorBackend(client, config)
	if err != nil {
		return nil, err
	}
	return NewMKSCollectorDeploymentAdapter(backend)
}

// NewMKSNRDOTCollectorDeploymentAdapter binds the provider-owned NRDOT Helm
// translator to MKS without duplicating chart or lifecycle logic. Live chart
// compatibility and signal delivery remain separately evidenced.
func NewMKSNRDOTCollectorDeploymentAdapter(api newrelic.NRDOTHelmReleaseAPI, config newrelic.NRDOTHelmConfig) (sdk.CollectorDeploymentAdapter, error) {
	backend, err := newrelic.NewNRDOTHelmCollectorBackend(api, config)
	if err != nil {
		return nil, err
	}
	return providerobservability.NewCollectorSDKAdapterWithDistribution(
		"ovh.mks.nrdot.collector", "ovh", "kubernetes", func(runtime sdk.RuntimeID) bool {
			return string(runtime) == "mks" || strings.HasPrefix(string(runtime), "mks-")
		}, func(distribution string) bool { return distribution == newrelic.NRDOTKubernetesDistribution }, backend,
	)
}
