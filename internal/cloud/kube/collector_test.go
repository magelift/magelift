package kube

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestKubernetesCollectorBackendUsesSecretReferenceAndVerifiesSignals(t *testing.T) {
	client := fake.NewClientset()
	probe := collectorProbe{delivered: true}
	backend, err := NewKubernetesCollectorBackend(client, KubernetesCollectorConfig{
		Namespace:             "magelift",
		ImageDigest:           "ghcr.io/example/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CredentialEnvironment: "OTEL_EXPORTER_API_KEY",
		CredentialHeader:      "x-api-key",
		Probe:                 probe,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := sdk.CollectorDeploymentPlan{
		TargetProvider: "gcp", TargetRuntime: "gke", Workload: "kubernetes", Distribution: "opentelemetry-collector-contrib",
		CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", Endpoint: "https://otlp.eu01.nr-data.net",
		NativeReference: "projects/example/locations/europe-west1/clusters/magelift", OwnershipMarker: "magelift/test/kube-collector",
		Signals: []string{"logs", "metrics", "traces"},
	}
	resource, err := backend.Ensure(context.Background(), plan)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !resource.Owned || resource.Identity != "deployment:magelift/magelift-collector-"+markerDigest(plan.OwnershipMarker) {
		t.Fatalf("resource = %#v", resource)
	}
	deployment, err := client.AppsV1().Deployments("magelift").Get(context.Background(), backend.deploymentName(plan.OwnershipMarker), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := deployment.Spec.Template.Spec.Containers[0].Env[1].Value; got != "" {
		t.Fatalf("secret environment value leaked into deployment: %q", got)
	}
	secret := deployment.Spec.Template.Spec.Containers[0].Env[1].ValueFrom
	if secret == nil || secret.SecretKeyRef == nil || secret.SecretKeyRef.Name != "new-relic" || secret.SecretKeyRef.Key != "license-key" {
		t.Fatalf("secret reference = %#v", secret)
	}
	configMap, err := client.CoreV1().ConfigMaps("magelift").Get(context.Background(), backend.configMapName(plan.OwnershipMarker), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	config := configMap.Data[collectorConfigMapKey]
	for _, expected := range []string{"logs:", "metrics:", "traces:", "x-api-key: ${OTEL_EXPORTER_API_KEY}", "exporters: [otlphttp/destination]"} {
		if !strings.Contains(config, expected) {
			t.Fatalf("collector config missing %q: %s", expected, config)
		}
	}
	if strings.Count(config, "receivers:\n") != 1 || strings.Count(config, "service:\n") != 1 {
		t.Fatalf("collector config contains duplicate YAML documents: %s", config)
	}
	if strings.Contains(config, collectorDefaultCredentialEnv) || strings.Contains(config, "\n      "+collectorDefaultCredentialHeader+":") {
		t.Fatalf("collector config contains the default credential boundary: %s", config)
	}
	if got := deployment.Spec.Template.Spec.Containers[0].Env[0].Value; got != plan.Endpoint {
		t.Fatalf("collector endpoint = %q, want %q", got, plan.Endpoint)
	}
	if got := deployment.Spec.Template.Spec.Containers[0].Env[1].Name; got != "OTEL_EXPORTER_API_KEY" {
		t.Fatalf("credential environment = %q", got)
	}

	replicas := int32(1)
	deployment.Status.ReadyReplicas = replicas
	deployment.Status.AvailableReplicas = replicas
	if _, err := client.AppsV1().Deployments("magelift").UpdateStatus(context.Background(), deployment, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	verification, err := backend.Verify(context.Background(), plan, resource)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !verification.Ready || !verification.Healthy || !verification.SignalsDelivered {
		t.Fatalf("verification = %#v", verification)
	}
}

func TestKubernetesCollectorBackendWaitsForReadinessBeforeProbing(t *testing.T) {
	client := fake.NewClientset()
	probeCalls := 0
	backend, err := NewKubernetesCollectorBackend(client, KubernetesCollectorConfig{
		Namespace:   "magelift",
		ImageDigest: "ghcr.io/example/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Probe:       collectorProbe{delivered: true, calls: &probeCalls},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := sdk.CollectorDeploymentPlan{
		TargetProvider: "gcp", TargetRuntime: "gke-autopilot", Workload: "kubernetes", Distribution: "opentelemetry-collector-contrib",
		CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", Endpoint: "https://otlp.eu01.nr-data.net",
		NativeReference: "projects/example/locations/europe-west1/clusters/magelift", OwnershipMarker: "magelift/test/readiness",
		Signals: []string{"logs"},
	}
	replicas := int32(1)
	labels, annotations := collectorMetadata(plan, collectorDefaultCredentialEnv, collectorDefaultCredentialHeader)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: backend.deploymentName(plan.OwnershipMarker), Namespace: "magelift", Labels: labels, Annotations: annotations},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "magelift-collector", collectorOwnershipDigestKey: markerDigest(plan.OwnershipMarker)}},
		},
	}
	resource := backend.resourceFromDeployment(deployment, plan)
	getCalls := 0
	client.PrependReactor("get", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		getCalls++
		current := deployment.DeepCopy()
		if getCalls >= 2 {
			current.Status.ReadyReplicas = replicas
			current.Status.AvailableReplicas = replicas
		}
		return true, current, nil
	})

	verification, err := backend.Verify(context.Background(), plan, resource)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if getCalls < 2 {
		t.Fatalf("deployment get calls = %d, want readiness polling", getCalls)
	}
	if probeCalls != 1 {
		t.Fatalf("signal probe calls = %d, want one probe after readiness", probeCalls)
	}
	if !verification.Ready || !verification.Healthy || !verification.SignalsDelivered {
		t.Fatalf("verification = %#v", verification)
	}
}

