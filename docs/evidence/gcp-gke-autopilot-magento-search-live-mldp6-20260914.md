# GCP order-9 search cell - 2026-09-14 (`mldp6`)

First live Magento search proof on MageLift infrastructure: reindex
exit 0, storefront query hits, pod-delete recycle with reconnect and
no manual reindex, on a one-replica OpenSearch StatefulSet
(`opensearchproject/opensearch:3`, 10 GB disk) behind a preview
Autopilot stack. The first reindex exposed a real config defect
(`#env()` literals under env.php `system/` never resolve); the fix
shipped as `CONFIG__DEFAULT__` bindings, deployed via ephemeral
provider `v0.0.0-dialproof.13` (consumed and deleted), and the cell
passed against the fixed stack. Stack retained under KEEP for the
manual proof, then destroyed via RESUME teardown. Do not reuse
`mldp6` for profile `preview` (WIF pool tombstoned 30d).

## Scope

| Field | Value |
| --- | --- |
| Project | GCP acceptance project (accountRef redacted in sealed JSONL) |
| Region | `europe-west1` |
| Name | `mldp6` (pools `ml-mldp6-preview` tombstoned) |
| Profile | preview, KEEP then RESUME teardown (pinned runId across both) |
| Runtime | `gke-autopilot` |
| Provider | subprocess `magelift-provider-gcp v0.0.0-dialproof.13` for the fix redeploy (`.12` for the base stack) |
| Catalog | `scripts/acceptance/cells-gcp-preview.txt` (13 cells) plus the manual search proof below |
| Artifact A | `…/magento-249-rc1-static-owned-dbhost-20260816@sha256:8588b13f…fdb2be4` |
| Seed | `magento-249-sanitized-definer-free.sql.gz` regenerated via `setup:install` of Artifact A (sha256 `a2cd333c…`, zero DEFINERs, values not logged) |
| KEEP | retained for the proof with this written reason, then destroyed |

## Defect and fix

`indexer:reindex catalogsearch_fulltext` failed pinging
`http://#env(MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME, …)…`:
`config:show` returned the raw literal, proving `#env()` under
env.php's `system/` section never resolves (top-level deployment
sections such as `db/` do). Fix (commit `6c53304`): emit the same
resolved search values through stock Magento's `CONFIG__DEFAULT__`
environment contract, which takes precedence over env.php system
values — cloud adapters via `CoreEnvBindings`, localdev compose
likewise. Unit plus local-gates green before the live redeploy.

Redeploy preview showed 3 updates (web plus cron Deployments);
`deploy --infra-only` applied them (full `deploy` on a live stack is
refused by the migrate Job re-create: `Unauthorized`). After the
roll, `config:show` returned host `mldp6-preview-search`, engine
`opensearch`.

## Proof

Against the fixed stack, in ADR 0012 order:

1. Reindex exit 0: `Catalog Search index has been rebuilt
   successfully in 00:00:01` (proof product `ML-SEARCH-PROBE-1`
   created via REST; indexers already `Update by Schedule`).
2. Storefront query `GET /catalogsearch/result/?q=Probe`: HTTP 200,
   product name present (43,899 bytes).
3. Recycle: `kubectl delete pod mldp6-preview-search-0`, Ready in
   ~44s; re-query with no reindex: HTTP 200, product name present
   (43,860 bytes).

Probe admin credentials and API token were generated on the operator
box, passed via files and stdin, never logged, and shredded after
the proof.

## Catalog

The 13 preview cells passed on the base stack before the proof
(`deploy:candidate` Magento HTTP 200, CLI 2.4.9). Pinned run
`run-20260914t190000z-700001` covers both invocations: the KEEP run
recorded the cells plus a SKIP cleanup (retention), the RESUME run
recorded the PASS cleanup (candidates redacted to the `gcap28`
mapping, SKIP record dropped, then sealed): cleanup PASS,
`assert_clean ok`, sealed bundle
[`runs/gcp-gke-autopilot-magento-search-mldp6-20260914.sealed.jsonl`](runs/gcp-gke-autopilot-magento-search-mldp6-20260914.sealed.jsonl).

## Spend

`cost:estimate` is account-free (no live Catalog prices). Paid
resources (Autopilot cluster, Cloud SQL, Memorystore, LB, OpenSearch
disk) lived for the base run (~50 min) plus the proof window (~2h
including the `.13` CI wait) plus teardown; no leftovers.

## Leftover

Order-10 preview loop rehearsal runs later in this packed session;
the order-9 AWS provisioned cell attaches to the AWS session.

## Non-claims

- Multi-node or HA search (single replica only)
- AOSS serverless (stays experimental)
- GKE Standard / EKS / OVH / Scaleway certified
