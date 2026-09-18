#!/usr/bin/env bash
# Packed-campaign isolation: one git worktree, one resource prefix,
# GOMEMLIMIT at 75% of available RAM, and Pulumi state that is not
# inherited from a sibling provider tree.
# shellcheck shell=bash

_campaign_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../go-memlimit.sh
source "$_campaign_root/scripts/go-memlimit.sh"

acceptance_campaign_go_memlimit() {
	magelift_apply_go_memlimit
	printf '+ campaign isolation GOMEMLIMIT=%s\n' "$GOMEMLIMIT"
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
