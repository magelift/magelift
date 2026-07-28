---
phase: 01-publishable-baseline-honest-fallbacks
plan: 03
subsystem: testing
tags: [cost, quality-01, platform, cli, offline]

requires:
  - phase: 01-publishable-baseline-honest-fallbacks
    provides: Lint coverage guard + offline CI deferral (01-02)
provides:
  - CLI cost command coverage via testCostEstimator seam
  - platform.ModuleCostEstimator unit coverage on frozen surface
  - Stable ErrNotSupported message prefix for plan 01-08
affects: [01-08, QUALITY-01, RELEASE-02]

tech-stack:
  added: []
  patterns:
    - "CLI cost tests inject platform.CostEstimator via options.testCostEstimator — no cloud imports"
    - "ErrNotSupported assertions use message prefix only so 01-08 can append certification tier"

key-files:
  created:
    - internal/cli/cost_test.go
    - internal/platform/cost_test.go
  modified: []

key-decisions:
  - "Unregistered-target fixture uses aws/eks-autopilot (valid schema, not in registerTestModules) instead of gcp/gke"
  - "platform go test -race deferred-local: ops_test.go pulls Pulumi AWS; race compile exhausted free RAM under Cursor"
  - "Preserved not-supported prefix for 01-08: `cost estimation is not supported for target %s/%s yet`"

patterns-established:
  - "Assert ExitCode(err) on cost failures — distinguishes loud fail from empty success"
  - "ModuleCostEstimator tested in package platform with minimal local StackModule stubs"

requirements-completed: [QUALITY-01]

coverage:
  - id: D1
    description: cost rejects positional args, honours --live, renders json/yaml/table
    requirement: QUALITY-01
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cli/ -run 'TestCost' -count=1"
        status: pass
    human_judgment: false
  - id: D2
    description: ErrNotSupported and missing estimator exit 2 naming target; other estimator errors exit 3 via usererr
    requirement: QUALITY-01
    verification:
      - kind: unit
        ref: "internal/cli/cost_test.go#TestCostErrNotSupportedExits2|TestCostOtherEstimatorErrorExits3|TestCostModuleWithoutEstimatorExits2"
        status: pass
    human_judgment: false
  - id: D3
    description: ModuleCostEstimator nil / no-interface / with-interface behaviours
    requirement: QUALITY-01
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/platform/ -run 'TestModuleCostEstimator' -count=1"
        status: pass
    human_judgment: false
  - id: D4
    description: platform package race verification for ModuleCostEstimator
    requirement: QUALITY-01
    verification: []
    human_judgment: true
    rationale: "deferred-local — go test -race ./internal/platform/ compiles ops_test AWS deps and exhausted free RAM (~47MB); verified without -race"

duration: ~6min
completed: 2026-07-28
status: complete
---

# Phase 1 Plan 03: Cost Command & Platform Helper Tests Summary

**First tests for `magelift cost` (flags, formats, exit 2/3) and `platform.ModuleCostEstimator`, with a stable not-supported message prefix for plan 01-08.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-07-28T11:30:54Z
- **Completed:** 2026-07-28T11:36:50Z
- **Tasks:** 2
- **Files modified:** 2 (test-only)

## Accomplishments

- Replaced obsolete `cost_test.go` (deleted after CostEstimator refactor) with command-level coverage through `testCostEstimator`
- Covered QUALITY-01: flag parsing, output formats, ErrNotSupported exit 2, other errors exit 3 with usererr, no-estimator stub exit 2, planning gate before estimator
- Locked `ModuleCostEstimator` behaviour before Phase 2 freezes the platform surface

## Task Commits

1. **Task 1: Cover the cost command's flags, output formats, and both error exits** - `41e5c9d` (test)
2. **Task 2: Cover the platform cost helper before Phase 2 freezes it** - `229d5af` (test)

**Plan metadata:** (pending docs commit)

## Files Created/Modified

- `internal/cli/cost_test.go` - Seven behaviours for cost CLI; no cloud package imports
- `internal/platform/cost_test.go` - Three subtests for ModuleCostEstimator; gofmt-clean

## Decisions Made

- Unregistered target: `aws`/`eks-autopilot` (schema-valid; not registered by test stub) — `gcp`/`gke` fails config validation before the planning gate
- Platform race: deferred-local under Cursor; serial non-race run passed
- **Message prefix plan 01-08 must preserve:** `cost estimation is not supported for target %s/%s yet` (then append certification tier)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Unregistered-target fixture used invalid runtime**
- **Found during:** Task 1
- **Issue:** `gcp`/`gke` rejected by config (`target.runtime must be gke-autopilot`) before `ModuleRegistry.Plan`
- **Fix:** Switch to `aws`/`eks-autopilot`
- **Files modified:** `internal/cli/cost_test.go`
- **Verification:** `TestCostUnregisteredTargetFailsBeforeEstimator` passes
- **Committed in:** `41e5c9d`

**2. [Rule 3 - Blocking] Aborted platform `-race` under memory pressure**
- **Found during:** Task 2 verify
- **Issue:** `go test -race ./internal/platform/` compiles `ops_test.go` → Pulumi AWS; free RAM ~47 MB
- **Fix:** Abort race compile; verify without `-race`; record deferred-local
- **Files modified:** none (verify path only)
- **Verification:** `TestModuleCostEstimator` PASS without race; gofmt clean
- **Committed in:** n/a (documented here + WINDOWS.md)

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking verify)
**Impact on plan:** Behaviours covered; race evidence deferred like 01-02 offline pattern. No production changes.

## Issues Encountered

- Platform package tests transitively compile cloud SDKs via existing `ops_test.go` — race under Cursor is unsafe on 16 GB

## Known Stubs

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- 01-03 complete offline; next plan **01-04** (wave 3 parallel candidate — recommend sequential offline under Cursor/memory constraints)
- Plan 01-08 can append certification tier after the preserved not-supported prefix

## Self-Check: PASSED

- FOUND: `internal/cli/cost_test.go`
- FOUND: `internal/platform/cost_test.go`
- FOUND: `01-03-SUMMARY.md`
- FOUND: commits `41e5c9d`, `229d5af`

---
*Phase: 01-publishable-baseline-honest-fallbacks*
*Completed: 2026-07-28*