func TestKubernetesCollectorBackendRefusesUnownedCollisionBeforeMutation(t *testing.T) {
	client := fake.NewClientset()
	backend, err := NewKubernetesCollectorBackend(client, KubernetesCollectorConfig{
		Namespace:   "magelift",
		ImageDigest: "ghcr.io/example/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Probe:       collectorProbe{delivered: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	marker := "magelift/test/collision"
	_, err = client.AppsV1().Deployments("magelift").Create(context.Background(), &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: backend.deploymentName(marker), Namespace: "magelift"}}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	plan := sdk.CollectorDeploymentPlan{TargetProvider: "aws", TargetRuntime: "eks", Workload: "kubernetes", Distribution: "opentelemetry-collector-contrib", CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", Endpoint: "https://otlp.nr-data.net", NativeReference: "cluster/example", OwnershipMarker: marker, Signals: []string{"logs"}}
	resource, found, err := backend.Find(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !found || resource.Owned {
		t.Fatalf("find = %#v, found %v; want unowned collision", resource, found)
	}
	if _, err := backend.Ensure(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "unowned") {
		t.Fatalf("ensure error = %v, want unowned refusal", err)
	}
	if _, err := client.CoreV1().ConfigMaps("magelift").Get(context.Background(), backend.configMapName(marker), metav1.GetOptions{}); err == nil {
		t.Fatal("Ensure created a ConfigMap after the unowned collision")
	}
}

func TestKubernetesCollectorBackendCleansNewResourcesWhenEnsureFails(t *testing.T) {
	client := fake.NewClientset()
	client.PrependReactor("create", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("injected deployment create failure")
	})
	backend, err := NewKubernetesCollectorBackend(client, KubernetesCollectorConfig{
		Namespace:   "magelift",
		ImageDigest: "ghcr.io/example/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Probe:       collectorProbe{delivered: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := sdk.CollectorDeploymentPlan{
		TargetProvider: "gcp", TargetRuntime: "gke-standard", Workload: "kubernetes", Distribution: "opentelemetry-collector-contrib",
		CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", Endpoint: "https://otlp.nr-data.net", NativeReference: "cluster/example",
		OwnershipMarker: "magelift/test/ensure-rollback", Signals: []string{"metrics"},
	}
	if _, err := backend.Ensure(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "injected deployment create failure") {
		t.Fatalf("ensure error = %v, want injected deployment failure", err)
	}
	if _, err := client.CoreV1().ConfigMaps("magelift").Get(context.Background(), backend.configMapName(plan.OwnershipMarker), metav1.GetOptions{}); err == nil {
		t.Fatal("failed Ensure left a new ConfigMap behind")
	}
	if _, err := client.CoreV1().ServiceAccounts("magelift").Get(context.Background(), backend.serviceAccountName(plan.OwnershipMarker), metav1.GetOptions{}); err == nil {
		t.Fatal("failed Ensure left a new ServiceAccount behind")
	}
	if _, err := client.AppsV1().Deployments("magelift").Get(context.Background(), backend.deploymentName(plan.OwnershipMarker), metav1.GetOptions{}); err == nil {
		t.Fatal("failed Ensure left a new Deployment behind")
	}
}

func TestKubernetesCollectorBackendInventoryAndDestroyUseDirectOwnershipChecks(t *testing.T) {
	client := fake.NewClientset()
	backend, err := NewKubernetesCollectorBackend(client, KubernetesCollectorConfig{
		Namespace:   "magelift",
		ImageDigest: "ghcr.io/example/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Probe:       collectorProbe{delivered: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := sdk.CollectorDeploymentPlan{TargetProvider: "ovh", TargetRuntime: "mks", Workload: "kubernetes", Distribution: "opentelemetry-collector-contrib", CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", Endpoint: "https://otlp.nr-data.net", NativeReference: "cluster/example", OwnershipMarker: "magelift/test/destroy", Signals: []string{"metrics"}}
	resource, err := backend.Ensure(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := backend.Inventory(context.Background(), plan.TargetProvider, plan.TargetRuntime, plan.OwnershipMarker)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 3 {
		t.Fatalf("inventory length = %d, want 3: %#v", len(resources), resources)
	}
	if err := backend.Destroy(context.Background(), plan, resource); err != nil {
		t.Fatalf("destroy: %v", err)
	}
	resources, err = backend.Inventory(context.Background(), plan.TargetProvider, plan.TargetRuntime, plan.OwnershipMarker)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 0 {
		t.Fatalf("remaining inventory = %#v", resources)
	}
}

func TestKubernetesCollectorBackendDoesNotReportAsyncCleanupAsComplete(t *testing.T) {
	client := fake.NewClientset()
	for _, resource := range []string{"deployments", "configmaps", "serviceaccounts"} {
		resource := resource
		client.PrependReactor("delete", resource, func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, nil
		})
	}
	backend, err := NewKubernetesCollectorBackend(client, KubernetesCollectorConfig{
		Namespace:   "magelift",
		ImageDigest: "ghcr.io/example/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Probe:       collectorProbe{delivered: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := sdk.CollectorDeploymentPlan{TargetProvider: "gcp", TargetRuntime: "gke-standard", Workload: "kubernetes", Distribution: "opentelemetry-collector-contrib", CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", Endpoint: "https://otlp.nr-data.net", NativeReference: "cluster/example", OwnershipMarker: "magelift/test/async-cleanup", Signals: []string{"metrics"}}
	resource, err := backend.Ensure(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := backend.Destroy(ctx, plan, resource); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("destroy error = %v, want context deadline while resources remain", err)
	}
	remaining, err := backend.Inventory(context.Background(), plan.TargetProvider, plan.TargetRuntime, plan.OwnershipMarker)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 3 {
		t.Fatalf("remaining inventory = %#v, want all terminating resources retained", remaining)
	}
}

type collectorProbe struct {
	delivered bool
	calls     *int
}

func (probe collectorProbe) VerifyCollectorSignals(_ context.Context, _ sdk.CollectorDeploymentPlan, _ providerobservability.CollectorResource) (bool, error) {
	if probe.calls != nil {
		*probe.calls++
	}
	return probe.delivered, nil
}

var _ kubernetes.Interface = (*fake.Clientset)(nil)
var _ providerobservability.CollectorBackend = (*KubernetesCollectorBackend)(nil)
