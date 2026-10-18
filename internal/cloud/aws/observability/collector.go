package observability

import (
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/external/newrelic"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
	"k8s.io/client-go/kubernetes"
)

// NewECSCollectorDeploymentAdapter binds the shared collector lifecycle to an
// injected ECS task-definition/service backend. The backend owns AWS SDK
// models; this package only fixes the supported AWS ECS boundary.
func NewECSCollectorDeploymentAdapter(backend providerobservability.CollectorBackend) (sdk.CollectorDeploymentAdapter, error) {
	return providerobservability.NewCollectorSDKAdapterWithDistribution(
		"aws.ecs.collector", "aws", "ecs", func(runtime sdk.RuntimeID) bool {
			return strings.HasPrefix(string(runtime), "ecs")
		},
		func(distribution string) bool { return distribution == "opentelemetry-collector-contrib" }, backend,
	)
}

// NewECSContribCollectorDeploymentAdapter wires the official ECS SDK port to
// the shared ECS lifecycle. It updates only the selected service's immutable
// task-definition revision and restores the previous revision on rollback.
func NewECSContribCollectorDeploymentAdapter(api ECSCollectorAPI, config ECSCollectorConfig) (sdk.CollectorDeploymentAdapter, error) {
	backend, err := NewECSCollectorBackend(api, config)
	if err != nil {
		return nil, err
	}
	return NewECSCollectorDeploymentAdapter(backend)
}

// NewECSContribCollectorDeploymentSDKAdapter constructs the provider-owned
// official SDK client without exposing AWS types through the shared port.
func NewECSContribCollectorDeploymentSDKAdapter(config awssdk.Config, collector ECSCollectorConfig) (sdk.CollectorDeploymentAdapter, error) {
	backend, err := NewECSCollectorSDKBackend(config, collector)
	if err != nil {
		return nil, err
	}
	return NewECSCollectorDeploymentAdapter(backend)
}

// NewEKSCollectorDeploymentAdapter binds the shared collector lifecycle to an
// injected Kubernetes Helm/client-go backend for EKS.
func NewEKSCollectorDeploymentAdapter(backend providerobservability.CollectorBackend) (sdk.CollectorDeploymentAdapter, error) {
	return providerobservability.NewCollectorSDKAdapterWithDistribution(
		"aws.eks.collector", "aws", "kubernetes", func(runtime sdk.RuntimeID) bool {
			return string(runtime) == "eks" || strings.HasPrefix(string(runtime), "eks-")
		},
		kubernetesCollectorDistribution, backend,
	)
}

// NewEKSContribCollectorDeploymentAdapter wires the concrete client-go
// Contrib backend to the shared EKS lifecycle. NRDOT remains an injected Helm
// backend because its chart owns distribution-specific RBAC and values.
func NewEKSContribCollectorDeploymentAdapter(client kubernetes.Interface, config kube.KubernetesCollectorConfig) (sdk.CollectorDeploymentAdapter, error) {
	backend, err := kube.NewKubernetesCollectorBackend(client, config)
	if err != nil {
		return nil, err
	}
	return NewEKSCollectorDeploymentAdapter(backend)
}

// NewEKSNRDOTCollectorDeploymentAdapter binds the documented NRDOT Helm path
// to the shared EKS lifecycle. The New Relic adapter owns chart values, RBAC,
// and Secret projection; the injected release API owns Helm SDK operations.
func NewEKSNRDOTCollectorDeploymentAdapter(api newrelic.NRDOTHelmReleaseAPI, config newrelic.NRDOTHelmConfig) (sdk.CollectorDeploymentAdapter, error) {
	backend, err := newrelic.NewNRDOTHelmCollectorBackend(api, config)
	if err != nil {
		return nil, err
	}
	return providerobservability.NewCollectorSDKAdapterWithDistribution(
		"aws.eks.nrdot.collector", "aws", "kubernetes", func(runtime sdk.RuntimeID) bool {
			return string(runtime) == "eks" || strings.HasPrefix(string(runtime), "eks-")
		}, func(distribution string) bool { return distribution == newrelic.NRDOTKubernetesDistribution }, backend,
	)
}

func kubernetesCollectorDistribution(distribution string) bool {
	return distribution == newrelic.NRDOTKubernetesDistribution || distribution == "opentelemetry-collector-contrib"
}
