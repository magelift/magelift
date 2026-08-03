#!/usr/bin/env bash
# Fail if vendored sibling PaaS / Adobe cloud tooling trees appear in the working tree.
# Clean-room policy: MageLift reimplements from public docs + behavioral evidence only
# (IMPORT-06 / docs/provenance.md). Do not vendor ece-tools, magento-cloud-patches,
# or Adobe Commerce Cloud CLI source dumps.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# Directory-name segments that indicate a forbidden vendored tree.
FORBIDDEN_NAMES='ece-tools|magento-cloud-patches|magento-cloud-docker|cloud-cli|adobe-commerce-cloud-cli|ece-patches|quality-patches|magento-ece-tools'

is_allowlisted() {
	case "$1" in
		docs/*|.agents/*|AGENTS.md|NOTICE|README.md|scripts/check-clean-room.sh)
			return 0
			;;
	esac
	return 1
}

hits=""

# Tracked + untracked files (respect .gitignore for untracked).
while IFS= read -r path; do
	[ -z "$path" ] && continue
	if is_allowlisted "$path"; then
		continue
	fi
	# Match only as a path segment (.../ece-tools/... or ece-tools/...), not a filename substring.
	if printf '%s\n' "$path" | grep -Eq "(^|/)(${FORBIDDEN_NAMES})(/|$)"; then
		hits="${hits}${path}"$'\n'
	fi
done <<EOF
$(git ls-files; git ls-files -o --exclude-standard)
EOF

# Walk for untracked/ignored dump directories that git ls-files may omit.
while IFS= read -r path; do
	[ -z "$path" ] && continue
	rel="${path#./}"
	if is_allowlisted "$rel"; then
		continue
	fi
	hits="${hits}${rel}/ (vendored tree name)"$'\n'
done <<EOF
$(find . \( \
	-path './.git' -o -path './.git/*' -o \
	-path './vendor' -o -path './vendor/*' -o \
	-path './build/vendor' -o -path './build/vendor/*' -o \
	-path './node_modules' -o -path './node_modules/*' -o \
	-path './dist' -o -path './dist/*' \
\) -prune -o -type d -print 2>/dev/null \
	| sed 's|^\./||' \
	| grep -E "(^|/)(${FORBIDDEN_NAMES})(/|\$)" || true)
EOF

if [ -n "$hits" ]; then
	uniq_hits=$(printf '%s' "$hits" | sed '/^$/d' | sort -u)
	if [ -n "$uniq_hits" ]; then
		printf 'check-clean-room: forbidden vendored PaaS/Adobe cloud trees detected:\n' >&2
		printf '%s\n' "$uniq_hits" | sed 's/^/  /' >&2
		printf 'Remove these paths or keep citations under docs/ only (IMPORT-06).\n' >&2
		exit 1
	fi
fi

printf 'check-clean-room: ok (no vendored ece-tools / magento-cloud-patches / ACC cli dumps)\n'
exit 0
