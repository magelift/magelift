package kube

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/platform"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const defaultNamespace = "default"

// ClientFactory builds a kubernetes.Interface from Magento stack outputs.
// Live adapters pass ClientFromOutputs; tests inject a client via NewObserve.
type ClientFactory func(outputs map[string]any) (kubernetes.Interface, error)

// Observe implements platform.RuntimeObserve against a Kubernetes API
// (GKE Autopilot, EKS Autopilot, OVH MKS, Scaleway Kapsule).
type Observe struct {
	client      kubernetes.Interface
	factory     ClientFactory
	namespace   string
	serviceName string // OutputServiceName cached by BindOutputs for TailLogs
}

// NewObserve returns Observe backed by an injected clientset (tests / pre-built clients).
func NewObserve(client kubernetes.Interface) *Observe {
	return &Observe{client: client, namespace: defaultNamespace}
}

// NewObserveWithFactory returns Observe that builds a client from stack outputs
// on each CheckRuntime/PrepareExec call. TailLogs requires an injected client
// (platform.RuntimeObserve.TailLogs has no outputs argument); factory-only
// Observe returns a clear error from TailLogs until CLI passes outputs.
func NewObserveWithFactory(factory ClientFactory) *Observe {
	if factory == nil {
		factory = ClientFromOutputs
	}
	return &Observe{factory: factory, namespace: defaultNamespace}
}

func (o *Observe) clientFor(outputs map[string]any) (kubernetes.Interface, error) {
	if o == nil {
		return nil, fmt.Errorf("kube observe is nil")
	}
	if o.client != nil {
		return o.client, nil
	}
	factory := o.factory
	if factory == nil {
		factory = ClientFromOutputs
	}
	client, err := factory(outputs)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, fmt.Errorf("kubernetes client factory returned nil")
	}
	return client, nil
}

func (o *Observe) ns() string {
	if o != nil && strings.TrimSpace(o.namespace) != "" {
		return o.namespace
	}
	return defaultNamespace
}

// BindOutputs builds and caches a kubernetes client from stack outputs so
// TailLogs (which has no outputs argument on platform.RuntimeObserve) can
// work with factory-backed Observe used by GKE/EKS/OVH/Scaleway adapters.
// Also caches OutputServiceName so TailLogs selects app=<service> (not bare "web").
func (o *Observe) BindOutputs(outputs map[string]any) error {
	client, err := o.clientFor(outputs)
	if err != nil {
		return err
	}
	svc, err := platform.RequireStringOutput(outputs, platform.OutputServiceName)
	if err != nil {
		return err
	}
	o.client = client
	o.serviceName = svc
	return nil
}

// TailLogs streams recent pod logs for the Magento workload (KUBE-01).
// Label selector is app=<deployment>, matching the Magento Deployment name.
// Factory-only Observe must BindOutputs first (CLI logs wires this).
func (o *Observe) TailLogs(ctx context.Context, planned platform.PlannedStack, query platform.LogQuery) ([]platform.LogEvent, error) {
	client, err := o.clientFor(nil)
	if err != nil {
		return nil, fmt.Errorf("tail logs: bind stack outputs first (missing kubeconfig): %w", err)
	}
	deployment := resolveDeployment(planned, string(query.Workload), o.serviceName)
	limit := query.Limit
	if limit <= 0 {
		limit = platform.DefaultLogLimit
	}
	if limit > platform.MaxLogLimit {
		limit = platform.MaxLogLimit
	}
	opts := &corev1.PodLogOptions{
		TailLines: int64Ptr(int64(limit)),
	}
	if !query.Since.IsZero() {
		opts.SinceTime = &metav1.Time{Time: query.Since.UTC()}
	}
	namespace := o.ns()
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + deployment,
	})
	if err != nil {
		return nil, fmt.Errorf("list pods for %s: %w", deployment, err)
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pods found for app=%s in namespace %s", deployment, namespace)
	}
	events := make([]platform.LogEvent, 0, limit)
	now := time.Now().UTC()
	for _, pod := range pods.Items {
		req := client.CoreV1().Pods(namespace).GetLogs(pod.Name, opts)
		stream, err := req.Stream(ctx)
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(stream)
		_ = stream.Close()
		if readErr != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			events = append(events, platform.LogEvent{Timestamp: now, Message: line})
			if len(events) >= limit {
				return events, nil
			}
		}
	}
	return events, nil
}

