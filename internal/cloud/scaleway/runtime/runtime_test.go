package runtime

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/cloud/scaleway/naming"
)

func TestBuildKubeconfigUsesStaticTokenNotExecPlugin(t *testing.T) {
	t.Parallel()
	kubeconfig := kube.BuildStaticTokenKubeconfig("magelift_shop-preview-kapsule", naming.FormatURL("https://xxx.pub.k8s.fr-par.scw.cloud:6443"), "Y2E=", "mock-scw-token")

	if strings.Contains(kubeconfig, "exec:") {
		t.Fatal("kubeconfig must not depend on an exec auth plugin")
	}
	if !strings.Contains(kubeconfig, "token: mock-scw-token") {
		t.Fatalf("kubeconfig missing static token auth: %s", kubeconfig)
	}
	if !strings.Contains(kubeconfig, "certificate-authority-data: Y2E=") {
		t.Fatal("kubeconfig missing cluster CA")
	}
	if !strings.Contains(kubeconfig, "server: https://xxx.pub.k8s.fr-par.scw.cloud:6443") {
		t.Fatal("kubeconfig missing cluster endpoint")
	}
	if !strings.Contains(kubeconfig, "name: magelift_shop-preview-kapsule") {
		t.Fatal("kubeconfig missing expected context name")
	}
}

func TestBuildKubeconfigAddsSchemeWhenHostHasNone(t *testing.T) {
	t.Parallel()
	kubeconfig := kube.BuildStaticTokenKubeconfig("magelift_shop-preview-kapsule", naming.FormatURL("1.2.3.4:6443"), "Y2E=", "mock-scw-token")
	if !strings.Contains(kubeconfig, "server: https://1.2.3.4:6443") {
		t.Fatalf("expected an https scheme to be added: %s", kubeconfig)
	}
}
