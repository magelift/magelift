---
phase: 02-tag-ready-release-surface
plan: 01
subsystem: docs
tags: [versioning, release-contract, rc, semver]

requires:
  - phase: 01-publishable-baseline-honest-fallbacks
    provides: Phase 1 offline baseline; hosted CI still HUMAN_GATE deferred
provides:
  - First public tag named as v1.0.0-rc.1 on README, versioning, gate board
  - RC stability statement for sdk/v1 and platform.StackModule with Phase 6 reservation
  - Satellite product-status claims stripped; CHANGELOG Unreleased aligned
affects: [02-02-contributor-path, 02-03-custom-cli, 02-04-packaging-smoke]

tech-stack:
  added: []
  patterns: [RC freeze surface + explicit Phase 6 kube-port reservation]

key-files:
  created:
    - docs/versioning.md
  modified:
    - README.md
    - docs/release-readiness.md
    - CONTRIBUTING.md
    - SUPPORT.md
    - SECURITY.md
    - CHANGELOG.md

key-decisions:
  - "First public tag is v1.0.0-rc.1 on all three criterion-1 surfaces (D-01)"
  - "RC stability locks sdk/v1 Target/Capability/Hook and StackModule core; Phase 6 may change kube-shaped platform ports without v2"
  - "Packaging smoke row left Partial for plan 02-04 / D-03"

patterns-established:
  - "Product-status claims live in README/versioning/gate board; satellites stay purpose-only"
  - "CHANGELOG historical entries untouched; Unreleased forward claim tracks RC.1"

requirements-completed: [RELEASE-01, RELEASE-02]

coverage:
  - id: D1
    description: README, versioning.md, and release-readiness gate board name v1.0.0-rc.1 as first public tag
    requirement: RELEASE-01
    verification:
      - kind: other
        ref: "rg -n 'v1\\.0\\.0-rc\\.1' README.md docs/versioning.md docs/release-readiness.md"
        status: pass
    human_judgment: false
  - id: D2
    description: RC stability statement for sdk/v1 + StackModule with Phase 6 shared-Kubernetes reservation
    requirement: RELEASE-02
    verification:
      - kind: other
        ref: "rg -n 'sdk/v1|StackModule|Phase 6' docs/versioning.md"
        status: pass
    human_judgment: false
  - id: D3
    description: Satellite strips + CHANGELOG Unreleased aligned; banned maturity/v0-first claims absent from product surfaces
    requirement: RELEASE-01
    verification:
      - kind: other
        ref: "! rg -n 'pre-alpha|v0\\.x' README.md docs/versioning.md docs/release-readiness.md CONTRIBUTING.md SUPPORT.md SECURITY.md"
        status: pass
    human_judgment: false

duration: 2min
completed: 2026-07-28
status: complete
---

# Phase 2 Plan 01: RC.1 Version Story Summary

**Public version story now names `v1.0.0-rc.1` with an explicit `sdk/v1` / `StackModule` RC stability statement that reserves Phase 6 shared-Kubernetes port churn.**

## Performance

- **Duration:** 2 min
- **Started:** 2026-07-28T14:42:42Z
- **Completed:** 2026-07-28T14:44:25Z
- **Tasks:** 3/3
- **Files modified:** 7

## Accomplishments

- README status banner, `docs/versioning.md`, and release-readiness contract row all name `v1.0.0-rc.1`
- RC stability matrix locks CLI/YAML/`sdk/v1`/`StackModule` core; Phase 6 `internal/cloud/kube` Observe/`deploy.Steps` consolidation explicitly reserved
- CONTRIBUTING / SUPPORT / SECURITY no longer carry pre-alpha product-status claims; Unreleased CHANGELOG points at RC.1
- Packaging smoke row remains **Partial** (plan 02-04)

## Task Commits

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 | End-to-end RC.1 naming (tracer) | d5c143a |
| 2 | RC stability + Phase 6 reservation | 3411046 |
| 3 | Satellite strip + CHANGELOG Unreleased | 194b769 |

## Deviations from Plan

None - plan executed exactly as written.

## Known Stubs

None.

## Self-Check: PASSED

- FOUND: docs/versioning.md, README.md, docs/release-readiness.md, CONTRIBUTING.md, SUPPORT.md, SECURITY.md, CHANGELOG.md
- FOUND: d5c143a, 3411046, 194b769
