package kube

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/platform"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
)

const (
	defaultNamespace      = "default"
	execReadyPollInterval = 2 * time.Second
	execReadyTimeout      = 5 * time.Minute
	magentoHTTPTimeout    = 5 * time.Second
	magentoHTTPMaxBody    = 1 << 20
)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

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
	httpDoer    httpDoer
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
		TailLines:  int64Ptr(int64(limit)),
		Timestamps: true,
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
	partial := make([]platform.LogReadFailure, 0)
	workload := strings.TrimSpace(string(query.Workload))
	if workload == "" {
		workload = "web"
	}
	for _, pod := range pods.Items {
		podOpts := *opts
		podOpts.Container = resolveLogContainer(pod, workload)
		req := client.CoreV1().Pods(namespace).GetLogs(pod.Name, &podOpts)
		stream, err := req.Stream(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			partial = append(partial, platform.LogReadFailure{Source: pod.Name, Err: errors.New(platform.RedactLogMessage(err.Error()))})
			continue
		}
		data, readErr := io.ReadAll(stream)
		_ = stream.Close()
		if readErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			partial = append(partial, platform.LogReadFailure{Source: pod.Name, Err: errors.New(platform.RedactLogMessage(readErr.Error()))})
			continue
		}
		for index, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
			timestamp, message := parseKubernetesLogLine(line, time.Now().UTC())
			if strings.TrimSpace(message) == "" {
				continue
			}
			if !query.Since.IsZero() && timestamp.Before(query.Since.UTC()) {
				continue
			}
			if query.Until != nil && timestamp.After(query.Until.UTC()) {
				continue
			}
			if filter := strings.TrimSpace(query.Filter); filter != "" && !strings.Contains(message, filter) {
				continue
			}
			events = append(events, platform.LogEvent{
				Timestamp: timestamp,
				Message:   platform.RedactLogMessage(message),
				Workload:  workload,
				Source:    pod.Name,
				EventID:   fmt.Sprintf("%s/%d", pod.Name, index),
			})
			if len(events) >= limit {
				return sortLogEvents(events), nil
			}
		}
	}
	if len(partial) > 0 {
		return sortLogEvents(events), &platform.PartialLogError{Failures: partial}
	}
	return sortLogEvents(events), nil
}

func resolveLogContainer(pod corev1.Pod, workload string) string {
	preferred := strings.TrimSpace(workload)
	if preferred == "" {
		preferred = "web"
	}
	for _, container := range pod.Spec.Containers {
		if container.Name == preferred {
			return preferred
		}
	}
	if len(pod.Spec.Containers) == 1 {
		return pod.Spec.Containers[0].Name
	}
	return preferred
}

func parseKubernetesLogLine(line string, fallback time.Time) (time.Time, string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return fallback.UTC(), ""
	}
	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 2 {
		if timestamp, err := time.Parse(time.RFC3339Nano, parts[0]); err == nil {
			return timestamp.UTC(), parts[1]
		}
	}
	return fallback.UTC(), line
}

func sortLogEvents(events []platform.LogEvent) []platform.LogEvent {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			if events[i].Source == events[j].Source {
				return events[i].EventID < events[j].EventID
			}
			return events[i].Source < events[j].Source
		}
		return events[i].Timestamp.Before(events[j].Timestamp)
	})
	return events
}

// CheckRuntime reports Deployment ready/desired replica health (KUBE-02) and,
// when a storefront URL is known, Magento HTTP health as runtime.web.
// Autopilot SkipAwait often leaves applicationURL empty; the Service
// LoadBalancer IP is then the preview HTTP origin. No URL means the Magento
// check is omitted rather than guessed.
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
	checks := []platform.RuntimeHealth{{
		ID: "runtime.kube.deployment", Service: service, Status: status, Detail: detail,
	}}
	if magento, ok := o.checkMagentoHTTP(ctx, client, outputs, service); ok {
		checks = append(checks, magento)
	}
	return checks, nil
}

