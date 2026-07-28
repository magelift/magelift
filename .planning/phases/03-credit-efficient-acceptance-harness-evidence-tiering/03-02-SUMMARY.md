---
phase: 03-credit-efficient-acceptance-harness-evidence-tiering
plan: 02
subsystem: testing
tags: [acceptance, assert-clean, stub, teardown]

requires:
  - phase: 03-01
    provides: acceptance harness libs and dry-run driver
provides:
  - Shared lib-assert-clean-aws.sh
  - Offline stub dual-outcome (clean→0, leftover→non-zero)
  - EXIT path sources shared helper
affects:
  - 03-06 live leftover HUMAN_GATE proof

tech-stack:
  added: []
  patterns:
    - MAGELIFT_ACCEPTANCE_AWS_STUB=1 + PATH-isolated fake aws
    - KEEP gate still wraps destroy + assert_clean

key-files:
  created:
    - scripts/acceptance/lib-assert-clean-aws.sh
    - tests/acceptance/assert_clean_stub_test.sh
  modified:
    - scripts/aws-acceptance-local.sh
    - docs/aws-acceptance.md
    - Makefile

key-decisions:
  - "Stub only under MAGELIFT_ACCEPTANCE_AWS_STUB=1 with PATH isolation"
  - "Live leftover half deferred to 03-06 HUMAN_GATE"

patterns-established:
  - "Pattern: assert_clean_aws PROJECT_TAG REGION shared helper"

requirements-completed: [ACCEPT-04]

coverage:
  - id: D1
    description: Stubbed assert_clean returns 0 when all describes report zero leftovers
    requirement: ACCEPT-04
    verification:
      - kind: unit
        ref: tests/acceptance/assert_clean_stub_test.sh --clean
        status: pass
    human_judgment: false
  - id: D2
    description: Stubbed assert_clean returns non-zero with FAILED message on leftovers
    requirement: ACCEPT-04
    verification:
      - kind: unit
        ref: tests/acceptance/assert_clean_stub_test.sh --leftover
        status: pass
    human_judgment: false

duration: 6min
completed: 2026-07-28
status: complete
---

# Phase 3 Plan 02: assert_clean Dual Outcome Summary

**Shared AWS assert_clean extracted and proven offline for both clean→0 and leftover→non-zero via PATH-isolated stub — live leftover half deferred to 03-06.**

## Performance

- **Duration:** 6 min
- **Started:** 2026-07-28T16:10:00Z
- **Completed:** 2026-07-28T16:16:00Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments

- Extracted VPC/RDS/ElastiCache/ALB/ECS/logs/SG queries into `lib-assert-clean-aws.sh`
- EXIT cleanup sources shared helper; KEEP gate unchanged
- Stub test covers clean and leftover; never touches a real account

## Task Commits

1. **Task 1: Extract assert_clean + clean stub** - `a51d5de` (feat)
2. **Task 2: Leftover stub + docs** - `aa2d1b1` (test)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing critical functionality] Leftover mode shipped in tracer test file**
- **Found during:** Task 1
- **Issue:** Tracer test included `--leftover` alongside `--clean` to keep one harness
- **Fix:** Task 2 focused on docs + Makefile wiring; leftover behavior already verified
- **Files modified:** tests/acceptance/assert_clean_stub_test.sh
- **Commit:** a51d5de

## Self-Check: PASSED

- FOUND: scripts/acceptance/lib-assert-clean-aws.sh
- FOUND: tests/acceptance/assert_clean_stub_test.sh
- FOUND: a51d5de
- FOUND: aa2d1b1
