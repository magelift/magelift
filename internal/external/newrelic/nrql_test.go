package newrelic

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	provider "github.com/magelift/magelift/internal/provider"
)

func TestNRQLClientPollsUntilOTLPMarkerIsVisibleWithoutLeakingCredential(t *testing.T) {
	const (
		accountID     = int64(8368691)
		credential    = "nr-license-secret"
		credentialRef = "newrelic://license"
		marker        = "magelift/test/nrql"
	)
	var attempts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("API-Key"); got != credential {
			t.Errorf("API-Key = %q, want scoped credential", got)
		}
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode NerdGraph request: %v", err)
			return
		}
		if strings.Contains(body.Query, credential) || !strings.Contains(body.Query, "account(id: 8368691)") || !strings.Contains(body.Query, "FROM Metric") || !strings.Contains(body.Query, "`magelift.ownership_marker`") || !strings.Contains(body.Query, marker) {
			t.Errorf("NerdGraph query = %q", body.Query)
		}
		count := int64(0)
		if attempts.Add(1) == 2 {
			count = 1
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(writer, `{"data":{"actor":{"account":{"nrql":{"results":[{"count":%d}]}}}}}`, count)
	}))
	t.Cleanup(server.Close)
	resolver := provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
		if reference != credentialRef {
			t.Fatalf("credential reference = %q", reference)
		}
		return consume([]byte(credential))
	})
	client, err := NewNRQLClientWithRetry(server.Client(), resolver, server.URL, accountID, RetryPolicy{MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.QueryMarker(context.Background(), MarkerQueryRequest{Signal: SignalMetrics, CredentialRef: credentialRef, OwnershipMarker: marker})
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 || !result.LabelsVerified || attempts.Load() != 2 {
		t.Fatalf("query result = %#v, attempts = %d", result, attempts.Load())
	}
}

func TestBuildMarkerQueryUsesOfficialOTLPEventTypes(t *testing.T) {
	tests := []struct {
		signal Signal
		event  string
	}{
		{signal: SignalLogs, event: "Log"},
		{signal: SignalMetrics, event: "Metric"},
		{signal: SignalTraces, event: "Span"},
	}
	for _, test := range tests {
		t.Run(string(test.signal), func(t *testing.T) {
			query, err := buildMarkerQuery(8368691, test.signal, "magelift/test/quote")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(query, "FROM "+test.event) || !strings.Contains(query, "count(*)") || !strings.Contains(query, "SINCE 1 hour ago") {
				t.Fatalf("query = %q", query)
			}
		})
	}
}

func TestNRQLClientRejectsControlCharactersBeforeResolvingCredential(t *testing.T) {
	resolverCalled := false
	client, err := NewNRQLClientWithRetry(&http.Client{}, provider.CredentialResolverFunc(func(context.Context, string, func([]byte) error) error {
		resolverCalled = true
		return nil
	}), "https://api.newrelic.com/graphql", 8368691, RetryPolicy{MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.QueryMarker(context.Background(), MarkerQueryRequest{Signal: SignalLogs, CredentialRef: "newrelic://license", OwnershipMarker: "magelift/test/\nunsafe"})
	if err == nil || resolverCalled {
		t.Fatalf("control-character validation error = %v, resolverCalled = %v", err, resolverCalled)
	}
}

func TestNRQLClientCanUseSeparateQueryCredential(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"data":{"actor":{"account":{"nrql":{"results":[{"count":1}]}}}}}`))
	}))
	t.Cleanup(server.Close)
	resolver := provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
		if reference != "newrelic://query" {
			t.Fatalf("query credential reference = %q", reference)
		}
		return consume([]byte("query-key"))
	})
	client, err := NewNRQLClientWithQueryCredentialAndRetry(server.Client(), resolver, server.URL, 8368691, "newrelic://query", RetryPolicy{MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.QueryMarker(context.Background(), MarkerQueryRequest{Signal: SignalLogs, CredentialRef: "newrelic://ingest", OwnershipMarker: "magelift/test/separate-query-key"})
	if err != nil || result.Count != 1 || !result.LabelsVerified {
		t.Fatalf("separate query credential result = %#v, err = %v", result, err)
	}
}
