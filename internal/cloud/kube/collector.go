package kube

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
)

const (
	collectorNamePrefix              = "magelift-collector-"
	collectorManagedBy               = "magelift"
	collectorOwnershipDigestKey      = "magelift.dev/ownership-digest"
	collectorOwnershipMarkerKey      = "magelift.dev/ownership-marker"
	collectorProviderKey             = "magelift.dev/target-provider"
	collectorRuntimeKey              = "magelift.dev/target-runtime"
	collectorWorkloadKey             = "magelift.dev/workload"
	collectorDistributionKey         = "magelift.dev/distribution"
	collectorConfigHashKey           = "magelift.dev/config-hash"
	collectorConfigMapKey            = "collector.yaml"
	collectorConfigMountPath         = "/etc/otel"
	collectorConfigFileName          = "collector.yaml"
	collectorSecretScheme            = "kubernetes-secret"
	collectorDefaultCredentialEnv    = "NEW_RELIC_LICENSE_KEY"
	collectorDefaultCredentialHeader = "api-key"
	collectorOTLPEndpointEnv         = "OTEL_EXPORTER_OTLP_ENDPOINT"
	collectorConfigArgument          = "--config=/etc/otel/collector.yaml"
	collectorGRPCPort                = int32(4317)
	collectorHTTPPort                = int32(4318)
	collectorCleanupTimeout          = 90 * time.Second
	collectorCleanupPollInterval     = 2 * time.Second
	collectorReadinessTimeout        = 5 * time.Minute
	collectorReadinessPollInterval   = 2 * time.Second
)

// CollectorSignalProbe proves delivery at the destination. Kubernetes API
// readiness alone is not telemetry delivery evidence, so production callers
// must inject a probe that can observe the selected New Relic or OTLP path.
type CollectorSignalProbe interface {
	VerifyCollectorSignals(context.Context, sdk.CollectorDeploymentPlan, providerobservability.CollectorResource) (bool, error)
}

// KubernetesCollectorConfig contains only deployment policy and an immutable
// image identity. Secret values and provider SDK clients never enter this
// configuration.
type KubernetesCollectorConfig struct {
	Namespace             string
	ImageDigest           string
	CredentialEnvironment string
	CredentialHeader      string
	Probe                 CollectorSignalProbe
}

// KubernetesCollectorBackend manages a dedicated collector Deployment and its
// ConfigMap/ServiceAccount. It is reusable by EKS, GKE, Kapsule, MKS, and
// community Kubernetes providers; provider-specific Helm or cloud-secret
// implementations can satisfy the same CollectorBackend port instead.
type KubernetesCollectorBackend struct {
	client                kubernetes.Interface
	namespace             string
	image                 string
	credentialEnvironment string
	credentialHeader      string
	probe                 CollectorSignalProbe
}

var _ providerobservability.CollectorBackend = (*KubernetesCollectorBackend)(nil)

// NewKubernetesCollectorBackend constructs the client-go implementation for
// the documented Contrib collector path. NRDOT/Helm callers should inject a
// provider-owned Helm backend because its chart-specific RBAC and values are
// not portable Kubernetes Deployment semantics.
func NewKubernetesCollectorBackend(client kubernetes.Interface, config KubernetesCollectorConfig) (*KubernetesCollectorBackend, error) {
	if client == nil {
		return nil, errors.New("Kubernetes collector client is required")
	}
	namespace := strings.TrimSpace(config.Namespace)
	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}
	if errs := validation.IsDNS1123Subdomain(namespace); len(errs) > 0 {
		return nil, fmt.Errorf("Kubernetes collector namespace is invalid: %s", strings.Join(errs, "; "))
	}
	image := strings.TrimSpace(config.ImageDigest)
	if !isImmutableImage(image) {
		return nil, errors.New("Kubernetes collector image must use an immutable repository@sha256 digest")
	}
	if config.Probe == nil {
		return nil, errors.New("Kubernetes collector signal probe is required")
	}
	credentialEnvironment := strings.TrimSpace(config.CredentialEnvironment)
	if credentialEnvironment == "" {
		credentialEnvironment = collectorDefaultCredentialEnv
	}
	if !validEnvironmentName(credentialEnvironment) {
		return nil, errors.New("Kubernetes collector credential environment must be a valid environment variable name")
	}
	credentialHeader := strings.TrimSpace(config.CredentialHeader)
	if credentialHeader == "" {
		credentialHeader = collectorDefaultCredentialHeader
	}
	if !validHeaderName(credentialHeader) {
		return nil, errors.New("Kubernetes collector credential header must be a single header name")
	}
	return &KubernetesCollectorBackend{client: client, namespace: namespace, image: image, credentialEnvironment: credentialEnvironment, credentialHeader: credentialHeader, probe: config.Probe}, nil
}

