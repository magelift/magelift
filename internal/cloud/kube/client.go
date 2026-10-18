package kube

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// RESTConfigFromKubeconfig parses kubeconfig bytes into the client-go REST
// configuration used by both typed API clients and remotecommand. It performs
// no network calls and never logs the configuration contents.
func RESTConfigFromKubeconfig(kubeconfig []byte) (*rest.Config, error) {
	if len(bytes.TrimSpace(kubeconfig)) == 0 {
		return nil, fmt.Errorf("kubeconfig is empty")
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
	}
	return cfg, nil
}

// ClientFromKubeconfig builds a kubernetes.Interface from kubeconfig bytes.
// Parse and client construction are offline; no API calls are made until the
// returned client is used. Empty or unparseable kubeconfig fails closed.
func ClientFromKubeconfig(kubeconfig []byte) (kubernetes.Interface, error) {
	cfg, err := RESTConfigFromKubeconfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes clientset: %w", err)
	}
	return clientset, nil
}

// ClientFromOutputs reads platform.OutputKubeconfig from stack outputs and
// builds a kubernetes.Interface. Missing or empty keys fail closed via
// RequireStringOutput.
// MAGELIFT_KUBECONFIG, when set, is a live kubeconfig path (gcloud
// get-credentials). Stack kubeconfig is a static OAuth token that expires in
// about an hour; migrate Jobs and exec must not use that expired token.
func ClientFromOutputs(outputs map[string]any) (kubernetes.Interface, error) {
	if override, err := kubeconfigOverrideFromEnv(); err != nil {
		return nil, err
	} else if len(override) > 0 {
		if err := kubeconfigMustMatchCluster(override, outputs); err != nil {
			return nil, err
		}
		return ClientFromKubeconfig(override)
	}
	text, err := platform.RequireStringOutput(outputs, platform.OutputKubeconfig)
	if err != nil {
		return nil, err
	}
	return ClientFromKubeconfig([]byte(text))
}

// kubeconfigMustMatchCluster rejects MAGELIFT_KUBECONFIG from another cell.
// GCP can reuse a GKE public endpoint IP; the stale CA then fails TLS as
// x509 unknown authority instead of a cluster-mismatch error.
func kubeconfigMustMatchCluster(kubeconfig []byte, outputs map[string]any) error {
	cluster, err := platform.RequireStringOutput(outputs, platform.OutputClusterName)
	if err != nil {
		return nil
	}
	if !bytes.Contains(kubeconfig, []byte(cluster)) {
		return fmt.Errorf("MAGELIFT_KUBECONFIG is not for stack cluster %s", cluster)
	}
	return nil
}

func kubeconfigOverrideFromEnv() ([]byte, error) {
	path := strings.TrimSpace(os.Getenv("MAGELIFT_KUBECONFIG"))
	if path == "" {
		return nil, nil
	}
	if strings.ContainsAny(path, "\r\n\x00") {
		return nil, fmt.Errorf("MAGELIFT_KUBECONFIG must be a single-line path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read MAGELIFT_KUBECONFIG: %w", err)
	}
	return data, nil
}
