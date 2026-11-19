package main

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const testImageDigest = "ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-releases@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParseOptionsAcceptsExplicitCollectorConfiguration(t *testing.T) {
	t.Setenv("MAGELIFT_GCP_COLLECTOR_LICENSE_KEY", "license-value-not-printed")
	t.Setenv("MAGELIFT_GCP_COLLECTOR_QUERY_KEY", "query-value-not-printed")
	parsed, err := parseOptions([]string{
		"--project=digital-lab-341608",
		"--region=europe-west1",
		"--cluster=magelift-gke",
		"--runtime=gke-standard",
		"--namespace=magelift",
		"--endpoint=https://otlp.eu01.nr-data.net",
		"--nerdgraph-endpoint=https://api.newrelic.com/graphql",
		"--account-id=8368691",
		"--image-digest=" + testImageDigest,
		"--marker=magelift/gcp/collector/test",
		"--signals=traces,logs,metrics",
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.runtime != "gke-standard" || strings.Join(parsed.signals, ",") != "traces,logs,metrics" {
		t.Fatalf("parsed options = %#v", parsed)
	}
	if len(parsed.licenseKey) == 0 || len(parsed.queryKey) == 0 {
		t.Fatal("credential inputs were not loaded for the provider-owned Secret/query boundaries")
	}
}

func TestParseOptionsRejectsUnsafeOrUnsupportedValues(t *testing.T) {
	t.Setenv("MAGELIFT_GCP_COLLECTOR_LICENSE_KEY", "license")
	t.Setenv("MAGELIFT_GCP_COLLECTOR_QUERY_KEY", "query")
	base := []string{
		"--project=digital-lab-341608",
		"--region=europe-west1",
		"--cluster=magelift-gke",
		"--endpoint=https://otlp.eu01.nr-data.net",
		"--nerdgraph-endpoint=https://api.newrelic.com/graphql",
		"--account-id=8368691",
		"--image-digest=" + testImageDigest,
		"--marker=magelift/gcp/collector/test",
	}
	tests := []struct {
		name string
		arg  string
	}{
		{name: "mutable image", arg: "--image-digest=otel/opentelemetry-collector-contrib:latest"},
		{name: "http endpoint", arg: "--endpoint=http://otlp.eu01.nr-data.net"},
		{name: "unsupported signal", arg: "--signals=events"},
		{name: "duplicate signal", arg: "--signals=logs,logs"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string(nil), base...)
			args = append(args, test.arg)
			if _, err := parseOptions(args); err == nil {
				t.Fatal("unsafe or unsupported option was accepted")
			}
		})
	}
}

func TestCollectorRequestKeepsSecretAsKubernetesReference(t *testing.T) {
	parsed := options{
		project: "digital-lab-341608", region: "europe-west1", cluster: "magelift-gke",
		runtime: "gke-autopilot", namespace: "magelift", endpoint: "https://otlp.eu01.nr-data.net",
		marker: "magelift/gcp/collector/test", signals: []string{"logs"}, imageDigest: testImageDigest,
	}
	request := collectorRequest(parsed, collectorSecretName(parsed.marker))
	if !strings.HasPrefix(request.CredentialRef, "kubernetes-secret://magelift/") || !strings.HasSuffix(request.CredentialRef, "#license-key") {
		t.Fatalf("credential reference = %q", request.CredentialRef)
	}
	if strings.Contains(request.CredentialRef, "license-value") || strings.Contains(request.CredentialRef, "query-value") {
		t.Fatalf("credential reference contains secret material: %q", request.CredentialRef)
	}
}

func TestCollectorSecretLifecycleIsOwnershipScoped(t *testing.T) {
	client := fake.NewClientset()
	ctx := context.Background()
	marker := "magelift/gcp/collector/secret-test"
	name := collectorSecretName(marker)
	if err := ensureCollectorSecret(ctx, client, "magelift", name, marker, []byte("license")); err != nil {
		t.Fatal(err)
	}
	secret, err := client.CoreV1().Secrets("magelift").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if secret.Annotations[secretOwnershipKey] != marker || string(secret.Data[secretKey]) != "license" {
		t.Fatalf("secret metadata/data = %#v", secret)
	}
	if err := ensureCollectorSecret(ctx, client, "magelift", name, marker, []byte("rotated")); err != nil {
		t.Fatal(err)
	}
	secret, err = client.CoreV1().Secrets("magelift").Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if string(secret.Data[secretKey]) != "rotated" {
		t.Fatalf("rotated secret data = %q", secret.Data[secretKey])
	}
	if err := deleteCollectorSecret(ctx, client, "magelift", name, marker); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().Secrets("magelift").Get(ctx, name, metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("secret after cleanup err = %v", err)
	}
}

func TestCollectorSecretCleanupRefusesOwnershipDrift(t *testing.T) {
	client := fake.NewClientset()
	_, err := client.CoreV1().Secrets("magelift").Create(context.Background(), &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "foreign", Namespace: "magelift", Annotations: map[string]string{secretOwnershipKey: "other-marker"}},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteCollectorSecret(context.Background(), client, "magelift", "foreign", "magelift/gcp/collector/test"); err == nil {
		t.Fatal("cleanup deleted or accepted a foreign Secret")
	}
}