func (backend *KubernetesCollectorBackend) Find(ctx context.Context, plan sdk.CollectorDeploymentPlan) (providerobservability.CollectorResource, bool, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.CollectorResource{}, false, err
	}
	resources, err := backend.inventory(ctx, plan.TargetProvider, plan.TargetRuntime, plan.OwnershipMarker)
	if err != nil {
		return providerobservability.CollectorResource{}, false, err
	}
	if len(resources) == 0 {
		return providerobservability.CollectorResource{}, false, nil
	}
	for _, resource := range resources {
		if resource.Identity != deploymentIdentity(backend.namespace, backend.deploymentName(plan.OwnershipMarker)) {
			continue
		}
		return resource, true, nil
	}
	return resources[0], true, nil
}

func (backend *KubernetesCollectorBackend) Ensure(ctx context.Context, plan sdk.CollectorDeploymentPlan) (providerobservability.CollectorResource, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.CollectorResource{}, err
	}
	existing, err := backend.inventory(ctx, plan.TargetProvider, plan.TargetRuntime, plan.OwnershipMarker)
	if err != nil {
		return providerobservability.CollectorResource{}, fmt.Errorf("inspect existing Kubernetes collector resources: %w", err)
	}
	if err := validateExistingResources(existing, plan); err != nil {
		return providerobservability.CollectorResource{}, err
	}
	existingIdentities := make(map[string]struct{}, len(existing))
	for _, resource := range existing {
		existingIdentities[resource.Identity] = struct{}{}
	}
	created := make([]collectorResourceRef, 0, 3)
	cleanupOnFailure := func(cause error) error {
		var cleanupErrs []error
		for index := len(created) - 1; index >= 0; index-- {
			resource := created[index]
			if err := backend.deleteResource(ctx, resource.kind, resource.name); err != nil {
				cleanupErrs = append(cleanupErrs, err)
			}
		}
		if err := waitForCollectorResourcesGone(ctx, func(checkCtx context.Context) (bool, error) {
			for _, resource := range created {
				metadata, exists, err := backend.getMetadata(checkCtx, resource.kind, resource.name)
				if err != nil {
					return false, err
				}
				if !exists {
					continue
				}
				if !ownedBy(metadata, plan.OwnershipMarker) {
					return false, fmt.Errorf("refusing to report failed collector cleanup after ownership changed for %s:%s", resource.kind, resource.name)
				}
				return false, nil
			}
			return true, nil
		}); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
		return errors.Join(cause, errors.Join(cleanupErrs...))
	}
	secretRef, err := parseKubernetesSecretReference(plan.CredentialRef, backend.namespace)
	if err != nil {
		return providerobservability.CollectorResource{}, err
	}
	configData := collectorConfig(plan, backend.credentialEnvironment, backend.credentialHeader)
	labels, annotations := collectorMetadata(plan, backend.credentialEnvironment, backend.credentialHeader)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: backend.configMapName(plan.OwnershipMarker), Namespace: backend.namespace, Labels: labels, Annotations: annotations},
		Data:       map[string]string{collectorConfigMapKey: configData},
	}
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: backend.serviceAccountName(plan.OwnershipMarker), Namespace: backend.namespace, Labels: labels, Annotations: annotations},
	}
	deployment := backend.deployment(plan, secretRef, configMap, labels, annotations)
	if err := backend.upsertConfigMap(ctx, configMap); err != nil {
		return providerobservability.CollectorResource{}, cleanupOnFailure(err)
	}
	configMapIdentity := collectorResourceIdentity("configmap", backend.namespace, configMap.Name)
	if _, found := existingIdentities[configMapIdentity]; !found {
		created = append(created, collectorResourceRef{kind: "configmap", name: configMap.Name})
	}
	if err := backend.upsertServiceAccount(ctx, serviceAccount); err != nil {
		return providerobservability.CollectorResource{}, cleanupOnFailure(err)
	}
	serviceAccountIdentity := collectorResourceIdentity("serviceaccount", backend.namespace, serviceAccount.Name)
	if _, found := existingIdentities[serviceAccountIdentity]; !found {
		created = append(created, collectorResourceRef{kind: "serviceaccount", name: serviceAccount.Name})
	}
	if err := backend.upsertDeployment(ctx, deployment); err != nil {
		return providerobservability.CollectorResource{}, cleanupOnFailure(err)
	}
	return backend.resourceFromDeployment(deployment, plan), nil
}

