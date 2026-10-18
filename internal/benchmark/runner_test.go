package benchmark

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunReportsLatencyFailuresAndMix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/failure" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	report, err := Run(context.Background(), Config{
		URL: server.URL, Duration: time.Second, Concurrency: 2,
		Requests: []Request{{Path: "/", Weight: 3}, {Path: "/failure", Weight: 1}},
		Client:   server.Client(), Catalog: testCatalog(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != SchemaVersion || report.Samples == 0 || report.Failed == 0 || report.Successful == 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.Cost.Status != "unpriced" || report.Cost.HoursPerMonth != 730 {
		t.Fatalf("cost evidence = %#v", report.Cost)
	}
	if report.LatencyMillis.P50 <= 0 || report.LatencyMillis.P99 < report.LatencyMillis.P50 {
		t.Fatalf("latencies = %#v", report.LatencyMillis)
	}
}

func TestConfigRejectsUnsafeMix(t *testing.T) {
	cfg := Config{URL: "http://localhost", Duration: time.Second, Concurrency: 1, Requests: []Request{{Path: "graphql", Weight: 1}}, Catalog: testCatalog()}
	if err := cfg.Validate(); err == nil {
		t.Fatal("relative request path accepted")
	}
}

func TestConfigRejectsSubsecondRun(t *testing.T) {
	cfg := Config{URL: "http://localhost", Duration: 999 * time.Millisecond, Concurrency: 1, Requests: []Request{{Path: "/", Weight: 1}}, Catalog: testCatalog()}
	if err := cfg.Validate(); err == nil {
		t.Fatal("subsecond benchmark accepted")
	}
}

func TestConfigRequiresReproducibleCatalogIdentity(t *testing.T) {
	cfg := Config{URL: "http://localhost", Duration: time.Second, Concurrency: 1, Requests: []Request{{Path: "/", Weight: 1}}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("benchmark without catalog identity accepted")
	}
}

func TestRunCountsClientErrorsAsFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	report, err := Run(context.Background(), Config{
		URL: server.URL, Duration: time.Second, Concurrency: 1,
		Requests: []Request{{Path: "/", Weight: 1}}, Client: server.Client(), Catalog: testCatalog(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Failed == 0 || report.Successful != 0 || report.Errors["http_404"] == 0 {
		t.Fatalf("client error was not classified as failure: %#v", report)
	}
}

func TestReportValidateRejectsInconsistentCounts(t *testing.T) {
	report := Report{SchemaVersion: SchemaVersion, TargetURL: "http://localhost", Catalog: testCatalog(), Concurrency: 1, Samples: 1, Cost: CostEvidence{Status: "unpriced", HoursPerMonth: 730}}
	if err := report.Validate(); err == nil {
		t.Fatal("inconsistent report accepted")
	}
}

func testCatalog() Catalog {
	return Catalog{Name: "magento-standard", Revision: "catalog-revision-1", Edition: "open-source", MagentoVersion: "2.4.9", PHPVersion: "8.5"}
}
