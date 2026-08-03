package kube

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

// testCAData is base64 PEM for a throwaway self-signed cert so NewForConfig can
// load root certificates offline. Do not print in logs (threat T-06-04).
const testCAData = "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSURFVENDQWZtZ0F3SUJBZ0lVRWFwZGFUWWZwK0xNUUNBcUN4Tmx6OXRSZ1lnd0RRWUpLb1pJaHZjTkFRRUwKQlFBd0dERVdNQlFHQTFVRUF3d05iV0ZuWld4cFpuUXRkR1Z6ZERBZUZ3MHlOakEzTXpBeE1EUTVOREZhRncweQpOakEzTXpFeE1EUTVOREZhTUJneEZqQVVCZ05WQkFNTURXMWhaMlZzYVdaMExYUmxjM1F3Z2dFaU1BMEdDU3FHClNJYjNEUUVCQVFVQUE0SUJEd0F3Z2dFS0FvSUJBUURURUhrckVick9zWUtHYzluRzJxTnpxQnRSbjNzY3hoYzEKaVVmMG5IUGsycEdtQ1hrU0daL29xbjJVdDc0TWNEQy8zN1BVZWFPaVJta0hOdDlxaXZjbW9VdHFtNHJkZ0lUMwo1SklmUWNrdHNON3VMWEhkaTIybW9jSmdXSjVWY0h1S2QvUk1CTkVmVVZOUERqNlgxMTNDc1FRU2NISHI1RXYwCnh6ellZZitiUlFRbWlsaTRBOXhKWmFuZFdmVE9HOVNkTmZVSGZiKzRxd0MzK2xlNmJ2aXNJaTV1SXYrSStlczEKcXE5U0tpOWpaYzN4Q3FBMXVEZ1k4aEttWklucU44WVRpc2NZcVpWZDhoUml1ZEZJLy9rNmxsbmthQnFlVVpENwpCcVk4eFBaOUtiOVJMSXUwYWJXR2V3YzZjblJFaXpBMW9kZzZtdkJ3Y0k2VmlaRSt6Y3ROQWdNQkFBR2pVekJSCk1CMEdBMVVkRGdRV0JCVG1zcWJ1ekd3U2JmcFVJVklVdTFHcCtXMlIvVEFmQmdOVkhTTUVHREFXZ0JUbXNxYnUKekd3U2JmcFVJVklVdTFHcCtXMlIvVEFQQmdOVkhSTUJBZjhFQlRBREFRSC9NQTBHQ1NxR1NJYjNEUUVCQ3dVQQpBNElCQVFDQWVaZm1iV2h0QXI1MzRTdEwyYWJZeXE4SVlkWUFMdWxhT2FyL0hYakJycjFUYWprMkprc0FwN2Y2CjRwUndPV0RFalUxRWhta1RoajFGYVhrdFl3Qzh0L2dHMFJTdEhiUFR0eDJ5bGs0ZExmQXdTck9JNlNocUY4aGMKb1QvRkdHSDdKOGtiQnRsUW5sMDE2WjA5Y05CMXhuZFhpWU15L2R0Yld4THB6VTgvTVd5WW01S0ROZWJRQWFNTgpPSDlRUzFTaSswTkFuTW9ZdmZLS0xhbktLOGlIK2RlaythelFySHZUb3ByTnRBQzRQZDhseEM1WG4xZVkycFlGCkY5U2hlTWtMWUlaRXd6Q3BONGFyamtaeGVQK240MWdvUWFQbCtObFZUV3dmTU5GWEQ0Lzg0NzdCL3lKbnlzdkYKYUhpZEZTVXdKb3I1N3JsUDg2My95Tzg2TWc5WQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="

func TestClientFromKubeconfigStaticToken(t *testing.T) {
	t.Parallel()
	kubeconfig := BuildStaticTokenKubeconfig("magelift_shop-cluster", "https://1.2.3.4:6443", testCAData, "mock-token")
	client, err := ClientFromKubeconfig([]byte(kubeconfig))
	if err != nil {
		t.Fatalf("ClientFromKubeconfig: %v", err)
	}
	if client == nil {
		t.Fatal("ClientFromKubeconfig returned nil Interface")
	}
}

func TestClientFromKubeconfigEmptyFailsClosed(t *testing.T) {
	t.Parallel()
	if _, err := ClientFromKubeconfig(nil); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty kubeconfig error, got %v", err)
	}
	if _, err := ClientFromKubeconfig([]byte("   \n")); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected whitespace-only kubeconfig error, got %v", err)
	}
}

func TestClientFromKubeconfigInvalidFailsClosed(t *testing.T) {
	t.Parallel()
	if _, err := ClientFromKubeconfig([]byte("not-a-kubeconfig")); err == nil {
		t.Fatal("invalid kubeconfig was accepted")
	}
}

func TestClientFromOutputs(t *testing.T) {
	t.Parallel()
	kubeconfig := BuildStaticTokenKubeconfig("magelift_shop-cluster", "https://1.2.3.4:6443", testCAData, "mock-token")
	client, err := ClientFromOutputs(map[string]any{platform.OutputKubeconfig: kubeconfig})
	if err != nil {
		t.Fatalf("ClientFromOutputs: %v", err)
	}
	if client == nil {
		t.Fatal("ClientFromOutputs returned nil Interface")
	}
}

func TestClientFromOutputsMissingKey(t *testing.T) {
	t.Parallel()
	_, err := ClientFromOutputs(map[string]any{platform.OutputClusterName: "shop"})
	if err == nil || !strings.Contains(err.Error(), platform.OutputKubeconfig) {
		t.Fatalf("expected missing %q error, got %v", platform.OutputKubeconfig, err)
	}
}

func TestClientFromOutputsEmptyKey(t *testing.T) {
	t.Parallel()
	_, err := ClientFromOutputs(map[string]any{platform.OutputKubeconfig: "  "})
	if err == nil || !strings.Contains(err.Error(), platform.OutputKubeconfig) {
		t.Fatalf("expected empty %q error, got %v", platform.OutputKubeconfig, err)
	}
}
