#!/usr/bin/env bash
# Offline tests for the shared disposable-run TTL guard.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

source "$ROOT/scripts/acceptance/lib-lifecycle.sh"

MAGELIFT_ACCEPTANCE_TTL_SECONDS=600
acceptance_prepare_lifecycle
[[ "$ACCEPTANCE_TTL_SECONDS" == 600 ]]
[[ "$ACCEPTANCE_EXPIRES_AT" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$ ]]

if MAGELIFT_ACCEPTANCE_TTL_SECONDS=0 acceptance_ttl_seconds >/dev/null 2>&1; then
	printf 'zero TTL must be rejected\n' >&2
	exit 1
fi
if MAGELIFT_ACCEPTANCE_TTL_SECONDS=86401 acceptance_ttl_seconds >/dev/null 2>&1; then
	printf 'TTL above the safety ceiling must be rejected\n' >&2
	exit 1
fi

acceptance_require_local_free_space 1 "$ROOT" >/dev/null
if acceptance_require_local_free_space 999999999999999999999999 "$ROOT" >/dev/null 2>&1; then
	printf 'an impossibly large free-space threshold must be rejected\n' >&2
	exit 1
fi
if acceptance_require_local_free_space invalid "$ROOT" >/dev/null 2>&1; then
	printf 'an invalid free-space threshold must be rejected\n' >&2
	exit 1
fi
if acceptance_require_local_free_space 1 >/dev/null 2>&1; then
	printf 'a missing free-space path must be rejected\n' >&2
	exit 1
fi
if (
	df() {
		printf 'Filesystem 1024-blocks Used Available Capacity Mounted on\n'
		printf '/dev/mock 1024 512 invalid 50%% /mock\n'
	}
	acceptance_require_local_free_space 1 "$ROOT"
) >/dev/null 2>&1; then
	printf 'invalid df output must be rejected\n' >&2
	exit 1
fi

marker="$TMP/expired"
acceptance_start_ttl_watchdog 60 "$marker"
acceptance_stop_ttl_watchdog
[[ ! -e "$marker" ]]
: >"$marker"
ACCEPTANCE_TTL_MARKER_FILE="$marker"
acceptance_ttl_expired

printf 'lifecycle_guard_test OK\n'
