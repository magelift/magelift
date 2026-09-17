package kube

import (
	"bytes"
	"context"
	"fmt"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
)

// TokenSource returns a fresh bearer token for Kubernetes API access.
// Providers wire ambient-credential refreshers; core callers leave it nil
// and keep static kubeconfig behavior.
type TokenSource func(ctx context.Context) (string, error)

// RefreshKubeconfigToken returns the kubeconfig with every user token
// replaced by a fresh one. kubectl targets written this way authenticate
// as the fresh identity without secrets on argv.
func RefreshKubeconfigToken(kubeconfig []byte, token string) ([]byte, error) {
	if len(kubeconfig) == 0 {
		return nil, fmt.Errorf("kubeconfig is required")
	}
	if token == "" {
		return nil, fmt.Errorf("fresh token is required")
	}
	obj, _, err := clientcmdlatest.Codec.Decode(kubeconfig, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("decode kubeconfig: %w", err)
	}
	config, ok := obj.(*clientcmdapi.Config)
	if !ok {
		return nil, fmt.Errorf("kubeconfig decoded as %T", obj)
	}
	if len(config.AuthInfos) == 0 {
		return nil, fmt.Errorf("kubeconfig has no users")
	}
	for name, info := range config.AuthInfos {
		info.Token = token
		info.TokenFile = ""
		config.AuthInfos[name] = info
	}
	var encoded bytes.Buffer
	if err := clientcmdlatest.Codec.Encode(config, &encoded); err != nil {
		return nil, fmt.Errorf("encode kubeconfig: %w", err)
	}
	return encoded.Bytes(), nil
}
