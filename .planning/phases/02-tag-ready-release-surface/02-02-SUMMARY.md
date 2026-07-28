---
phase: 02-tag-ready-release-surface
plan: 02
subsystem: docs
tags: [contributing, verify, release-06, human-gate]

requires:
  - phase: 02-tag-ready-release-surface
    provides: 02-01 RC.1 version story (satellite strip left CONTRIBUTING purpose-only)
provides:
  - CONTRIBUTING.md full verify prerequisites + honest hosted-CI deferral
  - VERIFY_PROOF_BLOCKED evidence for missing PHP/Composer (RELEASE-06 not closed)
affects: [02-03-custom-cli, release-06-reproof]

tech-stack:
  added: []
  patterns: [honest local-vs-hosted CI deferral in CONTRIBUTING]

key-files:
  created:
    - .planning/phases/02-tag-ready-release-surface/scratch/02-02-verify-proof.txt
  modified:
    - CONTRIBUTING.md
    - .planning/WINDOWS.md

key-decisions:
  - "Contributor gate is local make verify; hosted force-all remains HUMAN_GATE deferred (D-07)"
  - "RELEASE-06 not marked complete — PHP 8.2+ and Composer missing on proof host; recorded VERIFY_PROOF_BLOCKED"

patterns-established:
  - "Do not weaken CONTRIBUTING to Go-only and claim RELEASE-06 green"

requirements-completed: []  # RELEASE-06 blocked on user_setup (PHP+Composer)

coverage:
  - id: D1
    description: CONTRIBUTING lists make verify prerequisites including PHP/Composer/MkDocs and honest hosted-CI deferral
    requirement: RELEASE-06
    verification:
      - kind: other
        ref: "rg -ni 'PHP|Composer|MkDocs|deferred|HUMAN_GATE' CONTRIBUTING.md"
        status: pass
    human_judgment: false
  - id: D2
    description: Fresh-clone make verify proof
    requirement: RELEASE-06
    verification:
      - kind: other
        ref: "scratch/02-02-verify-proof.txt VERIFY_PROOF_BLOCKED"
        status: fail
    human_judgment: true
    rationale: "PHP 8.2+ and Composer must be installed on the proof host (user_setup); then re-run clean-checkout make verify"

duration: 2min
completed: 2026-07-28
status: complete
---

# Phase 2 Plan 02: CONTRIBUTING Verify Path Summary

**CONTRIBUTING.md now documents the full local `make verify` toolchain and Phase 1 hosted-CI deferral; fresh-clone proof is blocked until PHP 8.2+ and Composer are installed.**

## Performance

- **Duration:** 2 min
- **Started:** 2026-07-28T14:45:00Z
- **Completed:** 2026-07-28T14:45:45Z
- **Tasks:** 2/2
- **Files modified:** 3

## Accomplishments

- Prerequisites list Go, PHP 8.2+, Composer, MkDocs; golangci/go-licenses/actionlint via `make` `go run` pins
- Testing section states local `make verify` is the gate; Actions force-all / QUALITY-06 remains HUMAN_GATE deferred; `make ci-act-go` is not a full substitute
- No false "CI is green on main" claim
- Scratch proof: `VERIFY_PROOF_BLOCKED` — `php` / `composer` missing; failing target `php-test`

## Task Commits

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 | CONTRIBUTING verify narrative (tracer) | 0c97c15 |
| 2 | Fresh-clone verify proof (blocked) | bcb35b1 |

## Deviations from Plan

### Auto-fixed Issues

None.

### Documented blockers (plan-allowed)

**1. [user_setup] PHP 8.2+ and Composer missing**
- **Found during:** Task 2 precondition
- **Issue:** Host lacks `php` and `composer`; `make verify` includes `php-test`
- **Fix:** Did not weaken CONTRIBUTING to Go-only; wrote `VERIFY_PROOF_BLOCKED`; left RELEASE-06 incomplete
- **Files modified:** `scratch/02-02-verify-proof.txt`, `.planning/WINDOWS.md` (unmet-truth #8)
- **Commit:** bcb35b1

## Known Stubs

None that prevent the plan's documented-blocker path. RELEASE-06 green proof remains open.

## Self-Check: PASSED

- FOUND: CONTRIBUTING.md, scratch/02-02-verify-proof.txt
- FOUND: 0c97c15, bcb35b1
- VERIFY_PROOF_BLOCKED recorded (honest — not VERIFY_PROOF_OK)
