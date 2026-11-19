package database

import (
	"fmt"
	"regexp"
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

var mysqlVersionPattern = regexp.MustCompile(`^MYSQL_[0-9]+_[0-9]+(_[0-9]+)?$`)

func isSupportedDatabaseVersion(version string) bool {
	return mysqlVersionPattern.MatchString(version)
}

// DefaultTier selects a Cloud SQL tier for the resolved database version and
// MageLift preset. The MySQL 8.4 default remains a predefined Enterprise Plus
// tier; a dedicated-core Enterprise tier is an explicit override.
func DefaultTier(databaseVersion, preset string) string {
	if databaseVersion == DatabaseVersionMySQL84 {
		return DefaultMySQL84Tier
	}
	if preset == "preview" {
		return "db-custom-1-3840"
	}
	return "db-custom-2-7680"
}

// EditionForDatabaseVersion returns the default Cloud SQL edition for the
// release-aware database version.
func EditionForDatabaseVersion(databaseVersion string) (string, error) {
	if !isSupportedDatabaseVersion(databaseVersion) {
		return "", fmt.Errorf("invalid Cloud SQL MySQL version %q", databaseVersion)
	}
	if databaseVersion == DatabaseVersionMySQL84 {
		return DatabaseEditionEnterprisePlus, nil
	}
	return DatabaseEditionEnterprise, nil
}

// EditionForDatabaseVersionAndTier chooses an edition from the requested tier.
func EditionForDatabaseVersionAndTier(databaseVersion, tier string) (string, error) {
	if err := ValidateTier(databaseVersion, tier); err != nil {
		return "", err
	}
	tier = strings.TrimSpace(tier)
	if strings.HasPrefix(tier, "db-perf-optimized-") || strings.HasPrefix(tier, "db-c4a-") {
		return DatabaseEditionEnterprisePlus, nil
	}
	return DatabaseEditionEnterprise, nil
}

// ValidateTier checks the syntax needed by the Cloud SQL API. Availability
// of the selected version and tier remains a provider decision.
func ValidateTier(databaseVersion, tier string) error {
	if !isSupportedDatabaseVersion(databaseVersion) {
		return fmt.Errorf("invalid Cloud SQL MySQL version %q; use an API value such as MYSQL_8_4", databaseVersion)
	}
	tier = strings.TrimSpace(tier)
	if tier == "" {
		return fmt.Errorf("Cloud SQL tier is required")
	}
	if strings.ContainsAny(tier, " \t\r\n\x00") {
		return fmt.Errorf("invalid Cloud SQL tier %q", tier)
	}
	return nil
}
