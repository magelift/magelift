package newrelic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	sdk "github.com/magelift/magelift/sdk/v1"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const (
	collectorOTLPHTTPPort       = 4318
	collectorOwnershipMarkerKey = "magelift.dev/ownership-marker"
)

// KubernetesCollectorSignalProbeConfig supplies the target-cluster client and
// the separate New Relic query verifier used by a live collector check.
type KubernetesCollectorSignalProbeConfig struct {
	Client             kubernetes.Interface
	RESTConfig         *rest.Config
	Query              MarkerQueryer
	QueryCredentialRef string
	HTTPClient         HTTPDoer
}

// KubernetesCollectorSignalProbe sends OTLP probes through a port-forward to
// the owned collector Pod and verifies each marker through New Relic NRQL.
type KubernetesCollectorSignalProbe struct {
	client             kubernetes.Interface
	restConfig         *rest.Config
	query              MarkerQueryer
	queryCredentialRef string
	httpClient         HTTPDoer
}

var _ interface {
	VerifyCollectorSignals(context.Context, sdk.CollectorDeploymentPlan, providerobservability.CollectorResource) (bool, error)
} = (*KubernetesCollectorSignalProbe)(nil)

func NewKubernetesCollectorSignalProbe(config KubernetesCollectorSignalProbeConfig) (*KubernetesCollectorSignalProbe, error) {
	if config.Client == nil {
		return nil, errors.New("Kubernetes collector signal probe client is required")
	}
	if config.RESTConfig == nil {
		return nil, errors.New("Kubernetes collector signal probe REST config is required")
	}
	if config.Query == nil {
		return nil, errors.New("Kubernetes collector signal probe query verifier is required")
	}
	queryCredentialRef := strings.TrimSpace(config.QueryCredentialRef)
	if err := sdk.ValidateCredentialReference(queryCredentialRef); err != nil {
		return nil, fmt.Errorf("Kubernetes collector signal probe query credential reference: %w", err)
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &KubernetesCollectorSignalProbe{
		client:             config.Client,
		restConfig:         config.RESTConfig,
		query:              config.Query,
		queryCredentialRef: queryCredentialRef,
		httpClient:         httpClient,
	}, nil
}

func (probe *KubernetesCollectorSignalProbe) VerifyCollectorSignals(ctx context.Context, plan sdk.CollectorDeploymentPlan, resource providerobservability.CollectorResource) (bool, error) {
	if ctx == nil {
		return false, errors.New("Kubernetes collector signal probe context is required")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if probe == nil || probe.client == nil || probe.restConfig == nil || probe.query == nil || probe.httpClient == nil {
		return false, errors.New("Kubernetes collector signal probe is required")
	}
	if !resource.Owned || resource.OwnershipMarker != plan.OwnershipMarker {
		return false, errors.New("Kubernetes collector signal probe requires exact collector ownership")
	}
	namespace, deploymentName, err := deploymentReference(resource.Identity)
	if err != nil {
		return false, err
	}
	deployment, err := probe.client.AppsV1().Deployments(namespace).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("get collector Deployment for signal probe: %w", err)
	}
	if deployment.Annotations[collectorOwnershipMarkerKey] != plan.OwnershipMarker {
		return false, errors.New("collector Deployment ownership marker changed before signal probe")
	}
	if deployment.Spec.Selector == nil || len(deployment.Spec.Selector.MatchLabels) == 0 {
		return false, errors.New("collector Deployment has no owned Pod selector")
	}
	selector := labels.SelectorFromSet(labels.Set(deployment.Spec.Selector.MatchLabels))
	pods, err := probe.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return false, fmt.Errorf("list collector Pods for signal probe: %w", err)
	}
	podName := ""
	for _, pod := range pods.Items {
		if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning || !podReady(&pod) {
			continue
		}
		if pod.Annotations[collectorOwnershipMarkerKey] != plan.OwnershipMarker {
			continue
		}
		podName = pod.Name
		break
	}
	if podName == "" {
		return false, errors.New("collector signal probe found no ready owned Pod")
	}

	return probe.withPortForward(ctx, namespace, podName, func(endpoint string) (bool, error) {
		for _, requestedSignal := range plan.Signals {
			payload, signal, err := BuildOTLPProbePayload(requestedSignal, plan.OwnershipMarker, time.Now())
			if err != nil {
				return false, err
			}
			if err := probe.send(ctx, endpoint, signal, payload); err != nil {
				return false, err
			}
			result, err := probe.query.QueryMarker(ctx, MarkerQueryRequest{
				Signal:          signal,
				CredentialRef:   probe.queryCredentialRef,
				OwnershipMarker: plan.OwnershipMarker,
			})
			if err != nil {
				return false, fmt.Errorf("query New Relic collector signal %q: %w", signal, err)
			}
			if result.Count == 0 || !result.LabelsVerified {
				return false, nil
			}
		}
		return true, nil
	})
}

