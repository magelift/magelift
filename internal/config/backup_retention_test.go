package config

import "testing"

func TestRetainedBackupsOnDestroyGCPRegional(t *testing.T) {
	enabled := true
	retentionCount := 14
	logDays := 7
	got := RetainedBackupsOnDestroy(Config{
		Class:  "production",
		Preset: "standard",
		Target: Target{Provider: "gcp", GCP: &GCPTarget{
			CloudSQLAvailability:            "REGIONAL",
			CloudSQLBackupEnabled:           &enabled,
			CloudSQLBackupRetentionCount:    &retentionCount,
			CloudSQLTransactionLogRetention: &logDays,
		}},
	})
	if len(got) != 1 || got[0].Mechanism != "cloud-sql-final-backup" || got[0].RetentionDays != 14 || got[0].Provider != "gcp" {
		t.Fatalf("GCP retained backups = %#v", got)
	}
}

func TestRetainedBackupsOnDestroyGCPPreviewZonal(t *testing.T) {
	got := RetainedBackupsOnDestroy(Config{
		Class:  "preview",
		Preset: "preview",
		Target: Target{Provider: "gcp", GCP: &GCPTarget{CloudSQLAvailability: "ZONAL"}},
	})
	if len(got) != 0 {
		t.Fatalf("preview GCP backups should be disposable, got %#v", got)
	}
}

func TestRetainedBackupsOnDestroyGCPHighAvailabilityPreset(t *testing.T) {
	got := RetainedBackupsOnDestroy(Config{
		Class:    "preview",
		Defaults: Defaults{Preset: "high-availability"},
		Target:   Target{Provider: "gcp", GCP: &GCPTarget{}},
	})
	if len(got) != 1 || got[0].Mechanism != "cloud-sql-final-backup" || got[0].Provider != "gcp" {
		t.Fatalf("HA GCP backups should be retained until --destroy-backups, got %#v", got)
	}
}

func TestRetainedBackupsOnDestroyAWSProduction(t *testing.T) {
	got := RetainedBackupsOnDestroy(Config{
		Class: "production",
		Target: Target{Provider: "aws", AWS: &AWSTarget{
			Catalog: AWSCatalog{Retention: AWSCatalogRetention{BackupDays: 7}},
		}},
	})
	if len(got) != 1 || got[0].Mechanism != "rds-final-snapshot-and-automated-backups" || got[0].RetentionDays != 7 {
		t.Fatalf("AWS production retained backups = %#v", got)
	}
}

func TestRetainedBackupsOnDestroyAWSNonProductionDisposable(t *testing.T) {
	got := RetainedBackupsOnDestroy(Config{
		Class:  "staging",
		Target: Target{Provider: "aws", AWS: &AWSTarget{}},
	})
	if len(got) != 0 {
		t.Fatalf("staging AWS backups should be disposable by default, got %#v", got)
	}
}
