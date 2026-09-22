#!/usr/bin/env bash
# Commit-addressed canary ref. The release workflow only runs on v* tags,
# and provider signatures are bound to refs/tags/<that ref>.
# shellcheck shell=bash

# canary_ref_for_sha SHA
# Prints v0.0.0-canary.<40 lowercase hex> or returns 1.
canary_ref_for_sha() {
	local sha="${1:-}"
	if [[ ! "$sha" =~ ^[0-9a-f]{40}$ ]]; then
		printf 'commit SHA must be 40 lowercase hexadecimal characters\n' >&2
		return 1
	fi
	printf 'v0.0.0-canary.%s\n' "$sha"
}
