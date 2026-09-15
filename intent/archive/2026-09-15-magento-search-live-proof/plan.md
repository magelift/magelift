---
status: planned
slug: magento-search-live-proof
spec: spec.md
---

# Plan: live Magento search on both certified origins

Auto-approved per the standing `/goal` instruction.

## Files that change

- Evidence: search sections inside the order-8 session records (no
  separate files unless a cell fails independently).
- EDIT `docs/capability-matrix.md`: only if a cell status changes.

## Order of work

- [x] 1.1 GCP workload proof inside the GCP packed session —
  verify: reindex log, query output, recycle plus re-query log
  (`mldp6`: reindex exit 0, query HTTP 200, pod recycle + re-query
  HTTP 200; `gcp-gke-autopilot-magento-search-live-mldp6-20260914.md`)
- [x] 1.2 AWS provisioned proof inside the AWS packed session —
  verify: same three plus least-privilege notes, spend inside $5
  (CLOSED AS EXPERIMENTAL per the Risk rule: `mlaw1`
  `searchMode:provisioned` PASS infra-only, Magento connection
  validated by `setup:upgrade`, least-privilege notes in the mlaw1
  record; explicit reindex/query/recycle not run, no retry)
- [x] 1.3 Matrix update if statuses change — verify: docs build
  (no status changes; `make docs` strict green)

## Risks

- AWS single-attempt rule: a failure closes the cell as
  experimental, not as retry.

## Proof

Session transcripts, evidence rows, spend lines.
