---
phase: 04-brownfield-onramp-paas-import-ece-tools-parity
plan: 01
subsystem: cli
tags: [paas-import, acc, init, config-load, fixtures]

requires:
  - phase: 03-credit-efficient-acceptance-harness-evidence-tiering
    provides: Offline acceptance harness patterns and serial GOMAXPROCS discipline
provides:
  - Synthetic ACC fixture under testdata/fixtures/acc/supported/
  - internal/paasimport MapACC happy-path mapper
  - magelift init --from-acc writing Load-valid magelift.yaml
  - config.Load foreign-schema rejection for PaaS YAML as --config
affects:
  - 04-02 init CLI flags/--yes/--config-out
  - 04-03 unmapped sidecar + Upsun mapper

tech-stack:
  added: []
  patterns:
    - "paasimport pure library (no cobra); CLI flags hang off existing init"
    - "Crypt maps to encryptionKeySecretArn placeholder only — never plaintext"
    - "config.Load rejects missing schemaVersion: 1 before KnownFields decode"

key-files:
  created:
    - testdata/fixtures/acc/supported/.magento.app.yaml
    - testdata/fixtures/acc/supported/.magento/services.yaml
    - testdata/fixtures/acc/supported/.magento/routes.yaml
    - testdata/fixtures/acc/supported/.magento.env.yaml
    - internal/paasimport/doc.go
    - internal/paasimport/source.go
    - internal/paasimport/map.go
    - internal/paasimport/acc.go
    - internal/paasimport/map_test.go
    - internal/cli/init_import_test.go
    - internal/cli/foreign_config_test.go
  modified:
    - internal/cli/root.go
    - internal/config/config.go

key-decisions:
  - "Fixture uses php:8.5 to match Magento 2.4.9 compatibility catalog (not 8.3 from ACC docs examples)"
  - "CRYPT_KEY presence → target.aws.encryptionKeySecretArn ARN placeholder; value discarded"
  - "Foreign guard is schemaVersion probe in config.Load (clear PaaS guidance) rather than filename sniffing alone"

patterns-established:
  - "Pattern: init --from-acc uses cwd as ACC config root; tests copy fixture + Chdir"
  - "Pattern: IMPORT-05 fail-loud via missing schemaVersion: 1 before deploy/validate"

requirements-completed: [IMPORT-01, IMPORT-04, IMPORT-05, IMPORT-06]

coverage:
  - id: D1
    description: Operator can init --from-acc against supported ACC fixture and get Load-valid magelift.yaml
    requirement: IMPORT-01
    verification:
      - kind: integration
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/paasimport/ ./internal/cli/ -count=1 -run 'Acc|FromAcc|MapACC'"
        status: pass
    human_judgment: false
  - id: D2
    description: Generated YAML passes config.Load and config validate path
    requirement: IMPORT-04
    verification:
      - kind: integration
        ref: "internal/cli/init_import_test.go#TestInitFromAccWritesLoadValidConfig"
        status: pass
    human_judgment: false
  - id: D3
    description: Foreign .magento.app.yaml / .platform.app.yaml rejected as --config
    requirement: IMPORT-05
    verification:
      - kind: unit
        ref: "GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ ./internal/config/ -count=1 -run 'Foreign|MagentoApp|PlatformApp|schemaVersion'"
        status: pass
    human_judgment: false
  - id: D4
    description: CI fixtures are synthetic config-only; no sibling ece-tools/ACC trees vendored
    requirement: IMPORT-06
    verification:
      - kind: other
        ref: "testdata/fixtures/acc/supported/ (authored synthetic YAML only)"
        status: pass
    human_judgment: false

duration: 3min
completed: 2026-07-29
status: complete
---

# Phase 4 Plan 01: ACC init tracer + foreign --config rejection Summary

**Wave 1 tracer: synthetic ACC fixture → `magelift init --from-acc` → Load-valid `magelift.yaml`, plus loud rejection of PaaS YAML as `--config`.**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-07-29T15:18:50Z
- **Completed:** 2026-07-29T15:22:01Z
- **Tasks:** 2/2
- **Files modified:** 13

## Accomplishments

- Shipped config-only `testdata/fixtures/acc/supported/` (app/services/routes/env) with D-07 allowlist SCD + crypt
- Added `internal/paasimport` with `MapACC` producing schemaVersion-1 MageLift YAML (no plaintext crypt)
- Wired `init --from-acc` on existing init (refuse-if-exists exit 2 unchanged)
- Guarded `config.Load` so missing `schemaVersion: 1` fails with explicit PaaS/`--from-acc` guidance

## Task Commits

1. **Task 1: End-to-end ACC init --from-acc → Load-valid magelift.yaml** - `ddf1408` (feat)
2. **Task 2: Reject foreign PaaS YAML as --config** (TDD)
   - RED - `afdd3f2` (test)
   - GREEN - `4f13244` (feat)

**Plan metadata:** (pending final docs commit)

## Files Created/Modified

- `testdata/fixtures/acc/supported/*` - Synthetic ACC config tree for CI
- `internal/paasimport/*` - ACC detect/read/map library
- `internal/cli/root.go` - `--from-acc` on init
- `internal/cli/init_import_test.go` - init + validate integration
- `internal/cli/foreign_config_test.go` - IMPORT-05 tests
- `internal/config/config.go` - schemaVersion foreign-schema guard

## Decisions Made

- Use `php:8.5` in the supported fixture so Resolve/validate matches Magento 2.4.9 catalog (ACC doc examples often show 8.3)
- Crypt presence maps only to `encryptionKeySecretArn` placeholder ARN — fixture plaintext never written
- Prefer Load-level schemaVersion probe over opaque KnownFields errors for IMPORT-05 UX

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixture PHP 8.3 failed Magento 2.4.9 compatibility resolve**
- **Found during:** Task 1 (tracer verify)
- **Issue:** `ResolveBuild` rejected mapped PHP 8.3 against default Magento 2.4.9 catalog line
- **Fix:** Set fixture `type: php:8.5` to match starter/catalog; tests assert `php: "8.5"`
- **Files modified:** `testdata/fixtures/acc/supported/.magento.app.yaml`, `internal/paasimport/map_test.go`
- **Verification:** Acc/FromAcc/MapACC tests green
- **Committed in:** `ddf1408`

**Total deviations:** 1 auto-fixed (Rule 1)
**Impact on plan:** Necessary for IMPORT-04 validate path; no scope creep.

## Issues Encountered

None beyond the PHP/catalog mismatch above.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

Ready for 04-02 (`--from-upsun`, mutual exclusion, `--yes`, `--config-out`) and later 04-03 unmapped sidecar. Do not start those in this plan.

## Self-Check: PASSED

- FOUND: testdata/fixtures/acc/supported/.magento.app.yaml
- FOUND: internal/paasimport/acc.go
- FOUND: internal/cli/init_import_test.go
- FOUND: internal/cli/foreign_config_test.go
- FOUND: commits ddf1408, afdd3f2, 4f13244

---
*Phase: 04-brownfield-onramp-paas-import-ece-tools-parity*
*Completed: 2026-07-29*
