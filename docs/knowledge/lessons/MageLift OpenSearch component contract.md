---
type: lesson
title: MageLift OpenSearch component contract
description: The AWS search component token is magelift:aws:OpenSearch.
tags:
- pulumi
- aws
- opensearch
- security
status: stable
generated:
  at: '2026-07-24'
---

The AWS search component token is magelift:aws:OpenSearch. Preview uses a CLASSIC OpenSearch Serverless SEARCH collection group with zero minimum OCU, caller-bounded maximum indexing/search OCU, explicit cold-start acceptance, standby disabled, KMS encryption, a private VPC endpoint, non-public network policy, and ARN-scoped least-privilege data access. Standard and high-availability use provisioned encrypted VPC domains with caller-selected engine, data instance type/count, EBS type/size, and subnets. Both require HTTPS with Policy-Min-TLS-1-2-PFS-2023-10, node encryption, KMS disk encryption, IAM ARN fine-grained access, and no internal credentials. HA uses three zones, data node counts divisible by three, and caller-selected dedicated master type with exactly three masters.
