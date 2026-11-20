#!/usr/bin/env bash
# Offline community-provider proof: the public extension contract builds and
# runs with isolated module and compiler caches.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOG="$(mktemp)"
trap 'rm -f "$LOG"' EXIT

if ! bash "$ROOT/scripts/custom-extension-clean-room.sh" >"$LOG" 2>&1; then
	cat "$LOG" >&2
	exit 1
fi
grep -q '^example\.community-contract$' "$LOG"
printf 'custom_extension_clean_cache_test OK\n'
