# GCP GKE Autopilot Magento 2.4.9 - 2026-09-14 (`mldp3`)

First preview catalog through the Dial subprocess provider
(`magelift-provider-gcp v0.0.0-dialproof.12`, ephemeral tag consumed
and deleted). Preview catalog 13/13 PASS on a Cosign-signed Magento
digest, including `day2:exec` through the subprocess (proves the
`redacted-outputs` fix), Magento HTTP 200 after LoadBalancer
`base_url`, then destroy on EXIT. Public certified target remains
GKE Autopilot; current proof stays
[`gcap28`](gcp-gke-autopilot-magento-live-gcap28-20260820.md).
Do not reuse `mldp3` for profile `preview` (WIF pool tombstoned 30d).

## Scope

| Field | Value |
| --- | --- |
| Project | GCP acceptance project (accountRef redacted in sealed JSONL) |
| Region | `europe-west1` |
| Name | `mldp3` (pool `ml-mldp3-preview` tombstoned; `mldp`, `mldp2` already spent) |
| Profile | preview |
| Runtime | `gke-autopilot` |
| Provider | subprocess `magelift-provider-gcp v0.0.0-dialproof.12` (digest-pinned lockfile, Cosign bundle verified at load) |
| Catalog | `scripts/acceptance/cells-gcp-preview.txt` (13 cells) |
| Artifact A | `…/magento-249-rc1-static-owned-dbhost-20260816@sha256:8588b13f…fdb2be4` |
| Seed | `magento-249-sanitized-definer-free.sql.gz` regenerated via `setup:install` of Artifact A (sha256 `a2cd333c…`, zero DEFINERs, values not logged) |
| Cosign identity | Google SA `devops@…iam.gserviceaccount.com` (existing signature verified; no fresh sign) |
| Cosign issuer | `https://accounts.google.com` |
| KEEP | unset (destroy on EXIT) |

## Catalog

All 13 cells PASS: `bootstrap:wif`, `composer:sm-write`, `composer:sm-read`,
`day2:secrets`, `day2:state`, `day2:logs`, `day2:exec`, `day2:health`,
`search:health`, `migrate:dump`, `deploy:candidate`, `deploy:repeat`,
`cost:estimate`. `deploy:candidate` (225s) observed Magento HTTP 200
(`runtime.web`, `layer=magento`) and Magento CLI 2.4.9; early rollout
probes returned HTTP 500, converged healthy before the cell passed.

Unsealed JSONL run `run-20260914t153422z-422805` recorded those cells as
PASS (candidates redacted to the `gcap28` mapping, then sealed):
cleanup PASS, `assert_clean ok`, sealed bundle
[`runs/gcp-gke-autopilot-magento-mldp3-20260914.sealed.jsonl`](runs/gcp-gke-autopilot-magento-mldp3-20260914.sealed.jsonl).
Pulumi destroy reported one transient provider error; bounded orphan
cleanup then proved the ownership prefix empty (`assert_clean ok`).

## Spend

`cost:estimate` is account-free (no live Catalog prices). Paid resources
(GKE Autopilot cluster, Cloud SQL, Memorystore, LB) lived only for the
run window (~50 min create-to-destroy); no KEEP, no leftovers.

## Leftover

Standard (`gke-standard` override), HA, order-9 GCP search cell, and the
order-10 preview loop run later in this packed session. Fastly, New Relic,
Cloudflare, dump-retrieve, Magento-wired native obs, and HA Magento
known-content stay on prior evidence (`gcap27`, `gcha36`).
SendGrid delivery stays typed unsupported (preview YAML omits `email`).
Magelift-owned sink/dashboard stays typed unsupported.

## Non-claims

- GKE Standard / EKS / OVH / Scaleway public certified
- Homebrew/Scoop 4.3 (needs a public tag)
- Replacement of `gcap28` as the current certified-row proof
