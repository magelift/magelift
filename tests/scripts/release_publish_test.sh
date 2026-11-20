#!/usr/bin/env bash
set -Eeuo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mock="$work/gh"
cat >"$mock" <<'MOCK'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$RELEASE_PUBLISH_CALLS"
MOCK
chmod +x "$mock"
export MAGELIFT_GH="$mock"
export RELEASE_PUBLISH_CALLS="$work/calls"

"$root/scripts/release-publish.sh" v1.0.0
"$root/scripts/release-publish.sh" v1.2.3-rc.1
"$root/scripts/release-publish.sh" v1.2.3+build-linux
mapfile -t calls <"$RELEASE_PUBLISH_CALLS"
[[ "${calls[0]}" == 'release edit v1.0.0 --draft=false --latest=true' ]]
[[ "${calls[1]}" == 'release edit v1.2.3-rc.1 --draft=false --latest=false' ]]
[[ "${calls[2]}" == 'release edit v1.2.3+build-linux --draft=false --latest=true' ]]

if "$root/scripts/release-publish.sh" v1.0 2>/dev/null; then
	printf 'invalid release tag was accepted\n' >&2
	exit 1
fi
printf 'release publish channel tests passed\n'
