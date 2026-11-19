package plugin

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRuntimeHealthUsesPreviewNamespace(t *testing.T) {
	plan := deployTestPlan(t)
	replicas := int32(1)
	client := fake.NewClientset(&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "magelift-web", Namespace: "ml-pr-1"}, Spec: appsv1.DeploymentSpec{Replicas: &replicas}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1, Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}}})
	server := &Server{KubeClients: func(map[string]any) (kubernetes.Interface, error) { return client, nil }}
	outputs, _ := json.Marshal(map[string]any{"serviceName": "magelift-web", "namespace": "ml-pr-1"})
	var result sdk.CheckRuntimeResult
	err := server.CheckRuntime(&sdk.CheckRuntimeCall{ProtocolVersion: sdk.ProtocolV1, Envelope: testPlanRequest().Envelope, Plan: plan, OutputsJSON: outputs}, &result)
	if err != nil || result.Error != nil {
		t.Fatalf("runtime health: %v, %#v", err, result.Error)
	}
	var health []platform.RuntimeHealth
	if err := json.Unmarshal(result.HealthJSON, &health); err != nil || len(health) == 0 || health[0].Status != "healthy" {
		t.Fatalf("runtime result: %v, %#v", err, health)
	}
}

func TestExecAndTunnelKeepPreviewNamespace(t *testing.T) {
	plan := deployTestPlan(t)
	server := &Server{}
	outputs, _ := json.Marshal(map[string]any{"clusterName": "cluster", "serviceName": "magelift-web", "namespace": "ml-pr-1", "kubeconfig": "apiVersion: v1\nkind: Config\n", "searchEndpoint": "magelift-search"})
	execQuery, _ := json.Marshal(platform.ExecQuery{Command: []string{"php", "-v"}})
	var execResult sdk.PrepareExecResult
	if err := server.PrepareExec(&sdk.PrepareExecCall{ProtocolVersion: sdk.ProtocolV1, Envelope: testPlanRequest().Envelope, Plan: plan, OutputsJSON: outputs, QueryJSON: execQuery}, &execResult); err != nil || execResult.Error != nil {
		t.Fatalf("exec: %v %#v", err, execResult.Error)
	}
	var execTarget platform.ExecTarget
	if err := json.Unmarshal(execResult.TargetJSON, &execTarget); err != nil {
		t.Fatal(err)
	}
	for _, path := range execTarget.CleanupPaths {
		defer os.Remove(path)
	}
	if !strings.Contains(strings.Join(execTarget.Args, " "), "-n ml-pr-1") {
		t.Fatalf("exec namespace: %v", execTarget.Args)
	}
	query, _ := json.Marshal(platform.TunnelQuery{Target: "search", LocalPort: 19200, RemotePort: 9200})
	var tunnelResult sdk.PrepareTunnelResult
	if err := server.PrepareTunnel(&sdk.PrepareTunnelCall{ProtocolVersion: sdk.ProtocolV1, Envelope: testPlanRequest().Envelope, Plan: plan, OutputsJSON: outputs, QueryJSON: query}, &tunnelResult); err != nil || tunnelResult.Error != nil {
		t.Fatalf("tunnel: %v %#v", err, tunnelResult.Error)
	}
	var tunnel platform.ExecTarget
	if err := json.Unmarshal(tunnelResult.TargetJSON, &tunnel); err != nil {
		t.Fatal(err)
	}
	for _, path := range tunnel.CleanupPaths {
		defer os.Remove(path)
	}
	command := strings.Join(tunnel.Args, " ")
	if !strings.Contains(command, "-n ml-pr-1") || !strings.Contains(command, "--address 127.0.0.1") || !strings.Contains(command, "service/magelift-search") {
		t.Fatalf("tunnel: %s", command)
	}
	query, _ = json.Marshal(platform.TunnelQuery{Target: "database", LocalPort: 13306, RemotePort: 3306})
	tunnelResult = sdk.PrepareTunnelResult{}
	if err := server.PrepareTunnel(&sdk.PrepareTunnelCall{ProtocolVersion: sdk.ProtocolV1, Envelope: testPlanRequest().Envelope, Plan: plan, OutputsJSON: outputs, QueryJSON: query}, &tunnelResult); err != nil || tunnelResult.Error == nil {
		t.Fatalf("managed database tunnel should fail: %v %#v", err, tunnelResult)
	}
}
