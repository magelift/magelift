---
phase: 03-credit-efficient-acceptance-harness-evidence-tiering
plan: 03
subsystem: docs
tags: [capability-matrix, trust, evidence-tiers, port-coverage]

requires: []
provides:
  - Per-cell evidence tier column on free-tier acceptance table
  - Unverifiable triad (Aurora CreateDBCluster, amazon-mq×preview, OpenSearch SigV4)
  - Day-2 port coverage map for ACCEPT-06
  - matrix_tier_guard_test.sh honesty anchors
affects:
  - 03-04 Floci gap fill against port-coverage rows
  - 03-06 paid acceptance claims

tech-stack:
  added: []
  patterns:
    - Tier ≤ evidence honesty rule in capability-matrix
    - Grep guard for TRUST-04 unverifiable anchors

key-files:
  created:
    - tests/acceptance/matrix_tier_guard_test.sh
  modified:
    - docs/capability-matrix.md

key-decisions:
  - "Under-claim ecs-rabbitmq/artemis as Pulumi mocks until paid harness cites them"
  - "PrepareExec / ECS ExecuteCommand marked unit-fake or paid-only (not Floci-certified)"
  - "Bootstrap OIDC remains paid-only"

patterns-established:
  - "Pattern: Unverifiable on maintainer accounts table with specific reasons"
  - "Pattern: Day-2 port coverage table drives ACCEPT-06 gap fill"

requirements-completed: [TRUST-03, TRUST-04, ACCEPT-06]

coverage:
  - id: D1
    description: Per-cell evidence tier column; queueMode db tiered; no over-claim
    requirement: TRUST-03
    verification:
      - kind: unit
        ref: tests/acceptance/matrix_tier_guard_test.sh
        status: pass
    human_judgment: false
  - id: D2
    description: Aurora CreateDBCluster, amazon-mq×preview, OpenSearch SigV4 unverifiable reasons
    requirement: TRUST-04
    verification:
      - kind: unit
        ref: tests/acceptance/matrix_tier_guard_test.sh
        status: pass
    human_judgment: false
  - id: D3
    description: Day-2 port coverage map checked in for ACCEPT-06 foundation
    requirement: ACCEPT-06
    verification:
      - kind: unit
        ref: tests/acceptance/matrix_tier_guard_test.sh
        status: pass
    human_judgment: false

duration: 8min
completed: 2026-07-28
status: complete
---

# Phase 3 Plan 03: Capability Matrix Honesty Summary

**Capability matrix now records per-cell evidence tiers, the TRUST-04 unverifiable triad with specific reasons, and a checked-in Day-2 port-coverage map for ACCEPT-06.**

## Performance

- **Duration:** 8 min
- **Started:** 2026-07-28T16:02:00Z
- **Completed:** 2026-07-28T16:10:00Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- Free-tier table gained an Evidence tier column; `queueMode: db` cites real-account acceptance; broker cells under-claim as Pulumi mocks
- Unverifiable section documents Aurora CreateDBCluster free-tier API block, amazon-mq×preview 2-AZ vs CLUSTER_MULTI_AZ, OpenSearch SigV4 deferred paid
- Day-2 port coverage maps Bootstrap through Ops and storage to Floci / mocks / unit-fake / paid-only
- `matrix_tier_guard_test.sh` greps required honesty anchors

## Task Commits

1. **Task 1: Evidence tiers + Aurora unverifiable** - `2774e4f` (feat)
2. **Task 2: TRUST-04 triad + port coverage guard** - `c7b905e` (docs)

## Decisions Made

- Prefer under-claim for ecs-rabbitmq/artemis until paid harness cites them
- Live ECS ExecuteCommand / OIDC bootstrap stay paid-only
- ACCEPT-06 Floci gap closure deferred to plan 03-04 (map only here)

## Deviations from Plan

None - plan executed exactly as written.

## Self-Check: PASSED

- FOUND: docs/capability-matrix.md
- FOUND: tests/acceptance/matrix_tier_guard_test.sh
- FOUND: 2774e4f
- FOUND: c7b905e
