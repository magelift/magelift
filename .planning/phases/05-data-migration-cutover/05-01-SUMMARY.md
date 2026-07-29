---
phase: 05-data-migration-cutover
plan: 01
subsystem: config
tags: [seedDump, KnownFields, journal, migration, ADR-0010]

requires:
  - phase: 04-import-ece-parity
    provides: releasejournal exclusive-lock + atomic rename pattern under .magelift/
provides:
  - SeedDump on config.Config and config.Environment (KnownFields + schema)
  - internal/seeddump InitRecorded → .magelift/seed-dumps/<env>.json status=recorded
  - env create --dump journal-backed seedDumpStatus=recorded (no ADR placeholder prose)
affects: [05-02-status-merge, 05-03-dumpimport, 05-04-auto-import]

tech-stack:
  added: []
  patterns:
    - "Status state machine in single-document JSON under .magelift/seed-dumps (not YAML)"
    - "Overlay intent field SeedDump mirrored on Config + Environment like Account/Class"

key-files:
  created:
    - internal/seeddump/doc.go
    - internal/seeddump/store.go
    - internal/seeddump/store_test.go
    - internal/cli/env_seeddump_test.go
  modified:
    - internal/config/model.go
    - internal/config/config_test.go
    - internal/cli/env.go
    - schema/magelift.schema.json
    - docs/configuration.md

key-decisions:
  - "Journal path is .magelift/seed-dumps/<env>.json (single JSON document, not jsonl)"
  - "create output seedDumpStatus is exactly journal StatusRecorded string"
  - "Generated docs live at docs/configuration.md + schema/magelift.schema.json (repo paths)"

patterns-established:
  - "seeddump.InitRecorded: exclusive lock + atomic rename + 0600, mirrored from releasejournal"
  - "seedDumpStatus must never be a YAML Environment field (KnownFields rejection)"

requirements-completed: [MIGRATE-01, MIGRATE-02]

coverage:
  - id: D1
    description: "create --dump persists SeedDump through Load/Resolve"
    requirement: MIGRATE-01
    verification:
      - kind: unit
        ref: "internal/cli/env_seeddump_test.go#TestEnvCreateDumpPersistsSeedDumpAndJournalRecorded"
        status: pass
    human_judgment: false
  - id: D2
    description: "InitRecorded writes journal status=recorded; create output uses journal string"
    requirement: MIGRATE-02
    verification:
      - kind: unit
        ref: "internal/seeddump/store_test.go#TestInitRecordedRoundTrip"
        status: pass
      - kind: unit
        ref: "internal/cli/env_seeddump_test.go#TestEnvCreateDumpPersistsSeedDumpAndJournalRecorded"
        status: pass
    human_judgment: false
  - id: D3
    description: "seedDumpStatus YAML key rejected; SeedDump path Loads"
    requirement: MIGRATE-02
    verification:
      - kind: unit
        ref: "internal/config/config_test.go#TestSeedDumpPathLoadsAndResolves"
        status: pass
      - kind: unit
        ref: "internal/config/config_test.go#TestSeedDumpStatusYAMLFieldRejected"
        status: pass
    human_judgment: false
  - id: D4
    description: "make generate-check green after SeedDump schema land"
    requirement: MIGRATE-01
    verification:
      - kind: other
        ref: "make generate-check"
        status: pass
    human_judgment: false

duration: 3min
completed: 2026-07-29
status: complete
---

# Phase 5 Plan 01: SeedDump KnownFields + journal recorded Summary

**`env create --dump` now survives KnownFields, writes `.magelift/seed-dumps/<env>.json` with `status=recorded`, and reports that journal status instead of ADR placeholder prose.**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-07-29T16:01:22Z
- **Completed:** 2026-07-29T16:03:52Z
- **Tasks:** 2/2
- **Files modified:** 9

## Accomplishments

- Added `SeedDump` to `config.Config` and `config.Environment`; regenerated schema + configuration reference
- Shipped `internal/seeddump` with `InitRecorded` (lock + atomic rename + 0600)
- Wired `env create --dump` to init journal and emit `seedDumpStatus: recorded` only
- Honesty tests: SeedDump Loads; `seedDumpStatus` YAML key rejected

## Task Commits

1. **Task 1: End-to-end create --dump → SeedDump Load → journal recorded** - `cd3fca0` (feat)
2. **Task 2: Reject unknown seedDumpStatus YAML field honesty** - `b3efd46` (test)

**Plan metadata:** (pending docs commit)

## Files Created/Modified

- `internal/config/model.go` — `SeedDump` on Config + Environment
- `schema/magelift.schema.json` / `docs/configuration.md` — generated KnownFields surface
- `internal/seeddump/*` — journal InitRecorded + tests
- `internal/cli/env.go` — create path journal wiring
- `internal/cli/env_seeddump_test.go` — create → Load → Resolve → journal
- `internal/config/config_test.go` — SeedDump accept / seedDumpStatus reject

## Decisions Made

- Journal layout: `.magelift/seed-dumps/<env>.json` single document (state machine, not releasejournal jsonl append)
- Create JSON `seedDumpStatus` equals `seeddump.StatusRecorded` (`"recorded"`), never ADR prose
- Followed repo generate outputs (`docs/configuration.md`, `schema/magelift.schema.json`) rather than plan's outdated paths

## Deviations from Plan

### Auto-fixed Issues

None - plan executed as written for behavior.

### Path correction (non-blocking)

**1. [Rule 3 - Blocking issue] Generated artifact paths differ from PLAN.md file list**
- **Found during:** Task 1
- **Issue:** PLAN listed `docs/configuration-reference.md` and `internal/config/schema/magelift.schema.json`
- **Fix:** Committed actual generate outputs `docs/configuration.md` and `schema/magelift.schema.json`
- **Files modified:** schema + docs as above
- **Committed in:** `cd3fca0`

### TDD note

Task 2 `tdd="true"` honesty tests were written after the tracer already shipped SeedDump/KnownFields behavior; RED would have passed immediately. Committed as `test(05-01)` documenting the D-03 contract (no separate GREEN feat needed).

## Threat Flags

None — mitigations T-05-01..03 applied (journal-only status, no dump bytes in journal, lock+atomic rename+0600).

## Self-Check: PASSED

- All key files FOUND
- Commits `cd3fca0`, `b3efd46` FOUND
- No Known Stubs blocking plan goal
