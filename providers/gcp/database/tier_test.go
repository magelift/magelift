package database

import (
	"strings"
	"testing"
)

func TestDefaultTier(t *testing.T) {
	tests := []struct {
		name            string
		databaseVersion string
		preset          string
		want            string
	}{
		{name: "mysql 8.0 preview", databaseVersion: DatabaseVersionMySQL80, preset: "preview", want: "db-custom-1-3840"},
		{name: "mysql 8.0 standard", databaseVersion: DatabaseVersionMySQL80, preset: "standard", want: "db-custom-2-7680"},
		{name: "mysql 8.4 preview", databaseVersion: DatabaseVersionMySQL84, preset: "preview", want: DefaultMySQL84Tier},
		{name: "mysql 8.4 high availability", databaseVersion: DatabaseVersionMySQL84, preset: "high-availability", want: DefaultMySQL84Tier},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := DefaultTier(test.databaseVersion, test.preset); got != test.want {
				t.Fatalf("DefaultTier() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEditionForDatabaseVersion(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{version: DatabaseVersionMySQL80, want: DatabaseEditionEnterprise},
		{version: DatabaseVersionMySQL84, want: DatabaseEditionEnterprisePlus},
	}
	for _, test := range tests {
		t.Run(test.version, func(t *testing.T) {
			got, err := EditionForDatabaseVersion(test.version)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("EditionForDatabaseVersion() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEditionForDatabaseVersionAndTierMySQL84(t *testing.T) {
	tests := []struct {
		tier string
		want string
	}{
		{tier: "db-perf-optimized-N-2", want: DatabaseEditionEnterprisePlus},
		{tier: "db-c4a-highmem-2", want: DatabaseEditionEnterprisePlus},
		{tier: "db-custom-1-3840", want: DatabaseEditionEnterprise},
		{tier: "db-custom-2-7680", want: DatabaseEditionEnterprise},
	}
	for _, test := range tests {
		t.Run(test.tier, func(t *testing.T) {
			got, err := EditionForDatabaseVersionAndTier(DatabaseVersionMySQL84, test.tier)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("EditionForDatabaseVersionAndTier() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestEditionForDatabaseVersionAndTierMySQL80CustomStaysEnterprise(t *testing.T) {
	got, err := EditionForDatabaseVersionAndTier(DatabaseVersionMySQL80, "db-custom-2-7680")
	if err != nil {
		t.Fatal(err)
	}
	if got != DatabaseEditionEnterprise {
		t.Fatalf("edition = %q, want ENTERPRISE", got)
	}
}

func TestValidateTierMySQL84DedicatedCustomAccepted(t *testing.T) {
	for _, tier := range []string{"db-custom-1-3840", "db-custom-2-7680"} {
		if err := ValidateTier(DatabaseVersionMySQL84, tier); err != nil {
			t.Fatalf("valid dedicated custom tier %q was rejected: %v", tier, err)
		}
	}
	for _, tier := range []string{"db-perf-optimized-N-2", "db-c4a-highmem-2"} {
		if err := ValidateTier(DatabaseVersionMySQL84, tier); err != nil {
			t.Fatalf("valid MySQL 8.4 tier %q was rejected: %v", tier, err)
		}
	}
}

func TestValidateTierLeavesProviderTiersSelectable(t *testing.T) {
	for _, tier := range []string{"db-f1-micro", "db-custom-N4-2-7680", "db-perf-optimized-C4-2"} {
		if err := ValidateTier(DatabaseVersionMySQL84, tier); err != nil {
			t.Fatalf("tier %q was rejected: %v", tier, err)
		}
	}
}

func TestValidateTierUnsupportedEngineAndRequiredTier(t *testing.T) {
	if err := ValidateTier("POSTGRES_15", "db-custom-1-3840"); err == nil || !strings.Contains(err.Error(), "invalid Cloud SQL MySQL version") {
		t.Fatalf("unsupported engine: %v", err)
	}
	if err := ValidateTier(DatabaseVersionMySQL84, "   "); err == nil || !strings.Contains(err.Error(), "Cloud SQL tier is required") {
		t.Fatalf("empty tier: %v", err)
	}
}
