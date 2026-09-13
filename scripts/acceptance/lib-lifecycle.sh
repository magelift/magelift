#!/usr/bin/env bash
# Shared lifecycle guard for disposable acceptance runs.
# shellcheck shell=bash

ACCEPTANCE_TTL_DEFAULT_SECONDS=21600
ACCEPTANCE_TTL_MAX_SECONDS=86400
ACCEPTANCE_TTL_WATCHDOG_PID=""
ACCEPTANCE_TTL_MARKER_FILE=""

acceptance_require_local_free_space() {
	local LC_ALL=C
	local required_mib="${1:-}"
	local path="${2:-}"
	local df_output
	local available_mib
	local insufficient=0

	if [[ "$#" -ne 2 ]]; then
		printf 'usage: acceptance_require_local_free_space <required-mib> <path>\n' >&2
		return 1
	fi
	if [[ ! "$required_mib" =~ ^[1-9][0-9]*$ ]]; then
		printf 'local free-space preflight requires a positive integer MiB threshold: %s\n' \
			"${required_mib:-<empty>}" >&2
		return 1
	fi
	if [[ -z "$path" ]]; then
		printf 'local free-space preflight requires a non-empty path\n' >&2
		return 1
	fi

	if ! df_output="$(df -Pk "$path" 2>&1)"; then
		printf 'local free-space preflight could not query %s with df -Pk; check that the path exists and is accessible: %s\n' \
			"$path" "$df_output" >&2
		return 1
	fi
	if ! available_mib="$(
		printf '%s\n' "$df_output" |
			awk '
				NR == 2 {
					if (NF < 6 || $4 !~ /^[0-9]+$/) {
						invalid=1
					} else {
						available_mib = int($4 / 1024)
						found=1
					}
				}
				NR > 2 { invalid=1 }
				END {
					if (invalid || !found) {
						exit 1
					}
					print available_mib
				}
			'
	)"; then
		printf 'local free-space preflight received invalid df -Pk output for %s; refusing to continue\n' \
			"$path" >&2
		return 1
	fi
	if [[ ! "$available_mib" =~ ^[0-9]+$ ]]; then
		printf 'local free-space preflight received an invalid available-space value for %s; refusing to continue\n' \
			"$path" >&2
		return 1
	fi

	if (( ${#available_mib} < ${#required_mib} )); then
		insufficient=1
	elif (( ${#available_mib} == ${#required_mib} )) && [[ "$available_mib" < "$required_mib" ]]; then
		insufficient=1
	fi
	if (( insufficient != 0 )); then
		printf 'local free-space preflight failed for %s: %s MiB available, %s MiB required; reclaim local disk space or choose another filesystem before retrying\n' \
			"$path" "$available_mib" "$required_mib" >&2
		return 1
	fi

	printf '+ local free-space preflight verified path=%s available=%sMiB required=%sMiB\n' \
		"$path" "$available_mib" "$required_mib"
}

acceptance_seed_dump_stream() {
	local dump_path="${1:?seed dump path required}"
	local lower_path
	lower_path="$(printf '%s' "$dump_path" | tr '[:upper:]' '[:lower:]')"
	case "$lower_path" in
	*.sql)
		awk '{ print }' "$dump_path"
		;;
	*.sql.gz)
		gzip -cd -- "$dump_path"
		;;
	*)
		printf 'live Magento seed dump must be a .sql or .sql.gz file: %s\n' "$dump_path" >&2
		return 1
		;;
	esac
}

acceptance_validate_magento_seed_dump() {
	local dump_path="${1:-}"
	local lower_path
	local pipefail_was_set=0
	local stream_status

	if [[ -z "$dump_path" || ! -f "$dump_path" || ! -r "$dump_path" ]]; then
		printf 'live Magento seed dump is missing or unreadable: %s\n' "${dump_path:-<empty>}" >&2
		return 1
	fi

	lower_path="$(printf '%s' "$dump_path" | tr '[:upper:]' '[:lower:]')"
	case "$lower_path" in
	*.sql)
		;;
	*.sql.gz)
		if ! gzip -t -- "$dump_path" >/dev/null 2>&1; then
			printf 'live Magento seed dump is not a valid gzip stream: %s\n' "$dump_path" >&2
			return 1
		fi
		;;
	*)
		printf 'live Magento seed dump must be a .sql or .sql.gz file: %s\n' "$dump_path" >&2
		return 1
		;;
	esac

	if [[ "$(set -o | awk '$1 == "pipefail" { print $2 }')" == on ]]; then
		pipefail_was_set=1
	fi
	set -o pipefail
	if acceptance_seed_dump_stream "$dump_path" | awk '
