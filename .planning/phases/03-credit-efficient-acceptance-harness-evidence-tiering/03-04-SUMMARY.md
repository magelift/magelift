---
phase: 03-credit-efficient-acceptance-harness-evidence-tiering
plan: 04
subsystem: testing
tags: [floci, acceptance, port-coverage, unit-fake]

requires:
  - phase: 03-03
    provides: Day-2 port coverage table in capability-matrix
provides:
  - port_coverage_floci_gate_test.sh mapping mockable rows to tests
  - TestNewDeployStepsRejectsWrongBackend unit-fake
  - Green make floci-test for Floci-marked rows
affects:
  - 03-06 paid-only ports only

tech-stack:
  added: []
  patterns:
    - Gate fails closed when mockable row lacks test symbol
    - paid-only Bootstrap/ExecuteCommand exempt from Floci claims

key-files:
  created:
    - tests/acceptance/port_coverage_floci_gate_test.sh
    - internal/cloud/aws/ops/deploy_steps_test.go
  modified:
    - docs/capability-matrix.md
    - Makefile

key-decisions:
  - "NewDeploySteps covered by unit-fake reject path, not Floci"
  - "SelectTask unit-fake cited as PrepareExec helper coverage"

patterns-established:
  - "Pattern: port_coverage_floci_gate_test explicit symbol map"

requirements-completed: [ACCEPT-06]

coverage:
  - id: D1
    description: Every mockable port-coverage row maps to a passing test symbol
    requirement: ACCEPT-06
    verification:
      - kind: unit
        ref: tests/acceptance/port_coverage_floci_gate_test.sh
        status: pass
    human_judgment: false
  - id: D2
    description: make floci-test green for Floci-tagged coverage
    requirement: ACCEPT-06
    verification:
      - kind: integration
        ref: make floci-test
        status: pass
    human_judgment: false

duration: 15min
completed: 2026-07-28
status: complete
---

# Phase 3 Plan 04: Floci/Unit Port Coverage Summary

**ACCEPT-06 offline closed: mockable Day-2 ports gated to Floci/unit test symbols; make floci-test green; Bootstrap/ExecuteCommand remain paid-only.**

## Performance

- **Duration:** 15 min
- **Started:** 2026-07-28T16:16:00Z
- **Completed:** 2026-07-28T16:31:00Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments

- Gate script maps State/Secrets/TailLogs/CheckRuntime/AcquireLock/Media → Floci tests; PrepareExec/SelectTask/NewDeploySteps → unit-fakes
- Added `TestNewDeployStepsRejectsWrongBackend` for previously unmapped NewDeploySteps row
- `make floci-test` passed under GOMAXPROCS=1

## Task Commits

1. **Task 1: Gate + NewDeploySteps unit fake** - `5950f13` (feat)
2. **Task 2: Cite symbols + floci green** - `3649030` (test)

## Deviations from Plan

None material — existing Floci suite already covered most rows; gap was NewDeploySteps + gate.

## Self-Check: PASSED

- FOUND: tests/acceptance/port_coverage_floci_gate_test.sh
- FOUND: internal/cloud/aws/ops/deploy_steps_test.go
- FOUND: 5950f13
- FOUND: 3649030
