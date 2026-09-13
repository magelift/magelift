# GCP GKE Autopilot Magento 2.4.9 - 2026-08-20 (`gcap28`)

This is the packed RC1 Autopilot Magento origin. Preview catalog 13/13 PASS
on a Cosign-signed Magento digest, Magento HTTP 200 after LoadBalancer
`base_url`, then schema-mismatch leftover on the same KEEP stack. Public
certified target remains GKE Autopilot. Do not reuse `gcap28`.

## Scope

| Field | Value |
| --- | --- |
| Project | GCP acceptance project (accountRef redacted in sealed JSONL) |
| Region | `europe-west1` |
| Name | `gcap28` (never reuse `gcha23`–`gcha36` or `gcap24`–`gcap27`) |
| Profile | preview |
| Runtime | `gke-autopilot` |
| Catalog | `scripts/acceptance/cells-gcp-preview.txt` (13 cells) |
| Artifact A | `…/magento-249-rc1-static-owned-dbhost-20260816@sha256:8588b13f…fdb2be4` |
| Seed | `.magelift/seed/magento-249-sanitized-definer-free.sql.gz` (values not logged) |
| Cosign identity | Google SA `devops@…iam.gserviceaccount.com` |
| Cosign issuer | `https://accounts.google.com` |
| KEEP | `true` until schema-mismatch leftover, then destroy once |

## Catalog

All 13 cells PASS: `bootstrap:wif`, `composer:sm-write`, `composer:sm-read`,
`day2:secrets`, `day2:state`, `day2:logs`, `day2:exec`, `day2:health`,
`search:health`, `migrate:dump`, `deploy:candidate`, `deploy:repeat`,
`cost:estimate`. `deploy:candidate` observed Magento HTTP 200
(`runtime.web`, `layer=magento`) and Magento CLI 2.4.9.

Unsealed JSONL run `run-20260820t100542z-15777` recorded those cells as
PASS (reuse-boundary identities exported). Cleanup SKIP while KEEP was set
was superseded after destroy: cleanup PASS, `assert_clean ok`, sealed bundle
[`runs/gcp-gke-autopilot-magento-gcap28-20260820.sealed.jsonl`](runs/gcp-gke-autopilot-magento-gcap28-20260820.sealed.jsonl).
Destroy emptied the KEEP stack (`assert_clean ok`). Do not reuse `gcap28`.

## Leftover

Schema-mismatch (two Cosign-signed Magento digests, two `schemaEpoch`
values, rollback refused):
[`gcp-gke-autopilot-schema-mismatch-gcap28-20260820.md`](gcp-gke-autopilot-schema-mismatch-gcap28-20260820.md).

Fastly, New Relic, Cloudflare, dump-retrieve, Magento-wired native obs, and
HA Magento known-content stay on prior evidence (`gcap27`, `gcha36`).
SendGrid delivery stays typed unsupported (preview YAML omits `email`).
Magelift-owned sink/dashboard stays typed unsupported.

## Non-claims

- GKE Standard / EKS / OVH / Scaleway public certified
- Homebrew/Scoop 4.3 (needs a public tag)
