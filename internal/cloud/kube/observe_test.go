package kube

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

type observePlanned struct {
	project, env string
}

func (p observePlanned) StackName() string        { return p.project + "-" + p.env }
func (p observePlanned) Provider() sdk.ProviderID { return "gcp" }
func (p observePlanned) Runtime() sdk.RuntimeID   { return "gke-autopilot" }
func (p observePlanned) Project() string          { return p.project }
func (p observePlanned) Environment() string      { return p.env }
func (p observePlanned) Region() string           { return "europe-west9" }
func (p observePlanned) CertificationTier() platform.CertificationTier {
	return platform.TierCertified
}
func (p observePlanned) EnvironmentClass() string { return "preview" }
func (p observePlanned) Protected() bool          { return false }
func (p observePlanned) ImageDigest() string      { return "" }
func (p observePlanned) WithImageDigest(string) (platform.PlannedStack, error) {
	return p, nil
}
func (p observePlanned) TargetDescriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{Provider: "gcp", Runtime: "gke-autopilot"}
}

func TestObserveTailLogsFakeClientset(t *testing.T) {
	deployment := "shop-preview-app-web"
	cs := fake.NewClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "web-0",
			Namespace: "default",
			Labels:    map[string]string{"app": deployment},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "web"}}},
	})
	obs := NewObserve(cs)
	events, err := obs.TailLogs(context.Background(), observePlanned{project: "shop", env: "preview"}, platform.LogQuery{
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("TailLogs: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("TailLogs returned no events")
	}
	if !strings.Contains(events[0].Message, "fake logs") {
		t.Fatalf("message = %q, want fake logs", events[0].Message)
	}
}

func TestObserveTailLogsEmptyPods(t *testing.T) {
	obs := NewObserve(fake.NewClientset())
	_, err := obs.TailLogs(context.Background(), observePlanned{project: "shop", env: "preview"}, platform.LogQuery{
		Workload: "missing",
		Limit:    5,
	})
	if err == nil || !strings.Contains(err.Error(), "no pods found") {
		t.Fatalf("expected no pods found error, got %v", err)
	}
}

func TestObserveTailLogsFiltersOrdersBoundsAndRedacts(t *testing.T) {
	deployment := "shop-preview-app-web"
	client := fake.NewClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-b", Namespace: "default", Labels: map[string]string{"app": deployment}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "php-fpm"}, {Name: "web"}}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-a", Namespace: "default", Labels: map[string]string{"app": deployment}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "php-fpm"}, {Name: "web"}}}},
	)
	responses := []string{
		"2026-07-18T02:00:02.000000Z ERROR password=hidden-1\n2026-07-18T02:00:04.000000Z INFO ignored\n",
		"2026-07-18T02:00:01.000000Z ERROR Authorization: Bearer abc.def.ghi\n",
	}
	call := 0
	client.PrependReactor("get", "pods/log", func(action ktesting.Action) (bool, runtime.Object, error) {
		generic, ok := action.(ktesting.GenericAction)
		if !ok {
			t.Fatalf("action = %T, want GenericAction", action)
		}
		opts, ok := generic.GetValue().(*corev1.PodLogOptions)
		if !ok {
			t.Fatalf("log options = %T, want *PodLogOptions", generic.GetValue())
		}
		if !opts.Timestamps {
			t.Fatal("Kubernetes log timestamps must be requested")
		}
		if opts.Container != "web" {
			t.Fatalf("Kubernetes log container = %q, want web", opts.Container)
		}
		if call >= len(responses) {
			return true, nil, errors.New("unexpected extra log request")
		}
		response := &runtime.Unknown{Raw: []byte(responses[call])}
		call++
		return true, response, nil
	})

	obs := NewObserve(client)
	since := time.Date(2026, time.July, 18, 2, 0, 0, 0, time.UTC)
	until := since.Add(3 * time.Second)
	events, err := obs.TailLogs(context.Background(), observePlanned{project: "shop", env: "preview"}, platform.LogQuery{
		Since: since, Until: &until, Filter: "ERROR", Limit: 10,
	})
	if err != nil {
		t.Fatalf("TailLogs: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %#v, want two matching events", events)
	}
	if !events[0].Timestamp.Before(events[1].Timestamp) || events[0].Source != "web-b" || events[1].Source != "web-a" {
		t.Fatalf("events are not deterministically ordered: %#v", events)
	}
	if strings.Contains(events[0].Message, "abc.def.ghi") || strings.Contains(events[1].Message, "hidden-1") {
		t.Fatalf("credential leaked into events: %#v", events)
	}
	if events[0].Workload != "web" || events[0].EventID == "" {
		t.Fatalf("normalized metadata missing: %#v", events[0])
	}
}

