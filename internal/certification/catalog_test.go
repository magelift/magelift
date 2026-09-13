package certification

import (
	"strings"
	"testing"
	"time"
)

func TestCurrentCapabilityCatalogIsSourceDatedAndExplicit(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	catalog := CurrentCapabilityCatalog()
	if err := catalog.Validate(now); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Records) < 30 {
		t.Fatalf("records = %d, want provider/service coverage", len(catalog.Records))
	}
	valkey, ok := catalog.Find("gcp", "europe-west1", "memorystore", "valkey-9.0")
	if !ok || valkey.ProviderStatus != CapabilitySupported || valkey.CertificationStatus != CapabilityExperimental || valkey.Source.RetrievedAt != "2026-08-12" || valkey.Source.ReferenceDate != "2026-07-27" {
		t.Fatalf("GCP Valkey 9.0 record = %#v, found=%v", valkey, ok)
	}
	preview, ok := catalog.Find("gcp", "europe-west1", "memorystore", "valkey-9.1")
	if !ok || preview.Lifecycle != "preview" || preview.ProviderStatus != CapabilityExperimental || preview.MageLiftStatus != CapabilityExperimental || preview.Source.RetrievedAt != "2026-08-12" || preview.Source.ReferenceDate != "2026-07-27" {
		t.Fatalf("GCP Valkey 9.1 preview record = %#v, found=%v", preview, ok)
	}
	fckNat, ok := catalog.Find("aws", "eu-west-3", "fck-nat", "arm64-instance")
	if !ok || fckNat.Source.RetrievedAt != "2026-08-10" || !strings.Contains(fckNat.Source.URL, "fck-nat.dev/stable/deploying") {
		t.Fatalf("AWS fck-nat source record = %#v, found=%v", fckNat, ok)
	}
	scalewaySearch, ok := catalog.Find("scaleway", "fr-par", "search", "none")
	if !ok || scalewaySearch.ProviderStatus != CapabilityUnavailable || scalewaySearch.MageLiftStatus != CapabilityExperimental {
		t.Fatalf("Scaleway search gap = %#v, found=%v", scalewaySearch, ok)
	}
	for _, record := range catalog.Records {
		if strings.TrimSpace(record.Limits) == "" {
			t.Errorf("record %q has no explicit provider limit or eligibility boundary", record.ID)
		}
		if strings.TrimSpace(record.Source.ReferenceDate) == "" && record.CertificationStatus == CapabilityCertified {
			t.Errorf("certified record %q has no source reference date", record.ID)
		}
	}
}

func TestCapabilityCatalogRejectsStaleSources(t *testing.T) {
	catalog := CapabilityCatalog{
		Version: "test", UpdatedAt: "2026-08-08",
		Records: []CapabilityRecord{{
			ID: "test.record", Provider: "test", Region: "*", Service: "service", Role: "role", Lifecycle: "ga",
			AdobeStatus: AdobeCapabilityUnknown, ProviderStatus: CapabilitySupported, MageLiftStatus: CapabilityExperimental,
			CertificationStatus: CapabilityNotRun, Limits: "verify before mutation", Reason: "not live", Source: CapabilitySource{URL: "https://example.com/docs", RetrievedAt: "2025-01-01"},
		}},
	}
	if err := catalog.Validate(time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "older") {
		t.Fatalf("stale source was accepted: %v", err)
	}
}

func TestCapabilityCoverageIncludesReasonsAndCounts(t *testing.T) {
	report, err := CurrentCapabilityCatalog().Coverage(time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if report.Counts[CapabilityExperimental] == 0 || report.Counts[CapabilityUnavailable] == 0 {
		t.Fatalf("coverage counts = %#v", report.Counts)
	}
	for _, row := range report.Rows {
		if row.Status != CapabilityCertified && strings.TrimSpace(row.Reason) == "" {
			t.Fatalf("non-certified row has no reason: %#v", row)
		}
	}
}

func TestCapabilityCoverageHonorsAdobeUnsupportedStatus(t *testing.T) {
	catalog := CapabilityCatalog{
		Version: "test", UpdatedAt: "2026-08-08",
		Records: []CapabilityRecord{{
			ID: "test.redis", Provider: "scaleway", Region: "*", Service: "redis", Role: "cache", ServiceMajor: "redis", Lifecycle: "ga",
			AdobeStatus: AdobeCapabilityUnsupported, ProviderStatus: CapabilitySupported, MageLiftStatus: CapabilityCompatible, CertificationStatus: CapabilityNotRun,
			Limits: "unsupported for this Adobe release", Reason: "Adobe requires Valkey", Source: CapabilitySource{URL: "https://example.com/docs", RetrievedAt: "2026-08-08"},
		}},
	}
	report, err := catalog.Coverage(time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if report.Rows[0].Status != CapabilityUnsupported {
		t.Fatalf("Adobe-unsupported capability status = %q", report.Rows[0].Status)
	}
}
