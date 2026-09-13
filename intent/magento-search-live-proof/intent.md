---
status: accepted
slug: magento-search-live-proof
parent: magento-search-v1 (decision archived 2026-09-13, ADR 0012)
---

# Intent: live Magento search on both certified origins

## Problem

Certified search is `disabled`, and a catalog shop without search is a
spike. The decision half (shapes, spend cap, criteria) is frozen in
ADR 0012; no live Magento search has run on MageLift infrastructure
yet.

## Evidence

ADR 0012: AWS provisioned cell (1x `t3.small.search`,
OpenSearch_3.1, 10 GB gp3, HTTPS in-VPC, FGAC internal user, 8h max
lifetime, $5 cap, one attempt); GCP workload cell (1-replica
StatefulSet, pinned `opensearch:3`, 10 GB disk, survive pod delete).
Proof criteria: reindex exit 0, storefront query hits, reconnect
after recycle with no manual reindex; AWS adds least-privilege
review.

## Proposed outcome

One live Magento search cell per certified origin, each attached to
its origin's packed session (order 8). No standalone search stacks.
On AWS failure: keep the GCP proof, leave AWS search experimental
with the failure recorded, re-plan instead of retrying.

## Affected users and systems

Searchable storefronts on either origin. Provider search modes,
Magento reindex/query paths, evidence pack, capability matrix.

## Constraints

ADR 0012 shapes and caps are frozen. AWS one attempt, 8h domain
lifetime max, destroy immediately after proof. GCP destroy on
session exit with spend recorded.

## Out of scope

Three-node HA search, AOSS serverless certification, X-Ray,
regional DR (all non-goals for v1).

## Open questions

None. All decided in ADR 0012.
