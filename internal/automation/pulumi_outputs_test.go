package automation

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

func TestMapStackOutputsKeepsSecretValues(t *testing.T) {
	t.Parallel()
	got := mapStackOutputs(auto.OutputMap{
		"kubeconfig":     {Value: "apiVersion: v1\n", Secret: true},
		"clusterName":    {Value: "mlgcpwt-preview-gke", Secret: false},
		"applicationURL": {Value: "http://1.2.3.4", Secret: false},
		"emptySecret":    {Value: "", Secret: true},
	})
	if got["kubeconfig"] != "apiVersion: v1\n" {
		t.Fatalf("kubeconfig = %#v; secret outputs must remain usable for day-2", got["kubeconfig"])
	}
	if got["clusterName"] != "mlgcpwt-preview-gke" {
		t.Fatalf("clusterName = %#v", got["clusterName"])
	}
	redacted := redactSecretOutputs(auto.OutputMap{
		"kubeconfig":  {Value: "apiVersion: v1\n", Secret: true},
		"clusterName": {Value: "mlgcpwt-preview-gke", Secret: false},
	})
	if _, ok := redacted["kubeconfig"].(map[string]any); !ok {
		t.Fatalf("CLI redaction lost secret marker: %#v", redacted["kubeconfig"])
	}
	if redacted["clusterName"] != "mlgcpwt-preview-gke" {
		t.Fatalf("clusterName redacted incorrectly: %#v", redacted["clusterName"])
	}
}
