package config

import "strings"

// RetainedBackup is the destroy-time report of a provider backup that remains
// inside the backup policy after the environment is destroyed.
type RetainedBackup struct {
	DataClass     string `json:"dataClass" yaml:"dataClass"`
	Provider      string `json:"provider" yaml:"provider"`
	Mechanism     string `json:"mechanism" yaml:"mechanism"`
	RetentionDays int    `json:"retentionDays,omitempty" yaml:"retentionDays,omitempty"`
	Reason        string `json:"reason" yaml:"reason"`
}

const retainedBackupReason = "backup policy still requires retained backups"

// RetainedBackupsOnDestroy lists provider backups that destroy must keep unless
// the operator passes the documented --destroy-backups flag. Preview and other
// disposable policies return an empty list.
func RetainedBackupsOnDestroy(cfg Config) []RetainedBackup {
	switch strings.TrimSpace(cfg.Target.Provider) {
	case "gcp":
		return gcpRetainedBackupsOnDestroy(cfg)
	case "aws":
		return awsRetainedBackupsOnDestroy(cfg)
	default:
		return nil
	}
}

func gcpRetainedBackupsOnDestroy(cfg Config) []RetainedBackup {
	gcp := cfg.Target.GCP
	if gcp == nil {
		return nil
	}
	if !cloudSQLBackupsEnabled(cfg, gcp) {
		return nil
	}
	days := 7
	if gcp.CloudSQLTransactionLogRetention != nil && *gcp.CloudSQLTransactionLogRetention > days {
		days = *gcp.CloudSQLTransactionLogRetention
	}
	if gcp.CloudSQLBackupRetentionCount != nil && *gcp.CloudSQLBackupRetentionCount > days {
		days = *gcp.CloudSQLBackupRetentionCount
	}
	if days > 365 {
		days = 365
	}
	return []RetainedBackup{{
		DataClass:     "database",
		Provider:      "gcp",
		Mechanism:     "cloud-sql-final-backup",
		RetentionDays: days,
		Reason:        retainedBackupReason,
	}}
}

func cloudSQLBackupsEnabled(cfg Config, gcp *GCPTarget) bool {
	if gcp.CloudSQLBackupEnabled != nil {
		return *gcp.CloudSQLBackupEnabled
	}
	availability := strings.TrimSpace(gcp.CloudSQLAvailability)
	if availability == "" {
		availability = "ZONAL"
		if effectivePreset(cfg) != "preview" {
			availability = "REGIONAL"
		}
	}
	return availability == "REGIONAL"
}

func effectivePreset(cfg Config) string {
	preset := strings.TrimSpace(cfg.Preset)
	if preset != "" {
		return preset
	}
	return strings.TrimSpace(cfg.Defaults.Preset)
}

func awsRetainedBackupsOnDestroy(cfg Config) []RetainedBackup {
	aws := cfg.Target.AWS
	if aws == nil {
		return nil
	}
	deleteAutomated := cfg.Class != "production"
	if aws.Catalog.DatabaseDeleteAutomatedBackups != nil {
		deleteAutomated = *aws.Catalog.DatabaseDeleteAutomatedBackups
	}
	// Production always takes a final snapshot (SkipFinalSnapshot=false).
	// Non-production keeps backups only when automated-backup deletion is off.
	if cfg.Class != "production" && deleteAutomated {
		return nil
	}
	return []RetainedBackup{{
		DataClass:     "database",
		Provider:      "aws",
		Mechanism:     "rds-final-snapshot-and-automated-backups",
		RetentionDays: aws.Catalog.Retention.BackupDays,
		Reason:        retainedBackupReason,
	}}
}
