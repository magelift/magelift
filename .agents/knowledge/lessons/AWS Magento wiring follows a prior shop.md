---
type: lesson
title: AWS Magento wiring follows a prior shop
description: A prior private Magento-on-AWS shop is one field example. It is not the only architecture and it does not certify MageLift cells.
tags:
- magento
- aws
- opensearch
- aurora
- reference
status: stable
generated:
  at: '2026-08-23'
verified: 2026-08-23
---

One prior private sibling Magento Open Source shop on AWS ECS (staging/production) shows how an agency wired Aurora + provisioned OpenSearch. Treat it only as one field example. Do not copy files into MageLift. Do not treat that shop as the only supported AWS shape. Do not use it as certification evidence. MageLift certifies only `docs/capability-matrix.md` plus `docs/evidence/` for the exact MageLift tuple.

What that example happens to show (not a certified MageLift pin):

- Database: Aurora MySQL writer (`cluster_endpoint`) as `DB_HOST`. Magento sees host/port/name from env and username/password from Secrets Manager. RDS vs Aurora is an infra switch; Magento still uses the writer endpoint. That shop's engine is Aurora 8.4; MageLift's Adobe gate is still Aurora 3.11/3.12 unless product policy changes.
- Search: provisioned OpenSearch Service domain in the VPC, not AOSS. FGAC off. Access policy Allow `es:ESHttp*` for `AWS *` because ElasticSuite does not SigV4. ECS sets `OPENSEARCH_HOST=https://<domain_endpoint>` and `OPENSEARCH_PORT=443`. No signing sidecar.
- AOSS serverless is not that shop. Magento OSS still cannot sign SigV4, so MageLift `searchMode:serverless` needs a local SigV4 proxy. Do not put that sidecar on the provisioned domain path.

Look at `infrastructure/opensearch.tf`, `infrastructure/aurora.tf`, `infrastructure/locals.tf` (`DB_HOST` / `OPENSEARCH_*`), and `infrastructure/docker/scripts/generate-env.php` if you need the example. MageLift adapter tests remain the product contract.