func validateExistingResources(resources []providerobservability.CollectorResource, plan sdk.CollectorDeploymentPlan) error {
	for _, resource := range resources {
		if !resource.Owned || resource.OwnershipMarker != plan.OwnershipMarker {
			return fmt.Errorf("refusing to replace an unowned collector resource %q", resource.Identity)
		}
		if resource.TargetProvider != plan.TargetProvider || resource.TargetRuntime != plan.TargetRuntime || resource.Workload != plan.Workload || resource.Distribution != plan.Distribution {
			return fmt.Errorf("refusing to reuse collector resource %q for a different target or distribution", resource.Identity)
		}
	}
	return nil
}

func (backend *KubernetesCollectorBackend) Verify(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) (providerobservability.CollectorVerification, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.CollectorVerification{}, err
	}
	deadline := time.NewTimer(collectorReadinessTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(collectorReadinessPollInterval)
	defer ticker.Stop()
	for {
		deployment, err := backend.client.AppsV1().Deployments(backend.namespace).Get(ctx, backend.deploymentName(plan.OwnershipMarker), metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return providerobservability.CollectorVerification{Reason: "collector Deployment is missing"}, nil
		}
		if err != nil {
			return providerobservability.CollectorVerification{}, fmt.Errorf("get collector Deployment: %w", err)
		}
		if !ownedBy(deployment.ObjectMeta, plan.OwnershipMarker) || resource.Identity != deploymentIdentity(backend.namespace, deployment.Name) {
			return providerobservability.CollectorVerification{}, errors.New("collector Deployment ownership does not match the plan")
		}
		ready := deployment.Spec.Replicas != nil && *deployment.Spec.Replicas > 0 && deployment.Status.ReadyReplicas >= *deployment.Spec.Replicas && deployment.Status.AvailableReplicas >= *deployment.Spec.Replicas
		healthy := ready && !hasDeploymentProgressFailure(deployment)
		verification := providerobservability.CollectorVerification{Ready: ready, Healthy: healthy}
		if !ready {
			verification.Reason = "collector Deployment has no ready replicas"
			if hasDeploymentProgressFailure(deployment) {
				verification.Reason = "collector Deployment reported a rollout failure"
				return verification, nil
			}
		} else if !healthy {
			verification.Reason = "collector Deployment reported a rollout failure"
			return verification, nil
		}
		if healthy {
			delivered, probeErr := backend.probe.VerifyCollectorSignals(ctx, plan, resource)
			if probeErr != nil {
				return providerobservability.CollectorVerification{}, fmt.Errorf("verify collector signal delivery: %w", probeErr)
			}
			verification.SignalsDelivered = delivered
			if !delivered {
				verification.Reason = "collector signal probe did not verify delivery"
			}
			return verification, nil
		}
		select {
		case <-ctx.Done():
			return providerobservability.CollectorVerification{}, ctx.Err()
		case <-deadline.C:
			verification.Reason = "collector Deployment did not become ready within the bounded readiness budget"
			return verification, nil
		case <-ticker.C:
		}
	}
}

