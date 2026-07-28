---
phase: 01-publishable-baseline-honest-fallbacks
plan: 08
subsystem: cli
tags: [trust-02, not-supported, certification-tier, experimental-stubs, offline]

requires:
  - phase: 01-publishable-baseline-honest-fallbacks
    provides: Cost not-supported prefix from 01-03; experimental OVH/Scaleway stub shells
provides:
  - Uniform unsupported message naming surface, provider/runtime, and certification tier
  - stubExperimentalModule (ovh.mks) for CLI tier-direction tests without cloud imports
  - Thirty enumerated stub-method tests plus AST sentinel guard (AcquireLock exception named)
affects: [TRUST-02, plan 01-09 deploy fallback, plan 01-10 AcquireLock]

tech-stack:
  added: []
  patterns:
    - "notSupportedMessage: `<surface> is not supported for target <provider>/<runtime> yet (<tier>)`"
    - "Tier from platform.PlannedStack / StackModule only — never cloud packages in internal/cli"
    - "go/parser structural guard over unsupported/Ops methods; AcquireLock carve-out logged"

key-files:
  created:
    - internal/cli/ports_test.go
    - internal/cloud/ovh/stack/ops_test.go
    - internal/cloud/scaleway/stack/ops_test.go
  modified:
    - internal/cli/ports.go
    - internal/cli/logs.go
    - internal/cli/health.go
    - internal/cli/cost.go
    - internal/cli/stub_module_test.go
    - internal/cloud/ovh/stack/ops.go
    - internal/cloud/scaleway/stack/ops.go

key-decisions:
  - "Message shape appends tier after preserved `yet` so 01-03 costNotSupportedPrefix stays a byte prefix"
  - "Experimental CLI stub is stubExperimentalModule (ovh/mks), not a cloud import"
  - "AcquireLock named exception in AST guard — plan 01-10 owns the silent-success lock"

patterns-established:
  - "notSupported / notSupportedForPlanned / notSupportedForModule — one shape for ports and day-2 surfaces"
  - "TestUnsupportedSourceGuardSentinelReturns — parse ops.go, fail closed, name method on miss"

requirements-completed: [TRUST-02]

coverage:
  - id: D1
    description: Unsupported day-2 surfaces name surface, provider/runtime, and certification tier (both directions)
    requirement: TRUST-02
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cli/ -count=1 -run 'TestNotSupported|TestPortHelpers|TestCostErrNotSupported|TestLogsErrNotSupported|TestHealthRuntimeErrNotSupported'"
        status: pass
    human_judgment: false
  - id: D2
    description: Fifteen unsupported methods per experimental stub return ErrNotSupported with zero values; accessors non-nil
    requirement: TRUST-02
    verification:
      - kind: unit
        ref: "TestUnsupportedMethodsReturnSentinelAndZeroValues + TestModuleAccessorsReturnNonNilUnsupportedShells in ovh/stack and scaleway/stack"
        status: pass
    human_judgment: false
  - id: D3
    description: AST guard fails closed on nil-returning stub methods; AcquireLock exception named
    requirement: TRUST-02
    verification:
      - kind: unit
        ref: "TestUnsupportedSourceGuardSentinelReturns (observed fail on temporary SilentSuccess, then restored)"
        status: pass
    human_judgment: false

duration: 8min
completed: 2026-07-28
status: complete
---

# Phase 1 Plan 08: Tier-Named Unsupported Surfaces Summary

**Every unsupported day-2 path now says what it is, which target, and which certification tier — and both experimental shells prove fifteen sentinel+zero returns under an AST guard.**

## Performance

- **Duration:** ~8 min
- **Started:** 2026-07-28T13:43:16Z
- **Completed:** 2026-07-28T13:51:34Z
- **Tasks:** 3
- **Files modified:** 8

## Accomplishments

