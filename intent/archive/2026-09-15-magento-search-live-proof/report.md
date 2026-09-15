# Report: magento-search-live-proof — PASS (AWS closed experimental)

## What shipped

- GCP (`mldp6`, Autopilot): full workload triplet — `indexer:reindex
  catalogsearch_fulltext` exit 0, storefront query HTTP 200 with the
  product hit, OpenSearch pod recycled and re-queried HTTP 200 with no
  manual reindex. Evidence
  `gcp-gke-autopilot-magento-search-live-mldp6-20260914.md` plus sealed
  JSONL. The first reindex exposed the `#env()`-under-`system/` defect,
  fixed with the `CONFIG__DEFAULT__CATALOG__SEARCH__*` bindings.
- AWS (`mlaw1`, Fargate): `searchMode:provisioned` PASS (infra-only)
  with the legacy ES service-linked role and the `VpcId` fix; Magento
  `setup:upgrade` validated the HTTPS connection on the following
  cell (after the `https://` hostname fix). Least-privilege notes in
  the mlaw1 record: FGAC off, unsigned HTTPS in-VPC, SGs as the gate.
- No matrix status changes; search data-planes stay experimental.

## Deviations

- 1.2 closed as experimental per the plan's single-attempt rule: no
  explicit AWS reindex/query/recycle triplet, no retry session. The
  Magento-level triplet is proven on GCP only.

## Spend

- Inside the order-8 session caps (AWS $25 session, GCP packed
  session); no standalone search stacks were built, per the intent.

## Verdict: pass
