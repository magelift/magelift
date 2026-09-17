package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/magelift/magelift/internal/cloud/kube"
	"golang.org/x/oauth2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func recordingAPIServer(t *testing.T, seen *atomic.Value) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"kind":"NamespaceList","apiVersion":"v1","metadata":{},"items":[]}`))
	}))
}

func factoryOutputs(serverURL string) map[string]any {
	return map[string]any{
		"clusterName": "shop-cluster",
		"kubeconfig":  kube.BuildStaticTokenKubeconfig("ctx", serverURL, "Q0E=", "stale-token"),
	}
}

func TestClientFactorySendsFreshBearerNeverStored(t *testing.T) {
	t.Parallel()
	var seen atomic.Value
	server := recordingAPIServer(t, &seen)
	t.Cleanup(server.Close)

	factory := NewClientFactoryWithSource(func(context.Context) (oauth2.TokenSource, error) {
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "fresh-token"}), nil
	})
	client, err := factory(factoryOutputs(server.URL))
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	if _, err := client.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if got, _ := seen.Load().(string); got != "Bearer fresh-token" {
		t.Fatalf("authorization = %q", got)
	}
}

func TestClientFactorySurfacesRefreshFailure(t *testing.T) {
	t.Parallel()
	var seen atomic.Value
	server := recordingAPIServer(t, &seen)
	t.Cleanup(server.Close)

	factory := NewClientFactoryWithSource(func(context.Context) (oauth2.TokenSource, error) {
		return nil, errors.New("refresh exploded")
	})
	if _, err := factory(factoryOutputs(server.URL)); err == nil || err.Error() != "refresh exploded" {
		t.Fatalf("err = %v", err)
	}
}

func TestClientFactoryRebuildsSourcePerCall(t *testing.T) {
	t.Parallel()
	var builds atomic.Int32
	factory := NewClientFactoryWithSource(func(context.Context) (oauth2.TokenSource, error) {
		builds.Add(1)
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"}), nil
	})
	outputs := factoryOutputs("http://127.0.0.1:1")
	if _, err := factory(outputs); err != nil {
		t.Fatal(err)
	}
	if _, err := factory(outputs); err != nil {
		t.Fatal(err)
	}
	if got := builds.Load(); got != 2 {
		t.Fatalf("source constructions = %d, want 2 (stateless restart)", got)
	}
}

func TestClientFactoryRequiresKubeconfig(t *testing.T) {
	t.Parallel()
	factory := NewClientFactoryWithSource(func(context.Context) (oauth2.TokenSource, error) {
		return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "t"}), nil
	})
	if _, err := factory(map[string]any{}); err == nil {
		t.Fatal("missing kubeconfig accepted")
	}
}
