package main

import (
	"strings"
	"testing"
	"time"
)

const testCollectorImage = "public.ecr.aws/otel/collector@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestParseOptionsAcceptsExplicitValues(t *testing.T) {
	t.Setenv("MAGELIFT_AWS_COLLECTOR_QUERY_KEY", "query-key-value")
	parsed, err := parseOptions([]string{
		"--region", "eu-west-3", "--cluster", "collector-cluster", "--service", "collector-service",
		"--credential-ref", "aws-secrets-manager://magelift/collector", "--secret-arn", "arn:aws:secretsmanager:eu-west-3:123456789012:secret:magelift-collector",
		"--endpoint", "https://otlp.eu01.nr-data.net", "--nerdgraph-endpoint", "https://api.eu.newrelic.com/graphql", "--account-id", "8368691",
		"--image-digest", testCollectorImage, "--marker", "magelift/aws/collector/test", "--signals", "logs,traces", "--timeout", "2m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.region != "eu-west-3" || parsed.cluster != "collector-cluster" || parsed.service != "collector-service" || parsed.accountID != 8368691 {
		t.Fatalf("parsed options = %#v", parsed)
	}
	if strings.Join(parsed.signals, ",") != "logs,traces" || parsed.timeout != 2*time.Minute {
		t.Fatalf("parsed signals or timeout = %#v", parsed)
	}
}

func TestParseOptionsRejectsUnsafeOrMutableValues(t *testing.T) {
	t.Setenv("MAGELIFT_AWS_COLLECTOR_QUERY_KEY", "query-key-value")
	base := []string{
		"--region", "eu-west-3", "--cluster", "collector-cluster", "--service", "collector-service",
		"--credential-ref", "aws-secrets-manager://magelift/collector", "--secret-arn", "arn:aws:secretsmanager:eu-west-3:123456789012:secret:magelift-collector",
		"--endpoint", "https://otlp.eu01.nr-data.net", "--nerdgraph-endpoint", "https://api.eu.newrelic.com/graphql", "--account-id", "8368691",
		"--image-digest", testCollectorImage, "--marker", "magelift/aws/collector/test", "--signals", "logs",
	}
	tests := []struct {
		name  string
		flags []string
	}{
		{name: "mutable image", flags: []string{"--image-digest", "public.ecr.aws/otel/collector:latest"}},
		{name: "secret value", flags: []string{"--credential-ref", "aws-secrets-manager://plain secret"}},
		{name: "http endpoint", flags: []string{"--endpoint", "http://otlp.eu01.nr-data.net"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := append([]string(nil), base...)
			args = append(args, test.flags...)
			if _, err := parseOptions(args); err == nil {
				t.Fatalf("parseOptions accepted %s", test.name)
			}
		})
	}
}

func TestParseSignalsRejectsDuplicatesAndUnknownValues(t *testing.T) {
	if _, err := parseSignals("logs,logs"); err == nil {
		t.Fatal("duplicate signal accepted")
	}
	if _, err := parseSignals("events"); err == nil {
		t.Fatal("unknown signal accepted")
	}
	got, err := parseSignals("traces, metrics")
	if err != nil || strings.Join(got, ",") != "traces,metrics" {
		t.Fatalf("parseSignals = %#v, %v", got, err)
	}
}

func TestCollectorRequestUsesOpaqueSecretReference(t *testing.T) {
	parsed := options{region: "eu-west-3", cluster: "collector-cluster", service: "collector-service", secretReference: "aws-secrets-manager://magelift/collector", endpoint: "https://otlp.eu01.nr-data.net", marker: "magelift/aws/collector/test", signals: []string{"logs"}}
	request := collectorRequest(parsed)
	if request.CredentialRef != parsed.secretReference || request.NativeReference != "collector-cluster/collector-service" {
		t.Fatalf("collector request = %#v", request)
	}
	if strings.Contains(request.CredentialRef, "secret-value") {
		t.Fatal("collector request contains a secret value")
	}
}
