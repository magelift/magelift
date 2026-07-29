# 05-06 local cutover proof (offline)

**Date:** 2026-07-29T16:25:16Z  
**Decision:** D-06 / MIGRATE-04 local half  
**Claim boundary:** DNS + live non-prod cutover + managed-instance dump cell are **not** claimed. Those ride **Phase 7 HUMAN_GATE**. No new paid AWS pass.

Raw command capture: `05-cutover-local-proof-raw.log` (local scratch; may be gitignored).

## Commands run and outcomes

| Step | Command / action | Expected | Outcome |
| --- | --- | --- | --- |
| Dump import | `GOMAXPROCS=1 GOFLAGS=-p=1 go test ./internal/dumpimport/ -count=1 -run 'TestImport'` | tiny.sql (+ .gz) creates tables via ephemeral MySQL docker compose | **PASS** (~12s) |
| Dump overwrite safety | `go test ./internal/dumpimport/ -run 'NonEmpty\|Yes\|Refuse'` + `./internal/cli/ -run NonEmptyRetryRequiresYes` | Non-empty retry needs `--yes` | **PASS** |
| Media sync | `go test ./internal/mediasync/ -run 'ListingDiff\|MediaSync\|Sync'` | Fixture listing-diff empty; merge extras; path escape | **PASS** |
| CLI wiring | `go test ./internal/cli/ -run 'EnvImportDump\|MediaSync'` | import-dump journal + media-sync help/inject | **PASS** |
| Floci media | Re-run Floci listing-diff | Empty missing-key diff on LocalStack-compatible endpoint | **SKIP** — `localhost:4566` not up this session; prior proof in `05-05-SUMMARY.md` (`TestMediaSyncListingDiffAgainstFloci`, MAGELIFT_FLOCI=1) |
| Maintenance shape | `magelift exec --help` documents `--service web --container web -- <command>` | Operator can run `bin/magento maintenance:enable\|disable` via exec | **Documented** — no live ECS session (would need AWS account) |
| Reindex shape | `magelift reindex --help` | Fixed Magento reindex day-2 verb present | **OK** (help printed) |
| Verify shape | Runbook §5: `env status`, logs, health | Checklist only offline | **Documented** in `docs/migrating-from-paas.md` — no live ALB smoke |
| Rollback shape | `magelift env destroy --help` + DNS revert prose | Destroy requires `--yes`; DNS revert first | **Documented** — destroy not executed against cloud |
| DNS / live | — | Phase 7 HUMAN_GATE | **Not run** |

## Explicit non-claims

- No Route53/ACM DNS flip.
- No paid AWS or GCP cutover rehearsal.
- No managed Aurora/RDS dump-import acceptance cell (Phase 7).
- SC5 live half remains Pending; Phase 5 closes runbook + this scratch only.

## Pointers

- Runbook: `docs/migrating-from-paas.md` § Cutover runbook
- Fixtures: `testdata/fixtures/migrate/tiny.sql`, `testdata/fixtures/migrate/media/`
