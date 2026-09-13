package config

import "strings"

// CanonicalAuroraMySQLVersion maps truncated MageLift pins onto the
// EngineVersion string AWS catalogs. DescribeDBEngineVersions rejects
// 8.0.mysql_aurora.3.12 (empty) and returns 8.0.mysql_aurora.3.12.0.
func CanonicalAuroraMySQLVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "8.0.mysql_aurora.3.12" {
		return "8.0.mysql_aurora.3.12.0"
	}
	return version
}
