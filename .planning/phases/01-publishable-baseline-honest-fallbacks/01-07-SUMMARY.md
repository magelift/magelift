---
phase: 01-publishable-baseline-honest-fallbacks
plan: 07
subsystem: testing
tags: [aws, catalog-cells, projection-matrix, container-graph, quality-04, offline]

requires:
  - phase: 01-publishable-baseline-honest-fallbacks
    provides: Lint/offline CI deferral (01-02); AWS stack/runtime packages under test
provides:
  - Stack-level projection matrix over all 28 legal catalog cells + preview×amazon-mq rejection
  - Runtime container-graph coverage for the six previously uncovered interactions (U1–U6)
  - Immutable runtime_test.go baseline for plan 01-10 file-split criterion 2
affects: [QUALITY-04, plan 01-10 runtime split]

tech-stack:
  added: []
  patterns:
    - "Enumerate legal cells per preset (not flat 3×4×2) so designed-invalid cells stay rejected under test"
    - "Cheap stack projection matrix vs sparse Pulumi-mock container-graph matrix"
    - "validArgs()+deep-copy Secrets before mutate; never edit SigV4 sentinel tests"

key-files:
  created: []
  modified:
    - internal/cloud/aws/stack/component_test.go
    - internal/cloud/aws/runtime/runtime_test.go

key-decisions:
  - "preview×amazon-mq rejected via Spec.Validate with three AZs — reason preset requires 2 availability zones"
  - "U3/U4 assert sidecar presence in the container set; DependsOn gap recorded as finding, not fixed in this baseline"
  - "go test -race deferred-local under Cursor after swap exhaustion; non-race package verifies passed"

patterns-established:
  - "TestCatalogCellProjectionMatrix — exhaustive cheap projection over cells that live only in stack"
  - "TestRuntimeContainerGraphInteractions — six interaction rows; SigV4 tests remain untouched sentinels"

requirements-completed: [QUALITY-04]

coverage:
  - id: D1
    description: All 28 legal search×queue×webRuntime cells plus preset defaults project to consumer count, effective queue mode, Varnish image, and search-proxy image
    requirement: QUALITY-04
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/aws/stack/ -count=1 -run TestCatalogCellProjectionMatrix"
        status: pass
    human_judgment: false
  - id: D2
    description: preview×amazon-mq is rejected with the named AZ-count reason (guard under test)
    requirement: QUALITY-04
    verification:
      - kind: unit
        ref: "TestCatalogCellProjectionMatrix/preview/amazon-mq/rejected"
        status: pass
    human_judgment: false
  - id: D3
    description: Six previously uncovered runtime container-graph interactions (FrankenPHP sidecar/Varnish, queue/deploy/cron sidecar set, db queue connection, broker endpoint shapes)
    requirement: QUALITY-04
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/aws/runtime/ -count=1 -run 'TestRuntimeContainerGraphInteractions|TestAMQPSettingsBrokerEndpointShapes'"
        status: pass
    human_judgment: false

duration: 25min
completed: 2026-07-28
status: complete
---

# Phase 1 Plan 07: Catalog-Cell Combination Coverage Summary

**QUALITY-04 closed at both levels: exhaustive cheap projection matrix in `stack`, and six container-graph interaction rows in `runtime` — with SigV4 sentinels untouched for plan 01-10.**

## Performance

- **Duration:** ~25 min productive (wall clock longer; aborted parallel `-race` rebuilds when swap hit ~14 GB)
- **Started:** 2026-07-28T11:58:07Z
- **Completed:** 2026-07-28T13:42:00Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Added `TestCatalogCellProjectionMatrix` covering 12 preview + 16 standard legal cells, preset defaults, and an explicit preview×amazon-mq rejection naming the 2-AZ guard.
- Added `TestRuntimeContainerGraphInteractions` (U1–U5) and `TestAMQPSettingsBrokerEndpointShapes` (U6) without editing any pre-existing runtime test.
- Documented Finding F-01-07-1 (search-proxy DependsOn skipped on nginx-fpm queue/deploy/cron) without changing production code — plan forbids mixing a fix into the 01-10 baseline.

## Task Commits

1. **Task 1: Projection matrix over every legal cell combination** - `90e9355` (test)
2. **Task 2: Container-graph matrix over the six uncovered interactions** - `7836a39` (test)

**Plan metadata:** `973210d` (docs: complete plan)

## Files Created/Modified

- `internal/cloud/aws/stack/component_test.go` — projection matrix + AZ-guard rejection helper
- `internal/cloud/aws/runtime/runtime_test.go` — container-graph matrix + broker endpoint shapes (append-only)

## Decisions Made

- Reject preview×amazon-mq by giving it the three AZs amazon-mq needs and asserting `Spec.Validate` fails with `preset "preview" requires 2 availability zones` (pure, no Pulumi).
- For U3/U4, assert sidecar **presence** in the task container set; do not lock the missing DependsOn into the test.
- Skip local `go test -race` for these packages under Cursor after swap exhaustion; non-race package runs passed. Full race remains deferred-local / CI when minutes return.

## Deviations from Plan

### Auto-fixed Issues

None - plan executed as written for test additions.

### Verification adjustments

**1. [Rule 3 - Blocking] Aborted `go test -race` under Cursor**
- **Found during:** Task 1 verify
- **Issue:** Multiple concurrent race compiles of pulumi-aws drove swap to ~14 GB / 15 GB
- **Fix:** Killed builds, reaped `darwin_arm64` compile orphans, verified with `GOMAXPROCS=1 GOFLAGS=-p=1 go test` (non-race) per package
- **Impact:** Race coverage deferred-local; behavior coverage still proven

**Total deviations:** 1 verification adjustment
**Impact on plan:** No scope change; QUALITY-04 assertions land offline

## Findings (not fixed — baseline purity)

### F-01-07-1 — search-proxy DependsOn skipped for nginx-fpm non-php-fpm task containers

- **Where:** `appendSearchProxy` in `internal/cloud/aws/runtime/runtime.go` — wires DependsOn only when `WebRuntime != "nginx-fpm"` **or** container name is `php-fpm`
- **Symptom:** Queue, deploy, and cron tasks **include** the search-proxy container when `SearchProxyImage` is set, but the Magento container does **not** `DependsOn` it under default `nginx-fpm`
- **Risk:** Magento may talk to `:8081` before the proxy is listening (race at task start); deploy/reindex/queue consumers are the high-impact paths (T-01-24)
- **Disposition:** Recorded only — production change would invalidate plan 01-10's "unchanged tests prove split" claim if mixed into this baseline

### Plan 01-10 baseline note

**Plan 01-10 must recapture its verbose test baseline from this plan's final tree** (`7836a39` tip of runtime_test.go additions on top of the pre-existing SigV4 sentinels). Do not edit those sentinel tests when splitting files.

## Issues Encountered

- Prior dispatch left an uncommitted draft of the projection matrix; completed, verified, and committed it.
- Memory pressure from overlapping `-race` jobs from stalled agents — aborted per AGENTS.md.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- QUALITY-04 closed offline; next plan **01-08**.
- Runtime test file is the complete combination-coverage baseline 01-10 needs for criterion 2.
- Finding F-01-07-1 is available for a later correctness fix **after** 01-10 freezes the split, or as an explicit exception if fixed before the split with a refreshed baseline.

---
*Phase: 01-publishable-baseline-honest-fallbacks*
*Completed: 2026-07-28*

## Self-Check: PASSED

- SUMMARY, both test files, commits 90e9355 and 7836a39 present.
