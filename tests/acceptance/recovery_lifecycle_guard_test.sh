#!/usr/bin/env bash
# Offline contract: every disposable recovery wrapper has a shared TTL
# watchdog installed after its cleanup trap and stopped before cleanup work.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

wrappers=(
	 scripts/aws-recovery-acceptance-local.sh
	 scripts/aws-database-recovery-acceptance-local.sh
	 scripts/aws-secret-recovery-acceptance-local.sh
	 scripts/aws-sqs-acceptance-local.sh
	 providers/gcp/scripts/gcp-recovery-acceptance-local.sh
	 providers/gcp/scripts/gcp-secret-recovery-acceptance-local.sh
	 scripts/scaleway-recovery-acceptance-local.sh
	 scripts/scaleway-database-recovery-acceptance-local.sh
	 scripts/ovh-recovery-acceptance-local.sh
)

for relative in "${wrappers[@]}"; do
	file="$ROOT/$relative"
	bash -n "$file"
	grep -Fq 'source "$ROOT/scripts/acceptance/lib-lifecycle.sh"' "$file" || {
		printf '%s does not source the shared lifecycle helper\n' "$relative" >&2
		exit 1
	}
	grep -Fq 'acceptance_prepare_lifecycle' "$file" || {
		printf '%s does not prepare a bounded lifecycle\n' "$relative" >&2
		exit 1
	}
	grep -Fq 'acceptance_start_ttl_watchdog "$ACCEPTANCE_TTL_SECONDS" "$ttl_marker"' "$file" || {
		printf '%s does not start the TTL watchdog\n' "$relative" >&2
		exit 1
	}
	grep -Fq 'acceptance_stop_ttl_watchdog || true' "$file" || {
		printf '%s does not stop the TTL watchdog during cleanup\n' "$relative" >&2
		exit 1
	}
	trap_line="$(grep -n '^trap cleanup EXIT$' "$file" | head -n 1 | cut -d: -f1)"
	start_line="$(grep -n 'acceptance_start_ttl_watchdog' "$file" | tail -n 1 | cut -d: -f1)"
	if [[ -z "$trap_line" || -z "$start_line" || "$start_line" -le "$trap_line" ]]; then
		printf '%s must install the cleanup trap before starting the watchdog\n' "$relative" >&2
		exit 1
	fi
	stop_line="$(grep -n 'acceptance_stop_ttl_watchdog || true' "$file" | head -n 1 | cut -d: -f1)"
	cleanup_line="$(grep -n '^cleanup()' "$file" | head -n 1 | cut -d: -f1)"
	if [[ -z "$stop_line" || -z "$cleanup_line" || "$stop_line" -le "$cleanup_line" ]]; then
		printf '%s must stop the watchdog inside cleanup\n' "$relative" >&2
		exit 1
	fi
done

printf 'recovery_lifecycle_guard_test OK wrappers=%s\n' "${#wrappers[@]}"
