---
phase: 04-brownfield-onramp-paas-import-ece-tools-parity
plan: 05
subsystem: build
tags: [patches, m2-hotfixes, ece-tools, php, lifecycle, clean-room]

requires:
  - phase: 04-04
    provides: LifecyclePlan build-phase command sequence after composer install
provides:
  - Clean-room m2-hotfixes PatchApplier (alpha order, patch -p1)
  - ECE-02 closed; QUALITY_PATCHES intentional-gap stub in ece-parity.md
affects:
  - 04-06 (full ece-parity matrix including QUALITY_PATCHES row)

tech-stack:
  added: []
  patterns:
    - "Host patch(1) via Executable::Patch after composer install; no Adobe patch DB"
    - "Discover m2-hotfixes at prepare time under build root; empty/missing dir = no-op"

key-files:
  created:
    - build/src/Magento/PatchApplier.php
    - build/tests/Magento/PatchApplierTest.php
    - docs/ece-parity.md
  modified:
    - build/src/Magento/Executable.php
    - build/src/Magento/LifecyclePlan.php
    - build/src/Runner/NativePreparation.php
    - build/tests/Magento/LifecyclePlanTest.php
    - build/tests/Runner/NativePreparationTest.php

key-decisions:
  - "ECE-02 closes m2-hotfixes only; QUALITY_PATCHES remains intentional gap (no Adobe DB)"
  - "Discover patches under copied build root at prepare time; emit patch -p1 --forward --batch -i"
  - "Reject symlink hotfixes and path traversal outside m2-hotfixes/"

patterns-established:
  - "Clean-room patch apply: public Adobe docs behavior + host patch(1); never vendor ece-tools/magento-cloud-patches"

requirements-completed: [ECE-02]

coverage:
  - id: D1
    description: "m2-hotfixes/*.patch apply after composer install in alpha order via clean-room PatchApplier"
    requirement: ECE-02
    verification:
      - kind: unit
        ref: "make php-test (PatchApplierTest, LifecyclePlanTest::testPlansHotfixPatchesAfterComposerInstall, NativePreparationTest::testExecutesHotfixPatchesAfterComposerInstall)"
        status: pass
    human_judgment: false
  - id: D2
    description: "QUALITY_PATCHES explicitly not claimed — intentional gap stub in docs/ece-parity.md"
    requirement: ECE-02
    verification:
      - kind: other
        ref: "test -f docs/ece-parity.md && grep -E 'm2-hotfixes|QUALITY_PATCHES' docs/ece-parity.md"
        status: pass
    human_judgment: false

duration: 3min
completed: 2026-07-29
status: complete
---

# Phase 4 Plan 05: m2-hotfixes PatchApplier Summary

**Clean-room PatchApplier applies project `m2-hotfixes/*.patch` in alphabetical order after composer install; QUALITY_PATCHES stays an intentional gap.**

## Performance

- **Duration:** 3 min
- **Started:** 2026-07-29T15:32:52Z
- **Completed:** 2026-07-29T15:36:03Z
- **Tasks:** 2/2
- **Files modified:** 8

## Accomplishments

- Shipped `PatchApplier` discovering `m2-hotfixes/*.patch`, sorting alphabetically, applying via host `patch -p1`
- Wired apply into LifecyclePlan build phase (after composer install, before DI/SCD) via NativePreparation discovery
- Documented ECE-02 closed vs QUALITY_PATCHES intentional gap in `docs/ece-parity.md` stub

## Task Commits

1. **Task 1: End-to-end m2-hotfixes apply after composer in PHP build** - `ed4802b` (feat)
2. **Task 2: Record ECE-02 closed vs QUALITY_PATCHES gap stub for docs plan** - `508ff9d` (docs)

**Plan metadata:** `d70ac03` (docs: complete plan)

## Files Created/Modified

- `build/src/Magento/PatchApplier.php` — discover/commands/apply; path confinement; missing `patch` tool error
- `build/src/Magento/Executable.php` — `Patch = 'patch'`
- `build/src/Magento/LifecyclePlan.php` — hotfix commands after composer install
- `build/src/Runner/NativePreparation.php` — discover hotfixes on build root; require patch tool when present
- `build/tests/Magento/PatchApplierTest.php` — success, corrupt fail, alpha order, symlink reject
- `build/tests/Magento/LifecyclePlanTest.php` — argv order for patches
- `build/tests/Runner/NativePreparationTest.php` — prepare emits patches between composer and compile
- `docs/ece-parity.md` — patch rows stub for 04-06 expansion

## Decisions Made

- Close ECE-02 on m2-hotfixes only; QUALITY_PATCHES / Adobe QPT DB = intentional gap (research Open Q3)
- Use host `patch -p1 --forward --batch -i` (Assumption A1); actionable error if binary missing when patches exist
- Empty or missing `m2-hotfixes/` is a successful no-op

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Corrected STATE after `state.advance-plan` miscounted phase plans**
- **Found during:** State updates after Task 2
- **Issue:** SDK treated phase as last-plan / ready_for_verification (4/4) while ROADMAP still has 04-03 and 04-06 open
- **Fix:** Restored STATE to executing with remaining 04-03 / 04-06
- **Files modified:** `.planning/STATE.md`
- **Commit:** (plan metadata commit)

## Threat Mitigations

- **T-04-10:** Apply only under project hotfix dir; fail on nonzero; no remote patch DB fetch
- **T-04-11:** Resolve under project root; reject `..`, symlinks, and paths escaping `m2-hotfixes/`

## Known Stubs

None that block ECE-02. QUALITY_PATCHES is an intentional gap documented for 04-06 (not a silent success claim).

## Self-Check: PASSED

- FOUND: `build/src/Magento/PatchApplier.php`
- FOUND: `docs/ece-parity.md`
- FOUND: `04-05-SUMMARY.md`
- FOUND: commits `ed4802b`, `508ff9d`
- VERIFIED: `make php-test` OK (110 tests)
