package kube

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	projectionDefaultNamespace = metav1.NamespaceDefault
)

type ProjectionCommandRequest = cloudrecovery.ProjectionCommandRequest
type ProjectionCommandResult = cloudrecovery.ProjectionCommandResult
type ProjectionCommandRunner = cloudrecovery.ProjectionCommandRunner
type ProjectionVerifier = cloudrecovery.ProjectionVerifier

// KubernetesProjectionConfig contains only injected runtime ports. Target
// identity belongs to KubernetesCommandRunnerConfig, while provider
// credentials and native SDK responses stay outside the portable recovery
// contract.
type KubernetesProjectionConfig struct {
	Runner   ProjectionCommandRunner
	Verifier ProjectionVerifier
}

// KubernetesProjectionBackend maps Magento search/cache operations to a
// Kubernetes runtime. The shared command backend owns action, identity,
// output hygiene, and result normalization.
type KubernetesProjectionBackend struct {
	*cloudrecovery.CommandProjectionBackend
}

var _ cloudrecovery.ProjectionBackend = (*KubernetesProjectionBackend)(nil)

// NewKubernetesProjectionBackend constructs a reusable Kubernetes projection
// adapter. A verifier is mandatory because command exit status alone cannot
// prove known content, permissions, secret-reference reachability, or service
// health.
func NewKubernetesProjectionBackend(config KubernetesProjectionConfig) (*KubernetesProjectionBackend, error) {
	backend, err := cloudrecovery.NewCommandProjectionBackend(config.Runner, config.Verifier)
	if err != nil {
		return nil, err
	}
	return &KubernetesProjectionBackend{CommandProjectionBackend: backend}, nil
}

// NewKubernetesProjectionLifecycle constructs the complete shared projection
// lifecycle from a stack-produced kubeconfig. The kubeconfig is consumed only
// at this provider/runtime boundary; it is never copied into a projection
// request, operation ID, or proof record. EKS, GKE, Kapsule, MKS, and
// community Kubernetes providers therefore reuse one command transport and
// one core lifecycle while supplying only their target identity and verifier.
func NewKubernetesProjectionLifecycle(
	kubeconfig []byte,
	target KubernetesCommandRunnerConfig,
	verifier ProjectionVerifier,
) (*cloudrecovery.ProjectionLifecycle, error) {
	config, err := RESTConfigFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes projection client: %w", err)
	}
	runner, err := NewKubernetesCommandRunner(client, config, target)
	if err != nil {
		return nil, err
	}
	backend, err := NewKubernetesProjectionBackend(KubernetesProjectionConfig{
		Runner:   runner,
		Verifier: verifier,
	})
	if err != nil {
		return nil, err
	}
	lifecycle, err := cloudrecovery.NewProjectionLifecycle(backend)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes projection lifecycle: %w", err)
	}
	return lifecycle, nil
}

// KubernetesCommandRunner executes a command in one ready pod selected from
// the workload's app label. The selection is deterministic and only runs
// against a pod that is currently Running and Ready.
type KubernetesCommandRunner struct {
	client    kubernetes.Interface
	config    *rest.Config
	namespace string
	workload  string
	container string
}

// KubernetesCommandRunnerConfig identifies the workload whose ready pod will
// receive projection commands. It is deliberately runtime-specific: the core
// projection contract never needs to know how a Kubernetes workload is named.
type KubernetesCommandRunnerConfig struct {
	Namespace string
	Workload  string
	Container string
}

// NewKubernetesCommandRunner constructs the client-go remotecommand runner.
// The REST config is required separately because typed clients do not expose
// the transport configuration needed by the exec subresource.
func NewKubernetesCommandRunner(client kubernetes.Interface, config *rest.Config, target KubernetesCommandRunnerConfig) (*KubernetesCommandRunner, error) {
	if client == nil {
		return nil, errors.New("Kubernetes command runner client is required")
	}
	if config == nil {
		return nil, errors.New("Kubernetes command runner REST config is required")
	}
	namespace := strings.TrimSpace(target.Namespace)
	if namespace == "" {
		namespace = projectionDefaultNamespace
	}
	if errs := validation.IsDNS1123Subdomain(namespace); len(errs) > 0 {
		return nil, fmt.Errorf("Kubernetes command runner namespace is invalid: %s", strings.Join(errs, "; "))
	}
	workload := strings.TrimSpace(target.Workload)
	if workload == "" {
		return nil, errors.New("Kubernetes command runner workload is required")
	}
	if errs := validation.IsDNS1123Subdomain(workload); len(errs) > 0 {
		return nil, fmt.Errorf("Kubernetes command runner workload is invalid: %s", strings.Join(errs, "; "))
	}
	container := strings.TrimSpace(target.Container)
	if container != "" {
		if errs := validation.IsDNS1123Label(container); len(errs) > 0 {
			return nil, fmt.Errorf("Kubernetes command runner container is invalid: %s", strings.Join(errs, "; "))
		}
	}
	return &KubernetesCommandRunner{
		client: client, config: rest.CopyConfig(config), namespace: namespace,
		workload: workload, container: container,
	}, nil
}

