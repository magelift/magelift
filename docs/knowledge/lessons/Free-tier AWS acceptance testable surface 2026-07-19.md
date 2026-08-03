---
type: lesson
title: Free-tier AWS acceptance testable surface 2026-07-19
description: 'With free plan only: Floci covers offline AWS contracts.'
tags:
- aws
- acceptance
- free-tier
- floci
generated:
  at: '2026-07-24'
---

With free plan only: Floci covers offline AWS contracts. Real AWS can verify bootstrap, OIDC roles, S3 DIY Pulumi backend, secrets, Route53+ACM DNS, budgets/SNS, ECR push, cosign sign/promote, magelift preview. Cannot CreateDBCluster aurora-mysql (only aurora-postgresql). Full deploy/health/destroy suite blocked until account leaves free plan or Magelift adds RDS MySQL preview mode. Prefer fck-nat later to cut NAT cost.
