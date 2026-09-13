package newrelic

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/cloud/kube"
	"google.golang.org/protobuf/encoding/protojson"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

func TestBuildOTLPProbePayload(t *testing.T) {
	tests := []struct {
		name   string
		signal string
		want   Signal
	}{
		{name: "logs", signal: "logs", want: SignalLogs},
		{name: "metrics", signal: "metrics", want: SignalMetrics},
		{name: "traces", signal: "traces", want: SignalTraces},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload, signal, err := BuildOTLPProbePayload(test.signal, "magelift/test/collector", time.Unix(123, 456))
			if err != nil {
				t.Fatal(err)
			}
			if signal != test.want || payload == nil {
				t.Fatalf("payload signal=%q payload=%T, want %q and a payload", signal, payload, test.want)
			}
			encoded, err := protojson.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(encoded), "magelift/test/collector") || !strings.Contains(string(encoded), test.signal) {
				t.Fatalf("probe payload=%s", encoded)
			}
		})
	}
}

func TestBuildOTLPProbePayloadRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		signal string
		marker string
	}{
		{name: "unsupported signal", signal: "events", marker: "magelift/test/collector"},
		{name: "empty marker", signal: "metrics", marker: ""},
		{name: "multiline marker", signal: "metrics", marker: "magelift/test\ncollector"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := BuildOTLPProbePayload(test.signal, test.marker, time.Unix(123, 456)); err == nil {
				t.Fatal("invalid probe input was accepted")
			}
		})
	}
}

func TestNewKubernetesCollectorSignalProbeValidatesDependencies(t *testing.T) {
	valid := KubernetesCollectorSignalProbeConfig{
		Client:             fake.NewSimpleClientset(),
		RESTConfig:         &rest.Config{Host: "https://127.0.0.1"},
		Query:              fakeMarkerQueryer{},
		QueryCredentialRef: "newrelic-acceptance://user-key",
	}
	tests := []struct {
		name   string
		mutate func(*KubernetesCollectorSignalProbeConfig)
	}{
		{name: "valid", mutate: func(*KubernetesCollectorSignalProbeConfig) {}},
		{name: "missing client", mutate: func(config *KubernetesCollectorSignalProbeConfig) { config.Client = nil }},
		{name: "missing REST config", mutate: func(config *KubernetesCollectorSignalProbeConfig) { config.RESTConfig = nil }},
		{name: "missing query", mutate: func(config *KubernetesCollectorSignalProbeConfig) { config.Query = nil }},
		{name: "invalid query credential", mutate: func(config *KubernetesCollectorSignalProbeConfig) { config.QueryCredentialRef = "plaintext" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			probe, err := NewKubernetesCollectorSignalProbe(config)
			if test.name == "valid" {
				if err != nil || probe == nil {
					t.Fatalf("probe=%v err=%v", probe, err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid probe configuration was accepted")
			}
		})
	}
}

func TestKubernetesCollectorSignalProbeSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/metrics" {
			t.Fatalf("request=%s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Content-Type") != "application/x-protobuf" {
			t.Fatalf("content type=%q", request.Header.Get("Content-Type"))
		}
		if _, err := io.Copy(io.Discard, request.Body); err != nil {
			t.Fatal(err)
		}
		response.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	probe := &KubernetesCollectorSignalProbe{httpClient: server.Client()}
	payload, signal, err := BuildOTLPProbePayload("metrics", "magelift/test/collector", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := probe.send(context.Background(), server.URL, signal, payload); err != nil {
		t.Fatal(err)
	}
}

type fakeMarkerQueryer struct{}

func (fakeMarkerQueryer) QueryMarker(context.Context, MarkerQueryRequest) (MarkerQueryResult, error) {
	return MarkerQueryResult{Count: 1, LabelsVerified: true}, nil
}

var _ kube.CollectorSignalProbe = (*KubernetesCollectorSignalProbe)(nil)
