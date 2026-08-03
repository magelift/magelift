package kube

import (
	"bytes"
	"fmt"

	"github.com/magelift/magelift/internal/platform"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientFromKubeconfig builds a kubernetes.Interface from kubeconfig bytes.
// Parse and client construction are offline; no API calls are made until the
// returned client is used. Empty or unparseable kubeconfig fails closed.
func ClientFromKubeconfig(kubeconfig []byte) (kubernetes.Interface, error) {
	if len(bytes.TrimSpace(kubeconfig)) == 0 {
		return nil, fmt.Errorf("kubeconfig is empty")
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("parse kubeconfig: %w", err)
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
func ClientFromOutputs(outputs map[string]any) (kubernetes.Interface, error) {
	text, err := platform.RequireStringOutput(outputs, platform.OutputKubeconfig)
	if err != nil {
		return nil, err
	}
	return ClientFromKubeconfig([]byte(text))
}
