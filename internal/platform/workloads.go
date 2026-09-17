package platform

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// Magento workload shell contracts. Cloud adapters pass these into ECS tasks /
// GKE Jobs/Deployments; they must not invent alternate Magento CLI sequences.

// lifecycleDeployGolden is the PHP lifecycle plan's deploy plus post-deploy
// sequence, emitted by build/bin/magelift-lifecycle-export. The PHP plan is
// the single authority; Go renders it. Regenerate with
// `make lifecycle-golden`; CI fails on drift.
//
//go:embed testdata/lifecycle-deploy.json
var lifecycleDeployGolden []byte

// MagentoMigrationShell is the pre-traffic migrate candidate command,
// rendered from the PHP lifecycle golden: deploy steps then post-deploy
// steps joined for the single-shell candidate job. Static content is baked
// into the immutable image at build time and is never regenerated in the
// disposable candidate, whose filesystem never reaches serving containers.
func MagentoMigrationShell() []string {
	var golden struct {
		Deploy     [][]string `json:"deploy"`
		PostDeploy [][]string `json:"postDeploy"`
	}
	if err := json.Unmarshal(lifecycleDeployGolden, &golden); err != nil {
		panic(fmt.Sprintf("decode lifecycle golden: %v", err))
	}
	var parts []string
	for _, command := range append(append([][]string{}, golden.Deploy...), golden.PostDeploy...) {
		if len(command) == 0 {
			panic("lifecycle golden holds an empty command")
		}
		switch command[0] {
		case "bin/magento", "composer":
		default:
			panic(fmt.Sprintf("lifecycle golden executable %q is not allowed", command[0]))
		}
		for _, arg := range command[1:] {
			if strings.TrimSpace(arg) == "" || strings.ContainsAny(arg, " \t\n\"'") {
				panic(fmt.Sprintf("lifecycle golden argument %q needs quoting support", arg))
			}
		}
		parts = append(parts, strings.Join(command, " "))
	}
	if len(parts) == 0 {
		panic("lifecycle golden holds no commands")
	}
	return []string{"/bin/sh", "-ec", strings.Join(parts, " && ")}
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
