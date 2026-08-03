---
type: lesson
title: MageLift GitHub OIDC role lacks infrastructure permissions 2026-07-18
description: The bootstrap-created GitHub OIDC role (magelift-<project>-<environment>-deploy) is granted
  only Pulumi S3 state, KMS, and SSM metadata actions by internal/cloud/aws/bootstrap/identity.go.
tags:
- bootstrap
- iam
- github-oidc
- ci
- security
- finding
generated:
  at: '2026-07-24'
---

The bootstrap-created GitHub OIDC role (magelift-<project>-<environment>-deploy) is granted only Pulumi S3 state, KMS, and SSM metadata actions by internal/cloud/aws/bootstrap/identity.go. Generated CI expects per-environment role ARNs and invokes Pulumi plus ECS deploy/preview. The runtime ECS deployment role trusts ecs-tasks.amazonaws.com, not GitHub OIDC. No code provisions or documents a second GitHub-trusted infrastructure role, so fresh bootstrap output cannot execute generated workflows. Safest fix is to split state/bootstrap identity from explicitly provisioned least-privilege infrastructure roles or require user-managed role ARNs and document the gate; avoid broad wildcard policy.
