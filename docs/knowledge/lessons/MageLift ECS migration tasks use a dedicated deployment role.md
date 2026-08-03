---
type: lesson
title: MageLift ECS migration tasks use a dedicated deployment role
description: The durable web, cron, and queue tasks use the application task role, while the one-off setup:upgrade
  candidate task uses a separate deployment role.
tags:
- aws
- ecs
- iam
- security
generated:
  at: '2026-07-24'
---

The durable web, cron, and queue tasks use the application task role, while the one-off setup:upgrade candidate task uses a separate deployment role. The AWS stack attaches the same narrowly scoped capability policy to both roles because migrations need media, database, search, cache, and secret access; keeping the role distinct preserves future least-privilege reduction.
