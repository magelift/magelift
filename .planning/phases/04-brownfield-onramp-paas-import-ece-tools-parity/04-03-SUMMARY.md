---
phase: 04-brownfield-onramp-paas-import-ece-tools-parity
plan: 03
subsystem: cli
tags: [paas-import, acc, upsun, unmapped, allowlist, cron, soak]

requires:
  - phase: 04-brownfield-onramp-paas-import-ece-tools-parity
    provides: Init --from-acc/--from-upsun CLI + thin MapUpsun stub (04-02); SCD strategy/threads schema (04-04)
provides:
  - Shared ACC/Upsun mapper with D-07 allowlist
  - D-05 fail-loud sidecar (magelift.unmapped.md / stem.unmapped.md) + exit 2
  - Minimal application.cron schema for Magento cron:run
  - Soak tests gated on MAGELIFT_IMPORT_FIXTURE_ACC/UPSUN (skip when unset)
affects:
  - 04-06 ece-parity.md allowlist / intentional-gap docs

tech-stack:
  added: []
  patterns:
    - "Map → Result{YAML, Unmapped[]} → always write YAML; residuals → sidecar + invalid/exit 2"
    - "D-07 allowlist env only; relationships → capability catalog; crypt → secret-ref ARN placeholder"
    - "Magento cron:run → application.cron; free-form shell crons → unmapped"

key-files:
  created:
    - testdata/fixtures/acc/unmapped/
    - testdata/fixtures/upsun/supported/
    - testdata/fixtures/upsun/unmapped/
    - internal/paasimport/unmapped.go
    - internal/paasimport/allowlist.go
    - internal/paasimport/soak_test.go
  modified:
    - internal/paasimport/map.go
    - internal/paasimport/acc.go
    - internal/paasimport/upsun.go
    - internal/paasimport/source.go
    - internal/paasimport/map_test.go
    - internal/cli/root.go
    - internal/cli/init_import_test.go
    - internal/config/model.go
    - schema/magelift.schema.json
    - docs/configuration.md
    - testdata/fixtures/acc/supported/.magento.app.yaml

key-decisions:
  - "Checkpoint D-05: Ship YAML + magelift.unmapped.md + non-zero (d05-loud; locked via discuss --auto)"
  - "Sidecar adjacent to write path: stem.unmapped.md (config-out preserves basename stem)"
  - "application.cron list of {schedule, command} for Magento cron:run only"
  - "Upsun fixtures use php:8.5 to match Magento 2.4.9 catalog"

patterns-established:
  - "Pattern 2 (research): fail-loud unmapped report on disk, never exit 0 with warnings only"
  - "asStringMap normalizes named accAppRaw nested YAML maps from go.yaml.in/yaml/v4"

requirements-completed: [IMPORT-01, IMPORT-02, IMPORT-03, IMPORT-04, ECE-04]

coverage:
  - id: D1
    description: Supported ACC/Upsun fixtures produce Load-valid magelift.yaml with zero unmapped
    requirement: IMPORT-01
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/paasimport/ -count=1 -run 'Supported|Validate'"
        status: pass
    human_judgment: false
  - id: D2
    description: Unmapped fixtures write YAML + sidecar and exit non-zero
    requirement: IMPORT-03
    verification:
      - kind: integration
        ref: "internal/cli/init_import_test.go#TestInitFromAccUnmappedWritesSidecarAndExitsNonZero"
        status: pass
    human_judgment: false
  - id: D3
    description: D-07 allowlist maps crypt/routes/SCD/relationships; residuals reported
    requirement: ECE-04
    verification:
      - kind: unit
        ref: "internal/paasimport/map_test.go#TestAllowlistEnvKeys"
        status: pass
    human_judgment: false
  - id: D4
    description: Magento cron maps into application.cron; free-form shells stay unmapped
    requirement: IMPORT-03
    verification:
      - kind: unit
        ref: "internal/paasimport/soak_test.go#TestMapACCCronMapsMagentoOnly"
        status: pass
    human_judgment: false
  - id: D5
    description: Soak tests skip when MAGELIFT_IMPORT_FIXTURE_* unset
    requirement: IMPORT-04
    verification:
      - kind: unit
        ref: "internal/paasimport/soak_test.go#TestSoakACCFixtureWhenSet"
        status: pass
    human_judgment: false