func (backend *KubernetesCollectorBackend) Rollback(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) error {
	return backend.deleteOwned(ctx, plan, resource)
}

func (backend *KubernetesCollectorBackend) Destroy(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) error {
	return backend.deleteOwned(ctx, plan, resource)
}

func (backend *KubernetesCollectorBackend) Inventory(ctx context.Context, provider sdk.ProviderID, runtime sdk.RuntimeID, marker string) ([]providerobservability.CollectorResource, error) {
	if backend == nil || backend.client == nil {
		return nil, errors.New("Kubernetes collector backend is required")
	}
	if ctx == nil {
		return nil, errors.New("Kubernetes collector inventory context is required")
	}
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("Kubernetes collector ownership marker is required")
	}
	return backend.inventory(ctx, provider, runtime, marker)
}

func (backend *KubernetesCollectorBackend) inventory(ctx context.Context, provider sdk.ProviderID, runtime sdk.RuntimeID, marker string) ([]providerobservability.CollectorResource, error) {
	identities := []struct {
		kind string
		name string
	}{
		{kind: "deployment", name: backend.deploymentName(marker)},
		{kind: "configmap", name: backend.configMapName(marker)},
		{kind: "serviceaccount", name: backend.serviceAccountName(marker)},
	}
	resources := make([]providerobservability.CollectorResource, 0, len(identities))
	for _, identity := range identities {
		metadata, exists, err := backend.getMetadata(ctx, identity.kind, identity.name)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		resource := providerobservability.CollectorResource{
			Identity:        collectorResourceIdentity(identity.kind, backend.namespace, identity.name),
			TargetProvider:  sdk.ProviderID(metadata.Annotations[collectorProviderKey]),
			TargetRuntime:   sdk.RuntimeID(metadata.Annotations[collectorRuntimeKey]),
			Workload:        metadata.Annotations[collectorWorkloadKey],
			Distribution:    metadata.Annotations[collectorDistributionKey],
			OwnershipMarker: metadata.Annotations[collectorOwnershipMarkerKey],
			Status:          "configured",
			Owned:           ownedBy(metadata, marker),
		}
		if identity.kind == "deployment" {
			deployment, getErr := backend.client.AppsV1().Deployments(backend.namespace).Get(ctx, identity.name, metav1.GetOptions{})
			if getErr != nil {
				return nil, fmt.Errorf("get inventoried collector Deployment: %w", getErr)
			}
			resource.Status = deploymentStatus(deployment)
		}
		if resource.TargetProvider == "" {
			resource.TargetProvider = provider
		}
		if resource.TargetRuntime == "" {
			resource.TargetRuntime = runtime
		}
		resources = append(resources, resource)
	}
	return resources, nil
}

