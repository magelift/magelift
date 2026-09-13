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

- [ ] 1.1 GCP workload proof inside the GCP packed session —
  verify: reindex log, query output, recycle plus re-query log
- [ ] 1.2 AWS provisioned proof inside the AWS packed session —
  verify: same three plus least-privilege notes, spend inside $5
- [ ] 1.3 Matrix update if statuses change — verify: docs build

## Risks

- AWS single-attempt rule: a failure closes the cell as
  experimental, not as retry.

## Proof

Session transcripts, evidence rows, spend lines.
