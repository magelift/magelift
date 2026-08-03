---
type: lesson
title: MageLift bootstrap OIDC state-only role blocker 2026-07-18
description: Audit found generated CI assumes MAGELIFT_*_ROLE_ARN for Pulumi, ECS, and AWS managed resources,
  but bootstrap created only a GitHub OIDC role with S3 state, KMS, and SSM permissions.
tags:
- aws
- iam
- github-actions
- security
- release-blocker
generated:
  at: '2026-07-24'
---

Audit found generated CI assumes MAGELIFT_*_ROLE_ARN for Pulumi, ECS, and AWS managed resources, but bootstrap created only a GitHub OIDC role with S3 state, KMS, and SSM permissions. The ECS deployment role trusts ECS tasks, not GitHub. A fresh bootstrap therefore could not run generated CI. Fix requires separate state/recovery and CI infrastructure roles with explicit policies, not blindly widening the state policy.