func (backend *KubernetesCollectorBackend) deleteOwned(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) error {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return err
	}
	if !resource.Owned || resource.OwnershipMarker != plan.OwnershipMarker {
		return errors.New("refusing Kubernetes collector deletion without exact ownership")
	}
	resources, err := backend.inventory(ctx, plan.TargetProvider, plan.TargetRuntime, plan.OwnershipMarker)
	if err != nil {
		return err
	}
	for _, current := range resources {
		if !current.Owned || current.OwnershipMarker != plan.OwnershipMarker {
			return errors.New("refusing Kubernetes collector deletion after ownership changed")
		}
	}
	for _, current := range resources {
		kind, name, ok := splitResourceIdentity(current.Identity)
		if !ok {
			return fmt.Errorf("invalid Kubernetes collector resource identity %q", current.Identity)
		}
		if err := backend.deleteResource(ctx, kind, name); err != nil {
			return err
		}
	}
	return waitForCollectorResourcesGone(ctx, func(checkCtx context.Context) (bool, error) {
		remaining, err := backend.inventory(checkCtx, plan.TargetProvider, plan.TargetRuntime, plan.OwnershipMarker)
		if err != nil {
			return false, err
		}
		for _, current := range remaining {
			if !current.Owned || current.OwnershipMarker != plan.OwnershipMarker {
				return false, errors.New("refusing Kubernetes collector cleanup after ownership changed")
			}
		}
		return len(remaining) == 0, nil
	})
}

// waitForCollectorResourcesGone makes direct owning-service inventory the
// cleanup truth. Delete calls can return before Kubernetes or Helm has removed
// the object, so reporting success after one read would allow the next
// certification cell to reuse a still-terminating scope.
func waitForCollectorResourcesGone(ctx context.Context, check func(context.Context) (bool, error)) error {
	if ctx == nil {
		return errors.New("collector cleanup context is required")
	}
	if check == nil {
		return errors.New("collector cleanup inventory check is required")
	}
	if gone, err := check(ctx); err != nil || gone {
		return err
	}
	timer := time.NewTimer(collectorCleanupTimeout)
	defer timer.Stop()
	ticker := time.NewTicker(collectorCleanupPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("collector cleanup did not converge")
		case <-ticker.C:
			gone, err := check(ctx)
			if err != nil {
				return err
			}
			if gone {
				return nil
			}
		}
	}
}

func (backend *KubernetesCollectorBackend) validatePlan(ctx context.Context, plan sdk.CollectorDeploymentPlan) error {
	if backend == nil || backend.client == nil {
		return errors.New("Kubernetes collector backend is required")
	}
	if ctx == nil {
		return errors.New("Kubernetes collector context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(plan.OwnershipMarker) == "" || strings.ContainsAny(plan.OwnershipMarker, "\r\n\x00") {
		return errors.New("Kubernetes collector ownership marker is required")
	}
	if plan.Distribution != "opentelemetry-collector-contrib" {
		return sdk.CollectorCapabilityError{Action: "backend", Reason: "client-go collector backend supports only the OpenTelemetry Collector Contrib distribution; inject the documented NRDOT Helm backend for NRDOT"}
	}
	if plan.Endpoint == "" {
		return errors.New("Kubernetes collector endpoint is required")
	}
	return nil
}

func (backend *KubernetesCollectorBackend) deployment(plan sdk.CollectorDeploymentPlan, secretRef kubernetesSecretReference, configMap *corev1.ConfigMap, labels, annotations map[string]string) *appsv1.Deployment {
	replicas := int32(1)
	container := corev1.Container{
		Name:            "collector",
		Image:           backend.image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args:            []string{collectorConfigArgument},
		Ports:           []corev1.ContainerPort{{Name: "otlp-grpc", ContainerPort: collectorGRPCPort}, {Name: "otlp-http", ContainerPort: collectorHTTPPort}},
		Env: []corev1.EnvVar{
			{Name: collectorOTLPEndpointEnv, Value: plan.Endpoint},
			{Name: backend.credentialEnvironment, ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: secretRef.Name}, Key: secretRef.Key}}},
		},
		VolumeMounts:    []corev1.VolumeMount{{Name: "collector-config", MountPath: collectorConfigMountPath, ReadOnly: true}},
		ReadinessProbe:  &corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(collectorGRPCPort)}}, InitialDelaySeconds: 5, PeriodSeconds: 5, FailureThreshold: 6},
		LivenessProbe:   &corev1.Probe{ProbeHandler: corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(collectorGRPCPort)}}, InitialDelaySeconds: 15, PeriodSeconds: 10, FailureThreshold: 3},
		SecurityContext: &corev1.SecurityContext{AllowPrivilegeEscalation: boolPtr(false), ReadOnlyRootFilesystem: boolPtr(true), RunAsNonRoot: boolPtr(true)},
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: backend.deploymentName(plan.OwnershipMarker), Namespace: backend.namespace, Labels: labels, Annotations: annotations},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "magelift-collector", collectorOwnershipDigestKey: markerDigest(plan.OwnershipMarker)}},
			Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: labels, Annotations: annotations}, Spec: corev1.PodSpec{ServiceAccountName: backend.serviceAccountName(plan.OwnershipMarker), SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: boolPtr(true)}, Containers: []corev1.Container{container}, Volumes: []corev1.Volume{{Name: "collector-config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: configMap.Name}, Items: []corev1.KeyToPath{{Key: collectorConfigMapKey, Path: collectorConfigFileName}}}}}}}},
		},
	}
}

