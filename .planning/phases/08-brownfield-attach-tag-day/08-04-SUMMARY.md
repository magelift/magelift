---
phase: 08-brownfield-attach-tag-day
plan: 04
subsystem: infra
tags: [aws, adopt, brownfield, refuse-before-mutate, network, database, preview]

requires:
  - phase: 08-01
    provides: AdoptReport + RefuseAdoptedMutation for network
  - phase: 08-03
    provides: Spec.Existing.Database wired into database.Existing
provides:
  - Unified AdoptReport entries for network + database
  - RefuseAdoptedMutation naming every adopted externalId
  - Offline detach proof: no Delete for adopted VPC/RDS IDs
affects: [08-05, 08-06, ATTACH-03, ATTACH-04]

tech-stack:
  added: []
  patterns:
    - "AdoptReport accumulates independent network then database ADOPT entries"
    - "RefuseAdoptedMutation joins all adopted labels/externalIds in one ownership error"
    - "Offline detach: empty intent allows owned-child destroy; adopted IDs absent from mock state"

key-files:
  created: []
  modified:
    - internal/cloud/aws/stack/adopt.go
    - internal/cloud/aws/stack/adopt_test.go
    - internal/cli/lifecycle.go
    - internal/cli/lifecycle_test.go
    - internal/cli/stub_module_test.go

key-decisions:
  - "AdoptReport emits network and database independently (one or both)"
  - "Refuse lists every adopted resource in a single error when both are set"
  - "ATTACH-04 offline half via mock state exclusion + refuse; live describe-after-destroy deferred to 08-06"

patterns-established:
  - "ADOPT {kind} {externalId} lines stay distinct from Pulumi CREATE ChangeSummary"
  - "AdoptedRefuse* tests assert named failure for each resource kind"

requirements-completed: [ATTACH-03]

coverage:
  - id: D1
    description: Preview reports ADOPT network and ADOPT database lines with externalIds when both Existing refs are set; single-ref configs emit only that line
    requirement: ATTACH-03
    verification:
      - kind: unit
        ref: go test ./internal/cloud/aws/stack/ -run 'AdoptReport' -count=1
        status: pass
      - kind: unit
        ref: go test ./internal/cli/ -run 'PreviewReportsAdopt' -count=1
        status: pass
    human_judgment: false
  - id: D2
    description: RefuseAdoptedMutation fails closed naming database and network externalIds on destroy/replace intent
    requirement: ATTACH-03
    verification:
      - kind: unit
        ref: go test ./internal/cloud/aws/stack/ -run 'AdoptedRefuse|RefuseAdopted' -count=1
        status: pass
      - kind: unit
        ref: go test ./internal/cli/ -run 'InfrastructureRefuseAdopted' -count=1
        status: pass
    human_judgment: false
  - id: D3
    description: Offline detach/destroy of MageLift-owned children records no Delete for adopted VPC/RDS external IDs
    requirement: ATTACH-03
    verification:
      - kind: unit
        ref: go test ./internal/cloud/aws/stack/ -run 'TestAdoptedDetachDestroyRecordsNoDeleteForAdoptedIDs' -count=1
        status: pass
      - kind: unit
        ref: go test ./internal/cloud/aws/database/ -run 'Existing' -count=1
        status: pass
      - kind: unit
        ref: go test ./internal/cloud/aws/network/ -run 'ExistingNetwork' -count=1
        status: pass
    human_judgment: false

duration: 3min
completed: 2026-07-30
status: complete
---

# Phase 08 Plan 04: Unified ADOPT + Refuse for Network+DB Summary

**Preview honesty and refuse-before-mutate now cover both adopted VPC and RDS: operators see independent ADOPT lines, and destroy/replace intent fails naming every adopted externalId.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-07-30T12:17:47Z
- **Completed:** 2026-07-30T12:20:50Z
- **Tasks:** 2/2
- **Files modified:** 5

## Accomplishments
- `AdoptReport` emits `ADOPT network` and/or `ADOPT database` from `Spec.Existing`; CLI preview JSON/`stderr` surfaces both
- `RefuseAdoptedMutation` fails closed on destroy/replace naming all adopted resources (network regression retained)
- Offline detach proof: mock owned-child destroy never Deletes adopted VPC/RDS IDs; live describe-after-destroy left to 08-06

## Task Commits

Each task was committed atomically:

1. **Task 1: Unified ADOPT preview for network + database**
   - `1c06b2e` (test) — failing AdoptReport + CLI preview tests
   - `74c1106` (feat) — AdoptReport + stub Plan emit network+database ADOPT lines
2. **Task 2: Refuse destroy/replace for adopted DB + detach mock**
   - `aa9f167` (test) — AdoptedRefuse + detach offline proofs
   - `ff7af3e` (feat) — multi-resource RefuseAdoptedMutation

**Plan metadata:** (pending docs commit)

## Files Created/Modified
- `internal/cloud/aws/stack/adopt.go` — unified AdoptReport + multi-resource refuse
- `internal/cloud/aws/stack/adopt_test.go` — AdoptReport/AdoptedRefuse/detach tests
- `internal/cli/lifecycle.go` — refuse comment covers network+database
- `internal/cli/lifecycle_test.go` — preview + refuse CLI coverage for database
- `internal/cli/stub_module_test.go` — stub Plan/refuse mirrors stack contract

## Decisions Made
- Emit independent ADOPT entries (order: network then database) so single-ref configs stay quiet for the unset kind
- One refuse error naming every adopted label/externalId when both are set (T-08-10)
- ATTACH-04 offline half via state-exclusion mock + empty-intent allow; no detach CLI (08-05 docs)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- ATTACH-03 offline honesty closed for network+DB
- Ready for 08-05 (attach/detach docs) and 08-06 (live HUMAN_GATE describe-after-destroy)

## TDD Gate Compliance
- Task 1: RED `1c06b2e` → GREEN `74c1106`
- Task 2: RED `aa9f167` → GREEN `ff7af3e`

## Self-Check: PASSED

---
*Phase: 08-brownfield-attach-tag-day*
*Completed: 2026-07-30*
