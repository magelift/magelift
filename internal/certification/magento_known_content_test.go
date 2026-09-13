package certification

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestVerifyMagentoKnownContentAcceptsExpectedMarker(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, "<title>Magento</title><h1>Fusion Backpack</h1>")
	}))
	t.Cleanup(server.Close)
	if err := VerifyMagentoKnownContent(context.Background(), server.Client(), MagentoKnownContentRequest{
		URL: server.URL, Expect: "Fusion Backpack",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyMagentoKnownContentRejectsMissingMarkerAndNonOK(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		status  int
		body    string
		url     string
		expect  string
		wantErr string
		cancel  bool
	}{
		{name: "missing marker", status: http.StatusOK, body: "<title>Magento</title>", expect: "Fusion Backpack", wantErr: "did not contain the expected marker"},
		{name: "status 500", status: http.StatusInternalServerError, body: "Fusion Backpack", expect: "Fusion Backpack", wantErr: "status 500"},
		{name: "empty expect", url: "https://example.invalid/", expect: " ", wantErr: "single-line"},
		{name: "newline expect", url: "https://example.invalid/", expect: "Fusion\nBackpack", wantErr: "single-line"},
		{name: "credentials in URL", url: "https://user:pass@example.invalid/", expect: "Fusion Backpack", wantErr: "credentials"},
		{name: "ftp URL", url: "ftp://example.invalid/store", expect: "Fusion Backpack", wantErr: "http or https"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			requestURL := test.url
			client := http.DefaultClient
			if test.status != 0 {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(test.status)
					_, _ = io.WriteString(w, test.body)
				}))
				t.Cleanup(server.Close)
				requestURL = server.URL
				client = server.Client()
			}
			ctx := context.Background()
			if test.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := VerifyMagentoKnownContent(ctx, client, MagentoKnownContentRequest{URL: requestURL, Expect: test.expect})
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestVerifyMagentoKnownContentHonorsCanceledContext(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, "Fusion Backpack")
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	err := VerifyMagentoKnownContent(ctx, server.Client(), MagentoKnownContentRequest{URL: server.URL, Expect: "Fusion Backpack"})
	if err == nil || !(strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "canceled")) {
		t.Fatalf("error = %v", err)
	}
}

func TestVerifyMagentoCatalogSKUAcceptsExactSKU(t *testing.T) {
	observed := "+ magelift exec --service web -- php -r '...'\n24-MB01\n{\n  \"environment\": \"high-availability\"\n}\n"
	if err := VerifyMagentoCatalogSKU(observed, "24-MB01"); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyMagentoCatalogSKURejectsMismatchAndEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		observed string
		expect   string
		wantErr  string
	}{
		{name: "mismatch", observed: "24-MB02", expect: "24-MB01", wantErr: "did not match"},
		{name: "empty observed", observed: "+\n\n", expect: "24-MB01", wantErr: "was empty"},
		{name: "empty expect", observed: "24-MB01", expect: " ", wantErr: "single-line"},
		{name: "newline expect", observed: "24-MB01", expect: "24\nMB01", wantErr: "single-line"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := VerifyMagentoCatalogSKU(test.observed, test.expect)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestVerifyMagentoSeedProbeAcceptsFixtureLabel(t *testing.T) {
	observed := "+ magelift exec --service web -- php -r '...'\ntiny-fixture\n{\n  \"environment\": \"high-availability\"\n}\n"
	if err := VerifyMagentoSeedProbe(observed, "tiny-fixture"); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyMagentoSeedProbeSkipsExecHarnessNoise(t *testing.T) {
	observed := "+ magelift exec --service web -- php -r '...'\nwarning: target gcp/gke-standard is experimental: see docs/capability-matrix.md\nDefaulted container \"php-fpm\" out of: php-fpm, web\ntiny-fixture\n{\n  \"environment\": \"high-availability\"\n}\n"
	if err := VerifyMagentoSeedProbe(observed, "tiny-fixture"); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyMagentoSeedProbeRejectsMismatchAndEmpty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		observed string
		expect   string
		wantErr  string
	}{
		{name: "mismatch", observed: "other-fixture", expect: "tiny-fixture", wantErr: "did not match"},
		{name: "empty observed", observed: "+\n\n", expect: "tiny-fixture", wantErr: "was empty"},
		{name: "empty expect", observed: "tiny-fixture", expect: " ", wantErr: "single-line"},
		{name: "newline expect", observed: "tiny-fixture", expect: "tiny\nfixture", wantErr: "single-line"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := VerifyMagentoSeedProbe(test.observed, test.expect)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
