# GCP GKE Autopilot Magento 2.4.9 - 2026-08-23 (`gcap29`)

Packed Autopilot Magento origin for the 2026-08-23 cert campaign. Preview
catalog 13/13 PASS on the same Cosign-signed Magento 2.4.9 digest as `gcap28`,
Magento HTTP 200 after LoadBalancer `base_url`. KEEP was set during the
catalog. Destroy plus empty-prefix inventory closed the cell. This file is
not a certified-row replacement for
[gcap28](gcp-gke-autopilot-magento-live-gcap28-20260820.md). Do not reuse
`gcap29`. There is no signed 2.4.8-p5 image in this account; that Magento
release stays `not-run`.

## Scope

| Field | Value |
| --- | --- |
| Project | GCP acceptance project (accountRef redacted in sealed JSONL) |
| Region | `europe-west1` |
| Name | `gcap29` |
| Profile | preview |
| Runtime | `gke-autopilot` |
| Catalog | `scripts/acceptance/cells-gcp-preview.txt` (13 cells) |
| Artifact | `…/magento-249-rc1-static-owned-dbhost-20260816@sha256:8588b13f…fdb2be4` |
| Seed | `.magelift/seed/magento-249-sanitized-definer-free.sql.gz` (values not logged) |
| Cosign identity | Google SA `devops@…iam.gserviceaccount.com` |
| Cosign issuer | `https://accounts.google.com` |
| KEEP | closed: destroy emptied GKE/SQL/Memorystore/VPC/WIF; state bucket 404 |
| Catalog spec | `certification-gcp` |
| Cell | runtime `gke-autopilot`, Magento 2.4.9, Cloud SQL zonal preview, Memorystore Valkey 9.0, `openSearchMode: opensearch`, `queueMode: database`, digest above |
| Run id | `run-20260823t143648z-34214` |

## Catalog

All 13 cells PASS: `bootstrap:wif`, `composer:sm-write`, `composer:sm-read`,
`day2:secrets`, `day2:state`, `day2:logs`, `day2:exec`, `day2:health`,
`search:health`, `migrate:dump`, `deploy:candidate`, `deploy:repeat`,
`cost:estimate`. `deploy:candidate` observed Magento HTTP 200
(`runtime.web`, `layer=magento`) and Magento CLI 2.4.9. Checkpoint
`.magelift/gcap29/gcp-matrix/gcap29/acceptance-checkpoint.json`.

## Destroy

Direct `magelift destroy --yes --destroy-backups` on 2026-08-23 removed the
reachable Kubernetes objects (cluster already tearing down; `deleteUnreachable`
cleared leftover Services/Secrets from state). PSA peering delete raced
producer release and returned error code 9 once. After that, prefix inventory
was empty: no `gcap29` GKE cluster, Cloud SQL instance, Memorystore instance,
VPC, Secret Manager secret, WIF pool, or
`gs://magelift-…-gcap29-preview-state` bucket (describe 404). Magento 2.4.8-p5
was not attempted; Artifact Registry has no signed 2.4.8-p5 digest.

## Non-claims

- Certified-row replacement for `gcap28`
- Magento 2.4.8-p5 (no signed image in Artifact Registry; `not-run`)
- GKE Standard / EKS / OVH / Scaleway public certified
- Homebrew/Scoop 4.3 (needs a public tag)
