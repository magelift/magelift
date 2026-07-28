---
phase: 03-credit-efficient-acceptance-harness-evidence-tiering
plan: 01
subsystem: testing
tags: [acceptance, harness, checkpoint, evidence, dry-run]

requires: []
provides:
  - Offline dry-run AWS acceptance harness with cell catalog
  - Checkpoint/resume helpers under .magelift/acceptance-checkpoint.json
  - Evidence append_row with SC#2 six-column markdown rows
  - make acceptance-harness-test serial shell suite
affects:
  - 03-02 assert_clean extraction
  - 03-06 paid AWS HUMAN_GATE proof

tech-stack:
  added: []
  patterns:
    - MAGELIFT_ACCEPTANCE_DRY_RUN=1 short-circuits before Pulumi/AWS mutate
    - create-once then cell-update log markers for multi-cell KEEP semantics

key-files:
  created:
    - scripts/acceptance/lib-checkpoint.sh
    - scripts/acceptance/lib-evidence.sh
    - scripts/acceptance/cells-aws-preview.txt
    - tests/acceptance/checkpoint_resume_test.sh
    - tests/acceptance/evidence_append_test.sh
  modified:
    - scripts/aws-acceptance-local.sh
    - docs/aws-acceptance.md
    - Makefile

key-decisions:
  - "Env var MAGELIFT_ACCEPTANCE_DRY_RUN=1 for offline fixture path"
  - "Checkpoint JSON schema {cells:{id:{result,at}}} under .magelift/"
  - "Free-tier preview cells: queueMode db, ecs-rabbitmq, ecs-artemis"

patterns-established:
  - "Pattern: dry-run logs acceptance create-once once then acceptance cell-update per cell"
  - "Pattern: harness-only writer for matrix-results.md via append_row"

requirements-completed: [ACCEPT-01, ACCEPT-02, ACCEPT-03]

coverage:
  - id: D1
    description: Dry-run iterates ordered cell catalog with create-once then update-only markers
    requirement: ACCEPT-01
    verification:
      - kind: unit
        ref: tests/acceptance/evidence_append_test.sh
        status: pass
    human_judgment: false
  - id: D2
    description: Checkpoint resume skips completed cell and continues at next
    requirement: ACCEPT-02
    verification:
      - kind: unit
        ref: tests/acceptance/checkpoint_resume_test.sh
        status: pass
    human_judgment: false
  - id: D3
    description: append_row writes six SC#2 columns; header once across successive appends
    requirement: ACCEPT-03
    verification:
      - kind: unit
        ref: tests/acceptance/evidence_append_test.sh
        status: pass
    human_judgment: false

duration: 12min
completed: 2026-07-28
status: complete
---

# Phase 3 Plan 01: Offline Harness Core Summary

**Dry-run AWS acceptance harness with checkpoint resume and automatic SC#2 evidence append — proven offline with zero cloud spend.**

## Performance

- **Duration:** 12 min
- **Started:** 2026-07-28T15:50:52Z
- **Completed:** 2026-07-28T16:02:00Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments

- Shared `scripts/acceptance/` libs for checkpoint JSON and markdown evidence rows
- Free-tier preview cell catalog (≥3 queueMode cells; Aurora/amazon-mq/OpenSearch excluded)
- `MAGELIFT_ACCEPTANCE_DRY_RUN=1` short-circuits before any magelift preview/promote/deploy/destroy
- Serial `make acceptance-harness-test` runs both shell tests

## Task Commits

1. **Task 1: End-to-end dry-run tracer** - `870c06d` (feat)
2. **Task 2: Evidence append + create-once contract** - `9b5bfa5` (test)

## Files Created/Modified

- `scripts/acceptance/lib-checkpoint.sh` — load/save/cell_done/record_cell
- `scripts/acceptance/lib-evidence.sh` — append_row with six SC#2 columns
- `scripts/acceptance/cells-aws-preview.txt` — ordered free-tier cells
- `scripts/aws-acceptance-local.sh` — dry-run cell loop + live path preserved
- `tests/acceptance/checkpoint_resume_test.sh` — ACCEPT-02 offline proof
- `tests/acceptance/evidence_append_test.sh` — ACCEPT-01/03 offline proof
- `docs/aws-acceptance.md` — dry-run, paths, resume/lock hygiene
- `Makefile` — `acceptance-harness-test` target

## Decisions Made

- Named the dry-run flag `MAGELIFT_ACCEPTANCE_DRY_RUN` (documented in aws-acceptance.md)
- Checkpoint path override via `ACCEPTANCE_CHECKPOINT` / `MAGELIFT_ACCEPTANCE_CHECKPOINT`
- Live multi-cell paid proof deferred to 03-06 HUMAN_GATE

## Deviations from Plan

None - plan executed exactly as written.

## Self-Check: PASSED

- FOUND: scripts/acceptance/lib-checkpoint.sh
- FOUND: scripts/acceptance/lib-evidence.sh
- FOUND: scripts/acceptance/cells-aws-preview.txt
- FOUND: tests/acceptance/checkpoint_resume_test.sh
- FOUND: tests/acceptance/evidence_append_test.sh
- FOUND: 870c06d
- FOUND: 9b5bfa5
