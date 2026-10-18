# GCP GKE Standard HA Magento live - 2026-08-20 (`gcha36`)

Status: **PASS**. Profile/runtime were `high-availability` / GKE Standard in
`europe-west3`. Catalog 16/16 PASS, including Magento seed-probe
`tiny-fixture` after pod, node, and zone loss. EXIT `assert_clean` with
leftover Cloud SQL backups `remaining=0`. This closes OpenSpec HA task 3.6
Magento known-content integrity on GCP GKE Standard. It does not recertify
GKE Standard as a public certified target (still experimental).

## Catalog

| Field | Value |
| --- | --- |
| Profile / runtime / region | `high-availability` / GKE Standard / `europe-west3` |
| Create-once | 58 created, 19m7s |
| Magento | 2.4.9 digest `sha256:8588b13fc390bc2b733ec3d03a0501ecc82a6ebad7fcca7758c933977fdb2be4` |
| `deploy:candidate` | PASS after LoadBalancer `base_url` `http://<redacted>/` and Magento HTTP 200 |
| Seed plant | `tiny-fixture` after `migrate:dump` |
| `resilience:pod-loss` | PASS 24s; replacement pod; seed-probe PASS; Magento HTTP 200 |
| `resilience:node-loss` | PASS 292s; `instanceIDChanged=verified`; seed-probe PASS; RTO 289s |
| `resilience:zone-loss` | PASS 296s; `zoneReadyGapObserved=true`; recovered 3/3 nodes and zones; seed-probe PASS; RTO 293s |
| Cache | `cacheLoss=classified-not-injected-managed` |
| Fencing | `not-injected-single-runtime` (zone-loss) |

Physical provider zone outage stays typed unsupported. This cell is backing-VM
zone-loss simulation.

## Destroy leftovers

EXIT `magelift destroy --yes --destroy-backups`: 36 deleted, 2 errored
(12m52s, PSA error 9). Orphan sweep + 180s PSA soak. EXIT `assert_clean ok`
with `leftover Cloud SQL backups remaining=0 instance=gcha36-high-availability-sql`.
Independent inventory empty; WIF pool `ml-gcha36-high-availability` state
`DELETED`. Wrapper exit 0.

## Non-claims

Not GKE Standard public certification. Not sealed JSONL / RC1 5.1. Not two
Magento schema epochs (RC1 4.1). Not Homebrew/Scoop (4.3). Do not reuse
`gcha36`.
