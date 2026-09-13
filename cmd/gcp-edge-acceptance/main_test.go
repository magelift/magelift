package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAcceptanceRequestUsesNativeCloudCDNWithoutArmor(t *testing.T) {
	request, probe, err := acceptanceRequest(
		"magelift/gcp/edge/live-test",
		"ml-gcp-edge-live.acourtiol.com",
		"magelift-edge-live-cert",
		"magelift-edge-live-bs",
		"example.com",
		"",
		"developers.google.com",
		false,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.TargetProvider != "gcp" || request.TargetRuntime != "gcp-edge-live-cell" {
		t.Fatalf("target = %#v", request)
	}
	if request.Intent.NativeProvider != "cloud-cdn" || request.Intent.WAFPolicyRef != "" {
		t.Fatalf("intent = %#v", request.Intent)
	}
	if !request.Intent.PurgeOnDeploy || request.Intent.TLSMode != "managed" || request.Intent.DNSMode != "external" {
		t.Fatalf("intent = %#v", request.Intent)
	}
	if request.Configuration["securityPolicy"] != nil {
		t.Fatalf("configuration must not include securityPolicy: %#v", request.Configuration)
	}
	if request.Configuration["backendService"] != "magelift-edge-live-bs" || request.Configuration["sslCertificate"] != "magelift-edge-live-cert" {
		t.Fatalf("configuration = %#v", request.Configuration)
	}
	if probe.hosts[healthRef] != "example.com" {
		t.Fatalf("health hosts = %#v", probe.hosts)
	}
}

func TestAcceptanceRequestAttachesMagentoSafeArmor(t *testing.T) {
	request, _, err := acceptanceRequest(
		"magelift/gcp/edge/live-test",
		"ml-gcp-edge-live.acourtiol.com",
		"magelift-edge-live-cert",
		"magelift-edge-live-bs",
		"example.com",
		"",
		"developers.google.com",
		false,
		"magelift-edge-live-armor",
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Intent.WAFPolicyRef != "waf/magento-safe" {
		t.Fatalf("WAFPolicyRef = %q", request.Intent.WAFPolicyRef)
	}
	if request.Configuration["securityPolicy"] != "magelift-edge-live-armor" {
		t.Fatalf("configuration = %#v", request.Configuration)
	}
}

func TestAcceptanceRequestConfiguresOriginGroup(t *testing.T) {
	request, probe, err := acceptanceRequest(
		"magelift/gcp/edge/live-test",
		"ml-gcp-edge-live.acourtiol.com",
		"magelift-edge-live-cert",
		"magelift-edge-live-bs",
		"www.google.com",
		"magelift-edge-live-bs-sec",
		"developers.google.com",
		true,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if request.Intent.FailoverPolicyRef != "gcp/failover-policy" {
		t.Fatalf("FailoverPolicyRef = %q", request.Intent.FailoverPolicyRef)
	}
	group, ok := request.Configuration["originGroup"].(map[string]any)
	if !ok || group["primary"] != "magelift-edge-live-bs" || group["secondary"] != "magelift-edge-live-bs-sec" {
		t.Fatalf("originGroup = %#v", request.Configuration["originGroup"])
	}
	if group["primaryHealthRef"] != healthRef || group["secondaryHealthRef"] != healthRefSecondary {
		t.Fatalf("originGroup health = %#v", group)
	}
	if probe.hosts[healthRef] != "www.google.com" || probe.hosts[healthRefSecondary] != "developers.google.com" {
		t.Fatalf("health hosts = %#v", probe.hosts)
	}
}

func TestHTTPSOriginHealthProbeRequires2xxOr3xx(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != "MageLift-origin-health/1" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	probe := &httpsOriginHealthProbe{hosts: map[string]string{healthRef: strings.TrimPrefix(server.URL, "https://")}}
	probe.client = server.Client()
	if err := probe.VerifyOrigin(context.Background(), healthRef); err != nil {
		t.Fatalf("healthy origin: %v", err)
	}

	bad := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(bad.Close)
	probe.hosts[healthRef] = strings.TrimPrefix(bad.URL, "https://")
	probe.client = bad.Client()
	if err := probe.VerifyOrigin(context.Background(), healthRef); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("unhealthy origin error = %v", err)
	}
}

func TestRunRejectsMissingProject(t *testing.T) {
	err := run(context.Background(), []string{
		"--marker", "magelift/gcp/edge/test",
		"--domain", "ml-gcp-edge-test.acourtiol.com",
		"--ssl-certificate", "magelift-edge-test-cert",
		"--backend-service", "magelift-edge-test-bs",
	}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "project") {
		t.Fatalf("error = %v", err)
	}
}

func TestEdgeOperationPolicyIsBoundedForLiveControlPlane(t *testing.T) {
	policy := edgeOperationPolicy()
	if policy.Timeout < 30*time.Minute || policy.PollInterval <= 0 || policy.MaxAttempts <= 0 {
		t.Fatalf("policy = %#v", policy)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
