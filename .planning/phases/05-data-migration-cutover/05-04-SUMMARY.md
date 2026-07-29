---
phase: 05-data-migration-cutover
plan: 04
subsystem: cli
tags: [seedDump, import-dump, auto-import, deployflow, D-01, D-02, MIGRATE-01, MIGRATE-02]

requires:
  - phase: 05-data-migration-cutover
    provides: seeddump journal Mark* + SeedDump config (05-01/05-02)
  - phase: 05-data-migration-cutover
    provides: dumpimport Import/NonEmpty with Options.Yes (05-03)
provides:
  - magelift env import-dump CLI with journal transitions
  - maybeAutoImportSeedDump once-from-recorded after deployflow.Run
affects: [05-05-media-sync, 05-06-cutover-runbook]

tech-stack:
  added: []
  patterns:
    - "Hybrid D-01: auto-import once from recorded after deploy + env import-dump for retries"
    - "Injectable options.importSeedDump seam (same style as newDeploySteps fakes)"
    - "First recorded path forces dumpimport Yes=true; failed retries bind to persistent --yes"

key-files:
  created:
    - internal/cli/env_import_dump_test.go
    - internal/cli/lifecycle_seeddump_test.go
  modified:
    - internal/cli/env.go
    - internal/cli/root.go
    - internal/cli/lifecycle.go

key-decisions:
  - "D-01 locked option-a (AUTO): hybrid post-deployflow once-from-recorded + env import-dump; never env seed; never Pulumi create"
  - "Auto-import runs after deployflow.Run returns (lock already released)"
  - "Auto-import failure marks journal failed+reason and makes deploy CLI exit nonzero"
  - "imported→importing remains rejected by seeddump; retries are failed→importing with --yes when nonempty"

patterns-established:
  - "runSeedDumpImport shared by CLI and lifecycle with seedDumpImportExplicit|Auto kinds"
  - "Command Use import-dump only — no env seed subcommand"

requirements-completed: [MIGRATE-01, MIGRATE-02, MIGRATE-05]

coverage:
  - id: D1
    description: "env import-dump imports with journal importing→imported and forces Yes on recorded first path"
    requirement: MIGRATE-01
    verification:
      - kind: unit
        ref: "internal/cli/env_import_dump_test.go#TestEnvImportDumpSuccessMovesJournalImported"
        status: pass
    human_judgment: false
  - id: D2
    description: "Importer error marks failed+reason and exits nonzero"
    requirement: MIGRATE-01
    verification:
      - kind: unit
        ref: "internal/cli/env_import_dump_test.go#TestEnvImportDumpFailureMarksFailed"
        status: pass
    human_judgment: false
  - id: D3
    description: "Failed retry into nonempty DB requires persistent --yes (invalid exit 2)"
    requirement: MIGRATE-05
    verification:
      - kind: unit
        ref: "internal/cli/env_import_dump_test.go#TestEnvImportDumpNonEmptyRetryRequiresYes"
        status: pass
    human_judgment: false
  - id: D4
    description: "Command named import-dump; no env seed subcommand"
    requirement: MIGRATE-01
    verification:
      - kind: unit
        ref: "internal/cli/env_import_dump_test.go#TestEnvImportDumpCommandNamedImportDumpNotSeed"
        status: pass
    human_judgment: false
  - id: D5
    description: "Auto-import fires once from recorded after deploy; second deploy no-ops; after lock.release"
    requirement: MIGRATE-02
    verification:
      - kind: unit
        ref: "internal/cli/lifecycle_seeddump_test.go#TestAutoImportOnceFromRecorded"
        status: pass
    human_judgment: false
  - id: D6
    description: "Status imported skips auto-import (RecordedOnly)"
    requirement: MIGRATE-02
    verification:
      - kind: unit
        ref: "internal/cli/lifecycle_seeddump_test.go#TestAutoImportRecordedOnly"
        status: pass
    human_judgment: false
  - id: D7
    description: "Auto-import failure fails deploy CLI and journals failed"
    requirement: MIGRATE-01
    verification:
      - kind: unit
        ref: "internal/cli/lifecycle_seeddump_test.go#TestAutoImportFailureFailsDeploy"
        status: pass
    human_judgment: false

duration: 4min
completed: 2026-07-29
status: complete
---

# Phase 5 Plan 04: Hybrid Auto-Import + env import-dump Summary

**Hybrid D-01: once-from-recorded auto-import after successful deployflow plus `magelift env import-dump` for retries — no `env seed`, no Pulumi create.**

## Performance

- **Duration:** 4 min
- **Started:** 2026-07-29T16:14:15Z
- **Completed:** 2026-07-29T16:18:07Z
- **Tasks:** 3 (1 decision auto-locked + 2 auto)
- **Files modified:** 5

## Accomplishments

- Locked D-01 hybrid (option-a) per executor AUTO instruction matching CONTEXT
- `magelift env import-dump` runs dumpimporter with journal MarkImporting → MarkImported|MarkFailed; binds overwrite to persistent `--yes`
- After successful `deployflow.Run`, `maybeAutoImportSeedDump` fires once when SeedDump set and status=`recorded`; later deploys no-op

## Task Commits

1. **Task 1 (checkpoint): Lock D-01 hybrid** — auto-selected option-a (no commit; recorded here)
2. **Task 2: env import-dump command** — `69f3e08` (feat)
3. **Task 3: Post-deploy auto-import once from recorded** — `a716869` (feat)

**Plan metadata:** _(pending docs commit)_

## Files Created/Modified

- `internal/cli/env.go` — `envImportDumpCommand`, `runSeedDumpImport`, `maybeAutoImportSeedDump`
- `internal/cli/root.go` — register `import-dump`; injectable `importSeedDump` on options
- `internal/cli/lifecycle.go` — call auto-import after deployflow success (post-lock)
- `internal/cli/env_import_dump_test.go` — import-dump + --yes + naming tests
- `internal/cli/lifecycle_seeddump_test.go` — AutoImportOnce / RecordedOnly / failure fails deploy

## Decisions Made

- **D-01 option-a (AUTO):** Hybrid post-deployflow once-from-recorded + `env import-dump` for retries. Never Pulumi create. Never name command `env seed`.
- Auto-import failure: journal=`failed`+reason and deploy exits nonzero; recover via `env import-dump --yes`.
- First recorded path forces `dumpimport.Options.Yes=true` (Pattern 3); explicit retries from `failed` require operator `--yes` when nonempty.

## Deviations from Plan

None - plan executed exactly as written (checkpoint auto-locked to CONTEXT option-a per user AUTO instruction).

## Auth Gates

None.

## Known Stubs

None.

## Threat Flags

None beyond plan threat model (T-05-09/10/11 mitigated by recorded-only gate, post-lock placement, failed+nonzero on import error).

## Verification

```text
GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/cli/ -count=1 -run 'ImportDump|AutoImportOnce|RecordedOnly|SeedDump'
# ok
! grep -nE 'Use:[[:space:]]+"seed"|env seed' internal/cli/env.go internal/cli/root.go
```

## Self-Check: PASSED

- FOUND: `internal/cli/env_import_dump_test.go`, `lifecycle_seeddump_test.go`, `05-04-SUMMARY.md`
- FOUND commits: `69f3e08`, `a716869`
