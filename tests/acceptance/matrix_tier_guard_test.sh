#!/usr/bin/env bash
# Guard: capability-matrix honesty anchors for TRUST-03/04 and ACCEPT-06 map.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MATRIX="$ROOT/docs/capability-matrix.md"

if [[ ! -f "$MATRIX" ]]; then
	printf 'missing %s\n' "$MATRIX" >&2
	exit 1
fi

fail=0
require() {
	local label="$1"
	local pattern="$2"
	if ! grep -Eq "$pattern" "$MATRIX"; then
		printf 'MISSING anchor (%s): /%s/\n' "$label" "$pattern" >&2
		fail=1
	fi
}

# TRUST-04 triad
require "Aurora CreateDBCluster" 'CreateDBCluster'
require "Aurora free-tier block reason" 'Free-tier API block'
require "amazon-mq × preview" 'amazon-mq'
require "amazon-mq AZ incompatibility" 'CLUSTER_MULTI_AZ'
require "OpenSearch SigV4" 'SigV4'
require "OpenSearch deferred paid" 'not free-tier certifiable|deferred paid'

# queueMode:db must carry an evidence tier (TRUST-03)
require "queueMode db tier row" 'queueMode: db'
require "Evidence tier column header" 'Evidence tier'
require "Unverifiable section" 'Unverifiable on maintainer accounts'

# ACCEPT-06 foundation — Day-2 port coverage map
require "Day-2 port coverage heading" 'Day-2 port coverage'
require "Bootstrap port row" 'Bootstrap'
require "State port row" 'State'
require "Secrets port row" 'Secrets'
require "TailLogs port row" 'TailLogs'
require "CheckRuntime port row" 'CheckRuntime'
require "PrepareExec port row" 'PrepareExec'
require "AcquireLock port row" 'AcquireLock'

if [[ "$fail" -ne 0 ]]; then
	printf 'matrix_tier_guard_test FAILED\n' >&2
	exit 1
fi

printf 'matrix_tier_guard_test OK\n'
