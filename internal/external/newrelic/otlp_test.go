package newrelic

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	provider "github.com/magelift/magelift/internal/provider"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
)

func TestClientExportsOTLPProtobufWithScopedCredential(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if attempts.Add(1) != 1 {
			t.Fatalf("attempt count before successful request = %d", attempts.Load())
		}
		if request.URL.Path != "/v1/metrics" || request.Header.Get("Content-Type") != "application/x-protobuf" || request.Header.Get("api-key") != "license-key" {
			t.Fatalf("request = method %s path %s content-type %q api-key %q", request.Method, request.URL.Path, request.Header.Get("Content-Type"), request.Header.Get("api-key"))
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	resolver := provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
		if reference != "newrelic://license" {
			t.Fatalf("credential reference = %q", reference)
		}
		return consume([]byte("license-key"))
	})
	client, err := NewClientWithRetry(server.Client(), resolver, RetryPolicy{MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Export(context.Background(), ExportRequest{
		Endpoint:        server.URL,
		CredentialRef:   "newrelic://license",
		Signal:          SignalMetrics,
		Payload:         &metricsv1.ExportMetricsServiceRequest{},
		OwnershipMarker: "magelift/test/newrelic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Accepted || result.StatusCode != http.StatusOK || result.OperationID == "" || result.OwnershipMarker == "" {
		t.Fatalf("export result = %#v", result)
	}
}

func TestClientRetriesTransientStatusAndHonorsContext(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	resolver := provider.CredentialResolverFunc(func(_ context.Context, _ string, consume func([]byte) error) error { return consume([]byte("key")) })
	client, err := NewClientWithRetry(server.Client(), resolver, RetryPolicy{MaxAttempts: 2, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Export(context.Background(), ExportRequest{Endpoint: server.URL, CredentialRef: "newrelic://license", Signal: SignalLogs, Payload: &metricsv1.ExportMetricsServiceRequest{}, OwnershipMarker: "magelift/test/retry"})
	if err != nil || !result.Accepted || attempts.Load() != 2 {
		t.Fatalf("retry result = %#v, attempts=%d, err=%v", result, attempts.Load(), err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Export(ctx, ExportRequest{Endpoint: server.URL, CredentialRef: "newrelic://license", Signal: SignalLogs, Payload: &metricsv1.ExportMetricsServiceRequest{}, OwnershipMarker: "magelift/test/cancel"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled export error = %v", err)
	}
}

func TestClientNeverLeaksCredentialValueAndRejectsUnsafeEndpoints(t *testing.T) {
	secret := "license-key=super-secret-value"
	resolver := provider.CredentialResolverFunc(func(context.Context, string, func([]byte) error) error { return errors.New(secret) })
	client, err := NewClient(nil, resolver)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Export(context.Background(), ExportRequest{Endpoint: "https://otlp.nr-data.net", CredentialRef: "newrelic://license", Signal: SignalTraces, Payload: &metricsv1.ExportMetricsServiceRequest{}, OwnershipMarker: "magelift/test/redaction"})
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("credential error = %v", err)
	}
	_, err = client.Export(context.Background(), ExportRequest{Endpoint: "http://127.0.0.1:4318", CredentialRef: "newrelic://license", Signal: SignalTraces, Payload: &metricsv1.ExportMetricsServiceRequest{}, OwnershipMarker: "magelift/test/insecure"})
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("unsafe endpoint error = %v", err)
	}
}

func TestClientRejectsSignalSpecificEndpointForAnotherSignal(t *testing.T) {
	resolver := provider.CredentialResolverFunc(func(context.Context, string, func([]byte) error) error {
		return errors.New("resolver must not run")
	})
	client, err := NewClient(nil, resolver)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Export(context.Background(), ExportRequest{
		Endpoint:        "https://otlp.nr-data.net/v1/metrics",
		CredentialRef:   "newrelic://license",
		Signal:          SignalTraces,
		Payload:         &metricsv1.ExportMetricsServiceRequest{},
		OwnershipMarker: "magelift/test/endpoint",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match signal") {
		t.Fatalf("mismatched signal endpoint error = %v", err)
	}
}
