#!/usr/bin/env bash
# The promotion gate passes green checks, fails failed ones, and refuses
# to promote commits with missing checks.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GATE="$ROOT/scripts/release-checks-gate.sh"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/magelift-gate-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

write_payload() {
	python3 - "$WORK/checks.json" "$1" <<'PY'
import json
import sys

runs = [
    {"name": "lint", "status": "completed", "conclusion": "success", "completed_at": "2026-09-17T10:00:00Z"},
    {"name": "go-verify", "status": "completed", "conclusion": "success", "completed_at": "2026-09-17T10:05:00Z"},
    {"name": "sdk-verify", "status": "completed", "conclusion": "success", "completed_at": "2026-09-17T10:06:00Z"},
    {"name": "provider-verify", "status": "completed", "conclusion": sys.argv[2], "completed_at": "2026-09-17T10:07:00Z"},
    {"name": "floci-gcp", "status": "completed", "conclusion": "success", "completed_at": "2026-09-17T10:08:00Z"},
    {"name": "php", "status": "completed", "conclusion": "skipped", "completed_at": "2026-09-17T10:09:00Z"},
    {"name": "unrelated", "status": "completed", "conclusion": "failure", "completed_at": "2026-09-17T10:10:00Z"},
]
with open(sys.argv[1], "w") as handle:
    json.dump({"check_runs": runs}, handle)
PY
}

# 1. Green checks (plus an ignored unrelated failure and a skipped job) pass.
write_payload "success"
if ! "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/pass.out" 2>&1; then
	cat "$WORK/pass.out" >&2
	printf 'gate rejected a green commit\n' >&2
	exit 1
fi
grep -q "PASS     provider-verify: success" "$WORK/pass.out" || {
	printf 'gate hid the provider verdict\n' >&2
	exit 1
}

# 2. A failing provider-only test blocks promotion, even when the root
# suite is green.
write_payload "failure"
if "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/fail.out" 2>&1; then
	printf 'gate promoted despite provider-verify failure\n' >&2
	exit 1
fi
grep -q "BLOCKED  provider-verify: conclusion=failure" "$WORK/fail.out" || {
	printf 'gate did not name the failing check\n' >&2
	exit 1
}

# 3. Missing checks block promotion (tag must point at a CI-green commit).
python3 -c "import json; json.dump({'check_runs': []}, open('$WORK/empty.json', 'w'))"
if "$GATE" --sha abc123 --checks-json "$WORK/empty.json" >"$WORK/missing.out" 2>&1; then
	printf 'gate promoted a commit with no checks\n' >&2
	exit 1
fi
grep -q "BLOCKED  provider-verify: no check run" "$WORK/missing.out" || {
	printf 'gate did not flag missing checks\n' >&2
	exit 1
}

# 4. Custom check lists and missing flags behave.
if ! "$GATE" --sha abc123 --checks-json "$WORK/checks.json" lint >"$WORK/custom.out" 2>&1; then
	printf 'gate rejected a custom green list\n' >&2
	exit 1
fi
if "$GATE" --checks-json "$WORK/checks.json" >/dev/null 2>&1; then
	printf 'gate accepted a missing --sha\n' >&2
	exit 1
fi

printf 'release checks gate ok\n'
