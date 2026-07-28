#!/usr/bin/env bash
# Guard: capability-matrix honesty anchors for TRUST-03/04.
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

require "Aurora CreateDBCluster" 'CreateDBCluster'
require "Aurora free-tier block reason" 'Free-tier API block'
require "queueMode db cell" 'queueMode: db'
require "Evidence tier column header" 'Evidence tier'
require "Unverifiable section" 'Unverifiable on maintainer accounts'

if [[ "$fail" -ne 0 ]]; then
	printf 'matrix_tier_guard_test FAILED\n' >&2
	exit 1
fi

printf 'matrix_tier_guard_test OK\n'
