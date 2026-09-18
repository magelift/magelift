#!/usr/bin/env bash
# Promotion gate: fail unless one complete CI run on the commit is green.
# The release workflow calls this before publishing a draft candidate.
# Missing checks fail: tag a commit CI actually ran on, and dispatch
# full CI on the tag (gh workflow run ci.yml --ref <tag> -f all=true).
#
# Usage: release-checks-gate.sh --sha <sha> [--repo owner/name]
#        [--checks-json FILE] [--exclude-run ID] [check...]
# Default checks: CI passed, provider-verify, php (8.3), php (8.5).
# "CI passed" is the workflow aggregate (does not include
# provider-verify). PHP cells are the full-dispatch matrix; a
# truncated payload or a single cell is not enough. All required
# checks must succeed on the same workflow run so partial reruns
# cannot be stitched together.
# --checks-json evaluates one canned payload (or a JSON array of
# page payloads). Exit 0 pass, 1 blocked, 2 still pending.
# --exclude-run ignores check runs from one workflow run (the
# release itself) when deciding whether CI is still active.
set -Eeuo pipefail

REPO="magelift/magelift"
SHA=""
CHECKS_JSON=""
EXCLUDE_RUN=""
CHECKS=("CI passed" "provider-verify" "php (8.3)" "php (8.5)")
POLL_SECONDS="${GATE_POLL_SECONDS:-60}"
TIMEOUT_SECONDS="${GATE_TIMEOUT_SECONDS:-1800}"

while [[ $# -gt 0 ]]; do
	case "$1" in
	--sha)
		SHA="${2:?--sha needs a value}"
		shift 2
		;;
	--repo)
		REPO="${2:?--repo needs a value}"
		shift 2
		;;
	--checks-json)
		CHECKS_JSON="${2:?--checks-json needs a value}"
		shift 2
		;;
	--exclude-run)
		EXCLUDE_RUN="${2:?--exclude-run needs a value}"
		shift 2
		;;
	-h | --help)
		sed -n '2,17p' "$0"
		exit 0
		;;
	--* | "")
		printf 'release-checks-gate: unknown option %s\n' "$1" >&2
		exit 3
		;;
	*)
		break
		;;
	esac
