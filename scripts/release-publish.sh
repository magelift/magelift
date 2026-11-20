#!/usr/bin/env bash
set -Eeuo pipefail

if [[ $# -ne 1 || ! "$1" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
	printf 'usage: release-publish.sh vX.Y.Z[-prerelease]\n' >&2
	exit 2
fi

tag="$1"
gh_bin="${MAGELIFT_GH:-gh}"
latest=false
version_without_build=${tag%%+*}
if [[ "$version_without_build" != *-* ]]; then
	latest=true
fi

"$gh_bin" release edit "$tag" --draft=false --latest="$latest"