func (runner *KubernetesCommandRunner) Run(ctx context.Context, request ProjectionCommandRequest) (ProjectionCommandResult, error) {
	if runner == nil || runner.client == nil || runner.config == nil {
		return ProjectionCommandResult{}, errors.New("Kubernetes command runner is not configured")
	}
	if ctx == nil {
		return ProjectionCommandResult{}, errors.New("Kubernetes command runner context is required")
	}
	if err := ctx.Err(); err != nil {
		return ProjectionCommandResult{}, err
	}
	if len(request.Command) == 0 {
		return ProjectionCommandResult{}, errors.New("Kubernetes command request is empty")
	}
	pod, container, err := runner.readyPod(ctx)
	if err != nil {
		return ProjectionCommandResult{}, err
	}
	restClient := runner.client.CoreV1().RESTClient()
	if restClient == nil {
		return ProjectionCommandResult{}, errors.New("Kubernetes command runner REST client is unavailable")
	}
	execRequest := restClient.Post().Resource("pods").Name(pod.Name).Namespace(runner.namespace).SubResource("exec")
	execRequest.VersionedParams(&corev1.PodExecOptions{
		Container: container, Command: append([]string(nil), request.Command...), Stdout: true, Stderr: true,
	}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(runner.config, "POST", execRequest.URL())
	if err != nil {
		return ProjectionCommandResult{}, fmt.Errorf("create Kubernetes remote executor: %w", err)
	}
	var stdout, stderr bytes.Buffer
	started := time.Now()
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr})
	elapsed := time.Since(started)
	duration := int64(elapsed / time.Second)
	if elapsed%time.Second != 0 {
		duration++
	}
	if err != nil {
		return ProjectionCommandResult{}, fmt.Errorf("execute command in ready pod: %w", err)
	}
	return ProjectionCommandResult{
		Status:            sdk.ResilienceOperationSucceeded,
		OperationID:       projectionOperationID(request, runner.namespace, runner.workload, pod.Name),
		ResourceReference: "kubernetes-projection://" + runner.namespace + "/" + pod.Name,
		ProofReferences:   []string{"kubernetes.projection." + request.Projection.DataClass},
		Stdout:            append([]byte(nil), stdout.Bytes()...), Stderr: append([]byte(nil), stderr.Bytes()...),
		RestoreDurationSeconds: duration,
		Reason:                 "Kubernetes projection command completed in a ready pod",
	}, nil
}

func (runner *KubernetesCommandRunner) readyPod(ctx context.Context) (corev1.Pod, string, error) {
	pods, err := runner.client.CoreV1().Pods(runner.namespace).List(ctx, metav1.ListOptions{LabelSelector: labels.Set{"app": runner.workload}.AsSelector().String()})
	if err != nil {
		return corev1.Pod{}, "", fmt.Errorf("list ready Kubernetes projection pods: %w", err)
	}
	sort.Slice(pods.Items, func(i, j int) bool { return pods.Items[i].Name < pods.Items[j].Name })
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || !podReady(&pod) {
			continue
		}
		container := runner.container
		if container == "" && len(pod.Spec.Containers) > 0 {
			container = pod.Spec.Containers[0].Name
		}
		if container == "" {
			continue
		}
		for _, candidate := range pod.Spec.Containers {
			if candidate.Name == container {
				return pod, container, nil
			}
		}
	}
	return corev1.Pod{}, "", fmt.Errorf("no ready Kubernetes projection pod found for app=%s", runner.workload)
}

func podReady(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func projectionOperationID(request ProjectionCommandRequest, namespace, workload, podName string) string {
	hash := sha256.Sum256([]byte(strings.Join([]string{
		namespace, workload, podName, request.Projection.DataClass,
		string(request.Projection.Action), request.Projection.FixtureID, request.Projection.OwnershipMarker,
		request.Projection.IdempotencyKey,
	}, "\x00")))
	return "kubernetes-projection-operation://" + hex.EncodeToString(hash[:16])
}