func TestObserveTailLogsReturnsPartialRead(t *testing.T) {
	deployment := "shop-preview-app-web"
	client := fake.NewClientset(
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-good", Namespace: "default", Labels: map[string]string{"app": deployment}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "web-bad", Namespace: "default", Labels: map[string]string{"app": deployment}}},
	)
	call := 0
	wantErr := errors.New("pod log token=private failed")
	client.PrependReactor("get", "pods/log", func(_ ktesting.Action) (bool, runtime.Object, error) {
		call++
		if call == 1 {
			return true, &runtime.Unknown{Raw: []byte("good log")}, nil
		}
		return true, nil, wantErr
	})

	events, err := NewObserve(client).TailLogs(context.Background(), observePlanned{project: "shop", env: "preview"}, platform.LogQuery{Limit: 10})
	var partial *platform.PartialLogError
	if !errors.As(err, &partial) {
		t.Fatalf("error = %v, want PartialLogError", err)
	}
	if len(events) != 1 || len(partial.Failures) != 1 || events[0].Source == partial.Failures[0].Source {
		t.Fatalf("events=%#v partial=%#v", events, partial)
	}
	if strings.Contains(err.Error(), "private") {
		t.Fatalf("credential leaked in partial error: %v", err)
	}
}

func TestParseKubernetesLogLine(t *testing.T) {
	fallback := time.Date(2026, time.July, 18, 2, 0, 0, 0, time.UTC)
	stamp, message := parseKubernetesLogLine("2026-07-18T02:00:01.123456Z hello", fallback)
	if !stamp.Equal(fallback.Add(time.Second+123456000*time.Nanosecond)) || message != "hello" {
		t.Fatalf("parsed timestamp/message = %v/%q", stamp, message)
	}
	stamp, message = parseKubernetesLogLine("unstructured", fallback)
	if !stamp.Equal(fallback) || message != "unstructured" {
		t.Fatalf("fallback timestamp/message = %v/%q", stamp, message)
	}
}

func TestObserveCheckRuntimeHealthy(t *testing.T) {
	replicas := int32(2)
	cs := fake.NewClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-web", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 2,
			Conditions: []appsv1.DeploymentCondition{{
				Type:   appsv1.DeploymentAvailable,
				Status: corev1.ConditionTrue,
			}},
		},
	})
	obs := NewObserve(cs)
	health, err := obs.CheckRuntime(context.Background(), observePlanned{project: "shop", env: "preview"}, map[string]any{
		platform.OutputServiceName: "shop-web",
	})
	if err != nil {
		t.Fatalf("CheckRuntime: %v", err)
	}
	if len(health) != 1 || health[0].Status != "healthy" {
		t.Fatalf("health = %#v", health)
	}
	if health[0].ID != "runtime.kube.deployment" {
		t.Fatalf("id = %q", health[0].ID)
	}
}

