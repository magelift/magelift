#!/usr/bin/env bash
# Offline shape checks for the bounded New Relic data-plane acceptance probe.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SCRIPT="$ROOT/scripts/newrelic-acceptance-local.sh"

[[ -x "$SCRIPT" ]] || { printf 'New Relic acceptance script is not executable\n' >&2; exit 1; }
grep -q 'MAGELIFT_NEWRELIC_ACCEPTANCE' "$SCRIPT"
grep -q 'events post' "$SCRIPT"
grep -q 'nrql query' "$SCRIPT"
grep -q 'mageliftAcceptanceMarker' "$SCRIPT"
grep -q 'provider-managed' "$SCRIPT"

output="$(MAGELIFT_NEWRELIC_ACCEPTANCE=1 MAGELIFT_ACCEPTANCE_DRY_RUN=1 MAGELIFT_NEWRELIC_ACCEPTANCE_RUN_ID=offline-shape bash "$SCRIPT")"
[[ "$output" == *"newrelic acceptance dry-run ok"* ]]

printf 'newrelic_harness_shape_test OK\n'
