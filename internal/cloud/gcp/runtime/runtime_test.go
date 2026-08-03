package runtime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cloud/kube"
)

func TestBuildKubeconfigUsesOAuthTokenNotExecPlugin(t *testing.T) {
	t.Parallel()
	contextName := fmt.Sprintf("%s_magelift_%s", "shop-project", "shop-preview-cluster")
	kubeconfig := kube.BuildStaticTokenKubeconfig(contextName, "https://34.140.218.185", "Y2E=", "ya29.mock-access-token")

	if strings.Contains(kubeconfig, "gke-gcloud-auth-plugin") {
		t.Fatal("kubeconfig must not depend on gke-gcloud-auth-plugin")
	}
	if !strings.Contains(kubeconfig, "token: ya29.mock-access-token") {
		t.Fatalf("kubeconfig missing OAuth token auth: %s", kubeconfig)
	}
	if !strings.Contains(kubeconfig, "certificate-authority-data: Y2E=") {
		t.Fatal("kubeconfig missing cluster CA")
	}
	if !strings.Contains(kubeconfig, "server: https://34.140.218.185") {
		t.Fatal("kubeconfig missing cluster endpoint")
	}
	if !strings.Contains(kubeconfig, "name: shop-project_magelift_shop-preview-cluster") {
		t.Fatal("kubeconfig missing expected context name")
	}
}
