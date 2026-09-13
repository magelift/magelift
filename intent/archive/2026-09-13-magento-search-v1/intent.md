---
status: done
slug: magento-search-v1
---

# Intent: live Magento search proved on both certified origins

## Scope of this pass

Decision half only, per `ROADMAP.md` Phase 0: choose the two proof
shapes, write the AWS spend cap, and record the proof criteria. The live
runs move to the Phase 2 intent `magento-search-live-proof`. The Problem
through Constraints sections below describe the whole search goal; the
Proposed outcome states what this pass delivers.

## Problem

Certified search today is `disabled`. A catalog shop without search is a spike, not a store. V1 proves one live Magento search cell per certified origin: GCP GKE-workload OpenSearch on the unlimited project, and AWS provisioned OpenSearch done efficiently inside the packed AWS session so it spends the minimum necessary credits.

## Evidence

Capability matrix: `disabled` is the certified preview/free-tier cell; `serverless` (AOSS plus SigV4 sidecar) and `provisioned` (unsigned HTTPS 443 in-VPC) are experimental. `docs/release-readiness.md` rescoped the public-tag gate 2026-07-22 to three substitutes: offline wiring mocks, free-tier matrix, prior Chantelle Terraform ops. It states Chantelle proves ops, not MageLift live search. Paid checklist (index, query, reconnect after recycle, least-privilege task role) is deferred. Unit mocks `TestRuntimeWiresMagentoOpenSearchEnvFromEndpoint` and `TestRuntimeWiresAOSSThroughSigningProxy` cover env wiring only. Live Magento indexing or querying on a MageLift stack: not checked.

## Proposed outcome

This pass records the decision: the exact AWS provisioned shape and GCP
workload shape for the v1 search proof, a written AWS spend cap with the
lifetime rule, the proof criteria each cell must meet, and the evidence
artifacts the packed sessions must produce. The decision lands as an ADR
so Phase 2 runs it without re-litigating. No cloud resources are created
by this pass.

## Affected users and systems

Any shop with a catalog worth searching. AWS search capability (`provisioned`; AOSS SigV4 proxy stays experimental), GCP GKE OpenSearch workload, Magento reindex and query health checks, capability matrix, evidence pack, getting-started and compare-paas pages.

## Constraints

A search box is not Magento search: certification needs Magento reindex or query evidence plus the documented auth path. Floci does not prove the data plane. AWS search spend stays inside the written per-session cap: smallest credible shape, short-lived, immediate destroy, attached to the packed AWS session. GCP cell runs on the unlimited project but still destroys on exit with spend recorded.

## Out of scope

AOSS serverless certification, three-node HA OpenSearch (needs GKE Standard `vm.max_map_count`, experimental), ElasticSuite specifics, search relevance tuning.

## Open questions

What is the cheapest AWS provisioned shape that still proves index, query, and reconnect credibly? What exactly counts as the GCP workload proof (replica count, persistence across recycle)?