// BuildOTLPProbePayload is shared by live collector probes so the marker
// attributes match the direct New Relic OTLP acceptance path.
func BuildOTLPProbePayload(signal, marker string, now time.Time) (proto.Message, Signal, error) {
	if err := validateOwnershipMarker(marker); err != nil {
		return nil, "", err
	}
	return probePayload(signal, marker, now)
}

func (probe *KubernetesCollectorSignalProbe) send(ctx context.Context, endpoint string, signal Signal, payload proto.Message) error {
	encoded, err := proto.Marshal(payload)
	if err != nil {
		return errors.New("marshal collector OTLP probe failed")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/v1/"+string(signal), bytes.NewReader(encoded))
	if err != nil {
		return errors.New("build collector OTLP probe request failed")
	}
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("Accept", "application/x-protobuf")
	response, err := probe.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send collector OTLP probe: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.CopyN(io.Discard, response.Body, 1024)
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("collector OTLP probe returned HTTP status %d", response.StatusCode)
	}
	return nil
}

func (probe *KubernetesCollectorSignalProbe) withPortForward(ctx context.Context, namespace, podName string, verify func(string) (bool, error)) (bool, error) {
	if verify == nil {
		return false, errors.New("collector port-forward verification callback is required")
	}
	requestURL := probe.client.CoreV1().RESTClient().Post().Resource("pods").Namespace(namespace).Name(podName).SubResource("portforward").URL()
	transport, upgrader, err := spdy.RoundTripperFor(probe.restConfig)
	if err != nil {
		return false, fmt.Errorf("create Kubernetes port-forward transport: %w", err)
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, requestURL)
	stop := make(chan struct{})
	ready := make(chan struct{})
	forwarder, err := portforward.New(dialer, []string{fmt.Sprintf("0:%d", collectorOTLPHTTPPort)}, stop, ready, io.Discard, io.Discard)
	if err != nil {
		return false, fmt.Errorf("create Kubernetes port-forward: %w", err)
	}
	forwardErr := make(chan error, 1)
	go func() {
		forwardErr <- forwarder.ForwardPorts()
	}()
	defer func() {
		close(stop)
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case <-forwardErr:
		case <-timer.C:
		}
	}()

	select {
	case <-ready:
	case err := <-forwardErr:
		if err == nil {
			return false, errors.New("Kubernetes port-forward stopped before readiness")
		}
		return false, fmt.Errorf("Kubernetes port-forward stopped before readiness: %w", err)
	case <-ctx.Done():
		return false, ctx.Err()
	}
	ports, err := forwarder.GetPorts()
	if err != nil {
		return false, fmt.Errorf("get Kubernetes port-forwarded port: %w", err)
	}
	if len(ports) != 1 || ports[0].Local == 0 {
		return false, errors.New("Kubernetes port-forward did not expose exactly one local port")
	}
	return verify(fmt.Sprintf("http://127.0.0.1:%d", ports[0].Local))
}

func deploymentReference(identity string) (string, string, error) {
	kind, qualified, ok := strings.Cut(identity, ":")
	if !ok || kind != "deployment" {
		return "", "", fmt.Errorf("collector signal probe requires a Deployment identity, got %q", identity)
	}
	namespace, name, ok := strings.Cut(qualified, "/")
	if !ok || namespace == "" || name == "" || strings.ContainsAny(namespace+name, "\r\n\x00") {
		return "", "", fmt.Errorf("collector signal probe received an invalid Deployment identity %q", identity)
	}
	return namespace, name, nil
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}