func collectorConfig(plan sdk.CollectorDeploymentPlan, credentialEnvironment, credentialHeader string) string {
	config := "receivers:\n  otlp:\n    protocols:\n      grpc:\n        endpoint: 0.0.0.0:4317\n      http:\n        endpoint: 0.0.0.0:4318\nexporters:\n  otlphttp/destination:\n    endpoint: ${" + collectorOTLPEndpointEnv + "}\n    headers:\n      " + credentialHeader + ": ${" + credentialEnvironment + "}\nservice:\n  pipelines:\n"
	var builder strings.Builder
	builder.WriteString(config)
	for _, signal := range plan.Signals {
		builder.WriteString("    ")
		builder.WriteString(signal)
		builder.WriteString(":\n      receivers: [otlp]\n      exporters: [otlphttp/destination]\n")
	}
	return builder.String()
}

func (backend *KubernetesCollectorBackend) upsertConfigMap(ctx context.Context, desired *corev1.ConfigMap) error {
	existing, err := backend.client.CoreV1().ConfigMaps(backend.namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = backend.client.CoreV1().ConfigMaps(backend.namespace).Create(ctx, desired, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create collector ConfigMap: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get collector ConfigMap: %w", err)
	}
	if !ownedBy(existing.ObjectMeta, desired.Annotations[collectorOwnershipMarkerKey]) {
		return errors.New("refusing to replace an unowned collector ConfigMap")
	}
	desired.ResourceVersion = existing.ResourceVersion
	if _, err := backend.client.CoreV1().ConfigMaps(backend.namespace).Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update collector ConfigMap: %w", err)
	}
	return nil
}

func (backend *KubernetesCollectorBackend) upsertServiceAccount(ctx context.Context, desired *corev1.ServiceAccount) error {
	existing, err := backend.client.CoreV1().ServiceAccounts(backend.namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = backend.client.CoreV1().ServiceAccounts(backend.namespace).Create(ctx, desired, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create collector ServiceAccount: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get collector ServiceAccount: %w", err)
	}
	if !ownedBy(existing.ObjectMeta, desired.Annotations[collectorOwnershipMarkerKey]) {
		return errors.New("refusing to replace an unowned collector ServiceAccount")
	}
	desired.ResourceVersion = existing.ResourceVersion
	if _, err := backend.client.CoreV1().ServiceAccounts(backend.namespace).Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update collector ServiceAccount: %w", err)
	}
	return nil
}

