---
phase: 01-publishable-baseline-honest-fallbacks
plan: 02
subsystem: testing
tags: [ci, golangci-lint, lint-coverage, quality-06, offline]

requires:
  - phase: 01-publishable-baseline-honest-fallbacks
    provides: Partitioned lint matrix (aws/gcp/ovh/scaleway/core/aggregate)
provides:
  - Lint matrix union-coverage guard (internal/lintcoverage)
  - Offline lint-policy partitioning + race deferred measurements
  - CONVENTIONS.md aligned with partitioned lint
affects: [01-03, phase-1-criterion-1]

tech-stack:
  added: []
  patterns:
    - "Guard test parses ci.yml + expands patterns via go list"
    - "Offline mode: deferred-ci / deferred-local placeholders instead of assumed numbers"

key-files:
  created:
    - internal/lintcoverage/coverage_test.go
    - docs/lint-policy.md
  modified:
    - .github/workflows/ci.yml
    - .planning/codebase/CONVENTIONS.md

key-decisions:
  - "Offline mode: skip force-all workflow_dispatch and CI go-verify log reads (Actions minutes exhausted)"
  - "Full local go test -race ./... deferred-local under Cursor (16 GB Mac); narrow sample recorded instead"
  - "core matrix gained ./internal/lintcoverage/... so the guard package is itself linted"

patterns-established:
  - "internal/lintcoverage is the Pitfall-4 gate — matrix and assertion cannot drift"

requirements-completed: [QUALITY-06]

coverage:
  - id: D1
    description: Lint matrix union-coverage guard fails on gaps and overlaps
    requirement: QUALITY-06
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/lintcoverage/ -count=1"
        status: pass
    human_judgment: false
  - id: D2
    description: go test -race ./... measured on CI 2 vCPU / 7 GB runner
    requirement: QUALITY-06
    verification: []
    human_judgment: true
    rationale: "deferred-ci — no go-verify log; Actions minutes exhausted 2026-07-28"
  - id: D3
    description: Force-all CI run with all eight make verify targets green
    requirement: QUALITY-06
    verification: []
    human_judgment: true
    rationale: "deferred-ci — workflow_dispatch force-all not run; billing blocked"
  - id: D4
    description: CONVENTIONS.md no longer describes single-threaded 30m lint
    requirement: QUALITY-06
    verification:
      - kind: other
        ref: ".planning/codebase/CONVENTIONS.md linting section"
        status: pass
    human_judgment: false

duration: ~20min
completed: 2026-07-28
status: complete
---

# Phase 1 Plan 02: Lint Coverage Guard Summary

**Union-coverage guard landed and proven red/green locally; CI race + force-all verify evidence deferred until Actions minutes return.**

## Performance

- **Duration:** ~20 min
- **Started:** 2026-07-28T11:26:17Z
- **Completed:** 2026-07-28T11:28:25Z
- **Tasks:** 1 complete; 2 and 3 local docs done, CI portions deferred-ci
- **Files modified:** 4

## Accomplishments

- Added `internal/lintcoverage` guard that parses `.github/workflows/ci.yml`, expands matrix patterns with `go list`, subtracts `.golangci.yml` path exclusions, and fails on gaps or overlaps.
- Observed RED (new package uncovered), GREEN (after adding `./internal/lintcoverage/...` to `core`), and a deliberate fail when `./internal/health/...` was temporarily removed.
- Updated `docs/lint-policy.md` with partitioning narrative and race-measurement table (narrow local sample + deferred placeholders).
- Replaced obsolete CONVENTIONS lint guidance (concurrency:1 / 30m timeout) with partitioned CI reality + serial local-build warning.

## Task Results

| Task | Result | Commit(s) |
|------|--------|-----------|
| 1 Guard lint matrix coverage | **Done** (TDD RED→GREEN; fail-on-removed-pattern observed) | `3096d7e`, `b0b5e44` |
| 2 Settle `go test -race ./...` fit | **Partial** — narrow local sample recorded; full local + CI **deferred** | `51bc339` |
| 3 Force-all eight verify targets | **Partial** — CONVENTIONS + partitioning docs done; CI run **deferred-ci** | `74c45d3` (+ lint-policy in `51bc339`) |

## deferred-ci

