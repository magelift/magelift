---
type: lesson
title: MageLift runtime capability identity split 2026-07-18
description: ECS runtime identity is created before managed capabilities so OpenSearch can use the task
  role as its access identity.
tags:
- aws
- ecs
- runtime
- secrets
status: stable
generated:
  at: '2026-07-24'
---

ECS runtime identity is created before managed capabilities so OpenSearch can use the task role as its access identity. Task definitions are created after capability outputs resolve. Application containers receive only non-secret endpoints, bucket names, queue mode, and database secret ARN; secret material stays in ECS secret references. Deployment tasks use the separate deployment role. Nginx sidecars receive no application capability configuration.
