---
type: lesson
title: MageLift Magento operation ECS container selection 2026-07-18
description: Magento operational commands must run through ECS Exec in a container with PHP.
tags:
- magelift
- cli
- ecs
- frankenphp
- nginx-fpm
- operations
generated:
  at: '2026-07-24'
---

Magento operational commands must run through ECS Exec in a container with PHP. nginx-fpm tasks expose a web nginx sidecar and a php-fpm sidecar, so cache, index, cron, and queue commands target php-fpm. FrankenPHP classic tasks run PHP in their single web container, so those commands target web. The CLI chooses the container from the resolved web runtime and tests both modes.
