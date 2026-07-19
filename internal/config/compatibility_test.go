package config

import (
	"strings"
	"testing"
)

func TestSupportedMagentoPHPIntersections(t *testing.T) {
	tests := []struct {
		magento string
		php     string
	}{
		{magento: "2.4.9", php: "8.5"},
		{magento: "2.4.8", php: "8.3"},
		{magento: "2.4.8-p5", php: "8.4.1"},
		{magento: "2.4.7-p10", php: "8.2"},
		{magento: "2.4.7", php: "8.3"},
		{magento: "2.4.6-p15", php: "8.2.20"},
	}
	for _, tt := range tests {
		t.Run(tt.magento+"-php-"+tt.php, func(t *testing.T) {
			f, err := Load([]byte(withVersions(base, tt.magento, tt.php)))
			if err != nil {
				t.Fatal(err)
			}
			effective, err := f.Resolve("staging", ResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if effective.Compatibility.Status != CompatibilitySupported {
				t.Fatalf("status = %q", effective.Compatibility.Status)
			}
		})
	}
}

func TestFuturePatchesAreUnsupported(t *testing.T) {
	for _, version := range []string{"2.4.9-p1", "2.4.8-p6", "2.4.7-p11", "2.4.6-p16", "2.4.5-p18", "2.4.4-p19"} {
		t.Run(version, func(t *testing.T) {
			php := "8.2"
			if strings.HasPrefix(version, "2.4.9") {
				php = "8.5"
			} else if strings.HasPrefix(version, "2.4.8") {
				php = "8.4"
			} else if strings.HasPrefix(version, "2.4.5") || strings.HasPrefix(version, "2.4.4") {
				php = "8.1"
			}
			input := withVersions(base, version, php)
			f, err := Load([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.Resolve("staging", ResolveOptions{})
			if err == nil || !strings.Contains(err.Error(), "newer than the latest patch") {
				t.Fatalf("unexpected error: %v", err)
			}

			input = strings.Replace(input, "extensions:\n", "compatibility: {allowUnsupported: true}\nextensions:\n", 1)
			f, err = Load([]byte(input))
			if err != nil {
				t.Fatal(err)
			}
			effective, err := f.Resolve("staging", ResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if effective.Compatibility.Status != CompatibilityUnsupportedAllowed {
				t.Fatalf("status = %q", effective.Compatibility.Status)
			}
		})
	}
}

func TestCompatibilityRejectsUnresolvedRange(t *testing.T) {
	input := strings.Replace(base, "2.4.8-p5", "2.4.x", 1)
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "ranges are not resolved") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnsupportedCombinationRequiresCommittedOverride(t *testing.T) {
	input := withVersions(base, "2.4.8-p5", "8.2")
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "allowUnsupported: true") {
		t.Fatalf("unexpected error: %v", err)
	}

	input = strings.Replace(input, "extensions:\n", "compatibility: {allowUnsupported: true}\nextensions:\n", 1)
	f, err = Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := f.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Compatibility.Status != CompatibilityUnsupportedAllowed || len(effective.Compatibility.Reasons) == 0 {
		t.Fatalf("assessment = %#v", effective.Compatibility)
	}
}

func TestAdobeListedPHP81LinesAreUnavailable(t *testing.T) {
	input := withVersions(base, "2.4.5-p17", "8.1")
	f, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "below MageLift's PHP 8.2 minimum") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCompatibilityCatalogMetadataAndCopies(t *testing.T) {
	catalog := CurrentCompatibilityCatalog()
	if catalog.Metadata.UpdatedAt != "2026-07-18" || catalog.Metadata.AdobeSnapshotAt != "2026-06-01" || len(catalog.Metadata.Sources) != 4 {
		t.Fatalf("metadata = %#v", catalog.Metadata)
	}
	if !strings.Contains(catalog.Metadata.Scope, "AWS service combinations") {
		t.Fatalf("scope = %q", catalog.Metadata.Scope)
	}
	catalog.Lines[0].PHPBranches[0] = "changed"
	if CurrentCompatibilityCatalog().Lines[0].PHPBranches[0] != "8.5" {
		t.Fatal("catalog accessor exposed mutable package data")
	}
}

func TestCompatibilityCatalogCoversSupportedPHPBranches(t *testing.T) {
	catalog := CurrentCompatibilityCatalog()
	want := []PHPBranchSupport{
		{Branch: "8.2", Status: "security-fixes-only", ActiveSupportUntil: "2024-12-31", SecuritySupportUntil: "2026-12-31"},
		{Branch: "8.3", Status: "security-fixes-only", ActiveSupportUntil: "2025-12-31", SecuritySupportUntil: "2027-12-31"},
		{Branch: "8.4", Status: "active", ActiveSupportUntil: "2026-12-31", SecuritySupportUntil: "2028-12-31"},
		{Branch: "8.5", Status: "active", ActiveSupportUntil: "2027-12-31", SecuritySupportUntil: "2029-12-31"},
	}
	if len(catalog.PHP) != len(want) {
		t.Fatalf("PHP branch count = %d, want %d", len(catalog.PHP), len(want))
	}
	for index := range want {
		if catalog.PHP[index] != want[index] {
			t.Fatalf("PHP branch %d = %#v, want %#v", index, catalog.PHP[index], want[index])
		}
	}
	catalog.PHP[0].Status = "changed"
	if CurrentCompatibilityCatalog().PHP[0].Status != "security-fixes-only" {
		t.Fatal("catalog accessor exposed mutable PHP metadata")
	}
}

func withVersions(input, magento, php string) string {
	input = strings.Replace(input, "2.4.8-p5", magento, 1)
	return strings.Replace(input, `php: "8.3"`, `php: "`+php+`"`, 1)
}
