package platform

import (
	"net/url"
	"strings"
)

// Magento workload shell contracts. Cloud adapters pass these into ECS tasks /
// GKE Jobs/Deployments; they must not invent alternate Magento CLI sequences.

// MagentoMigrationShell is the pre-traffic migrate candidate command. It
// matches the PHP lifecycle Deploy plus PostDeploy sequence exactly: static
// content is baked into the immutable image at build time (the single
// authority) and is never regenerated in the disposable candidate, whose
// filesystem never reaches serving containers.
func MagentoMigrationShell() []string {
	return []string{
		"/bin/sh", "-ec",
		"bin/magento app:config:import --no-interaction && " +
			"bin/magento setup:upgrade --keep-generated --no-interaction && " +
			"bin/magento cache:clean && bin/magento cache:flush",
	}
}

// MagentoCronShell is the long-running cron loop used by certified runtimes.
func MagentoCronShell() []string {
	return []string{"/bin/sh", "-ec", "while true; do bin/magento cron:run; sleep 60; done"}
}

// MagentoQueueArgs starts Magento message-queue consumers.
func MagentoQueueArgs() []string {
	return MagentoQueueArgsFor(nil)
}

// MagentoProbeShell is the bounded deployment-readiness probe. It boots Magento
// just far enough to prove the release can serve: database status (fails on
// bootstrap failure, unreachable database, and pending or failed migrations),
// search-engine wiring when a search endpoint is provided, and unsigned
// endpoint reachability when checkReachability is set. Callers pass
// checkReachability=false for SigV4-only endpoints (unsigned curl is
// meaningless against them) and for disabled search. All three legs are
// non-interactive by nature, so no --no-interaction flag is set (an unknown
// flag would fail the probe spuriously).
//
// Search is proved through Magento's effective configuration, not just a
// reachable server: the configured search host must equal the expected
// endpoint's host, and the configured engine must equal expectedEngine
// when provided (empty keeps the legacy nonempty check). A reachable
// server with wrong Magento configuration fails the probe.
func MagentoProbeShell(searchEndpoint string, checkReachability bool, expectedEngine string) []string {
	script := "bin/magento setup:db:status"
	if strings.TrimSpace(searchEndpoint) != "" {
		script += " && test -n \"$(bin/magento config:show catalog/search/engine)\""
		host := probeSearchHost(searchEndpoint)
		script += " && test \"$(bin/magento config:show catalog/search/opensearch_server_hostname)\" = '" + shellQuote(host) + "'"
		if strings.TrimSpace(expectedEngine) != "" {
			script += " && test \"$(bin/magento config:show catalog/search/engine)\" = '" + shellQuote(strings.TrimSpace(expectedEngine)) + "'"
		}
		if checkReachability {
			script += " && curl -fsS --max-time 10 '" + shellQuote(searchEndpoint) + "' -o /dev/null"
		}
	}
	return []string{"/bin/sh", "-ec", script}
}

// probeSearchHost extracts the hostname Magento must be configured with.
// Unparseable endpoints compare literally so exotic-but-consistent setups
// keep working while garbage fails against real configuration.
func probeSearchHost(endpoint string) string {
	trimmed := strings.TrimSpace(endpoint)
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return trimmed
}

func shellQuote(value string) string {
	return strings.ReplaceAll(value, "'", "'\\''")
}

// MagentoQueueArgsFor starts the named Magento consumers, or Magento's default async worker.
func MagentoQueueArgsFor(names []string) []string {
	command := []string{"bin/magento", "queue:consumers:start"}
	if trimmed := nonEmptyStrings(names); len(trimmed) > 0 {
		command = append(command, trimmed...)
	} else {
		command = append(command, "async.operations.all")
	}
	return append(command, "--max-messages=10000")
}
