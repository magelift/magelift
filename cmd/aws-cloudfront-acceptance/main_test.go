package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/edge/waf"
)

func TestAcceptanceRequestUsesNativeCloudFrontWithoutWAF(t *testing.T) {
	request, probe, err := acceptanceRequest("magelift/aws/cloudfront/live-test", "ml-cf-live.acourtiol.com", "arn:aws:acm:us-east-1:123456789012:certificate/cert-1", "example.com", "www.example.com", true, "")
	if err != nil {
		t.Fatal(err)
	}
	if request.TargetProvider != "aws" || request.TargetRuntime != "cloudfront-live-cell" {
		t.Fatalf("target = %#v", request)
	}
	if request.Intent.NativeProvider != "cloudfront" || request.Intent.WAFPolicyRef != "" {
		t.Fatalf("intent = %#v", request.Intent)
	}
	if !request.Intent.PurgeOnDeploy || request.Intent.TLSMode != "managed" || request.Intent.DNSMode != "external" {
		t.Fatalf("intent = %#v", request.Intent)
	}
	if request.Configuration["webACLID"] != nil {
		t.Fatalf("configuration must not include webACLID: %#v", request.Configuration)
	}
	if probe.hosts[healthRefPrimary] != "example.com" || probe.hosts[healthRefSecondary] != "www.example.com" {
		t.Fatalf("health hosts = %#v", probe.hosts)
	}
}

func TestAcceptanceRequestSingleOriginOmitsOriginGroup(t *testing.T) {
	request, probe, err := acceptanceRequest("magelift/aws/cloudfront/single", "ml-cf-single.acourtiol.com", "arn:aws:acm:us-east-1:123456789012:certificate/cert-1", "example.com", "www.example.com", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if request.Intent.FailoverPolicyRef != "" || request.Configuration["originGroup"] != nil {
		t.Fatalf("single-origin request = %#v", request)
	}
	if len(probe.hosts) != 1 {
		t.Fatalf("health hosts = %#v", probe.hosts)
	}
}

func TestAcceptanceRequestAttachesCloudFrontScopeWAF(t *testing.T) {
	webACL := "arn:aws:wafv2:us-east-1:123456789012:global/webacl/owned/acl-1"
	request, _, err := acceptanceRequest("magelift/aws/cloudfront/waf", "ml-cf-waf.acourtiol.com", "arn:aws:acm:us-east-1:123456789012:certificate/cert-1", "example.com", "www.example.com", true, webACL)
	if err != nil {
		t.Fatal(err)
	}
	if request.Intent.WAFPolicyRef != waf.PolicyRef || request.Configuration["webACLID"] != webACL {
		t.Fatalf("waf request = %#v", request)
	}
}

func TestAcceptanceRequestRejectsRegionalWAFARN(t *testing.T) {
	_, _, err := acceptanceRequest("magelift/aws/cloudfront/waf", "ml-cf-waf.acourtiol.com", "arn:aws:acm:us-east-1:123456789012:certificate/cert-1", "example.com", "www.example.com", true, "arn:aws:wafv2:eu-west-1:123456789012:regional/webacl/owned/acl-1")
	if err == nil || !strings.Contains(err.Error(), "CLOUDFRONT-scope") {
		t.Fatalf("error = %v", err)
	}
}

func TestHTTPSOriginHealthProbeRequires2xxOr3xx(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	probe := &httpsOriginHealthProbe{hosts: map[string]string{healthRefPrimary: strings.TrimPrefix(server.URL, "https://")}}
	probe.client = server.Client()
	if err := probe.VerifyOrigin(context.Background(), healthRefPrimary); err != nil {
		t.Fatalf("healthy origin: %v", err)
	}

	bad := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(bad.Close)
	probe.hosts[healthRefPrimary] = strings.TrimPrefix(bad.URL, "https://")
	probe.client = bad.Client()
	if err := probe.VerifyOrigin(context.Background(), healthRefPrimary); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("unhealthy origin error = %v", err)
	}
}

func TestRunRejectsDestroyWithoutStatePath(t *testing.T) {
	err := run(context.Background(), []string{
		"--profile", "default",
		"--marker", "magelift/aws/cloudfront/test",
		"--domain", "ml-cf-test.acourtiol.com",
		"--certificate-arn", "arn:aws:acm:us-east-1:123456789012:certificate/cert-1",
		"--phase", "destroy",
	}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "state-path") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsFailoverWithoutStatePath(t *testing.T) {
	err := run(context.Background(), []string{
		"--profile", "default",
		"--marker", "magelift/aws/cloudfront/test",
		"--domain", "ml-cf-test.acourtiol.com",
		"--certificate-arn", "arn:aws:acm:us-east-1:123456789012:certificate/cert-1",
		"--phase", "failover",
	}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "state-path") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunRejectsInvalidCertificateRegion(t *testing.T) {
	err := run(context.Background(), []string{
		"--profile", "default",
		"--marker", "magelift/aws/cloudfront/test",
		"--domain", "ml-cf-test.acourtiol.com",
		"--certificate-arn", "arn:aws:acm:eu-west-3:123456789012:certificate/cert-1",
	}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "us-east-1") {
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
