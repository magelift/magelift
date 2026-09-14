# GCP GKE Standard HA Magento 2.4.9 - 2026-09-14 (`mldp5`)

HA catalog 16/16 PASS on `gke-standard` (multi-zone nodes,
multi-node OpenSearch) with a Cosign-signed Magento digest,
Magento HTTP 200, known-content survival across pod, node, and
zone loss, then destroy on EXIT. In-process backend by design
(the subprocess proof cell is `gke-autopilot` only). GKE
Standard HA stays experimental. Do not reuse `mldp5` for
profile `high-availability` (WIF pool tombstoned 30d).

## Scope

| Field | Value |
| --- | --- |
| Project | GCP acceptance project (accountRef redacted in sealed JSONL) |
| Region | `europe-west1` |
| Name | `mldp5` (pool `ml-mldp5-high-availability` tombstoned) |
| Profile | high-availability (`MAGELIFT_GCP_ACCEPTANCE_ALLOW_COSTLY=true`) |
| Runtime | `gke-standard` (profile default; Autopilot refused for HA) |
| Catalog | HA 16-cell set (standard cells plus `resilience:pod-loss`, `resilience:node-loss`, `resilience:zone-loss`) |
| Artifact A | `…/magento-249-rc1-static-owned-dbhost-20260816@sha256:8588b13f…fdb2be4` |
| Seed | `magento-249-sanitized-definer-free.sql.gz` regenerated via `setup:install` of Artifact A (sha256 `a2cd333c…`, zero DEFINERs, values not logged) |
| Cosign identity | Google SA `devops@…iam.gserviceaccount.com` (existing signature verified; no fresh sign) |
| Cosign issuer | `https://accounts.google.com` |
| KEEP | unset (destroy on EXIT) |

## Catalog

All 16 cells PASS: `bootstrap:wif`, `composer:sm-write`, `composer:sm-read`,
`day2:secrets`, `day2:state`, `day2:logs`, `day2:exec`, `day2:health`,
`search:health`, `migrate:dump`, `deploy:candidate` (111s),
`queue:health`, `cost:estimate`, `resilience:pod-loss` (31s),
`resilience:node-loss` (284s), `resilience:zone-loss` (281s).
`deploy:candidate` observed Magento HTTP 200 (`runtime.web`,
`layer=magento`) and Magento CLI 2.4.9.

Unsealed JSONL run `run-20260914t180332z-649081` recorded those cells as
PASS (candidates redacted to the `gcap28` mapping, then sealed):
cleanup PASS, `assert_clean ok`, sealed bundle
[`runs/gcp-gke-ha-standard-magento-mldp5-20260914.sealed.jsonl`](runs/gcp-gke-ha-standard-magento-mldp5-20260914.sealed.jsonl).

## Spend

`cost:estimate` is account-free (no live Catalog prices). Paid resources
(multi-zone GKE Standard nodes, Cloud SQL HA, Memorystore Valkey,
multi-node OpenSearch, LB) lived only for the run window (~55 min
create-to-destroy); no KEEP, no leftovers.

## Leftover

Order-9 GCP search cell and the order-10 preview loop run later in
this packed session. Fastly, New Relic, Cloudflare, dump-retrieve,
and Magento-wired native obs stay on prior evidence (`gcap27`).
SendGrid delivery stays typed unsupported (HA YAML omits `email`).
Magelift-owned sink/dashboard stays typed unsupported.

## Non-claims

- GKE Standard public certified (experimental target)
- Physical zone outage and regional DR (simulated loss only)
- EKS / OVH / Scaleway public certified
- Homebrew/Scoop 4.3 (needs a public tag)
