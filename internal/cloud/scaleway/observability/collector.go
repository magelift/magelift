package observability

import (
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/external/newrelic"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	sdk "github.com/magelift/magelift/sdk/v1"
	"k8s.io/client-go/kubernetes"
)

// NewKapsuleCollectorDeploymentAdapter binds the shared lifecycle to an
// injected Kubernetes Helm/client-go backend. OTLP is the documented default
// for Kapsule until a native New Relic collector path is separately verified.
func NewKapsuleCollectorDeploymentAdapter(backend providerobservability.CollectorBackend) (sdk.CollectorDeploymentAdapter, error) {
	return providerobservability.NewCollectorSDKAdapterWithDistribution(
		"scaleway.kapsule.collector", "scaleway", "kubernetes", func(runtime sdk.RuntimeID) bool {
			return string(runtime) == "kapsule" || strings.HasPrefix(string(runtime), "kapsule-")
		},
		func(distribution string) bool { return distribution == "opentelemetry-collector-contrib" }, backend,
	)
}

// NewKapsuleContribCollectorDeploymentAdapter wires the concrete client-go
// Contrib backend to the shared Kapsule lifecycle.
func NewKapsuleContribCollectorDeploymentAdapter(client kubernetes.Interface, config kube.KubernetesCollectorConfig) (sdk.CollectorDeploymentAdapter, error) {
	backend, err := kube.NewKubernetesCollectorBackend(client, config)
	if err != nil {
		return nil, err
	}
	return NewKapsuleCollectorDeploymentAdapter(backend)
}

// NewKapsuleNRDOTCollectorDeploymentAdapter binds the provider-owned NRDOT
// Helm translator to Kapsule without duplicating chart or lifecycle logic.
// Live chart compatibility and signal delivery remain separately evidenced.
func NewKapsuleNRDOTCollectorDeploymentAdapter(api newrelic.NRDOTHelmReleaseAPI, config newrelic.NRDOTHelmConfig) (sdk.CollectorDeploymentAdapter, error) {
	backend, err := newrelic.NewNRDOTHelmCollectorBackend(api, config)
	if err != nil {
		return nil, err
	}
	return providerobservability.NewCollectorSDKAdapterWithDistribution(
		"scaleway.kapsule.nrdot.collector", "scaleway", "kubernetes", func(runtime sdk.RuntimeID) bool {
			return string(runtime) == "kapsule" || strings.HasPrefix(string(runtime), "kapsule-")
		}, func(distribution string) bool { return distribution == newrelic.NRDOTKubernetesDistribution }, backend,
	)
}
