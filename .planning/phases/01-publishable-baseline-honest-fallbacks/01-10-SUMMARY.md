---
phase: 01-publishable-baseline-honest-fallbacks
plan: 10
subsystem: aws-runtime, ops-honesty
tags: [quality-07, trust-02, acquire-lock, runtime-split, offline, gap-closure]

requires:
  - phase: 01-publishable-baseline-honest-fallbacks
    provides: 01-07 combination/SigV4 runtime tests; 01-08 AST sentinel guards with AcquireLock carve-out
provides:
  - AWS runtime package split under 400-line non-test cap (QUALITY-07)
  - Warn-then-noop AcquireLock on ovh/scaleway/eksops (TRUST-02 AcquireLock clause)
  - AST guards narrowed to unsupported only; carve-outs removed
affects: [QUALITY-07, TRUST-02, Phase 6 real DIY locks, ROADMAP criteria 2 and 5]

tech-stack:
  added: []
  patterns:
    - "Intra-package concern split — New lives in component.go; tests keep unexported access"
    - "platform.WarnNoDIYLock + package diyLockWarnOut sink (default os.Stderr) for testable honesty warnings"
    - "ErrNotSupported AST walk covers unsupported only; AcquireLock proven by stderr warning tests"

key-files:
  created:
    - internal/cloud/aws/runtime/types.go
    - internal/cloud/aws/runtime/identity.go
    - internal/cloud/aws/runtime/component.go
    - internal/cloud/aws/runtime/validate.go
    - internal/cloud/aws/runtime/containers.go
    - internal/cloud/aws/runtime/sidecars.go
    - internal/cloud/aws/runtime/env.go
    - internal/cloud/aws/eksops/ops_test.go
  modified:
    - internal/platform/ops.go
    - internal/cloud/ovh/stack/ops.go
    - internal/cloud/ovh/stack/ops_test.go
    - internal/cloud/scaleway/stack/ops.go
    - internal/cloud/scaleway/stack/ops_test.go
    - internal/cloud/aws/eksops/ops.go
  deleted:
    - internal/cloud/aws/runtime/runtime.go

key-decisions:
  - "Warn+noop AcquireLock (not ErrNotSupported) so preview/apply keep working until Phase 6 real locks"
  - "Shared platform.WarnNoDIYLock for stable message shape across three adapters"
  - "AST option (a): drop Ops from ErrNotSupported walk; dedicated warning tests cover AcquireLock"
  - "F-01-07-1 search-proxy DependsOn gap left deferred — untouched during split"

patterns-established:
  - "diyLockWarnOut package sink for redirectable honesty warnings without Ops interface change"
  - "QUALITY-07 file map: types/identity/component/validate/containers/sidecars/env"

requirements-completed: [QUALITY-07, TRUST-02]

coverage:
  - id: D1
    description: No non-test file under internal/cloud/aws/runtime exceeds 400 lines; runtime.go removed; New in component.go
    requirement: QUALITY-07
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test -race ./internal/cloud/aws/runtime/ -count=1 (incl. SigV4 + combination rows)"
        status: pass
    human_judgment: false
  - id: D2
    description: ovh/scaleway/eksops AcquireLock warns that DIY lock was not taken then returns noop release; AST carve-outs gone
    requirement: TRUST-02
    verification:
      - kind: unit
        ref: "TestAcquireLockWarnsNoDIYLockTaken in ovh/stack, scaleway/stack, eksops"
        status: pass
    human_judgment: false

duration: 10min
completed: 2026-07-28
status: complete
---

# Phase 1 Plan 10: QUALITY-07 Split & AcquireLock Honesty Summary

**AWS ECS runtime construction split into seven ≤400-line concern files; experimental AcquireLock now warns that no DIY lock was taken before returning a noop release.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-07-28T14:12:51Z
- **Completed:** 2026-07-28T14:22:32Z
- **Tasks:** 2/2
- **Files modified:** 15 (7 created, 1 deleted, 7 modified)

## Accomplishments

- Replaced 991-line `runtime.go` with seven concern files; largest non-test file is `containers.go` at 226 lines
- Full `./internal/cloud/aws/runtime/` race suite green with zero intentional `runtime_test.go` edits (SigV4 + 01-07 combination rows intact)
- ovh, scaleway, and eksops `AcquireLock` emit a stable stderr warning then return noop release; 01-08 AST AcquireLock carve-outs removed

## Task Commits

1. **Task 1: Split aws/runtime by concern (QUALITY-07)** - `4c73516` (feat)
2. **Task 2 RED: AcquireLock warning tests + AST narrow** - `43ae293` (test)
3. **Task 2 GREEN: WarnNoDIYLock + adapter wiring** - `da47fa2` (feat)

**Plan metadata:** `9400582` (docs: complete plan)

## Files Created/Modified

### Non-test line counts (`internal/cloud/aws/runtime/`)

| File | Lines |
|------|------:|
| `types.go` | 114 |
| `identity.go` | 154 |
| `component.go` | 197 |
| `validate.go` | 135 |
| `containers.go` | 226 |
| `sidecars.go` | 87 |
| `env.go` | 131 |

- `runtime.go` — deleted
- `internal/platform/ops.go` — AcquireLock doc + `WarnNoDIYLock`
- `internal/cloud/ovh/stack/ops.go` (+ `ops_test.go`) — warn sink + warning test; AST carve-out dropped
- `internal/cloud/scaleway/stack/ops.go` (+ `ops_test.go`) — same
- `internal/cloud/aws/eksops/ops.go` + new `ops_test.go` — same

## Decisions Made

- Prefer warn+noop over `ErrNotSupported` for AcquireLock (ROADMAP criterion 5 / 01-RESEARCH)
- Message shape via `platform.WarnNoDIYLock`; adapters inject `diyLockWarnOut` (defaults to `os.Stderr`)
- Narrow AST sentinel to `unsupported` receivers only; positive warning tests own AcquireLock honesty

## AcquireLock warning text (verbatim)

```
warning: DIY deployment lock was not taken for %s
```

Examples: `ovh.mks`, `scaleway.kapsule`, `aws.eks-autopilot` (from `TargetDescriptor().ID`).

## AST guard narrowing

Dropped `acquireLockException` / `acquireLockReason` / `t.Logf("exception …")` and all "plan 01-10 revisits" comments from ovh and scaleway `ops_test.go`. The ErrNotSupported walk now applies only to `unsupported` receivers. AcquireLock honesty is asserted by `TestAcquireLockWarnsNoDIYLockTaken`.

## F-01-07-1

Untouched. Search-proxy `DependsOn` gap remains deferred (Finding F-01-07-1). Sidecar DependsOn strings moved with the split only — no logic change.

## Deferred (explicitly out of scope)

- Hosted CI / QUALITY-06 Actions proof — still HUMAN_GATE
- F-01-07-1 DependsOn ordering fix

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Warning text vs test substring mismatch**
- **Found during:** Task 2 GREEN
- **Issue:** First message used "no DIY … was taken", which does not contain the required substring "not taken"
- **Fix:** Message is now `warning: DIY deployment lock was not taken for %s`
- **Files modified:** `internal/platform/ops.go`
- **Committed in:** `da47fa2`

## Known Stubs

None.

## Threat Flags

None beyond plan threat model (T-01-38 mitigated by warning + tests).

## Self-Check: PASSED

- FOUND: seven non-test runtime files; no `runtime.go`
- FOUND commits: `4c73516`, `43ae293`, `da47fa2`
- FOUND: carve-out strings absent from ovh/scaleway ops_test.go
- FOUND: race tests pass for runtime + ovh/scaleway/eksops
