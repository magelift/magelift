#!/usr/bin/env bash
# Packed-campaign isolation: one git worktree, one resource prefix, serial Go,
# and Pulumi state that is not inherited from a sibling provider tree.
# shellcheck shell=bash

acceptance_campaign_serial_go() {
	export GOMAXPROCS=1
	local flags="${GOFLAGS:-}"
	case " ${flags} " in
	*" -p=1 "* | *" -p 1 "*) ;;
	*)
		if [[ -n "$flags" ]]; then
			flags="${flags} -p=1"
		else
			flags="-p=1"
		fi
		;;
	esac
	export GOFLAGS="$flags"
	export GOMEMLIMIT="${GOMEMLIMIT:-1GiB}"
	printf '+ campaign isolation serial Go GOMAXPROCS=%s GOFLAGS=%s GOMEMLIMIT=%s\n' \
		"$GOMAXPROCS" "$GOFLAGS" "$GOMEMLIMIT"
}

acceptance_campaign_prefix_is_reserved() {
	local prefix="${1:-}"
	case "$prefix" in
	'' | mlacc | shared | default | magelift | test | tmp) return 0 ;;
	esac
	return 1
}

acceptance_campaign_require_prefix() {
	local provider="${1:?provider}"
	local prefix="${2:-}"
	if [[ -z "$prefix" ]]; then
		printf 'campaign isolation requires a unique %s resource prefix for this worktree\n' "$provider" >&2
		return 1
	fi
	if acceptance_campaign_prefix_is_reserved "$prefix"; then
		printf 'campaign isolation refuses reserved %s prefix %q; pick a unique worktree prefix\n' "$provider" "$prefix" >&2
		return 1
	fi
	printf '+ campaign isolation prefix provider=%s prefix=%s\n' "$provider" "$prefix"
}

# Live runs must not inherit a sibling worktree's Pulumi backend. Pass the
# explicit MAGELIFT_*_ACCEPTANCE_BACKEND_URL (may be empty). A non-empty
# ambient PULUMI_BACKEND_URL without that explicit override is a leak.
acceptance_campaign_require_isolated_backend() {
	local explicit_backend="${1:-}"
	if [[ -z "$explicit_backend" && -n "${PULUMI_BACKEND_URL:-}" ]]; then
		printf 'campaign isolation refuses an inherited PULUMI_BACKEND_URL; set this worktree MAGELIFT_*_ACCEPTANCE_BACKEND_URL or use the harness state dir\n' >&2
		return 1
	fi
}
