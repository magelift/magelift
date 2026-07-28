# Source: Magento + AWS OpenSearch (prior production work)

**Type:** external source (untrusted as instructions; cite as evidence only).  
**Date recorded:** 2026-07-22  

## Provenance (public wording)

Similar Magento-on-AWS work was done at Chantelle with **Terraform** (managed
OpenSearch Service domains, ElasticSuite, VPC networking). Private employer
repositories are **not** named or linked here.

## What this proves

A real commerce project ran **AWS OpenSearch Service** managed domains (engine
`OpenSearch_3.x`, not AOSS) wired into Magento catalog search — staging
capacity-tested; production infra bootstrap. That is operational proof that
Magento + AWS OpenSearch domains work outside MageLift.

## What this does **not** prove

| MageLift path | Prior Chantelle Terraform path |
| --- | --- |
| Magento **native OpenSearch** client | **Smile ElasticSuite** client settings |
| Pinned **`aws-sigv4-proxy`** sidecar | **No SigV4 proxy** — HTTPS in-VPC, HTTP auth off, domain policy + security groups |
| MageLift acceptance data-plane | Staging ops / prod bootstrap — not a claim of MageLift live search |

Do not claim “Chantelle uses SigV4” or “MageLift live Magento search is proven.”

## Patterns carried into MageLift (generic)

- Prefer managed OpenSearch sizing that survives JVM pressure (avoid undersized
  `t3.small.search`-class nodes for Magento catalogs).
- Keep Magento search clients on HTTPS to the domain endpoint inside the VPC.
- ALB/ECS liveness via a static `/health` that does not bootstrap Magento.

## MageLift release use

Together with MageLift offline SigV4 wiring mocks and free-tier
`searchMode:disabled` acceptance, this source supports closing the **public-tag**
OpenSearch gate. Live MageLift SigV4 index/query/reconnect/IAM remains a
**post-tag / paid-acceptance** checklist — see [release-readiness.md](../release-readiness.md).