func TestObserveCheckRuntimeUnhealthy(t *testing.T) {
	replicas := int32(2)
	cs := fake.NewClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-web", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 0,
			Conditions: []appsv1.DeploymentCondition{{
				Type:   appsv1.DeploymentAvailable,
				Status: corev1.ConditionFalse,
			}},
		},
	})
	obs := NewObserve(cs)
	health, err := obs.CheckRuntime(context.Background(), observePlanned{project: "shop", env: "preview"}, map[string]any{
		platform.OutputServiceName: "shop-web",
	})
	if err != nil {
		t.Fatalf("CheckRuntime: %v", err)
	}
	if len(health) != 1 || health[0].Status != "unhealthy" {
		t.Fatalf("health = %#v", health)
	}
}

func readyWebDeployment() *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-web", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1,
			Conditions: []appsv1.DeploymentCondition{{
				Type:   appsv1.DeploymentAvailable,
				Status: corev1.ConditionTrue,
			}},
		},
	}
}

type staticHTTP struct {
	status   int
	location string
	err      error
	sawURL   string
}

func (s *staticHTTP) Do(req *http.Request) (*http.Response, error) {
	if s == nil {
		return nil, errors.New("http doer is nil")
	}
	if req != nil && req.URL != nil {
		s.sawURL = req.URL.String()
	}
	if s.err != nil {
		return nil, s.err
	}
	header := make(http.Header)
	if s.location != "" {
		header.Set("Location", s.location)
	}
	return &http.Response{
		StatusCode: s.status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader("ok")),
		Request:    req,
	}, nil
}

func TestObserveCheckRuntimeMagentoHTTPFromLoadBalancer(t *testing.T) {
	cs := fake.NewClientset(readyWebDeployment(), &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-web", Namespace: "default"},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{{IP: "203.0.113.10"}},
			},
		},
	})
	httpDoer := &staticHTTP{status: http.StatusOK}
	obs := NewObserve(cs)
	obs.httpDoer = httpDoer
	health, err := obs.CheckRuntime(context.Background(), observePlanned{project: "shop", env: "preview"}, map[string]any{
		platform.OutputServiceName: "shop-web",
	})
	if err != nil {
		t.Fatalf("CheckRuntime: %v", err)
	}
	if len(health) != 2 {
		t.Fatalf("health = %#v", health)
	}
	if health[1].ID != "runtime.web" || health[1].Status != "healthy" {
		t.Fatalf("magento health = %#v", health[1])
	}
	if httpDoer.sawURL != "http://203.0.113.10/" {
		t.Fatalf("probed %q", httpDoer.sawURL)
	}
}

func TestObserveCheckRuntimeMagentoHTTPPrefersLiveServiceOverStaleApplicationURL(t *testing.T) {
	cs := fake.NewClientset(readyWebDeployment(), &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-web", Namespace: "default"},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{{Hostname: "k8s-shop-new.elb.amazonaws.com"}},
			},
		},
	})
	httpDoer := &staticHTTP{status: http.StatusOK}
	obs := NewObserve(cs)
	obs.httpDoer = httpDoer
	health, err := obs.CheckRuntime(context.Background(), observePlanned{project: "shop", env: "preview"}, map[string]any{
		platform.OutputServiceName:    "shop-web",
		platform.OutputApplicationURL: "http://k8s-shop-stale.elb.amazonaws.com/",
	})
	if err != nil {
		t.Fatalf("CheckRuntime: %v", err)
	}
	if len(health) != 2 || health[1].Status != "healthy" {
		t.Fatalf("magento health = %#v", health[1])
	}
	if httpDoer.sawURL != "http://k8s-shop-new.elb.amazonaws.com/" {
		t.Fatalf("probed stale applicationURL %q", httpDoer.sawURL)
	}
}