done
if [[ $# -gt 0 ]]; then
	CHECKS=("$@")
fi
if [[ -z "$SHA" ]]; then
	printf 'release-checks-gate: --sha is required\n' >&2
	exit 3
fi

evaluate() {
	export GATE_PAYLOAD="$1"
	export GATE_CHECKS
	GATE_CHECKS="$(printf '%s\n' "${CHECKS[@]}")"
	export GATE_EXCLUDE_RUN="$EXCLUDE_RUN"
	python3 - <<'PY'
import json
import os
import re
import sys

try:
    raw = json.loads(os.environ["GATE_PAYLOAD"])
except (KeyError, ValueError) as exc:
    print(f"release-checks-gate: invalid check-runs payload: {exc}", file=sys.stderr)
    sys.exit(3)

if isinstance(raw, list):
    runs = []
    for page in raw:
        if not isinstance(page, dict):
            print("release-checks-gate: page payload must be an object", file=sys.stderr)
            sys.exit(3)
        runs.extend(page.get("check_runs") or [])
elif isinstance(raw, dict):
    runs = raw.get("check_runs") or []
else:
    print("release-checks-gate: payload must be an object or page list", file=sys.stderr)
    sys.exit(3)

wanted = [line for line in os.environ.get("GATE_CHECKS", "").splitlines() if line]


def matches(wanted_name, actual):
    # GitHub Actions matrix jobs are named "php (8.3)", not "php".
    # Require an exact name or the matrix suffix so "php" does not
    # accept "phpunit" or "frankenphp".
    return actual == wanted_name or actual.startswith(wanted_name + " (")


run_id_pattern = re.compile(r"/actions/runs/(\d+)/")


def run_id(run):
    match = run_id_pattern.search(run.get("html_url") or "")
    return match.group(1) if match else ""


exclude = os.environ.get("GATE_EXCLUDE_RUN", "").strip()


def own_run(run):
    return bool(exclude) and run_id(run) == exclude


# Latest attempt per (run, name): reruns get new ids.
latest = {}
for run in runs:
    if own_run(run):
        continue
    name = run.get("name") or ""
    rid = run_id(run)
    key = run.get("id") or 0
    slot = (rid, name)
    if slot not in latest or key >= latest[slot][0]:
        latest[slot] = (key, run)

by_run = {}
for (rid, name), entry in latest.items():
    by_run.setdefault(rid, {})[name] = entry[1]


def verdict(run):
    status = run.get("status") or ""
    conclusion = run.get("conclusion") or ""
    if status != "completed":
        return "pending", status or "unknown"
    if conclusion != "success":
        return "blocked", conclusion or "unknown"
    return "success", "success"


def required_on_run(jobs):
    pending = []
    blocked = []
    missing = []
    for name in wanted:
        matched = [(actual, jobs[actual]) for actual in jobs if matches(name, actual)]
        if not matched:
            missing.append(name)
            continue
        for actual, job in matched:
            state, detail = verdict(job)
            if state == "pending":
                pending.append((name, actual, detail, job.get("id", "?")))
            elif state == "blocked":
                blocked.append((name, actual, detail))
    return pending, blocked, missing


active = any(
    (run.get("status") or "") != "completed" and not own_run(run) for run in runs
)

# Prefer the newest run that has the aggregate "CI passed" when that
# check is required; otherwise the newest run that has any required job.
candidates = []
require_aggregate = any(item == "CI passed" for item in wanted)
for rid, jobs in by_run.items():
    if require_aggregate and "CI passed" not in jobs:
        continue
    candidates.append(rid)
candidates.sort(key=lambda rid: int(rid) if rid.isdigit() else -1, reverse=True)

if not candidates:
    if active:
        print("PENDING  CI passed: no complete validation run yet, CI still active")
        sys.exit(2)
    print("BLOCKED  CI passed: no check run on this commit (tag a CI-green commit, or dispatch full CI: gh workflow run ci.yml --ref <tag> -f all=true)")
    sys.exit(1)

code = 0
chosen = None
chosen_pending = False
for rid in candidates:
    pending, blocked, missing = required_on_run(by_run[rid])
    if blocked:
        for name, actual, detail in blocked:
            print(f"BLOCKED  {name}: conclusion={detail} (run {rid or 'unknown'}, check {actual})")
        code = 1
        chosen = rid
        break
    if pending or missing:
        if pending:
            name, actual, detail, attempt = pending[0]
            print(f"PENDING  {name}: status={detail} (attempt {attempt}, run {rid or 'unknown'})")
            chosen_pending = True
        elif missing and active:
            print(f"PENDING  {missing[0]}: no check run yet on run {rid or 'unknown'}, CI still active")
            chosen_pending = True
        else:
            for name in missing:
                print(f"BLOCKED  {name}: no check run on run {rid or 'unknown'} (tag a CI-green commit, or dispatch full CI: gh workflow run ci.yml --ref <tag> -f all=true)")
            code = 1
            chosen = rid
            break
        continue
    chosen = rid
    for name in wanted:
        print(f"PASS     {name}: success (run {rid or 'unknown'})")
    sys.exit(0)

if chosen_pending and code == 0:
    sys.exit(2)
if code == 1:
    sys.exit(1)
if active:
    print("PENDING  required checks: CI still active")
    sys.exit(2)
print("BLOCKED  required checks: no complete validation run on this commit")
sys.exit(1)
PY
}

if [[ -n "$CHECKS_JSON" ]]; then
	evaluate "$(cat "$CHECKS_JSON")"
	exit "$?"
fi

command -v gh >/dev/null 2>&1 || {
	printf 'release-checks-gate: gh is required\n' >&2
	exit 3
}
deadline=$((SECONDS + TIMEOUT_SECONDS))
while true; do
	# filter=all: the default latest filter orders by completed_at and can
	# omit a queued rerun (no completion time) that supersedes the visible
	# success. Paginate: required checks plus historical attempts exceed
	# one page of 100 on busy commits.
	pages='[]'
	page=1
	while true; do
		chunk="$(gh api "repos/$REPO/commits/$SHA/check-runs?per_page=100&page=${page}&filter=all")"
		pages="$(GATE_PAGES="$pages" GATE_CHUNK="$chunk" python3 - <<'PY'
import json, os
pages = json.loads(os.environ["GATE_PAGES"])
chunk = json.loads(os.environ["GATE_CHUNK"])
pages.append(chunk)
print(json.dumps(pages))
PY
)"
		more="$(GATE_CHUNK="$chunk" python3 - <<'PY'
import json, os
chunk = json.loads(os.environ["GATE_CHUNK"])
print("1" if len(chunk.get("check_runs") or []) >= 100 else "0")
PY
)"
		if [[ "$more" != "1" ]]; then
			break
		fi
		page=$((page + 1))
	done
	set +e
	evaluate "$pages"
	code="$?"
	set -e
	if [[ "$code" != "2" ]]; then
		exit "$code"
	fi
	if [[ "$SECONDS" -ge "$deadline" ]]; then
		printf 'release-checks-gate: timed out waiting for checks on %s\n' "$SHA" >&2
		printf 'dispatch full CI on the tag: gh workflow run ci.yml --ref <tag> -f all=true\n' >&2
		exit 1
	fi
	sleep "$POLL_SECONDS"
done