func (backend *KubernetesCollectorBackend) upsertDeployment(ctx context.Context, desired *appsv1.Deployment) error {
	existing, err := backend.client.AppsV1().Deployments(backend.namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = backend.client.AppsV1().Deployments(backend.namespace).Create(ctx, desired, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create collector Deployment: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get collector Deployment: %w", err)
	}
	if !ownedBy(existing.ObjectMeta, desired.Annotations[collectorOwnershipMarkerKey]) {
		return errors.New("refusing to replace an unowned collector Deployment")
	}
	desired.ResourceVersion = existing.ResourceVersion
	desired.Spec.RevisionHistoryLimit = existing.Spec.RevisionHistoryLimit
	if _, err := backend.client.AppsV1().Deployments(backend.namespace).Update(ctx, desired, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update collector Deployment: %w", err)
	}
	return nil
}

func (backend *KubernetesCollectorBackend) resourceFromDeployment(deployment *appsv1.Deployment, plan sdk.CollectorDeploymentPlan) providerobservability.CollectorResource {
	return providerobservability.CollectorResource{Identity: deploymentIdentity(backend.namespace, deployment.Name), TargetProvider: plan.TargetProvider, TargetRuntime: plan.TargetRuntime, Workload: plan.Workload, Distribution: plan.Distribution, OwnershipMarker: plan.OwnershipMarker, Status: deploymentStatus(deployment), Owned: true}
}

func (backend *KubernetesCollectorBackend) getMetadata(ctx context.Context, kind, name string) (metav1.ObjectMeta, bool, error) {
	switch kind {
	case "deployment":
		resource, err := backend.client.AppsV1().Deployments(backend.namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return metav1.ObjectMeta{}, false, nil
		}
		if err != nil {
			return metav1.ObjectMeta{}, false, fmt.Errorf("get collector Deployment inventory: %w", err)
		}
		return resource.ObjectMeta, true, nil
	case "configmap":
		resource, err := backend.client.CoreV1().ConfigMaps(backend.namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return metav1.ObjectMeta{}, false, nil
		}
		if err != nil {
			return metav1.ObjectMeta{}, false, fmt.Errorf("get collector ConfigMap inventory: %w", err)
		}
		return resource.ObjectMeta, true, nil
	case "serviceaccount":
		resource, err := backend.client.CoreV1().ServiceAccounts(backend.namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return metav1.ObjectMeta{}, false, nil
		}
		if err != nil {
			return metav1.ObjectMeta{}, false, fmt.Errorf("get collector ServiceAccount inventory: %w", err)
		}
		return resource.ObjectMeta, true, nil
	default:
		return metav1.ObjectMeta{}, false, fmt.Errorf("unsupported Kubernetes collector resource kind %q", kind)
	}
}

func (backend *KubernetesCollectorBackend) deleteResource(ctx context.Context, kind, name string) error {
	var err error
	switch kind {
	case "deployment":
		err = backend.client.AppsV1().Deployments(backend.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "configmap":
		err = backend.client.CoreV1().ConfigMaps(backend.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	case "serviceaccount":
		err = backend.client.CoreV1().ServiceAccounts(backend.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	default:
		return fmt.Errorf("unsupported Kubernetes collector resource kind %q", kind)
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete collector %s %q: %w", kind, name, err)
	}
	return nil
}

type kubernetesSecretReference struct {
	Namespace string
	Name      string
	Key       string
}

type collectorResourceRef struct {
	kind string
	name string
}

func parseKubernetesSecretReference(raw, namespace string) (kubernetesSecretReference, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != collectorSecretScheme || parsed.Host == "" || parsed.Path == "" || parsed.RawQuery != "" || parsed.Fragment == "" {
		return kubernetesSecretReference{}, errors.New("Kubernetes collector credential reference must be kubernetes-secret://namespace/name#key")
	}
	name := strings.TrimPrefix(parsed.Path, "/")
	if strings.Contains(name, "/") || len(validation.IsDNS1123Subdomain(name)) > 0 || len(validation.IsDNS1123Subdomain(parsed.Host)) > 0 || parsed.Host != namespace {
		return kubernetesSecretReference{}, errors.New("Kubernetes collector credential reference must name a Secret in the selected namespace")
	}
	if len(validation.IsConfigMapKey(parsed.Fragment)) > 0 {
		return kubernetesSecretReference{}, errors.New("Kubernetes collector credential reference Secret key is invalid")
	}
	return kubernetesSecretReference{Namespace: parsed.Host, Name: name, Key: parsed.Fragment}, nil
}

func collectorMetadata(plan sdk.CollectorDeploymentPlan, credentialEnvironment, credentialHeader string) (map[string]string, map[string]string) {
	digest := markerDigest(plan.OwnershipMarker)
	labels := map[string]string{"app.kubernetes.io/name": "magelift-collector", "app.kubernetes.io/managed-by": collectorManagedBy, collectorOwnershipDigestKey: digest}
	annotations := map[string]string{collectorOwnershipMarkerKey: plan.OwnershipMarker, collectorProviderKey: string(plan.TargetProvider), collectorRuntimeKey: string(plan.TargetRuntime), collectorWorkloadKey: plan.Workload, collectorDistributionKey: plan.Distribution, collectorConfigHashKey: configHash(plan, credentialEnvironment, credentialHeader)}
	return labels, annotations
}

func (backend *KubernetesCollectorBackend) deploymentName(marker string) string {
	return collectorNamePrefix + markerDigest(marker)
}

func (backend *KubernetesCollectorBackend) configMapName(marker string) string {
	return backend.deploymentName(marker) + "-config"
}

func (backend *KubernetesCollectorBackend) serviceAccountName(marker string) string {
	return backend.deploymentName(marker)
}

func collectorResourceIdentity(kind, namespace, name string) string {
	return kind + ":" + namespace + "/" + name
}

func deploymentIdentity(namespace, name string) string {
	return collectorResourceIdentity("deployment", namespace, name)
}

func splitResourceIdentity(identity string) (string, string, bool) {
	kind, name, ok := strings.Cut(identity, ":")
	if !ok || kind == "" || name == "" {
		return "", "", false
	}
	if namespace, resourceName, found := strings.Cut(name, "/"); found {
		if namespace == "" || resourceName == "" {
			return "", "", false
		}
		return kind, resourceName, true
	}
	return kind, name, true
}

func ownedBy(metadata metav1.ObjectMeta, marker string) bool {
	return metadata.Annotations[collectorOwnershipMarkerKey] == marker && metadata.Labels[collectorOwnershipDigestKey] == markerDigest(marker)
}

func deploymentStatus(deployment *appsv1.Deployment) string {
	if deployment == nil {
		return "missing"
	}
	if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas > 0 && deployment.Status.ReadyReplicas >= *deployment.Spec.Replicas && deployment.Status.AvailableReplicas >= *deployment.Spec.Replicas {
		return "ready"
	}
	return "pending"
}

func hasDeploymentProgressFailure(deployment *appsv1.Deployment) bool {
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing && condition.Status == corev1.ConditionFalse {
			return true
		}
	}
	return false
}

func isImmutableImage(image string) bool {
	separator := strings.LastIndex(image, "@sha256:")
	return separator > 0 && len(image[separator+len("@sha256:"):]) == 64 && strings.Trim(image[separator+len("@sha256:"):], "0123456789abcdef") == ""
}

func markerDigest(marker string) string {
	digest := sha256.Sum256([]byte(marker))
	return hex.EncodeToString(digest[:])[:16]
}

func configHash(plan sdk.CollectorDeploymentPlan, credentialEnvironment, credentialHeader string) string {
	digest := sha256.Sum256([]byte(collectorConfig(plan, credentialEnvironment, credentialHeader)))
	return hex.EncodeToString(digest[:])[:16]
}

func boolPtr(value bool) *bool { return &value }

func validEnvironmentName(value string) bool {
	if value == "" {
		return false
	}
	for index, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || character == '_' || (index > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

func validHeaderName(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", character) {
			continue
		}
		return false
	}
	return true
}
