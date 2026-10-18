#!/usr/bin/env bash
# Canary refs share the dialproof release lane and never clobber an asset.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORKFLOW="$ROOT/.github/workflows/release.yml"
UPLOAD="$ROOT/scripts/release-upload-assets.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

if ! grep -Fq "startsWith(github.ref_name, 'v0.0.0-canary.')" "$WORKFLOW"; then
	printf 'release.yml does not treat v0.0.0-canary. as the slim lane\n' >&2
	exit 1
fi
if grep -Fq -- '--clobber' "$WORKFLOW"; then
	printf 'release.yml still uploads with --clobber\n' >&2
	exit 1
fi
if ! grep -Fq 'scripts/release-upload-assets.sh' "$WORKFLOW"; then
	printf 'release.yml does not call release-upload-assets.sh\n' >&2
	exit 1
fi

mkdir -p "$TMP/bin"
cat >"$TMP/bin/gh" <<EOF
#!/bin/sh
printf '%s\n' "\$*" >> "$TMP/gh.log"
if [ "\$1" = release ] && [ "\$2" = view ]; then
	printf '%s\n' "\$GH_EXISTING"
	exit 0
fi
if [ "\$1" = release ] && [ "\$2" = download ]; then
	prev=
	for arg in "\$@"; do
		if [ "\$prev" = --output ]; then
			cp "\$GH_PAYLOAD" "\$arg"
			exit 0
		fi
		prev=\$arg
	done
	printf 'stub gh: download missing --output\n' >&2
	exit 1
fi
exit 0
EOF
chmod +x "$TMP/bin/gh"

run_with_stub() {
	GH_EXISTING="$1" GH_PAYLOAD="$2" MAGELIFT_GH="$TMP/bin/gh" bash "$UPLOAD" "$3" "$4"
}

printf 'same-bytes\n' >"$TMP/asset"
tag='v0.0.0-canary.aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
name="$(basename "$TMP/asset")"

: >"$TMP/gh.log"
if ! run_with_stub "$name" "$TMP/asset" "$tag" "$TMP/asset"; then
	printf 'identical canary asset was rejected\n' >&2
	exit 1
fi
if grep -q -- '--clobber' "$TMP/gh.log" || grep -q 'release upload' "$TMP/gh.log"; then
	printf 'identical canary asset was replaced: %s\n' "$(cat "$TMP/gh.log")" >&2
	exit 1
fi

printf 'other-bytes\n' >"$TMP/other"
: >"$TMP/gh.log"
if run_with_stub "$name" "$TMP/other" "$tag" "$TMP/asset"; then
	printf 'different canary digest was accepted\n' >&2
	exit 1
fi
if grep -q -- '--clobber' "$TMP/gh.log"; then
	printf 'rejected canary upload used --clobber\n' >&2
	exit 1
fi

: >"$TMP/gh.log"
if ! run_with_stub "" "$TMP/asset" "$tag" "$TMP/asset"; then
	printf 'missing canary asset was not uploaded\n' >&2
	exit 1
fi
if ! grep -q 'release upload' "$TMP/gh.log" || grep -q -- '--clobber' "$TMP/gh.log"; then
	printf 'missing canary upload was wrong: %s\n' "$(cat "$TMP/gh.log")" >&2
	exit 1
fi

: >"$TMP/gh.log"
if ! run_with_stub "" "$TMP/asset" 'v0.0.0-dialproof.1' "$TMP/asset"; then
	printf 'dialproof upload failed\n' >&2
	exit 1
fi
if ! grep -q -- '--clobber' "$TMP/gh.log"; then
	printf 'non-canary upload did not keep --clobber\n' >&2
	exit 1
fi

DEVLOOP_WORKFLOW="$ROOT/.github/workflows/alpha-development-loop.yml"
RUN_GATES="$ROOT/scripts/development-loop/run-gates.sh"
if ! grep -q 'Operator actions: pushing and deleting v0.0.0-canary.<sha>' "$DEVLOOP_WORKFLOW"; then
	printf 'alpha-development-loop workflow missing operator-action header\n' >&2
	exit 1
fi
if ! grep -q 'Operator actions: pushing and deleting v0.0.0-canary.<sha>' "$RUN_GATES"; then
	printf 'run-gates missing operator-action header\n' >&2
	exit 1
fi
if grep -Eq 'git push|gh release create' "$DEVLOOP_WORKFLOW"; then
	printf 'alpha-development-loop workflow must not push tags or create releases\n' >&2
	exit 1
fi

printf 'release_canary_predicate_test OK\n'
