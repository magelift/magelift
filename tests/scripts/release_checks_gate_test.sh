#!/usr/bin/env bash
# The promotion gate passes only a complete validation run, blocks
# failed/skipped/missing checks and stitched partial runs, and reports
# pending reruns as pending (never pass).
# Exit codes: 0 pass, 1 blocked, 2 pending, 3 usage.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GATE="$ROOT/scripts/release-checks-gate.sh"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/magelift-gate-test.XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

run_url() {
	local run_id="$1" job="$2"
	printf 'https://github.com/magelift/magelift/actions/runs/%s/job/%s' "$run_id" "$job"
}

write_payload() {
	python3 - "$WORK/checks.json" <<'PY'
import json
import sys

run = "100"
runs = [
    {"name": "CI passed", "status": "completed", "conclusion": "success", "id": 10, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/10"},
    {"name": "lint", "status": "completed", "conclusion": "success", "id": 1, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/1"},
    {"name": "go-verify", "status": "completed", "conclusion": "success", "id": 2, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/2"},
    {"name": "sdk-verify", "status": "completed", "conclusion": "success", "id": 3, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/3"},
    {"name": "provider-aws", "status": "completed", "conclusion": "success", "id": 14, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/14"},
    {"name": "provider-gcp", "status": "completed", "conclusion": "success", "id": 4, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/4"},
    {"name": "floci-gcp", "status": "completed", "conclusion": "success", "id": 5, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/5"},
    {"name": "provider-azure", "status": "completed", "conclusion": "success", "id": 15, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/15"},
    {"name": "floci-aws", "status": "completed", "conclusion": "success", "id": 25, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/25"},
    {"name": "floci-azure", "status": "completed", "conclusion": "success", "id": 26, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/26"},
    {"name": "php (8.3)", "status": "completed", "conclusion": "success", "id": 61, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/61"},
    {"name": "php (8.5)", "status": "completed", "conclusion": "success", "id": 62, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/62"},
    {"name": "unrelated", "status": "completed", "conclusion": "failure", "id": 7, "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/7"},
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
grep -q "PASS     provider-gcp: success" "$WORK/pass.out" || {
	printf 'gate hid the provider verdict\n' >&2
	exit 1
}
grep -q "PASS     CI passed: success" "$WORK/pass.out" || {
	printf 'gate hid the aggregate verdict\n' >&2
	exit 1
}

# 2. A failing provider-only test blocks promotion, even when the root
# suite is green.
mutate provider-gcp '[{"name": "provider-gcp", "status": "completed", "conclusion": "failure", "id": 4, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/4"}]'
if "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/fail.out" 2>&1; then
	printf 'gate promoted despite provider-gcp failure\n' >&2
	exit 1
fi
grep -q "BLOCKED  provider-gcp: conclusion=failure" "$WORK/fail.out" || {
	printf 'gate did not name the failing check\n' >&2
	exit 1
}

# 3. A skipped check blocks: releases need actual verification.
mutate provider-gcp '[{"name": "provider-gcp", "status": "completed", "conclusion": "skipped", "id": 4, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/4"}]'
if "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/skip.out" 2>&1; then
	printf 'gate promoted on a skipped check\n' >&2
	exit 1
fi
grep -q "BLOCKED  provider-gcp: conclusion=skipped" "$WORK/skip.out" || {
	printf 'gate did not flag the skipped check\n' >&2
	exit 1
}

# 4. Missing checks with CI complete block with the dispatch hint.
python3 -c "import json; json.dump({'check_runs': []}, open('$WORK/empty.json', 'w'))"
if "$GATE" --sha abc123 --checks-json "$WORK/empty.json" >"$WORK/missing.out" 2>&1; then
	printf 'gate promoted a commit with no checks\n' >&2
	exit 1
fi
grep -q "BLOCKED  CI passed: no check run" "$WORK/missing.out" || {
	printf 'gate did not flag missing checks\n' >&2
	exit 1
}
grep -q "gh workflow run ci.yml" "$WORK/missing.out" || {
	printf 'gate hid the full-CI dispatch hint\n' >&2
	exit 1
}

# 5. Missing checks with CI still active report pending, not blocked.
python3 -c "import json; json.dump({'check_runs': [{'name': 'lint', 'status': 'in_progress', 'conclusion': None, 'id': 1, 'html_url': 'https://github.com/magelift/magelift/actions/runs/100/job/1'}]}, open('$WORK/active.json', 'w'))"
code=0
"$GATE" --sha abc123 --checks-json "$WORK/active.json" >"$WORK/active.out" 2>&1 || code="$?"
if [[ "$code" != "2" ]]; then
	printf 'gate exit = %s, want 2 (pending)\n' "$code" >&2
	exit 1
fi
grep -q "PENDING" "$WORK/active.out" || {
	printf 'gate did not report the pending check\n' >&2
	exit 1
}

# 6. An older success with a newer rerun in progress reports pending:
# the latest attempt wins.
write_payload
mutate provider-gcp '[{"name": "provider-gcp", "status": "completed", "conclusion": "success", "id": 4, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/4"}, {"name": "provider-gcp", "status": "in_progress", "conclusion": null, "id": 14, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/14"}]'
code=0
"$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/rerun.out" 2>&1 || code="$?"
if [[ "$code" != "2" ]]; then
	printf 'gate exit = %s, want 2 (pending rerun)\n' "$code" >&2
	exit 1
fi
grep -q "PENDING  provider-gcp" "$WORK/rerun.out" || {
	printf 'gate did not flag the in-progress rerun\n' >&2
	exit 1
}

# 7. A failure repaired by a later success passes.
mutate provider-gcp '[{"name": "provider-gcp", "status": "completed", "conclusion": "failure", "id": 4, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/4"}, {"name": "provider-gcp", "status": "completed", "conclusion": "success", "id": 14, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/14"}]'
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
mutate provider-gcp '[{"name": "provider-gcp", "status": "completed", "conclusion": "success", "id": 10, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/10"}, {"name": "provider-gcp", "status": "queued", "conclusion": null, "id": 11, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/11"}]'
code=0
"$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/queued.out" 2>&1 || code="$?"
if [[ "$code" != "2" ]]; then
	printf 'gate exit = %s, want 2 (queued rerun)\n' "$code" >&2
	exit 1
fi
grep -q "PENDING  provider-gcp" "$WORK/queued.out" || {
	printf 'gate did not flag the queued rerun\n' >&2
	exit 1
}

# 10. A waiting rerun likewise reports pending, and its later success passes.
mutate provider-gcp '[{"name": "provider-gcp", "status": "completed", "conclusion": "success", "id": 10, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/10"}, {"name": "provider-gcp", "status": "waiting", "conclusion": null, "id": 11, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/11"}]'
code=0
"$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/waiting.out" 2>&1 || code="$?"
if [[ "$code" != "2" ]]; then
	printf 'gate exit = %s, want 2 (waiting rerun)\n' "$code" >&2
	exit 1
fi
mutate provider-gcp '[{"name": "provider-gcp", "status": "completed", "conclusion": "success", "id": 10, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/10"}, {"name": "provider-gcp", "status": "completed", "conclusion": "success", "id": 11, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/11"}]'
if ! "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/queuedone.out" 2>&1; then
	cat "$WORK/queuedone.out" >&2
	printf 'gate rejected a completed rerun\n' >&2
	exit 1
fi

# 11. The live fetch reads every page, even when the payload exceeds the
# process argument limit.
write_payload
python3 - "$WORK/checks.json" "$WORK/large.json" <<'PY'
import json
import sys

with open(sys.argv[1]) as source:
    selected = json.load(source)
older = {
    "check_runs": [
        {
            "name": "lint",
            "id": 1000 + index,
            "status": "completed",
            "conclusion": "failure",
            "html_url": f"https://github.com/magelift/magelift/actions/runs/99/job/{1000 + index}",
            "output": {"summary": "x" * 2048},
        }
        for index in range(100)
    ]
}
with open(sys.argv[2], "w") as destination:
    json.dump([older, selected], destination)
PY
mkdir "$WORK/bin"
cat >"$WORK/bin/gh" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail
[[ "$#" == 4 && "$1" == api && "$2" == --paginate && "$3" == --slurp ]]
[[ "$4" == 'repos/magelift/magelift/commits/abc123/check-runs?per_page=100&filter=all' ]]
cat "$GATE_TEST_PAYLOAD"
SH
chmod +x "$WORK/bin/gh"
if ! env -u BASH_ENV PATH="$WORK/bin:$PATH" GATE_TEST_PAYLOAD="$WORK/large.json" \
	"$BASH" "$GATE" --sha abc123 --run-id 100 >"$WORK/large.out" 2>&1; then
	cat "$WORK/large.out" >&2
	printf 'gate rejected a green run in a large response\n' >&2
	exit 1
fi
grep -q "PASS     provider-gcp: success (run 100" "$WORK/large.out"

# 12. The release run itself does not count as active CI.
write_payload
python3 - "$WORK/self.json" <<'PY'
import json
import sys

with open(sys.argv[1].replace("self.json", "checks.json")) as handle:
    data = json.load(handle)
data["check_runs"].append({
    "name": "release",
    "status": "in_progress",
    "conclusion": None,
    "id": 7,
    "html_url": "https://github.com/magelift/magelift/actions/runs/200/job/7",
})
with open(sys.argv[1], "w") as handle:
    json.dump(data, handle)
PY
if ! "$GATE" --sha abc123 --checks-json "$WORK/self.json" --exclude-run 200 >"$WORK/self.out" 2>&1; then
	cat "$WORK/self.out" >&2
	printf 'gate waited on its own release run\n' >&2
	exit 1
fi

# 13. Missing checks with only the excluded run active fail fast
# instead of polling the timeout (the stuck-publish shape).
python3 -c "import json; json.dump({'check_runs': [{'name': 'release', 'status': 'in_progress', 'conclusion': None, 'id': 7, 'html_url': 'https://github.com/magelift/magelift/actions/runs/200/job/7'}]}, open('$WORK/selfonly.json', 'w'))"
code=0
"$GATE" --sha abc123 --checks-json "$WORK/selfonly.json" --exclude-run 200 >"$WORK/selfonly.out" 2>&1 || code="$?"
if [[ "$code" != "1" ]]; then
	printf 'gate exit = %s, want 1 (missing with only self active)\n' "$code" >&2
	exit 1
fi
grep -q "BLOCKED  CI passed: no check run" "$WORK/selfonly.out" || {
	printf 'gate did not fail fast on missing checks\n' >&2
	exit 1
}

# 14. GitHub Actions matrix names satisfy the family check. A
# sibling name such as phpunit must not.
write_payload
if ! "$GATE" --sha abc123 --checks-json "$WORK/checks.json" php >"$WORK/matrix.out" 2>&1; then
	cat "$WORK/matrix.out" >&2
	printf 'gate rejected a green php matrix\n' >&2
	exit 1
fi
grep -q "PASS     php: success" "$WORK/matrix.out" || {
	printf 'gate hid the php matrix verdict\n' >&2
	exit 1
}

# 15. One failed matrix cell blocks the family.
mutate "php (8.5)" '[{"name": "php (8.5)", "status": "completed", "conclusion": "failure", "id": 62, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/62"}]'
if "$GATE" --sha abc123 --checks-json "$WORK/checks.json" php >"$WORK/matrixfail.out" 2>&1; then
	printf 'gate promoted despite a failed php matrix cell\n' >&2
	exit 1
fi
grep -q "BLOCKED  php: conclusion=failure" "$WORK/matrixfail.out" || {
	printf 'gate did not name the failed php matrix cell\n' >&2
	exit 1
}

# 16. A phpunit-only payload does not satisfy php.
python3 - "$WORK/phpunit.json" <<'PY'
import json
import sys

runs = [
    {"name": "CI passed", "status": "completed", "conclusion": "success", "id": 10, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/10"},
    {"name": "provider-gcp", "status": "completed", "conclusion": "success", "id": 4, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/4"},
    {"name": "phpunit", "status": "completed", "conclusion": "success", "id": 6, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/6"},
]
with open(sys.argv[1], "w") as handle:
    json.dump({"check_runs": runs}, handle)
PY
if "$GATE" --sha abc123 --checks-json "$WORK/phpunit.json" php >"$WORK/phpunit.out" 2>&1; then
	printf 'gate treated phpunit as php\n' >&2
	exit 1
fi
grep -q "BLOCKED  php: no check run" "$WORK/phpunit.out" || {
	printf 'gate hid the phpunit mismatch\n' >&2
	exit 1
}

# 17. Failed aggregate CI blocks even when the six older families are green.
write_payload
mutate "CI passed" '[{"name": "CI passed", "status": "completed", "conclusion": "failure", "id": 10, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/10"}]'
if "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/aggfail.out" 2>&1; then
	printf 'gate promoted despite failed CI passed\n' >&2
	exit 1
fi
grep -q "BLOCKED  CI passed: conclusion=failure" "$WORK/aggfail.out" || {
	printf 'gate hid the failed aggregate\n' >&2
	exit 1
}

# 18. A single PHP cell is not a complete dispatch matrix.
write_payload
mutate "php (8.5)" '[]'
if "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/phpone.out" 2>&1; then
	printf 'gate promoted with only php 8.3\n' >&2
	exit 1
fi
grep -q "BLOCKED  php (8.5): no check run" "$WORK/phpone.out" || {
	printf 'gate hid the missing php 8.5 cell\n' >&2
	exit 1
}

# 19. Successes from two different runs cannot be stitched.
python3 - "$WORK/stitch.json" <<'PY'
import json
import sys

runs = [
    {"name": "CI passed", "status": "completed", "conclusion": "success", "id": 10, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/10"},
    {"name": "provider-gcp", "status": "completed", "conclusion": "success", "id": 4, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/4"},
    {"name": "php (8.3)", "status": "completed", "conclusion": "success", "id": 61, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/61"},
    {"name": "php (8.5)", "status": "completed", "conclusion": "success", "id": 62, "html_url": "https://github.com/magelift/magelift/actions/runs/200/job/62"},
]
with open(sys.argv[1], "w") as handle:
    json.dump({"check_runs": runs}, handle)
PY
if "$GATE" --sha abc123 --checks-json "$WORK/stitch.json" >"$WORK/stitch.out" 2>&1; then
	printf 'gate stitched php 8.5 from another run\n' >&2
	exit 1
fi
# Newest run is 200 (php 8.5 only). Do not walk back to run 100.
grep -q "BLOCKED  CI passed: no check run on run 200" "$WORK/stitch.out" || {
	printf 'gate hid the stitched-run gap\n' >&2
	cat "$WORK/stitch.out" >&2
	exit 1
}

# 20. Required checks on a later page still count after merge.
python3 - "$WORK/pages.json" <<'PY'
import json
import sys

page1 = {"check_runs": [
    {"name": "lint", "status": "completed", "conclusion": "success", "id": i, "html_url": f"https://github.com/magelift/magelift/actions/runs/100/job/{i}"}
    for i in range(1, 101)
]}
page2 = {"check_runs": [
    {"name": "CI passed", "status": "completed", "conclusion": "success", "id": 201, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/201"},
    {"name": "go-verify", "status": "completed", "conclusion": "success", "id": 202, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/202"},
    {"name": "sdk-verify", "status": "completed", "conclusion": "success", "id": 203, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/203"},
    {"name": "provider-aws", "status": "completed", "conclusion": "success", "id": 214, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/214"},
    {"name": "provider-gcp", "status": "completed", "conclusion": "success", "id": 204, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/204"},
    {"name": "floci-gcp", "status": "completed", "conclusion": "success", "id": 205, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/205"},
    {"name": "provider-azure", "status": "completed", "conclusion": "success", "id": 215, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/215"},
    {"name": "floci-aws", "status": "completed", "conclusion": "success", "id": 225, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/225"},
    {"name": "floci-azure", "status": "completed", "conclusion": "success", "id": 226, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/226"},
    {"name": "php (8.3)", "status": "completed", "conclusion": "success", "id": 206, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/206"},
    {"name": "php (8.5)", "status": "completed", "conclusion": "success", "id": 207, "html_url": "https://github.com/magelift/magelift/actions/runs/100/job/207"},
]}
with open(sys.argv[1], "w") as handle:
    json.dump([page1, page2], handle)
PY
if ! "$GATE" --sha abc123 --checks-json "$WORK/pages.json" >"$WORK/pages.out" 2>&1; then
	cat "$WORK/pages.out" >&2
	printf 'gate dropped required checks on page 2\n' >&2
	exit 1
fi

# 22. A pending newer run must not fall back to an older success.
python3 - "$WORK/newer-pending.json" <<'PY'
import json
import sys

def job(run, name, status, conclusion, ident):
    return {
        "name": name,
        "status": status,
        "conclusion": conclusion,
        "id": ident,
        "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/{ident}",
    }

names = ["CI passed", "lint", "go-verify", "sdk-verify", "provider-aws", "provider-gcp", "provider-azure", "floci-aws", "floci-gcp", "floci-azure", "php (8.3)", "php (8.5)"]
runs = [job("100", name, "completed", "success", 10 + i) for i, name in enumerate(names)]
runs.extend([
    job("200", "CI passed", "queued", None, 20),
    job("200", "provider-gcp", "queued", None, 21),
])
with open(sys.argv[1], "w") as handle:
    json.dump({"check_runs": runs}, handle)
PY
code=0
"$GATE" --sha abc123 --checks-json "$WORK/newer-pending.json" >"$WORK/newer-pending.out" 2>&1 || code="$?"
if [[ "$code" != "2" ]]; then
	printf 'gate exit = %s, want 2 (pending newer run)\n' "$code" >&2
	cat "$WORK/newer-pending.out" >&2
	exit 1
fi
grep -q "PENDING  CI passed: status=queued" "$WORK/newer-pending.out" || {
	printf 'gate hid the pending newer aggregate\n' >&2
	cat "$WORK/newer-pending.out" >&2
	exit 1
}
if grep -q "PASS" "$WORK/newer-pending.out"; then
	printf 'gate fell back to the older green run\n' >&2
	cat "$WORK/newer-pending.out" >&2
	exit 1
fi

# 23. A newer run that does not yet have the aggregate stays pending.
python3 - "$WORK/newer-noagg.json" <<'PY'
import json
import sys

def job(run, name, status, conclusion, ident):
    return {
        "name": name,
        "status": status,
        "conclusion": conclusion,
        "id": ident,
        "html_url": f"https://github.com/magelift/magelift/actions/runs/{run}/job/{ident}",
    }

names = ["CI passed", "lint", "go-verify", "sdk-verify", "provider-aws", "provider-gcp", "provider-azure", "floci-aws", "floci-gcp", "floci-azure", "php (8.3)", "php (8.5)"]
runs = [job("100", name, "completed", "success", 10 + i) for i, name in enumerate(names)]
runs.append(job("200", "lint", "in_progress", None, 30))
with open(sys.argv[1], "w") as handle:
    json.dump({"check_runs": runs}, handle)
PY
code=0
"$GATE" --sha abc123 --checks-json "$WORK/newer-noagg.json" >"$WORK/newer-noagg.out" 2>&1 || code="$?"
if [[ "$code" != "2" ]]; then
	printf 'gate exit = %s, want 2 (newer run before aggregate)\n' "$code" >&2
	cat "$WORK/newer-noagg.out" >&2
	exit 1
fi
if grep -q "PASS" "$WORK/newer-noagg.out"; then
	printf 'gate promoted on an older run while a newer run lacked the aggregate\n' >&2
	cat "$WORK/newer-noagg.out" >&2
	exit 1
fi

# 24. Aggregate success with skipped release-mandatory jobs blocks.
write_payload
for name in lint go-verify sdk-verify provider-aws provider-gcp provider-azure floci-aws floci-gcp floci-azure; do
	mutate "$name" "[{\"name\": \"$name\", \"status\": \"completed\", \"conclusion\": \"skipped\", \"id\": 80, \"html_url\": \"https://github.com/magelift/magelift/actions/runs/100/job/80\"}]"
done
if "$GATE" --sha abc123 --checks-json "$WORK/checks.json" >"$WORK/skipped-mand.out" 2>&1; then
	printf 'gate promoted with skipped release-mandatory jobs\n' >&2
	cat "$WORK/skipped-mand.out" >&2
	exit 1
fi
grep -q "BLOCKED  lint: conclusion=skipped" "$WORK/skipped-mand.out" || {
	printf 'gate hid a skipped mandatory job\n' >&2
	cat "$WORK/skipped-mand.out" >&2
	exit 1
}

# 25. --run-id evaluates only that run.
write_payload
python3 - "$WORK/pin.json" <<'PY'
import json
import sys

with open(sys.argv[1].replace("pin.json", "checks.json")) as handle:
    data = json.load(handle)
data["check_runs"].append({
    "name": "CI passed",
    "status": "queued",
    "conclusion": None,
    "id": 90,
    "html_url": "https://github.com/magelift/magelift/actions/runs/200/job/90",
})
with open(sys.argv[1], "w") as handle:
    json.dump(data, handle)
PY
code=0
"$GATE" --sha abc123 --checks-json "$WORK/pin.json" --run-id 200 >"$WORK/pin.out" 2>&1 || code="$?"
if [[ "$code" != "2" ]]; then
	printf 'gate exit = %s, want 2 (pinned pending run)\n' "$code" >&2
	cat "$WORK/pin.out" >&2
	exit 1
fi
if grep -q "PASS" "$WORK/pin.out"; then
	printf 'gate ignored --run-id and passed the older run\n' >&2
	cat "$WORK/pin.out" >&2
	exit 1
fi

printf 'release checks gate ok\n'
