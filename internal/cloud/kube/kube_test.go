package kube

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

func TestSkipAwaitAnnotations(t *testing.T) {
	t.Parallel()
	annotations := SkipAwaitAnnotations()
	if len(annotations) != 1 {
		t.Fatalf("expected exactly one annotation, got %d", len(annotations))
	}
	if _, ok := annotations["pulumi.com/skipAwait"]; !ok {
		t.Fatal("missing pulumi.com/skipAwait annotation")
	}
}

func TestToStringArray(t *testing.T) {
	t.Parallel()
	if got := ToStringArray(nil); len(got) != 0 {
		t.Fatalf("nil input should yield empty array, got %d", len(got))
	}
	if got := ToStringArray([]string{"bin/magento", "cron:run"}); len(got) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(got))
	}
}

func TestEnvVars(t *testing.T) {
	t.Parallel()
	bindings := []platform.EnvBinding{
		{Name: "MAGE_MODE", Value: "production"},
		{Name: "DB_HOST", Value: "db.internal"},
	}
	env := EnvVars(bindings)
	if len(env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(env))
	}
	if env[0].Name != "MAGE_MODE" || env[0].Value == nil || *env[0].Value != "production" {
		t.Fatalf("first env var mismatch: %+v", env[0])
	}
	// Distinct backing addresses guard against the loop-variable aliasing bug.
	if env[1].Value == nil || *env[1].Value != "db.internal" {
		t.Fatalf("second env var mismatch: %+v", env[1])
	}
	if env[0].Value == env[1].Value {
		t.Fatal("env var values must not share a backing pointer")
	}
}

func TestBuildStaticTokenKubeconfigStaticToken(t *testing.T) {
	t.Parallel()
	kubeconfig := BuildStaticTokenKubeconfig("magelift_shop-cluster", "https://1.2.3.4:6443", "Y2E=", "mock-token")

	if strings.Contains(kubeconfig, "exec:") {
		t.Fatal("kubeconfig must not depend on an exec auth plugin")
	}
	if !strings.Contains(kubeconfig, "token: mock-token") {
		t.Fatalf("kubeconfig missing static token auth: %s", kubeconfig)
	}
	if !strings.Contains(kubeconfig, "certificate-authority-data: Y2E=") {
		t.Fatal("kubeconfig missing cluster CA")
	}
	if !strings.Contains(kubeconfig, "server: https://1.2.3.4:6443") {
		t.Fatal("kubeconfig missing cluster endpoint")
	}
	if !strings.Contains(kubeconfig, "name: magelift_shop-cluster") {
		t.Fatal("kubeconfig missing expected context name")
	}
}