// CheckRuntime reports Deployment ready/desired replica health (KUBE-02).
func (o *Observe) CheckRuntime(ctx context.Context, _ platform.PlannedStack, outputs map[string]any) ([]platform.RuntimeHealth, error) {
	service, err := platform.RequireStringOutput(outputs, platform.OutputServiceName)
	if err != nil {
		return nil, err
	}
	client, err := o.clientFor(outputs)
	if err != nil {
		return nil, err
	}
	namespace := o.ns()
	dep, err := client.AppsV1().Deployments(namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get deployment %s: %w", service, err)
	}
	desired := 0
	if dep.Spec.Replicas != nil {
		desired = int(*dep.Spec.Replicas)
	}
	available := false
	for _, condition := range dep.Status.Conditions {
		if condition.Type == "Available" && condition.Status == corev1.ConditionTrue {
			available = true
			break
		}
	}
	ready := int(dep.Status.ReadyReplicas)
	status, detail := "healthy", fmt.Sprintf("deployment has %d ready of %d desired replicas", ready, desired)
	if !available || desired <= 0 || ready < desired {
		status = "unhealthy"
	}
	return []platform.RuntimeHealth{{
		ID: "runtime.kube.deployment", Service: service, Status: status, Detail: detail,
	}}, nil
}

// PrepareExec returns a portable kubectl ExecTarget (KUBE-03). Args are the
// argv after the binary name (matching the AWS CLI Args shape).
// Writes kubeconfig to a temp file and passes --kubeconfig (T-06-08 fail-closed
// on missing output). Omits -t so scripted acceptance exec works without a TTY.
func (o *Observe) PrepareExec(_ context.Context, _ platform.PlannedStack, outputs map[string]any, query platform.ExecQuery) (platform.ExecTarget, error) {
	cluster, err := platform.RequireStringOutput(outputs, platform.OutputClusterName)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	service, err := platform.RequireStringOutput(outputs, platform.OutputServiceName)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	// Fail closed on missing kubeconfig so callers cannot silently fall back to
	// provider ADC (T-06-08). Value is not logged (T-06-07).
	kubeconfig, err := platform.RequireStringOutput(outputs, platform.OutputKubeconfig)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	kubePath, err := writeKubeconfigTemp(kubeconfig)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	deploy := resolveDeployment(nil, string(query.Workload), service)
	command := query.Command
	if len(command) == 0 {
		command = []string{"/bin/sh"}
	}
	args := []string{
		"--kubeconfig", kubePath,
		"exec", "-n", o.ns(), "-i",
	}
	if stdoutIsTerminal() {
		args = append(args, "-t")
	}
	args = append(args, "deploy/"+deploy, "--")
	args = append(args, command...)
	return platform.ExecTarget{
		Launcher:     "kubectl",
		Args:         args,
		Cluster:      cluster,
		Task:         deploy,
		Container:    strings.TrimSpace(query.Container),
		CleanupPaths: []string{kubePath},
	}, nil
}

func stdoutIsTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func writeKubeconfigTemp(kubeconfig string) (string, error) {
	f, err := os.CreateTemp("", "magelift-kubeconfig-*.yaml")
	if err != nil {
		return "", fmt.Errorf("create kubeconfig temp file: %w", err)
	}
	path := f.Name()
	if _, err := f.WriteString(kubeconfig); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write kubeconfig temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close kubeconfig temp file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("chmod kubeconfig temp file: %w", err)
	}
	return path, nil
}

func resolveDeployment(planned platform.PlannedStack, workload, service string) string {
	workload = strings.TrimSpace(workload)
	service = strings.TrimSpace(service)
	if service != "" {
		if workload == "" || workload == "web" {
			return service
		}
		return strings.TrimSuffix(service, "-web") + "-" + workload
	}
	if workload != "" {
		return workload
	}
	if planned == nil {
		return "web"
	}
	return planned.Project() + "-" + planned.Environment() + "-app-web"
}

func int64Ptr(v int64) *int64 { return &v }
