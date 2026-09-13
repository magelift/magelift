---
type: lesson
title: GCP HA Magento known-content must use the seed fixture
description: Live installed-schema dumps have catalog schema but no products and no magelift_seed_probe; plant tiny-fixture after migrate:dump instead of defaulting a sample-data SKU.
tags:
- gcp
- magento
- ha
- openspec
status: stable
generated:
  by: cursor-grok/desktop
  at: '2026-08-18'
---

`tiny.sql` is the offline dumpimport fixture. Live Magento cells use an
installed-schema dump (flag / setup_module / core_config_data). That live dump
creates `catalog_product_entity` with no product rows and does not include
`magelift_seed_probe`. A sample-data SKU such as `24-MB01` will fail.

`migrate:dump` therefore applies
`testdata/fixtures/migrate/magelift-seed-probe.sql` after `env import-dump`
while the mysql-client pod is still up, then verifies
`label=tiny-fixture`. HA pod/node/zone cells read that row through Magento's
resource layer (`magento-seed-probe`) unless
`MAGELIFT_HA_MAGENTO_SEED_PROBE=0`. Catalog SKU and HTTP storefront remain
optional extras for dumps that actually contain those markers.

`setup:db:status` is still not Magento known-content. A live HA PASS of the
seed probe is the upgrade path for the `gcha19`/`gcha22` ceiling. Not 3.6
complete until that cell exists.

## Related

See [GCP fence gap overlays the scenario matrix](GCP%20fence%20gap%20overlays%20the%20scenario%20matrix.md).
