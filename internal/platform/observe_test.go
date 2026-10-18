package platform

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
)

func TestValidateFirstPartyObservability(t *testing.T) {
	tests := []struct {
		name             string
		nativeProvider   string
		externalProvider string
		target           string
		logs             bool
		metrics          bool
		traces           bool
		wantError        string
	}{
		{name: "disabled"},
		{name: "AWS logs", nativeProvider: "cloudwatch", target: "aws", logs: true},
		{name: "GCP logs", nativeProvider: "google-cloud-operations", target: "gcp", logs: true},
		{name: "Scaleway Cockpit", nativeProvider: "scaleway-cockpit", target: "scaleway", logs: true, metrics: true},
		{name: "OVH Logs Data Platform", nativeProvider: "ovh-logs-data-platform", target: "ovh", logs: true},
		{name: "wrong cloud", nativeProvider: "cloudwatch", target: "gcp", logs: true, wantError: "target.provider aws"},
		{name: "extension provider is validated by its adapter", externalProvider: "datadog", target: "aws", logs: true},
		{name: "native and independent providers", nativeProvider: "cloudwatch", externalProvider: "newrelic", target: "aws", logs: true, metrics: true, traces: true},
		{name: "native provider accepts all declared signals", nativeProvider: "cloudwatch", target: "aws", logs: true, metrics: true, traces: true},
		{name: "X-Ray typed unavailable", nativeProvider: "xray", target: "aws", traces: true, wantError: "unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateFirstPartyObservability(config.Config{
				Target: config.Target{Provider: test.target},
				Observability: config.ObservabilityConfig{
					NativeProvider:   test.nativeProvider,
					ExternalProvider: test.externalProvider,
					Logs:             test.logs,
					Metrics:          test.metrics,
					Traces:           test.traces,
				},
			})
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

func TestRedactLogMessage(t *testing.T) {
	input := `password="dont-log" authorization: Bearer abc.def.ghi user=magento`
	got := RedactLogMessage(input)
	if strings.Contains(got, "dont-log") || strings.Contains(got, "abc.def.ghi") {
		t.Fatalf("secret value leaked: %q", got)
	}
	if !strings.Contains(got, "user=magento") {
		t.Fatalf("non-secret value was removed: %q", got)
	}
}

func TestValidateFirstPartyEdgeAcrossOriginProviders(t *testing.T) {
	tests := []struct {
		name           string
		target         string
		nativeProvider string
		external       string
		wantError      string
	}{
		{name: "disabled"},
		{name: "AWS CloudFront", target: "aws", nativeProvider: "cloudfront"},
		{name: "AWS CloudFront WAF", target: "aws", nativeProvider: "cloudfront-waf"},
		{name: "GCP load balancing", target: "gcp", nativeProvider: "google-cloud-load-balancing"},
		{name: "GCP CDN", target: "gcp", nativeProvider: "cloud-cdn"},
		{name: "GCP Armor", target: "gcp", nativeProvider: "cloud-armor"},
		{name: "Scaleway Edge Services", target: "scaleway", nativeProvider: "scaleway-edge-services"},
		{name: "Scaleway load balancer", target: "scaleway", nativeProvider: "scaleway-load-balancer"},
		{name: "OVH CDN", target: "ovh", nativeProvider: "ovh-cdn"},
		{name: "OVH load balancer", target: "ovh", nativeProvider: "ovh-public-cloud-load-balancer"},
		{name: "Fastly composition", target: "aws", external: "fastly"},
		{name: "wrong AWS provider", target: "aws", nativeProvider: "cloud-cdn", wantError: "target.provider \"aws\""},
		{name: "wrong GCP provider", target: "gcp", nativeProvider: "cloudfront", wantError: "target.provider \"gcp\""},
		{name: "wrong Scaleway provider", target: "scaleway", nativeProvider: "ovh-cdn", wantError: "target.provider \"scaleway\""},
		{name: "wrong OVH provider", target: "ovh", nativeProvider: "cloudfront", wantError: "target.provider \"ovh\""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateFirstPartyEdge(config.Config{
				Target: config.Target{Provider: test.target},
				Edge:   config.EdgeConfig{NativeProvider: test.nativeProvider, ExternalProvider: test.external},
			})
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}