func TestObserveCheckRuntimeMagentoHTTPLocalhostRedirect(t *testing.T) {
	cs := fake.NewClientset(readyWebDeployment())
	obs := NewObserve(cs)
	obs.httpDoer = &staticHTTP{status: http.StatusFound, location: "http://localhost:8080/"}
	health, err := obs.CheckRuntime(context.Background(), observePlanned{project: "shop", env: "preview"}, map[string]any{
		platform.OutputServiceName:    "shop-web",
		platform.OutputApplicationURL: "http://203.0.113.10/",
	})
	if err != nil {
		t.Fatalf("CheckRuntime: %v", err)
	}
	if len(health) != 2 || health[1].Status != "unhealthy" || !strings.Contains(health[1].Detail, "localhost") {
		t.Fatalf("magento health = %#v", health[1])
	}
}

func TestObserveCheckRuntimeMagentoHTTPRejectsLocalhostApplicationURL(t *testing.T) {
	cs := fake.NewClientset(readyWebDeployment())
	obs := NewObserve(cs)
	health, err := obs.CheckRuntime(context.Background(), observePlanned{project: "shop", env: "preview"}, map[string]any{
		platform.OutputServiceName:    "shop-web",
		platform.OutputApplicationURL: "http://localhost:8080/",
	})
	if err != nil {
		t.Fatalf("CheckRuntime: %v", err)
	}
	if len(health) != 2 || health[1].Status != "unhealthy" || !strings.Contains(health[1].Detail, "applicationURL") {
		t.Fatalf("magento health = %#v", health)
	}
}

func TestObservePrepareExecKubectl(t *testing.T) {
	replicas := int32(1)
	obs := NewObserve(fake.NewClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-web", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 1},
	}))
	target, err := obs.PrepareExec(context.Background(), observePlanned{project: "shop", env: "preview"}, map[string]any{
		platform.OutputClusterName: "shop-cluster",
		platform.OutputServiceName: "shop-web",
		platform.OutputKubeconfig:  BuildStaticTokenKubeconfig("c", "https://1.2.3.4", testCAData, "tok"),
	}, platform.ExecQuery{
		Workload:     "web",
		Command:      []string{"bin/magento", "cache:flush"},
		WaitForReady: true,
	})
	if err != nil {
		t.Fatalf("PrepareExec: %v", err)
	}
	if target.Launcher != "kubectl" {
		t.Fatalf("launcher = %q, want kubectl", target.Launcher)
	}
	if target.Launcher == "gke-job" {
		t.Fatal("gke-job launcher debt must be removed")
	}
	if len(target.Args) < 8 || target.Args[0] != "--kubeconfig" {
		t.Fatalf("args = %#v; want --kubeconfig <path> exec ...", target.Args)
	}
	if len(target.CleanupPaths) != 1 || target.CleanupPaths[0] != target.Args[1] {
		t.Fatalf("CleanupPaths = %#v; want kubeconfig path %q", target.CleanupPaths, target.Args[1])
	}
	joined := strings.Join(target.Args[2:], " ")
	want := "exec -n default -i deploy/shop-web -- bin/magento cache:flush"
	if joined != want {
		t.Fatalf("args after kubeconfig = %q, want %q", joined, want)
	}
}

func TestObservePrepareExecMintsFreshToken(t *testing.T) {
	replicas := int32(1)
	obs := NewObserve(fake.NewClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-web", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status:     appsv1.DeploymentStatus{ReadyReplicas: 1},
	})).WithTokenSource(func(context.Context) (string, error) { return "fresh-token", nil })
	target, err := obs.PrepareExec(context.Background(), observePlanned{project: "shop", env: "preview"}, map[string]any{
		platform.OutputClusterName: "shop-cluster",
		platform.OutputServiceName: "shop-web",
		platform.OutputKubeconfig:  BuildStaticTokenKubeconfig("c", "https://1.2.3.4", testCAData, "stale-token"),
	}, platform.ExecQuery{Workload: "web", Command: []string{"bin/magento", "cache:flush"}})
	if err != nil {
		t.Fatalf("PrepareExec: %v", err)
	}
	body, err := os.ReadFile(target.Args[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "fresh-token") || strings.Contains(string(body), "stale-token") {
		t.Fatalf("launched kubeconfig = %s", body)
	}
	for _, arg := range target.Args {
		if strings.Contains(arg, "fresh-token") || strings.Contains(arg, "stale-token") {
			t.Fatalf("token leaked to argv: %#v", target.Args)
		}
	}
}

