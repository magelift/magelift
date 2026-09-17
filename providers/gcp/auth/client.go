package auth

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/platform"
	"golang.org/x/oauth2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// NewClientFactory builds Kubernetes clients with per-request fresh bearer
// tokens. Cluster endpoint and CA come from the stack kubeconfig; the stored
// bearer is never used. A MAGELIFT_KUBECONFIG override still delegates to
// the standard loader (operator-owned, gcloud-refreshed credentials).
func NewClientFactory() kube.ClientFactory {
	return NewClientFactoryWithSource(func(ctx context.Context) (oauth2.TokenSource, error) {
		return DefaultTokenSource(ctx, CloudPlatformScope)
	})
}

// NewClientFactoryWithSource builds the factory with an injectable token
// source constructor. Tests supply static or failing sources.
func NewClientFactoryWithSource(newSource func(context.Context) (oauth2.TokenSource, error)) kube.ClientFactory {
	return func(outputs map[string]any) (kubernetes.Interface, error) {
		if newSource == nil {
			return nil, fmt.Errorf("GCP token source constructor is required")
		}
		// Operator escape hatch: a supplied kubeconfig (typically
		// gcloud-generated with its own auth plugin) loads as-is.
		if strings.TrimSpace(os.Getenv("MAGELIFT_KUBECONFIG")) != "" {
			return kube.ClientFromOutputs(outputs)
		}
		text, err := platform.RequireStringOutput(outputs, platform.OutputKubeconfig)
		if err != nil {
			return nil, err
		}
		restConfig, err := clientcmd.RESTConfigFromKubeConfig([]byte(text))
		if err != nil {
			return nil, fmt.Errorf("parse cluster kubeconfig: %w", err)
		}
		restConfig.BearerToken = ""
		restConfig.BearerTokenFile = ""
		source, err := newSource(context.Background())
		if err != nil {
			return nil, err
		}
		cached := oauth2.ReuseTokenSource(nil, source)
		previous := restConfig.WrapTransport
		restConfig.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
			if previous != nil {
				rt = previous(rt)
			}
			return &bearerTransport{source: cached, next: rt}
		}
		client, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			return nil, fmt.Errorf("build kubernetes client: %w", err)
		}
		return client, nil
	}
}

// bearerTransport injects a fresh bearer token per request. Expiry
// self-heals through the cached source; refresh failures fail the
// request with the raw cause.
type bearerTransport struct {
	source oauth2.TokenSource
	next   http.RoundTripper
}

func (t *bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t == nil || t.source == nil {
		return nil, fmt.Errorf("GCP token source is required")
	}
	token, err := t.source.Token()
	if err != nil {
		return nil, err
	}
	out := request.Clone(request.Context())
	out.Header.Set("Authorization", "Bearer "+token.AccessToken)
	next := t.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(out)
}
