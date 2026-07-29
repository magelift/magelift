---
phase: 05-data-migration-cutover
plan: 03
subsystem: database
tags: [dumpimport, mysql, seedDump, D-04, MIGRATE-05, schema-replace]

requires:
  - phase: 05-data-migration-cutover
    provides: seeddump journal Mark* + SeedDump config (05-01/05-02)
provides:
  - internal/dumpimport Import/NonEmpty with Options.Yes schema-replace (D-04)
  - Synthetic testdata/fixtures/migrate/*.sql(.gz) offline proof fixtures
affects: [05-04-auto-import, env-import-dump]

tech-stack:
  added: []
  patterns:
    - "Host mysql preferred; else docker compose exec -T database mysql pipe"
    - "Non-empty = ≥1 information_schema BASE TABLE; overwrite only via Options.Yes (no --force)"
    - "Schema-replace = DROP DATABASE + CREATE DATABASE then import (converge)"

key-files:
  created:
    - internal/dumpimport/doc.go
    - internal/dumpimport/import.go
    - internal/dumpimport/nonempty.go
    - internal/dumpimport/import_test.go
    - testdata/fixtures/migrate/tiny.sql
    - testdata/fixtures/migrate/tiny.sql.gz
    - testdata/fixtures/migrate/corrupt.sql
  modified: []

key-decisions:
  - "D-04 locked option-a: persistent --yes + schema-replace; do not invent --force"
  - "Non-empty = ≥1 BASE TABLE in target schema via information_schema"
  - "Importer uses root/root localdev defaults for DROP/CREATE privilege"
  - "Journal Mark* remains caller responsibility (05-04); dumpimport returns clear errors for MarkFailed"

patterns-established:
  - "dumpimport.Options.Yes binds to root PersistentFlags --yes in CLI (not package cobra)"
  - "Gunzip via gunzip -c pipe into mysql stdin (no Go SQL parser)"

requirements-completed: [MIGRATE-05]

coverage:
  - id: D1
    description: "Import tiny.sql into empty schema creates queryable tables"
    requirement: MIGRATE-01
    verification:
      - kind: integration
        ref: "internal/dumpimport/import_test.go#TestImportCreatesTablesFromTinySQL"
        status: pass
    human_judgment: false
  - id: D2
    description: "Import tiny.sql.gz via gunzip pipe"
    requirement: MIGRATE-01
    verification:
      - kind: integration
        ref: "internal/dumpimport/import_test.go#TestImportTinySQLGz"
        status: pass
    human_judgment: false
  - id: D3
    description: "Non-empty target refuses import without Yes (ErrNonEmptyRequiresYes)"
    requirement: MIGRATE-05
    verification:
      - kind: integration
        ref: "internal/dumpimport/import_test.go#TestNonEmptyWithoutYesRefuses"
        status: pass
    human_judgment: false
  - id: D4
    description: "Yes schema-replace converges on second import"
    requirement: MIGRATE-05
    verification:
      - kind: integration
        ref: "internal/dumpimport/import_test.go#TestYesSchemaReplaceConverges"
        status: pass
    human_judgment: false
  - id: D5
    description: "corrupt.sql surfaces clear mysql import error for journal failed mapping"
    requirement: MIGRATE-01
    verification:
      - kind: integration
        ref: "internal/dumpimport/import_test.go#TestCorruptSQLSurfacesClearError"
        status: pass
    human_judgment: false

duration: 5min
completed: 2026-07-29
status: complete
---

# Phase 5 Plan 03: Dumpimport Local MySQL Summary

**Offline dumpimporter with D-04 `--yes` schema-replace: nonempty refuse, converge on retry, synthetic fixtures proven against ephemeral compose MySQL.**

## Performance

- **Duration:** 5 min
- **Started:** 2026-07-29T16:08:53Z
- **Completed:** 2026-07-29T16:13:19Z
- **Tasks:** 2 (1 decision auto-locked + 1 TDD)
- **Files modified:** 7

## Accomplishments

- Locked D-04 to persistent `--yes` + full schema-replace (option-a); no `--force` / `--confirm-overwrite`
- Shipped `internal/dumpimport` with host `mysql` or `docker compose exec -T database mysql` pipe (`.sql` / `.sql.gz`)
- SC3 asserted: refuse nonempty without Yes; Yes DROP/CREATE then import converges; corrupt fails loudly for journal mapping

## Task Commits

1. **Task 1: Lock D-04 overwrite confirmation contract** - (decision, no commit) ⚡ Auto-selected option-a
2. **Task 2: dumpimport local MySQL + nonempty/--yes converge** - `2e6b979` (test RED) → `65b1082` (feat GREEN)

**Plan metadata:** `7a2a047` (docs: complete dumpimport plan)

## Files Created/Modified

- `internal/dumpimport/doc.go` — package contract (D-04, no Go SQL parser)
- `internal/dumpimport/import.go` — Import, runner resolution, gunzip pipe
- `internal/dumpimport/nonempty.go` — NonEmpty probe + schemaReplace
- `internal/dumpimport/import_test.go` — ephemeral compose MySQL integration tests
- `testdata/fixtures/migrate/tiny.sql` — synthetic seed tables (no PII)
- `testdata/fixtures/migrate/tiny.sql.gz` — gzip twin
- `testdata/fixtures/migrate/corrupt.sql` — invalid SQL error path

## Decisions Made

- **D-04 option-a (CONTEXT lock):** nonempty DB → nonzero unless `Options.Yes`; with Yes → DROP DATABASE + CREATE DATABASE then import. Do not invent `--force`.
- Non-empty definition: `information_schema.tables` count of BASE TABLE ≥ 1 for target schema.
- Journal transitions (`MarkImporting` / `MarkImported` / `MarkFailed`) stay in CLI auto-hook / `env import-dump` (05-04); this package only returns errors suitable for `MarkFailed` reasons.

## Deviations from Plan

None - plan executed exactly as written (yolo/auto locked option-a per CONTEXT).

## Issues Encountered

None. Docker available; host `mysql` absent — compose `exec -T` path exercised. Tests skip cleanly if Docker/MySQL unavailable or `-short`.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `dumpimport.Import` ready for 05-04 CLI `env import-dump` + post-deploy auto-hook
- Wire `o.yes` → `Options.Yes`; wrap `ErrNonEmptyRequiresYes` with `invalid()`
- Call seeddump `MarkImporting` → Import → `MarkImported` / `MarkFailed`
- MIGRATE-01 remains pending until auto-hook lands (05-04); MIGRATE-05 complete

## Self-Check: PASSED

- FOUND: all 7 artifact files
- FOUND: commits `2e6b979`, `65b1082`
- Verify: `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/dumpimport/ -count=1 -run 'NonEmpty|Yes|Converge|Import|Corrupt'` → 5 passed

---
*Phase: 05-data-migration-cutover*
*Completed: 2026-07-29*
