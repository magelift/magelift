package database

import (
	"fmt"
	"strings"
)

const (
	DatabaseVersionMySQL80 = "MYSQL_8_0"
	DatabaseVersionMySQL84 = "MYSQL_8_4"

	DatabaseEditionEnterprise     = "ENTERPRISE"
	DatabaseEditionEnterprisePlus = "ENTERPRISE_PLUS"

	DefaultMySQL84Tier = "db-perf-optimized-N-2"
)

// DatabaseVersionForMagento maps a supported Magento release line to the
// Cloud SQL database version required by that line's GCP MySQL probe.
//
// GCP offers Cloud SQL for MySQL, not MariaDB. The compatibility catalog still
// decides whether MySQL is an Adobe-listed choice for the release; this helper
// only prevents the GCP resource from silently using the wrong MySQL major.
func DatabaseVersionForMagento(version string) (string, error) {
	line := strings.TrimSpace(version)
	if index := strings.IndexByte(line, '-'); index >= 0 {
		line = line[:index]
	}
	switch line {
	case "2.4.6":
		return DatabaseVersionMySQL80, nil
	case "2.4.7", "2.4.8", "2.4.9":
		return DatabaseVersionMySQL84, nil
	default:
		return "", fmt.Errorf("no GCP Cloud SQL MySQL version is mapped for Magento release %q", version)
	}
}

func isSupportedDatabaseVersion(version string) bool {
	switch version {
	case DatabaseVersionMySQL80, DatabaseVersionMySQL84:
		return true
	default:
		return false
	}
}

// DefaultTier selects a Cloud SQL tier for the resolved database version and
// MageLift preset. MySQL 8.4 uses Enterprise Plus, which accepts predefined
// N2 or C4A tiers rather than db-custom-* tiers.
func DefaultTier(databaseVersion, preset string) string {
	if databaseVersion == DatabaseVersionMySQL84 {
		return DefaultMySQL84Tier
	}
	if preset == "preview" {
		return "db-custom-1-3840"
	}
	return "db-custom-2-7680"
}

// EditionForDatabaseVersion returns the explicit Cloud SQL edition required
// by the release-aware database version.
func EditionForDatabaseVersion(databaseVersion string) (string, error) {
	switch databaseVersion {
	case DatabaseVersionMySQL80:
		return DatabaseEditionEnterprise, nil
	case DatabaseVersionMySQL84:
		return DatabaseEditionEnterprisePlus, nil
	default:
		return "", fmt.Errorf("unsupported Cloud SQL database version %q", databaseVersion)
	}
}

// ValidateTier rejects the Cloud SQL tier combinations that the API rejects
// for the release-aware database version.
func ValidateTier(databaseVersion, tier string) error {
	if !isSupportedDatabaseVersion(databaseVersion) {
		return fmt.Errorf("unsupported Cloud SQL database version %q", databaseVersion)
	}
	tier = strings.TrimSpace(tier)
	if tier == "" {
		return fmt.Errorf("Cloud SQL tier is required")
	}
	if databaseVersion == DatabaseVersionMySQL84 &&
		!strings.HasPrefix(tier, "db-perf-optimized-N-") &&
		!strings.HasPrefix(tier, "db-c4a-") {
		return fmt.Errorf("Cloud SQL tier %q is not valid for %s; use a predefined db-perf-optimized-N-* or db-c4a-* tier", tier, databaseVersion)
	}
	return nil
}
