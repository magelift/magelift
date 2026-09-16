#!/usr/bin/env bash
# Offline contract: live acceptance entrypoints have a shared TTL watchdog
# installed after cleanup ownership and stopped before provider cleanup.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
wrappers=(
	scripts/aws-cloudwatch-acceptance-local.sh
	scripts/aws-collector-acceptance-local.sh
	providers/gcp/scripts/gcp-collector-acceptance-local.sh
	providers/gcp/scripts/gcp-cloudsql-acceptance-local.sh
	providers/gcp/scripts/gcp-observability-acceptance-local.sh
	providers/gcp/scripts/gcp-pubsub-acceptance-local.sh
	scripts/scaleway-observability-acceptance-local.sh
	scripts/scaleway-secret-recovery-acceptance-local.sh
)

for relative in "${wrappers[@]}"; do
	file="$ROOT/$relative"
	bash -n "$file"
	for required in \
		'source "$ROOT/scripts/acceptance/lib-lifecycle.sh"' \
		'acceptance_prepare_lifecycle' \
		'acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"' \
		'acceptance_stop_ttl_watchdog || true'; do
		grep -Fq "$required" "$file" || {
			printf '%s missing lifecycle contract: %s\n' "$relative" "$required" >&2
			exit 1
		}
	done
	trap_line="$(rg -n '^trap (cleanup|cloudwatch_cleanup) EXIT' "$file" | head -n 1 | cut -d: -f1)"
	start_line="$(rg -n 'acceptance_start_ttl_watchdog' "$file" | tail -n 1 | cut -d: -f1)"
	if [[ -z "$trap_line" || -z "$start_line" || "$start_line" -le "$trap_line" ]]; then
		printf '%s must install cleanup before starting the watchdog\n' "$relative" >&2
		exit 1
	fi
done

printf 'acceptance_lifecycle_guard_test OK wrappers=%s\n' "${#wrappers[@]}"