function identifier(value) {
	 gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
	 gsub(/[`"\r]/, "", value)
	 sub(/^.*\./, "", value)
	 return tolower(value)
}

{
 line = tolower($0)
 if (line !~ /^[[:space:]]*create[[:space:]]+(temporary[[:space:]]+)?table[[:space:]]+/) {
  next
 }
 sub(/^[[:space:]]*create[[:space:]]+(temporary[[:space:]]+)?table[[:space:]]+/, "", line)
 sub(/^if[[:space:]]+not[[:space:]]+exists[[:space:]]+/, "", line)
 sub(/[[:space:](].*$/, "", line)
 name = identifier(line)
 if (name == "flag" || name ~ /_flag$/) {
  seen["flag"] = 1
 }
 if (name == "setup_module" || name ~ /_setup_module$/) {
  seen["setup_module"] = 1
 }
 if (name == "core_config_data" || name ~ /_core_config_data$/) {
  seen["core_config_data"] = 1
 }
}

END {
 exit !(seen["flag"] && seen["setup_module"] && seen["core_config_data"])
}'
	then
		stream_status=0
	else
		stream_status=$?
	fi
	if [[ "$pipefail_was_set" -eq 0 ]]; then
		set +o pipefail
	fi
	if [[ "$stream_status" -ne 0 ]]; then
		printf 'live Magento seed dump must contain installed schema tables: flag, setup_module, core_config_data (rows are not required): %s\n' "$dump_path" >&2
		return 1
	fi

	printf '+ Magento seed dump contract verified path=%s (values not logged)\n' "$dump_path" >&2
}

acceptance_ttl_seconds() {
	local value="${MAGELIFT_ACCEPTANCE_TTL_SECONDS:-$ACCEPTANCE_TTL_DEFAULT_SECONDS}"
	if [[ ! "$value" =~ ^[1-9][0-9]*$ ]] || (( ${#value} > 5 )) || (( value > ACCEPTANCE_TTL_MAX_SECONDS )); then
		printf 'MAGELIFT_ACCEPTANCE_TTL_SECONDS must be a positive integer no greater than %s\n' \
			"$ACCEPTANCE_TTL_MAX_SECONDS" >&2
		return 1
	fi
	printf '%s' "$value"
}

acceptance_ttl_expiration() {
	local ttl="${1:?TTL seconds required}"
	local expires
	if expires="$(date -u -d "+${ttl} seconds" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"; then
		printf '%s' "$expires"
		return 0
	fi
	if expires="$(date -u -v+"${ttl}"S +%Y-%m-%dT%H:%M:%SZ 2>/dev/null)"; then
		printf '%s' "$expires"
		return 0
	fi
	printf 'unable to calculate an acceptance expiration timestamp\n' >&2
	return 1
}

acceptance_prepare_lifecycle() {
	ACCEPTANCE_TTL_SECONDS="$(acceptance_ttl_seconds)" || return 1
	ACCEPTANCE_EXPIRES_AT="$(acceptance_ttl_expiration "$ACCEPTANCE_TTL_SECONDS")" || return 1
	export ACCEPTANCE_TTL_SECONDS ACCEPTANCE_EXPIRES_AT
}

acceptance_start_ttl_watchdog() {
	local ttl="${1:-${ACCEPTANCE_TTL_SECONDS:-}}"
	local marker="${2:-${ACCEPTANCE_TTL_MARKER_FILE:-}}"
	if [[ ! "$ttl" =~ ^[1-9][0-9]*$ ]]; then
		printf 'acceptance TTL watchdog requires a positive integer\n' >&2
		return 1
	fi
	acceptance_stop_ttl_watchdog
	ACCEPTANCE_TTL_MARKER_FILE="$marker"
	(
		sleep "$ttl"
		if [[ -n "$marker" ]]; then
			: >"$marker"
		fi
		printf 'acceptance TTL expired after %ss; forcing cleanup\n' "$ttl" >&2
		kill -TERM "$$" 2>/dev/null || true
	) &
	ACCEPTANCE_TTL_WATCHDOG_PID=$!
}

acceptance_stop_ttl_watchdog() {
	local pid="${ACCEPTANCE_TTL_WATCHDOG_PID:-}"
	local child
	if [[ -z "$pid" ]]; then
		return 0
	fi
	# Killing only the watchdog subshell can reparent `sleep` to PID 1 and
	# leave EXIT `wait` blocked for the rest of the TTL.
	while IFS= read -r child; do
		[[ -z "$child" ]] && continue
		kill "$child" 2>/dev/null || true
	done < <(pgrep -P "$pid" 2>/dev/null || true)
	kill "$pid" 2>/dev/null || true
	wait "$pid" 2>/dev/null || true
	ACCEPTANCE_TTL_WATCHDOG_PID=""
}

acceptance_ttl_expired() {
	[[ -n "${ACCEPTANCE_TTL_MARKER_FILE:-}" && -f "$ACCEPTANCE_TTL_MARKER_FILE" ]]
}
