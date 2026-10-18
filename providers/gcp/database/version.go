package database

import (
	"fmt"
	"strconv"
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
	switch databaseVersion {
	case DatabaseVersionMySQL80:
		return DatabaseEditionEnterprise, nil
	case DatabaseVersionMySQL84:
		return DatabaseEditionEnterprisePlus, nil
	default:
		return "", fmt.Errorf("unsupported Cloud SQL database version %q", databaseVersion)
	}
}

// EditionForDatabaseVersionAndTier resolves the Cloud SQL edition from the
// database version and machine tier. MySQL 8.4 predefined N2 and C4A tiers
// stay on Enterprise Plus; valid db-custom-* dedicated-core tiers use Enterprise.
func EditionForDatabaseVersionAndTier(databaseVersion, tier string) (string, error) {
	switch databaseVersion {
	case DatabaseVersionMySQL80:
		return DatabaseEditionEnterprise, nil
	case DatabaseVersionMySQL84:
		tier = strings.TrimSpace(tier)
		if err := ValidateTier(databaseVersion, tier); err != nil {
			return "", err
		}
		if strings.HasPrefix(tier, dedicatedCustomTierPrefix) {
			return DatabaseEditionEnterprise, nil
		}
		return DatabaseEditionEnterprisePlus, nil
	default:
		return "", fmt.Errorf("unsupported Cloud SQL database version %q", databaseVersion)
	}
}

const dedicatedCustomTierPrefix = "db-custom-"

// isMySQL84DedicatedCustomTier reports whether tier is a valid Enterprise
// dedicated-core db-custom-<cpus>-<memoryMB> machine type for MySQL 8.4.
func isMySQL84DedicatedCustomTier(tier string) bool {
	if !strings.HasPrefix(tier, dedicatedCustomTierPrefix) {
		return false
	}
	if strings.HasPrefix(tier, dedicatedCustomTierPrefix+"N4-") {
		return false
	}
	suffix := tier[len(dedicatedCustomTierPrefix):]
	sep := strings.LastIndexByte(suffix, '-')
	if sep <= 0 || sep >= len(suffix)-1 {
		return false
	}
	cpuStr := suffix[:sep]
	memStr := suffix[sep+1:]
	if cpuStr[0] == '0' || memStr[0] == '0' {
		return false
	}
	for _, c := range cpuStr {
		if c < '0' || c > '9' {
			return false
		}
	}
	for _, c := range memStr {
		if c < '0' || c > '9' {
			return false
		}
	}
	cpus, err := strconv.Atoi(cpuStr)
	if err != nil || !validDedicatedCustomCPUs(cpus) {
		return false
	}
	memoryMB, err := strconv.Atoi(memStr)
	if err != nil || !validDedicatedCustomMemoryMB(cpus, memoryMB) {
		return false
	}
	return true
}

func validDedicatedCustomCPUs(cpus int) bool {
	if cpus == 1 {
		return true
	}
	return cpus >= 2 && cpus <= 96 && cpus%2 == 0
}

func validDedicatedCustomMemoryMB(cpus, memoryMB int) bool {
	if memoryMB < 3840 || memoryMB%256 != 0 {
		return false
	}
	// Bound memory before multiplication, then compare the 0.9 GiB minimum
	// with integer arithmetic. CPU validation bounds the right-hand side.
	const minGiBNumer, minGiBDenom = 9, 10
	const maxGiBPerCPU = 6656 // 6.5 * 1024 MiB
	if memoryMB > cpus*maxGiBPerCPU {
		return false
	}
	if memoryMB*minGiBDenom < cpus*minGiBNumer*1024 {
		return false
	}
	return true
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
	if databaseVersion == DatabaseVersionMySQL84 {
		if strings.HasPrefix(tier, "db-perf-optimized-N-") || strings.HasPrefix(tier, "db-c4a-") {
			return nil
		}
		if isMySQL84DedicatedCustomTier(tier) {
			return nil
		}
		return fmt.Errorf(
			"Cloud SQL tier %q is not valid for %s; use db-perf-optimized-N-*, db-c4a-*, or a valid db-custom-<cpus>-<memoryMB> Enterprise dedicated-core tier",
			tier, databaseVersion,
		)
	}
	return nil
}
