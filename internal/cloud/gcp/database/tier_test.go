package database

import "testing"

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

func TestValidateTierRejectsCustomMySQL84Tier(t *testing.T) {
	if err := ValidateTier(DatabaseVersionMySQL84, "db-custom-2-7680"); err == nil {
		t.Fatal("custom MySQL 8.4 tier was accepted")
	}
	for _, tier := range []string{"db-perf-optimized-N-2", "db-c4a-highmem-2"} {
		if err := ValidateTier(DatabaseVersionMySQL84, tier); err != nil {
			t.Fatalf("valid MySQL 8.4 tier %q was rejected: %v", tier, err)
		}
	}
}
