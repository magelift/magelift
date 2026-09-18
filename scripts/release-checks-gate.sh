#!/usr/bin/env bash
# Promotion gate: fail unless the required CI checks are green on a commit.
# The release workflow calls this before publishing a draft candidate.
# Missing checks fail: tag a commit CI actually ran on (main HEAD or a PR
# head), and dispatch full CI on the tag when filters would skip a
# required check (gh workflow run ci.yml --ref <tag> -f all=true).
#
# Usage: release-checks-gate.sh --sha <sha> [--repo owner/name]
#        [--checks-json FILE] [--exclude-run ID] [check...]
# Default checks: lint go-verify sdk-verify provider-verify floci-gcp php.
# A wanted name also matches GitHub Actions matrix titles ("php (8.3)").
# --checks-json evaluates one canned check-runs payload (tests):
# exit 0 pass, 1 blocked, 2 still pending. Without it, the gate polls
# the API until every required check completes or the timeout expires.
# --exclude-run ignores check runs from one workflow run (the release
# itself) when deciding whether CI is still active.
set -Eeuo pipefail

REPO="magelift/magelift"
SHA=""
CHECKS_JSON=""
EXCLUDE_RUN=""
CHECKS=(lint go-verify sdk-verify provider-verify floci-gcp php)
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
		sed -n '2,9p' "$0"
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
	export GATE_CHECKS="${CHECKS[*]}"
	export GATE_EXCLUDE_RUN="$EXCLUDE_RUN"
	python3 - <<'PY'
import json
import os
import re
import sys

try:
    data = json.loads(os.environ["GATE_PAYLOAD"])
except (KeyError, ValueError) as exc:
    print(f"release-checks-gate: invalid check-runs payload: {exc}", file=sys.stderr)
    sys.exit(3)

wanted = os.environ.get("GATE_CHECKS", "").split()
runs = data.get("check_runs", []) if isinstance(data, dict) else []


def matches(wanted_name, actual):
    # GitHub Actions matrix jobs are named "php (8.3)", not "php".
    # Require an exact name or the matrix suffix so "php" does not
    # accept "phpunit" or "frankenphp".
    return actual == wanted_name or actual.startswith(wanted_name + " (")


latest = {}
for run in runs:
    name = run.get("name", "")
    if not any(matches(item, name) for item in wanted):
        continue
    # Latest attempt wins: reruns get new check-run ids, so the
    # highest id is the newest attempt. Timestamps cannot serve:
    # a queued rerun has no start time yet and would sort before
    # the older success it supersedes. Key by the actual check
    # name so each matrix cell is judged on its own latest run.
    key = run.get("id") or 0
    if name not in latest or key >= latest[name][0]:
        latest[name] = (key, run)

exclude = os.environ.get("GATE_EXCLUDE_RUN", "").strip()
run_id_pattern = re.compile(r"/actions/runs/(\d+)/")


def own_run(run):
    if not exclude:
        return False
    match = run_id_pattern.search(run.get("html_url") or "")
    return match is not None and match.group(1) == exclude


active = any(
    run.get("status", "") != "completed" and not own_run(run) for run in runs
)
code = 0
for name in wanted:
    matched = [(actual, latest[actual]) for actual in latest if matches(name, actual)]
    if not matched:
        if active:
            print(f"PENDING  {name}: no check run yet, CI still active")
            code = 2
        else:
            print(f"BLOCKED  {name}: no check run on this commit (tag a CI-green commit, or dispatch full CI: gh workflow run ci.yml --ref <tag> -f all=true)")
            code = 1
        continue
    pending = False
    pending_run = None
    blocked = None
    for _actual, entry in matched:
        run = entry[1]
        status = run.get("status", "")
        conclusion = run.get("conclusion", "")
        if status != "completed":
            pending = True
            pending_run = run
        elif conclusion != "success":
            blocked = conclusion or "unknown"
    if blocked is not None:
        print(f"BLOCKED  {name}: conclusion={blocked}")
        code = 1
    elif pending:
        print(f"PENDING  {name}: status={pending_run.get('status') or 'unknown'} (attempt {pending_run.get('id', '?')})")
        code = 2
    else:
        print(f"PASS     {name}: success")

sys.exit(code)
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
	# success. Single page: required checks number well under 100.
	payload="$(gh api "repos/$REPO/commits/$SHA/check-runs?per_page=100&filter=all")"
	set +e
	evaluate "$payload"
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
