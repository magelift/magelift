---
phase: 05
slug: data-migration-cutover
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-07-29
---

# Phase 5 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Sourced from `05-RESEARCH.md` Validation Architecture + Wave 0 gaps.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` (race optional via Makefile `test`); Floci via `//go:build floci` |
| **Config file** | none — package tests |
| **Quick run command** | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/seeddump/ ./internal/dumpimport/ ./internal/mediasync/ ./internal/cli/ ./internal/config/ -count=1` |
| **Full suite command** | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/... -count=1` plus `make floci-test` when Docker/Floci up; `make generate-check`; `make check-clean-room` |
| **Estimated runtime** | ~60–180 seconds focused; Floci longer when enabled |

Serial builds only on this Mac (`AGENTS.md` / `.cursor/rules/serial-builds-only.mdc`): never raise parallelism; abort under memory pressure.

---

## Sampling Rate

- **After every task commit:** Run the plan task's `<automated>` command with `GOMAXPROCS=1 GOFLAGS=-p=1`
- **After every plan wave:** Quick run command + `make generate-check` when config touched
- **Before `/gsd-verify-work`:** Full suite green (or Floci skip documented) + scratch cutover log + `make check-clean-room`
- **Max feedback latency:** ~180 seconds for focused packages

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 05-01-01 | 01 | 1 | MIGRATE-01/02 | T-05-01 | Status not in YAML | unit | `go test ./internal/seeddump/ ./internal/cli/ ./internal/config/ -count=1 -run 'SeedDump\|Create.*Dump\|InitRecorded' && make generate-check` | ❌ W0 | ⬜ pending |
| 05-01-02 | 01 | 1 | MIGRATE-02 | T-05-01 | KnownFields rejects seedDumpStatus YAML | unit | `go test ./internal/config/ ./internal/seeddump/ -count=1 -run 'SeedDump\|KnownFields\|InitRecorded\|Status'` | ❌ W0 | ⬜ pending |
| 05-02-01 | 02 | 2 | MIGRATE-02 | T-05-05 | Journal transitions locked | unit | `go test ./internal/seeddump/ -count=1 -run 'Mark\|Status\|Failed\|Import'` | ❌ W0 | ⬜ pending |
| 05-02-02 | 02 | 2 | MIGRATE-02 | T-05-04 | env status merges journal | unit | `go test ./internal/cli/ ./internal/seeddump/ -count=1 -run 'EnvStatus\|SeedDumpStatus\|Status'` | ❌ W0 | ⬜ pending |
| 05-03-01 | 03 | 2 | MIGRATE-05 | T-05-06 | — (decision gate) | checkpoint | n/a | — | ⬜ pending |
| 05-03-02 | 03 | 2 | MIGRATE-01/05 | T-05-06 | Refuse nonempty without Yes | unit/int | `go test ./internal/dumpimport/ -count=1 -run 'NonEmpty\|Yes\|Converge\|Import\|Corrupt'` | ❌ W0 | ⬜ pending |
| 05-04-01 | 04 | 3 | MIGRATE-01 | T-05-09 | — (decision gate) | checkpoint | n/a | — | ⬜ pending |
| 05-04-02 | 04 | 3 | MIGRATE-01/05 | T-05-06 | import-dump + --yes | unit | `go test ./internal/cli/ -count=1 -run 'ImportDump\|SeedDump'` | ❌ W0 | ⬜ pending |
| 05-04-03 | 04 | 3 | MIGRATE-01/02 | T-05-09 | Auto once from recorded | unit | `go test ./internal/cli/ -count=1 -run 'AutoImportOnce\|RecordedOnly\|maybeAutoImport\|SeedDump'` | ❌ W0 | ⬜ pending |
| 05-05-01 | 05 | 4 | MIGRATE-03 | T-05-12 | Confine --source | unit | `go test ./internal/mediasync/ ./internal/cli/ -count=1 -run 'MediaSync\|ListingDiff\|mediasync'` | ❌ W0 | ⬜ pending |
| 05-05-02 | 05 | 4 | MIGRATE-03 | T-05-12 | Floci listing diff | floci | `make floci-test` / `-tags=floci` MediaSync | ❌ W0 | ⬜ pending |
| 05-06-01 | 06 | 4 | MIGRATE-04 | T-05-15 | Runbook complete | docs | python keyword gate + `make check-clean-room` | ❌ W0 | ⬜ pending |
| 05-06-02 | 06 | 4 | MIGRATE-04 | T-05-14 | Phase 7 HUMAN_GATE split | docs | scratch file + rg Phase 7/HUMAN_GATE | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

All `go test` invocations above must be prefixed with `GOMAXPROCS=1 GOFLAGS=-p=1` when run locally.

---

## Wave 0 Requirements

- [ ] `SeedDump` on `config.Environment` + `config.Config`; `make generate` / schema update; create `--dump` Load test (`05-01`)
- [ ] `internal/seeddump/` journal package + tests (`05-01`/`05-02`)
- [ ] `internal/dumpimport/` importer + nonempty/`--yes` tests + `testdata/fixtures/migrate/*.sql.gz` (`05-03`)
- [ ] CLI: `env status`, `env import-dump`, `env media-sync`; remove placeholder create-status prose (`05-02`/`05-04`/`05-05`)
- [ ] Lifecycle auto-import once-from-recorded test (`05-04`)
- [ ] Floci or fake-S3 media listing-diff test (`05-05`)
- [ ] Cutover section in `docs/migrating-from-paas.md` + scratch proof (`05-06`)
- [ ] REQUIREMENTS/STATE note: MIGRATE-04 DNS/live + managed dump → Phase 7 HUMAN_GATE (`05-06`)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live DNS + non-prod cutover on managed target | MIGRATE-04 / SC5 live | Paid cloud + DNS; locked Phase 7 HUMAN_GATE (D-06) | Phase 7 HUMAN_GATE — not Phase 5 |
| Managed Aurora/Cloud SQL dump-import cell | MIGRATE-01 managed half | Batched into Phase 7 GCP pass | Phase 7 — no Phase 5 paid AWS pass |

Local dump/media/maintenance/reindex/verify/rollback are automated or scratch-recorded in Phase 5 (`05-06`).

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies (checkpoints excepted)
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 180s for focused packages
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