func (o *Observe) checkMagentoHTTP(ctx context.Context, client kubernetes.Interface, outputs map[string]any, service string) (platform.RuntimeHealth, bool) {
	unhealthy := platform.RuntimeHealth{ID: "runtime.web", Service: service, Status: "unhealthy", Detail: "Magento HTTP probe failed"}
	// skipAwait freezes stack applicationURL on the first LoadBalancer hostname.
	// Auto Mode NLB replacement (internal → internet-facing) is only visible on
	// the live Service, so probe that first when Kubernetes can answer.
	if storefront, err := magentoLoadBalancerURL(ctx, client, o.ns(), service); err == nil && storefront != "" {
		return o.probeMagentoHTTP(ctx, service, storefront), true
	}
	if raw, _ := outputs[platform.OutputApplicationURL].(string); strings.TrimSpace(raw) != "" {
		storefront, err := validateMagentoStorefrontURL(raw)
		if err != nil {
			unhealthy.Detail = "Magento applicationURL is not a usable storefront URL"
			return unhealthy, true
		}
		return o.probeMagentoHTTP(ctx, service, storefront), true
	}
	return platform.RuntimeHealth{}, false
}

func (o *Observe) probeMagentoHTTP(ctx context.Context, service, storefront string) platform.RuntimeHealth {
	check := platform.RuntimeHealth{ID: "runtime.web", Service: service, Status: "unhealthy", Detail: "Magento HTTP probe failed"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, storefront, nil)
	if err != nil {
		check.Detail = "Magento HTTP probe could not build request"
		return check
	}
	resp, err := o.http().Do(req)
	if err != nil {
		check.Detail = "Magento HTTP probe did not complete"
		return check
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, magentoHTTPMaxBody))
	if isLoopbackHTTPLocation(resp.Header.Get("Location")) {
		check.Detail = "Magento HTTP redirected to localhost"
		return check
	}
	if resp.StatusCode != http.StatusOK {
		check.Detail = fmt.Sprintf("Magento HTTP status %d, want 200", resp.StatusCode)
		return check
	}
	check.Status = "healthy"
	check.Detail = "Magento HTTP returned 200"
	return check
}

