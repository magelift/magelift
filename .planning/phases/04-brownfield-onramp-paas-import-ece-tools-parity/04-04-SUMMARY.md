---
phase: 04-brownfield-onramp-paas-import-ece-tools-parity
plan: 04
subsystem: build
tags: [scd, static-content, ece-tools, protocol, php, lifecycle]

requires:
  - phase: foundation-build-runner
    provides: locale×theme StaticContent prepare protocol and LifecyclePlan SCD emission
provides:
  - build.staticContent strategy/threads end-to-end into setup:static-content:deploy -s/-j
  - Typed StaticContentSettings schema keys (locales, themes, strategy, threads)
affects:
  - 04-03 (SCD_* allowlist mapping into build.staticContent)
  - 04-06 (ece-parity.md ECE-03 row)

tech-stack:
  added: []
  patterns:
    - "SCD settings flow magelift.yaml → plan → PrepareRequest.StaticContent → LifecyclePlan argv"
    - "Typed StaticContentSettings replaces opaque map for schema honesty"

key-files:
  created: []
  modified:
    - internal/build/runner/protocol.go
    - internal/build/plan/plan.go
    - build/src/Magento/LifecyclePlan.php
    - build/src/Protocol/PrepareRequest.php
    - internal/config/model.go
    - schema/magelift.schema.json

key-decisions:
  - "Strategy/threads applied per locale×theme matrix entry (same values on each pair)"
  - "Typed StaticContentSettings with additionalProperties:false for schema pin"
  - "threads int uses 0 as unset (omitempty); reject negative; schema minimum=1 when present"

patterns-established:
  - "Pattern 3 (research): optional strategy enum quick|standard|compact and threads ≥1 → -s/-j"

requirements-completed: [ECE-03]

coverage:
  - id: D1
    description: "strategy/threads flow Go plan → runner protocol → PHP LifecyclePlan into SCD argv as -s/-j alongside locale×theme"
    requirement: ECE-03
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/build/plan/ ./internal/build/runner/ -count=1 -run 'Static|Strategy|Thread'"
        status: pass
      - kind: unit
        ref: "make php-test (LifecyclePlanTest::testPlansConfiguredStaticContentStrategyAndThreads, NativePreparationTest::testExecutesConfiguredStaticContentStrategyAndThreads)"
        status: pass
    human_judgment: false
  - id: D2
    description: "Schema documents build.staticContent locales/themes/strategy/threads; locale/theme-only configs still validate"
    requirement: ECE-03
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/config/ ./internal/build/plan/ -count=1 -run 'Static|Schema|Build|Strategy|Thread|Resolve|Prepare'"
        status: pass
    human_judgment: false

duration: 5min
completed: 2026-07-29
status: complete
---

# Phase 4 Plan 04: SCD strategy/threads Go→PHP Summary

**build.staticContent strategy/threads now reach Magento SCD argv as -s/-j through the prepare protocol, with schema keys pinned.**

## Performance

- **Duration:** 5 min
- **Started:** 2026-07-29T15:23:16Z
- **Completed:** 2026-07-29T15:27:56Z
- **Tasks:** 2/2
- **Files modified:** 14

## Accomplishments

- Wired optional `strategy` / `threads` on each StaticContent protocol entry and emitted `-s` / `-j` from `LifecyclePlan`
- Plan reads typed `build.staticContent` settings onto the locale×theme cartesian product with enum/positive-int validation
- Regenerated schema/reference so `locales`, `themes`, `strategy`, `threads` are explicit (`additionalProperties: false`)

## Task Commits

1. **Task 1: End-to-end SCD strategy/threads Go→PHP argv** - `967d879` (feat)
2. **Task 2: Pin build.staticContent schema keys for strategy/threads** - `561b257` (feat)

**Plan metadata:** `93529e2` (docs: complete plan)

## Files Created/Modified

- `internal/build/runner/protocol.go` — StaticContent Strategy/Threads + validate
- `internal/build/plan/plan.go` — read strategy/threads into matrix entries
- `internal/build/plan/plan_test.go` — strategy/threads + rejection cases
- `internal/build/runner/codec_test.go` — encode includes strategy/threads
- `build/src/Magento/LifecyclePlan.php` — append `-s`/`-j` when set
- `build/src/Protocol/PrepareRequest.php` — parse/canonicalize optional fields
- `build/tests/Magento/LifecyclePlanTest.php` — argv assertions for -s/-j
- `build/tests/Runner/NativePreparationTest.php` — prepare path with strategy/threads
- `internal/config/model.go` — `StaticContentSettings` typed struct
- `internal/config/schema.go` — `minimum` schema rule support
- `internal/config/config_test.go` — struct field access after resolve
- `schema/magelift.schema.json` / `docs/configuration.md` — generated pin
- `internal/paasimport/map.go` — emit SCD_THREADS as integer YAML

## Decisions Made

- Apply the same strategy/threads to every locale×theme pair (D-06 / Pattern 3)
- Replace `map[string]any` with `StaticContentSettings` so genconfig can document keys (opaque map cannot)
- Keep `threads: 0` as unset via omitempty; invalid explicit values use negative (schema rejects `<1` when present)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Correctness] Typed StaticContentSettings required plan + importer updates**
- **Found during:** Task 2
- **Issue:** Pinning schema keys needs a typed model; map-based plan readers and string `SCD_THREADS` YAML would break Load/PrepareRequest
- **Fix:** Updated `plan.staticContent` for the struct; parse importer threads with `strconv.Atoi`
- **Files modified:** `internal/build/plan/plan.go`, `internal/paasimport/map.go`, `internal/config/config_test.go`
- **Verification:** focused Go tests green
- **Committed in:** `561b257`

**2. [Rule 3 - Blocking] Composer vendor missing for make php-test**
- **Found during:** Task 1 verify
- **Issue:** `phpstan` / vendor absent in `build/`
- **Fix:** `composer install --working-dir=build`
- **Files modified:** none (local vendor; not committed)
- **Verification:** `make php-test` OK (100 tests)

## Threat Flags

None — strategy/threads validation covers T-04-09; no new network/auth surface.

## Known Stubs

None.

## Self-Check: PASSED

- FOUND: `internal/build/runner/protocol.go` Strategy/Threads fields
- FOUND: `build/src/Magento/LifecyclePlan.php` -s/-j emission
- FOUND: `schema/magelift.schema.json` staticContentSettings with strategy enum + threads minimum
- FOUND: commit `967d879`
- FOUND: commit `561b257`
