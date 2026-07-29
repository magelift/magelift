---
phase: 05-data-migration-cutover
verified: 2026-07-29T16:28:59Z
status: passed
score: 5/5 must-haves verified
behavior_unverified: 0
overrides_applied: 0
re_verification: false
deferred:
  - truth: "Managed-instance dump import after first successful deploy (SC1 managed half)"
    addressed_in: "Phase 7"
    evidence: "ROADMAP Phase 5 cloud spend + Phase 7 depends on Phase 5 ('dump-import cell rides this pass'); REQUIREMENTS MIGRATE-04 Pending → Phase 7 HUMAN_GATE; D-06"
  - truth: "Live DNS + full non-prod cutover rehearsal with run recorded (SC5 live half / MIGRATE-04 live)"
    addressed_in: "Phase 7"
    evidence: "docs/migrating-from-paas.md Evidence honesty table; REQUIREMENTS MIGRATE-04 honesty split; STATE.md; scratch/05-cutover-local-proof.md claim boundary; D-06"
---

# Phase 5: Data Migration & Cutover Verification Report

**Phase Goal:** A live store's data actually lands in MageLift — `seedDump` stops being a recorded status string, media follows, and a documented runbook moves a real store over with a way back.

**Verified:** 2026-07-29T16:28:59Z  
**Status:** passed  
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

Roadmap SC1–SC5. Phase 5 scope for SC1 managed + SC5 live is the documented D-06 honesty split (local proof here; live/DNS/managed cell → Phase 7 HUMAN_GATE). Per verifier brief: do **not** fail Phase 5 when runbook + scratch + HUMAN_GATE split recording exist.

| # | Truth | Status | Evidence |
| --- | ------- | ---------- | -------------- |
| 1 | `magelift env create --dump` + post-deploy import leaves dump tables queryable; placeholder status gone (SC1 local) | ✓ VERIFIED | `env create --dump` → `seeddump.InitRecorded` (`internal/cli/env.go`); auto-import `maybeAutoImportSeedDump` after deploy success (`lifecycle.go`); `TestAutoImportOnceFromRecorded` / `TestEnvCreateDumpPersistsSeedDumpAndJournalRecorded`; `dumpimport` `TestImportCreatesTablesFromTinySQL` (+ `.gz`) PASS (~31s, ephemeral MySQL). Managed-instance half → deferred Phase 7 |
| 2 | `magelift env status` shows `seedDumpStatus` recorded→importing→imported / failed+reason (SC2) | ✓ VERIFIED | `seeddump.Store` Mark* + `.magelift/seed-dumps/<env>.json`; `TestEnvStatusMergesSeedDumpPathAndImportedJournal`, `TestEnvStatusMergesFailedReasonFromJournal`; `TestEnvImportDumpFailureMarksFailed`; `TestCorruptSQLSurfacesClearError` |
| 3 | Non-empty re-import exits non-zero without `--yes`; interrupted re-run with `--yes` converges (SC3) | ✓ VERIFIED | `ErrNonEmptyRequiresYes`; `TestNonEmptyWithoutYesRefuses`, `TestYesSchemaReplaceConverges`; CLI `TestEnvImportDumpNonEmptyRetryRequiresYes`, `TestEnvImportDumpWithYesAllowsFailedRetry`. No `--force` / invent-confirm flag |
| 4 | Media-sync copies fixture media; listing diff vs source empty (SC4) | ✓ VERIFIED | `mediasync.Sync` + `DiffKeys`; `TestMediaSyncFixtureListingDiffEmpty` (+ merge/key-map tests) PASS; `TestEnvMediaSyncUploadsFixtureListingDiffEmpty` PASS. Floci `TestMediaSyncListingDiffAgainstFloci` not re-run (LocalStack `:4566` down); prior PASS cited in `05-05-SUMMARY.md` |
| 5 | Cutover runbook (DNS, maintenance, reindex, verification, rollback) + recorded offline proof (SC5 Phase 5 scope) | ✓ VERIFIED | `docs/migrating-from-paas.md` Cutover runbook §§ + Evidence honesty table; `scratch/05-cutover-local-proof.md` (2026-07-29); REQUIREMENTS/STATE keep MIGRATE-04 Pending with Phase 7 HUMAN_GATE. Live DNS/rehearsal → deferred |

