---
phase: 05-data-migration-cutover
plan: 06
subsystem: migration
tags: [cutover, runbook, migrate-04, human-gate, d-06]

requires:
  - phase: 05-data-migration-cutover
    provides: env import-dump + media-sync + dumpimport offline proofs (05-03..05-05)
provides:
  - Cutover runbook in docs/migrating-from-paas.md (DNS, maintenance, reindex, verification, rollback)
  - scratch/05-cutover-local-proof.md offline dump+media+command-shape proof
  - MIGRATE-04 honesty split: Phase 5 local / Phase 7 HUMAN_GATE for DNS+live+managed dump
affects: [phase-7-gcp-human-gate, phase-5-verification]

tech-stack:
  added: []
  patterns:
    - "MIGRATE-04 split: local runbook+scratch in Phase 5; live DNS under Phase 7 HUMAN_GATE"
    - "No fourth paid AWS pass in Phase 5 for cutover evidence"

key-files:
  created:
    - .planning/phases/05-data-migration-cutover/scratch/05-cutover-local-proof.md
    - .planning/phases/05-data-migration-cutover/05-06-SUMMARY.md
  modified:
    - docs/migrating-from-paas.md
    - docs/capability-matrix.md
    - .planning/REQUIREMENTS.md
    - .planning/STATE.md

key-decisions:
  - "D-06: Phase 5 closes local cutover half only; MIGRATE-04 stays Pending until Phase 7 HUMAN_GATE"
  - "Scratch uses dumpimport MySQL docker + mediasync/cli unit proofs; Floci media skipped this session (prior 05-05)"
  - "No paid AWS pass invented for SC5 live"

patterns-established:
  - "Evidence honesty table in migrating-from-paas cutover section"

requirements-completed: []  # MIGRATE-04 intentionally Pending — local half done; live DNS/managed dump = Phase 7 HUMAN_GATE

coverage:
  - id: D1
    description: Cutover runbook documents DNS, maintenance, reindex, verification, rollback + Phase 7 gate
    requirement: MIGRATE-04
    verification:
      - kind: other
        ref: "python keyword gate on docs/migrating-from-paas.md + make check-clean-room"
        status: pass
    human_judgment: false
  - id: D2
    description: Offline scratch proof for dump+media+day-2 command shapes without claiming DNS/live
    requirement: MIGRATE-04
    verification:
      - kind: unit
        ref: "scratch/05-cutover-local-proof.md (dumpimport TestImport PASS; mediasync PASS; cli PASS)"
        status: pass
    human_judgment: false
  - id: D3
    description: REQUIREMENTS/STATE record Phase 7 HUMAN_GATE split; MIGRATE-04 not marked Complete
    requirement: MIGRATE-04
    verification:
      - kind: other
        ref: "rg Phase 7|HUMAN_GATE in REQUIREMENTS.md + STATE.md; MIGRATE-04 checkbox unchecked"
        status: pass
    human_judgment: false

duration: 4min
completed: 2026-07-29
status: complete
---

# Phase 5 Plan 06: Cutover Runbook Summary

**Documented cutover runbook + offline scratch proof for dump/media/maintenance/reindex/verify/rollback shapes, with MIGRATE-04 kept Pending until Phase 7 DNS/live HUMAN_GATE (D-06).**

## Performance

- **Duration:** 4 min
- **Started:** 2026-07-29T16:23:03Z
- **Completed:** 2026-07-29T16:27:00Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments

- Replaced deferred dump/media one-liner with full cutover runbook referencing real CLI (`env import-dump`, `env media-sync`, `exec` maintenance, `reindex`, destroy/DNS rollback)
- Recorded offline scratch proof (dumpimport MySQL + mediasync/cli units); Floci media not re-run (endpoint down; prior 05-05)
- Locked MIGRATE-04 honesty split in REQUIREMENTS + STATE — no Complete for live DNS; no Phase 5 paid AWS pass
- Updated capability-matrix media row to unit+Floci media-sync tier

## Task Commits

1. **Task 1: Cutover runbook sections in migrating-from-paas.md** - `d6fc067` (docs)
2. **Task 2: Local scratch proof + REQUIREMENTS/STATE Phase 7 split** - `226df75` (docs)

## Files Created/Modified

- `docs/migrating-from-paas.md` — cutover runbook + evidence honesty table
- `docs/capability-matrix.md` — media-sync Floci/unit evidence note
- `.planning/phases/05-data-migration-cutover/scratch/05-cutover-local-proof.md` — offline proof log
- `.planning/REQUIREMENTS.md` — MIGRATE-04 Pending + Phase 7 HUMAN_GATE split
- `.planning/STATE.md` — session continuity for the split

## Decisions Made

- Keep MIGRATE-04 checkbox unchecked; Phase 5 only closes local documentation + scratch
- Skip re-running Floci this session; cite 05-05 rather than invent live LocalStack evidence
- Maintenance/reindex/verify/rollback against ECS left as documented command shapes (no AWS account)

## Deviations from Plan

### Auto-fixed Issues

None - plan executed as written.

### Notes

- Floci media-sync re-proof skipped (`localhost:4566` unavailable); unit listing-diff green; prior Floci pass recorded in 05-05-SUMMARY — not claimed as new Floci evidence here.

## Auth Gates

None.

## Known Stubs

None.

## Threat Flags

None beyond plan register (T-05-14/15 mitigated by scratch + REQUIREMENTS/STATE split + rollback section).

## Self-Check: PASSED

- Runbook, scratch, REQUIREMENTS/STATE, SUMMARY present
- Commits d6fc067, 226df75 present