func TestObservePrepareExecSurfacesTokenFailure(t *testing.T) {
	obs := NewObserve(fake.NewClientset()).WithTokenSource(func(context.Context) (string, error) {
		return "", errors.New("refresh exploded")
	})
	_, err := obs.PrepareExec(context.Background(), observePlanned{project: "shop", env: "preview"}, map[string]any{
		platform.OutputClusterName: "shop-cluster",
		platform.OutputServiceName: "shop-web",
		platform.OutputKubeconfig:  BuildStaticTokenKubeconfig("c", "https://1.2.3.4", testCAData, "stale-token"),
	}, platform.ExecQuery{Workload: "web"})
	if err == nil || !strings.Contains(err.Error(), "refresh exploded") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeploymentReady(t *testing.T) {
	replicas := int32(1)
	for name, tc := range map[string]struct {
		dep     *appsv1.Deployment
		desired int32
		want    bool
	}{
		"ready": {
			dep: &appsv1.Deployment{
				Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
				Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
			},
			desired: replicas,
			want:    true,
		},
		"not-ready": {
			dep: &appsv1.Deployment{
				Spec:   appsv1.DeploymentSpec{Replicas: &replicas},
				Status: appsv1.DeploymentStatus{ReadyReplicas: 0},
			},
			desired: replicas,
			want:    false,
		},
		"zero-desired": {
			dep:     &appsv1.Deployment{},
			desired: 0,
			want:    false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := deploymentReady(tc.dep, tc.desired); got != tc.want {
				t.Fatalf("deploymentReady() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestObservePrepareExecRequiresKubeconfig(t *testing.T) {
	obs := NewObserve(fake.NewClientset())
	_, err := obs.PrepareExec(context.Background(), observePlanned{project: "shop", env: "preview"}, map[string]any{
		platform.OutputClusterName: "shop-cluster",
		platform.OutputServiceName: "shop-web",
	}, platform.ExecQuery{Command: []string{"/bin/sh"}})
	if err == nil || !strings.Contains(err.Error(), platform.OutputKubeconfig) {
		t.Fatalf("expected kubeconfig required error, got %v", err)
	}
}

func TestGCPRuntimeObserveTypeIdentity(t *testing.T) {
	obs := NewObserveWithFactory(ClientFromOutputs)
	if _, ok := any(obs).(platform.RuntimeObserve); !ok {
		t.Fatal("Observe must implement platform.RuntimeObserve")
	}
	if _, ok := any(obs).(*Observe); !ok {
		t.Fatalf("want *Observe, got %T", obs)
	}
}

func TestObserveBindOutputsCachesServiceName(t *testing.T) {
	kubeconfig := BuildStaticTokenKubeconfig("c", "https://1.2.3.4", testCAData, "tok")
	obs := NewObserveWithFactory(ClientFromOutputs)
	if err := obs.BindOutputs(map[string]any{
		platform.OutputKubeconfig:  kubeconfig,
		platform.OutputServiceName: "mlgcpwt-preview-app-web",
	}); err != nil {
		t.Fatalf("BindOutputs: %v", err)
	}
	if obs.client == nil {
		t.Fatal("BindOutputs did not cache client")
	}
	if obs.serviceName != "mlgcpwt-preview-app-web" {
		t.Fatalf("serviceName = %q", obs.serviceName)
	}
	if got := resolveDeployment(nil, "web", obs.serviceName); got != "mlgcpwt-preview-app-web" {
		t.Fatalf("resolveDeployment = %q, want mlgcpwt-preview-app-web", got)
	}
}
