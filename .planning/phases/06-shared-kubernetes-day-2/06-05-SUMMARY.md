---
phase: 06-shared-kubernetes-day-2
plan: 05
subsystem: infra
tags: [kubernetes, honesty, acquire-lock, capability-matrix, ErrNotSupported]

requires:
  - phase: 06-01
    provides: OVH/SCW concrete State + AES256 DIY lock seam
  - phase: 06-03
    provides: shared kube.Observe wiring
  - phase: 06-04
    provides: shared kube.Steps wiring
provides:
  - Trimmed unsupported shells (Bootstrap/Secrets/Cost only)
  - AcquireLock → State.Lock on OVH/SCW/EKS
  - Allowlist + nil-success tests
  - capability-matrix + experimental docs honesty for day-2
affects:
  - 06-06 offline integration gates
  - Phase 7 DNS / managed dump
  - TRUST-02 matrix consumers

tech-stack:
  added: []
  patterns:
    - "unsupported allowlist = Bootstrap/Secrets/Cost only; State/Observe/Steps off the shell"
    - "Ops.AcquireLock → State.Lock with magelift-cli-{host}-{pid} owner"

key-files:
  created: []
  modified:
    - internal/cloud/ovh/stack/ops.go
    - internal/cloud/ovh/stack/ops_test.go
    - internal/cloud/ovh/stack/state_test.go
    - internal/cloud/scaleway/stack/ops.go
    - internal/cloud/scaleway/stack/ops_test.go
    - internal/cloud/scaleway/stack/state_test.go
    - internal/cloud/aws/eksops/ops.go
    - internal/cloud/aws/eksops/ops_test.go
    - docs/capability-matrix.md
    - docs/ovh-experimental.md
    - docs/scaleway-experimental.md

key-decisions:
  - "Remove State methods from unsupported{} — type system now proves State cannot be the shell"
  - "Delete diyLockWarnOut on OVH/SCW/EKS; all three have State"
  - "Skip release-readiness.md — no Phase 6 gate row to update (D-06)"

patterns-established:
  - "Allowlist tests enumerate exactly 6 ErrNotSupported methods"
  - "AcquireLock offline unit asserts State.Lock path via missing-bucket / bad-planned errors"

requirements-completed: [KUBE-06, KUBE-07]

coverage:
  - id: D1
    description: OVH/SCW unsupported allowlist is Bootstrap/Secrets/Cost only; Observe/Steps/State not ErrNotSupported
    requirement: KUBE-07
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/ovh/stack/ ./internal/cloud/scaleway/stack/ -count=1 -run 'Unsupported|Allowlist'"
        status: pass
    human_judgment: false
  - id: D2
    description: Bootstrap/Secrets remain loud ErrNotSupported with nil-success guards
    requirement: KUBE-06
    verification:
      - kind: unit
        ref: "TestBootstrapSecretsNilSuccessGuards in ovh/scaleway stack ops_test.go"
        status: pass
    human_judgment: false
  - id: D3
    description: AcquireLock delegates to State.Lock on OVH/SCW/EKS (no warn-then-noop)
    requirement: KUBE-06
    verification:
      - kind: unit
        ref: "TestAcquireLockDelegatesToState in ovh/scaleway/eksops"
        status: pass
    human_judgment: false
  - id: D4
    description: capability-matrix + experimental docs record Observe+Steps+State offline and remaining gaps
    requirement: KUBE-07
    verification:
      - kind: other
        ref: "rg Observe+Steps+State|shared kube docs/capability-matrix.md docs/ovh-experimental.md docs/scaleway-experimental.md"
        status: pass
    human_judgment: false

duration: 4min
completed: 2026-07-30
status: complete
---

# Phase 06 Plan 05: Unsupported Honesty + AcquireLock Flip Summary

**Trimmed OVH/SCW unsupported shells to Bootstrap/Secrets/Cost, flipped AcquireLock through State.Lock on OVH/SCW/EKS, and aligned capability-matrix/experimental docs with offline Observe+Steps+State evidence.**

## Performance

- **Duration:** 4 min
- **Started:** 2026-07-30T11:09:01Z
- **Completed:** 2026-07-30T11:12:22Z
- **Tasks:** 2/2
- **Files modified:** 11

## Accomplishments

- Deleted State stubs from `unsupported{}` — only VerifyAccount/Ensure/List/Set/Remove/Estimate remain
- `Ops.AcquireLock` on OVH/SCW/EKS calls `State.Lock` with CLI owner string (removed warn-then-noop)
- Docs: `eks-autopilot` / `mks` / `kapsule` rows + experimental Day-2 bullets match code; tier stays experimental

## Task Commits

1. **Task 1: End-to-end remaining-unsupported allowlist + AcquireLock flip** - `4d71b5e` (feat)
2. **Task 2: Matrix + experimental docs honesty for day-2 surface** - `8fcc607` (docs)

**Plan metadata:** (pending final docs commit)

## Files Created/Modified

- `internal/cloud/ovh/stack/ops.go` — trimmed unsupported; AcquireLock → State.Lock
- `internal/cloud/scaleway/stack/ops.go` — same
- `internal/cloud/aws/eksops/ops.go` — AcquireLock → State.Lock; removed diyLockWarnOut
- `internal/cloud/*/stack/ops_test.go` — allowlist, nil-success, AcquireLock delegation tests
- `docs/capability-matrix.md` — day-2 honesty rows + shared kube note
- `docs/ovh-experimental.md` / `docs/scaleway-experimental.md` — Day-2 bullets no longer claim all ports ErrNotSupported

## Decisions Made

- Remove `diyLockWarnOut` entirely from the three adapters (State exists everywhere AcquireLock ran warn-noop)
- Do not touch `docs/release-readiness.md` (no Phase 6 gate row)
- Drop compile-impossible `got.(unsupported)` State assertions after State methods left the shell (Rule 1 / type-system honesty)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Impossible type assertion after trimming State from unsupported**
- **Found during:** Task 1 (test compile)
- **Issue:** `m.State().(unsupported)` / `got.(unsupported)` no longer compile because `unsupported` lacks Backup/Lock/…
- **Fix:** Assert concrete `State` type only; type system now encodes “State is not the shell”
- **Files modified:** `ops_test.go` / `state_test.go` (ovh + scaleway)
- **Verification:** `go test … -run 'Unsupported|Allowlist|AcquireLock|NilSuccess|Bootstrap|Secrets'` green
- **Committed in:** `4d71b5e`

## Threat Flags

None — no new network endpoints or auth paths; AcquireLock now enforces DIY exclusion (closes T-06-14).

## Known Stubs

None — remaining Bootstrap/Secrets/Cost `ErrNotSupported` are intentional experimental gaps (KUBE-06), not silent stubs.

## Self-Check: PASSED

- Files present: ops.go (ovh/scw/eksops), capability-matrix, experimental docs, 06-05-SUMMARY.md
- Commits present: `4d71b5e`, `8fcc607`
