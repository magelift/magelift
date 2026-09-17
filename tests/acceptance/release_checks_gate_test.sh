#!/usr/bin/env bash
# The promotion gate passes green checks, blocks failed, skipped, or
# missing checks, and reports pending reruns as pending (never pass).
# Exit codes: 0 pass, 1 blocked, 2 pending, 3 usage.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GATE="$ROOT/scripts/release-checks-gate.sh"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/magelift-gate-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

write_payload() {
	python3 - "$WORK/checks.json" <<'PY'
import json
import sys

runs = [
    {"name": "lint", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:01:00Z", "id": 1},
    {"name": "go-verify", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:05:00Z", "id": 2},
    {"name": "sdk-verify", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:06:00Z", "id": 3},
    {"name": "provider-verify", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:07:00Z", "id": 4},
    {"name": "floci-gcp", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:08:00Z", "id": 5},
    {"name": "php", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:09:00Z", "id": 6},
    {"name": "unrelated", "status": "completed", "conclusion": "failure", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:10:00Z", "id": 7},
]
with open(sys.argv[1], "w") as handle:
    json.dump({"check_runs": runs}, handle)
PY
}

mutate() {
	python3 - "$WORK/checks.json" "$@" <<'PY'
import json
import sys

path, name = sys.argv[1], sys.argv[2]
with open(path) as handle:
    data = json.load(handle)
runs = [run for run in data["check_runs"] if run["name"] != name]
extra = json.loads(sys.argv[3])
runs.extend(extra)
data["check_runs"] = runs
with open(path, "w") as handle:
    json.dump(data, handle)
PY
}

# 1. Green checks (plus an ignored unrelated failure) pass.
write_payload
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
mutate provider-verify '[{"name": "provider-verify", "status": "completed", "conclusion": "failure", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:07:00Z", "id": 4}]'
if "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/fail.out" 2>&1; then
	printf 'gate promoted despite provider-verify failure\n' >&2
	exit 1
fi
grep -q "BLOCKED  provider-verify: conclusion=failure" "$WORK/fail.out" || {
	printf 'gate did not name the failing check\n' >&2
	exit 1
}

# 3. A skipped check blocks: releases need actual verification.
mutate provider-verify '[{"name": "provider-verify", "status": "completed", "conclusion": "skipped", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:07:00Z", "id": 4}]'
if "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/skip.out" 2>&1; then
	printf 'gate promoted on a skipped check\n' >&2
	exit 1
fi
grep -q "BLOCKED  provider-verify: conclusion=skipped" "$WORK/skip.out" || {
	printf 'gate did not flag the skipped check\n' >&2
	exit 1
}

# 4. Missing checks with CI complete block with the dispatch hint.
python3 -c "import json; json.dump({'check_runs': []}, open('$WORK/empty.json', 'w'))"
if "$GATE" --sha abc123 --checks-json "$WORK/empty.json" >"$WORK/missing.out" 2>&1; then
	printf 'gate promoted a commit with no checks\n' >&2
	exit 1
fi
grep -q "BLOCKED  provider-verify: no check run" "$WORK/missing.out" || {
	printf 'gate did not flag missing checks\n' >&2
	exit 1
}
grep -q "gh workflow run ci.yml" "$WORK/missing.out" || {
	printf 'gate hid the full-CI dispatch hint\n' >&2
	exit 1
}

# 5. Missing checks with CI still active report pending, not blocked.
python3 -c "import json; json.dump({'check_runs': [{'name': 'lint', 'status': 'in_progress', 'conclusion': None, 'started_at': '2026-09-17T10:00:00Z', 'id': 1}]}, open('$WORK/active.json', 'w'))"
code=0
"$GATE" --sha abc123 --checks-json "$WORK/active.json" >"$WORK/active.out" 2>&1 || code="$?"
if [[ "$code" != "2" ]]; then
	printf 'gate exit = %s, want 2 (pending)\n' "$code" >&2
	exit 1
fi
grep -q "PENDING  provider-verify: no check run yet" "$WORK/active.out" || {
	printf 'gate did not report the pending check\n' >&2
	exit 1
}

# 6. An older success with a newer rerun in progress reports pending:
# the latest attempt wins.
write_payload
mutate provider-verify '[{"name": "provider-verify", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:07:00Z", "id": 4}, {"name": "provider-verify", "status": "in_progress", "conclusion": null, "started_at": "2026-09-17T11:00:00Z", "id": 14}]'
code=0
"$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/rerun.out" 2>&1 || code="$?"
if [[ "$code" != "2" ]]; then
	printf 'gate exit = %s, want 2 (pending rerun)\n' "$code" >&2
	exit 1
fi
grep -q "PENDING  provider-verify" "$WORK/rerun.out" || {
	printf 'gate did not flag the in-progress rerun\n' >&2
	exit 1
}

# 7. A failure repaired by a later success passes.
mutate provider-verify '[{"name": "provider-verify", "status": "completed", "conclusion": "failure", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:07:00Z", "id": 4}, {"name": "provider-verify", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T11:00:00Z", "completed_at": "2026-09-17T11:07:00Z", "id": 14}]'
if ! "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/repaired.out" 2>&1; then
	cat "$WORK/repaired.out" >&2
	printf 'gate rejected a repaired check\n' >&2
	exit 1
fi

# 8. Custom check lists and missing flags behave.
if ! "$GATE" --sha abc123 --checks-json "$WORK/checks.json" lint >"$WORK/custom.out" 2>&1; then
	printf 'gate rejected a custom green list\n' >&2
	exit 1
fi
code=0
"$GATE" --checks-json "$WORK/checks.json" >/dev/null 2>&1 || code="$?"
if [[ "$code" != "3" ]]; then
	printf 'gate exit = %s for a missing --sha, want 3\n' "$code" >&2
	exit 1
fi

# 9. A queued rerun without a start time supersedes its older success.
write_payload
mutate provider-verify '[{"name": "provider-verify", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:07:00Z", "id": 10}, {"name": "provider-verify", "status": "queued", "conclusion": null, "started_at": null, "id": 11}]'
code=0
"$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/queued.out" 2>&1 || code="$?"
if [[ "$code" != "2" ]]; then
	printf 'gate exit = %s, want 2 (queued rerun)\n' "$code" >&2
	exit 1
fi
grep -q "PENDING  provider-verify" "$WORK/queued.out" || {
	printf 'gate did not flag the queued rerun\n' >&2
	exit 1
}

# 10. A waiting rerun likewise reports pending, and its later success passes.
mutate provider-verify '[{"name": "provider-verify", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:07:00Z", "id": 10}, {"name": "provider-verify", "status": "waiting", "conclusion": null, "started_at": null, "id": 11}]'
code=0
"$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/waiting.out" 2>&1 || code="$?"
if [[ "$code" != "2" ]]; then
	printf 'gate exit = %s, want 2 (waiting rerun)\n' "$code" >&2
	exit 1
fi
mutate provider-verify '[{"name": "provider-verify", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T10:00:00Z", "completed_at": "2026-09-17T10:07:00Z", "id": 10}, {"name": "provider-verify", "status": "completed", "conclusion": "success", "started_at": "2026-09-17T11:00:00Z", "completed_at": "2026-09-17T11:07:00Z", "id": 11}]'
if ! "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/queuedone.out" 2>&1; then
	cat "$WORK/queuedone.out" >&2
	printf 'gate rejected a completed rerun\n' >&2
	exit 1
fi

# 11. The live fetch requests every attempt, not the latest-only view.
grep -q "check-runs?per_page=100&filter=all" "$GATE" || {
	printf 'gate live fetch lost filter=all\n' >&2
	exit 1
}

printf 'release checks gate ok\n'