| Item | Why deferred | Resume when |
|------|--------------|-------------|
| CI `go test -race ./...` wall time + peak RSS from `go-verify` log | Actions minutes / billing exhausted (see `.planning/loop/HUMAN_GATE`) | Spending limit fixed; green `go-verify` exists |
| Force-all `workflow_dispatch` run URL | Same — must not `gh workflow run` in offline mode | Minutes return; then `gh workflow run ci.yml --ref main -f all=true` |
| Eight-target conclusion table (`generate-check`, `cli-docs-check`, `fmt-check`, `test`, `license-check`, `php-test`, `docs`, `workflow-check`) | Requires the force-all run above | After that run concludes |
| Whether CI needs race-test partitioning by the six lint groups | Depends on CI race outcome | After first completed `go-verify` |

### Force-all run (placeholder)

- **URL:** deferred-ci
- **Per-target table:** deferred-ci

## deferred-local

| Item | Why | Note |
|------|-----|------|
| `GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./... -count=1` full module | 16 GB Mac under Cursor — AGENTS.md kernel-panic history; full race links provider SDKs | Run later in a plain Terminal with nothing else heavy, or trust CI measurement |

### Local race sample (recorded)

| Command | Wall | Peak RSS | Outcome |
|---------|------|----------|---------|
| `go test -race ./internal/lintcoverage/ ./internal/usererr/ -count=1` (serial) | ~4.7s | ~102 MiB | pass |

## Task Commits

1. **Task 1 RED:** `3096d7e` — `test(01-02): add failing lint matrix coverage guard`
2. **Task 1 GREEN:** `b0b5e44` — `feat(01-02): cover lintcoverage in core lint partition`
3. **Task 2:** `51bc339` — `docs(01-02): record race measurement and CI lint partitioning`
4. **Task 3:** `74c45d3` — `docs(01-02): replace obsolete single-threaded lint guidance`

## Files Created/Modified

- `internal/lintcoverage/coverage_test.go` — union-coverage + structural failure tests
- `.github/workflows/ci.yml` — `./internal/lintcoverage/...` in `core` packages
- `docs/lint-policy.md` — partitioning, force-all recipe, race table
- `.planning/codebase/CONVENTIONS.md` — partitioned lint; serial local builds still required

## Decisions Made

- Offline mode honors HUMAN_GATE: no `gh workflow run` / run watch.
- Full-module local race deferred rather than risking kernel panic under Cursor.
- Guard uses existing `go.yaml.in/yaml/v4` — zero new dependencies.

## Deviations from Plan

### Auto-fixed Issues

None beyond offline-mode intentional skips.

### Offline-mode skips (not bugs)

1. **Task 2 precondition unmet** — no CI `go-verify` log with `go test -race ./...`; recorded deferred-ci placeholders in `docs/lint-policy.md`.
2. **Task 2 verify `go test -race ./...`** — deferred-local; narrow sample only.
3. **Task 3 force-all CI verify** — deferred-ci; docs/CONVENTIONS local deliverables still shipped.

**Total deviations:** 0 auto-fixed; 3 intentional offline deferrals  
**Impact on plan:** Criterion 1 CI evidence still open (carried from 01-01). Guard + docs progress unblocked.

## Threat Flags

None new beyond plan register. Guard mitigates T-01-05 locally; T-01-06 / T-01-08 remain open until deferred-ci items close.

## Known Stubs

None in code. Documentation placeholders (`deferred-ci`, `deferred-local`) are intentional offline markers, not runtime stubs.

## Issues Encountered

- Concurrent unrelated commit `89384c6` (nektos/act) appeared on `main` during execution — left untouched.
- Dirty tree had many unrelated files — only plan 01-02 paths were staged.

## User Setup Required

None new. Resume CI proof still requires fixing GitHub Actions billing (see `.planning/loop/HUMAN_GATE`).

## Next Phase Readiness

- **Next offline plan:** `01-03-PLAN.md` (continue Phase 1 without CI minutes).
- **When minutes return:** amend this SUMMARY with force-all URL + eight-target table + CI race figures; close HUMAN_GATE.

## Self-Check: PASSED

- FOUND: `internal/lintcoverage/coverage_test.go`
- FOUND: `docs/lint-policy.md` (race + partitioning)
- FOUND: commits `3096d7e`, `b0b5e44`, `51bc339`, `74c45d3`
- FOUND: CONVENTIONS no longer mentions concurrency:1 / 30-minute lint timeout
---
*Phase: 01-publishable-baseline-honest-fallbacks*
*Completed: 2026-07-28*
