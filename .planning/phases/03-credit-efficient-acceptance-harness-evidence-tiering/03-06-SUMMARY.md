---
phase: 03-credit-efficient-acceptance-harness-evidence-tiering
plan: 06
subsystem: testing
tags: [aws, acceptance, harness, paid, resume, assert_clean]

requires:
  - phase: 03-01
    provides: lib-checkpoint / lib-evidence
  - phase: 03-02
    provides: assert_clean AWS helper
  - phase: 03-03
    provides: free-tier cell catalog + matrix honesty
provides:
  - Live AWS free-tier create-once ≥3 cell proof with kill+resume
  - Dual assert_clean on real account (leftover≠0, clean=0)
  - Docs citations from harness evidence (capability-matrix, aws-acceptance, release-readiness)
affects:
  - Phase 7 GCP paid pass (reuses harness)
  - v1.0.0-rc.1 tag readiness after Phase 3

tech-stack:
  added: []
  patterns:
    - KEEP=true long-lived stack + RESUME skip create-once
    - Temp YAML queueMode patch via yq between cells

key-files:
  created:
    - .planning/phases/03-credit-efficient-acceptance-harness-evidence-tiering/scratch/03-06-paid-proof.md
  modified:
    - docs/aws-acceptance.md
    - docs/capability-matrix.md
    - docs/release-readiness.md

key-decisions:
  - "Acceptance digest must be signed MageLift php-runtime (GET /health 200)"
  - "GCP live deferred to Phase 7; AWS only for this HUMAN_GATE"

patterns-established:
  - "Pattern: kill after first cell PASS, unlock DIY lock, RESUME=1 for ACCEPT-02"

requirements-completed: [ACCEPT-01, ACCEPT-02, ACCEPT-03, ACCEPT-04]

coverage:
  - id: D1
    description: One create-once then ≥3 catalog cell updates without re-create
    requirement: ACCEPT-01
    verification:
      - kind: e2e
        ref: scratch/03-06-live-run-health.log (create-once + cell-update×3)
        status: pass
    human_judgment: false
  - id: D2
    description: Kill mid-matrix then resume skips completed cells
    requirement: ACCEPT-02
    verification:
      - kind: e2e
        ref: scratch/03-06-live-run-health.log (skip queueMode:db)
        status: pass
    human_judgment: false
  - id: D3
    description: Harness-written matrix-results six columns
    requirement: ACCEPT-03
    verification:
      - kind: e2e
        ref: .magelift/matrix-results.md
        status: pass
    human_judgment: false
  - id: D4
    description: assert_clean leftover non-zero and clean zero on real account
    requirement: ACCEPT-04
    verification:
      - kind: e2e
        ref: scratch/03-06-live-run-health.log (FAILED then ok)
        status: pass
    human_judgment: false

duration: ~3h
completed: 2026-07-29
status: complete
---

# Phase 3 Plan 06: Paid AWS Harness Proof Summary

**Live free-tier preview stack proved create-once → three queueMode cells → kill+resume → dual assert_clean; account destroyed clean afterward.**

## Performance

- **Duration:** ~3h wall (create ~10m13s; cells ~255s/254s/204s; destroy ~24m)
- **Started:** 2026-07-29T13:51:03Z
- **Completed:** 2026-07-29T14:41:25Z (destroy + clean)
- **Tasks:** 3 (preflight + HUMAN_GATE + live proof)
- **Files modified:** docs + scratch proof + this SUMMARY

## Accomplishments

- Signed php-runtime health digest; create-once ECS runtime health green
- Cells `db` / `ecs-rabbitmq` / `ecs-artemis` all PASS on one stack
- Resume skipped create-once and `queueMode:db`
- Leftover assert_clean FAILED; post-destroy assert_clean ok

## Deviations

- First live attempt used a non-runtime signed image → `/health` 404; rematerialized with health image + Cosign keyless sign
- Mid-cell kill required DIY `state unlock` after forced stop of in-flight deploy

## Next

- Phase 3 complete for paid AWS pass — update ROADMAP/STATE; GCP live remains Phase 7
