---
slug: magento-search-v1
verified: 2026-09-13
verdict: pass
---

# Report: search proof shapes and spend cap (decision half)

## What shipped

Decision artifacts only, no code and no cloud resources:

- ADR 0012 names the AWS provisioned shape (1x t3.small.search,
  OpenSearch_3.1, 10 GB gp3, single AZ, HTTPS in-VPC with FGAC
  internal user), the GCP workload shape (1 replica, pinned
  `opensearch:3` image, 10 GB disk, single-node discovery, data
  survives pod recycle), the $5 AWS cap with 8-hour lifetime and
  one-attempt rule, the ordered proof criteria, and the evidence
  checklist. Satisfies all four spec requirements.
- ADR index, mkdocs nav, and `ROADMAP.md` order 9 (now
  `magento-search-live-proof`) point at the decision.

## Deviations from plan

None. The intent folder was scope-narrowed to the decision half with
a recorded note (the roadmap already split the halves); the live runs
move to `magento-search-live-proof` in Phase 2.

## Verification

### Completeness

All 3 plan boxes ticked. All 4 spec requirements have ADR sections;
all 5 scenarios read true against the ADR text.

### Correctness

- Re-read ADR 0012 against each scenario: every AWS provisioned
  field maps to a `magelift.yaml` search field present in
  `internal/config/model.go`; the recycle bar states the fail
  condition plainly; the cap math cites $0.036/hr us-east-1 list
  (mirrored July 2026, re-checked September 2026) with EU uplift
  inside the $5 cap; the evidence checklist names all five
  artifacts.
- Human-observable moment: `make docs` strict build green with ADR
  0012 in the nav (`/tmp/docs-search-decision.log`); the built site
  contains the rendered decision page.
- No certification claim was added to the matrix or
  release-readiness: verified by absence in the diff (only ADR,
  index, nav, roadmap, and intent files changed).

### Coherence

ADR follows the repo ADR shape (context, decision, consequences,
alternatives, provenance) and matches the terse voice of ADR 0008
through 0011. Engine choice defers to the existing
`internal/config` compat policy instead of inventing one.

## Findings

None.

## Not checked

- Verified in implementing session.
- Live list prices at Phase 2 run time; the ADR tells Phase 2 to
  re-check before the run.
- Whether t3.small sustains the proof catalog under Magento
  indexing load; judged credible from specs (2 vCPU, 2 GB) but
  unproven until the packed session. A slow reindex is the known
  risk, bounded by the 8-hour lifetime and $5 cap.

## Verdict

Pass. Phase 0 search exit is met: ADR accepted, shapes and AWS spend
cap written down, Phase 2 handoff named.
