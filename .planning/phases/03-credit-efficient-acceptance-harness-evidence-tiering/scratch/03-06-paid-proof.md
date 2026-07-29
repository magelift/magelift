# 03-06 Paid AWS acceptance proof

**Status:** complete — 2026-07-29. Evidence under gitignored `.magelift/`; this note cites harness output only (no invented PASS rows).

Live log: `scratch/03-06-live-run-health.log`

## Required env (used)

| Variable | Value (non-secret) |
|----------|--------------------|
| `MAGELIFT_BIN` | `/tmp/magelift` (`version: dev`) |
| `MAGELIFT_CONFIG` | `.magelift/acceptance.magelift.yaml` |
| `MAGELIFT_AWS_ACCEPTANCE_DIGEST` | `…/magelift-acceptance@sha256:df04554d…` (php-runtime `/health`) |
| `MAGELIFT_AWS_CERTIFICATE_IDENTITY` | `alex.courtiol@gmail.com` |
| `MAGELIFT_AWS_CERTIFICATE_OIDC_ISSUER` | `https://github.com/login/oauth` |
| `AWS_REGION` | `eu-north-1` |
| `MAGELIFT_AWS_ACCEPTANCE_PROFILE` | `preview` |

Account: `669890779205`.

## Cell catalog (≥3, free-tier only)

1. `queueMode:db`
2. `queueMode:ecs-rabbitmq`
3. `queueMode:ecs-artemis`

## Proof checklist

| Criterion | Log / artifact proof | Done |
|-----------|----------------------|------|
| ACCEPT-01 create-once then ≥3 cell-updates, no re-create | Log: one `acceptance create-once` then `cell-update` for db / rabbitmq / artemis; resume skipped create-once | yes |
| ACCEPT-02 kill+resume skips PASS cells | After `queueMode:db` PASS, harness killed; resume logged `acceptance resume: skipping create-once` + `acceptance skip cell=queueMode:db` | yes |
| ACCEPT-03 harness-written matrix-results six columns | `.magelift/matrix-results.md` (sample below) | yes |
| ACCEPT-04 assert_clean leftover≠0 and clean=0 | Leftover while KEEP stack up → FAILED (VPC/RDS/ALB/ECS/…); after destroy → `assert_clean ok` / EXIT 0 | yes |

## Evidence sample (redacted)

From harness-written `.magelift/matrix-results.md`:

```
| cell | result | duration | provider | account | date |
|------|--------|----------|----------|---------|------|
| queueMode:db | PASS | 255s | aws | 669890779205 | 2026-07-29 |
| queueMode:ecs-rabbitmq | PASS | 254s | aws | 669890779205 | 2026-07-29 |
| queueMode:ecs-artemis | PASS | 204s | aws | 669890779205 | 2026-07-29 |
```

Create-once Pulumi apply Duration: **10m13s** (113 resources). Runtime health: healthy (ECS 1/1).

## assert_clean dual outcome

1. **Leftover → non-zero:** After kill mid-matrix with stack kept: `assert_clean FAILED` (leftover VPCs/RDS/ElastiCache/ALBs/ECS/logs/SGs).
2. **Clean → 0:** `magelift destroy --yes` → `DESTROY_EXIT:0`; `assert_clean ok` (re-checked EXIT 0). Spot-check: 0 VPCs / ECS clusters / RDS / ALBs tagged acceptance.

## HUMAN_GATE

- [x] Spend approval in chat (AWS + GCP approved; this pass used AWS free-tier preview only)
- [x] Live create only after approval
- [x] Account left clean after final destroy

## Notes

- Prior attempt with non-runtime digest failed ALB `/health` 404; this pass used signed `php-runtime` health image.
- GCP live create **not** run (Phase 7); 03-05 dry-run already covers harness shape.
