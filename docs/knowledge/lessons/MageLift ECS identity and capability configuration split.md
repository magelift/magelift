---
type: lesson
title: MageLift ECS identity and capability configuration split
description: AWS ECS runtime now supports a separate magelift:aws:EcsRuntimeIdentity component that creates
  execution, task, and deployment roles before capability resources.
tags:
- ecs
- pulumi
- capabilities
- secrets
- aws
status: stable
generated:
  at: '2026-07-24'
---

AWS ECS runtime now supports a separate magelift:aws:EcsRuntimeIdentity component that creates execution, task, and deployment roles before capability resources. OpenSearch can bind fine-grained access to Identity.TaskRoleARN, then runtime task definitions are created after database, cache, search, queue, and storage outputs resolve. runtime.CapabilityConfig accepts only non-secret endpoints and managed references: database writer, database secret ARN, cache/session endpoints, search endpoint, queue mode/endpoint, and media bucket. PHP-FPM and FrankenPHP receive these as stable MAGELIFT_* environment entries; the nginx sidecar receives none. Secret values remain ECS secret references. Deploy tasks use the deployment role, and runtime rejects identity/task secret-reference mismatches.
