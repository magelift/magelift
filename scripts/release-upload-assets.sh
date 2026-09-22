#!/usr/bin/env bash
# Upload release assets.
# v0.0.0-canary.* never uses --clobber: a missing asset is uploaded, an
# identical file is left in place, and a different digest fails the upload.
set -euo pipefail

if [[ $# -lt 2 ]]; then
	printf 'usage: release-upload-assets.sh TAG FILE...\n' >&2
	exit 2
fi

tag="$1"
shift
gh_bin="${MAGELIFT_GH:-gh}"

if [[ "$tag" != v0.0.0-canary.* ]]; then
	"$gh_bin" release upload "$tag" "$@" --clobber
	exit 0
fi

digest() {
	sha256sum "$1" | awk '{print $1}'
}

mapfile -t existing < <("$gh_bin" release view "$tag" --json assets --jq '.assets[]?.name')

has_asset() {
	local name="$1" item
	for item in "${existing[@]}"; do
		if [[ "$item" == "$name" ]]; then
			return 0
		fi
	done
	return 1
}

for file in "$@"; do
	if [[ ! -f "$file" ]]; then
		printf 'missing file: %s\n' "$file" >&2
		exit 1
	fi
	name="$(basename "$file")"
	if ! has_asset "$name"; then
		"$gh_bin" release upload "$tag" "$file"
		continue
	fi
	downloaded="$(mktemp)"
	"$gh_bin" release download "$tag" --pattern "$name" --output "$downloaded"
	if [[ "$(digest "$file")" != "$(digest "$downloaded")" ]]; then
		rm -f "$downloaded"
		printf 'canary asset %s already exists with a different digest\n' "$name" >&2
		exit 1
	fi
	rm -f "$downloaded"
done
