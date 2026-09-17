package kube

import (
	"strings"
	"testing"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clientcmdlatest "k8s.io/client-go/tools/clientcmd/api/latest"
)

func TestRefreshKubeconfigTokenReplacesUserBearer(t *testing.T) {
	t.Parallel()
	original := BuildStaticTokenKubeconfig("ctx", "https://10.0.0.1", "Q0E=", "stale-token")
	refreshed, err := RefreshKubeconfigToken([]byte(original), "fresh-token")
	if err != nil {
		t.Fatal(err)
	}
	obj, _, err := clientcmdlatest.Codec.Decode(refreshed, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	config, ok := obj.(*clientcmdapi.Config)
	if !ok {
		t.Fatalf("decoded as %T", obj)
	}
	for name, info := range config.AuthInfos {
		if info.Token != "fresh-token" {
			t.Fatalf("user %s token = %q", name, info.Token)
		}
	}
	if strings.Contains(string(refreshed), "stale-token") {
		t.Fatal("stale token survived refresh")
	}
	if !strings.Contains(string(refreshed), "https://10.0.0.1") {
		t.Fatal("cluster endpoint lost in refresh")
	}
}

func TestRefreshKubeconfigTokenRefusesBadInput(t *testing.T) {
	t.Parallel()
	if _, err := RefreshKubeconfigToken(nil, "t"); err == nil {
		t.Fatal("empty kubeconfig accepted")
	}
	if _, err := RefreshKubeconfigToken([]byte("apiVersion: v1\nkind: Config\n"), "t"); err == nil {
		t.Fatal("userless kubeconfig accepted")
	}
	original := BuildStaticTokenKubeconfig("ctx", "https://10.0.0.1", "Q0E=", "stale-token")
	if _, err := RefreshKubeconfigToken([]byte(original), ""); err == nil {
		t.Fatal("empty token accepted")
	}
	if _, err := RefreshKubeconfigToken([]byte("{{{nope"), "t"); err == nil {
		t.Fatal("malformed kubeconfig accepted")
	}
}