duration: 6min
completed: 2026-07-29
status: complete
---

# Phase 4 Plan 03: Shared PaaS Mapper + Unmapped Sidecar Summary

**Shared ACC/Upsun mapper ships D-05 fail-loud sidecar + D-07 allowlist, with portable `application.cron` for Magento cron:run and soak skips when fixture env vars are unset.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-07-29T15:37:15Z
- **Completed:** 2026-07-29T15:43:03Z
- **Tasks:** 3/3 (1 auto-selected checkpoint + 2 TDD)
- **Files modified:** 22

## Accomplishments

- Locked D-05 as `d05-loud` (YAML + adjacent `*.unmapped.md` + exit 2); did not reopen discuss
- Replaced MapUpsun stub with real `.platform*` mapper sharing ACC structural walk
- Added `application.cron` schema/model + Magento cron mapping; free-form shells remain unmapped
- CI fixtures under `testdata/fixtures/{acc,upsun}/{supported,unmapped}/` (clean-room, config-only)

## Checkpoint Resolutions

| Gate | Selection | Rationale |
|------|-----------|-----------|
| D-05 unmapped contract | `d05-loud` — YAML + sidecar + non-zero | Locked CONTEXT / discuss --auto; honesty-first CI contract |

## Task Commits

1. **Task 1: Checkpoint D-05** — auto-selected `d05-loud` (no commit)
2. **Task 2: Shared mapper + allowlist + sidecar** — `82774af` (feat)
3. **Task 3: Cron schema + soak + validate** — `e710236` (feat)

**Plan metadata:** `1b952ac` (docs: complete plan)

## Files Created/Modified

- `internal/paasimport/{unmapped,allowlist,soak_test}.go` — Result/sidecar/report, D-07 table, soak gates
- `internal/paasimport/{acc,upsun,map,source}.go` — shared map + DetectUpsun + ConfineConfigRoot
- `internal/cli/root.go` — write YAML; on residuals write sidecar + `invalid`
- `internal/config/model.go` + generated schema/docs — `application.cron`
- `testdata/fixtures/{acc,upsun}/…` — supported + unmapped trees

## Decisions Made

- Sidecar path: `{stem}.unmapped.md` beside the written YAML (including `--config-out`)
- Magento cron destination: `application.cron: [{schedule, command}]`
- Upsun PHP type `8.5` to satisfy Magento 2.4.9 catalog (same as ACC)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Nested YAML maps typed as named `accAppRaw`**
- **Found during:** Task 1 (mapper)
- **Issue:** `go.yaml.in/yaml/v4` decoded nested maps as `paasimport.accAppRaw`, so `map[string]any` assertions failed and hooks/crons collapsed to whole-key residuals
- **Fix:** `asStringMap` accepts `accAppRaw` / `map[string]any` / `map[any]any`
- **Files modified:** `internal/paasimport/acc.go`
- **Commit:** `82774af`

**2. [Rule 3 - Blocking] Upsun php:8.3 failed ResolveBuild catalog**
- **Found during:** Task 1
- **Issue:** Magento 2.4.9 catalog requires PHP 8.5
- **Fix:** Upsun fixtures use `php:8.5`
- **Files modified:** `testdata/fixtures/upsun/*/…`
- **Commit:** `82774af`

## TDD Gate Compliance

Tasks marked `tdd=true`; RED/GREEN were developed iteratively in-session and landed as single feat commits per task (tests included in each feat). No separate `test(...)` RED commits — noted for audit.

## Self-Check: PASSED

- Created files present (unmapped.go, allowlist.go, soak_test.go, fixtures, SUMMARY)
- Commits `82774af` and `e710236` present on branch
