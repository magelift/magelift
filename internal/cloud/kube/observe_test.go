package kube

import (
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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

func TestObservePrepareExecKubectl(t *testing.T) {
	obs := NewObserve(fake.NewClientset())
	target, err := obs.PrepareExec(context.Background(), observePlanned{project: "shop", env: "preview"}, map[string]any{
		platform.OutputClusterName: "shop-cluster",
		platform.OutputServiceName: "shop-web",
		platform.OutputKubeconfig:  BuildStaticTokenKubeconfig("c", "https://1.2.3.4", testCAData, "tok"),
	}, platform.ExecQuery{
		Workload: "web",
		Command:  []string{"bin/magento", "cache:flush"},
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