**Score:** 5/5 truths verified (0 present, behavior-unverified)

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | Managed-instance dump-import acceptance cell (SC1 managed) | Phase 7 HUMAN_GATE | Phase 5/7 ROADMAP spend map; MIGRATE-04 Pending note |
| 2 | Live DNS + full non-prod cutover rehearsal (SC5 live / MIGRATE-04) | Phase 7 HUMAN_GATE | Runbook honesty table; scratch non-claims; D-06 |

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | ----------- | ------ | ------- |
| `internal/seeddump/` | Journal store + status machine | ✓ VERIFIED | store.go 230 + status.go 51 + tests 325; path `.magelift/seed-dumps/` |
| `internal/dumpimport/` | Import + NonEmpty + Yes schema-replace | ✓ VERIFIED | import.go 275 + nonempty.go 78; package tests PASS |
| `internal/mediasync/` | Upload + listing diff | ✓ VERIFIED | sync.go 248; unit tests PASS |
| `internal/cli/env.go` | create --dump, status, import-dump, media-sync | ✓ VERIFIED | 692 lines; wired on env group via root.go |
| `internal/cli/lifecycle.go` | once-from-recorded auto-import | ✓ VERIFIED | `maybeAutoImportSeedDump` after deploy; lifecycle_seeddump_test.go |
| `testdata/fixtures/migrate/` | tiny/corrupt SQL + media fixture | ✓ VERIFIED | Present |
| `tests/floci/media_sync_test.go` | Floci listing-diff | ✓ VERIFIED | Exists (floci build tag); not re-executed this session |
| `docs/migrating-from-paas.md` | Cutover runbook | ✓ VERIFIED | DNS/maintenance/reindex/verify/rollback + honesty split |
| `scratch/05-cutover-local-proof.md` | Local offline proof | ✓ VERIFIED | Commands + outcomes + non-claims |
| REQUIREMENTS/STATE MIGRATE-04 split | Phase 7 HUMAN_GATE recording | ✓ VERIFIED | Checkbox unchecked; Pending + Phase 7 note |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `env create --dump` | YAML `seedDump` + journal `recorded` | `InitRecorded` | ✓ WIRED | create JSON returns `seedDumpStatus=recorded` (not ADR placeholder prose) |
| `env status` | YAML path + journal | `seeddump.Read` merge | ✓ WIRED | imported / failed+reason tests |
| Deploy success | Auto-import once | `maybeAutoImportSeedDump` when status=`recorded` | ✓ WIRED | `TestAutoImportOnceFromRecorded`, `TestAutoImportRecordedOnly` |
| `env import-dump` | `dumpimport.Import` + Mark* | `runSeedDumpImport` | ✓ WIRED | Named `import-dump` not `seed` |
| `env media-sync --source` | S3 PutObject walk → List → DiffKeys | `mediasync.Sync` | ✓ WIRED | Help documents merge + key mapping |
| Runbook | CLI commands + scratch | Docs § + proof table | ✓ WIRED | import-dump / media-sync / reindex / exec / destroy shapes |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| Dump import | SQL fixture → MySQL tables | `tiny.sql` / docker compose mysql | Yes — queryable tables in import tests | ✓ FLOWING |
| Seed status | journal Status/Reason | `.magelift/seed-dumps/*.json` | Yes — recorded/importing/imported/failed | ✓ FLOWING |
| Media sync | source keys → bucket listing | fixture walk + fake/Floci client | Yes — empty missing-key Diff | ✓ FLOWING |
| env status JSON | seedDump + seedDumpStatus | config Load + journal Read | Yes — merged in CLI tests | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| seeddump package | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/seeddump/ -count=1` | ok 0.427s | ✓ PASS |
| dumpimport package | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/dumpimport/ -count=1` | ok 31.290s | ✓ PASS |
| mediasync package | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/mediasync/ -count=1` | ok 0.339s | ✓ PASS |
| CLI migrate surface | `go test ./internal/cli/ -count=1 -run 'EnvSeedDump\|EnvStatus\|EnvImportDump\|MediaSync\|AutoImport\|SeedDump\|NonEmptyRetry'` | ok | ✓ PASS |
| media-sync help | `TestEnvMediaSyncHelpDocumentsMergeAndKeyMapping` | PASS | ✓ PASS |
| Floci media-sync | `MAGELIFT_FLOCI=1 … TestMediaSyncListingDiffAgainstFloci` | LocalStack `:4566` down — SKIP; cite `05-05-SUMMARY.md` prior PASS | ? SKIP |

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| — | — | No `scripts/*/tests/probe-*.sh` declared for Phase 5 | SKIPPED (none) |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ---------- | ----------- | ------ | -------- |
| MIGRATE-01 | 05-01, 05-03, 05-04 | `--dump` imports after first deploy; seedDump not inert | ✓ SATISFIED (local) | create+journal+auto-import+dumpimport tests; managed cell deferred Phase 7 |
| MIGRATE-02 | 05-01, 05-02, 05-04 | Honest seedDumpStatus machine + reason | ✓ SATISFIED | store Mark* + env status/import tests |
| MIGRATE-03 | 05-05 | Media sync to object storage | ✓ SATISFIED | mediasync + CLI media-sync tests; Floci prior PASS |
| MIGRATE-04 | 05-06 | Cutover runbook E2E | ✓ SATISFIED (Phase 5 local) / Pending live | Runbook+scratch+HUMAN_GATE split; checkbox correctly **unchecked** until Phase 7 |
| MIGRATE-05 | 05-03, 05-04 | Retry-safe; refuse nonempty without `--yes` | ✓ SATISFIED | dumpimport + CLI NonEmptyRetry tests |

No orphaned Phase 5 requirements outside MIGRATE-01..05.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| — | — | No TBD/FIXME/XXX/TODO/HACK/PLACEHOLDER in phase key Go files | — | None |

Placeholder ADR prose on create path replaced by journal `recorded` (asserted in `env_seeddump_test.go`).

### Human Verification Required

None for Phase 5 closure. Live DNS / managed dump / full non-prod cutover are **deferred** Phase 7 HUMAN_GATE items (not mid-phase UAT for this verify).

### Remaining HUMAN_GATEs

1. **Phase 7 — MIGRATE-04 live half:** DNS cutover + full non-prod rehearsal against a managed target, with the run recorded; clear MIGRATE-04 Complete only after this gate.
2. **Phase 7 — Managed dump-import cell:** SC1 “once on a managed instance” acceptance (rides GCP paid pass; no Phase 5 paid AWS pass).

### Gaps Summary

No blocking gaps for Phase 5 offline goal. Honesty split is intact: local dump/media/status/runbook proven; live DNS and managed dump explicitly Pending → Phase 7.

---

_Verified: 2026-07-29T16:28:59Z_  
_Verifier: Claude (gsd-verifier)_
