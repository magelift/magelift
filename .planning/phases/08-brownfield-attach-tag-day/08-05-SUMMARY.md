---
phase: 08-brownfield-attach-tag-day
plan: 05
subsystem: docs
tags: [brownfield, attach, detach, adr-0010, ATTACH-04]

requires:
  - phase: 08-03
    provides: existing.database adopt wiring
  - phase: 08-04
    provides: Unified ADOPT + RefuseAdoptedMutation + offline detach proof
provides:
  - docs/brownfield-attach.md runbook (attachable / not / egress / detach)
  - ADR 0010 attach-out-of-scope consequence superseded (ATTACH-02)
  - Cross-links from index, migrating-from-paas, post-beta, configuration, ADR README
affects: [08-06, ATTACH-04, RELEASE-05]

tech-stack:
  added: []
  patterns:
    - "Manual un-adopt detach (no magelift detach CLI)"
    - "Offline detach proof cited from docs ↔ adopt_test.go"

key-files:
  created:
    - docs/brownfield-attach.md
    - docs/adr/0010-database-dump-seed.md
  modified:
    - docs/adr/README.md
    - docs/index.md
    - docs/migrating-from-paas.md
    - docs/post-beta-roadmap.md
    - docs/configuration.md
    - internal/cloud/aws/stack/adopt_test.go

key-decisions:
  - "No magelift detach CLI — documented manual un-adopt only"
  - "ADR 0010 dump-seed decision unchanged; only attach-out-of-scope bullet superseded"
  - "Live describe-after-destroy deferred to 08-06; offline proof cited here"

patterns-established:
  - "Brownfield runbook is the operator source of truth for ATTACH-04 limits"
  - "Post-beta roadmap row narrowed to remaining multi-cloud / Cloud SQL attach"

requirements-completed: [ATTACH-04]

coverage:
  - id: D1
    description: Documented attach limits — AWS VPC+RDS attachable; Cloud SQL / non-AWS / Pulumi destroy-ownership import not; egress owned by operator in existing.network mode
    requirement: ATTACH-04
    verification:
      - kind: other
        ref: test -f docs/brownfield-attach.md && rg detach|existing.database|egress|Cloud SQL docs/brownfield-attach.md
        status: pass
    human_judgment: false
  - id: D2
    description: Detach path documented as manual un-adopt; offline proof TestAdoptedDetachDestroyRecordsNoDeleteForAdoptedIDs cited; no live AWS claim
    requirement: ATTACH-04
    verification:
      - kind: unit
        ref: GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cloud/aws/stack/ -count=1 -run 'Adopt|Refuse|Adopted|Detach'
        status: pass
      - kind: other
        ref: rg adopt_test|RefuseAdopted|Detach docs/brownfield-attach.md
        status: pass
    human_judgment: false
  - id: D3
    description: ADR 0010 no longer claims brownfield attach DB out of scope — superseded by Phase 8 / ATTACH-02; dump-seed otherwise unchanged
    requirement: ATTACH-04
    verification:
      - kind: other
        ref: rg ATTACH-02|Phase 8|supersed docs/adr/0010-database-dump-seed.md
        status: pass
    human_judgment: false

duration: 2min
completed: 2026-07-30
status: complete
---

# Phase 8 Plan 05: Attach docs + ADR 0010 supersede Summary

**Operator runbook for AWS VPC/RDS attach limits and manual detach; ADR 0010 honesty restored for ATTACH-02.**

## Performance

- **Duration:** 2 min
- **Started:** 2026-07-30T12:21:42Z
- **Completed:** 2026-07-30T12:23:01Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments

- Added `docs/brownfield-attach.md` covering attachable resources, milestone exclusions (Cloud SQL, non-AWS, Pulumi destroy-ownership import), egress ownership, and manual detach.
- Superseded ADR 0010’s “attach existing DB out of scope” consequence with Phase 8 / ATTACH-02; dump-seed decision unchanged.
- Cross-linked from index, migrating-from-paas, post-beta roadmap, configuration, and ADR README; offline detach test cites the runbook.

## Task Commits

1. **Task 1: Write brownfield-attach runbook + ADR 0010 supersede** - `a8fefa3` (docs)
2. **Task 2: Keep offline detach proof aligned with docs** - `6e193a8` (docs)

**Plan metadata:** `f3e99e4` (docs: complete plan)

## Files Created/Modified

- `docs/brownfield-attach.md` — ATTACH-04 runbook
- `docs/adr/0010-database-dump-seed.md` — supersede note for attach scope
- `docs/adr/README.md` — ADR 0010 index note
- `docs/index.md` — brownfield-attach row
- `docs/migrating-from-paas.md` — Phase 8 attach pointer
- `docs/post-beta-roadmap.md` — AWS attach shipped; remaining multi-cloud
- `docs/configuration.md` — link to runbook
- `internal/cloud/aws/stack/adopt_test.go` — doc cite on detach proof test

## Decisions Made

- No `magelift detach` CLI (plan discretion).
- Live `aws ec2/rds describe-*` after destroy stays 08-06 HUMAN_GATE.

## Deviations from Plan

None - plan executed exactly as written.

## TDD Gate Compliance

Task 2 has `tdd="true"` but reuses the existing 08-04 offline detach suite (already green). No new RED failing test was added — alignment was doc cite + discoverability comment only. Adopt/refuse/detach package tests passed after the comment change.

## Known Stubs

None.

## Threat Flags

None beyond plan threat model (T-08-13 / T-08-14 mitigated by explicit detach steps + ADR supersede).

## Self-Check: PASSED

- FOUND: docs/brownfield-attach.md, docs/adr/0010-database-dump-seed.md, 08-05-SUMMARY.md
- FOUND: commits a8fefa3, 6e193a8