- Unified message shape: `<surface> is not supported for target <provider>/<runtime> yet (<tier>)` — tier appended after the 01-03-preserved prefix.
- Routed logs, health, and cost through the shared helper; port helpers name target+tier instead of "this target".
- Added `stubExperimentalModule` (`ovh.mks`) so CLI tests assert experimental vs certified without importing cloud packages.
- Enumerated 15×2 stub methods with zero-value assertions; AST guard over `ops.go` with `Ops.AcquireLock` named for plan 01-10.

## Final message shape (verbatim)

```
<surface> is not supported for target <provider>/<runtime> yet (<tier>)
```

Examples:

- `cost estimation is not supported for target aws/ecs-fargate yet (certified)`
- `cost estimation is not supported for target ovh/mks yet (experimental)`
- `bootstrap is not supported for target ovh/mks yet (experimental)`

**Experimental stub module name:** `stubExperimentalModule` (descriptor `ovh.mks`) — plans 01-09 and 01-10 build on this.

## Task Commits

1. **Task 1 RED: failing tier-naming tests** - `bcc8ef5` (test)
2. **Task 1 GREEN: tier on every unsupported surface** - `4bc8315` (feat)
3. **Task 2: enumerate thirty unsupported methods** - `3f0fd55` (test)
4. **Task 3: AST sentinel guard (+ CostEstimator stubs)** - `e23cebe` (test)

**Plan metadata:** `0911bc6` (docs: complete plan)

## Files Created/Modified

- `internal/cli/ports.go` — `notSupportedMessage` / `notSupportedForPlanned` / `notSupportedForModule`; port helpers use module tier
- `internal/cli/logs.go`, `health.go`, `cost.go` — bespoke not-supported strings deleted; shared helper only
- `internal/cli/stub_module_test.go` — tier field on `stubPlanned`; `stubExperimentalModule`
- `internal/cli/ports_test.go` — certified + experimental directions, port helpers, logs/health/cost
- `internal/cloud/ovh/stack/ops_test.go`, `scaleway/stack/ops_test.go` — enumeration + AST guard
- `internal/cloud/ovh/stack/ops.go`, `scaleway/stack/ops.go` — `CostEstimator` / `Estimate` stubs (fifteen-method surface)

## Decisions Made

- Append `(<tier>)` after `yet` so `costNotSupportedPrefix` from 01-03 remains a literal prefix (HasPrefix assertions hold).
- Keep experimental CLI coverage in-package via `stubExperimentalModule`; never import OVH/Scaleway into `internal/cli` for tier strings.
- Document `AcquireLock` as the sole AST-guard exception with reason pointing at plan 01-10.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] CostEstimator stubs were uncommitted on HEAD**
- **Found during:** Task 3 (post Task 2 enumeration)
- **Issue:** Working tree already had `CostEstimator`/`Estimate` on OVH and Scaleway shells (needed for the exact fifteen-method count), but HEAD did not — tests would fail against committed tree.
- **Fix:** Included both `ops.go` files in the Task 3 commit.
- **Files modified:** `internal/cloud/ovh/stack/ops.go`, `internal/cloud/scaleway/stack/ops.go`
- **Commit:** `e23cebe`

**Total deviations:** 1
**Impact on plan:** Criterion 5 counts hold on HEAD; no scope change.

## Threat Flags

None — no new network endpoints, auth paths, or account identifiers in messages (T-01-31).

## Issues Encountered

None blocking. Guard failure observed once against temporary `unsupported.SilentSuccess` returning nil, then reverted.

## User Setup Required

None.

## Next Phase Readiness

- TRUST-02 enumeration clause and criterion 5 closed offline.
- Next plan **01-09** (deploy fallback). Silent-success lock remains **01-10**.
- Reuse message shape and `stubExperimentalModule` name from this SUMMARY.

---
*Phase: 01-publishable-baseline-honest-fallbacks*
*Completed: 2026-07-28*

## Self-Check: PASSED

- SUMMARY, ports_test.go, both ops_test.go, commits bcc8ef5/4bc8315/3f0fd55/e23cebe present.
