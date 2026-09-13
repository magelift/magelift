---
type: lesson
title: GCP fence gap overlays the scenario matrix
description: A typed fence/failover or missing-injector gap on gcp.gke must also reclassify the matching failure scenarios; leaving them automated overclaims executability.
tags:
- gcp
- resilience
- fencing
- openspec
status: stable
generated:
  by: cursor-grok/desktop
  at: '2026-08-18'
---

`gcp.gke` does not opt into `fence`, `failover`, or `failback`. Compile and Start
fail closed, and the profile reason records that gap. The failure-scenario
matrix is a separate planning surface. If it still marks `lost-credentials` or
`provider-outage` as operator-assisted, a release plan can look executable
while the adapter cannot fence.

Overlay those scenarios to typed unsupported on GCP GKE until a native fence
translator exists. Keep zone-loss automated when two zones are declared: the
evidenced ceiling is backing-VM zone-loss simulation (`gcha22`). Physical zone
outage is not injectable; say that in the scenario reason instead of
reclassifying zone-loss as unsupported.

`magelift cleanup reconcile` now registers a GCP Cloud SQL ledger adapter
alongside OVH and Scaleway. Interrupted-teardown is automated for marker-
owned Cloud SQL instances and backups. Deletion-protected instances stay
pending. A 2026-08-18 SQL-only cell passed claim/create/record/reconcile.
Other GCP kinds and remaining 3.9 scenarios still need their own evidence.

Failed-deployment has Backup and IntegrityCheck actions but no live injector
evidence. Leaving it automated overclaims that Magelift can inject a bad
artifact. Overlay it to operator-assisted on GCP until a live cell applies
`MAGELIFT_GCP_FAILED_DEPLOY_DIGEST`, observes unhealthy runtime, and restores
the current digest. The library and harness exist; unset keeps
`failed-deployment=not-run`. Keep GKE
pod/node/zone-loss modes unchanged; document that `gcha19`/`gcha22` proved
`setup:db:status`, not Magento known-content application-integrity. The HA
harness now calls `magento-known-content-probe` when
`MAGELIFT_HA_MAGENTO_CONTENT_URL` and `MAGELIFT_HA_MAGENTO_CONTENT_EXPECT` are
set, `magento-catalog-sku-probe` when `MAGELIFT_HA_MAGENTO_CONTENT_SKU` is
set, and `magento-seed-probe` after `migrate:dump` plants
`magelift_seed_probe.label=tiny-fixture` (live installed-schema dumps have
catalog schema but no products). Opt out with
`MAGELIFT_HA_MAGENTO_SEED_PROBE=0`. Do not default a sample-data SKU. Unset
HTTP/SKU plus seed opt-out keeps `known-content=not-run`.

Partial-restore is scoped to database, media, configuration, search, and
cache together. Single-class restore cells do not prove a subset restore.
Overlay it to operator-assisted on GCP until a multi-class injector exists.

Do not mark OpenSpec 3.6, 3.7, 3.8, or 3.9 complete from a matrix overlay.
Recording a gap is not live credential-loss, provider-outage, physical-zone,
failed-deployment, partial-restore, Magento HA integrity, or
interrupted-teardown evidence.

## Related

See [GKE node-loss proof must compare Compute Engine identity](GKE%20node-loss%20proof%20must%20compare%20Compute%20Engine%20identity%202026-08-15.md).
