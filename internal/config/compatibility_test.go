package config

import (
	"strings"
	"testing"
	"time"
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
	if catalog.Metadata.UpdatedAt != "2026-08-12" || catalog.Metadata.AdobeSnapshotAt != "2026-06-01" || len(catalog.Metadata.Sources) != 5 {
		t.Fatalf("metadata = %#v", catalog.Metadata)
	}
	if err := catalog.Validate(time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("catalog validation = %v", err)
	}
	if !strings.Contains(catalog.Metadata.Scope, "service choices") {
		t.Fatalf("scope = %q", catalog.Metadata.Scope)
	}
	catalog.Lines[0].PHPBranches[0] = "changed"
	if CurrentCompatibilityCatalog().Lines[0].PHPBranches[0] != "8.5" {
		t.Fatal("catalog accessor exposed mutable package data")
	}
}

func TestCompatibilityCatalogRejectsStaleOrMissingSources(t *testing.T) {
	catalog := CurrentCompatibilityCatalog()
	catalog.Metadata.Sources[0].RetrievedAt = "2025-01-01"
	if err := catalog.Validate(time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale source was accepted: %v", err)
	}

	catalog = CurrentCompatibilityCatalog()
	catalog.Metadata.Sources[0].URL = ""
	if err := catalog.Validate(time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("missing source URL was accepted: %v", err)
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

func TestCompatibilityServiceMatrixUsesAdobeStatuses(t *testing.T) {
	tests := []struct {
		name      string
		selection CompatibilitySelection
		want      CompatibilityServiceStatus
		wantError bool
	}{
		{
			name: "latest 2.4.6 rejects mysql",
			selection: CompatibilitySelection{
				Release: "2.4.6-p15", Component: CompatibilityDatabase, Option: "mysql",
			},
			want:      CompatibilityServiceUnsupported,
			wantError: true,
		},
		{
			name: "2.4.8 accepts elasticsearch",
			selection: CompatibilitySelection{
				Release: "2.4.8-p5", Component: CompatibilitySearch, Option: "elasticsearch", Version: "8",
			},
			want: CompatibilityAdobeSupported,
		},
		{
			name: "2.4.9 accepts MariaDB 11.8",
			selection: CompatibilitySelection{
				Release: "2.4.9", Component: CompatibilityDatabase, Option: "mariadb", Version: "11.8",
			},
			want: CompatibilityAdobeSupported,
		},
		{
			name: "2.4.9 rejects elasticsearch",
			selection: CompatibilitySelection{
				Release: "2.4.9", Component: CompatibilitySearch, Option: "elasticsearch",
			},
			want:      CompatibilityServiceUnsupported,
			wantError: true,
		},
		{
			name: "database queue is a MageLift choice",
			selection: CompatibilitySelection{
				Release: "2.4.9", Component: CompatibilityQueue, Option: "database",
			},
			want: CompatibilityMageLift,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requirement, err := ValidateCompatibilitySelection(tt.selection)
			if requirement.Status != tt.want {
				t.Fatalf("status = %q, want %q", requirement.Status, tt.want)
			}
			if tt.wantError && err == nil {
				t.Fatal("unsupported selection unexpectedly passed")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("selection failed: %v", err)
			}
		})
	}
}

func TestServiceRequirementForReleaseIsProviderNeutral(t *testing.T) {
	requirement, err := ServiceRequirementForRelease("2.4.9", CompatibilityCache, "valkey")
	if err != nil {
		t.Fatal(err)
	}
	if requirement.Status != CompatibilityAdobeSupported || len(requirement.Versions) != 1 || requirement.Versions[0] != "9" {
		t.Fatalf("Valkey requirement = %#v", requirement)
	}
	lineRequirement, err := ServiceRequirementForRelease("2.4.8", CompatibilityCache, "valkey")
	if err != nil {
		t.Fatal(err)
	}
	if len(lineRequirement.Versions) != 1 || lineRequirement.Versions[0] != "8.1" {
		t.Fatalf("release-line Valkey requirement = %#v", lineRequirement)
	}
}

func TestRequirementsForReleaseReturnsCopies(t *testing.T) {
	requirements := RequirementsForRelease("2.4.9")
	if len(requirements) == 0 {
		t.Fatal("2.4.9 has no service requirements")
	}
	requirements[0].Versions = append(requirements[0].Versions, "changed")
	if contains(CurrentCompatibilityCatalog().Requirements[0].Versions, "changed") {
		t.Fatal("requirements accessor exposed mutable package data")
	}
}

func TestAdobeUnsupportedMySQLFailsClosedUnlessHatch(t *testing.T) {
	input := withVersions(base, "2.4.6-p15", "8.2")
	input = strings.Replace(input, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {catalog: {databaseEngine: rds-mysql, versions: {mysql: 8.0.45, valkey: \"8.1\", openSearch: OpenSearch_2.19}}}}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "Adobe") || !strings.Contains(err.Error(), "mysql") || !strings.Contains(err.Error(), "mariadb") {
		t.Fatalf("Adobe-unsupported MySQL error = %v", err)
	}

	input = strings.Replace(input, "extensions:\n", "compatibility: {allowUnsupported: true}\nextensions:\n", 1)
	file, err = Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Compatibility.Status != CompatibilityUnsupportedAllowed {
		t.Fatalf("status = %q", effective.Compatibility.Status)
	}
	joined := strings.Join(effective.Compatibility.Reasons, "\n")
	if !strings.Contains(joined, "Adobe") || strings.Contains(joined, string(CompatibilityAdobeSupported)) {
		t.Fatalf("hatch provenance = %#v", effective.Compatibility)
	}
}

func TestExperimentalManagedInstancesWarnsWithoutHatch(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {catalog: {fargate: {computeMode: managed-instances}}}}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Compatibility.Status != CompatibilitySupported {
		t.Fatalf("status = %q", effective.Compatibility.Status)
	}
	joined := strings.Join(effective.Compatibility.Warnings, "\n")
	if !strings.Contains(joined, "MageLift") || !strings.Contains(joined, "experimental") || !strings.Contains(joined, "managed-instances") {
		t.Fatalf("warnings = %#v", effective.Compatibility.Warnings)
	}
}

func TestExperimentalAuroraWarnsWithoutHatch(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {catalog: {databaseEngine: aurora-mysql}}}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Compatibility.Status != CompatibilitySupported {
		t.Fatalf("status = %q", effective.Compatibility.Status)
	}
	joined := strings.Join(effective.Compatibility.Warnings, "\n")
	if !strings.Contains(joined, "MageLift") || !strings.Contains(joined, "Aurora") || !strings.Contains(joined, "experimental") || !strings.Contains(joined, "not certified") {
		t.Fatalf("warnings = %#v", effective.Compatibility.Warnings)
	}
}

func TestExperimentalAmazonMQWarnsWithoutHatch(t *testing.T) {
	input := strings.Replace(base, "target: {provider: aws, runtime: ecs-fargate}", "target: {provider: aws, runtime: ecs-fargate, aws: {catalog: {queueMode: amazon-mq}}}", 1)
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	effective, err := file.Resolve("staging", ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if effective.Compatibility.Status != CompatibilitySupported {
		t.Fatalf("status = %q", effective.Compatibility.Status)
	}
	joined := strings.Join(effective.Compatibility.Warnings, "\n")
	if !strings.Contains(joined, "MageLift") || !strings.Contains(joined, "Amazon MQ") || !strings.Contains(joined, "experimental") {
		t.Fatalf("warnings = %#v", effective.Compatibility.Warnings)
	}
}

func TestProviderUnavailableOVHCDNFailsClosed(t *testing.T) {
	input := `schemaVersion: 1
project: {name: shop}
application: {edition: open-source, version: 2.4.8-p5, mode: integrated}
build:
  php: "8.3"
  composer: {credentials: aws-secrets-manager://composer/auth}
target: {provider: ovh, runtime: mks, ovh: {serviceName: pc-example}}
edge: {mode: native, nativeProvider: cdn, originHealthRef: ovh/lb}
defaults: {region: GRA9, preset: preview}
environments:
  staging:
    account: "pc-example"
    domain: shop.example
extensions: {}
`
	file, err := Load([]byte(input))
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Resolve("staging", ResolveOptions{})
	if err == nil || !strings.Contains(err.Error(), "OVH") || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("unavailable OVH CDN error = %v", err)
	}
}

func withVersions(input, magento, php string) string {
	input = strings.Replace(input, "2.4.8-p5", magento, 1)
	return strings.Replace(input, `php: "8.3"`, `php: "`+php+`"`, 1)
}
