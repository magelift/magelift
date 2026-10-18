# GCP GKE Standard Magento 2.4.9 - 2026-09-14 (`mldp4`)

Standard catalog 13/13 PASS on `gke-standard` (explicit runtime
override) with a Cosign-signed Magento digest, Magento HTTP 200,
then destroy on EXIT. In-process backend by design (the subprocess
proof cell is `gke-autopilot` only). First attempt as `mldp3`
failed before any cell on transient GCP Valkey capacity
(`Error code 8`, zero cells, full destroy); this retry as `mldp4`
is the record. GKE Standard stays experimental. Do not reuse
`mldp4` for profile `standard` (WIF pool tombstoned 30d).

## Scope

| Field | Value |
| --- | --- |
| Project | GCP acceptance project (accountRef redacted in sealed JSONL) |
| Region | `europe-west1` |
| Name | `mldp4` (pool `ml-mldp4-standard` tombstoned; `ml-mldp3-standard` also spent by the capacity-failed attempt) |
| Profile | standard (`MAGELIFT_GCP_ACCEPTANCE_ALLOW_COSTLY=true`) |
| Runtime | `gke-standard` (explicit override) |
| Catalog | standard 13-cell set (preview cells plus `queue:health`, no `deploy:repeat`) |
| Artifact A | `…/magento-249-rc1-static-owned-dbhost-20260816@sha256:8588b13f…fdb2be4` |
| Seed | `magento-249-sanitized-definer-free.sql.gz` regenerated via `setup:install` of Artifact A (sha256 `a2cd333c…`, zero DEFINERs, values not logged) |
| Cosign identity | Google SA `devops@…iam.gserviceaccount.com` (existing signature verified; no fresh sign) |
| Cosign issuer | `https://accounts.google.com` |
| KEEP | unset (destroy on EXIT) |

## Catalog

All 13 cells PASS: `bootstrap:wif`, `composer:sm-write`, `composer:sm-read`,
`day2:secrets`, `day2:state`, `day2:logs`, `day2:exec`, `day2:health`,
`search:health`, `migrate:dump`, `deploy:candidate` (101s),
`queue:health`, `cost:estimate`. `deploy:candidate` observed Magento
HTTP 200 (`runtime.web`, `layer=magento`) and Magento CLI 2.4.9.

Unsealed JSONL run `run-20260914t171509z-569079` recorded those cells as
PASS (candidates redacted to the `gcap28` mapping, then sealed):
cleanup PASS, `assert_clean ok`, sealed bundle
[`runs/gcp-gke-standard-magento-mldp4-20260914.sealed.jsonl`](runs/gcp-gke-standard-magento-mldp4-20260914.sealed.jsonl).

## Spend

`cost:estimate` is account-free (no live Catalog prices). Paid resources
(GKE Standard nodes, Cloud SQL, Memorystore Valkey, LB) lived only for
the run window (~50 min create-to-destroy, plus the ~40 min
capacity-failed `mldp3` attempt that created-then-destroyed partial
infra); no KEEP, no leftovers.

## Leftover

HA, order-9 GCP search cell, and the order-10 preview loop run later
in this packed session. Fastly, New Relic, Cloudflare, dump-retrieve,
Magento-wired native obs, and HA Magento known-content stay on prior
evidence (`gcap27`, `gcha36`). SendGrid delivery stays typed
unsupported (standard YAML omits `email`). Magelift-owned
sink/dashboard stays typed unsupported.

## Non-claims

- GKE Standard public certified (experimental target)
- EKS / OVH / Scaleway public certified
- Homebrew/Scoop 4.3 (needs a public tag)
