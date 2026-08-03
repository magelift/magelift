---
type: lesson
title: MageLift ECS nginx-fpm sidecar contract 2026-07-18
description: 'The certified nginx-fpm runtime uses two ECS containers from the same immutable application
  image: a php-fpm container and an nginx HTTP sidecar on port 8080.'
tags:
- aws
- ecs
- nginx
- php-fpm
- security
generated:
  at: '2026-07-24'
---

The certified nginx-fpm runtime uses two ECS containers from the same immutable application image: a php-fpm container and an nginx HTTP sidecar on port 8080. They share the task network namespace and task-scoped writable mounts; only nginx is attached to the ALB. FrankenPHP classic remains a single web container. Web tasks use the application task role, while deploy tasks use the deployment role.
