---
phase: 04-brownfield-onramp-paas-import-ece-tools-parity
plan: 06
subsystem: docs
tags: [ece-parity, clean-room, paas-import, provenance, d-07, honesty]

requires:
  - phase: 04-brownfield-onramp-paas-import-ece-tools-parity
    provides: Shared ACC/Upsun mapper + D-07 allowlist (04-03); SCD strategy/threads (04-04); m2-hotfixes PatchApplier + ece-parity stub (04-05)
provides:
  - Complete docs/ece-parity.md matrix (no blank status cells)
  - D-07 allowlist operator documentation (ECE-04)
  - Migrating/post-beta/provenance updates for shipped importers (IMPORT-06)
  - scripts/check-clean-room.sh + make check-clean-room
affects:
  - Phase 5 dump/media docs (still deferred)
  - Operator trust in closed vs intentional-gap claims

tech-stack:
  added: []
  patterns:
    - "Parity matrix status vocabulary: closed | intentional-gap only"
    - "Clean-room gate: path-segment grep for forbidden sibling product trees"

key-files:
  created:
    - scripts/check-clean-room.sh
  modified:
    - docs/ece-parity.md
    - docs/migrating-from-paas.md
    - docs/post-beta-roadmap.md
    - docs/provenance.md
    - docs/knowledge/lessons/MageLift reference and clean-room policy.md
    - docs/configuration.md
    - internal/config/schema.go
    - mkdocs.yml
    - Makefile

key-decisions:
  - "Only mark closed what 04-03/04-04/04-05 SUMMARY shipped; QUALITY_PATCHES and long-tail stage vars stay intentional-gap"
  - "D-07 allowlist documented as frozen v1: CRYPT_KEY, UPDATE_URLS, SCD_STRATEGY, SCD_THREADS + relationship/cron structural maps"
  - "Post-beta roadmap drops PaaS config importers as future track; dump/media + brownfield attach remain deferred"

patterns-established:
  - "Honesty close-out: ece-parity rows are public commitments tied to shipped code"
  - "IMPORT-06 automation via make check-clean-room (offline, no vendoring)"

requirements-completed: [ECE-01, ECE-04, IMPORT-06]

coverage:
  - id: D1
    description: ece-parity.md lists every build/deploy/post-deploy hook and ACC/Upsun-shaped env setting as closed or intentional-gap with reason
    requirement: ECE-01
    verification:
      - kind: other
        ref: "python3 assert no blank status cells; m2-hotfixes present"
        status: pass
    human_judgment: false
  - id: D2
    description: D-07 allowlist mapping documented for operators; long-tail vars are intentional gaps
    requirement: ECE-04
    verification:
      - kind: other
        ref: "docs/ece-parity.md D-07 section + docs/migrating-from-paas.md importer section"
        status: pass
    human_judgment: false
  - id: D3
    description: Migration/provenance/knowledge lesson record shipped clean-room importers; clean-room script exits 0
    requirement: IMPORT-06
    verification:
      - kind: other
        ref: "bash scripts/check-clean-room.sh && make check-clean-room; grep from-acc migrating-from-paas.md"
        status: pass
    human_judgment: false

duration: 3min
completed: 2026-07-29
status: complete
---

# Phase 4 Plan 06: ece-parity honesty close-out Summary

**Complete ece-parity matrix (closed/intentional-gap only), D-07 allowlist docs, shipped importer migration wording, and offline clean-room vendoring check.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-07-29T15:43:59Z
- **Completed:** 2026-07-29T15:47:03Z
- **Tasks:** 2/2
- **Files modified:** 10

## Accomplishments

- Expanded `docs/ece-parity.md` with build/deploy/post-deploy hook rows and D-07 env/relationship allowlist — every data row status is `closed` or `intentional-gap` (ECE-01, D-06).
- Documented QUALITY_PATCHES and long-tail stage vars as intentional gaps; only 04-03/04-04/04-05 shipped behavior marked closed (T-04-12).
- Replaced post-beta-only importer wording with `magelift init --from-acc` / `--from-upsun` instructions (refuse/`--yes`, `--config-out`, unmapped sidecar, foreign-schema rejection).
- Added `scripts/check-clean-room.sh` + `make check-clean-room` wired into `verify` (IMPORT-06).

## Task Commits

1. **Task 1: Complete docs/ece-parity.md matrix + D-07 allowlist docs** - `7bc4ca3` (docs)
2. **Task 2: Update migration/provenance docs + clean-room check** - `0a5858b` (docs)

## Files Created/Modified

- `docs/ece-parity.md` — full parity matrix + D-07 allowlist
- `docs/configuration.md` / `internal/config/schema.go` — cross-link staticContent strategy/threads → ece-parity
- `mkdocs.yml` — nav entry for ece-parity
- `docs/migrating-from-paas.md` — shipped importer instructions
- `docs/post-beta-roadmap.md` — importers no longer post-beta-only; dump/media deferred
- `docs/provenance.md` — Phase 4 clean-room import/patch rows
- `docs/knowledge/lessons/MageLift reference and clean-room policy.md` — Phase 4 clean-room update
- `scripts/check-clean-room.sh` — forbidden vendored tree gate
- `Makefile` — `check-clean-room` target + verify dependency

## Decisions Made

- Status vocabulary is hyphenated `intentional-gap` (assert gate) — not free prose “gap”.
- Closed rows limited to LifecyclePlan / PatchApplier / paasimport allowlist evidence from prior Phase 4 plans.
- Clean-room check matches path segments for sibling product directory names; docs/.planning citations allowlisted.

## Deviations from Plan

None - plan executed exactly as written.

### Rule 2 note (non-deviation)

Cross-link to `build.staticContent` required editing generated `docs/configuration.md` via `internal/config/schema.go` + `go run ./cmd/genconfig` so genconfig --check stays green.

## Auth Gates

None.

## Known Stubs

None — no TODO/FIXME stubs that block the plan goal. Encryption ARN “placeholder” wording in docs is intentional operator guidance (replace with real secret refs), not an incomplete feature stub.

## Threat Flags

None beyond plan threat model (T-04-12 docs honesty, T-04-13 clean-room script).

## Self-Check: PASSED

- FOUND: docs/ece-parity.md
- FOUND: scripts/check-clean-room.sh
- FOUND: 7bc4ca3
- FOUND: 0a5858b
- VERIFY: ece-parity assert rows=35 bad=0; check-clean-room exit 0
