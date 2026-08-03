---
type: lesson
title: MageLift nginx sidecar secret boundary 2026-07-18
description: The nginx sidecar in an nginx-fpm ECS task receives no Magento application secrets.
tags:
- aws
- ecs
- nginx
- secrets
- security
generated:
  at: '2026-07-24'
---

The nginx sidecar in an nginx-fpm ECS task receives no Magento application secrets. Only the sibling php-fpm container receives the configured Secrets Manager or SSM references; the task role remains shared by ECS task design.