func (o *Observe) http() httpDoer {
	if o != nil && o.httpDoer != nil {
		return o.httpDoer
	}
	return &http.Client{
		Timeout: magentoHTTPTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func magentoLoadBalancerURL(ctx context.Context, client kubernetes.Interface, namespace, service string) (string, error) {
	if client == nil {
		return "", nil
	}
	svc, err := client.CoreV1().Services(namespace).Get(ctx, service, metav1.GetOptions{})
	if err != nil {
		return "", nil
	}
	host := loadBalancerHost(svc)
	if host == "" {
		return "", nil
	}
	if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return validateMagentoStorefrontURL("http://" + host + "/")
}

func loadBalancerHost(svc *corev1.Service) string {
	if svc == nil {
		return ""
	}
	for _, ingress := range svc.Status.LoadBalancer.Ingress {
		if ip := strings.TrimSpace(ingress.IP); ip != "" && net.ParseIP(ip) != nil && !net.ParseIP(ip).IsLoopback() && !net.ParseIP(ip).IsUnspecified() {
			return ip
		}
		if host := strings.TrimSpace(ingress.Hostname); host != "" && !isLoopbackHost(host) {
			return host
		}
	}
	return ""
}

func validateMagentoStorefrontURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("magento storefront URL must be an absolute http(s) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("magento storefront URL must use http or https")
	}
	if parsed.User != nil {
		return "", errors.New("magento storefront URL must not contain credentials")
	}
	if isLoopbackHost(parsed.Hostname()) {
		return "", errors.New("magento storefront URL must not be localhost")
	}
	return parsed.String(), nil
}

func isLoopbackHTTPLocation(location string) bool {
	if strings.TrimSpace(location) == "" {
		return false
	}
	parsed, err := url.Parse(location)
	if err != nil {
		return false
	}
	return isLoopbackHost(parsed.Hostname())
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// PrepareExec returns a portable kubectl ExecTarget (KUBE-03). Args are the
// argv after the binary name (matching the AWS CLI Args shape).
// Writes kubeconfig to a temp file and passes --kubeconfig (T-06-08 fail-closed
// on missing output). Omits -t so scripted acceptance exec works without a TTY.
func (o *Observe) PrepareExec(ctx context.Context, _ platform.PlannedStack, outputs map[string]any, query platform.ExecQuery) (platform.ExecTarget, error) {
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
	deploy := resolveDeployment(nil, string(query.Workload), service)
	if query.WaitForReady {
		client, err := o.clientFor(outputs)
		if err != nil {
			return platform.ExecTarget{}, fmt.Errorf("prepare exec client: %w", err)
		}
		if err := waitForDeploymentReady(ctx, client, o.ns(), deploy); err != nil {
			return platform.ExecTarget{}, err
		}
	}
	kubePath, err := writeKubeconfigTemp(kubeconfig)
	if err != nil {
		return platform.ExecTarget{}, err
	}
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

// PrepareTunnel returns a loopback-only kubectl port-forward target for
// provider-owned Kubernetes Services. Managed database access and dashboards
// need provider-specific adapters and fail closed here.
func (o *Observe) PrepareTunnel(ctx context.Context, _ platform.PlannedStack, outputs map[string]any, query platform.TunnelQuery) (platform.ExecTarget, error) {
	target, err := platform.NormalizeTunnelTarget(query.Target)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	query.Target = target
	if err := platform.ValidateTunnelQuery(query); err != nil {
		return platform.ExecTarget{}, err
	}
	if err := ctx.Err(); err != nil {
		return platform.ExecTarget{}, err
	}
	expectedRemotePort := platform.DefaultTunnelRemotePort(target)
	if query.RemotePort != expectedRemotePort {
		return platform.ExecTarget{}, fmt.Errorf("remote port %d is not supported for tunnel target %q (want %d): %w", query.RemotePort, target, expectedRemotePort, platform.ErrNotSupported)
	}
	switch target {
	case platform.TunnelTargetDatabase, platform.TunnelTargetDatabaseUI:
		return platform.ExecTarget{}, fmt.Errorf("Kubernetes runtime has no verified private tunnel for %q: the database is managed outside the cluster: %w", target, platform.ErrNotSupported)
	case platform.TunnelTargetSearchUI:
		return platform.ExecTarget{}, fmt.Errorf("search management UI is not provisioned by this stack: %w", platform.ErrNotSupported)
	}

	serviceKey := platform.OutputServiceName
	if target == platform.TunnelTargetQueue || target == platform.TunnelTargetQueueUI {
		serviceKey = platform.OutputQueueHost
	}
	if target == platform.TunnelTargetSearch {
		serviceKey = platform.OutputSearchEndpoint
	}
	service, err := tunnelServiceOutput(outputs, serviceKey, target)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	kubeconfig, err := platform.RequireStringOutput(outputs, platform.OutputKubeconfig)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	kubePath, err := writeKubeconfigTemp(kubeconfig)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	return platform.ExecTarget{
		Launcher: "kubectl",
		Args: []string{
			"--kubeconfig", kubePath,
			"port-forward", "-n", o.ns(), "--address", "127.0.0.1",
			"service/" + service,
			fmt.Sprintf("%d:%d", query.LocalPort, query.RemotePort),
		},
		Task:         service,
		CleanupPaths: []string{kubePath},
	}, nil
}

func tunnelServiceOutput(outputs map[string]any, key, target string) (string, error) {
	value, err := platform.RequireStringOutput(outputs, key)
	if err != nil {
		return "", fmt.Errorf("tunnel target %q is not provisioned: %w: %w", target, err, platform.ErrNotSupported)
	}
	if problems := validation.IsDNS1123Subdomain(value); len(problems) > 0 {
		return "", fmt.Errorf("tunnel target %q has invalid provider service %q: %s", target, value, strings.Join(problems, "; "))
	}
	return value, nil
}

func waitForDeploymentReady(ctx context.Context, client kubernetes.Interface, namespace, deployment string) error {
	if client == nil {
		return errors.New("prepare exec client is nil")
	}
	waitContext, cancel := context.WithTimeout(ctx, execReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(execReadyPollInterval)
	defer ticker.Stop()
	var lastDetail string
	for {
		dep, err := client.AppsV1().Deployments(namespace).Get(waitContext, deployment, metav1.GetOptions{})
		if err != nil {
			lastDetail = err.Error()
		} else {
			desired := int32(1)
			if dep.Spec.Replicas != nil {
				desired = *dep.Spec.Replicas
			}
			if deploymentReady(dep, desired) {
				return nil
			}
			lastDetail = fmt.Sprintf("ready replicas=%d desired replicas=%d", dep.Status.ReadyReplicas, desired)
		}
		select {
		case <-waitContext.Done():
			return fmt.Errorf("wait for deployment %s readiness: %s: %w", deployment, lastDetail, waitContext.Err())
		case <-ticker.C:
		}
	}
}

func deploymentReady(dep *appsv1.Deployment, desired int32) bool {
	return dep != nil && desired > 0 && dep.Status.ReadyReplicas >= desired
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
